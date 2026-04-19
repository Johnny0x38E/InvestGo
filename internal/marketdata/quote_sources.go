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

type SinaQuoteProvider struct {
	client *http.Client
}

type XueqiuQuoteProvider struct {
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

type xueqiuQuoteResponse struct {
	Data []struct {
		Symbol    string   `json:"symbol"`
		Name      string   `json:"name"`
		Current   *float64 `json:"current"`
		Percent   *float64 `json:"percent"`
		Chg       *float64 `json:"chg"`
		High      *float64 `json:"high"`
		Low       *float64 `json:"low"`
		Open      *float64 `json:"open"`
		LastClose *float64 `json:"last_close"`
		Timestamp *int64   `json:"timestamp"`
	} `json:"data"`
	ErrorCode        int    `json:"error_code"`
	ErrorDescription string `json:"error_description"`
}

// DefaultQuoteSourceRegistry returns the default quote source registry and its frontend display options.
func DefaultQuoteSourceRegistry(client *http.Client) (map[string]monitor.QuoteProvider, []monitor.QuoteSourceOption) {
	eastMoney := NewEastMoneyQuoteProvider(client)
	yahoo := NewYahooQuoteProvider(client)
	sina := NewSinaQuoteProvider(client)
	xueqiu := NewXueqiuQuoteProvider(client)

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
		{
			ID:               "sina",
			Name:             "Sina Finance",
			Description:      "Fast quote source exposed across China, Hong Kong, and US selections for direct comparison.",
			SupportedMarkets: []string{"CN-A", "CN-GEM", "CN-STAR", "CN-ETF", "HK-MAIN", "HK-GEM", "HK-ETF", "US-STOCK", "US-ETF"},
		},
		{
			ID:               "xueqiu",
			Name:             "Xueqiu",
			Description:      "Quote source exposed across China, Hong Kong, and US selections for direct comparison.",
			SupportedMarkets: []string{"CN-A", "CN-GEM", "CN-STAR", "CN-ETF", "HK-MAIN", "HK-GEM", "HK-ETF", "US-STOCK", "US-ETF"},
		},
	}

	return map[string]monitor.QuoteProvider{
		"eastmoney": eastMoney,
		"yahoo":     yahoo,
		"sina":      sina,
		"xueqiu":    xueqiu,
	}, options
}

// NewYahooQuoteProvider creates a Yahoo real-time quote provider.
func NewYahooQuoteProvider(client *http.Client) *YahooQuoteProvider {
	if client == nil {
		client = &http.Client{Timeout: 8 * time.Second}
	}

	return &YahooQuoteProvider{client: client}
}

func (p *YahooQuoteProvider) Name() string {
	return "Yahoo Finance"
}

// Fetch requests Yahoo Finance real-time quotes in batch and maps them to the standard Quote structure.
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

// fetchChartSnapshot calls the Yahoo Finance chart API, parses the last 5 days of daily data,
// and builds a Quote from the latest price point.
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

// NewEastMoneyQuoteProvider creates an EastMoney real-time quote provider.
func NewEastMoneyQuoteProvider(client *http.Client) *EastMoneyQuoteProvider {
	if client == nil {
		client = &http.Client{Timeout: 8 * time.Second}
	}

	return &EastMoneyQuoteProvider{client: client}
}

// Name returns the display name of the EastMoney quote source.
func (p *EastMoneyQuoteProvider) Name() string {
	return "EastMoney"
}

// NewSinaQuoteProvider creates a Sina real-time quote provider for China and Hong Kong markets.
func NewSinaQuoteProvider(client *http.Client) *SinaQuoteProvider {
	if client == nil {
		client = &http.Client{Timeout: 8 * time.Second}
	}
	return &SinaQuoteProvider{client: client}
}

func (p *SinaQuoteProvider) Name() string {
	return "Sina Finance"
}

// NewXueqiuQuoteProvider creates a Xueqiu real-time quote provider for China and Hong Kong markets.
func NewXueqiuQuoteProvider(client *http.Client) *XueqiuQuoteProvider {
	if client == nil {
		client = &http.Client{Timeout: 8 * time.Second}
	}
	return &XueqiuQuoteProvider{client: client}
}

