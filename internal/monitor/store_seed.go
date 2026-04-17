package monitor

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// seedState returns sample state used on first startup.
func seedState() persistedState {
	now := time.Now()
	items := []WatchlistItem{
		{
			ID:           newID("item"),
			Symbol:       "09988.HK",
			Name:         "阿里巴巴-W",
			Market:       "HK-MAIN",
			Currency:     "HKD",
			Quantity:     100,
			CostPrice:    310,
			CurrentPrice: 328,
			Thesis:       "阿里巴巴港股，长期持有，关注电商和云计算业务发展",
			Tags:         []string{"互联网平台", "观察"},
			UpdatedAt:    now.Add(-2 * time.Hour),
		},
		{
			ID:           newID("item"),
			Symbol:       "VOO",
			Name:         "标普500ETF-Vanguard",
			Market:       "US-ETF",
			Currency:     "USD",
			Quantity:     15,
			CostPrice:    430,
			CurrentPrice: 447,
			Thesis:       "标普500指数ETF，分散投资美国大盘股，长期持有",
			Tags:         []string{"ETF"},
			UpdatedAt:    now.Add(-90 * time.Minute),
		},
	}

	alerts := []AlertRule{
		{
			ID:        newID("alert"),
			ItemID:    items[0].ID,
			Name:      "阿里巴巴下破300止损",
			Condition: AlertBelow,
			Threshold: 300,
			Enabled:   true,
			UpdatedAt: now.Add(-30 * time.Minute),
		},
		{
			ID:        newID("alert"),
			ItemID:    items[1].ID,
			Name:      "VOO 上破450止盈",
			Condition: AlertAbove,
			Threshold: 450,
			Enabled:   true,
			UpdatedAt: now.Add(-15 * time.Minute),
		},
	}

	state := persistedState{
		Items:  items,
		Alerts: alerts,
		Settings: AppSettings{
			RefreshIntervalSeconds: 60,
			QuoteSource:            DefaultQuoteSourceID,
			CNQuoteSource:          DefaultCNQuoteSourceID,
			HKQuoteSource:          DefaultHKQuoteSourceID,
			USQuoteSource:          DefaultUSQuoteSourceID,
			HotUSSource:            "eastmoney",
			ThemeMode:              "system",
			ColorTheme:             "blue",
			FontPreset:             "system",
			AmountDisplay:          "full",
			CurrencyDisplay:        "symbol",
			PriceColorScheme:       "cn",
			Locale:                 "system",
			DeveloperMode:          false,
			DashboardCurrency:      "CNY",
			UseNativeTitleBar:      false,
		},
	}

	store := &Store{state: state}
	store.evaluateAlertsLocked()
	store.state.UpdatedAt = now
	return store.state
}

// newID generates a prefixed random ID; falls back to timestamp scheme when random numbers are unavailable.
func newID(prefix string) string {
	buffer := make([]byte, 6)
	if _, err := rand.Read(buffer); err != nil {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}
	return prefix + "-" + hex.EncodeToString(buffer)
}

// ptrTime returns an independent pointer copy of the given time value.
func ptrTime(value time.Time) *time.Time {
	copy := value
	return &copy
}

// nonZeroTime falls back zero-value time to current time.
func nonZeroTime(value time.Time) time.Time {
	if value.IsZero() {
		return time.Now()
	}
	return value
}
