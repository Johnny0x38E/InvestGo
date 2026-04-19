package monitor

import (
	"context"
	"errors"
	"fmt"
	"time"
)

type quoteRefreshResult struct {
	attemptedAt time.Time
	quotes      map[string]Quote
	problems    []string
	fxFetched   bool
}

// Refresh refreshes real-time quotes and alert status, but does not touch historical trend cache.
// This allows the frontend to fetch charts on demand instead of repackaging historical data into the baseline snapshot on every refresh.
func (s *Store) Refresh(ctx context.Context) (StateSnapshot, error) {
	// First copy current items slice to avoid holding read lock for extended period during network requests.
	s.mu.RLock()
	items := append([]WatchlistItem(nil), s.state.Items...)
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
			target, err := ResolveQuoteTarget(s.state.Items[idx])
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

	if fetchErr := joinProblems(result.problems); fetchErr != nil {
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
	if err := s.saveLocked(); err != nil {
		s.logError("storage", fmt.Sprintf("save state failed after quote refresh: %v", err))
		return StateSnapshot{}, err
	}

	return s.snapshotLocked(), nil
}

// RefreshItem refreshes only one tracked instrument so view-local refresh flows can avoid sending the full watchlist to upstream providers.
func (s *Store) RefreshItem(ctx context.Context, itemID string) (StateSnapshot, error) {
	s.mu.RLock()
	index := s.findItemIndexLocked(itemID)
	if index == -1 {
		s.mu.RUnlock()
		return StateSnapshot{}, fmt.Errorf("Item not found: %s", itemID)
	}
	item := s.state.Items[index]
	s.mu.RUnlock()

	result := s.refreshQuotesForItems(ctx, []WatchlistItem{item})

	s.mu.Lock()
	defer s.mu.Unlock()

	s.runtime.LastQuoteAttemptAt = ptrTime(result.attemptedAt)
	s.runtime.LastQuoteError = ""
	s.runtime.QuoteSource = s.quoteProviderSummaryLocked()

	if target, err := ResolveQuoteTarget(item); err == nil {
		if quote, ok := result.quotes[target.Key]; ok {
			index = s.findItemIndexLocked(itemID)
			if index >= 0 {
				applyQuoteToItem(&s.state.Items[index], quote)
				s.runtime.LastQuoteRefreshAt = ptrTime(time.Now())
			}
		}
	}

	if fetchErr := joinProblems(result.problems); fetchErr != nil {
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
	if err := s.saveLocked(); err != nil {
		s.logError("storage", fmt.Sprintf("save state failed after single-item quote refresh: %v", err))
		return StateSnapshot{}, err
	}

	return s.snapshotLocked(), nil
}

// refreshQuotesForItems batches items by their active market-specific provider so multi-market lists still respect per-market source settings.
func (s *Store) refreshQuotesForItems(ctx context.Context, items []WatchlistItem) quoteRefreshResult {
	result := quoteRefreshResult{
		attemptedAt: time.Now(),
		quotes:      map[string]Quote{},
	}

	// Refresh FX opportunistically alongside quote requests so derived dashboard values stay aligned after quote updates.
	if s.fxRates.IsStale() {
		s.fxRates.Fetch(ctx)
		result.fxFetched = true
	}

	if len(items) == 0 {
		return result
	}

	grouped := make(map[string][]WatchlistItem)
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
func (s *Store) ItemHistory(ctx context.Context, itemID string, interval HistoryInterval) (HistorySeries, error) {
	s.mu.RLock()
	index := s.findItemIndexLocked(itemID)
	if index == -1 {
		s.mu.RUnlock()
		return HistorySeries{}, fmt.Errorf("Item not found: %s", itemID)
	}
	item := s.state.Items[index]
	s.mu.RUnlock()

	if s.historyProvider == nil {
		return HistorySeries{}, errors.New("History provider is not configured")
	}

	series, err := s.historyProvider.Fetch(ctx, item, interval)
	if err != nil {
		return HistorySeries{}, err
	}
	series.Snapshot = buildMarketSnapshot(decorateItemDerived(item), series)
	return series, nil
}

// OverviewAnalytics builds the overview analytics payload used by the dashboard overview module.
func (s *Store) OverviewAnalytics(ctx context.Context) (OverviewAnalytics, error) {
	s.mu.RLock()
	items := append([]WatchlistItem(nil), s.state.Items...)
	displayCurrency := s.state.Settings.DashboardCurrency
	s.mu.RUnlock()

	calculator := newOverviewCalculator(s.fxRates, displayCurrency, func(ctx context.Context, item WatchlistItem, interval HistoryInterval) (HistorySeries, error) {
		if s.historyProvider == nil {
			return HistorySeries{}, errors.New("History provider is not configured")
		}
		return s.historyProvider.Fetch(ctx, item, interval)
	})

	return calculator.Build(ctx, items)
}