func (p *XueqiuQuoteProvider) Name() string {
	return "Xueqiu"
}

// Fetch requests EastMoney real-time quotes in batch and maps them to the standard Quote structure.
func (p *EastMoneyQuoteProvider) Fetch(ctx context.Context, items []monitor.WatchlistItem) (map[string]monitor.Quote, error) {
	targets, problems := collectQuoteTargets(items)
	quotes := make(map[string]monitor.Quote, len(targets))
	if len(targets) == 0 {
		return quotes, collapseProblems(problems)
	}

	// EastMoney queries by secid in batch, so map standard targets to secids first.
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

// Fetch requests Sina real-time quotes for China and Hong Kong items.
func (p *SinaQuoteProvider) Fetch(ctx context.Context, items []monitor.WatchlistItem) (map[string]monitor.Quote, error) {
	targets, problems := collectQuoteTargets(items)
	quotes := make(map[string]monitor.Quote, len(targets))
	if len(targets) == 0 {
		return quotes, collapseProblems(problems)
	}

	itemByKey := make(map[string]monitor.WatchlistItem, len(targets))
	sinaCodes := make([]string, 0, len(targets))
	targetByCode := make(map[string]monitor.QuoteTarget, len(targets))
	for _, item := range items {
		target, err := monitor.ResolveQuoteTarget(item)
		if err != nil {
			continue
		}
		code, err := resolveSinaQuoteCode(target)
		if err != nil {
			problems = append(problems, err.Error())
			continue
		}
		itemByKey[target.Key] = item
		sinaCodes = append(sinaCodes, code)
		targetByCode[code] = target
	}

	if len(sinaCodes) == 0 {
		return quotes, collapseProblems(problems)
	}

	text, err := fetchTextWithHeaders(ctx, p.client, "https://hq.sinajs.cn/list="+strings.Join(sinaCodes, ","), map[string]string{
		"Referer":    sinaReferer,
		"User-Agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
	}, true)
	if err != nil {
		return quotes, err
	}

	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		code, fields, ok := parseSinaQuoteLine(line)
		if !ok {
			continue
		}
		target, ok := targetByCode[code]
		if !ok {
			continue
		}
		item := itemByKey[target.Key]
		quote, ok := buildSinaQuote(item, code, fields)
		if !ok {
			continue
		}
		quote.Symbol = target.DisplaySymbol
		quote.Market = target.Market
		quote.Currency = firstNonEmpty(quote.Currency, target.Currency)
		quotes[target.Key] = quote
	}

	if len(quotes) == 0 && len(problems) == 0 {
		problems = append(problems, "Sina quote response is empty")
	}
	return quotes, collapseProblems(problems)
}

