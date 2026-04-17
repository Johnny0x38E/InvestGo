package monitor

import (
	"context"
	"fmt"
	"sync"
	"time"

	"investgo/internal/monitor/persistence"
)

// Store is the core state manager for the monitoring module,
// responsible for maintaining and coordinating all state data related to frontend interaction,
// providing thread-safe access interfaces. Its responsibilities include:
// 1. Persistence management: Responsible for persisting user settings and monitoring data to disk, and loading this data on application startup to ensure continuity of user configuration.
// 2. Runtime state maintenance: Maintains current monitoring status, historical data, and FX (Foreign Exchange) rate information for frontend dashboard display and interaction.
// 3. Dependency coordination: Coordinates multiple components including quote providers, historical data providers, and logging systems to ensure their data is correctly reflected on the frontend.
// 4. Lock management: Ensures safe access to state in multi-threaded environments through read-write lock mechanisms, avoiding data races and inconsistencies.
type Store struct {
	mu                 sync.RWMutex
	repository         persistence.Repository
	quoteProviders     map[string]QuoteProvider
	quoteSourceOptions []QuoteSourceOption
	historyProviders   map[string]HistoryProvider
	logs               *LogBook
	state              persistedState
	runtime            RuntimeStatus
	fxRates            *FxRates
}

// NewStore creates a Store and completes state loading and runtime dependency injection.
func NewStore(path string, quoteProviders map[string]QuoteProvider, quoteSourceOptions []QuoteSourceOption, historyProviders map[string]HistoryProvider, logs *LogBook, appVersion string) (*Store, error) {
	return NewStoreWithRepository(
		persistence.NewJSONRepository(path),
		quoteProviders,
		quoteSourceOptions,
		historyProviders,
		logs,
		appVersion,
	)
}

// NewStoreWithRepository creates a Store with an explicit persistence backend.
func NewStoreWithRepository(repository persistence.Repository, quoteProviders map[string]QuoteProvider, quoteSourceOptions []QuoteSourceOption, historyProviders map[string]HistoryProvider, logs *LogBook, appVersion string) (*Store, error) {
	store := &Store{
		repository:         repository,
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

// Save persists current in-memory state to disk.
func (s *Store) Save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	err := s.saveLocked()
	if err != nil {
		s.logError("storage", fmt.Sprintf("save state failed: %v", err))
	}
	return err
}

// Snapshot returns a complete state snapshot required for frontend startup and interaction.
func (s *Store) Snapshot() StateSnapshot {
	if s.fxRates.IsStale() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		s.fxRates.Fetch(ctx)

		// Write FX rate fetch result to runtime status and log it
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
			s.logInfo("fx-rates", fmt.Sprintf("FX rates refreshed for %d currencies", s.fxRates.CurrencyCount()))
			s.mu.Unlock()
		}
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.snapshotLocked()
}

// CurrentSettings returns a read-only copy of current persisted settings for use by hot lists and other components.
func (s *Store) CurrentSettings() AppSettings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state.Settings
}
