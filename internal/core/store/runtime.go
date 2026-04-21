package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"investgo/internal/common/errs"
	"investgo/internal/core"
)

type quoteRefreshResult struct {
	attemptedAt time.Time
	quotes      map[string]core.Quote
	problems    []string
	fxFetched   bool
}

// Refresh refreshes real-time quotes and alert status, but does not touch historical trend cache.
// This allows the frontend to fetch charts on demand instead of repackaging historical data into the baseline snapshot on every refresh.
func (s *Store) Refresh(ctx context.Context, force bool) (core.StateSnapshot, error) {
	if !force {
		if cached, _, ok := s.refreshCache.Get("all"); ok {
			return cloneStateSnapshot(cached), nil
		}
	}

	// First copy current items slice to avoid holding read lock for extended period during network requests.
	s.mu.RLock()
	items := append([]core.WatchlistItem(nil), s.state.Items...)
	s.mu.RUnlock()

	result := s.refreshQuotesForItems(ctx, items)

	s.mu.Lock()
	defer s.mu.Unlock()

	s.runtime.LastQuoteAttemptAt = ptrTime(result.attemptedAt)
	s.runtime.LastQuoteError = ""
	s.runtime.QuoteSource = s.quoteProviderSummaryLocked()

	if len(result.quotes) > 0 {
		// Match results by normalized target key to handle symbol format variations between user input and provider responses.
		for idx := range s.state.Items {
			target, err := core.ResolveQuoteTarget(s.state.Items[idx])
			if err != nil {
				continue
			}
			quote, ok := result.quotes[target.Key]
			if !ok {
				continue
			}
			applyQuoteToItem(&s.state.Items[idx], quote)
		}
		s.runtime.LastQuoteRefreshAt = ptrTime(time.Now())
	}

	if fetchErr := errs.JoinProblems(result.problems); fetchErr != nil {
		s.runtime.LastQuoteError = fetchErr.Error()
		s.logWarn("quotes", fmt.Sprintf("quote refresh failed: %v", fetchErr))
	}

	// Update FX rate runtime status.
	if result.fxFetched {
		if fxErr := s.fxRates.LastError(); fxErr != "" {
			s.runtime.LastFxError = fxErr
			s.logWarn("fx-rates", fxErr)
		} else {
			s.runtime.LastFxError = ""
			s.runtime.LastFxRefreshAt = ptrTime(s.fxRates.ValidAt())
			s.logInfo("fx-rates", fmt.Sprintf("FX rates refreshed for %d currencies", s.fxRates.CurrencyCount()))
		}
	}

	s.evaluateAlertsLocked()
	s.state.UpdatedAt = time.Now()
	// Price refreshes do not change portfolio structure, so only the quote-result caches
	// are invalidated. The history and overview caches remain valid across price ticks.
	s.invalidatePriceCachesLocked()
	if err := s.saveLocked(); err != nil {
		s.logError("storage", fmt.Sprintf("save state failed after quote refresh: %v", err))
		return core.StateSnapshot{}, err
	}

	snapshot := s.snapshotLocked()
	s.refreshCache.Set("all", cloneStateSnapshot(snapshot), s.quoteRefreshTTLLocked())
	return snapshot, nil
}

// RefreshItem refreshes only one tracked instrument so view-local refresh flows can avoid sending the full watchlist to upstream providers.
func (s *Store) RefreshItem(ctx context.Context, itemID string, force bool) (core.StateSnapshot, error) {
	if !force {
		if cached, _, ok := s.itemRefreshCache.Get(itemID); ok {
			snapshot := cloneStateSnapshot(cached)
			return snapshot, nil
		}
	}

	s.mu.RLock()
	index := s.findItemIndexLocked(itemID)
	if index == -1 {
		s.mu.RUnlock()
		return core.StateSnapshot{}, fmt.Errorf("Item not found: %s", itemID)
	}
	item := s.state.Items[index]
	s.mu.RUnlock()

	result := s.refreshQuotesForItems(ctx, []core.WatchlistItem{item})

	s.mu.Lock()
	defer s.mu.Unlock()

	s.runtime.LastQuoteAttemptAt = ptrTime(result.attemptedAt)
	s.runtime.LastQuoteError = ""
	s.runtime.QuoteSource = s.quoteProviderSummaryLocked()

	if target, err := core.ResolveQuoteTarget(item); err == nil {
		if quote, ok := result.quotes[target.Key]; ok {
			index = s.findItemIndexLocked(itemID)
			if index >= 0 {
				applyQuoteToItem(&s.state.Items[index], quote)
				s.runtime.LastQuoteRefreshAt = ptrTime(time.Now())
			}
		}
	}

	if fetchErr := errs.JoinProblems(result.problems); fetchErr != nil {
		s.runtime.LastQuoteError = fetchErr.Error()
		s.logWarn("quotes", fmt.Sprintf("quote refresh failed: %v", fetchErr))
	}

	if result.fxFetched {
		if fxErr := s.fxRates.LastError(); fxErr != "" {
			s.runtime.LastFxError = fxErr
			s.logWarn("fx-rates", fxErr)
		} else {
			s.runtime.LastFxError = ""
			s.runtime.LastFxRefreshAt = ptrTime(s.fxRates.ValidAt())
			s.logInfo("fx-rates", fmt.Sprintf("FX rates refreshed for %d currencies", s.fxRates.CurrencyCount()))
		}
	}

	s.evaluateAlertsLocked()
	s.state.UpdatedAt = time.Now()
	s.invalidatePriceCachesLocked()
	if err := s.saveLocked(); err != nil {
		s.logError("storage", fmt.Sprintf("save state failed after single-item quote refresh: %v", err))
		return core.StateSnapshot{}, err
	}

	snapshot := s.snapshotLocked()
	s.itemRefreshCache.Set(itemID, cloneStateSnapshot(snapshot), s.quoteRefreshTTLLocked())
	return snapshot, nil
}

