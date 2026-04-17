package monitor

import "context"

// HistoryProvider is the interface for loading historical price series, used by the Store for chart rendering and portfolio trend calculation.
type HistoryProvider interface {
	Fetch(ctx context.Context, item WatchlistItem, interval HistoryInterval) (HistorySeries, error)
	Name() string
}

// HistoryInterval specifies the time range and granularity of a historical price request.
type HistoryInterval string

const (
	HistoryRange1h  HistoryInterval = "1h"
	HistoryRange1d  HistoryInterval = "1d"
	HistoryRange1w  HistoryInterval = "1w"
	HistoryRange1mo HistoryInterval = "1mo"
	HistoryRange1y  HistoryInterval = "1y"
	HistoryRange3y  HistoryInterval = "3y"
	HistoryRangeAll HistoryInterval = "all"
)
