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

// load loads state file from disk; if file does not exist, write a seed state.
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

// normaliseLocked is compatible with old state files and falls back missing fields to safe default values.
func (s *Store) normaliseLocked() {
	if s.state.Items == nil {
		s.state.Items = []WatchlistItem{}
	}
	if s.state.Alerts == nil {
		s.state.Alerts = []AlertRule{}
	}

	// Old version states may be missing new fields; here we uniformly populate default settings.
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

	// Items in historical states may be missing ID, name, or update time; here we complete them.
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

// saveLocked persists state using a temporary file with atomic replacement.
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

// snapshotLocked returns a read-only snapshot copy for frontend consumption.
func (s *Store) snapshotLocked() StateSnapshot {
	items := append([]WatchlistItem{}, s.state.Items...)
	alerts := append([]AlertRule{}, s.state.Alerts...)
	quoteSources := append([]QuoteSourceOption{}, s.quoteSourceOptions...)
	runtime := s.runtime
	runtime.QuoteSource = s.quoteProviderSummaryLocked()
	runtime.LivePriceCount = countLiveQuotes(items)

	// Snapshot sorting only affects output order, not internal persisted slice order.
	sort.Slice(items, func(i, j int) bool {
		if items[i].PinnedAt != nil || items[j].PinnedAt != nil {
			if items[i].PinnedAt == nil {
				return false
			}
			if items[j].PinnedAt == nil {
				return true
			}
			if !items[i].PinnedAt.Equal(*items[j].PinnedAt) {
				return items[i].PinnedAt.After(*items[j].PinnedAt)
			}
		}
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

// evaluateAlertsLocked recalculates trigger status of all alerts based on current prices.
func (s *Store) evaluateAlertsLocked() {
	// First build index to avoid repeatedly scanning all items in the alert loop.
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

// findItemIndexLocked returns the index of the specified item in the state slice; returns -1 if not found.
func (s *Store) findItemIndexLocked(id string) int {
	for idx := range s.state.Items {
		if s.state.Items[idx].ID == id {
			return idx
		}
	}
	return -1
}

// findAlertIndexLocked returns the index of the specified alert in the state slice; returns -1 if not found.
func (s *Store) findAlertIndexLocked(id string) int {
	for idx := range s.state.Alerts {
		if s.state.Alerts[idx].ID == id {
			return idx
		}
	}
	return -1
}

// quoteProviderNameLocked returns the display name of the currently active quote source.
func (s *Store) quoteProviderNameLocked(market string) string {
	if provider := s.activeQuoteProviderLocked(market); provider != nil {
		return provider.Name()
	}

	return "Manual"
}

// quoteProviderSummaryLocked returns a brief description of current quote sources for each market for UI display.
func (s *Store) quoteProviderSummaryLocked() string {
	parts := []string{
		"CN " + s.quoteProviderNameLocked("CN-A"),
		"HK " + s.quoteProviderNameLocked("HK-MAIN"),
		"US " + s.quoteProviderNameLocked("US-STOCK"),
	}
	return strings.Join(parts, " / ")
}

// defaultQuoteSourceIDForMarket returns the default quote source ID for the given market.
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

// marketGroupForMarket groups specific markets into broader market groups for settings and logic reuse.
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

// quoteSourceIDForMarketLocked returns the quote source ID that should be effective for the given market, with priority:
// 1. Market-specific settings (HKQuoteSource, USQuoteSource, CNQuoteSource)
// 2. General settings (QuoteSource)
// 3. Market-specific defaults (defaultQuoteSourceIDForMarket)
// 4. First available option in quote source list that supports this market
// 5. First available option in quote source list
// 6. Built-in default DefaultQuoteSourceID
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

// normaliseQuoteSourceIDLocked validates and normalizes user-provided quote source ID,
// ensuring it is valid in available options and supports the specified market;
// otherwise falls back to reasonable defaults.
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

// quoteSourceSupportsMarketLocked checks whether the specified quote source ID is in available options and supports the given market.
func (s *Store) quoteSourceSupportsMarketLocked(sourceID, market string) bool {
	for _, option := range s.quoteSourceOptions {
		if option.ID == sourceID {
			return s.quoteSourceSupportsMarketOption(option, market)
		}
	}
	return false
}

// quoteSourceSupportsMarketOption checks whether the quote source option supports the given market;
// if SupportedMarkets is empty, it means all markets are supported.
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

// activeQuoteProviderLocked returns the quote provider that should be effective for the given market.
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

// activeQuoteSourceIDLocked returns the currently effective quote source ID.
func (s *Store) activeQuoteSourceIDLocked(market string) string {
	return s.quoteSourceIDForMarketLocked(market)
}

// historyProviderCandidatesLocked returns a list of historical data provider candidates for the given market, with priority:
// 1. Current quote source (if it also provides historical data)
// 2. Other default quote sources in the same market group (if user-set quote source does not support historical data, fall back to same market group default)
// 3. Other available quote sources in the same market group (if same market group default does not support historical data, fall back to other options in same market group)
// 4. Default quote sources in other market groups (if no options in same market group support historical data, fall back to other market group defaults)
// 5. Available quote sources in other market groups (if other market group default does not support historical data, fall back to other options in other market groups)
// 6. First available option in quote source list that provides historical data
// 7. Built-in default DefaultQuoteSourceID (if it provides historical data)
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

// buildDashboard builds dashboard summary data based on items, alerts, and FX rate information.
func buildDashboard(items []WatchlistItem, alerts []AlertRule, fx *FxRates, displayCurrency string) DashboardSummary {
	var summary DashboardSummary
	summary.ItemCount = len(items)

	if displayCurrency == "" {
		displayCurrency = "CNY"
	}
	summary.DisplayCurrency = displayCurrency

	// First convert each item's cost and market value to unified display currency, then perform portfolio aggregation.
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
		// Only items with actual holdings (Quantity > 0) and recorded cost price participate in PnL (Profit and Loss) counting.
		// Pure observation items (Quantity=0, CostPrice=0) and edge cases where cost becomes zero after DCA (Dollar-Cost Averaging) are excluded.
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
			ItemID:    items[1].ID,
			Name:      "阿里巴巴下破300止损",
			Condition: AlertBelow,
			Threshold: 300,
			Enabled:   true,
			UpdatedAt: now.Add(-30 * time.Minute),
		},
		{
			ID:        newID("alert"),
			ItemID:    items[2].ID,
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
