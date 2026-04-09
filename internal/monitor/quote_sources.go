package monitor

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
)

const defaultQuoteSourceID = "tx-sina"

type QuoteSourceOption struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type TencentQuoteProvider struct {
	*PublicQuoteProvider
}

type SinaQuoteProvider struct {
	*PublicQuoteProvider
}

type ReversePublicQuoteProvider struct {
	*PublicQuoteProvider
}

type YahooQuoteProvider struct {
	client *http.Client
}

type EastMoneyQuoteProvider struct {
	client *http.Client
}

type eastMoneyQuoteResponse struct {
	RC   int `json:"rc"`
	Data struct {
		Diff []struct {
			MarketID      int     `json:"f13"`
			Code          string  `json:"f12"`
			Name          string  `json:"f14"`
			CurrentPrice  float64 `json:"f2"`
			ChangePercent float64 `json:"f3"`
			Change        float64 `json:"f4"`
			DayHigh       float64 `json:"f15"`
			DayLow        float64 `json:"f16"`
			OpenPrice     float64 `json:"f17"`
			PreviousClose float64 `json:"f18"`
		} `json:"diff"`
	} `json:"data"`
}

func DefaultQuoteSourceRegistry(client *http.Client) (map[string]QuoteProvider, []QuoteSourceOption) {
	tencent := NewTencentQuoteProvider(client)
	sina := NewSinaQuoteProvider(client)
	yahoo := NewYahooQuoteProvider(client)
	eastMoney := NewEastMoneyQuoteProvider(client)
	tencentFirst := NewPublicQuoteProvider(client)
	sinaFirst := NewReversePublicQuoteProvider(client)

	options := []QuoteSourceOption{
		{
			ID:          defaultQuoteSourceID,
			Name:        "腾讯优先，新浪回补",
			Description: "默认策略。A 股和港股优先用腾讯，未命中时回退到新浪。",
		},
		{
			ID:          "sina-tx",
			Name:        "新浪优先，腾讯回补",
			Description: "优先新浪，再回退腾讯。适合你更信任新浪字段时使用。",
		},
		{
			ID:          "tencent",
			Name:        "仅腾讯",
			Description: "单独使用腾讯行情，覆盖 A 股、港股较好。",
		},
		{
			ID:          "sina",
			Name:        "仅新浪",
			Description: "单独使用新浪行情，A 股、港股、美股都可尝试。",
		},
		{
			ID:          "yahoo",
			Name:        "Yahoo Finance",
			Description: "改用 Yahoo Chart 接口获取实时快照，适合美股、ETF 和多数港股。",
		},
		{
			ID:          "eastmoney",
			Name:        "东方财富",
			Description: "免费的综合行情来源，适合 A 股、港股和多数美股代码。",
		},
	}

	return map[string]QuoteProvider{
		defaultQuoteSourceID: tencentFirst,
		"sina-tx":            sinaFirst,
		"tencent":            tencent,
		"sina":               sina,
		"yahoo":              yahoo,
		"eastmoney":          eastMoney,
	}, options
}

func NewTencentQuoteProvider(client *http.Client) *TencentQuoteProvider {
	return &TencentQuoteProvider{PublicQuoteProvider: NewPublicQuoteProvider(client)}
}

func (p *TencentQuoteProvider) Name() string {
	return "Tencent"
}

func (p *TencentQuoteProvider) Fetch(ctx context.Context, items []WatchlistItem) (map[string]Quote, error) {
	targets, problems := collectQuoteTargets(items)
	txTargets := make([]quoteTarget, 0, len(targets))
	for _, target := range targets {
		if target.TXCode == "" {
			problems = append(problems, fmt.Sprintf("腾讯不支持该标的: %s", target.DisplaySymbol))
			continue
		}
		txTargets = append(txTargets, target)
	}

	quotes, txProblems := p.fetchTencent(ctx, txTargets)
	problems = append(problems, txProblems...)
	for key, target := range targets {
		if _, ok := quotes[key]; ok || target.TXCode == "" {
			continue
		}
		problems = append(problems, fmt.Sprintf("未收到 %s 的腾讯行情", target.DisplaySymbol))
	}

	return quotes, collapseProblems(problems)
}

func NewSinaQuoteProvider(client *http.Client) *SinaQuoteProvider {
	return &SinaQuoteProvider{PublicQuoteProvider: NewPublicQuoteProvider(client)}
}

