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

// applyHistorySummary 根据历史点序列计算涨跌摘要和区间高低点。
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

// minInt 返回两个整数中的较小值。
func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

// trimHistoryPoints 截取窗口内的历史点，保留原始时间顺序。
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

// chinaLocation 定义了中国时区，供解析东方财富返回的时间戳使用。
// 东方财富的 K 线接口返回的时间戳是中国时间，需要用这个时区来正确解析。
var chinaLocation = time.FixedZone("CST", 8*3600)

// EastMoneyChartProvider 通过东方财富 K 线接口抓取历史行情数据。
// 应用当前统一使用该接口作为历史图表数据源。
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
	klt        int           // K 线周期（101=日，102=周，103=月，60=60min）
	beg        string        // 起始日期 YYYYMMDD；"0" 表示最早
	end        string        // 截止日期 YYYYMMDD
	lmt        int           // 最多返回多少根 K 线（0=不限）
	intraday   bool          // 是否为分钟级别（时间戳含时分秒）
	trimWindow time.Duration // 截取最近多长时间的数据（0=不截取）
}

// NewEastMoneyChartProvider 创建东方财富历史行情 provider。
func NewEastMoneyChartProvider(client *http.Client) *EastMoneyChartProvider {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &EastMoneyChartProvider{client: client}
}

// Name 返回东方财富历史源的显示名称。
func (p *EastMoneyChartProvider) Name() string {
	return "EastMoney"
}

// Fetch 通过东方财富 K 线接口抓取历史行情数据。
func (p *EastMoneyChartProvider) Fetch(ctx context.Context, item monitor.WatchlistItem, interval monitor.HistoryInterval) (monitor.HistorySeries, error) {
	target, err := monitor.ResolveQuoteTarget(item)
	if err != nil {
		return monitor.HistorySeries{}, fmt.Errorf("EastMoney history failed to resolve item %s: %w", item.Symbol, err)
	}

	secid, err := resolveEastMoneySecID(target)
	if err != nil {
		return monitor.HistorySeries{}, fmt.Errorf("EastMoney history failed to resolve secid: %w", err)
	}

	// 东方财富不同周期依赖不同的 klt、日期窗口和裁剪策略。
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

// eastMoneyHistorySpecFor 把图表区间映射为东方财富 K 线请求参数。
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

// parseEastMoneyKlines 把东方财富 K 线字符串列表解析为历史点切片。
// kline 字段顺序：日期, 开盘, 收盘, 最高, 最低, 成交量, 成交额（逗号分隔）。
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

// NewSmartHistoryProvider 创建包含东方财富和 Yahoo 两个历史行情提供者的映射表，
// 供 monitor.NewStore 使用。client 为 nil 时各 provider 使用自身默认超时。
func NewSmartHistoryProvider(client *http.Client) map[string]monitor.HistoryProvider {
	return map[string]monitor.HistoryProvider{
		"eastmoney": NewEastMoneyChartProvider(client),
		"yahoo":     NewYahooChartProvider(client),
	}
}
