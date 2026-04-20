// provider_alphavantage.go - Alpha Vantage quote and history provider (US only, API key required).
package marketdata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"investgo/internal/datasource"
	"investgo/internal/monitor"
)

type AlphaVantageQuoteProvider struct {
	client   *http.Client
	settings func() monitor.AppSettings
}

type AlphaVantageHistoryProvider struct {
	client   *http.Client
	settings func() monitor.AppSettings
}

type alphaVantageQuoteResponse struct {
	GlobalQuote  map[string]string `json:"Global Quote"`
	Note         string            `json:"Note"`
	Information  string            `json:"Information"`
	ErrorMessage string            `json:"Error Message"`
}

func NewAlphaVantageQuoteProvider(client *http.Client, settings func() monitor.AppSettings) *AlphaVantageQuoteProvider {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	if settings == nil {
		settings = func() monitor.AppSettings { return monitor.AppSettings{} }
	}
	return &AlphaVantageQuoteProvider{client: client, settings: settings}
}

func (p *AlphaVantageQuoteProvider) Name() string { return "Alpha Vantage" }

func (p *AlphaVantageQuoteProvider) Fetch(ctx context.Context, items []monitor.WatchlistItem) (map[string]monitor.Quote, error) {
	apiKey := strings.TrimSpace(p.settings().AlphaVantageAPIKey)
	if apiKey == "" {
		return nil, errors.New("Alpha Vantage API key is required")
	}

	targets, problems := collectQuoteTargets(items)
	quotes := make(map[string]monitor.Quote, len(targets))
	if len(targets) == 0 {
		return quotes, monitor.JoinProblems(problems)
	}

	for _, item := range items {
		target, err := monitor.ResolveQuoteTarget(item)
		if err != nil {
			continue
		}
		if target.Market != "US-STOCK" && target.Market != "US-ETF" {
			problems = append(problems, fmt.Sprintf("Alpha Vantage does not support item: %s", target.DisplaySymbol))
			continue
		}

		quote, err := fetchAlphaVantageQuote(ctx, p.client, target.DisplaySymbol, item.Name, item.Currency, apiKey)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", target.DisplaySymbol, err))
			continue
		}
		quote.Symbol = target.DisplaySymbol
		quote.Market = target.Market
		quote.Currency = firstNonEmpty(quote.Currency, target.Currency)
		quotes[target.Key] = quote
	}

	return quotes, monitor.JoinProblems(problems)
}

func NewAlphaVantageHistoryProvider(client *http.Client, settings func() monitor.AppSettings) *AlphaVantageHistoryProvider {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	if settings == nil {
		settings = func() monitor.AppSettings { return monitor.AppSettings{} }
	}
	return &AlphaVantageHistoryProvider{client: client, settings: settings}
}

func (p *AlphaVantageHistoryProvider) Name() string { return "Alpha Vantage" }

func (p *AlphaVantageHistoryProvider) Fetch(ctx context.Context, item monitor.WatchlistItem, interval monitor.HistoryInterval) (monitor.HistorySeries, error) {
	apiKey := strings.TrimSpace(p.settings().AlphaVantageAPIKey)
	if apiKey == "" {
		return monitor.HistorySeries{}, errors.New("Alpha Vantage API key is required")
	}
	target, err := monitor.ResolveQuoteTarget(item)
	if err != nil {
		return monitor.HistorySeries{}, err
	}
	if target.Market != "US-STOCK" && target.Market != "US-ETF" {
		return monitor.HistorySeries{}, fmt.Errorf("Alpha Vantage does not support market: %s", target.DisplaySymbol)
	}

	points, currency, err := fetchAlphaVantageHistory(ctx, p.client, target.DisplaySymbol, interval, apiKey)
	if err != nil {
		return monitor.HistorySeries{}, err
	}
	if len(points) == 0 {
		return monitor.HistorySeries{}, errors.New("History response contains no valid price points")
	}

	series := monitor.HistorySeries{
		Symbol:      item.Symbol,
		Name:        firstNonEmpty(item.Name, item.Symbol),
		Market:      item.Market,
		Currency:    firstNonEmpty(currency, item.Currency),
		Interval:    interval,
		Source:      p.Name(),
		Points:      points,
		GeneratedAt: time.Now(),
	}
	applyHistorySummary(&series)
	return series, nil
}

