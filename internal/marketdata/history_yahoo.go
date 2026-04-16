package marketdata

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"investgo/internal/monitor"
)

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

// Fetch 实现了 monitor.HistoryProvider 接口，负责从 Yahoo Finance 获取历史行情数据并转换为统一格式。
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

// historyQuerySpecFor 根据用户选择的历史范围返回适合 Yahoo Finance API 的查询参数和数据修剪窗口。
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

// buildHistoryPoints 从 Yahoo Finance 的原始数据中构造统一的历史价格点列表，自动过滤掉无效数据。
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

// DefaultHistorySourceRegistry 返回默认历史行情源注册表。
func DefaultHistorySourceRegistry(client *http.Client) map[string]monitor.HistoryProvider {
	return map[string]monitor.HistoryProvider{
		"eastmoney": NewEastMoneyChartProvider(client),
		"yahoo":     NewYahooChartProvider(client),
	}
}