// Fetch requests Xueqiu real-time quotes for China and Hong Kong items.
func (p *XueqiuQuoteProvider) Fetch(ctx context.Context, items []monitor.WatchlistItem) (map[string]monitor.Quote, error) {
	targets, problems := collectQuoteTargets(items)
	quotes := make(map[string]monitor.Quote, len(targets))
	if len(targets) == 0 {
		return quotes, collapseProblems(problems)
	}

	itemByKey := make(map[string]monitor.WatchlistItem, len(targets))
	xueqiuSymbols := make([]string, 0, len(targets))
	targetBySymbol := make(map[string]monitor.QuoteTarget, len(targets))
	for _, item := range items {
		target, err := monitor.ResolveQuoteTarget(item)
		if err != nil {
			continue
		}
		symbol, err := resolveXueqiuQuoteSymbol(target)
		if err != nil {
			problems = append(problems, err.Error())
			continue
		}
		itemByKey[target.Key] = item
		xueqiuSymbols = append(xueqiuSymbols, symbol)
		targetBySymbol[symbol] = target
	}

	if len(xueqiuSymbols) == 0 {
		return quotes, collapseProblems(problems)
	}

	params := url.Values{}
	params.Set("symbol", strings.Join(xueqiuSymbols, ","))
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://stock.xueqiu.com/v5/stock/realtime/quotec.json?"+params.Encode(), nil)
	if err != nil {
		return quotes, err
	}
	request.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
	request.Header.Set("Referer", "https://xueqiu.com/")
	request.Header.Set("Origin", "https://xueqiu.com")

	response, err := p.client.Do(request)
	if err != nil {
		return quotes, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return quotes, fmt.Errorf("Xueqiu quote request failed: status %d", response.StatusCode)
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return quotes, err
	}

	var parsed xueqiuQuoteResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return quotes, err
	}
	if parsed.ErrorCode != 0 {
		return quotes, fmt.Errorf("Xueqiu quote error %d: %s", parsed.ErrorCode, parsed.ErrorDescription)
	}

	for _, entry := range parsed.Data {
		target, ok := targetBySymbol[strings.TrimSpace(entry.Symbol)]
		if !ok {
			continue
		}
		item := itemByKey[target.Key]
		current := derefFloat64(entry.Current)
		previous := derefFloat64(entry.LastClose)
		open := derefFloat64(entry.Open)
		high := derefFloat64(entry.High)
		low := derefFloat64(entry.Low)
		updatedAt := time.Now()
		if entry.Timestamp != nil && *entry.Timestamp > 0 {
			updatedAt = time.UnixMilli(*entry.Timestamp)
		}
		quote := buildQuote(
			firstNonEmpty(entry.Name, item.Name, target.DisplaySymbol),
			current,
			previous,
			open,
			high,
			low,
			updatedAt,
			p.Name(),
		)
		quote.Symbol = target.DisplaySymbol
		quote.Market = target.Market
		quote.Currency = target.Currency
		if quote.CurrentPrice <= 0 {
			continue
		}
		quotes[target.Key] = quote
	}

	if len(quotes) == 0 && len(problems) == 0 {
		problems = append(problems, "Xueqiu quote response is empty")
	}
	return quotes, collapseProblems(problems)
}

func resolveSinaQuoteCode(target monitor.QuoteTarget) (string, error) {
	switch {
	case strings.HasSuffix(target.Key, ".SH"):
		return "sh" + strings.TrimSuffix(target.Key, ".SH"), nil
	case strings.HasSuffix(target.Key, ".SZ"):
		return "sz" + strings.TrimSuffix(target.Key, ".SZ"), nil
	case strings.HasSuffix(target.Key, ".BJ"):
		return "bj" + strings.TrimSuffix(target.Key, ".BJ"), nil
	case strings.HasSuffix(target.Key, ".HK"):
		return "rt_hk" + strings.TrimSuffix(target.Key, ".HK"), nil
	case target.Market == "US-STOCK" || target.Market == "US-ETF":
		return "gb_" + strings.ToLower(target.DisplaySymbol), nil
	default:
		return "", fmt.Errorf("Sina does not support item: %s", target.DisplaySymbol)
	}
}

func resolveXueqiuQuoteSymbol(target monitor.QuoteTarget) (string, error) {
	switch {
	case strings.HasSuffix(target.Key, ".SH"):
		return "SH" + strings.TrimSuffix(target.Key, ".SH"), nil
	case strings.HasSuffix(target.Key, ".SZ"):
		return "SZ" + strings.TrimSuffix(target.Key, ".SZ"), nil
	case strings.HasSuffix(target.Key, ".BJ"):
		return "", fmt.Errorf("Xueqiu does not support item: %s", target.DisplaySymbol)
	case strings.HasSuffix(target.Key, ".HK"):
		return "HK" + strings.TrimSuffix(target.Key, ".HK"), nil
	case target.Market == "US-STOCK" || target.Market == "US-ETF":
		return target.DisplaySymbol, nil
	default:
		return "", fmt.Errorf("Xueqiu does not support item: %s", target.DisplaySymbol)
	}
}

func parseSinaQuoteLine(line string) (string, []string, bool) {
	const prefix = "var hq_str_"
	if !strings.HasPrefix(line, prefix) {
		return "", nil, false
	}
	eq := strings.Index(line, "=")
	if eq <= len(prefix) {
		return "", nil, false
	}
	code := strings.TrimSpace(line[len(prefix):eq])
	raw := strings.TrimSpace(strings.TrimSuffix(line[eq+1:], ";"))
	raw = strings.Trim(raw, "\"")
	if raw == "" {
		return code, nil, false
	}
	return code, strings.Split(raw, ","), true
}