func (p *SinaQuoteProvider) Name() string {
	return "Sina"
}

func (p *SinaQuoteProvider) Fetch(ctx context.Context, items []WatchlistItem) (map[string]Quote, error) {
	targets, problems := collectQuoteTargets(items)
	sinaTargets := make([]quoteTarget, 0, len(targets))
	for _, target := range targets {
		if target.SinaCode == "" {
			problems = append(problems, fmt.Sprintf("新浪不支持该标的: %s", target.DisplaySymbol))
			continue
		}
		sinaTargets = append(sinaTargets, target)
	}

	quotes, sinaProblems := p.fetchSina(ctx, sinaTargets)
	problems = append(problems, sinaProblems...)
	for key, target := range targets {
		if _, ok := quotes[key]; ok || target.SinaCode == "" {
			continue
		}
		problems = append(problems, fmt.Sprintf("未收到 %s 的新浪行情", target.DisplaySymbol))
	}

	return quotes, collapseProblems(problems)
}

func NewReversePublicQuoteProvider(client *http.Client) *ReversePublicQuoteProvider {
	return &ReversePublicQuoteProvider{PublicQuoteProvider: NewPublicQuoteProvider(client)}
}

func (p *ReversePublicQuoteProvider) Name() string {
	return "Sina + Tencent"
}

func (p *ReversePublicQuoteProvider) Fetch(ctx context.Context, items []WatchlistItem) (map[string]Quote, error) {
	targets, problems := collectQuoteTargets(items)
	quotes := make(map[string]Quote, len(targets))

	sinaTargets := make([]quoteTarget, 0, len(targets))
	for _, target := range targets {
		if target.SinaCode != "" {
			sinaTargets = append(sinaTargets, target)
		}
	}

	sinaQuotes, sinaProblems := p.fetchSina(ctx, sinaTargets)
	for key, quote := range sinaQuotes {
		quotes[key] = quote
	}
	problems = append(problems, sinaProblems...)

	txTargets := make([]quoteTarget, 0, len(targets))
	for key, target := range targets {
		if _, ok := quotes[key]; ok {
			continue
		}
		if target.TXCode != "" {
			txTargets = append(txTargets, target)
			continue
		}
		problems = append(problems, fmt.Sprintf("没有可用行情代码: %s", target.DisplaySymbol))
	}

	txQuotes, txProblems := p.fetchTencent(ctx, txTargets)
	for key, quote := range txQuotes {
		quotes[key] = quote
	}
	problems = append(problems, txProblems...)

	for key, target := range targets {
		if _, ok := quotes[key]; ok {
			continue
		}
		problems = append(problems, fmt.Sprintf("未收到 %s 的实时行情", target.DisplaySymbol))
	}

	return quotes, collapseProblems(problems)
}

func NewYahooQuoteProvider(client *http.Client) *YahooQuoteProvider {
	if client == nil {
		client = &http.Client{Timeout: 8 * time.Second}
	}

	return &YahooQuoteProvider{client: client}
}

func (p *YahooQuoteProvider) Name() string {
	return "Yahoo Finance"
}

