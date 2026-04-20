// provider_twelvedata.go - Twelve Data quote and history provider (US only, API key required).
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

type TwelveDataQuoteProvider struct {
	client   *http.Client
	settings func() monitor.AppSettings
}

type TwelveDataHistoryProvider struct {
	client   *http.Client
	settings func() monitor.AppSettings
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

func NewTwelveDataQuoteProvider(client *http.Client, settings func() monitor.AppSettings) *TwelveDataQuoteProvider {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	if settings == nil {
		settings = func() monitor.AppSettings { return monitor.AppSettings{} }
	}
	return &TwelveDataQuoteProvider{client: client, settings: settings}
}

func (p *TwelveDataQuoteProvider) Name() string { return "Twelve Data" }

func (p *TwelveDataQuoteProvider) Fetch(ctx context.Context, items []monitor.WatchlistItem) (map[string]monitor.Quote, error) {
	apiKey := strings.TrimSpace(p.settings().TwelveDataAPIKey)
	if apiKey == "" {
		return nil, errors.New("Twelve Data API key is required")
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
		quote.Currency = firstNonEmpty(quote.Currency, target.Currency)
		quotes[target.Key] = quote
	}

	return quotes, monitor.JoinProblems(problems)
}

func NewTwelveDataHistoryProvider(client *http.Client, settings func() monitor.AppSettings) *TwelveDataHistoryProvider {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	if settings == nil {
		settings = func() monitor.AppSettings { return monitor.AppSettings{} }
	}
	return &TwelveDataHistoryProvider{client: client, settings: settings}
}

func (p *TwelveDataHistoryProvider) Name() string { return "Twelve Data" }

func (p *TwelveDataHistoryProvider) Fetch(ctx context.Context, item monitor.WatchlistItem, interval monitor.HistoryInterval) (monitor.HistorySeries, error) {
	apiKey := strings.TrimSpace(p.settings().TwelveDataAPIKey)
	if apiKey == "" {
		return monitor.HistorySeries{}, errors.New("Twelve Data API key is required")
	}
	target, err := monitor.ResolveQuoteTarget(item)
	if err != nil {
		return monitor.HistorySeries{}, err
	}
	if target.Market != "US-STOCK" && target.Market != "US-ETF" {
		return monitor.HistorySeries{}, fmt.Errorf("Twelve Data does not support market: %s", target.DisplaySymbol)
	}

	points, currency, err := fetchTwelveDataHistory(ctx, p.client, target.DisplaySymbol, interval, apiKey)
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

func fetchTwelveDataQuote(ctx context.Context, client *http.Client, symbol, fallbackName, fallbackCurrency, apiKey string) (monitor.Quote, error) {
	params := url.Values{}
	params.Set("symbol", symbol)
	params.Set("apikey", apiKey)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, datasource.URLWithQuery(datasource.TwelveDataQuoteAPI, params), nil)
	if err != nil {
		return monitor.Quote{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return monitor.Quote{}, fmt.Errorf("Twelve Data quote request failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return monitor.Quote{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return monitor.Quote{}, fmt.Errorf("Twelve Data quote request failed: status %d", resp.StatusCode)
	}
	var parsed twelveDataQuoteResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return monitor.Quote{}, err
	}
	if parsed.Status == "error" || parsed.Code != 0 {
		return monitor.Quote{}, errors.New(firstNonEmpty(parsed.Message, "Twelve Data quote response is empty"))
	}
	quote := buildQuote(
		firstNonEmpty(parsed.Name, fallbackName, symbol),
		parseFloat(parsed.Close),
		parseFloat(parsed.PreviousClose),
		parseFloat(parsed.Open),
		parseFloat(parsed.High),
		parseFloat(parsed.Low),
		time.Now(),
		"Twelve Data",
	)
	if quote.Change == 0 {
		quote.Change = parseFloat(parsed.Change)
	}
	if quote.ChangePercent == 0 {
		quote.ChangePercent = parseFloat(strings.TrimSuffix(parsed.PercentChange, "%"))
	}
	quote.Currency = firstNonEmpty(parsed.Currency, fallbackCurrency)
	return quote, nil
}

func fetchTwelveDataHistory(ctx context.Context, client *http.Client, symbol string, interval monitor.HistoryInterval, apiKey string) ([]monitor.HistoryPoint, string, error) {
	params := url.Values{}
	params.Set("symbol", symbol)
	params.Set("apikey", apiKey)
	params.Set("interval", twelveDataInterval(interval))
	params.Set("outputsize", twelveDataOutputSize(interval))
	params.Set("order", "ASC")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, datasource.URLWithQuery(datasource.TwelveDataTimeSeriesAPI, params), nil)
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
		return nil, "", errors.New(firstNonEmpty(parsed.Message, "History response contains no valid price points"))
	}
	points := make([]monitor.HistoryPoint, 0, len(parsed.Values))
	for _, value := range parsed.Values {
		pointTime := parseUSAPITimestamp(value.Datetime)
		if pointTime.IsZero() {
			continue
		}
		points = append(points, monitor.HistoryPoint{
			Timestamp: pointTime,
			Open:      parseFloat(value.Open),
			High:      parseFloat(value.High),
			Low:       parseFloat(value.Low),
			Close:     parseFloat(value.Close),
			Volume:    parseFloat(value.Volume),
		})
	}
	points = trimHistoryPoints(points, historyTrimWindow(interval))
	return points, parsed.Meta.Currency, nil
}

func twelveDataInterval(interval monitor.HistoryInterval) string {
	switch interval {
	case monitor.HistoryRange1h:
		return "5min"
	case monitor.HistoryRange1d:
		return "15min"
	case monitor.HistoryRange3y:
		return "1week"
	case monitor.HistoryRangeAll:
		return "1month"
	default:
		return "1day"
	}
}

func twelveDataOutputSize(interval monitor.HistoryInterval) string {
	switch interval {
	case monitor.HistoryRange1h:
		return "24"
	case monitor.HistoryRange1d:
		return "120"
	case monitor.HistoryRange1w:
		return "10"
	case monitor.HistoryRange1mo:
		return "40"
	case monitor.HistoryRange1y:
		return "260"
	case monitor.HistoryRange3y:
		return "170"
	default:
		return "120"
	}
}
