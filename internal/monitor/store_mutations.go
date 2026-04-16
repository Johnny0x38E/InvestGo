package monitor

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// UpsertItem 新增或更新自选标的，并在实时模式下尽量补齐最新行情。
func (s *Store) UpsertItem(input WatchlistItem) (StateSnapshot, error) {
	item, err := sanitiseItem(input)
	if err != nil {
		return StateSnapshot{}, err
	}

	// 先在读锁内取出运行时依赖和旧值，避免后续网络请求占用写锁。
	s.mu.RLock()
	provider := s.activeQuoteProviderLocked(item.Market)
	var existing *WatchlistItem
	if input.ID != "" {
		if index := s.findItemIndexLocked(input.ID); index >= 0 {
			copy := s.state.Items[index]
			existing = &copy
		}
	}
	s.mu.RUnlock()

	if existing != nil {
		item = inheritLiveFields(item, *existing)
	}

	if provider != nil {
		// 标的保存后立即补一跳行情，确保当前价始终来自统一行情源。
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		quotes, quoteErr := provider.Fetch(ctx, []WatchlistItem{item})
		cancel()

		if quoteErr == nil {
			if target, resolveErr := ResolveQuoteTarget(item); resolveErr == nil {
				if quote, ok := quotes[target.Key]; ok {
					applyQuoteToItem(&item, quote)
				}
			}
		}
	}

	if item.Name == "" {
		if existing != nil && existing.Name != "" {
			item.Name = existing.Name
		} else {
			item.Name = item.Symbol
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if item.ID == "" {
		item.ID = newID("item")
		item.UpdatedAt = time.Now()
		s.state.Items = append(s.state.Items, item)
		s.logInfo("watchlist", fmt.Sprintf("added item %s", item.Symbol))
	} else {
		index := s.findItemIndexLocked(item.ID)
		if index == -1 {
			return StateSnapshot{}, fmt.Errorf("Item not found: %s", item.ID)
		}
		item.UpdatedAt = time.Now()
		s.state.Items[index] = item
		s.logInfo("watchlist", fmt.Sprintf("updated item %s", item.Symbol))
	}

	s.runtime.QuoteSource = s.quoteProviderSummaryLocked()
	s.state.UpdatedAt = time.Now()
	s.evaluateAlertsLocked()
	if err := s.saveLocked(); err != nil {
		s.logError("storage", fmt.Sprintf("save state failed after item update: %v", err))
		return StateSnapshot{}, err
	}

	return s.snapshotLocked(), nil
}

// DeleteItem 删除指定标的，并同步删除其关联的提醒规则。
func (s *Store) DeleteItem(id string) (StateSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	index := s.findItemIndexLocked(id)
	if index == -1 {
		return StateSnapshot{}, fmt.Errorf("Item not found: %s", id)
	}

	itemSymbol := s.state.Items[index].Symbol
	s.state.Items = append(s.state.Items[:index], s.state.Items[index+1:]...)
	// 删除标的后，挂在该标的上的提醒也必须一起清掉，避免留下悬空引用。
	filteredAlerts := s.state.Alerts[:0]
	for _, alert := range s.state.Alerts {
		if alert.ItemID != id {
			filteredAlerts = append(filteredAlerts, alert)
		}
	}
	s.state.Alerts = filteredAlerts
	s.state.UpdatedAt = time.Now()

	if err := s.saveLocked(); err != nil {
		s.logError("storage", fmt.Sprintf("save state failed after item delete: %v", err))
		return StateSnapshot{}, err
	}

	s.logInfo("watchlist", fmt.Sprintf("deleted item %s", itemSymbol))

	return s.snapshotLocked(), nil
}

// UpsertAlert 新增或更新价格提醒规则。
func (s *Store) UpsertAlert(input AlertRule) (StateSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	alert, err := sanitiseAlert(input)
	if err != nil {
		return StateSnapshot{}, err
	}
	if s.findItemIndexLocked(alert.ItemID) == -1 {
		return StateSnapshot{}, fmt.Errorf("Alert item not found: %s", alert.ItemID)
	}

	if alert.ID == "" {
		alert.ID = newID("alert")
		alert.UpdatedAt = time.Now()
		s.state.Alerts = append(s.state.Alerts, alert)
		s.logInfo("alerts", fmt.Sprintf("created alert %s", alert.Name))
	} else {
		index := s.findAlertIndexLocked(alert.ID)
		if index == -1 {
			return StateSnapshot{}, fmt.Errorf("Alert not found: %s", alert.ID)
		}
		alert.UpdatedAt = time.Now()
		s.state.Alerts[index] = alert
		s.logInfo("alerts", fmt.Sprintf("updated alert %s", alert.Name))
	}

	s.state.UpdatedAt = time.Now()
	s.evaluateAlertsLocked()
	if err := s.saveLocked(); err != nil {
		s.logError("storage", fmt.Sprintf("save state failed after alert update: %v", err))
		return StateSnapshot{}, err
	}

	return s.snapshotLocked(), nil
}

// DeleteAlert 删除指定提醒规则。
func (s *Store) DeleteAlert(id string) (StateSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	index := s.findAlertIndexLocked(id)
	if index == -1 {
		return StateSnapshot{}, fmt.Errorf("Alert not found: %s", id)
	}

	alertName := s.state.Alerts[index].Name
	s.state.Alerts = append(s.state.Alerts[:index], s.state.Alerts[index+1:]...)
	s.state.UpdatedAt = time.Now()

	if err := s.saveLocked(); err != nil {
		s.logError("storage", fmt.Sprintf("save state failed after alert delete: %v", err))
		return StateSnapshot{}, err
	}

	s.logInfo("alerts", fmt.Sprintf("deleted alert %s", alertName))

	return s.snapshotLocked(), nil
}

// UpdateSettings 更新应用设置并立即持久化。
func (s *Store) UpdateSettings(input AppSettings) (StateSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	settings, err := sanitiseSettings(input, s.state.Settings, s.quoteProviders)
	if err != nil {
		return StateSnapshot{}, err
	}

	s.state.Settings = settings
	s.state.UpdatedAt = time.Now()
	if err := s.saveLocked(); err != nil {
		s.logError("storage", fmt.Sprintf("save state failed after settings update: %v", err))
		return StateSnapshot{}, err
	}

	s.logInfo(
		"settings",
		fmt.Sprintf(
			"updated settings: cn=%s hk=%s us=%s hotUS=%s refresh=%ds theme=%s color=%s developerMode=%t",
			settings.CNQuoteSource,
			settings.HKQuoteSource,
			settings.USQuoteSource,
			settings.HotUSSource,
			settings.RefreshIntervalSeconds,
			settings.ThemeMode,
			settings.ColorTheme,
			settings.DeveloperMode,
		),
	)

	return s.snapshotLocked(), nil
}

// sanitiseItem 规范化标的信息并执行基础合法性校验。
func sanitiseItem(input WatchlistItem) (WatchlistItem, error) {
	item := input
	item.Name = strings.TrimSpace(item.Name)
	item.Thesis = strings.TrimSpace(item.Thesis)
	item.Tags = normaliseTags(item.Tags)

	target, err := resolveQuoteTarget(item.Symbol, item.Market, item.Currency)
	if err != nil {
		return WatchlistItem{}, err
	}

	item.Symbol = target.DisplaySymbol
	item.Market = target.Market
	item.Currency = target.Currency
	item.QuoteSource = strings.TrimSpace(item.QuoteSource)

	// 若有定投记录，先过滤并规范化条目，再自动推算累计份额和加权均价。
	// 推算规则：
	//   1. 优先使用手动录入的买入价（Price > 0）：effectiveCost = Price × Shares
	//   2. 无买入价时，从总投入中扣除手续费：effectiveCost = max(Amount - Fee, 0)
	// 加权均价 = Σ effectiveCost_i / Σ Shares_i
	if len(item.DCAEntries) > 0 {
		valid := make([]DCAEntry, 0, len(item.DCAEntries))
		for _, e := range item.DCAEntries {
			if e.Shares <= 0 || e.Amount <= 0 {
				continue
			}
			if e.ID == "" {
				e.ID = newID("dca")
			}
			valid = append(valid, e)
		}
		item.DCAEntries = valid

		if len(item.DCAEntries) > 0 {
			totalShares := 0.0
			totalEffectiveCost := 0.0
			for _, e := range item.DCAEntries {
				totalShares += e.Shares
				var effectiveCost float64
				if e.Price > 0 {
					effectiveCost = e.Price * e.Shares
				} else {
					net := e.Amount - e.Fee
					if net < 0 {
						net = 0
					}
					effectiveCost = net
				}
				totalEffectiveCost += effectiveCost
			}
			item.Quantity = totalShares
			if totalShares > 0 {
				item.CostPrice = totalEffectiveCost / totalShares
			}
		}
	}

	if item.Quantity < 0 {
		return WatchlistItem{}, errors.New("Quantity must not be negative")
	}
	if item.CostPrice < 0 || item.CurrentPrice < 0 {
		return WatchlistItem{}, errors.New("Price must not be negative")
	}

	return item, nil
}

// sanitiseAlert 规范化提醒规则并执行基础合法性校验。
func sanitiseAlert(input AlertRule) (AlertRule, error) {
	alert := input
	alert.Name = strings.TrimSpace(alert.Name)
	alert.ItemID = strings.TrimSpace(alert.ItemID)

	if alert.Name == "" {
		return AlertRule{}, errors.New("Alert name is required")
	}
	if alert.ItemID == "" {
		return AlertRule{}, errors.New("Alert must reference an item")
	}
	if alert.Condition != AlertAbove && alert.Condition != AlertBelow {
		return AlertRule{}, errors.New("Alert condition is invalid")
	}
	if alert.Threshold <= 0 {
		return AlertRule{}, errors.New("Alert threshold must be greater than 0")
	}

	return alert, nil
}

// sanitiseSettings 把用户输入与当前配置合并，并执行统一的合法性校验。
func sanitiseSettings(input AppSettings, current AppSettings, quoteProviders map[string]QuoteProvider) (AppSettings, error) {
	settings := current
	if input.RefreshIntervalSeconds > 0 {
		settings.RefreshIntervalSeconds = input.RefreshIntervalSeconds
	}
	if strings.TrimSpace(input.QuoteSource) != "" {
		settings.QuoteSource = strings.ToLower(strings.TrimSpace(input.QuoteSource))
	}
	if strings.TrimSpace(input.CNQuoteSource) != "" {
		settings.CNQuoteSource = strings.ToLower(strings.TrimSpace(input.CNQuoteSource))
	}
	if strings.TrimSpace(input.HKQuoteSource) != "" {
		settings.HKQuoteSource = strings.ToLower(strings.TrimSpace(input.HKQuoteSource))
	}
	if strings.TrimSpace(input.USQuoteSource) != "" {
		settings.USQuoteSource = strings.ToLower(strings.TrimSpace(input.USQuoteSource))
	}
	if strings.TrimSpace(input.HotUSSource) != "" {
		settings.HotUSSource = strings.ToLower(strings.TrimSpace(input.HotUSSource))
	}
	if strings.TrimSpace(input.ThemeMode) != "" {
		settings.ThemeMode = strings.ToLower(strings.TrimSpace(input.ThemeMode))
	}
	if strings.TrimSpace(input.ColorTheme) != "" {
		settings.ColorTheme = strings.ToLower(strings.TrimSpace(input.ColorTheme))
	}
	if strings.TrimSpace(input.FontPreset) != "" {
		settings.FontPreset = strings.ToLower(strings.TrimSpace(input.FontPreset))
	}
	if strings.TrimSpace(input.AmountDisplay) != "" {
		settings.AmountDisplay = strings.ToLower(strings.TrimSpace(input.AmountDisplay))
	}
	if strings.TrimSpace(input.CurrencyDisplay) != "" {
		settings.CurrencyDisplay = strings.ToLower(strings.TrimSpace(input.CurrencyDisplay))
	}
	if strings.TrimSpace(input.PriceColorScheme) != "" {
		settings.PriceColorScheme = strings.ToLower(strings.TrimSpace(input.PriceColorScheme))
	}
	if strings.TrimSpace(input.Locale) != "" {
		settings.Locale = strings.TrimSpace(input.Locale)
	}
	if strings.TrimSpace(input.DashboardCurrency) != "" {
		settings.DashboardCurrency = strings.ToUpper(strings.TrimSpace(input.DashboardCurrency))
	}
	// 布尔值无法通过"空字符串"区分是否传入，这里显式采用输入值覆盖。
	settings.DeveloperMode = input.DeveloperMode
	settings.UseNativeTitleBar = input.UseNativeTitleBar

	if settings.RefreshIntervalSeconds < 10 {
		return AppSettings{}, errors.New("Refresh interval must be at least 10 seconds")
	}
	settings.CNQuoteSource = normaliseQuoteSourceIDForSettings(settings.CNQuoteSource, settings.QuoteSource, "CN-A", quoteProviders)
	settings.HKQuoteSource = normaliseQuoteSourceIDForSettings(settings.HKQuoteSource, settings.QuoteSource, "HK-MAIN", quoteProviders)
	settings.USQuoteSource = normaliseQuoteSourceIDForSettings(settings.USQuoteSource, settings.QuoteSource, "US-STOCK", quoteProviders)
	settings.QuoteSource = DefaultQuoteSourceID
	if len(quoteProviders) > 0 {
		if _, ok := quoteProviders[settings.CNQuoteSource]; !ok {
			return AppSettings{}, errors.New("China quote source is invalid")
		}
		if _, ok := quoteProviders[settings.HKQuoteSource]; !ok {
			return AppSettings{}, errors.New("Hong Kong quote source is invalid")
		}
		if _, ok := quoteProviders[settings.USQuoteSource]; !ok {
			return AppSettings{}, errors.New("US quote source is invalid")
		}
	}
	switch settings.FontPreset {
	case "", "system":
		settings.FontPreset = "system"
	case "reading", "compact":
	default:
		return AppSettings{}, errors.New("Font preset must be one of: system / reading / compact")
	}
	switch settings.ThemeMode {
	case "", "system":
		settings.ThemeMode = "system"
	case "light", "dark":
	default:
		return AppSettings{}, errors.New("Theme mode must be one of: system / light / dark")
	}
	switch settings.ColorTheme {
	case "", "blue":
		settings.ColorTheme = "blue"
	case "graphite", "forest", "sunset":
	default:
		return AppSettings{}, errors.New("Color theme must be one of: blue / graphite / forest / sunset")
	}
	switch settings.AmountDisplay {
	case "", "full":
		settings.AmountDisplay = "full"
	case "compact":
	default:
		return AppSettings{}, errors.New("Amount display must be one of: full / compact")
	}
	switch settings.CurrencyDisplay {
	case "", "symbol":
		settings.CurrencyDisplay = "symbol"
	case "code":
	default:
		return AppSettings{}, errors.New("Currency display must be one of: symbol / code")
	}
	switch settings.PriceColorScheme {
	case "", "cn":
		settings.PriceColorScheme = "cn"
	case "intl":
	default:
		return AppSettings{}, errors.New("Price color scheme must be one of: cn / intl")
	}
	switch settings.Locale {
	case "", "system":
		settings.Locale = "system"
	case "zh-CN", "en-US":
	default:
		return AppSettings{}, errors.New("Locale must be one of: system / zh-CN / en-US")
	}
	switch settings.DashboardCurrency {
	case "", "CNY":
		settings.DashboardCurrency = "CNY"
	case "HKD", "USD":
	default:
		return AppSettings{}, errors.New("Dashboard currency must be one of: CNY / HKD / USD")
	}
	switch settings.HotUSSource {
	case "", "eastmoney":
		settings.HotUSSource = "eastmoney"
	case "yahoo":
		// valid
	default:
		return AppSettings{}, errors.New("US hot source must be one of: eastmoney / yahoo")
	}

	return settings, nil
}

// normaliseQuoteSourceIDForSettings 根据用户输入、市场类型和可用行情源列表，确定最终使用的行情源 ID。
func normaliseQuoteSourceIDForSettings(sourceID, legacySource, market string, providers map[string]QuoteProvider) string {
	sourceID = strings.ToLower(strings.TrimSpace(sourceID))
	if sourceID == "" {
		sourceID = strings.ToLower(strings.TrimSpace(legacySource))
	}
	if sourceID != "" {
		if _, ok := providers[sourceID]; ok && quoteSourceSupportsMarketForSettings(sourceID, market) {
			return sourceID
		}
	}
	switch marketGroupForMarket(market) {
	case "hk":
		if _, ok := providers[DefaultHKQuoteSourceID]; ok {
			return DefaultHKQuoteSourceID
		}
	case "us":
		if _, ok := providers[DefaultUSQuoteSourceID]; ok {
			return DefaultUSQuoteSourceID
		}
	default:
		if _, ok := providers[DefaultCNQuoteSourceID]; ok {
			return DefaultCNQuoteSourceID
		}
	}
	if _, ok := providers[DefaultQuoteSourceID]; ok {
		return DefaultQuoteSourceID
	}
	for id := range providers {
		return id
	}
	return DefaultQuoteSourceID
}

func quoteSourceSupportsMarketForSettings(sourceID, market string) bool {
	switch sourceID {
	case "eastmoney", "yahoo":
		return market != "CN-BJ"
	default:
		return false
	}
}

// normaliseTags 去除空标签并保持标签集合唯一。
func normaliseTags(tags []string) []string {
	seen := make(map[string]struct{}, len(tags))
	result := make([]string, 0, len(tags))
	for _, tag := range tags {
		clean := strings.TrimSpace(tag)
		if clean == "" {
			continue
		}
		if _, exists := seen[clean]; exists {
			continue
		}
		seen[clean] = struct{}{}
		result = append(result, clean)
	}
	return result
}

// logInfo 在日志簿可用时写入 info 级别日志。
func (s *Store) logInfo(scope, message string) {
	if s.logs != nil {
		s.logs.Info("backend", scope, message)
	}
}

// logWarn 在日志簿可用时写入 warn 级别日志。
func (s *Store) logWarn(scope, message string) {
	if s.logs != nil {
		s.logs.Warn("backend", scope, message)
	}
}

// logError 在日志簿可用时写入 error 级别日志。
func (s *Store) logError(scope, message string) {
	if s.logs != nil {
		s.logs.Error("backend", scope, message)
	}
}
