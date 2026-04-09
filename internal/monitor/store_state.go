package monitor

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

func (s *Store) load() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}

	payload, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		s.state = seedState()
		s.runtime.QuoteSource = s.quoteProviderNameLocked()
		s.logInfo("storage", fmt.Sprintf("state file not found, seeding %s", s.path))
		return s.Save()
	}
	if err != nil {
		s.logError("storage", fmt.Sprintf("read state failed: %v", err))
		return err
	}

	if err := json.Unmarshal(payload, &s.state); err != nil {
		s.logError("storage", fmt.Sprintf("decode state failed: %v", err))
		return err
	}

	s.normaliseLocked()
	s.runtime.QuoteSource = s.quoteProviderNameLocked()
	s.logInfo("storage", fmt.Sprintf("loaded state from %s", s.path))
	return nil
}

// normaliseLocked 用来兼容旧状态文件，确保新增字段缺省时也能回落到安全默认值。
func (s *Store) normaliseLocked() {
	if s.state.Items == nil {
		s.state.Items = []WatchlistItem{}
	}
	if s.state.Alerts == nil {
		s.state.Alerts = []AlertRule{}
	}

	if s.state.Settings.PriceMode == "" {
		s.state.Settings.PriceMode = "live"
	}
	if s.state.Settings.RefreshIntervalSeconds <= 0 {
		s.state.Settings.RefreshIntervalSeconds = 20
	}
	if _, ok := s.quoteProviders[s.state.Settings.QuoteSource]; !ok {
		s.state.Settings.QuoteSource = defaultQuoteSourceID
	}
	if s.state.Settings.FontPreset == "" {
		s.state.Settings.FontPreset = "system"
	}
	if s.state.Settings.AmountDisplay == "" {
		s.state.Settings.AmountDisplay = "full"
	}
	if s.state.Settings.CurrencyDisplay == "" {
		s.state.Settings.CurrencyDisplay = "symbol"
	}
	if s.state.Settings.PriceColorScheme == "" {
		s.state.Settings.PriceColorScheme = "cn"
	}
	if s.state.Settings.Locale == "" {
		s.state.Settings.Locale = "system"
	}

	for idx := range s.state.Items {
		item, err := sanitiseItem(s.state.Items[idx])
		if err != nil {
			continue
		}
		if item.ID == "" {
			item.ID = newID("item")
		}
		if item.Name == "" {
			item.Name = item.Symbol
		}
		if item.UpdatedAt.IsZero() {
			item.UpdatedAt = time.Now()
		}
		s.state.Items[idx] = item
	}

	for idx := range s.state.Alerts {
		alert, err := sanitiseAlert(s.state.Alerts[idx])
		if err != nil {
			continue
		}
		if alert.ID == "" {
			alert.ID = newID("alert")
		}
		if alert.UpdatedAt.IsZero() {
			alert.UpdatedAt = time.Now()
		}
		s.state.Alerts[idx] = alert
	}

	s.evaluateAlertsLocked()
}

// saveLocked 采用临时文件再原子替换的方式，减少异常退出时把 state.json 写坏的风险。
func (s *Store) saveLocked() error {
	payload, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return err
	}

	tempPath := s.path + ".tmp"
	if err := os.WriteFile(tempPath, payload, 0o644); err != nil {
		return err
	}

	return os.Rename(tempPath, s.path)
}

// snapshotLocked 只返回排序后的副本，避免前端读取顺序影响内部持久化状态。
func (s *Store) snapshotLocked() StateSnapshot {
	items := append([]WatchlistItem{}, s.state.Items...)
	alerts := append([]AlertRule{}, s.state.Alerts...)
	quoteSources := append([]QuoteSourceOption{}, s.quoteSourceOptions...)
	runtime := s.runtime
	runtime.QuoteSource = s.quoteProviderNameLocked()
	runtime.LivePriceCount = countLiveQuotes(items)

	sort.Slice(items, func(i, j int) bool {
		return items[i].UpdatedAt.After(items[j].UpdatedAt)
	})
	sort.Slice(alerts, func(i, j int) bool {
		if alerts[i].Triggered != alerts[j].Triggered {
			return alerts[i].Triggered
		}
		return alerts[i].UpdatedAt.After(alerts[j].UpdatedAt)
	})

	return StateSnapshot{
		Dashboard:    buildDashboard(items, alerts),
		Items:        items,
		Alerts:       alerts,
		Settings:     s.state.Settings,
		Runtime:      runtime,
		QuoteSources: quoteSources,
		StoragePath:  s.path,
		GeneratedAt:  time.Now(),
	}
}

func (s *Store) evaluateAlertsLocked() {
	priceByItem := make(map[string]float64, len(s.state.Items))
	for _, item := range s.state.Items {
		priceByItem[item.ID] = item.CurrentPrice
	}

	now := time.Now()
	for idx := range s.state.Alerts {
		alert := &s.state.Alerts[idx]
		alert.Triggered = false
		if !alert.Enabled {
			continue
		}

		price, ok := priceByItem[alert.ItemID]
		if !ok || price <= 0 {
			continue
		}

		triggered := false
		switch alert.Condition {
		case AlertAbove:
			triggered = price >= alert.Threshold
		case AlertBelow:
			triggered = price <= alert.Threshold
		}

		alert.Triggered = triggered
		if triggered {
			alert.LastTriggeredAt = &now
		}
	}
}

