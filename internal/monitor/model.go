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
type WatchlistItem = domain.WatchlistItem
type AlertRule = domain.AlertRule
type AppSettings = domain.AppSettings
type DashboardSummary = domain.DashboardSummary
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

// HistoryPoint represents a single historical K-line data point
type HistoryPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Open      float64   `json:"open"`
	High      float64   `json:"high"`
	Low       float64   `json:"low"`
	Close     float64   `json:"close"`
	Volume    float64   `json:"volume"`
}

// HistorySeries represents the complete historical trend for a given instrument in a specified interval
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
	GeneratedAt   time.Time       `json:"generatedAt"`   // Data generation time
}

// RuntimeStatus stores application runtime status information
type RuntimeStatus struct {
	LastQuoteAttemptAt *time.Time `json:"lastQuoteAttemptAt,omitempty"` // Last quote request time
	LastQuoteRefreshAt *time.Time `json:"lastQuoteRefreshAt,omitempty"` // Last quote refresh time
	LastQuoteError     string     `json:"lastQuoteError,omitempty"`     // Last quote request error message
	QuoteSource        string     `json:"quoteSource"`                  // Currently used data source, "auto" means automatic selection
	LivePriceCount     int        `json:"livePriceCount"`               // Number of items in current holdings with prices
	AppVersion         string     `json:"appVersion"`                   // Current application version
	LastFxError        string     `json:"lastFxError,omitempty"`        // Last FX rate error message
	LastFxRefreshAt    *time.Time `json:"lastFxRefreshAt,omitempty"`    // Last FX rate refresh time
}

// persistedState represents application state that needs to be persisted
type persistedState struct {
	Items     []WatchlistItem `json:"items"`     // Position list
	Alerts    []AlertRule     `json:"alerts"`    // Price alert rule list
	Settings  AppSettings     `json:"settings"`  // Settings
	UpdatedAt time.Time       `json:"updatedAt"` // Last updated timestamp
}

// StateSnapshot represents a complete application state snapshot
type StateSnapshot struct {
	Dashboard    DashboardSummary    `json:"dashboard"`    // Dashboard aggregated data
	Items        []WatchlistItem     `json:"items"`        // Position list
	Alerts       []AlertRule         `json:"alerts"`       // Price alert rule list
	Settings     AppSettings         `json:"settings"`     // Settings
	Runtime      RuntimeStatus       `json:"runtime"`      // Runtime status
	QuoteSources []QuoteSourceOption `json:"quoteSources"` // Available data source list
	StoragePath  string              `json:"storagePath"`  // Persistent storage path
	GeneratedAt  time.Time           `json:"generatedAt"`  // Snapshot generation time
}