func buildSinaQuote(item monitor.WatchlistItem, code string, fields []string) (monitor.Quote, bool) {
	switch {
	case strings.HasPrefix(code, "sh") || strings.HasPrefix(code, "sz") || strings.HasPrefix(code, "bj"):
		if len(fields) < 6 {
			return monitor.Quote{}, false
		}
		name := partsAt(fields, 0)
		open := parseFloat(partsAt(fields, 1))
		previous := parseFloat(partsAt(fields, 2))
		current := parseFloat(partsAt(fields, 3))
		high := parseFloat(partsAt(fields, 4))
		low := parseFloat(partsAt(fields, 5))
		quote := buildQuote(firstNonEmpty(name, item.Name, item.Symbol), current, previous, open, high, low, time.Now(), "Sina Finance")
		quote.Currency = firstNonEmpty(item.Currency, "CNY")
		return quote, quote.CurrentPrice > 0
	case strings.HasPrefix(code, "rt_hk"):
		if len(fields) < 7 {
			return monitor.Quote{}, false
		}
		name := partsAt(fields, 1)
		open := parseFloat(partsAt(fields, 2))
		previous := parseFloat(partsAt(fields, 3))
		current := parseFloat(partsAt(fields, 6))
		high := parseFloat(partsAt(fields, 4))
		low := parseFloat(partsAt(fields, 5))
		updatedAt := parseTimestamp(partsAt(fields, 17) + " " + partsAt(fields, 18))
		if updatedAt.IsZero() {
			updatedAt = time.Now()
		}
		quote := buildQuote(firstNonEmpty(name, item.Name, item.Symbol), current, previous, open, high, low, updatedAt, "Sina Finance")
		quote.Currency = firstNonEmpty(item.Currency, "HKD")
		return quote, quote.CurrentPrice > 0
	case strings.HasPrefix(code, "gb_"):
		if len(fields) < 6 {
			return monitor.Quote{}, false
		}
		name := partsAt(fields, 0)
		current := parseFloat(partsAt(fields, 1))
		change := parseFloat(partsAt(fields, 2))
		changePercent := parseFloat(partsAt(fields, 3))
		previous := current - change
		open := parseFloat(partsAt(fields, 5))
		high := parseFloat(partsAt(fields, 6))
		low := parseFloat(partsAt(fields, 7))
		quote := buildQuote(firstNonEmpty(name, item.Name, item.Symbol), current, previous, open, high, low, time.Now(), "Sina Finance")
		quote.Change = change
		quote.ChangePercent = changePercent
		quote.Currency = firstNonEmpty(item.Currency, "USD")
		return quote, quote.CurrentPrice > 0
	default:
		return monitor.Quote{}, false
	}
}

// resolveEastMoneySecID converts a standard target to the secid required by the EastMoney API (returns the first match).
// Note: US stocks default to 105 (NASDAQ), but may actually be on 106 (NYSE) or 107 (NYSE Arca).
// For batch quote requests, prefer resolveAllEastMoneySecIDs to cover all US exchanges.
func resolveEastMoneySecID(target monitor.QuoteTarget) (string, error) {
	ids, err := resolveAllEastMoneySecIDs(target)
	if err != nil {
		return "", err
	}
	return ids[0], nil
}

// resolveAllEastMoneySecIDs converts a standard target to all possible secids required by the EastMoney API.
// For US stocks, since the same ticker may list on NASDAQ (105), NYSE (106), or NYSE Arca (107),
// all three variants are returned to ensure the correct exchange is hit.
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
		// 105=NASDAQ, 106=NYSE, 107=NYSE Arca — request all three to cover every exchange
		return []string{"105." + ticker, "106." + ticker, "107." + ticker}, nil
	default:
		return nil, fmt.Errorf("Market type is unsupported: %s", market)
	}
}
