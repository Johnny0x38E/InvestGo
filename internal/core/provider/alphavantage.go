// provider_alphavantage.go - Alpha Vantage quote and history provider (US only, API key required).
package provider

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

	"investgo/internal/common/errs"
	"investgo/internal/core"
	"investgo/internal/core/endpoint"
)

type AlphaVantageQuoteProvider struct {
	client   *http.Client
	settings func() core.AppSettings
}

type AlphaVantageHistoryProvider struct {
	client   *http.Client
	settings func() core.AppSettings
}

type alphaVantageQuoteResponse struct {
	GlobalQuote  map[string]string `json:"Global Quote"`
	Note         string            `json:"Note"`
	Information  string            `json:"Information"`
	ErrorMessage string            `json:"Error Message"`
}

func NewAlphaVantageQuoteProvider(client *http.Client, settings func() core.AppSettings) *AlphaVantageQuoteProvider {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	if settings == nil {
		settings = func() core.AppSettings { return core.AppSettings{} }
	}
	return &AlphaVantageQuoteProvider{client: client, settings: settings}
}

func (p *AlphaVantageQuoteProvider) Name() string { return "Alpha Vantage" }

func (p *AlphaVantageQuoteProvider) Fetch(ctx context.Context, items []core.WatchlistItem) (map[string]core.Quote, error) {
	apiKey := strings.TrimSpace(p.settings().AlphaVantageAPIKey)
	if apiKey == "" {
		return nil, errors.New("Alpha Vantage API key is required")
	}

	targets, problems := CollectQuoteTargets(items)
	quotes := make(map[string]core.Quote, len(targets))
	if len(targets) == 0 {
		return quotes, errs.JoinProblems(problems)
	}

	for _, item := range items {
		target, err := core.ResolveQuoteTarget(item)
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
		quote.Currency = FirstNonEmpty(quote.Currency, target.Currency)
		quotes[target.Key] = quote
	}

	return quotes, errs.JoinProblems(problems)
}

func NewAlphaVantageHistoryProvider(client *http.Client, settings func() core.AppSettings) *AlphaVantageHistoryProvider {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	if settings == nil {
		settings = func() core.AppSettings { return core.AppSettings{} }
	}
	return &AlphaVantageHistoryProvider{client: client, settings: settings}
}

func (p *AlphaVantageHistoryProvider) Name() string { return "Alpha Vantage" }

func (p *AlphaVantageHistoryProvider) Fetch(ctx context.Context, item core.WatchlistItem, interval core.HistoryInterval) (core.HistorySeries, error) {
	apiKey := strings.TrimSpace(p.settings().AlphaVantageAPIKey)
	if apiKey == "" {
		return core.HistorySeries{}, errors.New("Alpha Vantage API key is required")
	}
	target, err := core.ResolveQuoteTarget(item)
	if err != nil {
		return core.HistorySeries{}, err
	}
	if target.Market != "US-STOCK" && target.Market != "US-ETF" {
		return core.HistorySeries{}, fmt.Errorf("Alpha Vantage does not support market: %s", target.DisplaySymbol)
	}

	points, currency, err := fetchAlphaVantageHistory(ctx, p.client, target.DisplaySymbol, interval, apiKey)
	if err != nil {
		return core.HistorySeries{}, err
	}
	if len(points) == 0 {
		return core.HistorySeries{}, errors.New("History response contains no valid price points")
	}

	series := core.HistorySeries{
		Symbol:      item.Symbol,
		Name:        FirstNonEmpty(item.Name, item.Symbol),
		Market:      item.Market,
		Currency:    FirstNonEmpty(currency, item.Currency),
		Interval:    interval,
		Source:      p.Name(),
		Points:      points,
		GeneratedAt: time.Now(),
	}
	ApplyHistorySummary(&series)
	return series, nil
}

