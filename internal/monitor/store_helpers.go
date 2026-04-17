package monitor

import (
	"errors"
	"strings"
)

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

// applyQuoteToItem writes the latest quote data onto the given item in place.
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

// inheritLiveFields copies live market data from an existing item so that a user edit does not erase the last known quote.
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

// countLiveQuotes returns the number of items that have received at least one live price update.
func countLiveQuotes(items []WatchlistItem) int {
	total := 0
	for _, item := range items {
		if item.QuoteUpdatedAt != nil && !item.QuoteUpdatedAt.IsZero() {
			total++
		}
	}
	return total
}
