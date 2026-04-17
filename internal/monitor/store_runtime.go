package monitor

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Refresh refreshes real-time quotes and alert status, but does not touch historical trend cache.
// This allows the frontend to fetch charts on demand instead of repackaging historical data into the baseline snapshot on every refresh.
func (s *Store) Refresh(ctx context.Context) (StateSnapshot, error) {
	// First copy current items slice to avoid holding read lock for extended period during network requests.
	s.mu.RLock()
	items := append([]WatchlistItem(nil), s.state.Items...)
	s.mu.RUnlock()

	attemptedAt := time.Now()
	quotes := map[string]Quote{}
	var problems []string

	// Also refresh FX rate cache (if expired), in parallel with quote refresh, not participating in Store lock lifecycle.
	var fxFetched bool
	if s.fxRates.IsStale() {
		s.fxRates.Fetch(ctx)
		fxFetched = true
	}

	if len(items) > 0 {
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
			batchQuotes, err := provider.Fetch(ctx, batch)
			if err != nil {
				problems = append(problems, fmt.Sprintf("%s: %v", provider.Name(), err))
			}
			for key, quote := range batchQuotes {
				quotes[key] = quote
			}
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.runtime.LastQuoteAttemptAt = ptrTime(attemptedAt)
	s.runtime.LastQuoteError = ""
	s.runtime.QuoteSource = s.quoteProviderSummaryLocked()

	if len(quotes) > 0 {
		// Match results by normalized target key to handle symbol format variations between user input and provider responses.
		for idx := range s.state.Items {
			target, err := ResolveQuoteTarget(s.state.Items[idx])
			if err != nil {
				continue
			}
			quote, ok := quotes[target.Key]
			if !ok {
				continue
			}
			applyQuoteToItem(&s.state.Items[idx], quote)
		}
		s.runtime.LastQuoteRefreshAt = ptrTime(time.Now())
	}

	if fetchErr := joinProblems(problems); fetchErr != nil {
		s.runtime.LastQuoteError = fetchErr.Error()
		s.logWarn("quotes", fmt.Sprintf("quote refresh failed: %v", fetchErr))
	}

	// Update FX rate runtime status.
	if fxFetched {
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

// ItemHistory queries historical trend for the specified item and delegates to history provider implementation.
func (s *Store) ItemHistory(ctx context.Context, itemID string, interval HistoryInterval) (HistorySeries, error) {
	s.mu.RLock()
	index := s.findItemIndexLocked(itemID)
	if index == -1 {
		s.mu.RUnlock()
		return HistorySeries{}, fmt.Errorf("Item not found: %s", itemID)
	}
	item := s.state.Items[index]
	providers := s.historyProviderCandidatesLocked(item.Market)
	s.mu.RUnlock()

	if len(providers) == 0 {
		return HistorySeries{}, errors.New("History provider is not configured")
	}

	var problems []string
	for _, provider := range providers {
		series, err := provider.Fetch(ctx, item, interval)
		if err == nil {
			series.Snapshot = buildMarketSnapshot(decorateItemDerived(item), series)
			return series, nil
		}
		problems = append(problems, fmt.Sprintf("%s: %v", provider.Name(), err))
	}
	return HistorySeries{}, joinProblems(problems)
}

// OverviewAnalytics builds the overview analytics payload used by the dashboard overview module.
func (s *Store) OverviewAnalytics(ctx context.Context) (OverviewAnalytics, error) {
	s.mu.RLock()
	items := append([]WatchlistItem(nil), s.state.Items...)
	displayCurrency := s.state.Settings.DashboardCurrency
	s.mu.RUnlock()

	calculator := newOverviewCalculator(s.fxRates, displayCurrency, func(ctx context.Context, item WatchlistItem, interval HistoryInterval) (HistorySeries, error) {
		s.mu.RLock()
		providers := s.historyProviderCandidatesLocked(item.Market)
		s.mu.RUnlock()
		if len(providers) == 0 {
			return HistorySeries{}, errors.New("History provider is not configured")
		}

		var problems []string
		for _, provider := range providers {
			series, err := provider.Fetch(ctx, item, interval)
			if err == nil {
				return series, nil
			}
			problems = append(problems, fmt.Sprintf("%s: %v", provider.Name(), err))
		}
		return HistorySeries{}, joinProblems(problems)
	})

	return calculator.Build(ctx, items)
}
