package monitor

import (
	"context"
	"io"
	"maps"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"investgo/internal/datasource"
)

// FxRates 缓存常用货币对人民币的汇率，用于仪表盘多币种合并计算。
// 所有汇率以"1单位外币 = X 人民币"为基准存储。
type FxRates struct {
	mu      sync.RWMutex
	rates   map[string]float64
	validAt time.Time
	client  *http.Client
}

// fallbackFxRates 在接口不可用时使用的兜底汇率（粗略值）。
var fallbackFxRates = map[string]float64{
	"CNY": 1.0,
	"USD": 6.9,
	"HKD": 0.85,
}

// NewFxRates 创建汇率服务，并初始化兜底汇率。
func NewFxRates(client *http.Client) *FxRates {
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	f := &FxRates{
		client: client,
		rates:  make(map[string]float64, len(fallbackFxRates)),
	}
	maps.Copy(f.rates, fallbackFxRates)
	return f
}

// IsStale 返回汇率缓存是否已超过允许的陈旧时间。
func (f *FxRates) IsStale() bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.validAt.IsZero() || time.Since(f.validAt) > 4*time.Hour
}

// Fetch 从新浪财经拉取 USD/CNY 和 HKD/CNY 实时汇率。
// 若拉取失败，保留既有汇率（兜底或上次成功值）不报错。
func (f *FxRates) Fetch(ctx context.Context) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, datasource.SinaFXRatesAPI, nil)
	if err != nil {
		return
	}
	req.Header.Set("Referer", datasource.SinaFinanceReferer)
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := f.client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return
	}

	parsed := parseSinaFxRates(string(body))
	if len(parsed) == 0 {
		return
	}

	f.mu.Lock()
	maps.Copy(f.rates, parsed)
	f.rates["CNY"] = 1.0
	f.validAt = time.Now()
	f.mu.Unlock()
}

// Convert 将给定金额从来源货币转换为目标货币，并以 CNY 作为中间货币。
// 若货币相同或无法解析，返回原值。
func (f *FxRates) Convert(value float64, from, to string) float64 {
	from = strings.ToUpper(strings.TrimSpace(from))
	to = strings.ToUpper(strings.TrimSpace(to))
	if from == to || value == 0 {
		return value
	}

	f.mu.RLock()
	defer f.mu.RUnlock()

	// 先折算成 CNY，再从 CNY 换算到目标货币，避免维护全量货币对。
	fromRate := f.rateOrFallback(from)
	if fromRate <= 0 {
		return value
	}
	cnyValue := value * fromRate

	if to == "CNY" {
		return cnyValue
	}
	toRate := f.rateOrFallback(to)
	if toRate <= 0 {
		return cnyValue
	}
	return cnyValue / toRate
}

// rateOrFallback 返回指定货币的可用汇率，不存在时回退到兜底值。
func (f *FxRates) rateOrFallback(currency string) float64 {
	if r, ok := f.rates[currency]; ok && r > 0 {
		return r
	}
	return fallbackFxRates[currency]
}

// parseSinaFxRates 解析新浪财经汇率响应，并返回货币到人民币的汇率映射。
// 期望格式：var hq_str_usdcny="美元人民币,7.2516,...";
func parseSinaFxRates(raw string) map[string]float64 {
	rates := make(map[string]float64)
	// strings.SplitSeq 返回迭代器，可避免一次性切分大字符串带来的额外分配。
	for line := range strings.SplitSeq(raw, "\n") {
		line = strings.TrimSpace(line)
		var currency string
		switch {
		case strings.Contains(line, "hq_str_usdcny"):
			currency = "USD"
		case strings.Contains(line, "hq_str_hkdcny"):
			currency = "HKD"
		default:
			continue
		}

		start := strings.Index(line, `"`)
		end := strings.LastIndex(line, `"`)
		if start < 0 || end <= start {
			continue
		}
		parts := strings.Split(line[start+1:end], ",")
		if len(parts) < 2 {
			continue
		}
		if r := parseFloat(parts[1]); r > 0 {
			rates[currency] = r
		}
	}
	return rates
}

// parseFloat 把接口返回的数值字符串安全解析为 float64。
func parseFloat(raw string) float64 {
	clean := strings.TrimSpace(strings.NewReplacer("\"", "", ";", "", ",", "").Replace(raw))
	if clean == "" || clean == "-" {
		return 0
	}

	value, err := strconv.ParseFloat(clean, 64)
	if err != nil {
		return 0
	}
	return value
}
