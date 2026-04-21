package fx

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const frankfurterAPI = "https://api.frankfurter.dev/v1/latest"

// FxRates caches FX rates of various currencies against CNY for dashboard multi-currency aggregation.
// All rates are stored with the benchmark "1 unit of foreign currency = X CNY".
// Uses Frankfurter API (European Central Bank data), cached for at least 2 hours.
type FxRates struct {
	mu        sync.RWMutex
	fetchMu   sync.Mutex         // prevents concurrent in-flight fetches
	rates     map[string]float64 // foreign currency -> CNY
	validAt   time.Time
	lastError string // error message from the most recent fetch failure
	client    *http.Client
}

// NewFxRates creates FX rate service, initialized with only CNY=1.0.
func NewFxRates(client *http.Client) *FxRates {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &FxRates{
		client: client,
		rates:  map[string]float64{"CNY": 1.0},
	}
}

// NewFxRatesWithRates creates an FxRates instance pre-loaded with a specific rate map.
// Intended for use in tests that need deterministic conversion results without network calls.
func NewFxRatesWithRates(rates map[string]float64) *FxRates {
	if rates == nil {
		rates = map[string]float64{"CNY": 1.0}
	}
	return &FxRates{
		client: &http.Client{Timeout: 10 * time.Second},
		rates:  rates,
	}
}

// IsStale returns whether FX rate cache has exceeded 2 hours.
func (f *FxRates) IsStale() bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.validAt.IsZero() || time.Since(f.validAt) > 2*time.Hour
}

// LastError returns the error message from the most recent fetch failure; empty on success.
func (f *FxRates) LastError() string {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.lastError
}

// ValidAt returns the time of the most recent successful FX rate fetch.
func (f *FxRates) ValidAt() time.Time {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.validAt
}

// CurrencyCount returns the number of currencies currently cached.
func (f *FxRates) CurrencyCount() int {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return len(f.rates)
}

// frankfurterResponse is the structure of the Frankfurter API response.
type frankfurterResponse struct {
	Base  string             `json:"base"`
	Date  string             `json:"date"`
	Rates map[string]float64 `json:"rates"`
}

// Fetch fetches FX rates of various currencies against CNY from the Frankfurter API.
// Fetches foreign currency rates with CNY as base, then takes reciprocals to get "foreign currency → CNY" mapping.
// Clears lastError on success, records error message on failure.
func (f *FxRates) Fetch(ctx context.Context) {
	// Only one goroutine fetches at a time; concurrent callers return immediately.
	if !f.fetchMu.TryLock() {
		return
	}
	defer f.fetchMu.Unlock()

	url := frankfurterAPI + "?from=CNY"
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
			// Frankfurter returns rates as "1 CNY = X <currency>"; invert to store as "1 <currency> = X CNY".
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

// Convert converts a given amount from source currency to target currency, using CNY as the intermediate currency.
// Returns the original value if currencies are the same or cannot be resolved.
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
