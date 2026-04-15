package monitor

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Store 是监控模块的核心状态管理器，负责维护和协调所有与前端交互相关的状态数据，提供线程安全的访问接口。它的职责包括：
// 1. 持久化管理：负责将用户设置和监控数据持久化到磁盘，并在应用启动时装载这些数据，确保用户配置的连续性。
// 2. 运行时状态维护：维护当前的监控状态、历史数据和汇率信息，供前端仪表盘展示和交互使用。
// 3. 依赖协调：协调行情提供者、历史数据提供者和日志系统等多个组件，确保它们的数据能够正确地反映在前端。
// 4. 锁管理：通过读写锁机制确保在多线程环境下对状态的安全访问，避免数据竞争和不一致。
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
		runtime:            RuntimeStatus{AppVersion: appVersion},
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

		// 将汇率获取结果写入运行时状态并记录日志。
		if fxErr := s.fxRates.LastError(); fxErr != "" {
			s.mu.Lock()
			s.runtime.LastFxError = fxErr
			s.logWarn("fx-rates", fxErr)
			s.mu.Unlock()
		} else {
			validAt := s.fxRates.ValidAt()
			s.mu.Lock()
			s.runtime.LastFxError = ""
			s.runtime.LastFxRefreshAt = ptrTime(validAt)
			s.logInfo("fx-rates", fmt.Sprintf("汇率已刷新，共 %d 个币种", s.fxRates.CurrencyCount()))
			s.mu.Unlock()
		}
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
