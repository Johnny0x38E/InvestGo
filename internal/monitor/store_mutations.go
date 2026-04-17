package monitor

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"investgo/internal/monitor/validation"
)

// UpsertItem adds or updates a watchlist item, and tries to fetch the latest quote in real-time mode.
func (s *Store) UpsertItem(input WatchlistItem) (StateSnapshot, error) {
	item, err := sanitiseItem(input)
	if err != nil {
		return StateSnapshot{}, err
	}

	// First extract runtime dependencies and old values within read lock to avoid holding write lock during subsequent network requests.
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
		if existing.PinnedAt != nil {
			item.PinnedAt = ptrTime(*existing.PinnedAt)
		} else {
			item.PinnedAt = nil
		}
	}

	if provider != nil {
		// Fetch one quote immediately after saving the item to ensure current price always comes from a unified quote source.
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

// SetItemPinned updates whether the specified item is pinned to the top of watchlist-oriented views.
func (s *Store) SetItemPinned(id string, pinned bool) (StateSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	index := s.findItemIndexLocked(id)
	if index == -1 {
		return StateSnapshot{}, fmt.Errorf("Item not found: %s", id)
	}

	now := time.Now()
	item := s.state.Items[index]
	if pinned {
		item.PinnedAt = &now
	} else {
		item.PinnedAt = nil
	}
	s.state.Items[index] = item
	s.state.UpdatedAt = now

	if err := s.saveLocked(); err != nil {
		s.logError("storage", fmt.Sprintf("save state failed after pin update: %v", err))
		return StateSnapshot{}, err
	}

	action := "unpinned"
	if pinned {
		action = "pinned"
	}
	s.logInfo("watchlist", fmt.Sprintf("%s item %s", action, item.Symbol))

	return s.snapshotLocked(), nil
}

// DeleteItem deletes the specified item and synchronously deletes its associated alert rules.
func (s *Store) DeleteItem(id string) (StateSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	index := s.findItemIndexLocked(id)
	if index == -1 {
		return StateSnapshot{}, fmt.Errorf("Item not found: %s", id)
	}

	itemSymbol := s.state.Items[index].Symbol
	s.state.Items = append(s.state.Items[:index], s.state.Items[index+1:]...)
	// After deleting the item, alerts attached to it must also be cleared to avoid dangling references.
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

// UpsertAlert adds or updates a price alert rule.
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

// DeleteAlert deletes the specified alert rule.
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

// UpdateSettings updates application settings and immediately persists them.
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

// sanitiseItem normalizes item information and performs basic validation.
func sanitiseItem(input WatchlistItem) (WatchlistItem, error) {
	item := input
	item.Name = strings.TrimSpace(item.Name)
	item.Thesis = strings.TrimSpace(item.Thesis)
	item.Tags = validation.NormalizeTags(item.Tags)

	target, err := resolveQuoteTarget(item.Symbol, item.Market, item.Currency)
	if err != nil {
		return WatchlistItem{}, err
	}

	item.Symbol = target.DisplaySymbol
	item.Market = target.Market
	item.Currency = target.Currency
	item.QuoteSource = strings.TrimSpace(item.QuoteSource)

	// If there are DCA (Dollar-Cost Averaging) records, first filter and normalize entries, then automatically calculate accumulated shares and weighted average price.
	// Calculation rules:
	//   1. Prefer manually entered buy price (Price > 0): effectiveCost = Price × Shares
	//   2. When no buy price, deduct fee from total investment: effectiveCost = max(Amount - Fee, 0)
	// Weighted average price = Σ effectiveCost_i / Σ Shares_i
	if len(item.DCAEntries) > 0 {
		validEntries, totalShares, averageCost := validation.NormalizeDCAEntries(item.DCAEntries, newID)
		item.DCAEntries = validEntries
		if len(item.DCAEntries) > 0 {
			item.Quantity = totalShares
			if totalShares > 0 {
				item.CostPrice = averageCost
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

// sanitiseAlert normalizes alert rules and performs basic validation.
func sanitiseAlert(input AlertRule) (AlertRule, error) {
	return validation.SanitizeAlert(input)
}

// logInfo writes info level logs when logbook is available.
func (s *Store) logInfo(scope, message string) {
	if s.logs != nil {
		s.logs.Info("backend", scope, message)
	}
}

// logWarn writes warn level logs when logbook is available.
func (s *Store) logWarn(scope, message string) {
	if s.logs != nil {
		s.logs.Warn("backend", scope, message)
	}
}

// logError writes error level logs when logbook is available.
func (s *Store) logError(scope, message string) {
	if s.logs != nil {
		s.logs.Error("backend", scope, message)
	}
}
