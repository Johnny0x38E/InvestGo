package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"investgo/internal/datasource"
)

// FxRates 缓存各货币对人民币的汇率，用于仪表盘多币种合并计算。
// 所有汇率以"1单位外币 = X 人民币"为基准存储。
// 使用 Frankfurter API（欧洲央行数据），缓存至少 2 小时。
type FxRates struct {
	mu        sync.RWMutex
	rates     map[string]float64 // 外币 → 人民币
	validAt   time.Time
	lastError string // 最近一次获取失败的错误信息
	client    *http.Client
}

// NewFxRates 创建汇率服务，初始化时仅包含 CNY=1.0。
func NewFxRates(client *http.Client) *FxRates {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &FxRates{
		client: client,
		rates:  map[string]float64{"CNY": 1.0},
	}
}

// IsStale 返回汇率缓存是否已超过 2 小时。
func (f *FxRates) IsStale() bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.validAt.IsZero() || time.Since(f.validAt) > 2*time.Hour
}

// LastError 返回最近一次获取失败的错误信息，成功时为空。
func (f *FxRates) LastError() string {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.lastError
}

// ValidAt 返回最近一次成功获取汇率的时间。
func (f *FxRates) ValidAt() time.Time {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.validAt
}

// CurrencyCount 返回当前缓存中的币种数量。
func (f *FxRates) CurrencyCount() int {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return len(f.rates)
}

// frankfurterResponse 是 Frankfurter API 响应的结构。
type frankfurterResponse struct {
	Base  string             `json:"base"`
	Date  string             `json:"date"`
	Rates map[string]float64 `json:"rates"`
}

// Fetch 从 Frankfurter API 拉取各货币对 CNY 的汇率。
// 以 CNY 为 base 获取各外币汇率，然后取倒数得到"外币→人民币"的映射。
// 成功时清除 lastError，失败时记录错误信息。
func (f *FxRates) Fetch(ctx context.Context) {
	url := datasource.FrankfurterAPI + "?from=CNY"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		f.setError(fmt.Sprintf("Failed to create FX request: %v", err))
		return
	}

	resp, err := f.client.Do(req)
	if err != nil {
		f.setError(fmt.Sprintf("FX service is unreachable: %v", err))
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		f.setError(fmt.Sprintf("Failed to read FX response: %v", err))
		return
	}

	if resp.StatusCode != http.StatusOK {
		detail := string(body)
		if len(detail) > 200 {
			detail = detail[:200]
		}
		f.setError(fmt.Sprintf("FX service returned %d: %s", resp.StatusCode, detail))
		return
	}

	var data frankfurterResponse
	if err := json.Unmarshal(body, &data); err != nil {
		f.setError(fmt.Sprintf("Failed to decode FX data: %v", err))
		return
	}
	if data.Base != "CNY" || len(data.Rates) == 0 {
		f.setError("FX payload is invalid")
		return
	}

	newRates := make(map[string]float64, len(data.Rates)+1)
	newRates["CNY"] = 1.0
	for currency, rate := range data.Rates {
		if rate > 0 {
			// rate 是 1 CNY = X 外币，取倒数得到 1 外币 = X CNY
			newRates[currency] = 1.0 / rate
		}
	}

	f.mu.Lock()
	f.rates = newRates
	f.validAt = time.Now()
	f.lastError = ""
	f.mu.Unlock()
}

func (f *FxRates) setError(msg string) {
	f.mu.Lock()
	f.lastError = msg
	f.mu.Unlock()
}

// Convert 将给定金额从来源货币转换为目标货币，以 CNY 作为中间货币。
// 若货币相同或无法解析，返回原值。
func (f *FxRates) Convert(value float64, from, to string) float64 {
	from = strings.ToUpper(strings.TrimSpace(from))
	to = strings.ToUpper(strings.TrimSpace(to))
	if from == to || value == 0 {
		return value
	}

	f.mu.RLock()
	defer f.mu.RUnlock()

	fromRate, ok := f.rates[from]
	if !ok || fromRate <= 0 {
		return value
	}
	cnyValue := value * fromRate

	if to == "CNY" {
		return cnyValue
	}
	toRate, ok := f.rates[to]
	if !ok || toRate <= 0 {
		return cnyValue
	}
	return cnyValue / toRate
}
