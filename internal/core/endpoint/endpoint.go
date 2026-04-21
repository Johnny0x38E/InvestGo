package endpoint

import "net/url"

const (
	EastMoneyQuoteAPI       = "https://push2.eastmoney.com/api/qt/ulist.np/get"
	EastMoneyHotAPI         = "https://push2.eastmoney.com/api/qt/clist/get"
	EastMoneyUSHotAPI       = "https://push2.eastmoney.com/api/qt/clist/get"
	EastMoneyStockAPI       = "https://push2.eastmoney.com/api/qt/stock/get"
	EastMoneyHistoryAPI     = "https://push2his.eastmoney.com/api/qt/stock/kline/get"
	EastMoneyWebReferer     = "https://quote.eastmoney.com/"
	EastMoneySuggestAPI     = "https://searchapi.eastmoney.com/api/suggest/get"
	SinaQuoteAPI            = "https://hq.sinajs.cn/list="
	SinaHotAPI              = "https://vip.stock.finance.sina.com.cn/quotes_service/api/json_v2.php/Market_Center.getHQNodeData"
	SinaCountAPI            = "https://vip.stock.finance.sina.com.cn/quotes_service/api/json_v2.php/Market_Center.getHQNodeStockCount"
	SinaFinanceReferer      = "https://finance.sina.com.cn/"
	TencentQuoteAPI         = "https://qt.gtimg.cn/q="
	TencentFQKlineAPI       = "https://web.ifzq.gtimg.cn/appstock/app/fqkline/get"
	TencentFinanceReferer   = "https://gu.qq.com/"
	XueqiuQuoteAPI          = "https://stock.xueqiu.com/v5/stock/realtime/quotec.json"
	XueqiuScreenerAPI       = "https://xueqiu.com/service/v5/stock/screener/quote/list"
	XueqiuOrigin            = "https://xueqiu.com"
	XueqiuReferer           = "https://xueqiu.com/"
	AlphaVantageAPI         = "https://www.alphavantage.co/query"
	TwelveDataQuoteAPI      = "https://api.twelvedata.com/quote"
	TwelveDataTimeSeriesAPI = "https://api.twelvedata.com/time_series"
	FinnhubQuoteAPI         = "https://finnhub.io/api/v1/quote"
	FinnhubCandleAPI        = "https://finnhub.io/api/v1/stock/candle"
	PolygonSnapshotAPI      = "https://api.polygon.io/v2/snapshot/locale/us/markets/stocks/tickers"
	PolygonAggsAPI          = "https://api.polygon.io/v2/aggs/ticker"
	YahooFinanceDomain      = "finance.yahoo.com"
	YahooFinanceOrigin      = "https://finance.yahoo.com"
	YahooFinanceReferer     = "https://finance.yahoo.com/"
	YahooSearchPath         = "/v1/finance/search"
	YahooScreenerListAPI    = "https://query1.finance.yahoo.com/v1/finance/screener/predefined/saved"
	YahooScreenerAPI        = "https://query2.finance.yahoo.com/v1/finance/screener"
	YahooChartPathPrefix    = "/v8/finance/chart/"
	FrankfurterAPI          = "https://api.frankfurter.dev/v1/latest" // European Central Bank (ECB) data providing multi-currency FX rates
)

var YahooChartHosts = [...]string{
	"query1.finance.yahoo.com",
	"query2.finance.yahoo.com",
}

var YahooSearchHosts = [...]string{
	"query1.finance.yahoo.com",
	"query2.finance.yahoo.com",
}

// URLWithQuery uniformly builds an endpoint with a query string for centralized base URL maintenance.
func URLWithQuery(base string, params url.Values) string {
	if len(params) == 0 {
		return base
	}
	return base + "?" + params.Encode()
}
