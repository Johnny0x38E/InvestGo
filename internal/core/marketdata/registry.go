package marketdata

import (
	"net/http"

	"investgo/internal/core/provider"
	"investgo/internal/core"
)

// DataSource bundles the quote and history capabilities of a single market data
// provider under a common identifier.  Every registered source exposes at least
// one of QuoteProvider or HistoryProvider; sources that support both allow the
// user's per-market quote source setting to automatically govern history
// routing as well.
type DataSource struct {
	id      string
	name    string
	desc    string
	markets []string
	quote   core.QuoteProvider
	history core.HistoryProvider
}

// ID returns the unique identifier of this data source (e.g. "yahoo", "eastmoney").
func (ds *DataSource) ID() string { return ds.id }

// DisplayName returns the human-readable provider name.
func (ds *DataSource) DisplayName() string { return ds.name }

// Description returns a short description suitable for the settings UI.
func (ds *DataSource) Description() string { return ds.desc }

// SupportedMarkets returns the list of market identifiers this source covers.
func (ds *DataSource) SupportedMarkets() []string { return ds.markets }

// QuoteProvider returns the real-time quote provider, or nil if this source
// does not support live quotes.
func (ds *DataSource) QuoteProvider() core.QuoteProvider { return ds.quote }

// HistoryProvider returns the historical chart provider, or nil if this source
// does not support historical data.
func (ds *DataSource) HistoryProvider() core.HistoryProvider { return ds.history }

// HasQuote reports whether this source supports real-time quotes.
func (ds *DataSource) HasQuote() bool { return ds.quote != nil }

// HasHistory reports whether this source supports historical chart data.
func (ds *DataSource) HasHistory() bool { return ds.history != nil }

// Registry is the central registry of all market data sources.
//
// It is the single source of truth for provider capabilities and is used by
// the Store (for quote routing), the HistoryRouter (for history fallback
// chains), and the HotService (for quote overlays).  Providers are created
// once at startup and shared across all consumers.
type Registry struct {
	sources map[string]*DataSource
	order   []string // preserves registration order for UI display
}

// NewDataSource creates a DataSource for registration in the Registry.
// This constructor is the only way to create a DataSource from outside the
// marketdata package, since all DataSource fields are unexported.
func NewDataSource(id, name, desc string, markets []string, quote core.QuoteProvider, history core.HistoryProvider) *DataSource {
	return &DataSource{
		id:      id,
		name:    name,
		desc:    desc,
		markets: markets,
		quote:   quote,
		history: history,
	}
}

// NewRegistry creates an empty Registry.
func NewRegistry() *Registry {
	return &Registry{
		sources: make(map[string]*DataSource),
	}
}

// Register adds a DataSource to the registry.  If a source with the same ID
// already exists it is silently replaced.
func (r *Registry) Register(ds *DataSource) {
	if _, exists := r.sources[ds.id]; !exists {
		r.order = append(r.order, ds.id)
	}
	r.sources[ds.id] = ds
}

// Source returns the DataSource for the given ID, or nil if not registered.
func (r *Registry) Source(id string) *DataSource {
	return r.sources[id]
}

// QuoteProvider returns the QuoteProvider for the given source ID, or nil.
func (r *Registry) QuoteProvider(id string) core.QuoteProvider {
	if ds := r.sources[id]; ds != nil {
		return ds.quote
	}
	return nil
}

// HistoryProvider returns the HistoryProvider for the given source ID, or nil.
func (r *Registry) HistoryProvider(id string) core.HistoryProvider {
	if ds := r.sources[id]; ds != nil {
		return ds.history
	}
	return nil
}

// HasQuote reports whether the given source ID is registered and supports quotes.
func (r *Registry) HasQuote(id string) bool {
	ds := r.sources[id]
	return ds != nil && ds.quote != nil
}

// HasHistory reports whether the given source ID is registered and supports history.
func (r *Registry) HasHistory(id string) bool {
	ds := r.sources[id]
	return ds != nil && ds.history != nil
}

// QuoteProviders returns a map of all registered QuoteProviders keyed by
// source ID.  This is compatible with the Store constructor signature.
func (r *Registry) QuoteProviders() map[string]core.QuoteProvider {
	out := make(map[string]core.QuoteProvider, len(r.sources))
	for id, ds := range r.sources {
		if ds.quote != nil {
			out[id] = ds.quote
		}
	}
	return out
}

// HistoryProviders returns a map of all registered HistoryProviders keyed by
// source ID.  This is compatible with the HistoryRouter constructor.
func (r *Registry) HistoryProviders() map[string]core.HistoryProvider {
	out := make(map[string]core.HistoryProvider, len(r.sources))
	for id, ds := range r.sources {
		if ds.history != nil {
			out[id] = ds.history
		}
	}
	return out
}

// QuoteSourceOptions returns the ordered list of QuoteSourceOption descriptors
// for the settings UI.  This is compatible with the Store constructor signature.
func (r *Registry) QuoteSourceOptions() []core.QuoteSourceOption {
	out := make([]core.QuoteSourceOption, 0, len(r.order))
	for _, id := range r.order {
		ds := r.sources[id]
		if ds == nil || ds.quote == nil {
			continue
		}
		out = append(out, core.QuoteSourceOption{
			ID:               ds.id,
			Name:             ds.name,
			Description:      ds.desc,
			SupportedMarkets: ds.markets,
		})
	}
	return out
}

