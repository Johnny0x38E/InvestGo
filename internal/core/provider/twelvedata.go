// provider_twelvedata.go - Twelve Data quote and history provider (US only, API key required).
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

type TwelveDataQuoteProvider struct {
	client   *http.Client
	settings func() core.AppSettings
}

type TwelveDataHistoryProvider struct {
	client   *http.Client
	settings func() core.AppSettings
}

type twelveDataQuoteResponse struct {
	Symbol        string `json:"symbol"`
	Name          string `json:"name"`
	Currency      string `json:"currency"`
	Open          string `json:"open"`
	High          string `json:"high"`
	Low           string `json:"low"`
	Close         string `json:"close"`
	PreviousClose string `json:"previous_close"`
	Change        string `json:"change"`
	PercentChange string `json:"percent_change"`
	Code          int    `json:"code"`
	Message       string `json:"message"`
	Status        string `json:"status"`
}

type twelveDataSeriesResponse struct {
	Meta struct {
		Symbol   string `json:"symbol"`
		Name     string `json:"name"`
		Currency string `json:"currency"`
	} `json:"meta"`
	Values []struct {
		Datetime string `json:"datetime"`
		Open     string `json:"open"`
		High     string `json:"high"`
		Low      string `json:"low"`
		Close    string `json:"close"`
		Volume   string `json:"volume"`
	} `json:"values"`
	Code    int    `json:"code"`
	Message string `json:"message"`
	Status  string `json:"status"`
}

func NewTwelveDataQuoteProvider(client *http.Client, settings func() core.AppSettings) *TwelveDataQuoteProvider {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	if settings == nil {
		settings = func() core.AppSettings { return core.AppSettings{} }
	}
	return &TwelveDataQuoteProvider{client: client, settings: settings}
}

func (p *TwelveDataQuoteProvider) Name() string { return "Twelve Data" }

func (p *TwelveDataQuoteProvider) Fetch(ctx context.Context, items []core.WatchlistItem) (map[string]core.Quote, error) {
	apiKey := strings.TrimSpace(p.settings().TwelveDataAPIKey)
	if apiKey == "" {
		return nil, errors.New("Twelve Data API key is required")
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
			problems = append(problems, fmt.Sprintf("Twelve Data does not support item: %s", target.DisplaySymbol))
			continue
		}

		quote, err := fetchTwelveDataQuote(ctx, p.client, target.DisplaySymbol, item.Name, item.Currency, apiKey)
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

func NewTwelveDataHistoryProvider(client *http.Client, settings func() core.AppSettings) *TwelveDataHistoryProvider {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	if settings == nil {
		settings = func() core.AppSettings { return core.AppSettings{} }
	}
	return &TwelveDataHistoryProvider{client: client, settings: settings}
}

func (p *TwelveDataHistoryProvider) Name() string { return "Twelve Data" }

func (p *TwelveDataHistoryProvider) Fetch(ctx context.Context, item core.WatchlistItem, interval core.HistoryInterval) (core.HistorySeries, error) {
	apiKey := strings.TrimSpace(p.settings().TwelveDataAPIKey)
	if apiKey == "" {
		return core.HistorySeries{}, errors.New("Twelve Data API key is required")
	}
	target, err := core.ResolveQuoteTarget(item)
	if err != nil {
		return core.HistorySeries{}, err
	}
	if target.Market != "US-STOCK" && target.Market != "US-ETF" {
		return core.HistorySeries{}, fmt.Errorf("Twelve Data does not support market: %s", target.DisplaySymbol)
	}

	points, currency, err := fetchTwelveDataHistory(ctx, p.client, target.DisplaySymbol, interval, apiKey)
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

func fetchTwelveDataQuote(ctx context.Context, client *http.Client, symbol, fallbackName, fallbackCurrency, apiKey string) (core.Quote, error) {
	params := url.Values{}
	params.Set("symbol", symbol)
	params.Set("apikey", apiKey)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.URLWithQuery(endpoint.TwelveDataQuoteAPI, params), nil)
	if err != nil {
		return core.Quote{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return core.Quote{}, fmt.Errorf("Twelve Data quote request failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return core.Quote{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return core.Quote{}, fmt.Errorf("Twelve Data quote request failed: status %d", resp.StatusCode)
	}
	var parsed twelveDataQuoteResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return core.Quote{}, err
	}
	if parsed.Status == "error" || parsed.Code != 0 {
		return core.Quote{}, errors.New(FirstNonEmpty(parsed.Message, "Twelve Data quote response is empty"))
	}
	quote := BuildQuote(
		FirstNonEmpty(parsed.Name, fallbackName, symbol),
		ParseFloat(parsed.Close),
		ParseFloat(parsed.PreviousClose),
		ParseFloat(parsed.Open),
		ParseFloat(parsed.High),
		ParseFloat(parsed.Low),
		time.Now(),
		"Twelve Data",
	)
	if quote.Change == 0 {
		quote.Change = ParseFloat(parsed.Change)
	}
	if quote.ChangePercent == 0 {
		quote.ChangePercent = ParseFloat(strings.TrimSuffix(parsed.PercentChange, "%"))
	}
	quote.Currency = FirstNonEmpty(parsed.Currency, fallbackCurrency)
	return quote, nil
}

func fetchTwelveDataHistory(ctx context.Context, client *http.Client, symbol string, interval core.HistoryInterval, apiKey string) ([]core.HistoryPoint, string, error) {
	params := url.Values{}
	params.Set("symbol", symbol)
	params.Set("apikey", apiKey)
	params.Set("interval", twelveDataInterval(interval))
	params.Set("outputsize", twelveDataOutputSize(interval))
	params.Set("order", "ASC")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.URLWithQuery(endpoint.TwelveDataTimeSeriesAPI, params), nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("Twelve Data history request failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("Twelve Data history request failed: status %d", resp.StatusCode)
	}
	var parsed twelveDataSeriesResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, "", err
	}
	if parsed.Status == "error" || parsed.Code != 0 {
		return nil, "", errors.New(FirstNonEmpty(parsed.Message, "History response contains no valid price points"))
	}
	points := make([]core.HistoryPoint, 0, len(parsed.Values))
	for _, value := range parsed.Values {
		pointTime := ParseUSAPITimestamp(value.Datetime)
		if pointTime.IsZero() {
			continue
		}
		points = append(points, core.HistoryPoint{
			Timestamp: pointTime,
			Open:      ParseFloat(value.Open),
			High:      ParseFloat(value.High),
			Low:       ParseFloat(value.Low),
			Close:     ParseFloat(value.Close),
			Volume:    ParseFloat(value.Volume),
		})
	}
	points = TrimHistoryPoints(points, HistoryTrimWindow(interval))
	return points, parsed.Meta.Currency, nil
}

func twelveDataInterval(interval core.HistoryInterval) string {
	switch interval {
	case core.HistoryRange1h:
		return "5min"
	case core.HistoryRange1d:
		return "15min"
	case core.HistoryRange3y:
		return "1week"
	case core.HistoryRangeAll:
		return "1month"
	default:
		return "1day"
	}
}

func twelveDataOutputSize(interval core.HistoryInterval) string {
	switch interval {
	case core.HistoryRange1h:
		return "24"
	case core.HistoryRange1d:
		return "120"
	case core.HistoryRange1w:
		return "10"
	case core.HistoryRange1mo:
		return "40"
	case core.HistoryRange1y:
		return "260"
	case core.HistoryRange3y:
		return "170"
	default:
		return "120"
	}
}
