// provider_polygon.go - Polygon.io quote and history provider (US only, API key required).
package provider

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

	"investgo/internal/common/errs"
	"investgo/internal/core"
	"investgo/internal/core/endpoint"
)

type PolygonQuoteProvider struct {
	client   *http.Client
	settings func() core.AppSettings
}

type PolygonHistoryProvider struct {
	client   *http.Client
	settings func() core.AppSettings
}

type polygonSnapshotResponse struct {
	Status string `json:"status"`
	Ticker *struct {
		Ticker    string `json:"ticker"`
		Name      string `json:"name"`
		LastTrade *struct {
			Price     float64 `json:"p"`
			Timestamp int64   `json:"t"`
		} `json:"lastTrade"`
		Min *struct {
			Open   float64 `json:"o"`
			High   float64 `json:"h"`
			Low    float64 `json:"l"`
			Close  float64 `json:"c"`
			Volume float64 `json:"v"`
		} `json:"min"`
		Day *struct {
			Open   float64 `json:"o"`
			High   float64 `json:"h"`
			Low    float64 `json:"l"`
			Close  float64 `json:"c"`
			Volume float64 `json:"v"`
		} `json:"day"`
		PrevDay *struct {
			Close float64 `json:"c"`
		} `json:"prevDay"`
	} `json:"ticker"`
}

type polygonAggsResponse struct {
	Status       string `json:"status"`
	ResultsCount int    `json:"resultsCount"`
	Results      []struct {
		Timestamp int64   `json:"t"`
		Open      float64 `json:"o"`
		High      float64 `json:"h"`
		Low       float64 `json:"l"`
		Close     float64 `json:"c"`
		Volume    float64 `json:"v"`
	} `json:"results"`
}

func NewPolygonQuoteProvider(client *http.Client, settings func() core.AppSettings) *PolygonQuoteProvider {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	if settings == nil {
		settings = func() core.AppSettings { return core.AppSettings{} }
	}
	return &PolygonQuoteProvider{client: client, settings: settings}
}

func (p *PolygonQuoteProvider) Name() string { return "Polygon" }

func (p *PolygonQuoteProvider) Fetch(ctx context.Context, items []core.WatchlistItem) (map[string]core.Quote, error) {
	apiKey := strings.TrimSpace(p.settings().PolygonAPIKey)
	if apiKey == "" {
		return nil, errors.New("Polygon API key is required")
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
			problems = append(problems, fmt.Sprintf("Polygon does not support item: %s", target.DisplaySymbol))
			continue
		}

		quote, err := fetchPolygonQuote(ctx, p.client, target.DisplaySymbol, item.Name, item.Currency, apiKey)
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

func NewPolygonHistoryProvider(client *http.Client, settings func() core.AppSettings) *PolygonHistoryProvider {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	if settings == nil {
		settings = func() core.AppSettings { return core.AppSettings{} }
	}
	return &PolygonHistoryProvider{client: client, settings: settings}
}

func (p *PolygonHistoryProvider) Name() string { return "Polygon" }

func (p *PolygonHistoryProvider) Fetch(ctx context.Context, item core.WatchlistItem, interval core.HistoryInterval) (core.HistorySeries, error) {
	apiKey := strings.TrimSpace(p.settings().PolygonAPIKey)
	if apiKey == "" {
		return core.HistorySeries{}, errors.New("Polygon API key is required")
	}
	target, err := core.ResolveQuoteTarget(item)
	if err != nil {
		return core.HistorySeries{}, err
	}
	if target.Market != "US-STOCK" && target.Market != "US-ETF" {
		return core.HistorySeries{}, fmt.Errorf("Polygon does not support market: %s", target.DisplaySymbol)
	}

	points, err := fetchPolygonHistory(ctx, p.client, target.DisplaySymbol, interval, apiKey)
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
		Currency:    FirstNonEmpty(item.Currency, target.Currency),
		Interval:    interval,
		Source:      p.Name(),
		Points:      points,
		GeneratedAt: time.Now(),
	}
	ApplyHistorySummary(&series)
	return series, nil
}

