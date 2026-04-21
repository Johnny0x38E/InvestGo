package store

import (
	"errors"
	"net/url"
	"strings"

	"investgo/internal/core"
)

// sanitiseSettings merges user input with current configuration and performs unified validation.
func sanitiseSettings(input core.AppSettings, current core.AppSettings, quoteProviders map[string]core.QuoteProvider, quoteSourceOptions []core.QuoteSourceOption) (core.AppSettings, error) {
	settings := current
	if input.HotCacheTTLSeconds > 0 {
		settings.HotCacheTTLSeconds = input.HotCacheTTLSeconds
	}
	if strings.TrimSpace(input.CNQuoteSource) != "" {
		settings.CNQuoteSource = strings.ToLower(strings.TrimSpace(input.CNQuoteSource))
	}
	if strings.TrimSpace(input.HKQuoteSource) != "" {
		settings.HKQuoteSource = strings.ToLower(strings.TrimSpace(input.HKQuoteSource))
	}
	if strings.TrimSpace(input.USQuoteSource) != "" {
		settings.USQuoteSource = strings.ToLower(strings.TrimSpace(input.USQuoteSource))
	}
	if strings.TrimSpace(input.ThemeMode) != "" {
		settings.ThemeMode = strings.ToLower(strings.TrimSpace(input.ThemeMode))
	}
	if strings.TrimSpace(input.ColorTheme) != "" {
		settings.ColorTheme = strings.ToLower(strings.TrimSpace(input.ColorTheme))
	}
	if strings.TrimSpace(input.FontPreset) != "" {
		settings.FontPreset = strings.ToLower(strings.TrimSpace(input.FontPreset))
	}
	if strings.TrimSpace(input.AmountDisplay) != "" {
		settings.AmountDisplay = strings.ToLower(strings.TrimSpace(input.AmountDisplay))
	}
	if strings.TrimSpace(input.CurrencyDisplay) != "" {
		settings.CurrencyDisplay = strings.ToLower(strings.TrimSpace(input.CurrencyDisplay))
	}
	if strings.TrimSpace(input.PriceColorScheme) != "" {
		settings.PriceColorScheme = strings.ToLower(strings.TrimSpace(input.PriceColorScheme))
	}
	if strings.TrimSpace(input.Locale) != "" {
		settings.Locale = strings.TrimSpace(input.Locale)
	}
	if strings.TrimSpace(input.ProxyMode) != "" {
		settings.ProxyMode = strings.ToLower(strings.TrimSpace(input.ProxyMode))
	}
	if input.ProxyURL != "" || strings.TrimSpace(current.ProxyURL) != "" {
		settings.ProxyURL = strings.TrimSpace(input.ProxyURL)
	}
	if input.AlphaVantageAPIKey != "" || strings.TrimSpace(current.AlphaVantageAPIKey) != "" {
		settings.AlphaVantageAPIKey = strings.TrimSpace(input.AlphaVantageAPIKey)
	}
	if input.TwelveDataAPIKey != "" || strings.TrimSpace(current.TwelveDataAPIKey) != "" {
		settings.TwelveDataAPIKey = strings.TrimSpace(input.TwelveDataAPIKey)
	}
	if input.FinnhubAPIKey != "" || strings.TrimSpace(current.FinnhubAPIKey) != "" {
		settings.FinnhubAPIKey = strings.TrimSpace(input.FinnhubAPIKey)
	}
	if input.PolygonAPIKey != "" || strings.TrimSpace(current.PolygonAPIKey) != "" {
		settings.PolygonAPIKey = strings.TrimSpace(input.PolygonAPIKey)
	}
	if strings.TrimSpace(input.DashboardCurrency) != "" {
		settings.DashboardCurrency = strings.ToUpper(strings.TrimSpace(input.DashboardCurrency))
	}
	settings.DeveloperMode = input.DeveloperMode
	settings.UseNativeTitleBar = input.UseNativeTitleBar

	if settings.HotCacheTTLSeconds < 10 {
		return core.AppSettings{}, errors.New("Cache TTL must be at least 10 seconds")
	}
	settings.CNQuoteSource = normaliseQuoteSourceIDForSettings(settings.CNQuoteSource, "CN-A", quoteProviders, quoteSourceOptions)
	settings.HKQuoteSource = normaliseQuoteSourceIDForSettings(settings.HKQuoteSource, "HK-MAIN", quoteProviders, quoteSourceOptions)
	settings.USQuoteSource = normaliseQuoteSourceIDForSettings(settings.USQuoteSource, "US-STOCK", quoteProviders, quoteSourceOptions)
	if len(quoteProviders) > 0 {
		if _, ok := quoteProviders[settings.CNQuoteSource]; !ok {
			return core.AppSettings{}, errors.New("China quote source is invalid")
		}
		if _, ok := quoteProviders[settings.HKQuoteSource]; !ok {
			return core.AppSettings{}, errors.New("Hong Kong quote source is invalid")
		}
		if _, ok := quoteProviders[settings.USQuoteSource]; !ok {
			return core.AppSettings{}, errors.New("US quote source is invalid")
		}
	}
	switch settings.USQuoteSource {
	case "alpha-vantage":
		if settings.AlphaVantageAPIKey == "" {
			return core.AppSettings{}, errors.New("Alpha Vantage API key is required")
		}
	case "twelve-data":
		if settings.TwelveDataAPIKey == "" {
			return core.AppSettings{}, errors.New("Twelve Data API key is required")
		}
	case "finnhub":
		if settings.FinnhubAPIKey == "" {
			return core.AppSettings{}, errors.New("Finnhub API key is required")
		}
	case "polygon":
		if settings.PolygonAPIKey == "" {
			return core.AppSettings{}, errors.New("Polygon API key is required")
		}
	}
	switch settings.FontPreset {
	case "", "system":
		settings.FontPreset = "system"
	case "reading", "compact":
	default:
		return core.AppSettings{}, errors.New("Font preset must be one of: system / reading / compact")
	}
	switch settings.ThemeMode {
	case "", "system":
		settings.ThemeMode = "system"
	case "light", "dark":
	default:
		return core.AppSettings{}, errors.New("Theme mode must be one of: system / light / dark")
	}
	switch settings.ColorTheme {
	case "", "blue":
		settings.ColorTheme = "blue"
	case "graphite", "forest", "sunset", "rose", "violet", "amber":
	default:
		return core.AppSettings{}, errors.New("Color theme must be one of: blue / graphite / forest / sunset / rose / violet / amber")
	}
	switch settings.AmountDisplay {
	case "", "full":
		settings.AmountDisplay = "full"
	case "compact":
	default:
		return core.AppSettings{}, errors.New("Amount display must be one of: full / compact")
	}
	switch settings.CurrencyDisplay {
	case "", "symbol":
		settings.CurrencyDisplay = "symbol"
	case "code":
	default:
		return core.AppSettings{}, errors.New("Currency display must be one of: symbol / code")
	}
	switch settings.PriceColorScheme {
	case "", "cn":
		settings.PriceColorScheme = "cn"
	case "intl":
	default:
		return core.AppSettings{}, errors.New("Price color scheme must be one of: cn / intl")
	}
	switch settings.Locale {
	case "", "system":
		settings.Locale = "system"
	case "zh-CN", "en-US":
	default:
		return core.AppSettings{}, errors.New("Locale must be one of: system / zh-CN / en-US")
	}
	switch settings.ProxyMode {
	case "":
		settings.ProxyMode = "system"
		settings.ProxyURL = ""
	case "none":
		settings.ProxyMode = "none"
		settings.ProxyURL = ""
	case "system":
		settings.ProxyURL = ""
	case "custom":
		if settings.ProxyURL == "" {
			return core.AppSettings{}, errors.New("Custom proxy URL is required")
		}
		parsed, err := url.Parse(settings.ProxyURL)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return core.AppSettings{}, errors.New("Custom proxy URL is invalid")
		}
	default:
		return core.AppSettings{}, errors.New("Proxy mode must be one of: none / system / custom")
	}
	switch settings.DashboardCurrency {
	case "", "CNY":
		settings.DashboardCurrency = "CNY"
	case "HKD", "USD":
	default:
		return core.AppSettings{}, errors.New("Dashboard currency must be one of: CNY / HKD / USD")
	}
	return settings, nil
}