func fetchAlphaVantageQuote(ctx context.Context, client *http.Client, symbol, fallbackName, fallbackCurrency, apiKey string) (core.Quote, error) {
	params := url.Values{}
	params.Set("function", "GLOBAL_QUOTE")
	params.Set("symbol", symbol)
	params.Set("apikey", apiKey)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.URLWithQuery(endpoint.AlphaVantageAPI, params), nil)
	if err != nil {
		return core.Quote{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return core.Quote{}, fmt.Errorf("Alpha Vantage quote request failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return core.Quote{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return core.Quote{}, fmt.Errorf("Alpha Vantage quote request failed: status %d", resp.StatusCode)
	}
	var parsed alphaVantageQuoteResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return core.Quote{}, err
	}
	if parsed.ErrorMessage != "" {
		return core.Quote{}, errors.New(parsed.ErrorMessage)
	}
	if parsed.Information != "" {
		return core.Quote{}, errors.New(parsed.Information)
	}
	if parsed.Note != "" {
		return core.Quote{}, errors.New(parsed.Note)
	}
	if len(parsed.GlobalQuote) == 0 {
		return core.Quote{}, errors.New("Alpha Vantage quote response is empty")
	}
	quote := BuildQuote(
		FirstNonEmpty(fallbackName, symbol),
		ParseFloat(parsed.GlobalQuote["05. price"]),
		ParseFloat(parsed.GlobalQuote["08. previous close"]),
		ParseFloat(parsed.GlobalQuote["02. open"]),
		ParseFloat(parsed.GlobalQuote["03. high"]),
		ParseFloat(parsed.GlobalQuote["04. low"]),
		time.Now(),
		"Alpha Vantage",
	)
	if quote.Change == 0 {
		quote.Change = ParseFloat(parsed.GlobalQuote["09. change"])
	}
	if quote.ChangePercent == 0 {
		quote.ChangePercent = ParseFloat(strings.TrimSuffix(parsed.GlobalQuote["10. change percent"], "%"))
	}
	quote.Currency = fallbackCurrency
	return quote, nil
}

func fetchAlphaVantageHistory(ctx context.Context, client *http.Client, symbol string, interval core.HistoryInterval, apiKey string) ([]core.HistoryPoint, string, error) {
	params := url.Values{}
	params.Set("symbol", symbol)
	params.Set("apikey", apiKey)
	seriesKey := ""
	switch interval {
	case core.HistoryRange1h, core.HistoryRange1d:
		params.Set("function", "TIME_SERIES_INTRADAY")
		params.Set("interval", "60min")
		params.Set("outputsize", "full")
		seriesKey = "Time Series (60min)"
	case core.HistoryRange1w, core.HistoryRange1mo, core.HistoryRange1y:
		params.Set("function", "TIME_SERIES_DAILY")
		params.Set("outputsize", "full")
		seriesKey = "Time Series (Daily)"
	case core.HistoryRange3y:
		params.Set("function", "TIME_SERIES_WEEKLY")
		seriesKey = "Weekly Time Series"
	case core.HistoryRangeAll:
		params.Set("function", "TIME_SERIES_MONTHLY")
		seriesKey = "Monthly Time Series"
	default:
		return nil, "", errors.New("History interval must be one of: 1h / 1d / 1w / 1mo / 1y / 3y / all")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.URLWithQuery(endpoint.AlphaVantageAPI, params), nil)
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
	points := make([]core.HistoryPoint, 0, len(series))
	for ts, values := range series {
		pointTime := ParseUSAPITimestamp(ts)
		if pointTime.IsZero() {
			continue
		}
		points = append(points, core.HistoryPoint{
			Timestamp: pointTime,
			Open:      ParseFloat(values["1. open"]),
			High:      ParseFloat(values["2. high"]),
			Low:       ParseFloat(values["3. low"]),
			Close:     ParseFloat(values["4. close"]),
			Volume:    ParseFloat(values["5. volume"]),
		})
	}
	sort.Slice(points, func(i, j int) bool { return points[i].Timestamp.Before(points[j].Timestamp) })
	points = TrimHistoryPoints(points, HistoryTrimWindow(interval))
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