func fetchAlphaVantageQuote(ctx context.Context, client *http.Client, symbol, fallbackName, fallbackCurrency, apiKey string) (monitor.Quote, error) {
	params := url.Values{}
	params.Set("function", "GLOBAL_QUOTE")
	params.Set("symbol", symbol)
	params.Set("apikey", apiKey)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, datasource.URLWithQuery(datasource.AlphaVantageAPI, params), nil)
	if err != nil {
		return monitor.Quote{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return monitor.Quote{}, fmt.Errorf("Alpha Vantage quote request failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return monitor.Quote{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return monitor.Quote{}, fmt.Errorf("Alpha Vantage quote request failed: status %d", resp.StatusCode)
	}
	var parsed alphaVantageQuoteResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return monitor.Quote{}, err
	}
	if parsed.ErrorMessage != "" {
		return monitor.Quote{}, errors.New(parsed.ErrorMessage)
	}
	if parsed.Information != "" {
		return monitor.Quote{}, errors.New(parsed.Information)
	}
	if parsed.Note != "" {
		return monitor.Quote{}, errors.New(parsed.Note)
	}
	if len(parsed.GlobalQuote) == 0 {
		return monitor.Quote{}, errors.New("Alpha Vantage quote response is empty")
	}
	quote := buildQuote(
		firstNonEmpty(fallbackName, symbol),
		parseFloat(parsed.GlobalQuote["05. price"]),
		parseFloat(parsed.GlobalQuote["08. previous close"]),
		parseFloat(parsed.GlobalQuote["02. open"]),
		parseFloat(parsed.GlobalQuote["03. high"]),
		parseFloat(parsed.GlobalQuote["04. low"]),
		time.Now(),
		"Alpha Vantage",
	)
	if quote.Change == 0 {
		quote.Change = parseFloat(parsed.GlobalQuote["09. change"])
	}
	if quote.ChangePercent == 0 {
		quote.ChangePercent = parseFloat(strings.TrimSuffix(parsed.GlobalQuote["10. change percent"], "%"))
	}
	quote.Currency = fallbackCurrency
	return quote, nil
}

func fetchAlphaVantageHistory(ctx context.Context, client *http.Client, symbol string, interval monitor.HistoryInterval, apiKey string) ([]monitor.HistoryPoint, string, error) {
	params := url.Values{}
	params.Set("symbol", symbol)
	params.Set("apikey", apiKey)
	seriesKey := ""
	switch interval {
	case monitor.HistoryRange1h, monitor.HistoryRange1d:
		params.Set("function", "TIME_SERIES_INTRADAY")
		params.Set("interval", "60min")
		params.Set("outputsize", "full")
		seriesKey = "Time Series (60min)"
	case monitor.HistoryRange1w, monitor.HistoryRange1mo, monitor.HistoryRange1y:
		params.Set("function", "TIME_SERIES_DAILY")
		params.Set("outputsize", "full")
		seriesKey = "Time Series (Daily)"
	case monitor.HistoryRange3y:
		params.Set("function", "TIME_SERIES_WEEKLY")
		seriesKey = "Weekly Time Series"
	case monitor.HistoryRangeAll:
		params.Set("function", "TIME_SERIES_MONTHLY")
		seriesKey = "Monthly Time Series"
	default:
		return nil, "", errors.New("History interval must be one of: 1h / 1d / 1w / 1mo / 1y / 3y / all")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, datasource.URLWithQuery(datasource.AlphaVantageAPI, params), nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("Alpha Vantage history request failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("Alpha Vantage history request failed: status %d", resp.StatusCode)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, "", err
	}
	if msg := decodeRawString(raw["Error Message"]); msg != "" {
		return nil, "", errors.New(msg)
	}
	if msg := decodeRawString(raw["Information"]); msg != "" {
		return nil, "", errors.New(msg)
	}
	if msg := decodeRawString(raw["Note"]); msg != "" {
		return nil, "", errors.New(msg)
	}
	var series map[string]map[string]string
	if err := json.Unmarshal(raw[seriesKey], &series); err != nil || len(series) == 0 {
		return nil, "", errors.New("History response contains no valid price points")
	}
	points := make([]monitor.HistoryPoint, 0, len(series))
	for ts, values := range series {
		pointTime := parseUSAPITimestamp(ts)
		if pointTime.IsZero() {
			continue
		}
		points = append(points, monitor.HistoryPoint{
			Timestamp: pointTime,
			Open:      parseFloat(values["1. open"]),
			High:      parseFloat(values["2. high"]),
			Low:       parseFloat(values["3. low"]),
			Close:     parseFloat(values["4. close"]),
			Volume:    parseFloat(values["5. volume"]),
		})
	}
	sort.Slice(points, func(i, j int) bool { return points[i].Timestamp.Before(points[j].Timestamp) })
	points = trimHistoryPoints(points, historyTrimWindow(interval))
	return points, "", nil
}

func decodeRawString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return ""
	}
	return value
}
