package monitor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"
)

const hotPageSize = 20

type HotService struct {
	client        *http.Client
	mu            sync.RWMutex
	universeCache map[HotCategory]cachedHotUniverse
	quoteCache    map[HotCategory]cachedHotQuotes
}

type hotSeed struct {
	Symbol   string
	Name     string
	Market   string
	Currency string
}

type cachedHotUniverse struct {
	items     []hotSeed
	expiresAt time.Time
}

type cachedHotQuotes struct {
	items     []HotItem
	expiresAt time.Time
}

type hotUniverseSource struct {
	url           string
	symbolHeaders []string
	nameHeaders   []string
	market        string
	currency      string
}

type eastMoneyHotResponse struct {
	RC   int `json:"rc"`
	Data struct {
		Total int `json:"total"`
		Diff  []struct {
			MarketID      int     `json:"f13"`
			Code          string  `json:"f12"`
			Name          string  `json:"f14"`
			CurrentPrice  float64 `json:"f2"`
			ChangePercent float64 `json:"f3"`
			Change        float64 `json:"f4"`
			Volume        float64 `json:"f5"`
			MarketCap     float64 `json:"f20"`
		} `json:"diff"`
	} `json:"data"`
}

func NewHotService(client *http.Client) *HotService {
	if client == nil {
		client = &http.Client{Timeout: 12 * time.Second}
	}

	return &HotService{
		client:        client,
		universeCache: make(map[HotCategory]cachedHotUniverse),
		quoteCache:    make(map[HotCategory]cachedHotQuotes),
	}
}

// List 对外提供统一的热门列表接口：
// 不同市场的数据抓取策略不同，但分页、排序和响应结构在这里收口。
func (s *HotService) List(ctx context.Context, category HotCategory, sortBy HotSort, page, pageSize int) (HotListResponse, error) {
	category, err := normaliseHotCategory(category)
	if err != nil {
		return HotListResponse{}, err
	}
	sortBy = normaliseHotSort(sortBy)
	page = maxInt(page, 1)
	pageSize = hotPageSize

	if category == HotCategoryHKMain {
		return s.listHK(ctx, sortBy, page, pageSize)
	}

	items, err := s.snapshotForCategory(ctx, category)
	if err != nil {
		return HotListResponse{}, err
	}

	sorted := append([]HotItem(nil), items...)
	sortHotItems(sorted, sortBy)

	start := (page - 1) * pageSize
	if start > len(sorted) {
		start = len(sorted)
	}
	end := minInt(start+pageSize, len(sorted))

	return HotListResponse{
		Category:    category,
		Sort:        sortBy,
		Page:        page,
		PageSize:    pageSize,
		Total:       len(sorted),
		HasMore:     end < len(sorted),
		Items:       sorted[start:end],
		GeneratedAt: time.Now(),
	}, nil
}

func (s *HotService) snapshotForCategory(ctx context.Context, category HotCategory) ([]HotItem, error) {
	s.mu.RLock()
	if cached, ok := s.quoteCache[category]; ok && time.Now().Before(cached.expiresAt) {
		items := append([]HotItem(nil), cached.items...)
		s.mu.RUnlock()
		return items, nil
	}
	s.mu.RUnlock()

	seeds, err := s.loadSeeds(ctx, category)
	if err != nil {
		return nil, err
	}

	items, err := s.fetchUniverseQuotes(ctx, seeds)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	s.quoteCache[category] = cachedHotQuotes{
		items:     append([]HotItem(nil), items...),
		expiresAt: time.Now().Add(3 * time.Minute),
	}
	s.mu.Unlock()

	return append([]HotItem(nil), items...), nil
}

// loadSeeds 会优先读取静态种子，再回退到远端成分表抓取，并把结果做缓存。
func (s *HotService) loadSeeds(ctx context.Context, category HotCategory) ([]hotSeed, error) {
	if seeds := staticHotSeeds(category); len(seeds) > 0 {
		return seeds, nil
	}

	s.mu.RLock()
	if cached, ok := s.universeCache[category]; ok && time.Now().Before(cached.expiresAt) {
		items := append([]hotSeed(nil), cached.items...)
		s.mu.RUnlock()
		return items, nil
	}
	s.mu.RUnlock()

	source, ok := hotUniverseSources[category]
	if !ok {
		return nil, fmt.Errorf("不支持的热门分类: %s", category)
	}

	seeds, err := s.fetchWikipediaUniverse(ctx, source)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	s.universeCache[category] = cachedHotUniverse{
		items:     append([]hotSeed(nil), seeds...),
		expiresAt: time.Now().Add(12 * time.Hour),
	}
	s.mu.Unlock()

	return append([]hotSeed(nil), seeds...), nil
}

