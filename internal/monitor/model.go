package monitor

import "time"

type AlertCondition string

const (
	AlertAbove AlertCondition = "above"
	AlertBelow AlertCondition = "below"
)

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
	Thesis         string     `json:"thesis"`
	Tags           []string   `json:"tags"`
	UpdatedAt      time.Time  `json:"updatedAt"`
}

func (w WatchlistItem) CostBasis() float64 {
	return w.Quantity * w.CostPrice
}

func (w WatchlistItem) MarketValue() float64 {
	return w.Quantity * w.CurrentPrice
}

func (w WatchlistItem) UnrealisedPnL() float64 {
	return w.MarketValue() - w.CostBasis()
}

func (w WatchlistItem) UnrealisedPnLPct() float64 {
	if w.CostBasis() == 0 {
		return 0
	}

	return w.UnrealisedPnL() / w.CostBasis() * 100
}

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

type AppSettings struct {
	PriceMode              string `json:"priceMode"`
	RefreshIntervalSeconds int    `json:"refreshIntervalSeconds"`
	QuoteSource            string `json:"quoteSource"`
	FontPreset             string `json:"fontPreset"`
	AmountDisplay          string `json:"amountDisplay"`
	CurrencyDisplay        string `json:"currencyDisplay"`
	PriceColorScheme       string `json:"priceColorScheme"`
	Locale                 string `json:"locale"`
	DeveloperMode          bool   `json:"developerMode"`
}

type HistoryInterval string

const (
	HistoryLive  HistoryInterval = "live"
	HistoryHour1 HistoryInterval = "1h"
	HistoryHour6 HistoryInterval = "6h"
	HistoryDay   HistoryInterval = "day"
	HistoryWeek  HistoryInterval = "week"
	HistoryMonth HistoryInterval = "month"
)

type HistoryPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Open      float64   `json:"open"`
	High      float64   `json:"high"`
	Low       float64   `json:"low"`
	Close     float64   `json:"close"`
	Volume    float64   `json:"volume"`
}

type HistorySeries struct {
	Symbol        string          `json:"symbol"`
	Name          string          `json:"name"`
	Market        string          `json:"market"`
	Currency      string          `json:"currency"`
	Interval      HistoryInterval `json:"interval"`
	Source        string          `json:"source"`
	StartPrice    float64         `json:"startPrice"`
	EndPrice      float64         `json:"endPrice"`
	High          float64         `json:"high"`
	Low           float64         `json:"low"`
	Change        float64         `json:"change"`
	ChangePercent float64         `json:"changePercent"`
	Points        []HistoryPoint  `json:"points"`
	GeneratedAt   time.Time       `json:"generatedAt"`
}

type RuntimeStatus struct {
	LastQuoteAttemptAt *time.Time `json:"lastQuoteAttemptAt,omitempty"`
	LastQuoteRefreshAt *time.Time `json:"lastQuoteRefreshAt,omitempty"`
	LastQuoteError     string     `json:"lastQuoteError,omitempty"`
	QuoteSource        string     `json:"quoteSource"`
	LivePriceCount     int        `json:"livePriceCount"`
}

type persistedState struct {
	Items     []WatchlistItem `json:"items"`
	Alerts    []AlertRule     `json:"alerts"`
	Settings  AppSettings     `json:"settings"`
	UpdatedAt time.Time       `json:"updatedAt"`
}

type DashboardSummary struct {
	TotalCost       float64 `json:"totalCost"`
	TotalValue      float64 `json:"totalValue"`
	TotalPnL        float64 `json:"totalPnL"`
	TotalPnLPct     float64 `json:"totalPnLPct"`
	ItemCount       int     `json:"itemCount"`
	TriggeredAlerts int     `json:"triggeredAlerts"`
	WinCount        int     `json:"winCount"`
	LossCount       int     `json:"lossCount"`
}

type StateSnapshot struct {
	Dashboard    DashboardSummary    `json:"dashboard"`
	Items        []WatchlistItem     `json:"items"`
	Alerts       []AlertRule         `json:"alerts"`
	Settings     AppSettings         `json:"settings"`
	Runtime      RuntimeStatus       `json:"runtime"`
	QuoteSources []QuoteSourceOption `json:"quoteSources"`
	StoragePath  string              `json:"storagePath"`
	GeneratedAt  time.Time           `json:"generatedAt"`
}

type HotCategory string

const (
	HotCategoryUSSP500   HotCategory = "us-sp500"
	HotCategoryUSNasdaq  HotCategory = "us-nasdaq100"
	HotCategoryUSDow     HotCategory = "us-dow30"
	HotCategoryETFBroad  HotCategory = "etf-broad"
	HotCategoryETFSector HotCategory = "etf-sector"
	HotCategoryETFIncome HotCategory = "etf-income"
	HotCategoryHKMain    HotCategory = "hk-main"
)

type HotSort string

const (
	HotSortVolume    HotSort = "volume"
	HotSortGainers   HotSort = "gainers"
	HotSortLosers    HotSort = "losers"
	HotSortMarketCap HotSort = "market-cap"
	HotSortPrice     HotSort = "price"
)

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
