package marketdata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"investgo/internal/datasource"
	"investgo/internal/monitor"
)

type YahooQuoteProvider struct {
	client *http.Client
}

type EastMoneyQuoteProvider struct {
	client *http.Client
}

type eastMoneyQuoteResponse struct {
	RC   int                `json:"rc"`
	Data EastMoneyQuoteData `json:"data"`
}

type EastMoneyQuoteData struct {
	Diff []EastMoneyQuoteDataDiff `json:"diff"`
}

type EastMoneyQuoteDataDiff struct {
	MarketID      int     `json:"f13"`
	Code          string  `json:"f12"`
	Name          string  `json:"f14"`
	CurrentPrice  emFloat `json:"f2"`
	ChangePercent emFloat `json:"f3"`
	Change        emFloat `json:"f4"`
	DayHigh       emFloat `json:"f15"`
	DayLow        emFloat `json:"f16"`
	OpenPrice     emFloat `json:"f17"`
	PreviousClose emFloat `json:"f18"`
}

// DefaultQuoteSourceRegistry 返回默认行情源注册表及其前端展示配置。
func DefaultQuoteSourceRegistry(client *http.Client) (map[string]monitor.QuoteProvider, []monitor.QuoteSourceOption) {
	eastMoney := NewEastMoneyQuoteProvider(client)
	yahoo := NewYahooQuoteProvider(client)

	options := []monitor.QuoteSourceOption{
		{
			ID:               "eastmoney",
			Name:             "EastMoney",
			Description:      "Best overall coverage for China, Hong Kong, and US markets with the most complete fields.",
			SupportedMarkets: []string{"CN-A", "CN-GEM", "CN-STAR", "CN-ETF", "HK-MAIN", "HK-GEM", "HK-ETF", "US-STOCK", "US-ETF"},
		},
		{
			ID:               "yahoo",
			Name:             "Yahoo Finance",
			Description:      "Stable coverage for Hong Kong and US markets, especially for overseas-focused portfolios.",
			SupportedMarkets: []string{"CN-A", "CN-GEM", "CN-STAR", "CN-ETF", "HK-MAIN", "HK-GEM", "HK-ETF", "US-STOCK", "US-ETF"},
		},
	}

	return map[string]monitor.QuoteProvider{
		"eastmoney": eastMoney,
		"yahoo":     yahoo,
	}, options
}

// NewYahooQuoteProvider 创建 Yahoo 实时报价 provider。
func NewYahooQuoteProvider(client *http.Client) *YahooQuoteProvider {
	if client == nil {
		client = &http.Client{Timeout: 8 * time.Second}
	}

	return &YahooQuoteProvider{client: client}
}

func (p *YahooQuoteProvider) Name() string {
	return "Yahoo Finance"
}

// Fetch 批量请求 Yahoo Finance 实时行情，并映射回标准 Quote 结构。
func (p *YahooQuoteProvider) Fetch(ctx context.Context, items []monitor.WatchlistItem) (map[string]monitor.Quote, error) {
	targets, problems := collectQuoteTargets(items)
	quotes := make(map[string]monitor.Quote, len(targets))
	if len(targets) == 0 {
		return quotes, collapseProblems(problems)
	}

	for _, item := range items {
		target, err := monitor.ResolveQuoteTarget(item)
		if err != nil {
			continue
		}

		yahooSymbol, err := resolveYahooSymbol(item)
		if err != nil {
			problems = append(problems, fmt.Sprintf("Yahoo does not support item: %s", target.DisplaySymbol))
			continue
		}

		quote, err := p.fetchChartSnapshot(ctx, item, yahooSymbol)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", target.DisplaySymbol, err))
			continue
		}

		quote.Symbol = target.DisplaySymbol
		quote.Market = target.Market
		quote.Currency = firstNonEmpty(quote.Currency, target.Currency)
		quotes[target.Key] = quote
	}

	return quotes, collapseProblems(problems)
}