func (s *HotService) fetchWikipediaUniverse(ctx context.Context, source hotUniverseSource) ([]hotSeed, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, source.url, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/123.0.0.0 Safari/537.36")

	response, err := s.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("热门成分股请求失败: status %d", response.StatusCode)
	}

	document, err := html.Parse(response.Body)
	if err != nil {
		return nil, err
	}

	seeds := parseUniverseTable(document, source.symbolHeaders, source.nameHeaders, source.market, source.currency)
	if len(seeds) == 0 {
		return nil, errors.New("未解析到热门成分股列表")
	}

	return dedupeHotSeeds(seeds), nil
}

func parseUniverseTable(root *html.Node, symbolHeaders, nameHeaders []string, market, currency string) []hotSeed {
	for _, table := range findElements(root, "table") {
		rows := findElements(table, "tr")
		if len(rows) < 2 {
			continue
		}

		headers := collectRowValues(rows[0], "th")
		if len(headers) == 0 {
			headers = collectRowValues(rows[0], "td")
		}
		symbolIndex := findHeaderIndex(headers, symbolHeaders)
		nameIndex := findHeaderIndex(headers, nameHeaders)
		if symbolIndex < 0 || nameIndex < 0 {
			continue
		}

		seeds := make([]hotSeed, 0, len(rows)-1)
		for _, row := range rows[1:] {
			cells := collectRowValues(row, "td")
			if len(cells) <= maxInt(symbolIndex, nameIndex) {
				continue
			}

			symbol := normaliseHotSymbol(cleanNodeText(cells[symbolIndex]), market)
			name := cleanNodeText(cells[nameIndex])
			if symbol == "" || name == "" {
				continue
			}

			seeds = append(seeds, hotSeed{
				Symbol:   symbol,
				Name:     name,
				Market:   market,
				Currency: currency,
			})
		}

		if len(seeds) > 0 {
			return seeds
		}
	}

	return nil
}

// findElements 会递归地在一个 HTML 节点树中查找所有指定标签的元素，并返回它们的节点列表。
func findElements(root *html.Node, tag string) []*html.Node {
	var result []*html.Node
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode && strings.EqualFold(node.Data, tag) {
			result = append(result, node)
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	return result
}

// collectRowValues 会收集一个表格行节点中所有指定标签的子节点，通常用于提取表头或单元格数据。
func collectRowValues(row *html.Node, tag string) []*html.Node {
	values := make([]*html.Node, 0, 8)
	for child := row.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode && strings.EqualFold(child.Data, tag) {
			values = append(values, child)
		}
	}
	return values
}

// findHeaderIndex 会在给定的表头节点列表中查找包含任一候选字符串的节点，并返回其索引，找不到则返回 -1。
func findHeaderIndex(headers []*html.Node, wanted []string) int {
	for index, header := range headers {
		text := cleanNodeText(header)
		for _, candidate := range wanted {
			if strings.Contains(text, candidate) {
				return index
			}
		}
	}
	return -1
}

