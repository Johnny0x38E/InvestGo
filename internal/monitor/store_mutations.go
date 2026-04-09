package monitor

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// UpsertItem 负责标的增改，并在 live 模式下尽量补齐一跳最新行情。
func (s *Store) UpsertItem(input WatchlistItem) (StateSnapshot, error) {
	s.mu.RLock()
	priceMode := s.state.Settings.PriceMode
	provider := s.activeQuoteProviderLocked()
	var existing *WatchlistItem
	if input.ID != "" {
		if index := s.findItemIndexLocked(input.ID); index >= 0 {
			copy := s.state.Items[index]
			existing = &copy
		}
	}
	s.mu.RUnlock()

	item, err := sanitiseItem(input)
	if err != nil {
		return StateSnapshot{}, err
	}

	if existing != nil {
		item = inheritLiveFields(item, *existing)
	}

	if strings.EqualFold(priceMode, "live") && provider != nil {
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
			return StateSnapshot{}, fmt.Errorf("标的不存在: %s", item.ID)
		}
		item.UpdatedAt = time.Now()
		s.state.Items[index] = item
		s.logInfo("watchlist", fmt.Sprintf("updated item %s", item.Symbol))
	}

	s.runtime.QuoteSource = s.quoteProviderNameLocked()
	s.state.UpdatedAt = time.Now()
	s.evaluateAlertsLocked()
	if err := s.saveLocked(); err != nil {
		s.logError("storage", fmt.Sprintf("save state failed after item update: %v", err))
		return StateSnapshot{}, err
	}

	return s.snapshotLocked(), nil
}

func (s *Store) DeleteItem(id string) (StateSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	index := s.findItemIndexLocked(id)
	if index == -1 {
		return StateSnapshot{}, fmt.Errorf("标的不存在: %s", id)
	}

	itemSymbol := s.state.Items[index].Symbol
	s.state.Items = append(s.state.Items[:index], s.state.Items[index+1:]...)
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

func (s *Store) UpsertAlert(input AlertRule) (StateSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	alert, err := sanitiseAlert(input)
	if err != nil {
		return StateSnapshot{}, err
	}
	if s.findItemIndexLocked(alert.ItemID) == -1 {
		return StateSnapshot{}, fmt.Errorf("提醒关联的标的不存在: %s", alert.ItemID)
	}

	if alert.ID == "" {
		alert.ID = newID("alert")
		alert.UpdatedAt = time.Now()
		s.state.Alerts = append(s.state.Alerts, alert)
		s.logInfo("alerts", fmt.Sprintf("created alert %s", alert.Name))
	} else {
		index := s.findAlertIndexLocked(alert.ID)
		if index == -1 {
			return StateSnapshot{}, fmt.Errorf("提醒不存在: %s", alert.ID)
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

func (s *Store) DeleteAlert(id string) (StateSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	index := s.findAlertIndexLocked(id)
	if index == -1 {
		return StateSnapshot{}, fmt.Errorf("提醒不存在: %s", id)
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
			"updated settings: quoteSource=%s refresh=%ds developerMode=%t",
			settings.QuoteSource,
			settings.RefreshIntervalSeconds,
			settings.DeveloperMode,
		),
	)

	return s.snapshotLocked(), nil
}

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

	if item.Quantity < 0 {
		return WatchlistItem{}, errors.New("持仓数量不能小于 0")
	}
	if item.CostPrice < 0 || item.CurrentPrice < 0 {
		return WatchlistItem{}, errors.New("价格不能小于 0")
	}

	return item, nil
}

func sanitiseAlert(input AlertRule) (AlertRule, error) {
	alert := input
	alert.Name = strings.TrimSpace(alert.Name)
	alert.ItemID = strings.TrimSpace(alert.ItemID)

	if alert.Name == "" {
		return AlertRule{}, errors.New("提醒名称不能为空")
	}
	if alert.ItemID == "" {
		return AlertRule{}, errors.New("提醒必须关联一个标的")
	}
	if alert.Condition != AlertAbove && alert.Condition != AlertBelow {
		return AlertRule{}, errors.New("提醒条件无效")
	}
	if alert.Threshold <= 0 {
		return AlertRule{}, errors.New("提醒阈值必须大于 0")
	}

	return alert, nil
}

// sanitiseSettings 会把用户输入与当前配置合并，并做统一的合法性校验。
func sanitiseSettings(input AppSettings, current AppSettings, quoteProviders map[string]QuoteProvider) (AppSettings, error) {
	settings := current
	if strings.TrimSpace(input.PriceMode) != "" {
		settings.PriceMode = strings.ToLower(strings.TrimSpace(input.PriceMode))
	}
	if input.RefreshIntervalSeconds > 0 {
		settings.RefreshIntervalSeconds = input.RefreshIntervalSeconds
	}
	if strings.TrimSpace(input.QuoteSource) != "" {
		settings.QuoteSource = strings.ToLower(strings.TrimSpace(input.QuoteSource))
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
	settings.DeveloperMode = input.DeveloperMode

	switch settings.PriceMode {
	case "", "live":
		settings.PriceMode = "live"
	case "manual":
	default:
		return AppSettings{}, errors.New("价格模式仅支持 live 或 manual")
	}

	if settings.RefreshIntervalSeconds < 10 || settings.RefreshIntervalSeconds > 300 {
		return AppSettings{}, errors.New("刷新间隔需要在 10 到 300 秒之间")
	}
	if settings.QuoteSource == "" {
		settings.QuoteSource = defaultQuoteSourceID
	}
	if len(quoteProviders) > 0 {
		if _, ok := quoteProviders[settings.QuoteSource]; !ok {
			return AppSettings{}, errors.New("行情来源无效")
		}
	}
	switch settings.FontPreset {
	case "", "system":
		settings.FontPreset = "system"
	case "reading", "compact":
	default:
		return AppSettings{}, errors.New("字体预设仅支持 system / reading / compact")
	}
	switch settings.AmountDisplay {
	case "", "full":
		settings.AmountDisplay = "full"
	case "compact":
	default:
		return AppSettings{}, errors.New("金额展示仅支持 full / compact")
	}
	switch settings.CurrencyDisplay {
	case "", "symbol":
		settings.CurrencyDisplay = "symbol"
	case "code":
	default:
		return AppSettings{}, errors.New("币种展示仅支持 symbol / code")
	}
	switch settings.PriceColorScheme {
	case "", "cn":
		settings.PriceColorScheme = "cn"
	case "intl":
	default:
		return AppSettings{}, errors.New("涨跌配色仅支持 cn / intl")
	}
	switch settings.Locale {
	case "", "system":
		settings.Locale = "system"
	case "zh-CN", "en-US":
	default:
		return AppSettings{}, errors.New("语言区域仅支持 system / zh-CN / en-US")
	}

	return settings, nil
}

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

func (s *Store) logInfo(scope, message string) {
	if s.logs != nil {
		s.logs.Info("backend", scope, message)
	}
}

func (s *Store) logWarn(scope, message string) {
	if s.logs != nil {
		s.logs.Warn("backend", scope, message)
	}
}

func (s *Store) logError(scope, message string) {
	if s.logs != nil {
		s.logs.Error("backend", scope, message)
	}
}
