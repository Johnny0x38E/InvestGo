package monitor

import "time"

// AlertCondition defines price alert conditions
type AlertCondition string

const (
	AlertAbove AlertCondition = "above"
	AlertBelow AlertCondition = "below"
)

// DCAEntry represents a Dollar-Cost Averaging entry
type DCAEntry struct {
	ID     string    `json:"id"`
	Date   time.Time `json:"date"`
	Amount float64   `json:"amount"`          // Investment amount for this entry
	Shares float64   `json:"shares"`          // Shares purchased this time
	Price  float64   `json:"price,omitempty"` // Manually entered buy price (actual transaction price including fee), 0 means not filled
	Fee    float64   `json:"fee,omitempty"`   // Fee for this DCA entry, 0 means not filled
	Note   string    `json:"note,omitempty"`
}

// WatchlistItem represents an investment item on the watchlist
type WatchlistItem struct {
	ID             string     `json:"id"`                       // Unique identifier, formatted as "market-code", e.g., "CN-A-000001", "HK-MAIN-00700", "US-STOCK-AAPL"
	Symbol         string     `json:"symbol"`                   // Standardized instrument code, formatted as "market-code", e.g., "CN-A-000001", "HK-MAIN-00700", "US-STOCK-AAPL"
	Name           string     `json:"name"`                     // Instrument name, e.g., "Apple Inc.", "Tesla, Inc.", "贵州茅台"
	Market         string     `json:"market"`                   // Market identifier, e.g., "CN-A" (A-share), "HK-MAIN" (Hong Kong Main Board), "US-STOCK" (US stocks)
	Currency       string     `json:"currency"`                 // Currency code, e.g., "CNY", "HKD", "USD"
	Quantity       float64    `json:"quantity"`                 // Position quantity
	CostPrice      float64    `json:"costPrice"`                // Average cost price of position (including fee), i.e., total cost amount divided by position quantity
	CurrentPrice   float64    `json:"currentPrice"`             // Current price
	PreviousClose  float64    `json:"previousClose"`            // Previous close price
	OpenPrice      float64    `json:"openPrice"`                // Today's opening price
	DayHigh        float64    `json:"dayHigh"`                  // Today's high price
	DayLow         float64    `json:"dayLow"`                   // Today's low price
	Change         float64    `json:"change"`                   // Today's change amount
	ChangePercent  float64    `json:"changePercent"`            // Today's change percentage
	QuoteSource    string     `json:"quoteSource"`              // Quote data source, e.g. "eastmoney", "yahoo"
	QuoteUpdatedAt *time.Time `json:"quoteUpdatedAt,omitempty"` // Last quote update time
	PinnedAt       *time.Time `json:"pinnedAt,omitempty"`       // Time when the item was pinned to the top; nil means not pinned
	Thesis         string     `json:"thesis"`                   // Notes
	Tags           []string   `json:"tags"`                     // Tag list
	DCAEntries     []DCAEntry `json:"dcaEntries,omitempty"`     // DCA (Dollar-Cost Averaging) entry list
	UpdatedAt      time.Time  `json:"updatedAt"`
}

// CostBasis returns position cost amount, i.e., buy quantity multiplied by buy price.
func (w WatchlistItem) CostBasis() float64 {
	return w.Quantity * w.CostPrice
}

// MarketValue returns current position value, i.e., position quantity multiplied by current price.
func (w WatchlistItem) MarketValue() float64 {
	return w.Quantity * w.CurrentPrice
}

// UnrealisedPnL returns position position PnL (Profit and Loss) amount, i.e., current value minus cost amount.
func (w WatchlistItem) UnrealisedPnL() float64 {
	return w.MarketValue() - w.CostBasis()
}

// UnrealisedPnLPct returns position position PnL percentage, i.e., position PnL amount divided by cost amount multiplied by 100.
func (w WatchlistItem) UnrealisedPnLPct() float64 {
	if w.CostBasis() == 0 {
		return 0
	}

	return w.UnrealisedPnL() / w.CostBasis() * 100
}

// AlertRule represents a price alert rule
type AlertRule struct {
	ID              string         `json:"id"`
	ItemID          string         `json:"itemId"`
	Name            string         `json:"name"`
	Condition       AlertCondition `json:"condition"` // "above" triggers when price exceeds threshold, "below" triggers when price falls below threshold
	Threshold       float64        `json:"threshold"` // Price threshold
	Enabled         bool           `json:"enabled"`
	Triggered       bool           `json:"triggered"`                 // Whether it has been triggered before, automatically set to true after triggering
	LastTriggeredAt *time.Time     `json:"lastTriggeredAt,omitempty"` // Last triggered time
	UpdatedAt       time.Time      `json:"updatedAt"`
}