// cleanNodeText 会递归地把一个 HTML 节点及其子节点中的文本内容提取出来，并做基本的清洗（去除多余空白、合并连续空格）。
func cleanNodeText(node *html.Node) string {
	var builder strings.Builder
	var walk func(*html.Node)
	walk = func(current *html.Node) {
		if current.Type == html.TextNode {
			builder.WriteString(current.Data)
			builder.WriteByte(' ')
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return strings.Join(strings.Fields(strings.TrimSpace(builder.String())), " ")
}

// normaliseHotSymbol 会把抓取到的原始代码转换为可用于请求行情的标准代码，主要处理空格、大小写、特殊符号和美股的点横线问题。
func normaliseHotSymbol(raw, market string) string {
	symbol := strings.ToUpper(strings.TrimSpace(strings.ReplaceAll(raw, "\u00a0", " ")))
	symbol = strings.Join(strings.Fields(symbol), "")
	symbol = strings.TrimSuffix(symbol, "*")
	if market == "US" || market == "US ETF" {
		symbol = strings.ReplaceAll(symbol, ".", "-")
	}
	return symbol
}

func dedupeHotSeeds(items []hotSeed) []hotSeed {
	seen := make(map[string]struct{}, len(items))
	result := make([]hotSeed, 0, len(items))
	for _, item := range items {
		key := item.Symbol + "|" + item.Market
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, item)
	}
	return result
}

// fetchUniverseQuotes 会根据成分股列表批量请求行情数据，针对美股会做多代码尝试以提高成功率，并且会去重和过滤掉请求失败的项。
func (s *HotService) fetchUniverseQuotes(ctx context.Context, seeds []hotSeed) ([]HotItem, error) {
	indexBySecID := make(map[string]hotSeed, len(seeds)*2)
	secids := make([]string, 0, len(seeds)*2)
	for _, seed := range seeds {
		candidates, err := resolveHotSecIDs(seed)
		if err != nil {
			continue
		}
		for _, secid := range candidates {
			if _, ok := indexBySecID[secid]; ok {
				continue
			}
			indexBySecID[secid] = seed
			secids = append(secids, secid)
		}
	}

	if len(secids) == 0 {
		return nil, errors.New("热门列表没有可请求的行情代码")
	}

	items := make([]HotItem, 0, len(seeds))
	seenSymbols := make(map[string]struct{}, len(seeds))
	for start := 0; start < len(secids); start += 240 {
		end := minInt(start+240, len(secids))
		batch, err := s.fetchEastMoneyBatch(ctx, secids[start:end], indexBySecID, seenSymbols)
		if err != nil {
			return nil, err
		}
		items = append(items, batch...)
	}

	if len(items) == 0 {
		return nil, errors.New("未拿到热门行情数据")
	}

	return items, nil
}

// fetchEastMoneyBatch 会针对一批 secid 请求东方财富的批量行情接口，并把返回的数据转换为 HotItem 列表，
// 过程中会根据 secid 找到对应的种子信息，并且过滤掉重复的股票（同一只股票可能有多个 secid）。
func (s *HotService) fetchEastMoneyBatch(ctx context.Context, secids []string, indexBySecID map[string]hotSeed, seenSymbols map[string]struct{}) ([]HotItem, error) {
	params := url.Values{}
	params.Set("fltt", "2")
	params.Set("invt", "2")
	params.Set("np", "1")
	params.Set("ut", "bd1d9ddb04089700cf9c27f6f7426281")
	params.Set("fields", "f2,f3,f4,f5,f12,f13,f14,f20")
	params.Set("secids", strings.Join(secids, ","))

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://push2.eastmoney.com/api/qt/ulist.np/get?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Referer", "https://quote.eastmoney.com/")
	request.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/123.0.0.0 Safari/537.36")

	response, err := s.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("热门行情请求失败: status %d", response.StatusCode)
	}

	payload, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}

	var parsed eastMoneyHotResponse
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return nil, err
	}
	if parsed.RC != 0 {
		return nil, fmt.Errorf("热门行情返回 rc=%d", parsed.RC)
	}

	result := make([]HotItem, 0, len(parsed.Data.Diff))
	for _, item := range parsed.Data.Diff {
		secid := fmt.Sprintf("%d.%s", item.MarketID, normaliseEastMoneyCode(item.Code, item.MarketID))
		seed, ok := indexBySecID[secid]
		if !ok {
			continue
		}
		if _, exists := seenSymbols[seed.Symbol]; exists {
			continue
		}

		seenSymbols[seed.Symbol] = struct{}{}
		result = append(result, HotItem{
			Symbol:        seed.Symbol,
			Name:          firstNonEmpty(item.Name, seed.Name),
			Market:        seed.Market,
			Currency:      seed.Currency,
			CurrentPrice:  item.CurrentPrice,
			Change:        item.Change,
			ChangePercent: item.ChangePercent,
			Volume:        item.Volume,
			MarketCap:     item.MarketCap,
			QuoteSource:   "EastMoney",
			UpdatedAt:     time.Now(),
		})
	}

	return result, nil
}