// refreshQuotesForItems batches items by their active market-specific provider so multi-market lists still respect per-market source settings.
func (s *Store) refreshQuotesForItems(ctx context.Context, items []core.WatchlistItem) quoteRefreshResult {
	result := quoteRefreshResult{
		attemptedAt: time.Now(),
		quotes:      map[string]core.Quote{},
	}

	// Refresh FX opportunistically alongside quote requests so derived dashboard values stay aligned after quote updates.
	if s.fxRates.IsStale() {
		s.fxRates.Fetch(ctx)
		result.fxFetched = true
	}

	if len(items) == 0 {
		return result
	}

	grouped := make(map[string][]core.WatchlistItem)
	for _, item := range items {
		s.mu.RLock()
		sourceID := s.activeQuoteSourceIDLocked(item.Market)
		provider := s.activeQuoteProviderLocked(item.Market)
		s.mu.RUnlock()
		if provider == nil || sourceID == "" {
			continue
		}
		grouped[sourceID] = append(grouped[sourceID], item)
	}

	for sourceID, batch := range grouped {
		provider := s.quoteProviders[sourceID]
		if provider == nil {
			continue
		}
		batchQuotes, err := provider.Fetch(ctx, batch)
		if err != nil {
			result.problems = append(result.problems, fmt.Sprintf("%s: %v", provider.Name(), err))
		}
		for key, quote := range batchQuotes {
			result.quotes[key] = quote
		}
	}

	return result
}

// ItemHistory queries historical price data for the specified item.
// Routing to the appropriate data source is handled by the historyProvider (HistoryRouter),
// which selects and sequences providers based on the item market and user settings.
func (s *Store) ItemHistory(ctx context.Context, itemID string, interval core.HistoryInterval, force bool) (core.HistorySeries, error) {
	cacheKey := itemID + "|" + string(interval)
	if !force {
		if cached, expiresAt, ok := s.historyCache.Get(cacheKey); ok {
			series := cloneHistorySeries(cached)
			series.Cached = true
			series.CacheExpiresAt = ptrTime(expiresAt)
			return series, nil
		}
	}

	s.mu.RLock()
	index := s.findItemIndexLocked(itemID)
	if index == -1 {
		s.mu.RUnlock()
		return core.HistorySeries{}, fmt.Errorf("Item not found: %s", itemID)
	}
	item := s.state.Items[index]
	s.mu.RUnlock()

	if s.historyProvider == nil {
		return core.HistorySeries{}, errors.New("History provider is not configured")
	}

	series, err := s.historyProvider.Fetch(ctx, item, interval)
	if err != nil {
		return core.HistorySeries{}, err
	}
	series.Snapshot = buildMarketSnapshot(decorateItemDerived(item), series)
	series.Cached = false
	// History OHLCV data is stable within each interval window; use a longer
	// per-interval TTL rather than the short HotCacheTTLSeconds setting.
	expiresAt := s.historyCache.Set(cacheKey, cloneHistorySeries(series), historyCacheTTLForInterval(interval))
	series.CacheExpiresAt = ptrTime(expiresAt)
	return series, nil
}

// OverviewAnalytics builds the overview analytics payload used by the dashboard overview module.
func (s *Store) OverviewAnalytics(ctx context.Context, force bool) (core.OverviewAnalytics, error) {
	s.mu.RLock()
	items := append([]core.WatchlistItem(nil), s.state.Items...)
	displayCurrency := s.state.Settings.DashboardCurrency
	// stateStamp guards against a race where a structural mutation (item add/remove/update,
	// settings change) happens concurrently with an in-flight overview build: if the result
	// is cached with an old stamp the next caller sees a miss and rebuilds. In the common
	// case invalidatePriceCachesLocked clears overviewCache after every price refresh, so
	// overviewCache.Get returns ok=false before this stamp comparison matters.
	stateStamp := s.holdingsUpdatedAt
	s.mu.RUnlock()
	if !force {
		if cached, expiresAt, ok := s.overviewCache.Get("all"); ok && cached.stateStamp.Equal(stateStamp) {
			analytics := cloneOverviewAnalytics(cached.analytics)
			analytics.Cached = true
			analytics.CacheExpiresAt = ptrTime(expiresAt)
			return analytics, nil
		}
	}

	relevantItems := make([]core.WatchlistItem, 0, len(items))
	for _, item := range items {
		if item.Quantity > 0 || len(validOverviewDCAEntries(item.DCAEntries)) > 0 {
			relevantItems = append(relevantItems, item)
		}
	}

	// Route history through ItemHistory so the shared historyCache is used.
	// This makes overview rebuilds cheap (no extra network calls) when history
	// is already cached, which is the common case after the user loads the chart
	// for any holding. Bypassing historyProvider.Fetch directly would circumvent
	// the cache and issue redundant network requests on every price refresh.
	calculator := newOverviewCalculator(s.fxRates, displayCurrency, func(ctx context.Context, item core.WatchlistItem, interval core.HistoryInterval) (core.HistorySeries, error) {
		return s.ItemHistory(ctx, item.ID, interval, false)
	})

	analytics, err := calculator.Build(ctx, relevantItems)
	if err != nil {
		return core.OverviewAnalytics{}, err
	}
	analytics.Cached = false

	expiresAt := s.overviewCache.Set("all", cachedOverviewValue{
		analytics:  cloneOverviewAnalytics(analytics),
		stateStamp: stateStamp,
	}, s.derivedCacheTTL())
	analytics.CacheExpiresAt = ptrTime(expiresAt)

	return analytics, nil
}