// fetchChartSnapshot 请求 Yahoo Finance 的图表数据接口，解析最近 5 天的日线数据，并从中提取最新价格点构建 Quote 对象。
func (p *YahooQuoteProvider) fetchChartSnapshot(ctx context.Context, item monitor.WatchlistItem, yahooSymbol string) (monitor.Quote, error) {
	params := url.Values{}
	params.Set("range", "5d")
	params.Set("interval", "1d")
	params.Set("includePrePost", "false")
	params.Set("events", "div,splits")

	parsed, err := fetchYahooChart(ctx, p.client, yahooSymbol, params)
	if err != nil {
		return monitor.Quote{}, fmt.Errorf("Yahoo quote request failed: %w", err)
	}
	if len(parsed.Chart.Result) == 0 || len(parsed.Chart.Result[0].Indicators.Quote) == 0 {
		return monitor.Quote{}, errors.New("Yahoo quote response is empty")
	}

	result := parsed.Chart.Result[0]
	points := buildHistoryPoints(result.Timestamp, result.Indicators.Quote[0])
	if len(points) == 0 {
		return monitor.Quote{}, errors.New("Yahoo quote response contains no valid price points")
	}

	latest := points[len(points)-1]
	previousClose := latest.Open
	if len(points) >= 2 && points[len(points)-2].Close > 0 {
		previousClose = points[len(points)-2].Close
	}
	if previousClose <= 0 {
		previousClose = latest.Close
	}

	quote := buildQuote(
		firstNonEmpty(item.Name, result.Meta.Symbol, item.Symbol),
		firstNonEmptyFloat(result.Meta.Price, latest.Close),
		previousClose,
		latest.Open,
		latest.High,
		latest.Low,
		latest.Timestamp,
		p.Name(),
	)
	quote.Currency = firstNonEmpty(result.Meta.Currency, item.Currency)
	return quote, nil
}

// NewEastMoneyQuoteProvider 创建东方财富实时报价 provider。
func NewEastMoneyQuoteProvider(client *http.Client) *EastMoneyQuoteProvider {
	if client == nil {
		client = &http.Client{Timeout: 8 * time.Second}
	}

	return &EastMoneyQuoteProvider{client: client}
}

// Name 返回东方财富报价源的显示名称。
func (p *EastMoneyQuoteProvider) Name() string {
	return "EastMoney"
}

// Fetch 批量请求东方财富实时行情，并映射回标准 Quote 结构。
func (p *EastMoneyQuoteProvider) Fetch(ctx context.Context, items []monitor.WatchlistItem) (map[string]monitor.Quote, error) {
	targets, problems := collectQuoteTargets(items)
	quotes := make(map[string]monitor.Quote, len(targets))
	if len(targets) == 0 {
		return quotes, collapseProblems(problems)
	}

	// 东方财富按 secid 批量查询，因此先把标准目标映射为 secid。
	secids := make([]string, 0, len(targets)*2)
	indexBySecID := make(map[string]monitor.QuoteTarget, len(targets)*2)
	for _, target := range targets {
		ids, err := resolveAllEastMoneySecIDs(target)
		if err != nil {
			problems = append(problems, err.Error())
			continue
		}
		for _, secid := range ids {
			secids = append(secids, secid)
			indexBySecID[secid] = target
		}
	}

	if len(secids) == 0 {
		return quotes, collapseProblems(problems)
	}

	params := url.Values{}
	params.Set("fltt", "2")
	params.Set("invt", "2")
	params.Set("np", "1")
	params.Set("ut", "bd1d9ddb04089700cf9c27f6f7426281")
	params.Set("fields", "f2,f3,f4,f12,f13,f14,f15,f16,f17,f18")
	params.Set("secids", strings.Join(secids, ","))

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, datasource.URLWithQuery(datasource.EastMoneyQuoteAPI, params), nil)
	if err != nil {
		return quotes, err
	}
	request.Header.Set("Referer", datasource.EastMoneyWebReferer)
	request.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/123.0.0.0 Safari/537.36")

	response, err := p.client.Do(request)
	if err != nil {
		return quotes, err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return quotes, fmt.Errorf("EastMoney quote request failed: status %d", response.StatusCode)
	}

	payload, err := io.ReadAll(response.Body)
	if err != nil {
		return quotes, err
	}

	var parsed eastMoneyQuoteResponse
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return quotes, err
	}
	if parsed.RC != 0 {
		return quotes, fmt.Errorf("EastMoney quote response returned rc=%d", parsed.RC)
	}

	for _, item := range parsed.Data.Diff {
		secid := fmt.Sprintf("%d.%s", item.MarketID, normaliseEastMoneyCode(item.Code, item.MarketID))
		target, ok := indexBySecID[secid]
		if !ok {
			continue
		}

		quote := buildQuote(
			item.Name,
			float64(item.CurrentPrice),
			float64(item.PreviousClose),
			float64(item.OpenPrice),
			float64(item.DayHigh),
			float64(item.DayLow),
			time.Now(),
			p.Name(),
		)
		quote.Symbol = target.DisplaySymbol
		quote.Market = target.Market
		quote.Currency = target.Currency
		if item.ChangePercent != 0 {
			quote.ChangePercent = float64(item.ChangePercent)
		}
		if item.Change != 0 {
			quote.Change = float64(item.Change)
		}
		quotes[target.Key] = quote
	}

	for secid, target := range indexBySecID {
		if _, ok := quotes[target.Key]; ok {
			continue
		}
		problems = append(problems, fmt.Sprintf("Did not receive EastMoney quote for %s (%s)", target.DisplaySymbol, secid))
	}

	return quotes, collapseProblems(problems)
}

