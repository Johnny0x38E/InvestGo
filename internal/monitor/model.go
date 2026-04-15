package monitor

import "time"

// AlertCondition 价格提醒的条件
type AlertCondition string

const (
	AlertAbove AlertCondition = "above"
	AlertBelow AlertCondition = "below"
)

// DCAEntry 定投
type DCAEntry struct {
	ID     string    `json:"id"`
	Date   time.Time `json:"date"`
	Amount float64   `json:"amount"`          // 本次投入金额
	Shares float64   `json:"shares"`          // 本次买入份额
	Price  float64   `json:"price,omitempty"` // 手动录入的买入价（含手续费后的实际成交价），0 表示未填写
	Fee    float64   `json:"fee,omitempty"`   // 本次定投手续费，0 表示未填写
	Note   string    `json:"note,omitempty"`
}

// WatchlistItem 自选的一个投资标的
type WatchlistItem struct {
	ID             string     `json:"id"`                       // 唯一标识符，格式为 "市场-代码"，如 "CN-A-000001"、"HK-MAIN-00700"、"US-STOCK-AAPL"
	Symbol         string     `json:"symbol"`                   // 标准化的标的代码，格式为 "市场-代码"，如 "CN-A-000001"、"HK-MAIN-00700"、"US-STOCK-AAPL"
	Name           string     `json:"name"`                     // 标的名称，如 "Apple Inc."、"Tesla, Inc."、"贵州茅台"
	Market         string     `json:"market"`                   // 市场标识，如 "CN-A"（沪深A股）、"HK-MAIN"（港股主板）、"US-STOCK"（美股）
	Currency       string     `json:"currency"`                 // 货币代码，如 "CNY"、"HKD"、"USD"
	Quantity       float64    `json:"quantity"`                 // 持仓数量
	CostPrice      float64    `json:"costPrice"`                // 持仓的平均成本价（含手续费），即总成本金额除以持仓数量
	CurrentPrice   float64    `json:"currentPrice"`             // 当前价格
	PreviousClose  float64    `json:"previousClose"`            // 昨收价格
	OpenPrice      float64    `json:"openPrice"`                // 今日开盘价
	DayHigh        float64    `json:"dayHigh"`                  // 今日最高价
	DayLow         float64    `json:"dayLow"`                   // 今日最低价
	Change         float64    `json:"change"`                   // 今日涨跌额
	ChangePercent  float64    `json:"changePercent"`            // 今日涨跌幅（百分比）
	QuoteSource    string     `json:"quoteSource"`              // 当前价格数据来源，如 "eastmoney"、"yahoo"
	QuoteUpdatedAt *time.Time `json:"quoteUpdatedAt,omitempty"` // 当前价格的最后更新时间
	Thesis         string     `json:"thesis"`                   // 备注
	Tags           []string   `json:"tags"`                     // 标签列表
	DCAEntries     []DCAEntry `json:"dcaEntries,omitempty"`     // 定投记录列表
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

// AlertRule 价格提醒规则
type AlertRule struct {
	ID              string         `json:"id"`
	ItemID          string         `json:"itemId"`
	Name            string         `json:"name"`
	Condition       AlertCondition `json:"condition"` // "above" 表示价格超过阈值时触发，"below" 表示价格低于阈值时触发
	Threshold       float64        `json:"threshold"` // 价格阈值
	Enabled         bool           `json:"enabled"`
	Triggered       bool           `json:"triggered"`                 // 是否已触发过，触发后会自动设置为 true
	LastTriggeredAt *time.Time     `json:"lastTriggeredAt,omitempty"` // 上次触发时间
	UpdatedAt       time.Time      `json:"updatedAt"`
}

// AppSettings 应用设置
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

// HistoryPoint 一根历史行情 K 线数据点
type HistoryPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Open      float64   `json:"open"`
	High      float64   `json:"high"`
	Low       float64   `json:"low"`
	Close     float64   `json:"close"`
	Volume    float64   `json:"volume"`
}

// HistorySeries 标的在指定区间下的完整历史走势
type HistorySeries struct {
	Symbol        string          `json:"symbol"`
	Name          string          `json:"name"`
	Market        string          `json:"market"`
	Currency      string          `json:"currency"`
	Interval      HistoryInterval `json:"interval"`      // 历史数据的时间间隔，如 "1d","1mo"
	Source        string          `json:"source"`        // 数据来源，如 "eastmoney"、"yahoo"
	StartPrice    float64         `json:"startPrice"`    // 区间的起始价格，即第一个数据点的开盘价
	EndPrice      float64         `json:"endPrice"`      // 区间的结束价格，即最后一个数据点的收盘价
	High          float64         `json:"high"`          // 区间内的最高价
	Low           float64         `json:"low"`           // 区间内的最低价
	Change        float64         `json:"change"`        // 区间内的涨跌额，即结束价格减去起始价格
	ChangePercent float64         `json:"changePercent"` // 区间内的涨跌幅，即涨跌额除以起始价格再乘以100
	Points        []HistoryPoint  `json:"points"`        // 数据点列表，按时间顺序排列
	GeneratedAt   time.Time       `json:"generatedAt"`   // 数据生成时间
}

