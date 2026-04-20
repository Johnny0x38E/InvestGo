// provider_finnhub.go â Finnhub quote and history provider (US only, API key required).
package marketdata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"investgo/internal/datasource"
	"investgo/internal/monitor"
)

type FinnhubQuoteProvider struct {
	client   *http.Client
	settings func() monitor.AppSettings
}

type FinnhubHistoryProvider struct {
	client   *http.Client
	settings func() monitor.AppSettings
}

type finnhubQuoteResponse struct {
	Current       float64 `json:"c"`
	DayHigh       float64 `json:"h"`
	DayLow        float64 `json:"l"`
	Open          float64 `json:"o"`
	PreviousClose float64 `json:"pc"`
	Timestamp     int64   `json:"t"`
}

type finnhubCandleResponse struct {
	Close  []float64 `json:"c"`
	High   []float64 `json:"h"`
	Low    []float64 `json:"l"`
	Open   []float64 `json:"o"`
	Status string    `json:"s"`
	Time   []int64   `json:"t"`
	Volume []float64 `json:"v"`
}

func NewFinnhubQuoteProvider(client *http.Client, settings func() monitor.AppSettings) *FinnhubQuoteProvider {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	if settings == nil {
		settings = func() monitor.AppSettings { return monitor.AppSettings{} }
	}
	return &FinnhubQuoteProvider{client: client, settings: settings}
}

func (p *FinnhubQuoteProvider) Name() string { return "Finnhub" }

func (p *FinnhubQuoteProvider) Fetch(ctx context.Context, items []monitor.WatchlistItem) (map[string]monitor.Quote, error) {
	apiKey := strings.TrimSpace(p.settings().FinnhubAPIKey)
	if apiKey == "" {
		return nil, errors.New("Finnhub API key is required")
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
			problems = append(problems, fmt.Sprintf("Finnhub does not support item: %s", target.DisplaySymbol))
			continue
		}

		quote, err := fetchFinnhubQuote(ctx, p.client, target.DisplaySymbol, item.Name, item.Currency, apiKey)
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

func NewFinnhubHistoryProvider(client *http.Client, settings func() monitor.AppSettings) *FinnhubHistoryProvider {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	if settings == nil {
		settings = func() monitor.AppSettings { return monitor.AppSettings{} }
	}
	return &FinnhubHistoryProvider{client: client, settings: settings}
}

func (p *FinnhubHistoryProvider) Name() string { return "Finnhub" }

func (p *FinnhubHistoryProvider) Fetch(ctx context.Context, item monitor.WatchlistItem, interval monitor.HistoryInterval) (monitor.HistorySeries, error) {
	apiKey := strings.TrimSpace(p.settings().FinnhubAPIKey)
	if apiKey == "" {
		return monitor.HistorySeries{}, errors.New("Finnhub API key is required")
	}
	target, err := monitor.ResolveQuoteTarget(item)
	if err != nil {
		return monitor.HistorySeries{}, err
	}
	if target.Market != "US-STOCK" && target.Market != "US-ETF" {
		return monitor.HistorySeries{}, fmt.Errorf("Finnhub does not support market: %s", target.DisplaySymbol)
	}

	points, err := fetchFinnhubHistory(ctx, p.client, target.DisplaySymbol, interval, apiKey)
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
		Currency:    firstNonEmpty(item.Currency, target.Currency),
		Interval:    interval,
		Source:      p.Name(),
		Points:      points,
		GeneratedAt: time.Now(),
	}
	applyHistorySummary(&series)
	return series, nil
}

func fetchFinnhubQuote(ctx context.Context, client *http.Client, symbol, fallbackName, fallbackCurrency, apiKey string) (monitor.Quote, error) {
	params := url.Values{}
	params.Set("symbol", symbol)
	params.Set("token", apiKey)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, datasource.URLWithQuery(datasource.FinnhubQuoteAPI, params), nil)
	if err != nil {
		return monitor.Quote{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return monitor.Quote{}, fmt.Errorf("Finnhub quote request failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return monitor.Quote{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return monitor.Quote{}, fmt.Errorf("Finnhub quote request failed: status %d", resp.StatusCode)
	}

	var parsed finnhubQuoteResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return monitor.Quote{}, err
	}
	if parsed.Current <= 0 && parsed.PreviousClose <= 0 {
		return monitor.Quote{}, errors.New("History response contains no valid price points")
	}

	updatedAt := time.Now()
	if parsed.Timestamp > 0 {
		updatedAt = time.Unix(parsed.Timestamp, 0)
	}
	quote := buildQuote(
		firstNonEmpty(fallbackName, symbol),
		parsed.Current,
		parsed.PreviousClose,
		parsed.Open,
		parsed.DayHigh,
		parsed.DayLow,
		updatedAt,
		"Finnhub",
	)
	quote.Currency = fallbackCurrency
	return quote, nil
}

func fetchFinnhubHistory(ctx context.Context, client *http.Client, symbol string, interval monitor.HistoryInterval, apiKey string) ([]monitor.HistoryPoint, error) {
	now := time.Now()
	from := now.Add(-historyTrimWindow(interval))
	if interval == monitor.HistoryRangeAll {
		from = now.AddDate(-20, 0, 0)
	}

	params := url.Values{}
	params.Set("symbol", symbol)
	params.Set("resolution", finnhubResolution(interval))
	params.Set("from", fmt.Sprintf("%d", from.Unix()))
	params.Set("to", fmt.Sprintf("%d", now.Unix()))
	params.Set("token", apiKey)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, datasource.URLWithQuery(datasource.FinnhubCandleAPI, params), nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Finnhub history request failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Finnhub history request failed: status %d", resp.StatusCode)
	}

	var parsed finnhubCandleResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	if parsed.Status != "ok" {
		if parsed.Status == "no_data" {
			return nil, errors.New("History response contains no valid price points")
		}
		return nil, errors.New(firstNonEmpty(parsed.Status, "History response contains no valid price points"))
	}

	size := minInt(len(parsed.Time), minInt(len(parsed.Open), minInt(len(parsed.High), minInt(len(parsed.Low), minInt(len(parsed.Close), len(parsed.Volume))))))
	points := make([]monitor.HistoryPoint, 0, size)
	for idx := 0; idx < size; idx++ {
		if parsed.Close[idx] <= 0 {
			continue
		}
		points = append(points, monitor.HistoryPoint{
			Timestamp: time.Unix(parsed.Time[idx], 0),
			Open:      parsed.Open[idx],
			High:      parsed.High[idx],
			Low:       parsed.Low[idx],
			Close:     parsed.Close[idx],
			Volume:    parsed.Volume[idx],
		})
	}
	return trimHistoryPoints(points, historyTrimWindow(interval)), nil
}

func finnhubResolution(interval monitor.HistoryInterval) string {
	switch interval {
	case monitor.HistoryRange1h:
		return "5"
	case monitor.HistoryRange1d:
		return "15"
	case monitor.HistoryRange3y:
		return "W"
	case monitor.HistoryRangeAll:
		return "M"
	default:
		return "D"
	}
}
