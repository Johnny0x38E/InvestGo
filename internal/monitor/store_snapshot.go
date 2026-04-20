package monitor

import (
	"sort"
	"strings"
	"time"
)

// snapshotLocked returns a read-only snapshot copy for frontend consumption.
//
// The result is cached by state stamp (state.UpdatedAt) so that repeated calls
// from read-only endpoints such as GET /api/state skip the sort and per-item
// decoration work when nothing in the persisted state has changed since the
// last build. The cache is invalidated by invalidateAllCachesLocked and
// invalidatePriceCachesLocked, both of which are called after every mutation
// and quote refresh respectively.
func (s *Store) snapshotLocked() StateSnapshot {
	stamp := s.state.UpdatedAt
	if entry := s.snapshotCache.Load(); entry != nil && entry.stamp.Equal(stamp) {
		// Fast path: state is unchanged, return a shallow clone so callers can
		// modify the top-level slices without aliasing the cached copy.
		return cloneStateSnapshot(entry.snapshot)
	}

	items := append([]WatchlistItem{}, s.state.Items...)
	alerts := append([]AlertRule{}, s.state.Alerts...)
	quoteSources := append([]QuoteSourceOption{}, s.quoteSourceOptions...)
	runtime := s.runtime
	runtime.QuoteSource = s.quoteProviderSummaryLocked()
	runtime.LivePriceCount = countLiveQuotes(items)

	// Snapshot sorting only affects output order, not internal persisted slice order.
	sort.Slice(items, func(i, j int) bool {
		if items[i].PinnedAt != nil || items[j].PinnedAt != nil {
			if items[i].PinnedAt == nil {
				return false
			}
			if items[j].PinnedAt == nil {
				return true
			}
			if !items[i].PinnedAt.Equal(*items[j].PinnedAt) {
				return items[i].PinnedAt.After(*items[j].PinnedAt)
			}
		}
		return items[i].UpdatedAt.After(items[j].UpdatedAt)
	})
	sort.Slice(alerts, func(i, j int) bool {
		if alerts[i].Triggered != alerts[j].Triggered {
			return alerts[i].Triggered
		}
		return alerts[i].UpdatedAt.After(alerts[j].UpdatedAt)
	})

	for index := range items {
		items[index] = decorateItemDerived(items[index])
	}

	snapshot := StateSnapshot{
		Dashboard:    buildDashboard(items, alerts, s.fxRates, s.state.Settings.DashboardCurrency),
		Items:        items,
		Alerts:       alerts,
		Settings:     s.state.Settings,
		Runtime:      runtime,
		QuoteSources: quoteSources,
		StoragePath:  s.repository.Path(),
		GeneratedAt:  time.Now(),
	}

	// Store into the atomic cache so subsequent read-only calls skip the rebuild.
	s.snapshotCache.Store(&cachedSnapshot{stamp: stamp, snapshot: cloneStateSnapshot(snapshot)})
	return snapshot
}

// buildDashboard builds dashboard summary data based on items, alerts, and FX rate information.
func buildDashboard(items []WatchlistItem, alerts []AlertRule, fx *FxRates, displayCurrency string) DashboardSummary {
	var summary DashboardSummary
	summary.ItemCount = len(items)

	if displayCurrency == "" {
		displayCurrency = "CNY"
	}
	summary.DisplayCurrency = displayCurrency

	// First convert each item's cost and market value to unified display currency, then perform portfolio aggregation.
	for _, item := range items {
		costBasis := item.CostBasis()
		marketValue := item.MarketValue()

		itemCurrency := strings.ToUpper(strings.TrimSpace(item.Currency))
		if fx != nil && itemCurrency != "" && itemCurrency != displayCurrency {
			costBasis = fx.Convert(costBasis, itemCurrency, displayCurrency)
			marketValue = fx.Convert(marketValue, itemCurrency, displayCurrency)
		}

		summary.TotalCost += costBasis
		summary.TotalValue += marketValue
		// Only items with an actual position (Quantity > 0) and a recorded cost price contribute to the win/loss tally.
		// Watch-only items and zero-cost DCA edge cases are excluded from this count.
		if item.Quantity > 0 && item.CostPrice > 0 {
			if item.CurrentPrice > item.CostPrice {
				summary.WinCount++
			} else if item.CurrentPrice < item.CostPrice {
				summary.LossCount++
			}
		}
	}

	summary.TotalPnL = summary.TotalValue - summary.TotalCost
	if summary.TotalCost > 0 {
		summary.TotalPnLPct = summary.TotalPnL / summary.TotalCost * 100
	}

	for _, alert := range alerts {
		if alert.Triggered {
			summary.TriggeredAlerts++
		}
	}

	return summary
}
