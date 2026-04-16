package monitor

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Refresh 刷新实时行情与提醒状态，但不触碰历史走势缓存。
// 这样前端可以按需拉取走势图，而不是每次刷新都把历史数据重新打包进基线快照。
func (s *Store) Refresh(ctx context.Context) (StateSnapshot, error) {
	// 先复制当前标的切片，避免网络请求阶段长期持有读锁。
	s.mu.RLock()
	items := append([]WatchlistItem(nil), s.state.Items...)
	s.mu.RUnlock()

	attemptedAt := time.Now()
	quotes := map[string]Quote{}
	var problems []string

	// 顺带刷新汇率缓存（若已过期），与行情刷新并行，不参与 Store 锁生命周期。
	var fxFetched bool
	if s.fxRates.IsStale() {
		s.fxRates.Fetch(ctx)
		fxFetched = true
	}

	if len(items) > 0 {
		grouped := make(map[string][]WatchlistItem)
		for _, item := range items {
			s.mu.RLock()
			sourceID := s.activeQuoteSourceIDLocked(item.Market)
			provider := s.activeQuoteProviderLocked(item.Market)
			s.mu.RUnlock()
			if provider == nil || sourceID == "" {
				continue
			}
			grouped[sourceID] = append(grouped[sourceID], item)
		}

		for sourceID, batch := range grouped {
			provider := s.quoteProviders[sourceID]
			batchQuotes, err := provider.Fetch(ctx, batch)
			if err != nil {
				problems = append(problems, fmt.Sprintf("%s: %v", provider.Name(), err))
			}
			for key, quote := range batchQuotes {
				quotes[key] = quote
			}
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.runtime.LastQuoteAttemptAt = ptrTime(attemptedAt)
	s.runtime.LastQuoteError = ""
	s.runtime.QuoteSource = s.quoteProviderSummaryLocked()

	if len(quotes) > 0 {
		// 以规范化后的目标键匹配返回结果，避免用户输入格式差异影响回填。
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

	if fetchErr := joinProblems(problems); fetchErr != nil {
		s.runtime.LastQuoteError = fetchErr.Error()
		s.logWarn("quotes", fmt.Sprintf("quote refresh failed: %v", fetchErr))
	}

	// 更新汇率运行时状态。
	if fxFetched {
		if fxErr := s.fxRates.LastError(); fxErr != "" {
			s.runtime.LastFxError = fxErr
			s.logWarn("fx-rates", fxErr)
		} else {
			s.runtime.LastFxError = ""
			s.runtime.LastFxRefreshAt = ptrTime(s.fxRates.ValidAt())
			s.logInfo("fx-rates", fmt.Sprintf("FX rates refreshed for %d currencies", s.fxRates.CurrencyCount()))
		}
	}

	s.evaluateAlertsLocked()
	s.state.UpdatedAt = time.Now()
	if err := s.saveLocked(); err != nil {
		s.logError("storage", fmt.Sprintf("save state failed after quote refresh: %v", err))
		return StateSnapshot{}, err
	}

	return s.snapshotLocked(), nil
}

// ItemHistory 查询指定标的的历史走势，并委托给历史 provider 实现。
func (s *Store) ItemHistory(ctx context.Context, itemID string, interval HistoryInterval) (HistorySeries, error) {
	s.mu.RLock()
	index := s.findItemIndexLocked(itemID)
	if index == -1 {
		s.mu.RUnlock()
		return HistorySeries{}, fmt.Errorf("Item not found: %s", itemID)
	}
	item := s.state.Items[index]
	providers := s.historyProviderCandidatesLocked(item.Market)
	s.mu.RUnlock()

	if len(providers) == 0 {
		return HistorySeries{}, errors.New("History provider is not configured")
	}

	var problems []string
	for _, provider := range providers {
		series, err := provider.Fetch(ctx, item, interval)
		if err == nil {
			return series, nil
		}
		problems = append(problems, fmt.Sprintf("%s: %v", provider.Name(), err))
	}
	return HistorySeries{}, joinProblems(problems)
}

func joinProblems(problems []string) error {
	if len(problems) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(problems))
	unique := make([]string, 0, len(problems))
	for _, problem := range problems {
		problem = strings.TrimSpace(problem)
		if problem == "" {
			continue
		}
		if _, ok := seen[problem]; ok {
			continue
		}
		seen[problem] = struct{}{}
		unique = append(unique, problem)
	}
	if len(unique) == 0 {
		return nil
	}
	return errors.New(strings.Join(unique, "; "))
}

// applyQuoteToItem 把最新行情字段回填到标的对象上。
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

// inheritLiveFields 继承旧条目里的实时字段，避免表单更新时覆盖掉盘口信息。
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

// countLiveQuotes 统计当前有有效实时更新时间的标的数量。
func countLiveQuotes(items []WatchlistItem) int {
	total := 0
	for _, item := range items {
		if item.QuoteUpdatedAt != nil && !item.QuoteUpdatedAt.IsZero() {
			total++
		}
	}
	return total
}