// RuntimeStatus 存储应用运行时的状态信息
type RuntimeStatus struct {
	LastQuoteAttemptAt *time.Time `json:"lastQuoteAttemptAt,omitempty"` // 上次行情请求时间
	LastQuoteRefreshAt *time.Time `json:"lastQuoteRefreshAt,omitempty"` // 上次行情刷新时间
	LastQuoteError     string     `json:"lastQuoteError,omitempty"`     // 上次行情请求错误信息
	QuoteSource        string     `json:"quoteSource"`                  // 当前使用的数据来源，"auto" 表示自动选择
	LivePriceCount     int        `json:"livePriceCount"`               // 当前持仓中有价格的标的数量
	AppVersion         string     `json:"appVersion"`                   // 当前应用版本
	LastFxError        string     `json:"lastFxError,omitempty"`        // 上次汇率错误信息
	LastFxRefreshAt    *time.Time `json:"lastFxRefreshAt,omitempty"`    // 上次汇率刷新时间
}

// persistedState 需要持久化存储的应用状态
type persistedState struct {
	Items     []WatchlistItem `json:"items"`     // 持仓列表
	Alerts    []AlertRule     `json:"alerts"`    // 价格提醒规则列表
	Settings  AppSettings     `json:"settings"`  // 设置
	UpdatedAt time.Time       `json:"updatedAt"` // 上次更新的时间戳
}

// DashboardSummary 仪表盘上需要展示的汇总数据
type DashboardSummary struct {
	TotalCost       float64 `json:"totalCost"`       // 组合成本
	TotalValue      float64 `json:"totalValue"`      // 总资产
	TotalPnL        float64 `json:"totalPnL"`        // 总盈亏金额
	TotalPnLPct     float64 `json:"totalPnLPct"`     // 总盈亏百分比
	ItemCount       int     `json:"itemCount"`       // 持仓数量
	TriggeredAlerts int     `json:"triggeredAlerts"` // 触发的提醒数量
	WinCount        int     `json:"winCount"`        // 盈亏统计中的盈利数量
	LossCount       int     `json:"lossCount"`       // 盈亏统计中的亏损数量
	DisplayCurrency string  `json:"displayCurrency"` // 汇总数据的显示货币
}

// StateSnapshot 应用状态的完整快照
type StateSnapshot struct {
	Dashboard    DashboardSummary    `json:"dashboard"`    // 仪表盘汇总数据
	Items        []WatchlistItem     `json:"items"`        // 持仓列表
	Alerts       []AlertRule         `json:"alerts"`       // 价格提醒规则列表
	Settings     AppSettings         `json:"settings"`     // 设置
	Runtime      RuntimeStatus       `json:"runtime"`      // 运行状态
	QuoteSources []QuoteSourceOption `json:"quoteSources"` // 可用的数据来源列表
	StoragePath  string              `json:"storagePath"`  // 持久化存储路径
	GeneratedAt  time.Time           `json:"generatedAt"`  // 快照生成时间
}

// HotCategory 热门榜单的分类
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

// HotSort 热门榜单表格排序
type HotSort string

const (
	HotSortVolume    HotSort = "volume"     // 成交量榜
	HotSortGainers   HotSort = "gainers"    // 涨幅榜
	HotSortLosers    HotSort = "losers"     // 跌幅榜
	HotSortMarketCap HotSort = "market-cap" // 市值榜
	HotSortPrice     HotSort = "price"      // 价格榜
)

// HotItem 热门榜单中每个标的的详细信息
type HotItem struct {
	Symbol        string    `json:"symbol"`
	Name          string    `json:"name"`
	Market        string    `json:"market"`
	Currency      string    `json:"currency"`
	CurrentPrice  float64   `json:"currentPrice"`
	Change        float64   `json:"change"`        // 涨跌额
	ChangePercent float64   `json:"changePercent"` // 涨跌幅（百分比）
	Volume        float64   `json:"volume"`        // 成交量
	MarketCap     float64   `json:"marketCap"`     // 市值
	QuoteSource   string    `json:"quoteSource"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

// HotListResponse 表示热门榜单接口返回的分页结果
type HotListResponse struct {
	Category    HotCategory `json:"category"` // 热门榜单分类，如 "cn-a"、"hk"、"us-sp500"
	Sort        HotSort     `json:"sort"`     // 排序方式，如 "volume"、"gainers"、"losers"
	Page        int         `json:"page"`
	PageSize    int         `json:"pageSize"`
	Total       int         `json:"total"`
	HasMore     bool        `json:"hasMore"`
	Items       []HotItem   `json:"items"`
	GeneratedAt time.Time   `json:"generatedAt"`
}
