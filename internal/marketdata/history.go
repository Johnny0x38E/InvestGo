package marketdata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"investgo/internal/datasource"
	"investgo/internal/monitor"
)

// applyHistorySummary calculates the gain/loss summary and period high/low from the history point series.
func applyHistorySummary(series *monitor.HistorySeries) {
	if len(series.Points) == 0 {
		return
	}

	series.StartPrice = series.Points[0].Close
	series.EndPrice = series.Points[len(series.Points)-1].Close
	series.High = series.Points[0].High
	series.Low = series.Points[0].Low

	for _, point := range series.Points {
		if point.High > series.High {
			series.High = point.High
		}
		if point.Low < series.Low {
			series.Low = point.Low
		}
	}

	series.Change = series.EndPrice - series.StartPrice
	if series.StartPrice > 0 {
		series.ChangePercent = series.Change / series.StartPrice * 100
	}
}

// minInt returns the smaller of two integers.
func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

// trimHistoryPoints trims history points to the given window, preserving original chronological order.
func trimHistoryPoints(points []monitor.HistoryPoint, window time.Duration) []monitor.HistoryPoint {
	if window <= 0 || len(points) == 0 {
		return points
	}

	latest := points[len(points)-1].Timestamp
	if latest.IsZero() {
		return append([]monitor.HistoryPoint(nil), points...)
	}

	cutoff := latest.Add(-window)
	start := 0
	for start < len(points) && points[start].Timestamp.Before(cutoff) {
		start++
	}
	if start >= len(points) {
		return nil
	}
	return append([]monitor.HistoryPoint(nil), points[start:]...)
}

// ── EastMoney K-line chart provider ──────────────────────────────────────────

// chinaLocation defines the China time zone for parsing timestamps returned by EastMoney.
// EastMoney's K-line API returns timestamps in China time, which must be parsed with this location.
var chinaLocation = time.FixedZone("CST", 8*3600)

// EastMoneyChartProvider fetches historical quote data via the EastMoney K-line API.
// The app currently uses this API as the unified historical chart data source.
type EastMoneyChartProvider struct {
	client *http.Client
}

type eastMoneyKlineResponse struct {
	RC   int    `json:"rc"`
	Info string `json:"info,omitempty"`
	Data *struct {
		Code   string   `json:"code"`
		Market int      `json:"market"`
		Name   string   `json:"name"`
		KLines []string `json:"klines"`
	} `json:"data"`
}

type eastMoneyHistorySpec struct {
	klt        int           // candlestick period (101=daily, 102=weekly, 103=monthly, 60=60min)
	beg        string        // start date YYYYMMDD; "0" means earliest
	end        string        // end date YYYYMMDD
	lmt        int           // max number of candlesticks to return (0=unlimited)
	intraday   bool          // whether it is minute-level (timestamp includes hour:minute:second)
	trimWindow time.Duration // trim to the most recent duration (0=do not trim)
}

// NewEastMoneyChartProvider creates an EastMoney historical quote provider.
func NewEastMoneyChartProvider(client *http.Client) *EastMoneyChartProvider {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &EastMoneyChartProvider{client: client}
}

// Name returns the display name of the EastMoney history source.
func (p *EastMoneyChartProvider) Name() string {
	return "EastMoney"
}

