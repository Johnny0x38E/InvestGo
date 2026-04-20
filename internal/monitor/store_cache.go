package monitor

import "time"

type cachedOverviewValue struct {
	analytics  OverviewAnalytics
	stateStamp time.Time
}

// cachedSnapshot pairs a built StateSnapshot with the state stamp it was derived from.
type cachedSnapshot struct {
	stamp    time.Time
	snapshot StateSnapshot
}

// quoteRefreshTTL returns the TTL used when caching a quote-refresh result.
// It delegates to derivedCacheTTLLocked so that all data caches (watchlist
// quotes, hot list rankings, portfolio overview) are controlled by the single
// unified CacheTTLSeconds setting. Auto-refresh always forces a live fetch
// and is not subject to this TTL.
func (s *Store) quoteRefreshTTL() time.Duration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.derivedCacheTTLLocked()
}

func (s *Store) quoteRefreshTTLLocked() time.Duration {
	return s.derivedCacheTTLLocked()
}

// derivedCacheTTL returns the unified TTL for all non-history data caches:
// watchlist quote-refresh results, hot list rankings, and portfolio overview analytics.
// Historical OHLCV series use per-interval TTLs via historyCacheTTLForInterval instead.
func (s *Store) derivedCacheTTL() time.Duration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.derivedCacheTTLLocked()
}

func (s *Store) derivedCacheTTLLocked() time.Duration {
	seconds := s.state.Settings.HotCacheTTLSeconds
	if seconds < 10 {
		seconds = 60
	}
	return time.Duration(seconds) * time.Second
}

// historyCacheTTLForInterval returns an appropriate TTL for the given history interval.
// Historical OHLCV data changes much less frequently than live quote rankings, so a
// per-interval TTL is used instead of the shorter HotCacheTTLSeconds setting.
func historyCacheTTLForInterval(interval HistoryInterval) time.Duration {
	switch interval {
	case HistoryRange1h:
		return 5 * time.Minute
	case HistoryRange1d:
		return 10 * time.Minute
	case HistoryRange1w, HistoryRange1mo:
		return 30 * time.Minute
	default: // 1y, 3y, all
		return 4 * time.Hour
	}
}

// invalidateAllCachesLocked clears every cache layer. Call this after structural mutations
// (item add/remove/update, settings change) so derived data is rebuilt from scratch.
func (s *Store) invalidateAllCachesLocked() {
	s.refreshCache.Clear()
	s.itemRefreshCache.Clear()
	s.historyCache.Clear()
	s.overviewCache.Clear()
	s.snapshotCache.Store(nil)
}

// invalidatePriceCachesLocked clears the quote-result caches and the overview cache.
// Call this after a routine price refresh so that the next Snapshot / RefreshItem call
// re-reads live prices. The overview cache is also cleared because portfolio values
// (allocation weights, current position values) change with every price tick; history
// rebuilds cheaply via historyCache so this does not trigger expensive network fetches.
// The historyCache itself is left intact — historical OHLCV data is unaffected by the
// current price tick.
func (s *Store) invalidatePriceCachesLocked() {
	s.refreshCache.Clear()
	s.itemRefreshCache.Clear()
	s.overviewCache.Clear()
	s.snapshotCache.Store(nil)
}

func cloneStateSnapshot(snapshot StateSnapshot) StateSnapshot {
	out := snapshot
	out.Items = append([]WatchlistItem(nil), snapshot.Items...)
	out.Alerts = append([]AlertRule(nil), snapshot.Alerts...)
	out.QuoteSources = append([]QuoteSourceOption(nil), snapshot.QuoteSources...)
	return out
}

func cloneHistorySeries(series HistorySeries) HistorySeries {
	out := series
	out.Points = append([]HistoryPoint(nil), series.Points...)
	if series.Snapshot != nil {
		snapshot := *series.Snapshot
		out.Snapshot = &snapshot
	}
	return out
}

func cloneOverviewAnalytics(analytics OverviewAnalytics) OverviewAnalytics {
	out := analytics
	out.Breakdown = append([]OverviewHoldingSlice(nil), analytics.Breakdown...)
	out.Trend.Dates = append([]time.Time(nil), analytics.Trend.Dates...)
	out.Trend.Series = make([]OverviewTrendSeries, len(analytics.Trend.Series))
	for index, series := range analytics.Trend.Series {
		cloned := series
		cloned.Values = append([]float64(nil), series.Values...)
		out.Trend.Series[index] = cloned
	}
	if analytics.Trend.StartDate != nil {
		start := *analytics.Trend.StartDate
		out.Trend.StartDate = &start
	}
	if analytics.Trend.EndDate != nil {
		end := *analytics.Trend.EndDate
		out.Trend.EndDate = &end
	}
	return out
}
