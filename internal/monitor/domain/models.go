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
	ID     string    `json:"id"`
	Date   time.Time `json:"date"`
	Amount float64   `json:"amount"`
	Shares float64   `json:"shares"`
	Price  float64   `json:"price,omitempty"`
	Fee    float64   `json:"fee,omitempty"`
	Note   string    `json:"note,omitempty"`
}

// WatchlistItem represents an investment item on the watchlist.
type WatchlistItem struct {
	ID             string     `json:"id"`
	Symbol         string     `json:"symbol"`
	Name           string     `json:"name"`
	Market         string     `json:"market"`
	Currency       string     `json:"currency"`
	Quantity       float64    `json:"quantity"`
	CostPrice      float64    `json:"costPrice"`
	CurrentPrice   float64    `json:"currentPrice"`
	PreviousClose  float64    `json:"previousClose"`
	OpenPrice      float64    `json:"openPrice"`
	DayHigh        float64    `json:"dayHigh"`
	DayLow         float64    `json:"dayLow"`
	Change         float64    `json:"change"`
	ChangePercent  float64    `json:"changePercent"`
	QuoteSource    string     `json:"quoteSource"`
	QuoteUpdatedAt *time.Time `json:"quoteUpdatedAt,omitempty"`
	PinnedAt       *time.Time `json:"pinnedAt,omitempty"`
	Thesis         string     `json:"thesis"`
	Tags           []string   `json:"tags"`
	DCAEntries     []DCAEntry `json:"dcaEntries,omitempty"`
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

// UnrealisedPnL returns position PnL amount, i.e., current value minus cost amount.
func (w WatchlistItem) UnrealisedPnL() float64 {
	return w.MarketValue() - w.CostBasis()
}

// UnrealisedPnLPct returns position PnL percentage.
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
	Category    HotCategory `json:"category"`
	Sort        HotSort     `json:"sort"`
	Page        int         `json:"page"`
	PageSize    int         `json:"pageSize"`
	Total       int         `json:"total"`
	HasMore     bool        `json:"hasMore"`
	Items       []HotItem   `json:"items"`
	GeneratedAt time.Time   `json:"generatedAt"`
}