func (s *Store) findItemIndexLocked(id string) int {
	for idx := range s.state.Items {
		if s.state.Items[idx].ID == id {
			return idx
		}
	}
	return -1
}

func (s *Store) findAlertIndexLocked(id string) int {
	for idx := range s.state.Alerts {
		if s.state.Alerts[idx].ID == id {
			return idx
		}
	}
	return -1
}

func (s *Store) quoteProviderNameLocked() string {
	if provider := s.activeQuoteProviderLocked(); provider != nil {
		return provider.Name()
	}

	return "Manual"
}

func (s *Store) activeQuoteProviderLocked() QuoteProvider {
	if len(s.quoteProviders) == 0 {
		return nil
	}

	if provider, ok := s.quoteProviders[s.state.Settings.QuoteSource]; ok {
		return provider
	}

	if provider, ok := s.quoteProviders[defaultQuoteSourceID]; ok {
		return provider
	}

	for _, option := range s.quoteSourceOptions {
		if provider, ok := s.quoteProviders[option.ID]; ok {
			return provider
		}
	}

	for _, provider := range s.quoteProviders {
		return provider
	}

	return nil
}

func (s *Store) activeQuoteSourceIDLocked() string {
	if _, ok := s.quoteProviders[s.state.Settings.QuoteSource]; ok {
		return s.state.Settings.QuoteSource
	}
	return defaultQuoteSourceID
}

func buildDashboard(items []WatchlistItem, alerts []AlertRule) DashboardSummary {
	var summary DashboardSummary
	summary.ItemCount = len(items)

	for _, item := range items {
		summary.TotalCost += item.CostBasis()
		summary.TotalValue += item.MarketValue()
		if item.CurrentPrice > item.CostPrice {
			summary.WinCount++
		} else if item.CurrentPrice < item.CostPrice {
			summary.LossCount++
		}
	}

	summary.TotalPnL = summary.TotalValue - summary.TotalCost
	if summary.TotalCost > 0 {
		summary.TotalPnLPct = summary.TotalPnL / summary.TotalCost * 100
	}

	for _, alert := range alerts {
		if alert.Triggered {
			summary.TriggeredAlerts++
		}
	}

	return summary
}

func seedState() persistedState {
	now := time.Now()
	items := []WatchlistItem{
		{
			ID:           newID("item"),
			Symbol:       "600519.SH",
			Name:         "贵州茅台",
			Market:       "A-Share",
			Currency:     "CNY",
			Quantity:     20,
			CostPrice:    1680,
			CurrentPrice: 1728,
			Thesis:       "高端白酒现金流稳定，适合作为组合压舱石。",
			Tags:         []string{"白酒", "核心仓"},
			UpdatedAt:    now.Add(-4 * time.Hour),
		},
		{
			ID:           newID("item"),
			Symbol:       "00700.HK",
			Name:         "腾讯控股",
			Market:       "HK",
			Currency:     "HKD",
			Quantity:     100,
			CostPrice:    310,
			CurrentPrice: 328,
			Thesis:       "广告和游戏现金牛，适合中长期跟踪估值修复。",
			Tags:         []string{"互联网平台", "观察"},
			UpdatedAt:    now.Add(-2 * time.Hour),
		},
		{
			ID:           newID("item"),
			Symbol:       "QQQ",
			Name:         "Invesco QQQ Trust",
			Market:       "US ETF",
			Currency:     "USD",
			Quantity:     15,
			CostPrice:    430,
			CurrentPrice: 447,
			Thesis:       "用来观察美股科技主线风险偏好。",
			Tags:         []string{"ETF", "科技"},
			UpdatedAt:    now.Add(-90 * time.Minute),
		},
	}

	alerts := []AlertRule{
		{
			ID:        newID("alert"),
			ItemID:    items[0].ID,
			Name:      "茅台上破观察位",
			Condition: AlertAbove,
			Threshold: 1750,
			Enabled:   true,
			UpdatedAt: now.Add(-45 * time.Minute),
		},
		{
			ID:        newID("alert"),
			ItemID:    items[1].ID,
			Name:      "腾讯回撤止损位",
			Condition: AlertBelow,
			Threshold: 300,
			Enabled:   true,
			UpdatedAt: now.Add(-30 * time.Minute),
		},
		{
			ID:        newID("alert"),
			ItemID:    items[2].ID,
			Name:      "QQQ 上破趋势确认位",
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
			PriceMode:              "live",
			RefreshIntervalSeconds: 20,
			QuoteSource:            defaultQuoteSourceID,
			FontPreset:             "system",
			AmountDisplay:          "full",
			CurrencyDisplay:        "symbol",
			PriceColorScheme:       "cn",
			Locale:                 "system",
			DeveloperMode:          false,
		},
	}

	store := &Store{state: state}
	store.evaluateAlertsLocked()
	store.state.UpdatedAt = now
	return store.state
}

func newID(prefix string) string {
	buffer := make([]byte, 6)
	if _, err := rand.Read(buffer); err != nil {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}
	return prefix + "-" + hex.EncodeToString(buffer)
}

func ptrTime(value time.Time) *time.Time {
	copy := value
	return &copy
}

func nonZeroTime(value time.Time) time.Time {
	if value.IsZero() {
		return time.Now()
	}
	return value
}
