// provider_yahoo.go — Yahoo Finance quote and history provider.
package marketdata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync"
	"time"

	"investgo/internal/datasource"
	"investgo/internal/monitor"
)

// ---------------------------------------------------------------------------
// Yahoo HTTP infrastructure
// ---------------------------------------------------------------------------

type yahooChartResponse struct {
	Chart struct {
		Result []struct {
			Meta struct {
				Currency  string  `json:"currency"`
				Symbol    string  `json:"symbol"`
				ShortName string  `json:"shortName"`
				LongName  string  `json:"longName"`
				Price     float64 `json:"regularMarketPrice"`
			} `json:"meta"`
			Timestamp  []int64 `json:"timestamp"`
			Indicators struct {
				Quote []struct {
					Open   []*float64 `json:"open"`
					High   []*float64 `json:"high"`
					Low    []*float64 `json:"low"`
					Close  []*float64 `json:"close"`
					Volume []*float64 `json:"volume"`
				} `json:"quote"`
			} `json:"indicators"`
		} `json:"result"`
		Error *struct {
			Description string `json:"description"`
		} `json:"error"`
	} `json:"chart"`
}

var yahooCookieJarOnce sync.Once
var yahooCookieJar http.CookieJar

func getYahooCookieJar() http.CookieJar {
	yahooCookieJarOnce.Do(func() {
		jar, err := cookiejar.New(nil)
		if err == nil {
			yahooCookieJar = jar
		}
	})
	return yahooCookieJar
}

func cloneYahooClient(client *http.Client) *http.Client {
	cloned := *client
	if cloned.Timeout == 0 {
		cloned.Timeout = 10 * time.Second
	}
	if cloned.Jar == nil {
		cloned.Jar = getYahooCookieJar()
	}
	return &cloned
}

func setYahooBrowserHeaders(request *http.Request, host string) {
	request.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15")
	request.Header.Set("Accept", "application/json,text/plain,*/*")
	request.Header.Set("Accept-Language", "en-US,en;q=0.9")
	request.Header.Set("Origin", datasource.YahooFinanceOrigin)
	request.Header.Set("Referer", datasource.YahooFinanceReferer)
	request.Header.Set("Sec-Fetch-Site", "same-site")
	request.Header.Set("Sec-Fetch-Mode", "cors")
	request.Header.Set("Sec-Fetch-Dest", "empty")
	request.Header.Set("Cache-Control", "no-cache")
	request.Header.Set("Pragma", "no-cache")
	request.Header.Set("Connection", "keep-alive")
	request.Host = host
}

func primeYahooSession(ctx context.Context, client *http.Client) error {
	if client == nil {
		return errors.New("client is nil")
	}
	if client.Jar == nil {
		return errors.New("cookie jar is not configured")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, datasource.YahooFinanceOrigin, nil)
	if err != nil {
		return err
	}
	setYahooBrowserHeaders(req, datasource.YahooFinanceDomain)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}

// fetchYahooChart polls multiple Yahoo Finance hosts for quote data, returning the first successful response or a combined error message.
func fetchYahooChart(ctx context.Context, client *http.Client, symbol string, params url.Values) (yahooChartResponse, error) {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}

	primedClient := cloneYahooClient(client)
	_ = primeYahooSession(ctx, primedClient)

	problems := make([]string, 0, len(datasource.YahooChartHosts))
	for _, host := range datasource.YahooChartHosts {
		parsed, err := fetchYahooChartFromHost(ctx, primedClient, host, symbol, params)
		if err == nil {
			return parsed, nil
		}
		problems = append(problems, fmt.Sprintf("%s: %v", host, err))
	}

	return yahooChartResponse{}, monitor.JoinProblems(problems)
}

// fetchYahooChartFromHost sends a request to the specified Yahoo Finance host, parses the response and handles possible errors.
func fetchYahooChartFromHost(ctx context.Context, client *http.Client, host, symbol string, params url.Values) (yahooChartResponse, error) {
	query := make(url.Values, len(params))
	for key, values := range params {
		query[key] = append([]string(nil), values...)
	}
	query.Set("corsDomain", datasource.YahooFinanceDomain)

	requestURL := url.URL{
		Scheme:   "https",
		Host:     host,
		Path:     datasource.YahooChartPathPrefix + url.PathEscape(symbol),
		RawQuery: query.Encode(),
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL.String(), nil)
	if err != nil {
		return yahooChartResponse{}, err
	}
	setYahooBrowserHeaders(request, host)

	response, err := client.Do(request)
	if err != nil {
		return yahooChartResponse{}, err
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return yahooChartResponse{}, err
	}

	if response.StatusCode != http.StatusOK {
		var parsed yahooChartResponse
		if err := json.Unmarshal(body, &parsed); err == nil && parsed.Chart.Error != nil {
			return yahooChartResponse{}, errors.New(parsed.Chart.Error.Description)
		}
		return yahooChartResponse{}, fmt.Errorf("status %d", response.StatusCode)
	}

	var parsed yahooChartResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return yahooChartResponse{}, err
	}
	if parsed.Chart.Error != nil {
		return yahooChartResponse{}, errors.New(parsed.Chart.Error.Description)
	}
	if len(parsed.Chart.Result) == 0 {
		return yahooChartResponse{}, errors.New("No results returned")
	}

	return parsed, nil
}

