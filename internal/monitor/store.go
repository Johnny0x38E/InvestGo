package monitor

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Store 是桌面端的中心协调器：
// 1. 管理本地持久化状态；
// 2. 协调实时行情与历史行情；
// 3. 输出适合前端直接消费的快照。
type Store struct {
	mu                 sync.RWMutex
	path               string
	quoteProviders     map[string]QuoteProvider
	quoteSourceOptions []QuoteSourceOption
	historyProviders   map[string]HistoryProvider
	logs               *LogBook
	state              persistedState
	runtime            RuntimeStatus
	fxRates            *FxRates
}

// NewStore 创建 Store，并完成状态装载与运行时依赖注入。
func NewStore(path string, quoteProviders map[string]QuoteProvider, quoteSourceOptions []QuoteSourceOption, historyProviders map[string]HistoryProvider, logs *LogBook, appVersion string) (*Store, error) {
	store := &Store{
		path:               path,
		quoteProviders:     quoteProviders,
		quoteSourceOptions: append([]QuoteSourceOption(nil), quoteSourceOptions...),
		historyProviders:   historyProviders,
		logs:               logs,
		fxRates:            NewFxRates(nil),
		runtime: RuntimeStatus{
			AppVersion: appVersion,
		},
	}
	if err := store.load(); err != nil {
		return nil, err
	}

	return store, nil
}

// Save 把当前内存状态持久化到磁盘。
func (s *Store) Save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	err := s.saveLocked()
	if err != nil {
		s.logError("storage", fmt.Sprintf("save state failed: %v", err))
	}
	return err
}

// Snapshot 返回前端启动和交互依赖的完整状态快照。
func (s *Store) Snapshot() StateSnapshot {
	// 汇率刷新不参与 Store 锁生命周期，避免慢网络放大临界区。
	if s.fxRates.IsStale() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		s.fxRates.Fetch(ctx)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.snapshotLocked()
}

// CurrentSettings 返回当前持久化设置的只读副本，供热门榜单等内部消费者使用。
func (s *Store) CurrentSettings() AppSettings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state.Settings
}
