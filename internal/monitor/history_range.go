package monitor

import "context"

// HistoryProvider 定义了获取历史数据的接口，供监控模块调用以展示历史走势图表。
type HistoryProvider interface {
	Fetch(ctx context.Context, item WatchlistItem, interval HistoryInterval) (HistorySeries, error)
	Name() string
}

// HistoryInterval 定义了历史数据的时间范围和粒度，供 Fetch 方法使用。
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
