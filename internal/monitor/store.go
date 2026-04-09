package monitor

import (
	"fmt"
	"sync"
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
	historyProvider    HistoryProvider
	logs               *LogBook
	state              persistedState
	runtime            RuntimeStatus
}

// NewStore 负责装载状态文件，并把运行时依赖挂到 Store 上。
func NewStore(path string, quoteProviders map[string]QuoteProvider, quoteSourceOptions []QuoteSourceOption, historyProvider HistoryProvider, logs *LogBook) (*Store, error) {
	store := &Store{
		path:               path,
		quoteProviders:     quoteProviders,
		quoteSourceOptions: append([]QuoteSourceOption(nil), quoteSourceOptions...),
		historyProvider:    historyProvider,
		logs:               logs,
	}
	if err := store.load(); err != nil {
		return nil, err
	}

	return store, nil
}

// Save 会把当前内存状态刷回磁盘。
func (s *Store) Save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	err := s.saveLocked()
	if err != nil {
		s.logError("storage", fmt.Sprintf("save state failed: %v", err))
	}
	return err
}

// Snapshot 返回前端启动和交互都依赖的完整视图快照。
func (s *Store) Snapshot() StateSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.snapshotLocked()
}
