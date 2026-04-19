package domain

import "time"

// AlertCondition defines price alert conditions.
type AlertCondition string

const (
	AlertAbove AlertCondition = "above"
	AlertBelow AlertCondition = "below"
)

// DCAEntry represents a Dollar-Cost Averaging entry.
type DCAEntry struct {
	ID             string    `json:"id"`
	Date           time.Time `json:"date"`
	Amount         float64   `json:"amount"`
	Shares         float64   `json:"shares"`
	Price          float64   `json:"price,omitempty"`
	Fee            float64   `json:"fee,omitempty"`
	Note           string    `json:"note,omitempty"`
	EffectivePrice float64   `json:"effectivePrice,omitempty"`
}

// DCASummary represents aggregated metrics derived from saved DCA records.
type DCASummary struct {
	Count           int     `json:"count"`
	TotalAmount     float64 `json:"totalAmount"`
	TotalShares     float64 `json:"totalShares"`
	TotalFees       float64 `json:"totalFees"`
	AverageCost     float64 `json:"averageCost"`
	CurrentValue    float64 `json:"currentValue"`
	PnL             float64 `json:"pnl"`
	PnLPct          float64 `json:"pnlPct"`
	HasCurrentPrice bool    `json:"hasCurrentPrice"`
}

// PositionSummary represents backend-derived position metrics for a tracked item that holds a position.
type PositionSummary struct {
	CostBasis        float64 `json:"costBasis"`
	MarketValue      float64 `json:"marketValue"`
	UnrealisedPnL    float64 `json:"unrealisedPnL"`
	UnrealisedPnLPct float64 `json:"unrealisedPnLPct"`
	HasPosition      bool    `json:"hasPosition"`
}

// WatchlistItem represents any tracked item — either a watch-only entry (Quantity == 0, no DCAEntries)
// or an active holding (Quantity > 0 or DCAEntries non-empty). Both watchlist and holdings views
// are backed by this single type; the frontend distinguishes them by the presence of position data.
type WatchlistItem struct {
	ID             string           `json:"id"`
	Symbol         string           `json:"symbol"`
	Name           string           `json:"name"`
	Market         string           `json:"market"`
	Currency       string           `json:"currency"`
	Quantity       float64          `json:"quantity"`
	CostPrice      float64          `json:"costPrice"`
	AcquiredAt     *time.Time       `json:"acquiredAt,omitempty"`
	CurrentPrice   float64          `json:"currentPrice"`
	PreviousClose  float64          `json:"previousClose"`
	OpenPrice      float64          `json:"openPrice"`
	DayHigh        float64          `json:"dayHigh"`
	DayLow         float64          `json:"dayLow"`
	Change         float64          `json:"change"`
	ChangePercent  float64          `json:"changePercent"`
	QuoteSource    string           `json:"quoteSource"`
	QuoteUpdatedAt *time.Time       `json:"quoteUpdatedAt,omitempty"`
	PinnedAt       *time.Time       `json:"pinnedAt,omitempty"`
	Thesis         string           `json:"thesis"`
	Tags           []string         `json:"tags"`
	DCAEntries     []DCAEntry       `json:"dcaEntries,omitempty"`
	DCASummary     *DCASummary      `json:"dcaSummary,omitempty"`
	Position       *PositionSummary `json:"position,omitempty"`
	UpdatedAt      time.Time        `json:"updatedAt"`
}

// CostBasis returns the total cost of the position (quantity × cost price).
func (w WatchlistItem) CostBasis() float64 {
	return w.Quantity * w.CostPrice
}

// MarketValue returns the current market value of the position (quantity × current price).
func (w WatchlistItem) MarketValue() float64 {
	return w.Quantity * w.CurrentPrice
}

// UnrealisedPnL returns the unrealized profit or loss (market value − cost basis).
func (w WatchlistItem) UnrealisedPnL() float64 {
	return w.MarketValue() - w.CostBasis()
}

// UnrealisedPnLPct returns unrealized P&L as a percentage of cost basis.
func (w WatchlistItem) UnrealisedPnLPct() float64 {
	if w.CostBasis() == 0 {
		return 0
	}

	return w.UnrealisedPnL() / w.CostBasis() * 100
}

// AlertRule represents a price alert rule.
type AlertRule struct {
	ID              string         `json:"id"`
	ItemID          string         `json:"itemId"`
	Name            string         `json:"name"`
	Condition       AlertCondition `json:"condition"`
	Threshold       float64        `json:"threshold"`
	Enabled         bool           `json:"enabled"`
	Triggered       bool           `json:"triggered"`
	LastTriggeredAt *time.Time     `json:"lastTriggeredAt,omitempty"`
	UpdatedAt       time.Time      `json:"updatedAt"`
}

