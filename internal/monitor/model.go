package monitor

import "time"

type AlertCondition string

const (
	AlertAbove AlertCondition = "above"
	AlertBelow AlertCondition = "below"
)

// DCAEntry 记录一次定投操作。date + amount + shares 三者确定一笔定投。
type DCAEntry struct {
	ID     string    `json:"id"`
	Date   time.Time `json:"date"`
	Amount float64   `json:"amount"`          // 本次投入金额
	Shares float64   `json:"shares"`          // 本次买入份额
	Price  float64   `json:"price,omitempty"` // 手动录入的买入价（含手续费后的实际成交价），0 表示未填写
	Fee    float64   `json:"fee,omitempty"`   // 本次定投手续费，0 表示未填写
	Note   string    `json:"note,omitempty"`
}

// WatchlistItem 表示用户自选的一个投资标的，可以是股票、基金、加密货币等任何有价格的资产。
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
	DCAEntries     []DCAEntry `json:"dcaEntries,omitempty"`
	UpdatedAt      time.Time  `json:"updatedAt"`
}

// CostBasis 返回持仓的成本金额，即买入数量乘以买入价格。
func (w WatchlistItem) CostBasis() float64 {
	return w.Quantity * w.CostPrice
}

// MarketValue 返回持仓的当前资产，即持仓数量乘以当前价格。
func (w WatchlistItem) MarketValue() float64 {
	return w.Quantity * w.CurrentPrice
}

// UnrealisedPnL 返回持仓的未实现盈亏金额，即当前资产减去成本金额。
func (w WatchlistItem) UnrealisedPnL() float64 {
	return w.MarketValue() - w.CostBasis()
}

// UnrealisedPnLPct 返回持仓的未实现盈亏百分比，即未实现盈亏金额除以成本金额再乘以100。
func (w WatchlistItem) UnrealisedPnLPct() float64 {
	if w.CostBasis() == 0 {
		return 0
	}

	return w.UnrealisedPnL() / w.CostBasis() * 100
}

// AlertRule 定义了一个价格提醒规则，用户可以设置当某个标的的价格超过或低于某个阈值时触发提醒。
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

// AppSettings 存储用户的应用设置，如刷新频率、数据来源和显示偏好等。
type AppSettings struct {
	RefreshIntervalSeconds int    `json:"refreshIntervalSeconds"`
	QuoteSource            string `json:"quoteSource"` // "auto"、"cn"、"hk"、"us"，默认 "auto"
	CNQuoteSource          string `json:"cnQuoteSource"`
	HKQuoteSource          string `json:"hkQuoteSource"`
	USQuoteSource          string `json:"usQuoteSource"`
	HotUSSource            string `json:"hotUSSource"` // "eastmoney" 或 "yahoo"，默认 "eastmoney"
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

// HistoryPoint 表示一根历史行情 K 线数据点。
type HistoryPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Open      float64   `json:"open"`
	High      float64   `json:"high"`
	Low       float64   `json:"low"`
	Close     float64   `json:"close"`
	Volume    float64   `json:"volume"`
}

// HistorySeries 表示某个标的在指定区间下的完整历史走势。
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

// RuntimeStatus 存储应用运行时的状态信息，
// 如上次行情请求时间、上次行情刷新时间、上次行情错误信息、当前使用的数据来源和有效价格数量等。
type RuntimeStatus struct {
	LastQuoteAttemptAt *time.Time `json:"lastQuoteAttemptAt,omitempty"`
	LastQuoteRefreshAt *time.Time `json:"lastQuoteRefreshAt,omitempty"`
	LastQuoteError     string     `json:"lastQuoteError,omitempty"`
	QuoteSource        string     `json:"quoteSource"`
	LivePriceCount     int        `json:"livePriceCount"`
	AppVersion         string     `json:"appVersion"`
}

// persistedState 定义了需要持久化存储的应用状态,
// 包括自选项列表、价格提醒规则列表、用户设置和上次更新的时间戳等。
type persistedState struct {
	Items     []WatchlistItem `json:"items"`
	Alerts    []AlertRule     `json:"alerts"`
	Settings  AppSettings     `json:"settings"`
	UpdatedAt time.Time       `json:"updatedAt"`
}

// DashboardSummary 定义了仪表盘上需要展示的汇总数据
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

// StateSnapshot 定义了应用状态的完整快照，
// 包括仪表盘汇总数据、自选项列表、价格提醒规则列表、用户设置、运行时状态、可用的数据来源列表和存储路径等。
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

// HotCategory 定义了热门榜单的分类。
// A股合并为一个分类（排除B股/ST），ETF单列；港股不再细分主板/创业板；美股按三大指数分类。
type HotCategory string

const (
	HotCategoryCNA      HotCategory = "cn-a"      // 沪深A股（主板+创业板+科创板）
	HotCategoryCNETF    HotCategory = "cn-etf"    // 沪深ETF
	HotCategoryHK       HotCategory = "hk"        // 港股
	HotCategoryHKETF    HotCategory = "hk-etf"    // 港股ETF
	HotCategoryUSSP500  HotCategory = "us-sp500"  // 标普500
	HotCategoryUSNasdaq HotCategory = "us-nasdaq" // 纳斯达克100
	HotCategoryUSDow    HotCategory = "us-dow"    // 道琼斯30
	HotCategoryUSETF    HotCategory = "us-etf"    // 美股ETF
)

type HotSort string

const (
	HotSortVolume    HotSort = "volume"
	HotSortGainers   HotSort = "gainers"
	HotSortLosers    HotSort = "losers"
	HotSortMarketCap HotSort = "market-cap"
	HotSortPrice     HotSort = "price"
)

// HotItem 定义了热门榜单中每个标的的详细信息，
// 包括代码、名称、市场、货币、当前价格、涨跌额、涨跌幅、成交量、市值、数据来源和更新时间等。
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

// HotListResponse 表示热门榜单接口返回的分页结果。
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