// Fetch fetches historical quote data via the EastMoney K-line API.
func (p *EastMoneyChartProvider) Fetch(ctx context.Context, item monitor.WatchlistItem, interval monitor.HistoryInterval) (monitor.HistorySeries, error) {
	target, err := monitor.ResolveQuoteTarget(item)
	if err != nil {
		return monitor.HistorySeries{}, fmt.Errorf("EastMoney history failed to resolve item %s: %w", item.Symbol, err)
	}

	secid, err := resolveEastMoneySecID(target)
	if err != nil {
		return monitor.HistorySeries{}, fmt.Errorf("EastMoney history failed to resolve secid: %w", err)
	}

	// EastMoney uses different klt, date windows and trimming strategies for different intervals.
	spec, err := eastMoneyHistorySpecFor(interval)
	if err != nil {
		return monitor.HistorySeries{}, err
	}

	params := url.Values{}
	params.Set("secid", secid)
	params.Set("ut", "bd1d9ddb04089700cf9c27f6f7426281")
	params.Set("fields1", "f1,f2,f3,f4,f5,f6")
	params.Set("fields2", "f51,f52,f53,f54,f55,f56,f57")
	params.Set("klt", strconv.Itoa(spec.klt))
	params.Set("fqt", "1")
	params.Set("beg", spec.beg)
	params.Set("end", spec.end)
	if spec.lmt > 0 {
		params.Set("lmt", strconv.Itoa(spec.lmt))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		datasource.URLWithQuery(datasource.EastMoneyHistoryAPI, params), nil)
	if err != nil {
		return monitor.HistorySeries{}, err
	}
	req.Header.Set("Referer", datasource.EastMoneyWebReferer)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/123.0.0.0 Safari/537.36")

	resp, err := p.client.Do(req)
	if err != nil {
		return monitor.HistorySeries{}, fmt.Errorf("EastMoney history request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return monitor.HistorySeries{}, fmt.Errorf("EastMoney history request failed: status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return monitor.HistorySeries{}, err
	}

	var parsed eastMoneyKlineResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return monitor.HistorySeries{}, fmt.Errorf("EastMoney history decode failed: %w", err)
	}
	if parsed.RC != 0 {
		return monitor.HistorySeries{}, fmt.Errorf("EastMoney history response returned rc=%d", parsed.RC)
	}
	if parsed.Data == nil || len(parsed.Data.KLines) == 0 {
		return monitor.HistorySeries{}, errors.New("EastMoney history response is empty")
	}

	points := parseEastMoneyKlines(parsed.Data.KLines, spec.intraday)
	if len(points) == 0 {
		return monitor.HistorySeries{}, errors.New("EastMoney history contains no valid price points")
	}

	if spec.trimWindow > 0 {
		points = trimHistoryPoints(points, spec.trimWindow)
	}
	if len(points) == 0 {
		return monitor.HistorySeries{}, errors.New("EastMoney history contains no valid price points after trimming")
	}

	series := monitor.HistorySeries{
		Symbol:      item.Symbol,
		Name:        firstNonEmpty(parsed.Data.Name, item.Name, item.Symbol),
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

// eastMoneyHistorySpecFor maps a chart interval to EastMoney K-line request parameters.
func eastMoneyHistorySpecFor(interval monitor.HistoryInterval) (eastMoneyHistorySpec, error) {
	now := time.Now()
	end := now.AddDate(0, 0, 1).Format("20060102")

	switch interval {
	case monitor.HistoryRange1h:
		return eastMoneyHistorySpec{klt: 60, beg: now.AddDate(0, 0, -5).Format("20060102"), end: end, lmt: 50, intraday: true, trimWindow: time.Hour}, nil
	case monitor.HistoryRange1d:
		return eastMoneyHistorySpec{klt: 60, beg: now.AddDate(0, 0, -5).Format("20060102"), end: end, lmt: 50, intraday: true, trimWindow: 24 * time.Hour}, nil
	case monitor.HistoryRange1w:
		return eastMoneyHistorySpec{klt: 101, beg: now.AddDate(0, 0, -14).Format("20060102"), end: end, lmt: 10}, nil
	case monitor.HistoryRange1mo:
		return eastMoneyHistorySpec{klt: 101, beg: now.AddDate(0, -2, 0).Format("20060102"), end: end, lmt: 35}, nil
	case monitor.HistoryRange1y:
		return eastMoneyHistorySpec{klt: 101, beg: now.AddDate(-1, -1, 0).Format("20060102"), end: end, lmt: 270}, nil
	case monitor.HistoryRange3y:
		return eastMoneyHistorySpec{klt: 102, beg: now.AddDate(-3, -1, 0).Format("20060102"), end: end, lmt: 160}, nil
	case monitor.HistoryRangeAll:
		return eastMoneyHistorySpec{klt: 103, beg: "0", end: "20500101", lmt: 999}, nil
	default:
		return eastMoneyHistorySpec{}, fmt.Errorf("EastMoney does not support history interval: %s", interval)
	}
}

// parseEastMoneyKlines parses the EastMoney K-line string list into a slice of history points.
// K-line field order: date, open, close, high, low, volume, turnover (comma-separated).
func parseEastMoneyKlines(klines []string, intraday bool) []monitor.HistoryPoint {
	layout := "2006-01-02"
	if intraday {
		layout = "2006-01-02 15:04:05"
	}

	points := make([]monitor.HistoryPoint, 0, len(klines))
	for _, kline := range klines {
		parts := strings.SplitN(kline, ",", 8)
		if len(parts) < 6 {
			continue
		}
		t, err := time.ParseInLocation(layout, strings.TrimSpace(parts[0]), chinaLocation)
		if err != nil {
			continue
		}

		open := parseFloat(parts[1])
		closePrice := parseFloat(parts[2])
		high := parseFloat(parts[3])
		low := parseFloat(parts[4])
		volume := parseFloat(parts[5])

		if closePrice <= 0 {
			continue
		}

		points = append(points, monitor.HistoryPoint{
			Timestamp: t,
			Open:      open,
			High:      high,
			Low:       low,
			Close:     closePrice,
			Volume:    volume,
		})
	}
	return points
}

// NewSmartHistoryProvider creates a HistoryRouter that routes every history
// request through the market-aware dispatch layer.
//
//   - client is the shared HTTP client used by both underlying providers; pass
//     nil to use each provider default timeout.
//   - settings is called on every Fetch to read the current user preferences
//     (CNQuoteSource / HKQuoteSource / USQuoteSource).  Pass
//     store.CurrentSettings after the Store has been initialised; pass nil to
//     fall back to market defaults (EastMoney for CN/HK, Yahoo for US).
func NewSmartHistoryProvider(client *http.Client, settings func() monitor.AppSettings) monitor.HistoryProvider {
	if settings == nil {
		settings = func() monitor.AppSettings { return monitor.AppSettings{} }
	}
	providers := map[string]monitor.HistoryProvider{
		"eastmoney":     NewEastMoneyChartProvider(client),
		"yahoo":         NewYahooChartProvider(client),
		"alpha-vantage": NewAlphaVantageHistoryProvider(client, settings),
		"twelve-data":   NewTwelveDataHistoryProvider(client, settings),
	}
	return NewHistoryRouter(providers, settings)
}