// ---------------------------------------------------------------------------
// Yahoo quote provider
// ---------------------------------------------------------------------------

type YahooQuoteProvider struct {
	client *http.Client
}

func NewYahooQuoteProvider(client *http.Client) *YahooQuoteProvider {
	if client == nil {
		client = &http.Client{Timeout: 8 * time.Second}
	}

	return &YahooQuoteProvider{client: client}
}

func (p *YahooQuoteProvider) Name() string {
	return "Yahoo Finance"
}

// Fetch requests Yahoo Finance real-time quotes in batch and maps them to the standard Quote structure.
func (p *YahooQuoteProvider) Fetch(ctx context.Context, items []monitor.WatchlistItem) (map[string]monitor.Quote, error) {
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

		yahooSymbol, err := resolveYahooSymbol(item)
		if err != nil {
			problems = append(problems, fmt.Sprintf("Yahoo does not support item: %s", target.DisplaySymbol))
			continue
		}

		quote, err := p.fetchChartSnapshot(ctx, item, yahooSymbol)
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

// fetchChartSnapshot calls the Yahoo Finance chart API, parses the last 5 days of daily data,
// and builds a Quote from the latest price point.
func (p *YahooQuoteProvider) fetchChartSnapshot(ctx context.Context, item monitor.WatchlistItem, yahooSymbol string) (monitor.Quote, error) {
	params := url.Values{}
	params.Set("range", "5d")
	params.Set("interval", "1d")
	params.Set("includePrePost", "false")
	params.Set("events", "div,splits")

	parsed, err := fetchYahooChart(ctx, p.client, yahooSymbol, params)
	if err != nil {
		return monitor.Quote{}, fmt.Errorf("Yahoo quote request failed: %w", err)
	}
	if len(parsed.Chart.Result) == 0 || len(parsed.Chart.Result[0].Indicators.Quote) == 0 {
		return monitor.Quote{}, errors.New("Yahoo quote response is empty")
	}

	result := parsed.Chart.Result[0]
	points := buildHistoryPoints(result.Timestamp, result.Indicators.Quote[0])
	if len(points) == 0 {
		return monitor.Quote{}, errors.New("Yahoo quote response contains no valid price points")
	}

	latest := points[len(points)-1]
	previousClose := latest.Open
	if len(points) >= 2 && points[len(points)-2].Close > 0 {
		previousClose = points[len(points)-2].Close
	}
	if previousClose <= 0 {
		previousClose = latest.Close
	}

	quote := buildQuote(
		firstNonEmpty(result.Meta.LongName, result.Meta.ShortName, item.Name, result.Meta.Symbol, item.Symbol),
		firstNonEmptyFloat(result.Meta.Price, latest.Close),
		previousClose,
		latest.Open,
		latest.High,
		latest.Low,
		latest.Timestamp,
		p.Name(),
	)
	quote.Currency = firstNonEmpty(result.Meta.Currency, item.Currency)
	return quote, nil
}

// ---------------------------------------------------------------------------
// Yahoo history (chart) provider
// ---------------------------------------------------------------------------

type YahooChartProvider struct {
	client *http.Client
}

type historyQuerySpec struct {
	requestRange    string
	requestInterval string
	trimWindow      time.Duration
}

func NewYahooChartProvider(client *http.Client) *YahooChartProvider {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}

	return &YahooChartProvider{client: client}
}

func (p *YahooChartProvider) Name() string {
	return "Yahoo Finance"
}

// Fetch implements the monitor.HistoryProvider interface,
// fetching historical quote data from Yahoo Finance and converting it to the unified format.
func (p *YahooChartProvider) Fetch(ctx context.Context, item monitor.WatchlistItem, interval monitor.HistoryInterval) (monitor.HistorySeries, error) {
	yahooSymbol, err := resolveYahooSymbol(item)
	if err != nil {
		return monitor.HistorySeries{}, err
	}

	spec, err := historyQuerySpecFor(interval)
	if err != nil {
		return monitor.HistorySeries{}, err
	}

	params := url.Values{}
	params.Set("range", spec.requestRange)
	params.Set("interval", spec.requestInterval)
	params.Set("includePrePost", "false")
	params.Set("events", "div,splits")

	parsed, err := fetchYahooChart(ctx, p.client, yahooSymbol, params)
	if err != nil {
		return monitor.HistorySeries{}, fmt.Errorf("History request failed: %w", err)
	}

	result := parsed.Chart.Result[0]
	if len(result.Indicators.Quote) == 0 {
		return monitor.HistorySeries{}, errors.New("History response is missing price data")
	}

	points := buildHistoryPoints(result.Timestamp, result.Indicators.Quote[0])
	points = trimHistoryPoints(points, spec.trimWindow)
	if len(points) == 0 {
		return monitor.HistorySeries{}, errors.New("History response contains no valid price points")
	}

	series := monitor.HistorySeries{
		Symbol:      item.Symbol,
		Name:        firstNonEmpty(item.Name, item.Symbol),
		Market:      item.Market,
		Currency:    firstNonEmpty(result.Meta.Currency, item.Currency),
		Interval:    interval,
		Source:      p.Name(),
		Points:      points,
		GeneratedAt: time.Now(),
	}
	applyHistorySummary(&series)
	return series, nil
}

