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
	"strings"
	"time"
)

// load 从磁盘加载状态文件；若文件不存在则写入一份种子状态。
func (s *Store) load() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}

	payload, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		s.state = seedState()
		s.runtime.QuoteSource = s.quoteProviderSummaryLocked()
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
	s.runtime.QuoteSource = s.quoteProviderSummaryLocked()
	s.logInfo("storage", fmt.Sprintf("loaded state from %s", s.path))
	return nil
}

// normaliseLocked 兼容旧状态文件，并把缺省字段回落到安全默认值。
func (s *Store) normaliseLocked() {
	if s.state.Items == nil {
		s.state.Items = []WatchlistItem{}
	}
	if s.state.Alerts == nil {
		s.state.Alerts = []AlertRule{}
	}

	// 旧版本状态可能缺少新增字段，这里统一补齐默认设置。
	if s.state.Settings.RefreshIntervalSeconds <= 0 {
		s.state.Settings.RefreshIntervalSeconds = 60
	}
	legacySource := strings.ToLower(strings.TrimSpace(s.state.Settings.QuoteSource))
	if s.state.Settings.CNQuoteSource == "" {
		s.state.Settings.CNQuoteSource = legacySource
	}
	if s.state.Settings.HKQuoteSource == "" {
		s.state.Settings.HKQuoteSource = legacySource
	}
	if s.state.Settings.USQuoteSource == "" {
		s.state.Settings.USQuoteSource = legacySource
	}
	s.state.Settings.CNQuoteSource = s.normaliseQuoteSourceIDLocked(s.state.Settings.CNQuoteSource, "CN-A")
	s.state.Settings.HKQuoteSource = s.normaliseQuoteSourceIDLocked(s.state.Settings.HKQuoteSource, "HK-MAIN")
	s.state.Settings.USQuoteSource = s.normaliseQuoteSourceIDLocked(s.state.Settings.USQuoteSource, "US-STOCK")
	if _, ok := s.quoteProviders[legacySource]; ok {
		s.state.Settings.QuoteSource = legacySource
	} else {
		s.state.Settings.QuoteSource = DefaultQuoteSourceID
	}
	if s.state.Settings.FontPreset == "" {
		s.state.Settings.FontPreset = "system"
	}
	if s.state.Settings.ThemeMode == "" {
		s.state.Settings.ThemeMode = "system"
	}
	if s.state.Settings.ColorTheme == "" {
		s.state.Settings.ColorTheme = "blue"
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
	if s.state.Settings.DashboardCurrency == "" {
		s.state.Settings.DashboardCurrency = "CNY"
	}
	switch s.state.Settings.HotUSSource {
	case "eastmoney", "yahoo":
		// valid
	default:
		s.state.Settings.HotUSSource = "eastmoney"
	}

	// 历史状态里的条目可能缺少 ID、名称或更新时间，这里顺手补全。
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

// saveLocked 使用临时文件加原子替换的方式持久化状态。
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

// snapshotLocked 返回用于前端消费的只读快照副本。
func (s *Store) snapshotLocked() StateSnapshot {
	items := append([]WatchlistItem{}, s.state.Items...)
	alerts := append([]AlertRule{}, s.state.Alerts...)
	quoteSources := append([]QuoteSourceOption{}, s.quoteSourceOptions...)
	runtime := s.runtime
	runtime.QuoteSource = s.quoteProviderSummaryLocked()
	runtime.LivePriceCount = countLiveQuotes(items)

	// 快照排序只影响输出顺序，不反向修改内部持久化切片顺序。
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
		Dashboard:    buildDashboard(items, alerts, s.fxRates, s.state.Settings.DashboardCurrency),
		Items:        items,
		Alerts:       alerts,
		Settings:     s.state.Settings,
		Runtime:      runtime,
		QuoteSources: quoteSources,
		StoragePath:  s.path,
		GeneratedAt:  time.Now(),
	}
}

// evaluateAlertsLocked 根据当前价格重新计算所有提醒的触发状态。
func (s *Store) evaluateAlertsLocked() {
	// 先建索引，避免在提醒循环里重复扫描全部标的。
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

// findItemIndexLocked 返回指定标的在状态切片中的下标，不存在时返回 -1。
func (s *Store) findItemIndexLocked(id string) int {
	for idx := range s.state.Items {
		if s.state.Items[idx].ID == id {
			return idx
		}
	}
	return -1
}

// findAlertIndexLocked 返回指定提醒在状态切片中的下标，不存在时返回 -1。
func (s *Store) findAlertIndexLocked(id string) int {
	for idx := range s.state.Alerts {
		if s.state.Alerts[idx].ID == id {
			return idx
		}
	}
	return -1
}

// quoteProviderNameLocked 返回当前激活行情源的显示名称。
func (s *Store) quoteProviderNameLocked(market string) string {
	if provider := s.activeQuoteProviderLocked(market); provider != nil {
		return provider.Name()
	}

	return "Manual"
}

func (s *Store) quoteProviderSummaryLocked() string {
	parts := []string{
		"A股 " + s.quoteProviderNameLocked("CN-A"),
		"港股 " + s.quoteProviderNameLocked("HK-MAIN"),
		"美股 " + s.quoteProviderNameLocked("US-STOCK"),
	}
	return strings.Join(parts, " / ")
}

func defaultQuoteSourceIDForMarket(market string) string {
	switch marketGroupForMarket(market) {
	case "cn":
		return DefaultCNQuoteSourceID
	case "hk":
		return DefaultHKQuoteSourceID
	case "us":
		return DefaultUSQuoteSourceID
	default:
		return DefaultQuoteSourceID
	}
}

func marketGroupForMarket(market string) string {
	switch market {
	case "CN-A", "CN-GEM", "CN-STAR", "CN-ETF", "CN-BJ":
		return "cn"
	case "HK-MAIN", "HK-GEM", "HK-ETF":
		return "hk"
	case "US-STOCK", "US-ETF":
		return "us"
	default:
		return "cn"
	}
}

func (s *Store) quoteSourceIDForMarketLocked(market string) string {
	settings := s.state.Settings
	switch marketGroupForMarket(market) {
	case "hk":
		return s.normaliseQuoteSourceIDLocked(settings.HKQuoteSource, market)
	case "us":
		return s.normaliseQuoteSourceIDLocked(settings.USQuoteSource, market)
	default:
		return s.normaliseQuoteSourceIDLocked(settings.CNQuoteSource, market)
	}
}

func (s *Store) normaliseQuoteSourceIDLocked(sourceID, market string) string {
	sourceID = strings.ToLower(strings.TrimSpace(sourceID))
	if sourceID != "" {
		if _, ok := s.quoteProviders[sourceID]; ok && s.quoteSourceSupportsMarketLocked(sourceID, market) {
			return sourceID
		}
	}

	defaultID := defaultQuoteSourceIDForMarket(market)
	if _, ok := s.quoteProviders[defaultID]; ok && s.quoteSourceSupportsMarketLocked(defaultID, market) {
		return defaultID
	}

	for _, option := range s.quoteSourceOptions {
		if _, ok := s.quoteProviders[option.ID]; ok && s.quoteSourceSupportsMarketOption(option, market) {
			return option.ID
		}
	}

	for id := range s.quoteProviders {
		return id
	}

	return DefaultQuoteSourceID
}

func (s *Store) quoteSourceSupportsMarketLocked(sourceID, market string) bool {
	for _, option := range s.quoteSourceOptions {
		if option.ID == sourceID {
			return s.quoteSourceSupportsMarketOption(option, market)
		}
	}
	return false
}

func (s *Store) quoteSourceSupportsMarketOption(option QuoteSourceOption, market string) bool {
	if len(option.SupportedMarkets) == 0 {
		return true
	}
	for _, candidate := range option.SupportedMarkets {
		if candidate == market {
			return true
		}
	}
	return false
}

// activeQuoteProviderLocked 返回给定市场当前应生效的行情 provider。
func (s *Store) activeQuoteProviderLocked(market string) QuoteProvider {
	if len(s.quoteProviders) == 0 {
		return nil
	}

	sourceID := s.quoteSourceIDForMarketLocked(market)
	if provider, ok := s.quoteProviders[sourceID]; ok {
		return provider
	}

	for _, provider := range s.quoteProviders {
		return provider
	}

	return nil
}

// activeQuoteSourceIDLocked 返回当前有效的行情源 ID。
func (s *Store) activeQuoteSourceIDLocked(market string) string {
	return s.quoteSourceIDForMarketLocked(market)
}

func (s *Store) historyProviderCandidatesLocked(market string) []HistoryProvider {
	if len(s.historyProviders) == 0 {
		return nil
	}

	preferredSource := s.quoteSourceIDForMarketLocked(market)
	candidates := make([]HistoryProvider, 0, 3)
	seen := make(map[string]struct{}, 3)
	appendProvider := func(id string) {
		if id == "" {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		provider, ok := s.historyProviders[id]
		if !ok {
			return
		}
		seen[id] = struct{}{}
		candidates = append(candidates, provider)
	}

	appendProvider(preferredSource)
	switch marketGroupForMarket(market) {
	case "us":
		appendProvider("yahoo")
		appendProvider("eastmoney")
	default:
		appendProvider("eastmoney")
		appendProvider("yahoo")
	}

	return candidates
}

// buildDashboard 基于标的、提醒和汇率信息构建仪表盘汇总数据。
func buildDashboard(items []WatchlistItem, alerts []AlertRule, fx *FxRates, displayCurrency string) DashboardSummary {
	var summary DashboardSummary
	summary.ItemCount = len(items)

	if displayCurrency == "" {
		displayCurrency = "CNY"
	}
	summary.DisplayCurrency = displayCurrency

	// 先把各标的成本和市值折算到统一展示货币，再做组合聚合。
	for _, item := range items {
		costBasis := item.CostBasis()
		marketValue := item.MarketValue()

		itemCurrency := strings.ToUpper(strings.TrimSpace(item.Currency))
		if fx != nil && itemCurrency != "" && itemCurrency != displayCurrency {
			costBasis = fx.Convert(costBasis, itemCurrency, displayCurrency)
			marketValue = fx.Convert(marketValue, itemCurrency, displayCurrency)
		}

		summary.TotalCost += costBasis
		summary.TotalValue += marketValue
		// 只有有实际持仓（Quantity > 0）且录入了成本价的标的才参与盈亏计数。
		// 纯观察标的（Quantity=0，CostPrice=0）以及定投后成本归零的边缘情况均排除在外。
		if item.Quantity > 0 && item.CostPrice > 0 {
			if item.CurrentPrice > item.CostPrice {
				summary.WinCount++
			} else if item.CurrentPrice < item.CostPrice {
				summary.LossCount++
			}
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

// seedState 返回首次启动时使用的示例状态。
func seedState() persistedState {
	now := time.Now()
	items := []WatchlistItem{
		{
			ID:           newID("item"),
			Symbol:       "600519.SH",
			Name:         "贵州茅台",
			Market:       "CN-A",
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
			Market:       "HK-MAIN",
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
			Market:       "US-ETF",
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

// newID 生成带前缀的随机 ID；随机数不可用时退回时间戳方案。
func newID(prefix string) string {
	buffer := make([]byte, 6)
	if _, err := rand.Read(buffer); err != nil {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}
	return prefix + "-" + hex.EncodeToString(buffer)
}

// ptrTime 返回给定时间值的独立指针副本。
func ptrTime(value time.Time) *time.Time {
	copy := value
	return &copy
}

// nonZeroTime 把零值时间回落为当前时间。
func nonZeroTime(value time.Time) time.Time {
	if value.IsZero() {
		return time.Now()
	}
	return value
}
