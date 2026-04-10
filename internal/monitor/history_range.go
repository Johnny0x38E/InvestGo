package monitor

import "context"

// HistoryProvider abstracts historical chart backends away from Store.
type HistoryProvider interface {
	Fetch(ctx context.Context, item WatchlistItem, interval HistoryInterval) (HistorySeries, error)
	Name() string
}

// HistoryInterval represents the chart range selected by the UI.
type HistoryInterval string

const (
	HistoryRange5m  HistoryInterval = "5m"
	HistoryRange15m HistoryInterval = "15m"
	HistoryRange30m HistoryInterval = "30m"
	HistoryRange1h  HistoryInterval = "1h"
	HistoryRange1d  HistoryInterval = "1d"
	HistoryRange1w  HistoryInterval = "1w"
	HistoryRange1mo HistoryInterval = "1mo"
	HistoryRange1y  HistoryInterval = "1y"
	HistoryRange3y  HistoryInterval = "3y"
	HistoryRangeAll HistoryInterval = "all"
)
