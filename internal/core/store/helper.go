package store

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"investgo/internal/core"
)

// applyQuoteToItem writes the latest quote data onto the given item in place.
func applyQuoteToItem(item *core.WatchlistItem, quote core.Quote) {
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
func inheritLiveFields(item core.WatchlistItem, existing core.WatchlistItem) core.WatchlistItem {
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
func countLiveQuotes(items []core.WatchlistItem) int {
	total := 0
	for _, item := range items {
		if item.QuoteUpdatedAt != nil && !item.QuoteUpdatedAt.IsZero() {
			total++
		}
	}
	return total
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
