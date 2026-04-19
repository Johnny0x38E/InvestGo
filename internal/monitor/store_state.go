package monitor

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// load loads state file from disk; if file does not exist, write a seed state.
func (s *Store) load() error {
	if s.repository == nil {
		return fmt.Errorf("state repository is not configured")
	}

	state := persistedState{}
	found, err := s.repository.Load(&state)
	if !found && err == nil {
		s.state = seedState()
		s.runtime.QuoteSource = s.quoteProviderSummaryLocked()
		s.logInfo("storage", fmt.Sprintf("state file not found, seeding %s", s.repository.Path()))
		return s.Save()
	}
	if err != nil {
		s.logError("storage", fmt.Sprintf("read state failed: %v", err))
		return err
	}

	s.state = state
	s.normaliseLocked()
	s.runtime.QuoteSource = s.quoteProviderSummaryLocked()
	s.logInfo("storage", fmt.Sprintf("loaded state from %s", s.repository.Path()))
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
	if s.state.Settings.HotCacheTTLSeconds <= 0 {
		s.state.Settings.HotCacheTTLSeconds = 60
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
	if s.state.Settings.ProxyMode == "" {
		s.state.Settings.ProxyMode = "system"
	}
	if s.state.Settings.DashboardCurrency == "" {
		s.state.Settings.DashboardCurrency = "CNY"
	}
	s.state.Settings.HotUSSource = s.state.Settings.USQuoteSource

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
	if s.repository == nil {
		return fmt.Errorf("state repository is not configured")
	}
	return s.repository.Save(s.state)
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

	for index := range items {
		items[index] = decorateItemDerived(items[index])
	}

	return StateSnapshot{
		Dashboard:    buildDashboard(items, alerts, s.fxRates, s.state.Settings.DashboardCurrency),
		Items:        items,
		Alerts:       alerts,
		Settings:     s.state.Settings,
		Runtime:      runtime,
		QuoteSources: quoteSources,
		StoragePath:  s.repository.Path(),
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

// quoteSourceIDForMarketLocked returns the quote source ID that should be effective for the given market.
// The legacy single-field QuoteSource compatibility path is handled during state normalization and settings sanitisation,
// so runtime selection only depends on the market-specific settings plus fallback rules.
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
		// Only items with an actual position (Quantity > 0) and a recorded cost price contribute to the win/loss tally.
		// Watch-only items and zero-cost DCA edge cases are excluded from this count.
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
