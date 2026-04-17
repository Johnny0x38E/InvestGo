package monitor

import (
	"time"

	"investgo/internal/monitor/domain"
)

type AlertCondition = domain.AlertCondition

const (
	AlertAbove = domain.AlertAbove
	AlertBelow = domain.AlertBelow
)

type DCAEntry = domain.DCAEntry
type DCASummary = domain.DCASummary
type PositionSummary = domain.PositionSummary
type WatchlistItem = domain.WatchlistItem
type AlertRule = domain.AlertRule
type AppSettings = domain.AppSettings
type DashboardSummary = domain.DashboardSummary
type OverviewHoldingSlice = domain.OverviewHoldingSlice
type OverviewTrendSeries = domain.OverviewTrendSeries
type OverviewTrend = domain.OverviewTrend
type OverviewAnalytics = domain.OverviewAnalytics
type HotCategory = domain.HotCategory
type HotSort = domain.HotSort
type HotItem = domain.HotItem
type HotListResponse = domain.HotListResponse

const (
	HotCategoryCNA      = domain.HotCategoryCNA
	HotCategoryCNETF    = domain.HotCategoryCNETF
	HotCategoryHK       = domain.HotCategoryHK
	HotCategoryHKETF    = domain.HotCategoryHKETF
	HotCategoryUSSP500  = domain.HotCategoryUSSP500
	HotCategoryUSNasdaq = domain.HotCategoryUSNasdaq
	HotCategoryUSDow    = domain.HotCategoryUSDow
	HotCategoryUSETF    = domain.HotCategoryUSETF
)

const (
	HotSortVolume    = domain.HotSortVolume
	HotSortGainers   = domain.HotSortGainers
	HotSortLosers    = domain.HotSortLosers
	HotSortMarketCap = domain.HotSortMarketCap
	HotSortPrice     = domain.HotSortPrice
)

// HistoryPoint represents a single OHLCV (candlestick) data point in a price history series.
type HistoryPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Open      float64   `json:"open"`
	High      float64   `json:"high"`
	Low       float64   `json:"low"`
	Close     float64   `json:"close"`
	Volume    float64   `json:"volume"`
}

// HistorySeries holds a complete price history series for one instrument over a specified time range.
type HistorySeries struct {
	Symbol        string          `json:"symbol"`
	Name          string          `json:"name"`
	Market        string          `json:"market"`
	Currency      string          `json:"currency"`
	Interval      HistoryInterval `json:"interval"`      // Time interval of historical data, e.g., "1d", "1mo"
	Source        string          `json:"source"`        // Data source, e.g., "eastmoney", "yahoo"
	StartPrice    float64         `json:"startPrice"`    // Starting price of interval, i.e., opening price of the first data point
	EndPrice      float64         `json:"endPrice"`      // Ending price of interval, i.e., closing price of the last data point
	High          float64         `json:"high"`          // Highest price within interval
	Low           float64         `json:"low"`           // Lowest price within interval
	Change        float64         `json:"change"`        // Change amount within interval, i.e., ending price minus starting price
	ChangePercent float64         `json:"changePercent"` // Change percentage within interval, i.e., change amount divided by starting price multiplied by 100
	Points        []HistoryPoint  `json:"points"`        // Data point list, ordered by time
	Snapshot      *MarketSnapshot `json:"snapshot,omitempty"`
	GeneratedAt   time.Time       `json:"generatedAt"` // Data generation time
}

// MarketSnapshot represents backend-derived market and position metrics for the selected item and interval.
type MarketSnapshot struct {
	LivePrice          float64 `json:"livePrice"`
	EffectiveChange    float64 `json:"effectiveChange"`
	EffectiveChangePct float64 `json:"effectiveChangePct"`
	PreviousClose      float64 `json:"previousClose"`
	OpenPrice          float64 `json:"openPrice"`
	RangeHigh          float64 `json:"rangeHigh"`
	RangeLow           float64 `json:"rangeLow"`
	AmplitudePct       float64 `json:"amplitudePct"`
	PositionValue      float64 `json:"positionValue"`
	PositionBaseline   float64 `json:"positionBaseline"`
	PositionPnL        float64 `json:"positionPnL"`
	PositionPnLPct     float64 `json:"positionPnLPct"`
}

// RuntimeStatus holds live operational metrics exposed to the frontend status panel.
type RuntimeStatus struct {
	LastQuoteAttemptAt *time.Time `json:"lastQuoteAttemptAt,omitempty"` // Last quote request time
	LastQuoteRefreshAt *time.Time `json:"lastQuoteRefreshAt,omitempty"` // Last quote refresh time
	LastQuoteError     string     `json:"lastQuoteError,omitempty"`     // Last quote request error message
	QuoteSource        string     `json:"quoteSource"`                  // Currently used data source, "auto" means automatic selection
	LivePriceCount     int        `json:"livePriceCount"`               // Number of tracked items with an active price quote
	AppVersion         string     `json:"appVersion"`                   // Current application version
	LastFxError        string     `json:"lastFxError,omitempty"`        // Last FX rate error message
	LastFxRefreshAt    *time.Time `json:"lastFxRefreshAt,omitempty"`    // Last FX rate refresh time
}

// persistedState represents application state that needs to be persisted
type persistedState struct {
	Items     []WatchlistItem `json:"items"`     // Tracked item list (watch-only entries and held positions)
	Alerts    []AlertRule     `json:"alerts"`    // Price alert rule list
	Settings  AppSettings     `json:"settings"`  // Settings
	UpdatedAt time.Time       `json:"updatedAt"` // Last updated timestamp
}

// StateSnapshot represents a complete application state snapshot
type StateSnapshot struct {
	Dashboard    DashboardSummary    `json:"dashboard"`    // Dashboard aggregated data
	Items        []WatchlistItem     `json:"items"`        // Tracked item list (watch-only entries and held positions)
	Alerts       []AlertRule         `json:"alerts"`       // Price alert rule list
	Settings     AppSettings         `json:"settings"`     // Settings
	Runtime      RuntimeStatus       `json:"runtime"`      // Runtime status
	QuoteSources []QuoteSourceOption `json:"quoteSources"` // Available data source list
	StoragePath  string              `json:"storagePath"`  // Persistent storage path
	GeneratedAt  time.Time           `json:"generatedAt"`  // Snapshot generation time
}