func (p *YahooQuoteProvider) Fetch(ctx context.Context, items []WatchlistItem) (map[string]Quote, error) {
	targets, problems := collectQuoteTargets(items)
	quotes := make(map[string]Quote, len(targets))
	if len(targets) == 0 {
		return quotes, collapseProblems(problems)
	}

	for _, item := range items {
		target, err := ResolveQuoteTarget(item)
		if err != nil {
			continue
		}

		yahooSymbol, err := resolveYahooSymbol(item)
		if err != nil {
			problems = append(problems, fmt.Sprintf("Yahoo 不支持该标的: %s", target.DisplaySymbol))
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

func (p *YahooQuoteProvider) fetchChartSnapshot(ctx context.Context, item WatchlistItem, yahooSymbol string) (Quote, error) {
	params := url.Values{}
	params.Set("range", "5d")
	params.Set("interval", "1d")
	params.Set("includePrePost", "false")
	params.Set("events", "div,splits")

	parsed, err := fetchYahooChart(ctx, p.client, yahooSymbol, params)
	if err != nil {
		return Quote{}, fmt.Errorf("Yahoo 行情请求失败: %w", err)
	}
	if len(parsed.Chart.Result) == 0 || len(parsed.Chart.Result[0].Indicators.Quote) == 0 {
		return Quote{}, errors.New("Yahoo 行情为空")
	}

	result := parsed.Chart.Result[0]
	points := buildHistoryPoints(result.Timestamp, result.Indicators.Quote[0])
	if len(points) == 0 {
		return Quote{}, errors.New("Yahoo 行情缺少有效价格点")
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

func NewEastMoneyQuoteProvider(client *http.Client) *EastMoneyQuoteProvider {
	if client == nil {
		client = &http.Client{Timeout: 8 * time.Second}
	}

	return &EastMoneyQuoteProvider{client: client}
}

func (p *EastMoneyQuoteProvider) Name() string {
	return "EastMoney"
}

func (p *EastMoneyQuoteProvider) Fetch(ctx context.Context, items []WatchlistItem) (map[string]Quote, error) {
	targets, problems := collectQuoteTargets(items)
	quotes := make(map[string]Quote, len(targets))
	if len(targets) == 0 {
		return quotes, collapseProblems(problems)
	}

	secids := make([]string, 0, len(targets))
	indexBySecID := make(map[string]quoteTarget, len(targets))
	for _, target := range targets {
		secid, err := resolveEastMoneySecID(target)
		if err != nil {
			problems = append(problems, err.Error())
			continue
		}
		secids = append(secids, secid)
		indexBySecID[secid] = target
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

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://push2.eastmoney.com/api/qt/ulist.np/get?"+params.Encode(), nil)
	if err != nil {
		return quotes, err
	}
	request.Header.Set("Referer", "https://quote.eastmoney.com/")
	request.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/123.0.0.0 Safari/537.36")

	response, err := p.client.Do(request)
	if err != nil {
		return quotes, err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return quotes, fmt.Errorf("东方财富行情请求失败: status %d", response.StatusCode)
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
		return quotes, fmt.Errorf("东方财富行情返回 rc=%d", parsed.RC)
	}

	for _, item := range parsed.Data.Diff {
		secid := fmt.Sprintf("%d.%s", item.MarketID, normaliseEastMoneyCode(item.Code, item.MarketID))
		target, ok := indexBySecID[secid]
		if !ok {
			continue
		}

		quote := buildQuote(
			item.Name,
			item.CurrentPrice,
			item.PreviousClose,
			item.OpenPrice,
			item.DayHigh,
			item.DayLow,
			time.Now(),
			p.Name(),
		)
		quote.Symbol = target.DisplaySymbol
		quote.Market = target.Market
		quote.Currency = target.Currency
		if item.ChangePercent != 0 {
			quote.ChangePercent = item.ChangePercent
		}
		if item.Change != 0 {
			quote.Change = item.Change
		}
		quotes[target.Key] = quote
	}

	for secid, target := range indexBySecID {
		if _, ok := quotes[target.Key]; ok {
			continue
		}
		problems = append(problems, fmt.Sprintf("未收到 %s 的东方财富行情 (%s)", target.DisplaySymbol, secid))
	}

	return quotes, collapseProblems(problems)
}

func resolveEastMoneySecID(target quoteTarget) (string, error) {
	symbol := target.DisplaySymbol
	switch {
	case strings.HasSuffix(symbol, ".SH"):
		return "1." + strings.TrimSuffix(symbol, ".SH"), nil
	case strings.HasSuffix(symbol, ".SZ"):
		return "0." + strings.TrimSuffix(symbol, ".SZ"), nil
	case strings.HasSuffix(symbol, ".HK"):
		return "116." + strings.TrimSuffix(symbol, ".HK"), nil
	case strings.HasSuffix(symbol, ".BJ"):
		return "", fmt.Errorf("东方财富暂不支持北交所实时行情: %s", symbol)
	case isLetters(symbol):
		return "105." + symbol, nil
	default:
		return "", fmt.Errorf("无法转换为东方财富行情代码: %s", symbol)
	}
}

func normaliseEastMoneyCode(code string, marketID int) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	switch marketID {
	case 116:
		if len(code) < 5 && isDigits(code) {
			return strings.Repeat("0", 5-len(code)) + code
		}
	case 0, 1:
		if len(code) < 6 && isDigits(code) {
			return strings.Repeat("0", 6-len(code)) + code
		}
	}
	return code
}

func firstNonEmptyFloat(left, right float64) float64 {
	if left > 0 {
		return left
	}
	return right
}
