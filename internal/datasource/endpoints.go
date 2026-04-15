package datasource

import "net/url"

const (
	EastMoneyQuoteAPI    = "https://push2.eastmoney.com/api/qt/ulist.np/get"
	EastMoneyHotAPI      = "https://push2.eastmoney.com/api/qt/clist/get"
	EastMoneyHistoryAPI  = "https://push2his.eastmoney.com/api/qt/stock/kline/get"
	EastMoneyWebReferer  = "https://quote.eastmoney.com/"
	YahooFinanceDomain   = "finance.yahoo.com"
	YahooFinanceOrigin   = "https://finance.yahoo.com"
	YahooFinanceReferer  = "https://finance.yahoo.com/"
	YahooSearchPath      = "/v1/finance/search"
	YahooScreenerListAPI = "https://query1.finance.yahoo.com/v1/finance/screener/predefined/saved"
	YahooScreenerAPI     = "https://query2.finance.yahoo.com/v1/finance/screener"
	YahooChartPathPrefix = "/v8/finance/chart/"
	FrankfurterAPI       = "https://api.frankfurter.dev/v1/latest" // 欧洲央行数据，提供多币种汇率
)

var YahooChartHosts = [...]string{
	"query1.finance.yahoo.com",
	"query2.finance.yahoo.com",
}

var YahooSearchHosts = [...]string{
	"query1.finance.yahoo.com",
	"query2.finance.yahoo.com",
}

// URLWithQuery 统一拼接带 query string 的 endpoint，便于集中维护基础地址。
func URLWithQuery(base string, params url.Values) string {
	if len(params) == 0 {
		return base
	}
	return base + "?" + params.Encode()
}