// NewHistoryRouter creates a HistoryRouter backed by all history-capable
// sources in this registry.
func (r *Registry) NewHistoryRouter(settings func() core.AppSettings) core.HistoryProvider {
	if settings == nil {
		settings = func() core.AppSettings { return core.AppSettings{} }
	}
	return NewHistoryRouter(r.HistoryProviders(), settings)
}

// IDs returns the ordered list of all registered source IDs.
func (r *Registry) IDs() []string {
	return append([]string(nil), r.order...)
}

// DefaultRegistry constructs the standard registry with all known market data
// providers.  The client is shared across all providers; settings is a lazy
// getter called at fetch time to read the current AppSettings (API keys and
// per-market source preferences).
func DefaultRegistry(client *http.Client, settings func() core.AppSettings) *Registry {
	if settings == nil {
		settings = func() core.AppSettings { return core.AppSettings{} }
	}

	r := NewRegistry()

	r.Register(&DataSource{
		id:   "eastmoney",
		name: "EastMoney",
		desc: "Best overall coverage for China, Hong Kong, and US markets with the most complete fields.",
		markets: []string{
			"CN-A", "CN-GEM", "CN-STAR", "CN-ETF",
			"HK-MAIN", "HK-GEM", "HK-ETF",
			"US-STOCK", "US-ETF",
		},
		quote:   provider.NewEastMoneyQuoteProvider(client),
		history: provider.NewEastMoneyChartProvider(client),
	})

	r.Register(&DataSource{
		id:   "yahoo",
		name: "Yahoo Finance",
		desc: "Stable coverage for Hong Kong and US markets, especially for overseas-focused portfolios.",
		markets: []string{
			"CN-A", "CN-GEM", "CN-STAR", "CN-ETF",
			"HK-MAIN", "HK-GEM", "HK-ETF",
			"US-STOCK", "US-ETF",
		},
		quote:   provider.NewYahooQuoteProvider(client),
		history: provider.NewYahooChartProvider(client),
	})

	r.Register(&DataSource{
		id:   "sina",
		name: "Sina Finance",
		desc: "Fast quote source exposed across China, Hong Kong, and US selections for direct comparison.",
		markets: []string{
			"CN-A", "CN-GEM", "CN-STAR", "CN-ETF",
			"HK-MAIN", "HK-GEM", "HK-ETF",
			"US-STOCK", "US-ETF",
		},
		quote: provider.NewSinaQuoteProvider(client),
		// Sina has no history API
	})

	r.Register(&DataSource{
		id:   "xueqiu",
		name: "Xueqiu",
		desc: "Quote source exposed across China, Hong Kong, and US selections for direct comparison.",
		markets: []string{
			"CN-A", "CN-GEM", "CN-STAR", "CN-ETF",
			"HK-MAIN", "HK-GEM", "HK-ETF",
			"US-STOCK", "US-ETF",
		},
		quote: provider.NewXueqiuQuoteProvider(client),
		// Xueqiu has no history API
	})

	r.Register(&DataSource{
		id:   "tencent",
		name: "Tencent Finance",
		desc: "Cross-market quote source with broad China, Hong Kong, and US coverage plus lightweight history endpoints.",
		markets: []string{
			"CN-A", "CN-GEM", "CN-STAR", "CN-ETF",
			"HK-MAIN", "HK-GEM", "HK-ETF",
			"US-STOCK", "US-ETF",
		},
		quote:   provider.NewTencentQuoteProvider(client),
		history: provider.NewTencentHistoryProvider(client),
	})

	r.Register(&DataSource{
		id:      "alpha-vantage",
		name:    "Alpha Vantage",
		desc:    "API-based US stock and ETF source with both live quote and history support.",
		markets: []string{"US-STOCK", "US-ETF"},
		quote:   provider.NewAlphaVantageQuoteProvider(client, settings),
		history: provider.NewAlphaVantageHistoryProvider(client, settings),
	})

	r.Register(&DataSource{
		id:      "twelve-data",
		name:    "Twelve Data",
		desc:    "API-based US stock and ETF source suited for using the same provider across quote and history flows.",
		markets: []string{"US-STOCK", "US-ETF"},
		quote:   provider.NewTwelveDataQuoteProvider(client, settings),
		history: provider.NewTwelveDataHistoryProvider(client, settings),
	})

	r.Register(&DataSource{
		id:      "finnhub",
		name:    "Finnhub",
		desc:    "API-based US stock and ETF source with both live quote and history support.",
		markets: []string{"US-STOCK", "US-ETF"},
		quote:   provider.NewFinnhubQuoteProvider(client, settings),
		history: provider.NewFinnhubHistoryProvider(client, settings),
	})

	r.Register(&DataSource{
		id:      "polygon",
		name:    "Polygon",
		desc:    "Polygon.io / Massive API source for US stocks and ETFs with real-time and history support.",
		markets: []string{"US-STOCK", "US-ETF"},
		quote:   provider.NewPolygonQuoteProvider(client, settings),
		history: provider.NewPolygonHistoryProvider(client, settings),
	})

	return r
}