// AppSettings represents application settings.
type AppSettings struct {
	RefreshIntervalSeconds int    `json:"refreshIntervalSeconds"`
	HotCacheTTLSeconds     int    `json:"hotCacheTTLSeconds"`
	QuoteSource            string `json:"quoteSource"`
	CNQuoteSource          string `json:"cnQuoteSource"`
	HKQuoteSource          string `json:"hkQuoteSource"`
	USQuoteSource          string `json:"usQuoteSource"`
	HotUSSource            string `json:"hotUSSource"`
	ThemeMode              string `json:"themeMode"`
	ColorTheme             string `json:"colorTheme"`
	FontPreset             string `json:"fontPreset"`
	AmountDisplay          string `json:"amountDisplay"`
	CurrencyDisplay        string `json:"currencyDisplay"`
	PriceColorScheme       string `json:"priceColorScheme"`
	Locale                 string `json:"locale"`
	ProxyMode              string `json:"proxyMode"`
	ProxyURL               string `json:"proxyURL"`
	DeveloperMode          bool   `json:"developerMode"`
	DashboardCurrency      string `json:"dashboardCurrency"`
	UseNativeTitleBar      bool   `json:"useNativeTitleBar"`
}

// DashboardSummary represents aggregated data to be displayed on the dashboard.
type DashboardSummary struct {
	TotalCost       float64 `json:"totalCost"`
	TotalValue      float64 `json:"totalValue"`
	TotalPnL        float64 `json:"totalPnL"`
	TotalPnLPct     float64 `json:"totalPnLPct"`
	ItemCount       int     `json:"itemCount"`
	TriggeredAlerts int     `json:"triggeredAlerts"`
	WinCount        int     `json:"winCount"`
	LossCount       int     `json:"lossCount"`
	DisplayCurrency string  `json:"displayCurrency"`
}

// OverviewHoldingSlice represents one instrument slice in the overview breakdown chart.
type OverviewHoldingSlice struct {
	ItemID   string  `json:"itemId"`
	Symbol   string  `json:"symbol"`
	Name     string  `json:"name"`
	Market   string  `json:"market"`
	Currency string  `json:"currency"`
	Value    float64 `json:"value"`
	Weight   float64 `json:"weight"`
}

// OverviewTrendSeries represents one instrument series in the overview stacked trend chart.
type OverviewTrendSeries struct {
	ItemID       string    `json:"itemId"`
	Symbol       string    `json:"symbol"`
	Name         string    `json:"name"`
	Market       string    `json:"market"`
	Currency     string    `json:"currency"`
	LatestValue  float64   `json:"latestValue"`
	FirstBuyDate time.Time `json:"firstBuyDate"`
	Values       []float64 `json:"values"`
}

// OverviewTrend represents the instrument-level portfolio trend timeline for the overview module.
type OverviewTrend struct {
	StartDate  *time.Time            `json:"startDate,omitempty"`
	EndDate    *time.Time            `json:"endDate,omitempty"`
	Dates      []time.Time           `json:"dates"`
	Series     []OverviewTrendSeries `json:"series"`
	TotalValue float64               `json:"totalValue"`
}

// OverviewAnalytics represents backend-computed overview analytics prepared for frontend rendering.
type OverviewAnalytics struct {
	DisplayCurrency string                 `json:"displayCurrency"`
	Breakdown       []OverviewHoldingSlice `json:"breakdown"`
	Trend           OverviewTrend          `json:"trend"`
	GeneratedAt     time.Time              `json:"generatedAt"`
}

// HotCategory represents hot list categories.
type HotCategory string

const (
	HotCategoryCNA      HotCategory = "cn-a"
	HotCategoryCNETF    HotCategory = "cn-etf"
	HotCategoryHK       HotCategory = "hk"
	HotCategoryHKETF    HotCategory = "hk-etf"
	HotCategoryUSSP500  HotCategory = "us-sp500"
	HotCategoryUSNasdaq HotCategory = "us-nasdaq"
	HotCategoryUSDow    HotCategory = "us-dow"
	HotCategoryUSETF    HotCategory = "us-etf"
)

// HotSort represents hot list table sorting options.
type HotSort string

const (
	HotSortVolume    HotSort = "volume"
	HotSortGainers   HotSort = "gainers"
	HotSortLosers    HotSort = "losers"
	HotSortMarketCap HotSort = "market-cap"
	HotSortPrice     HotSort = "price"
)

// HotItem represents detailed information for each item in the hot list.
type HotItem struct {
	Symbol        string    `json:"symbol"`
	Name          string    `json:"name"`
	Market        string    `json:"market"`
	Currency      string    `json:"currency"`
	CurrentPrice  float64   `json:"currentPrice"`
	Change        float64   `json:"change"`
	ChangePercent float64   `json:"changePercent"`
	Volume        float64   `json:"volume"`
	MarketCap     float64   `json:"marketCap"`
	QuoteSource   string    `json:"quoteSource"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

// HotListResponse represents paginated results returned by the hot list API.
type HotListResponse struct {
	Category       HotCategory `json:"category"`
	Sort           HotSort     `json:"sort"`
	Page           int         `json:"page"`
	PageSize       int         `json:"pageSize"`
	Total          int         `json:"total"`
	HasMore        bool        `json:"hasMore"`
	Items          []HotItem   `json:"items"`
	Cached         bool        `json:"cached"`
	CacheExpiresAt *time.Time  `json:"cacheExpiresAt,omitempty"`
	GeneratedAt    time.Time   `json:"generatedAt"`
}
