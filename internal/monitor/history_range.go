package monitor

import "context"

// HistoryProvider defines the interface for fetching historical data, called by the monitoring module to display historical trend charts.
type HistoryProvider interface {
	Fetch(ctx context.Context, item WatchlistItem, interval HistoryInterval) (HistorySeries, error)
	Name() string
}

// HistoryInterval defines the time range and granularity of historical data for use by the Fetch method.
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
