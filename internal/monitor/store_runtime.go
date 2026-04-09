package monitor

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Refresh 只刷新实时行情和提醒状态，不触碰历史走势缓存。
// 这样前端可以按需拉取走势图，而不是每次刷新都把历史数据重新打包进基线快照。
func (s *Store) Refresh(ctx context.Context) (StateSnapshot, error) {
	s.mu.RLock()
	priceMode := s.state.Settings.PriceMode
	items := append([]WatchlistItem(nil), s.state.Items...)
	provider := s.activeQuoteProviderLocked()
	s.mu.RUnlock()

	attemptedAt := time.Now()
	quotes := map[string]Quote{}
	fetchErr := error(nil)

	if strings.EqualFold(priceMode, "live") && provider != nil && len(items) > 0 {
		quotes, fetchErr = provider.Fetch(ctx, items)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.runtime.LastQuoteAttemptAt = ptrTime(attemptedAt)
	s.runtime.LastQuoteError = ""
	s.runtime.QuoteSource = s.quoteProviderNameLocked()

	if len(quotes) > 0 {
		for idx := range s.state.Items {
			target, err := ResolveQuoteTarget(s.state.Items[idx])
			if err != nil {
				continue
			}
			quote, ok := quotes[target.Key]
			if !ok {
				continue
			}
			applyQuoteToItem(&s.state.Items[idx], quote)
		}
		s.runtime.LastQuoteRefreshAt = ptrTime(time.Now())
	}

	if fetchErr != nil {
		s.runtime.LastQuoteError = fetchErr.Error()
		s.logWarn("quotes", fmt.Sprintf("quote refresh failed: %v", fetchErr))
	}

	s.evaluateAlertsLocked()
	s.state.UpdatedAt = time.Now()
	if err := s.saveLocked(); err != nil {
		s.logError("storage", fmt.Sprintf("save state failed after quote refresh: %v", err))
		return StateSnapshot{}, err
	}

	return s.snapshotLocked(), nil
}

// ItemHistory 把历史走势查询委托给独立 provider，避免 Store 直接耦合外部数据源细节。
func (s *Store) ItemHistory(ctx context.Context, itemID string, interval HistoryInterval) (HistorySeries, error) {
	if s.historyProvider == nil {
		return HistorySeries{}, errors.New("历史行情 provider 未配置")
	}

	s.mu.RLock()
	index := s.findItemIndexLocked(itemID)
	if index == -1 {
		s.mu.RUnlock()
		return HistorySeries{}, fmt.Errorf("标的不存在: %s", itemID)
	}
	item := s.state.Items[index]
	s.mu.RUnlock()

	return s.historyProvider.Fetch(ctx, item, interval)
}

func applyQuoteToItem(item *WatchlistItem, quote Quote) {
	if strings.TrimSpace(quote.Name) != "" {
		item.Name = quote.Name
	}
	item.CurrentPrice = quote.CurrentPrice
	item.PreviousClose = quote.PreviousClose
	item.OpenPrice = quote.OpenPrice
	item.DayHigh = quote.DayHigh
	item.DayLow = quote.DayLow
	item.Change = quote.Change
	item.ChangePercent = quote.ChangePercent
	item.QuoteSource = quote.Source
	item.QuoteUpdatedAt = ptrTime(nonZeroTime(quote.UpdatedAt))
}

// inheritLiveFields 用于保留编辑表单里看不见的实时字段，避免用户改备注时把盘口信息抹掉。
func inheritLiveFields(item WatchlistItem, existing WatchlistItem) WatchlistItem {
	item.PreviousClose = existing.PreviousClose
	item.OpenPrice = existing.OpenPrice
	item.DayHigh = existing.DayHigh
	item.DayLow = existing.DayLow
	item.Change = existing.Change
	item.ChangePercent = existing.ChangePercent
	item.QuoteSource = existing.QuoteSource
	item.QuoteUpdatedAt = existing.QuoteUpdatedAt

	if item.CurrentPrice == 0 && existing.CurrentPrice > 0 {
		item.CurrentPrice = existing.CurrentPrice
	}

	return item
}

func countLiveQuotes(items []WatchlistItem) int {
	total := 0
	for _, item := range items {
		if item.QuoteUpdatedAt != nil && !item.QuoteUpdatedAt.IsZero() {
			total++
		}
	}
	return total
}