// listHK 专门处理港股热门的请求，因为港股没有公开的成分表，直接通过东方财富的热门接口来获取，
func (s *HotService) listHK(ctx context.Context, sortBy HotSort, page, pageSize int) (HotListResponse, error) {
	fid, po := resolveEastMoneySort(sortBy)

	params := url.Values{}
	params.Set("pn", strconv.Itoa(page))
	params.Set("pz", strconv.Itoa(pageSize))
	params.Set("po", strconv.Itoa(po))
	params.Set("np", "1")
	params.Set("fltt", "2")
	params.Set("invt", "2")
	params.Set("ut", "bd1d9ddb04089700cf9c27f6f7426281")
	params.Set("fid", fid)
	params.Set("fs", "m:128 t:1,m:128 t:2,m:128 t:3,m:128 t:4")
	params.Set("fields", "f2,f3,f4,f5,f12,f13,f14,f20")

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://push2.eastmoney.com/api/qt/clist/get?"+params.Encode(), nil)
	if err != nil {
		return HotListResponse{}, err
	}
	request.Header.Set("Referer", "https://quote.eastmoney.com/")
	request.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/123.0.0.0 Safari/537.36")

	response, err := s.client.Do(request)
	if err != nil {
		return HotListResponse{}, err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return HotListResponse{}, fmt.Errorf("港股热门请求失败: status %d", response.StatusCode)
	}

	payload, err := io.ReadAll(response.Body)
	if err != nil {
		return HotListResponse{}, err
	}

	var parsed eastMoneyHotResponse
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return HotListResponse{}, err
	}
	if parsed.RC != 0 {
		return HotListResponse{}, fmt.Errorf("港股热门返回 rc=%d", parsed.RC)
	}

	items := make([]HotItem, 0, len(parsed.Data.Diff))
	for _, item := range parsed.Data.Diff {
		code := normaliseEastMoneyCode(item.Code, item.MarketID)
		items = append(items, HotItem{
			Symbol:        strings.ToUpper(code + ".HK"),
			Name:          item.Name,
			Market:        "HK",
			Currency:      "HKD",
			CurrentPrice:  item.CurrentPrice,
			Change:        item.Change,
			ChangePercent: item.ChangePercent,
			Volume:        item.Volume,
			MarketCap:     item.MarketCap,
			QuoteSource:   "EastMoney",
			UpdatedAt:     time.Now(),
		})
	}

	return HotListResponse{
		Category:    HotCategoryHKMain,
		Sort:        sortBy,
		Page:        page,
		PageSize:    pageSize,
		Total:       parsed.Data.Total,
		HasMore:     page*pageSize < parsed.Data.Total,
		Items:       items,
		GeneratedAt: time.Now(),
	}, nil
}

// resolveHotSecIDs 会根据种子信息生成一个或多个可能的 secid，以提高后续请求行情的成功率，特别是针对美股会有多个代码变体。
func resolveHotSecIDs(seed hotSeed) ([]string, error) {
	target, err := resolveQuoteTarget(seed.Symbol, seed.Market, seed.Currency)
	if err != nil {
		return nil, err
	}

	if target.Market == "US" || target.Market == "US ETF" {
		var secids []string
		for _, symbol := range hotUSSymbolVariants(target.DisplaySymbol) {
			for _, marketID := range []int{105, 106, 107} {
				secids = append(secids, fmt.Sprintf("%d.%s", marketID, symbol))
			}
		}
		return uniqStrings(secids), nil
	}

	secid, err := resolveEastMoneySecID(target)
	if err != nil {
		return nil, err
	}

	return []string{secid}, nil
}

// hotUSSymbolVariants 会针对美股代码生成一些常见的变体，主要是处理点和横线的替换，因为不同的数据源可能使用不同的格式。
func hotUSSymbolVariants(symbol string) []string {
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	variants := []string{symbol}
	if strings.Contains(symbol, "-") {
		variants = append(variants, strings.ReplaceAll(symbol, "-", "."))
	}
	if strings.Contains(symbol, ".") {
		variants = append(variants, strings.ReplaceAll(symbol, ".", "-"))
	}
	return uniqStrings(variants)
}

func uniqStrings(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	result := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	return result
}

func resolveEastMoneySort(sortBy HotSort) (string, int) {
	switch sortBy {
	case HotSortGainers:
		return "f3", 1
	case HotSortLosers:
		return "f3", 0
	case HotSortMarketCap:
		return "f20", 1
	case HotSortPrice:
		return "f2", 1
	default:
		return "f5", 1
	}
}

func normaliseHotCategory(category HotCategory) (HotCategory, error) {
	switch category {
	case "", HotCategoryUSSP500:
		return HotCategoryUSSP500, nil
	case HotCategoryUSNasdaq, HotCategoryUSDow, HotCategoryETFBroad, HotCategoryETFSector, HotCategoryETFIncome, HotCategoryHKMain:
		return category, nil
	default:
		return "", fmt.Errorf("不支持的热门分类: %s", category)
	}
}

func normaliseHotSort(sortBy HotSort) HotSort {
	switch sortBy {
	case HotSortGainers, HotSortLosers, HotSortMarketCap, HotSortPrice:
		return sortBy
	default:
		return HotSortVolume
	}
}

func sortHotItems(items []HotItem, sortBy HotSort) {
	sort.SliceStable(items, func(left, right int) bool {
		a := items[left]
		b := items[right]
		switch sortBy {
		case HotSortGainers:
			if a.ChangePercent != b.ChangePercent {
				return a.ChangePercent > b.ChangePercent
			}
		case HotSortLosers:
			if a.ChangePercent != b.ChangePercent {
				return a.ChangePercent < b.ChangePercent
			}
		case HotSortMarketCap:
			if a.MarketCap != b.MarketCap {
				return a.MarketCap > b.MarketCap
			}
		case HotSortPrice:
			if a.CurrentPrice != b.CurrentPrice {
				return a.CurrentPrice > b.CurrentPrice
			}
		default:
			if a.Volume != b.Volume {
				return a.Volume > b.Volume
			}
		}

		if a.Volume != b.Volume {
			return a.Volume > b.Volume
		}
		return a.Symbol < b.Symbol
	})
}

