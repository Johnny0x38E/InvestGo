package monitor

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	ttlcache "investgo/internal/cache"
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
	historyProvider    HistoryProvider
	logs               *LogBook
	state              persistedState
	runtime            RuntimeStatus
	fxRates            *FxRates
	refreshCache       *ttlcache.TTL[string, StateSnapshot]
	itemRefreshCache   *ttlcache.TTL[string, StateSnapshot]
	historyCache       *ttlcache.TTL[string, HistorySeries]
	overviewCache      *ttlcache.TTL[string, cachedOverviewValue]
	// holdingsUpdatedAt tracks the last time portfolio holdings changed structurally
	// (item add/remove/update, settings change). It is used as the stateStamp for the
	// overviewCache to detect structural changes. The overviewCache is also explicitly
	// cleared by invalidatePriceCachesLocked so portfolio values always reflect the
	// latest prices after any quote refresh.
	holdingsUpdatedAt  time.Time
	// snapshotCache holds the last built StateSnapshot so repeated Snapshot() calls
	// (e.g. every /api/state request) avoid re-sorting and re-decorating all items
	// when nothing in the persisted state has changed.
	snapshotCache      atomic.Pointer[cachedSnapshot]
}

// NewStore creates a Store and completes state loading and runtime dependency injection.
func NewStore(path string, quoteProviders map[string]QuoteProvider, quoteSourceOptions []QuoteSourceOption, historyProvider HistoryProvider, logs *LogBook, appVersion string, httpClient *http.Client) (*Store, error) {
	return NewStoreWithRepository(
		persistence.NewJSONRepository(path),
		quoteProviders,
		quoteSourceOptions,
		historyProvider,
		logs,
		appVersion,
		httpClient,
	)
}

// NewStoreWithRepository creates a Store with an explicit persistence backend.
func NewStoreWithRepository(repository persistence.Repository, quoteProviders map[string]QuoteProvider, quoteSourceOptions []QuoteSourceOption, historyProvider HistoryProvider, logs *LogBook, appVersion string, httpClient *http.Client) (*Store, error) {
	store := &Store{
		repository:         repository,
		quoteProviders:     quoteProviders,
		quoteSourceOptions: append([]QuoteSourceOption(nil), quoteSourceOptions...),
		historyProvider:    historyProvider,
		logs:               logs,
		fxRates:            NewFxRates(httpClient), // use shared http.Client so proxy transport settings apply to FX rate requests
		runtime:            RuntimeStatus{AppVersion: appVersion},
		refreshCache:       ttlcache.NewTTLWithMax[string, StateSnapshot](32),
		itemRefreshCache:   ttlcache.NewTTLWithMax[string, StateSnapshot](32),
		historyCache:       ttlcache.NewTTLWithMax[string, HistorySeries](512),
		overviewCache:      ttlcache.NewTTLWithMax[string, cachedOverviewValue](16),
	}
	if err := store.load(); err != nil {
		return nil, err
	}
	store.holdingsUpdatedAt = store.state.UpdatedAt

	// Kick off an initial FX rate fetch in the background so Snapshot() never blocks
	// waiting for network on the first call.
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		store.fxRates.Fetch(ctx)
		store.mu.Lock()
		if fxErr := store.fxRates.LastError(); fxErr != "" {
			store.runtime.LastFxError = fxErr
			store.logWarn("fx-rates", fxErr)
		} else if validAt := store.fxRates.ValidAt(); !validAt.IsZero() {
			store.runtime.LastFxError = ""
			store.runtime.LastFxRefreshAt = ptrTime(validAt)
			store.logInfo("fx-rates", fmt.Sprintf("FX rates ready (%d currencies)", store.fxRates.CurrencyCount()))
		}
		store.mu.Unlock()
	}()

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
// FX rates are fetched asynchronously in the background; this method never blocks on network.
func (s *Store) Snapshot() StateSnapshot {
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