func fetchPolygonQuote(ctx context.Context, client *http.Client, symbol, fallbackName, fallbackCurrency, apiKey string) (core.Quote, error) {
	params := url.Values{}
	params.Set("apiKey", apiKey)
	apiURL := endpoint.PolygonSnapshotAPI + "/" + url.PathEscape(symbol)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.URLWithQuery(apiURL, params), nil)
	if err != nil {
		return core.Quote{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return core.Quote{}, fmt.Errorf("Polygon quote request failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return core.Quote{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return core.Quote{}, fmt.Errorf("Polygon quote request failed: status %d", resp.StatusCode)
	}

	var parsed polygonSnapshotResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return core.Quote{}, err
	}
	if parsed.Ticker == nil {
		return core.Quote{}, errors.New("Polygon quote response is empty")
	}

	current := 0.0
	updatedAt := time.Now()
	if parsed.Ticker.LastTrade != nil {
		current = parsed.Ticker.LastTrade.Price
		if parsed.Ticker.LastTrade.Timestamp > 0 {
			updatedAt = polygonTimestamp(parsed.Ticker.LastTrade.Timestamp)
		}
	}
	if current <= 0 && parsed.Ticker.Day != nil {
		current = parsed.Ticker.Day.Close
	}

	previousClose := 0.0
	if parsed.Ticker.PrevDay != nil {
		previousClose = parsed.Ticker.PrevDay.Close
	}
	openPrice, dayHigh, dayLow := 0.0, 0.0, 0.0
	if parsed.Ticker.Day != nil {
		openPrice = parsed.Ticker.Day.Open
		dayHigh = parsed.Ticker.Day.High
		dayLow = parsed.Ticker.Day.Low
	}
	if openPrice == 0 && parsed.Ticker.Min != nil {
		openPrice = parsed.Ticker.Min.Open
	}
	if dayHigh == 0 && parsed.Ticker.Min != nil {
		dayHigh = parsed.Ticker.Min.High
	}
	if dayLow == 0 && parsed.Ticker.Min != nil {
		dayLow = parsed.Ticker.Min.Low
	}
	if current <= 0 && previousClose <= 0 {
		return core.Quote{}, errors.New("Polygon quote response is empty")
	}

	quote := BuildQuote(
		FirstNonEmpty(parsed.Ticker.Name, fallbackName, symbol),
		current,
		previousClose,
		openPrice,
		dayHigh,
		dayLow,
		updatedAt,
		"Polygon",
	)
	quote.Currency = fallbackCurrency
	return quote, nil
}

func fetchPolygonHistory(ctx context.Context, client *http.Client, symbol string, interval core.HistoryInterval, apiKey string) ([]core.HistoryPoint, error) {
	multiplier, resolution := polygonRangeConfig(interval)
	params := url.Values{}
	params.Set("adjusted", "true")
	params.Set("sort", "asc")
	params.Set("limit", "50000")
	params.Set("apiKey", apiKey)

	from, to := polygonHistoryWindow(interval)
	apiURL := endpoint.PolygonAggsAPI + "/" + url.PathEscape(symbol) + "/range/" + multiplier + "/" + resolution + "/" + from + "/" + to
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.URLWithQuery(apiURL, params), nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Polygon history request failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Polygon history request failed: status %d", resp.StatusCode)
	}

	var parsed polygonAggsResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	if parsed.Status != "OK" && parsed.Status != "DELAYED" {
		return nil, errors.New(FirstNonEmpty(parsed.Status, "History response contains no valid price points"))
	}
	if len(parsed.Results) == 0 {
		return nil, errors.New("History response contains no valid price points")
	}

	points := make([]core.HistoryPoint, 0, len(parsed.Results))
	for _, result := range parsed.Results {
		if result.Close <= 0 {
			continue
		}
		points = append(points, core.HistoryPoint{
			Timestamp: polygonTimestamp(result.Timestamp),
			Open:      result.Open,
			High:      result.High,
			Low:       result.Low,
			Close:     result.Close,
			Volume:    result.Volume,
		})
	}
	return TrimHistoryPoints(points, HistoryTrimWindow(interval)), nil
}

func polygonRangeConfig(interval core.HistoryInterval) (string, string) {
	switch interval {
	case core.HistoryRange1h:
		return "5", "minute"
	case core.HistoryRange1d:
		return "15", "minute"
	case core.HistoryRange3y:
		return "1", "week"
	case core.HistoryRangeAll:
		return "1", "month"
	default:
		return "1", "day"
	}
}

func polygonHistoryWindow(interval core.HistoryInterval) (string, string) {
	now := time.Now()
	from := now.Add(-HistoryTrimWindow(interval))
	if interval == core.HistoryRangeAll {
		from = now.AddDate(-20, 0, 0)
	}
	return from.Format(time.DateOnly), now.Format(time.DateOnly)
}

func polygonTimestamp(value int64) time.Time {
	switch {
	case value > 1_000_000_000_000_000:
		return time.Unix(0, value)
	case value > 1_000_000_000_000:
		return time.UnixMilli(value)
	default:
		return time.Unix(value, 0)
	}
}