// resolveEastMoneySecID 把标准目标转换为东方财富接口需要的 secid（返回第一个匹配）。
// 注意：美股会返回 105（NASDAQ）作为默认值，但实际可能在 106（NYSE）或 107（NYSE Arca）上。
// 批量行情请求应优先使用 resolveAllEastMoneySecIDs 以覆盖所有美股交易所。
func resolveEastMoneySecID(target monitor.QuoteTarget) (string, error) {
	ids, err := resolveAllEastMoneySecIDs(target)
	if err != nil {
		return "", err
	}
	return ids[0], nil
}

// resolveAllEastMoneySecIDs 把标准目标转换为东方财富接口可能需要的所有 secid。
// 对于美股，由于同一 ticker 可能在 NASDAQ(105)、NYSE(106) 或 NYSE Arca(107) 上市，
// 返回所有三个变体以确保能命中正确的交易所。
func resolveAllEastMoneySecIDs(target monitor.QuoteTarget) ([]string, error) {
	symbol := target.DisplaySymbol
	market := target.Market

	switch market {
	case "CN-A", "CN-GEM", "CN-STAR", "CN-ETF":
		if strings.HasSuffix(symbol, ".SH") {
			return []string{"1." + strings.TrimSuffix(symbol, ".SH")}, nil
		}
		if strings.HasSuffix(symbol, ".SZ") {
			return []string{"0." + strings.TrimSuffix(symbol, ".SZ")}, nil
		}
		return nil, fmt.Errorf("A-share / ETF symbol format is invalid: %s", symbol)
	case "CN-BJ":
		return nil, fmt.Errorf("Realtime quotes are not supported for Beijing Exchange symbols in EastMoney: %s", symbol)
	case "HK-MAIN", "HK-GEM", "HK-ETF":
		if strings.HasSuffix(symbol, ".HK") {
			return []string{"116." + strings.TrimSuffix(symbol, ".HK")}, nil
		}
		return nil, fmt.Errorf("Hong Kong symbol format is invalid: %s", symbol)
	case "US-STOCK", "US-ETF":
		var ticker string
		if isLetters(symbol) {
			ticker = symbol
		} else if strings.Contains(symbol, "-") {
			ticker = strings.ReplaceAll(symbol, "-", ".")
		} else {
			return nil, fmt.Errorf("US symbol format is invalid: %s", symbol)
		}
		// 105=NASDAQ, 106=NYSE, 107=NYSE Arca — 三个都请求以覆盖所有交易所
		return []string{"105." + ticker, "106." + ticker, "107." + ticker}, nil
	default:
		return nil, fmt.Errorf("Market type is unsupported: %s", market)
	}
}
