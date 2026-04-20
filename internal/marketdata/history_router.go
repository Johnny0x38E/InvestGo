package marketdata

import (
	"context"
	"fmt"
	"strings"

	"investgo/internal/monitor"
)

// HistoryRouter is the market-aware history data dispatch layer.
//
// It routes each history request to the most suitable underlying provider
// based on two inputs:
//  1. The item market (CN / HK / US) determines which providers are technically
//     capable of serving that market.
//  2. The user per-market quote source setting (CNQuoteSource / HKQuoteSource /
//     USQuoteSource) is respected as the first-choice provider when that source
//     also has history capability.
//
// Quote sources that have no dedicated K-line / chart API (Sina, Xueqiu) are
// automatically skipped and the next provider in the market priority chain is
// tried, so charts are always available regardless of the user quote source
// selection.
type HistoryRouter struct {
	// providers holds the concrete history providers keyed by source ID.
	// Only register sources that have an actual K-line or chart API
	// (currently eastmoney and yahoo).
	providers map[string]monitor.HistoryProvider

	// settings returns the latest application settings at call time.
	// Using a function ensures the router always sees current user preferences
	// without being recreated when settings change.
	settings func() monitor.AppSettings
}

// NewHistoryRouter creates a HistoryRouter.
//
//   - providers maps source IDs to concrete HistoryProvider implementations.
//     Register only sources that have K-line or chart history capability.
//   - settings is called on every Fetch to obtain the current user preferences.
//     Pass store.CurrentSettings after the Store has been initialised.
func NewHistoryRouter(providers map[string]monitor.HistoryProvider, settings func() monitor.AppSettings) *HistoryRouter {
	out := make(map[string]monitor.HistoryProvider, len(providers))
	for k, v := range providers {
		out[k] = v
	}
	return &HistoryRouter{providers: out, settings: settings}
}

// Name implements monitor.HistoryProvider.
func (r *HistoryRouter) Name() string { return "HistoryRouter" }

// Fetch implements monitor.HistoryProvider.
//
// It builds a provider chain for the item market (respecting user settings),
// tries each provider in order, and returns the first successful result.
// When every provider in the chain fails the returned error names each provider
// and its individual error for diagnostics.
func (r *HistoryRouter) Fetch(ctx context.Context, item monitor.WatchlistItem, interval monitor.HistoryInterval) (monitor.HistorySeries, error) {
	chain := r.chainForMarket(item.Market)

	var errs []string
	for _, id := range chain {
		p, ok := r.providers[id]
		if !ok {
			continue
		}
		series, err := p.Fetch(ctx, item, interval)
		if err == nil {
			return series, nil
		}
		errs = append(errs, fmt.Sprintf("[%s] %v", p.Name(), err))
	}

	if len(errs) > 0 {
		return monitor.HistorySeries{}, fmt.Errorf("all history providers failed for %s (%s): %s", item.Symbol, item.Market, strings.Join(errs, "; "))
	}
	return monitor.HistorySeries{}, fmt.Errorf("no history provider configured for market: %s", item.Market)
}

// chainForMarket builds the priority-ordered list of provider IDs for the given
// market.
//
// Rules:
//  1. The user configured quote source for this market is placed first, but only
//     when that source is registered in the providers map (i.e. it has a K-line
//     API). Sources such as Sina and Xueqiu are quote-only and absent from the
//     providers map, so they are silently skipped.
//  2. The remaining slots are filled by the market-appropriate default chain,
//     excluding any ID already placed at position 1 to avoid duplicates.
func (r *HistoryRouter) chainForMarket(market string) []string {
	settings := r.settings()
	preferred := r.preferredSourceID(market, settings)
	defaults := defaultHistoryChain(market)

	if preferred == "" {
		return defaults
	}

	chain := make([]string, 0, len(defaults)+1)
	chain = append(chain, preferred)
	for _, id := range defaults {
		if id != preferred {
			chain = append(chain, id)
		}
	}
	return chain
}

// preferredSourceID returns the provider ID that aligns with the user configured
// quote source for the given market, provided that source is registered in the
// providers map (i.e. it has history capability).
// Returns an empty string when the configured source has no history API
// (e.g. sina, xueqiu).
func (r *HistoryRouter) preferredSourceID(market string, settings monitor.AppSettings) string {
	var configured string
	switch historyMarketGroup(market) {
	case "cn":
		configured = strings.ToLower(strings.TrimSpace(settings.CNQuoteSource))
	case "hk":
		configured = strings.ToLower(strings.TrimSpace(settings.HKQuoteSource))
	case "us":
		configured = strings.ToLower(strings.TrimSpace(settings.USQuoteSource))
	}

	if _, ok := r.providers[configured]; ok {
		return configured
	}
	return ""
}

// defaultHistoryChain returns the market-appropriate provider priority order
// used when the user preferred source has no history capability, or as the
// fallback tail after the preferred source.
//
// CN and HK: EastMoney K-line is preferred (richer candlestick granularity for
// domestic and Hong Kong markets).
// US: user-selected API-backed providers are respected when configured; the
// default chain still falls back to Yahoo Finance, then Finnhub, Polygon, and
// finally EastMoney so startup does not require API-key-backed sources.
func defaultHistoryChain(market string) []string {
	switch historyMarketGroup(market) {
	case "us":
		return []string{"yahoo", "finnhub", "polygon", "alpha-vantage", "twelve-data", "eastmoney"}
	default:
		return []string{"yahoo", "eastmoney"}
	}
}

// historyMarketGroup maps a detailed market identifier to a broad group used
// for routing decisions inside this package.
func historyMarketGroup(market string) string {
	switch market {
	case "US-STOCK", "US-ETF":
		return "us"
	case "HK-MAIN", "HK-GEM", "HK-ETF":
		return "hk"
	default:
		return "cn"
	}
}