var hotUniverseSources = map[HotCategory]hotUniverseSource{
	HotCategoryUSSP500: {
		url:           "https://en.wikipedia.org/wiki/List_of_S%26P_500_companies",
		symbolHeaders: []string{"ticker symbol", "symbol", "ticker"},
		nameHeaders:   []string{"security", "company"},
		market:        "US",
		currency:      "USD",
	},
	HotCategoryUSNasdaq: {
		url:           "https://en.wikipedia.org/wiki/Nasdaq-100",
		symbolHeaders: []string{"ticker", "ticker symbol", "symbol"},
		nameHeaders:   []string{"company", "security"},
		market:        "US",
		currency:      "USD",
	},
	HotCategoryUSDow: {
		url:           "https://en.wikipedia.org/wiki/Dow_Jones_Industrial_Average",
		symbolHeaders: []string{"symbol", "ticker", "ticker symbol"},
		nameHeaders:   []string{"company", "security"},
		market:        "US",
		currency:      "USD",
	},
}

func staticHotSeeds(category HotCategory) []hotSeed {
	switch category {
	case HotCategoryUSSP500:
		return []hotSeed{
			{Symbol: "AAPL", Name: "Apple", Market: "US", Currency: "USD"},
			{Symbol: "MSFT", Name: "Microsoft", Market: "US", Currency: "USD"},
			{Symbol: "NVDA", Name: "NVIDIA", Market: "US", Currency: "USD"},
			{Symbol: "AMZN", Name: "Amazon", Market: "US", Currency: "USD"},
			{Symbol: "GOOGL", Name: "Alphabet Class A", Market: "US", Currency: "USD"},
			{Symbol: "META", Name: "Meta Platforms", Market: "US", Currency: "USD"},
			{Symbol: "BRK-B", Name: "Berkshire Hathaway Class B", Market: "US", Currency: "USD"},
			{Symbol: "JPM", Name: "JPMorgan Chase", Market: "US", Currency: "USD"},
			{Symbol: "V", Name: "Visa", Market: "US", Currency: "USD"},
			{Symbol: "MA", Name: "Mastercard", Market: "US", Currency: "USD"},
			{Symbol: "LLY", Name: "Eli Lilly", Market: "US", Currency: "USD"},
			{Symbol: "WMT", Name: "Walmart", Market: "US", Currency: "USD"},
			{Symbol: "COST", Name: "Costco Wholesale", Market: "US", Currency: "USD"},
			{Symbol: "PG", Name: "Procter & Gamble", Market: "US", Currency: "USD"},
			{Symbol: "UNH", Name: "UnitedHealth Group", Market: "US", Currency: "USD"},
			{Symbol: "XOM", Name: "Exxon Mobil", Market: "US", Currency: "USD"},
			{Symbol: "JNJ", Name: "Johnson & Johnson", Market: "US", Currency: "USD"},
			{Symbol: "HD", Name: "Home Depot", Market: "US", Currency: "USD"},
			{Symbol: "ABBV", Name: "AbbVie", Market: "US", Currency: "USD"},
			{Symbol: "BAC", Name: "Bank of America", Market: "US", Currency: "USD"},
			{Symbol: "KO", Name: "Coca-Cola", Market: "US", Currency: "USD"},
			{Symbol: "CVX", Name: "Chevron", Market: "US", Currency: "USD"},
			{Symbol: "AVGO", Name: "Broadcom", Market: "US", Currency: "USD"},
			{Symbol: "ORCL", Name: "Oracle", Market: "US", Currency: "USD"},
			{Symbol: "CSCO", Name: "Cisco", Market: "US", Currency: "USD"},
			{Symbol: "CRM", Name: "Salesforce", Market: "US", Currency: "USD"},
			{Symbol: "NFLX", Name: "Netflix", Market: "US", Currency: "USD"},
			{Symbol: "AMD", Name: "AMD", Market: "US", Currency: "USD"},
			{Symbol: "QCOM", Name: "Qualcomm", Market: "US", Currency: "USD"},
			{Symbol: "PEP", Name: "PepsiCo", Market: "US", Currency: "USD"},
		}
	case HotCategoryUSNasdaq:
		return []hotSeed{
			{Symbol: "AAPL", Name: "Apple", Market: "US", Currency: "USD"},
			{Symbol: "MSFT", Name: "Microsoft", Market: "US", Currency: "USD"},
			{Symbol: "NVDA", Name: "NVIDIA", Market: "US", Currency: "USD"},
			{Symbol: "AMZN", Name: "Amazon", Market: "US", Currency: "USD"},
			{Symbol: "META", Name: "Meta Platforms", Market: "US", Currency: "USD"},
			{Symbol: "GOOGL", Name: "Alphabet Class A", Market: "US", Currency: "USD"},
			{Symbol: "TSLA", Name: "Tesla", Market: "US", Currency: "USD"},
			{Symbol: "AVGO", Name: "Broadcom", Market: "US", Currency: "USD"},
			{Symbol: "NFLX", Name: "Netflix", Market: "US", Currency: "USD"},
			{Symbol: "AMD", Name: "AMD", Market: "US", Currency: "USD"},
			{Symbol: "ADBE", Name: "Adobe", Market: "US", Currency: "USD"},
			{Symbol: "COST", Name: "Costco Wholesale", Market: "US", Currency: "USD"},
			{Symbol: "PDD", Name: "PDD Holdings", Market: "US", Currency: "USD"},
			{Symbol: "QCOM", Name: "Qualcomm", Market: "US", Currency: "USD"},
			{Symbol: "AMGN", Name: "Amgen", Market: "US", Currency: "USD"},
			{Symbol: "CSCO", Name: "Cisco", Market: "US", Currency: "USD"},
			{Symbol: "INTU", Name: "Intuit", Market: "US", Currency: "USD"},
			{Symbol: "TXN", Name: "Texas Instruments", Market: "US", Currency: "USD"},
			{Symbol: "INTC", Name: "Intel", Market: "US", Currency: "USD"},
			{Symbol: "ADP", Name: "Automatic Data Processing", Market: "US", Currency: "USD"},
			{Symbol: "PANW", Name: "Palo Alto Networks", Market: "US", Currency: "USD"},
			{Symbol: "CMCSA", Name: "Comcast", Market: "US", Currency: "USD"},
			{Symbol: "AMAT", Name: "Applied Materials", Market: "US", Currency: "USD"},
			{Symbol: "MU", Name: "Micron Technology", Market: "US", Currency: "USD"},
			{Symbol: "BKNG", Name: "Booking Holdings", Market: "US", Currency: "USD"},
			{Symbol: "LRCX", Name: "Lam Research", Market: "US", Currency: "USD"},
			{Symbol: "KLAC", Name: "KLA", Market: "US", Currency: "USD"},
			{Symbol: "MELI", Name: "MercadoLibre", Market: "US", Currency: "USD"},
			{Symbol: "ASML", Name: "ASML Holding", Market: "US", Currency: "USD"},
			{Symbol: "CRWD", Name: "CrowdStrike", Market: "US", Currency: "USD"},
		}
	case HotCategoryUSDow:
		return []hotSeed{
			{Symbol: "AAPL", Name: "Apple", Market: "US", Currency: "USD"},
			{Symbol: "AMGN", Name: "Amgen", Market: "US", Currency: "USD"},
			{Symbol: "AXP", Name: "American Express", Market: "US", Currency: "USD"},
			{Symbol: "BA", Name: "Boeing", Market: "US", Currency: "USD"},
			{Symbol: "CAT", Name: "Caterpillar", Market: "US", Currency: "USD"},
			{Symbol: "CRM", Name: "Salesforce", Market: "US", Currency: "USD"},
			{Symbol: "CSCO", Name: "Cisco", Market: "US", Currency: "USD"},
			{Symbol: "CVX", Name: "Chevron", Market: "US", Currency: "USD"},
			{Symbol: "DIS", Name: "Walt Disney", Market: "US", Currency: "USD"},
			{Symbol: "GS", Name: "Goldman Sachs", Market: "US", Currency: "USD"},
			{Symbol: "HD", Name: "Home Depot", Market: "US", Currency: "USD"},
			{Symbol: "HON", Name: "Honeywell", Market: "US", Currency: "USD"},
			{Symbol: "IBM", Name: "IBM", Market: "US", Currency: "USD"},
			{Symbol: "INTC", Name: "Intel", Market: "US", Currency: "USD"},
			{Symbol: "JNJ", Name: "Johnson & Johnson", Market: "US", Currency: "USD"},
			{Symbol: "JPM", Name: "JPMorgan Chase", Market: "US", Currency: "USD"},
			{Symbol: "KO", Name: "Coca-Cola", Market: "US", Currency: "USD"},
			{Symbol: "MCD", Name: "McDonald's", Market: "US", Currency: "USD"},
			{Symbol: "MMM", Name: "3M", Market: "US", Currency: "USD"},
			{Symbol: "MRK", Name: "Merck", Market: "US", Currency: "USD"},
			{Symbol: "MSFT", Name: "Microsoft", Market: "US", Currency: "USD"},
			{Symbol: "NKE", Name: "Nike", Market: "US", Currency: "USD"},
			{Symbol: "PG", Name: "Procter & Gamble", Market: "US", Currency: "USD"},
			{Symbol: "TRV", Name: "Travelers", Market: "US", Currency: "USD"},
			{Symbol: "UNH", Name: "UnitedHealth Group", Market: "US", Currency: "USD"},
			{Symbol: "V", Name: "Visa", Market: "US", Currency: "USD"},
			{Symbol: "VZ", Name: "Verizon", Market: "US", Currency: "USD"},
			{Symbol: "WBA", Name: "Walgreens Boots Alliance", Market: "US", Currency: "USD"},
			{Symbol: "WMT", Name: "Walmart", Market: "US", Currency: "USD"},
			{Symbol: "DOW", Name: "Dow", Market: "US", Currency: "USD"},
		}
	case HotCategoryETFBroad:
		return []hotSeed{
			{Symbol: "SPY", Name: "SPDR S&P 500 ETF Trust", Market: "US ETF", Currency: "USD"},
			{Symbol: "IVV", Name: "iShares Core S&P 500 ETF", Market: "US ETF", Currency: "USD"},
			{Symbol: "VOO", Name: "Vanguard S&P 500 ETF", Market: "US ETF", Currency: "USD"},
			{Symbol: "QQQ", Name: "Invesco QQQ Trust", Market: "US ETF", Currency: "USD"},
			{Symbol: "VTI", Name: "Vanguard Total Stock Market ETF", Market: "US ETF", Currency: "USD"},
			{Symbol: "VT", Name: "Vanguard Total World Stock ETF", Market: "US ETF", Currency: "USD"},
			{Symbol: "DIA", Name: "SPDR Dow Jones Industrial Average ETF", Market: "US ETF", Currency: "USD"},
			{Symbol: "IWM", Name: "iShares Russell 2000 ETF", Market: "US ETF", Currency: "USD"},
			{Symbol: "VEA", Name: "Vanguard FTSE Developed Markets ETF", Market: "US ETF", Currency: "USD"},
			{Symbol: "VWO", Name: "Vanguard FTSE Emerging Markets ETF", Market: "US ETF", Currency: "USD"},
			{Symbol: "EFA", Name: "iShares MSCI EAFE ETF", Market: "US ETF", Currency: "USD"},
			{Symbol: "EEM", Name: "iShares MSCI Emerging Markets ETF", Market: "US ETF", Currency: "USD"},
			{Symbol: "SCHB", Name: "Schwab U.S. Broad Market ETF", Market: "US ETF", Currency: "USD"},
			{Symbol: "SPLG", Name: "SPDR Portfolio S&P 500 ETF", Market: "US ETF", Currency: "USD"},
			{Symbol: "ITOT", Name: "iShares Core S&P Total U.S. Stock Market ETF", Market: "US ETF", Currency: "USD"},
			{Symbol: "ACWI", Name: "iShares MSCI ACWI ETF", Market: "US ETF", Currency: "USD"},
			{Symbol: "VXUS", Name: "Vanguard Total International Stock ETF", Market: "US ETF", Currency: "USD"},
			{Symbol: "TLT", Name: "iShares 20+ Year Treasury Bond ETF", Market: "US ETF", Currency: "USD"},
			{Symbol: "BND", Name: "Vanguard Total Bond Market ETF", Market: "US ETF", Currency: "USD"},
			{Symbol: "AGG", Name: "iShares Core U.S. Aggregate Bond ETF", Market: "US ETF", Currency: "USD"},
		}
	case HotCategoryETFSector:
		return []hotSeed{
			{Symbol: "XLK", Name: "Technology Select Sector SPDR Fund", Market: "US ETF", Currency: "USD"},
			{Symbol: "XLF", Name: "Financial Select Sector SPDR Fund", Market: "US ETF", Currency: "USD"},
			{Symbol: "XLV", Name: "Health Care Select Sector SPDR Fund", Market: "US ETF", Currency: "USD"},
			{Symbol: "XLE", Name: "Energy Select Sector SPDR Fund", Market: "US ETF", Currency: "USD"},
			{Symbol: "XLI", Name: "Industrial Select Sector SPDR Fund", Market: "US ETF", Currency: "USD"},
			{Symbol: "XLY", Name: "Consumer Discretionary Select Sector SPDR Fund", Market: "US ETF", Currency: "USD"},
			{Symbol: "XLP", Name: "Consumer Staples Select Sector SPDR Fund", Market: "US ETF", Currency: "USD"},
			{Symbol: "XLU", Name: "Utilities Select Sector SPDR Fund", Market: "US ETF", Currency: "USD"},
			{Symbol: "XLB", Name: "Materials Select Sector SPDR Fund", Market: "US ETF", Currency: "USD"},
			{Symbol: "XLRE", Name: "Real Estate Select Sector SPDR Fund", Market: "US ETF", Currency: "USD"},
			{Symbol: "SMH", Name: "VanEck Semiconductor ETF", Market: "US ETF", Currency: "USD"},
			{Symbol: "SOXX", Name: "iShares Semiconductor ETF", Market: "US ETF", Currency: "USD"},
			{Symbol: "XBI", Name: "SPDR S&P Biotech ETF", Market: "US ETF", Currency: "USD"},
			{Symbol: "IBB", Name: "iShares Biotechnology ETF", Market: "US ETF", Currency: "USD"},
			{Symbol: "ARKK", Name: "ARK Innovation ETF", Market: "US ETF", Currency: "USD"},
			{Symbol: "KWEB", Name: "KraneShares CSI China Internet ETF", Market: "US ETF", Currency: "USD"},
			{Symbol: "ICLN", Name: "iShares Global Clean Energy ETF", Market: "US ETF", Currency: "USD"},
			{Symbol: "TAN", Name: "Invesco Solar ETF", Market: "US ETF", Currency: "USD"},
			{Symbol: "VNQ", Name: "Vanguard Real Estate ETF", Market: "US ETF", Currency: "USD"},
			{Symbol: "XME", Name: "SPDR S&P Metals & Mining ETF", Market: "US ETF", Currency: "USD"},
		}
	case HotCategoryETFIncome:
		return []hotSeed{
			{Symbol: "SCHD", Name: "Schwab U.S. Dividend Equity ETF", Market: "US ETF", Currency: "USD"},
			{Symbol: "VIG", Name: "Vanguard Dividend Appreciation ETF", Market: "US ETF", Currency: "USD"},
			{Symbol: "JEPI", Name: "JPMorgan Equity Premium Income ETF", Market: "US ETF", Currency: "USD"},
			{Symbol: "JEPQ", Name: "JPMorgan Nasdaq Equity Premium Income ETF", Market: "US ETF", Currency: "USD"},
			{Symbol: "DGRO", Name: "iShares Core Dividend Growth ETF", Market: "US ETF", Currency: "USD"},
			{Symbol: "HDV", Name: "iShares Core High Dividend ETF", Market: "US ETF", Currency: "USD"},
			{Symbol: "HYG", Name: "iShares iBoxx High Yield Corporate Bond ETF", Market: "US ETF", Currency: "USD"},
			{Symbol: "LQD", Name: "iShares iBoxx $ Investment Grade Corporate Bond ETF", Market: "US ETF", Currency: "USD"},
			{Symbol: "IEF", Name: "iShares 7-10 Year Treasury Bond ETF", Market: "US ETF", Currency: "USD"},
			{Symbol: "SHY", Name: "iShares 1-3 Year Treasury Bond ETF", Market: "US ETF", Currency: "USD"},
			{Symbol: "TIP", Name: "iShares TIPS Bond ETF", Market: "US ETF", Currency: "USD"},
			{Symbol: "TLT", Name: "iShares 20+ Year Treasury Bond ETF", Market: "US ETF", Currency: "USD"},
			{Symbol: "GLD", Name: "SPDR Gold Shares", Market: "US ETF", Currency: "USD"},
			{Symbol: "IAU", Name: "iShares Gold Trust", Market: "US ETF", Currency: "USD"},
			{Symbol: "SLV", Name: "iShares Silver Trust", Market: "US ETF", Currency: "USD"},
			{Symbol: "USO", Name: "United States Oil Fund", Market: "US ETF", Currency: "USD"},
			{Symbol: "UUP", Name: "Invesco DB US Dollar Index Bullish Fund", Market: "US ETF", Currency: "USD"},
			{Symbol: "BIL", Name: "SPDR Bloomberg 1-3 Month T-Bill ETF", Market: "US ETF", Currency: "USD"},
			{Symbol: "SGOV", Name: "iShares 0-3 Month Treasury Bond ETF", Market: "US ETF", Currency: "USD"},
			{Symbol: "MINT", Name: "PIMCO Enhanced Short Maturity Active ETF", Market: "US ETF", Currency: "USD"},
		}
	default:
		return nil
	}
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}
