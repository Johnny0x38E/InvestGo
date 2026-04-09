package monitor

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// HistoryProvider hides provider-specific history quirks from the rest of the app.
// The UI only asks for day/week/month market trends for a watchlist item.
type HistoryProvider interface {
	Fetch(ctx context.Context, item WatchlistItem, interval HistoryInterval) (HistorySeries, error)
	Name() string
}

type YahooChartProvider struct {
	client *http.Client
}

type yahooChartResponse struct {
	Chart struct {
		Result []struct {
			Meta struct {
				Currency string  `json:"currency"`
				Symbol   string  `json:"symbol"`
				Price    float64 `json:"regularMarketPrice"`
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

func NewYahooChartProvider(client *http.Client) *YahooChartProvider {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}

	return &YahooChartProvider{client: client}
}

func (p *YahooChartProvider) Name() string {
	return "Yahoo Finance"
}

func (p *YahooChartProvider) Fetch(ctx context.Context, item WatchlistItem, interval HistoryInterval) (HistorySeries, error) {
	yahooSymbol, err := resolveYahooSymbol(item)
	if err != nil {
		return HistorySeries{}, err
	}

	rangeValue, intervalValue, err := historyQuery(interval)
	if err != nil {
		return HistorySeries{}, err
	}

	params := url.Values{}
	params.Set("range", rangeValue)
	params.Set("interval", intervalValue)
	params.Set("includePrePost", "false")
	params.Set("events", "div,splits")

	parsed, err := fetchYahooChart(ctx, p.client, yahooSymbol, params)
	if err != nil {
		return HistorySeries{}, fmt.Errorf("历史行情请求失败: %w", err)
	}

	result := parsed.Chart.Result[0]
	if len(result.Indicators.Quote) == 0 {
		return HistorySeries{}, errors.New("历史行情缺少价格数据")
	}

	quote := result.Indicators.Quote[0]
	points := buildHistoryPoints(result.Timestamp, quote)
	points = trimHistoryPoints(points, interval)
	if len(points) == 0 {
		return HistorySeries{}, errors.New("历史行情缺少有效价格点")
	}

	series := HistorySeries{
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

func historyQuery(interval HistoryInterval) (rangeValue string, intervalValue string, err error) {
	switch interval {
	case HistoryLive:
		return "1d", "1m", nil
	case HistoryHour1:
		return "1d", "1m", nil
	case HistoryHour6:
		return "5d", "5m", nil
	case HistoryDay:
		return "6mo", "1d", nil
	case HistoryWeek:
		return "3y", "1wk", nil
	case HistoryMonth:
		return "10y", "1mo", nil
	default:
		return "", "", errors.New("走势周期仅支持 live / 1h / 6h / day / week / month")
	}
}

func trimHistoryPoints(points []HistoryPoint, interval HistoryInterval) []HistoryPoint {
	switch interval {
	case HistoryHour1:
		return trimTrailingDuration(points, time.Hour)
	case HistoryHour6:
		return trimTrailingDuration(points, 6*time.Hour)
	case HistoryLive:
		return trimTrailingDuration(points, 24*time.Hour)
	default:
		return points
	}
}

func trimTrailingDuration(points []HistoryPoint, window time.Duration) []HistoryPoint {
	if len(points) == 0 {
		return points
	}

	cutoff := points[len(points)-1].Timestamp.Add(-window)
	start := 0
	for start < len(points)-1 && points[start].Timestamp.Before(cutoff) {
		start++
	}

	return append([]HistoryPoint(nil), points[start:]...)
}

func resolveYahooSymbol(item WatchlistItem) (string, error) {
	symbol := strings.ToUpper(strings.TrimSpace(item.Symbol))
	if symbol == "" {
		return "", errors.New("股票代码不能为空")
	}

	switch {
	case strings.HasSuffix(symbol, ".SH"):
		return strings.TrimSuffix(symbol, ".SH") + ".SS", nil
	case strings.HasSuffix(symbol, ".SZ"):
		return symbol, nil
	case strings.HasSuffix(symbol, ".HK"):
		digits := strings.TrimLeft(strings.TrimSuffix(symbol, ".HK"), "0")
		if digits == "" {
			digits = "0"
		}
		if len(digits) < 4 {
			digits = strings.Repeat("0", 4-len(digits)) + digits
		}
		return digits + ".HK", nil
	case strings.HasSuffix(symbol, ".BJ"):
		return "", fmt.Errorf("暂不支持北交所走势: %s", symbol)
	case isUSSymbol(symbol):
		return normaliseUSSymbol(symbol), nil
	default:
		return "", fmt.Errorf("无法转换为历史行情代码: %s", symbol)
	}
}

func buildHistoryPoints(timestamps []int64, quote struct {
	Open   []*float64 `json:"open"`
	High   []*float64 `json:"high"`
	Low    []*float64 `json:"low"`
	Close  []*float64 `json:"close"`
	Volume []*float64 `json:"volume"`
}) []HistoryPoint {
	limit := len(timestamps)
	limit = minInt(limit, len(quote.Open))
	limit = minInt(limit, len(quote.High))
	limit = minInt(limit, len(quote.Low))
	limit = minInt(limit, len(quote.Close))
	if len(quote.Volume) > 0 {
		limit = minInt(limit, len(quote.Volume))
	}

	points := make([]HistoryPoint, 0, limit)
	for index := 0; index < limit; index++ {
		open := derefFloat(quote.Open[index])
		high := derefFloat(quote.High[index])
		low := derefFloat(quote.Low[index])
		close := derefFloat(quote.Close[index])
		if close <= 0 {
			continue
		}

		volume := 0.0
		if len(quote.Volume) > index {
			volume = derefFloat(quote.Volume[index])
		}

		points = append(points, HistoryPoint{
			Timestamp: time.Unix(timestamps[index], 0),
			Open:      open,
			High:      high,
			Low:       low,
			Close:     close,
			Volume:    volume,
		})
	}

	return points
}

func applyHistorySummary(series *HistorySeries) {
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

func derefFloat(value *float64) float64 {
	if value == nil {
		return 0
	}
	return *value
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}