func resolveYahooSymbol(item monitor.WatchlistItem) (string, error) {
	target, err := monitor.ResolveQuoteTarget(item)
	if err != nil {
		return "", err
	}

	switch target.Market {
	case "CN-A", "CN-GEM", "CN-STAR", "CN-ETF":
		if strings.HasSuffix(target.DisplaySymbol, ".SH") {
			return strings.TrimSuffix(target.DisplaySymbol, ".SH") + ".SS", nil
		}
		if strings.HasSuffix(target.DisplaySymbol, ".SZ") {
			return target.DisplaySymbol, nil
		}
	case "HK-MAIN", "HK-GEM", "HK-ETF":
		digits := strings.TrimLeft(strings.TrimSuffix(target.DisplaySymbol, ".HK"), "0")
		if digits == "" {
			digits = "0"
		}
		if len(digits) < 4 {
			digits = strings.Repeat("0", 4-len(digits)) + digits
		}
		return digits + ".HK", nil
	case "US-STOCK", "US-ETF":
		return target.DisplaySymbol, nil
	}

	return "", fmt.Errorf("Yahoo does not support market: %s", target.DisplaySymbol)
}

// historyQuerySpecFor returns the query parameters and data trim window suitable for the Yahoo Finance API based on the user-selected history range.
func historyQuerySpecFor(interval monitor.HistoryInterval) (historyQuerySpec, error) {
	switch interval {
	case monitor.HistoryRange1h:
		return historyQuerySpec{requestRange: "1d", requestInterval: "1m", trimWindow: time.Hour}, nil
	case monitor.HistoryRange1d:
		return historyQuerySpec{requestRange: "1d", requestInterval: "1m", trimWindow: 24 * time.Hour}, nil
	case monitor.HistoryRange1w:
		return historyQuerySpec{requestRange: "5d", requestInterval: "5m", trimWindow: 7 * 24 * time.Hour}, nil
	case monitor.HistoryRange1mo:
		return historyQuerySpec{requestRange: "1mo", requestInterval: "1d", trimWindow: 30 * 24 * time.Hour}, nil
	case monitor.HistoryRange1y:
		return historyQuerySpec{requestRange: "1y", requestInterval: "1d", trimWindow: 365 * 24 * time.Hour}, nil
	case monitor.HistoryRange3y:
		return historyQuerySpec{requestRange: "5y", requestInterval: "1wk", trimWindow: 3 * 365 * 24 * time.Hour}, nil
	case monitor.HistoryRangeAll:
		return historyQuerySpec{requestRange: "max", requestInterval: "1mo", trimWindow: 0}, nil
	default:
		return historyQuerySpec{}, errors.New("History interval must be one of: 1h / 1d / 1w / 1mo / 1y / 3y / all")
	}
}

// buildHistoryPoints constructs a unified list of historical price points from raw Yahoo Finance data,
// automatically filtering out invalid entries.
func buildHistoryPoints(timestamps []int64, quote struct {
	Open   []*float64 `json:"open"`
	High   []*float64 `json:"high"`
	Low    []*float64 `json:"low"`
	Close  []*float64 `json:"close"`
	Volume []*float64 `json:"volume"`
}) []monitor.HistoryPoint {
	limit := len(timestamps)
	limit = minInt(limit, len(quote.Open))
	limit = minInt(limit, len(quote.High))
	limit = minInt(limit, len(quote.Low))
	limit = minInt(limit, len(quote.Close))
	if len(quote.Volume) > 0 {
		limit = minInt(limit, len(quote.Volume))
	}

	points := make([]monitor.HistoryPoint, 0, limit)
	for index := 0; index < limit; index++ {
		open := derefFloat(quote.Open[index])
		high := derefFloat(quote.High[index])
		low := derefFloat(quote.Low[index])
		closePrice := derefFloat(quote.Close[index])
		if closePrice <= 0 {
			continue
		}

		volume := 0.0
		if len(quote.Volume) > index {
			volume = derefFloat(quote.Volume[index])
		}

		points = append(points, monitor.HistoryPoint{
			Timestamp: time.Unix(timestamps[index], 0),
			Open:      open,
			High:      high,
			Low:       low,
			Close:     closePrice,
			Volume:    volume,
		})
	}

	return points
}

func derefFloat(value *float64) float64 {
	if value == nil {
		return 0
	}
	return *value
}