// AppSettings represents application settings
type AppSettings struct {
	RefreshIntervalSeconds int    `json:"refreshIntervalSeconds"`
	QuoteSource            string `json:"quoteSource"` // "auto", "cn", "hk", "us", default "auto"
	CNQuoteSource          string `json:"cnQuoteSource"`
	HKQuoteSource          string `json:"hkQuoteSource"`
	USQuoteSource          string `json:"usQuoteSource"`
	HotUSSource            string `json:"hotUSSource"` // "eastmoney" or "yahoo", default "eastmoney"
	ThemeMode              string `json:"themeMode"`
	ColorTheme             string `json:"colorTheme"`
	FontPreset             string `json:"fontPreset"`
	AmountDisplay          string `json:"amountDisplay"`
	CurrencyDisplay        string `json:"currencyDisplay"`
	PriceColorScheme       string `json:"priceColorScheme"`
	Locale                 string `json:"locale"`
	DeveloperMode          bool   `json:"developerMode"`
	DashboardCurrency      string `json:"dashboardCurrency"`
	UseNativeTitleBar      bool   `json:"useNativeTitleBar"`
}

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

// DashboardSummary represents aggregated data to be displayed on the dashboard
type DashboardSummary struct {
	TotalCost       float64 `json:"totalCost"`       // Portfolio cost
	TotalValue      float64 `json:"totalValue"`      // Total assets
	TotalPnL        float64 `json:"totalPnL"`        // Total PnL (Profit and Loss) amount
	TotalPnLPct     float64 `json:"totalPnLPct"`     // Total PnL percentage
	ItemCount       int     `json:"itemCount"`       // Position quantity
	TriggeredAlerts int     `json:"triggeredAlerts"` // Number of triggered alerts
	WinCount        int     `json:"winCount"`        // Profit count in PnL statistics
	LossCount       int     `json:"lossCount"`       // Loss count in PnL statistics
	DisplayCurrency string  `json:"displayCurrency"` // Display currency for aggregated data
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

// HotCategory represents hot list categories
type HotCategory string

const (
	HotCategoryCNA      HotCategory = "cn-a"      // Shanghai/Shenzhen A-shares (Main Board + GEM + STAR Market)
	HotCategoryCNETF    HotCategory = "cn-etf"    // Shanghai/Shenzhen ETFs
	HotCategoryHK       HotCategory = "hk"        // Hong Kong stocks
	HotCategoryHKETF    HotCategory = "hk-etf"    // Hong Kong stocks ETFs
	HotCategoryUSSP500  HotCategory = "us-sp500"  // S&P 500
	HotCategoryUSNasdaq HotCategory = "us-nasdaq" // Nasdaq 100
	HotCategoryUSDow    HotCategory = "us-dow"    // Dow Jones 30
	HotCategoryUSETF    HotCategory = "us-etf"    // US ETFs
)

// HotSort represents hot list table sorting options
type HotSort string

const (
	HotSortVolume    HotSort = "volume"     // Volume leaderboard
	HotSortGainers   HotSort = "gainers"    // Gainers leaderboard
	HotSortLosers    HotSort = "losers"     // Losers leaderboard
	HotSortMarketCap HotSort = "market-cap" // Market cap leaderboard
	HotSortPrice     HotSort = "price"      // Price leaderboard
)

// HotItem represents detailed information for each item in the hot list
type HotItem struct {
	Symbol        string    `json:"symbol"`
	Name          string    `json:"name"`
	Market        string    `json:"market"`
	Currency      string    `json:"currency"`
	CurrentPrice  float64   `json:"currentPrice"`
	Change        float64   `json:"change"`        // Change amount
	ChangePercent float64   `json:"changePercent"` // Change percentage
	Volume        float64   `json:"volume"`        // Trading volume
	MarketCap     float64   `json:"marketCap"`     // Market capitalization
	QuoteSource   string    `json:"quoteSource"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

// HotListResponse represents paginated results returned by the hot list API
type HotListResponse struct {
	Category    HotCategory `json:"category"` // Hot list category, e.g., "cn-a", "hk", "us-sp500"
	Sort        HotSort     `json:"sort"`     // Sorting method, e.g., "volume", "gainers", "losers"
	Page        int         `json:"page"`
	PageSize    int         `json:"pageSize"`
	Total       int         `json:"total"`
	HasMore     bool        `json:"hasMore"`
	Items       []HotItem   `json:"items"`
	GeneratedAt time.Time   `json:"generatedAt"`
}