// normaliseQuoteSourceIDForSettings determines the final quote source ID to use based on user input, market type, and available quote source list.
func normaliseQuoteSourceIDForSettings(sourceID, market string, providers map[string]core.QuoteProvider, options []core.QuoteSourceOption) string {
	sourceID = strings.ToLower(strings.TrimSpace(sourceID))
	if sourceID != "" {
		if _, ok := providers[sourceID]; ok && quoteSourceSupportsMarketForSettings(sourceID, market, options) {
			return sourceID
		}
	}
	switch marketGroupForMarket(market) {
	case "hk":
		if _, ok := providers[core.DefaultHKQuoteSourceID]; ok {
			return core.DefaultHKQuoteSourceID
		}
	case "us":
		if _, ok := providers[core.DefaultUSQuoteSourceID]; ok {
			return core.DefaultUSQuoteSourceID
		}
	default:
		if _, ok := providers[core.DefaultCNQuoteSourceID]; ok {
			return core.DefaultCNQuoteSourceID
		}
	}
	if _, ok := providers[core.DefaultQuoteSourceID]; ok {
		return core.DefaultQuoteSourceID
	}
	for id := range providers {
		return id
	}
	return core.DefaultQuoteSourceID
}

// quoteSourceSupportsMarketForSettings returns whether the given quote source supports the specified market
// by consulting the authoritative QuoteSourceOption.SupportedMarkets list.
func quoteSourceSupportsMarketForSettings(sourceID string, market string, options []core.QuoteSourceOption) bool {
	for _, opt := range options {
		if opt.ID == sourceID {
			if len(opt.SupportedMarkets) == 0 {
				return true
			}
			for _, m := range opt.SupportedMarkets {
				if m == market {
					return true
				}
			}
			return false
		}
	}
	return false
}
