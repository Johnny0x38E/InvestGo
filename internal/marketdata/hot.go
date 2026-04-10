package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"investgo/internal/datasource"
	"investgo/internal/monitor"
)

const (
	hotDefaultPageSize = 20
	hotSearchFetchSize = 200
	hotSearchCacheTTL  = 45 * time.Second
)

// HotService 负责热门榜单的实时数据抓取与分页。
// CN-A/HK 市场使用东方财富 clist 接口并回退到数据池；ETF 和 US 分类始终使用数据池 + 实时行情。
type HotService struct {
	client *http.Client
	mu     sync.RWMutex
	cache  map[string]cachedHotPage
}

type cachedHotPage struct {
	items     []monitor.HotItem
	total     int
	expiresAt time.Time
}

type eastMoneyHotResponse struct {
	RC   int `json:"rc"`
	Data struct {
		Total int `json:"total"`
		Diff  []struct {
			MarketID      int     `json:"f13"`
			Code          string  `json:"f12"`
			Name          string  `json:"f14"`
			CurrentPrice  emFloat `json:"f2"`
			ChangePercent emFloat `json:"f3"`
			Change        emFloat `json:"f4"`
			Volume        emFloat `json:"f5"`
			MarketCap     emFloat `json:"f20"`
		} `json:"diff"`
	} `json:"data"`
}

type yahooSearchResponse struct {
	Quotes []struct {
		Symbol    string `json:"symbol"`
		ShortName string `json:"shortname"`
		LongName  string `json:"longname"`
		QuoteType string `json:"quoteType"`
		TypeDisp  string `json:"typeDisp"`
		Exchange  string `json:"exchange"`
		ExchDisp  string `json:"exchDisp"`
	} `json:"quotes"`
}

// NewHotService 创建热门榜单服务。
func NewHotService(client *http.Client) *HotService {
	if client == nil {
		client = &http.Client{Timeout: 12 * time.Second}
	}
	return &HotService{
		client: client,
		cache:  make(map[string]cachedHotPage),
	}
}

// List 返回指定分类和排序条件下的热门榜单。
func (s *HotService) List(ctx context.Context, category monitor.HotCategory, sortBy monitor.HotSort, keyword string, page, pageSize int) (monitor.HotListResponse, error) {
	category = normaliseHotCategory(category)
	sortBy = normaliseHotSort(sortBy)
	keyword = normaliseHotKeyword(keyword)
	page = maxInt(page, 1)
	if pageSize <= 0 {
		pageSize = hotDefaultPageSize
	}
	if keyword != "" {
		return s.search(ctx, category, sortBy, keyword, page, pageSize)
	}

	switch {
	case category == monitor.HotCategoryCNA:
		// 沪深A股 uses East Money clist API, falls back to pool
		list, err := s.listEastMoney(ctx, category, sortBy, page, pageSize)
		if err == nil {
			return list, nil
		}
		return s.listFromPool(ctx, category, sortBy, page, pageSize)
	case category == monitor.HotCategoryHK:
		// 港股 uses East Money clist API, falls back to pool
		list, err := s.listEastMoney(ctx, category, sortBy, page, pageSize)
		if err == nil {
			return list, nil
		}
		return s.listFromPool(ctx, category, sortBy, page, pageSize)
	case category == monitor.HotCategoryCNETF || category == monitor.HotCategoryHKETF || isUSHotCategory(category):
		// ETF and US indices always use pool + real-time quotes
		return s.listFromPool(ctx, category, sortBy, page, pageSize)
	default:
		return monitor.HotListResponse{}, fmt.Errorf("不支持的热门分类: %s", category)
	}
}

func (s *HotService) search(ctx context.Context, category monitor.HotCategory, sortBy monitor.HotSort, keyword string, page, pageSize int) (monitor.HotListResponse, error) {
	if category == monitor.HotCategoryUSETF {
		return s.searchUSETFs(ctx, sortBy, keyword, page, pageSize)
	}

	items, err := s.listAllSearchableItems(ctx, category, sortBy)
	if err != nil {
		return monitor.HotListResponse{}, err
	}

	filtered := filterHotItems(items, keyword)
	start, end := paginateHotItems(len(filtered), page, pageSize)
	pageItems := filtered[start:end]

	return monitor.HotListResponse{
		Category:    category,
		Sort:        sortBy,
		Page:        page,
		PageSize:    pageSize,
		Total:       len(filtered),
		HasMore:     end < len(filtered),
		Items:       pageItems,
		GeneratedAt: time.Now(),
	}, nil
}

func (s *HotService) searchUSETFs(ctx context.Context, sortBy monitor.HotSort, keyword string, page, pageSize int) (monitor.HotListResponse, error) {
	seeds := filterHotSeeds(hotConstituents[monitor.HotCategoryUSETF], keyword)

	remoteSeeds, err := s.searchYahooUSSeeds(ctx, keyword)
	if err == nil {
		seeds = mergeHotSeeds(seeds, remoteSeeds)
	}

	items, err := s.loadHotItemsForSeeds(ctx, seeds, "Yahoo Finance Search")
	if err != nil {
		return monitor.HotListResponse{}, err
	}

	sortHotItems(items, sortBy)
	start, end := paginateHotItems(len(items), page, pageSize)
	return monitor.HotListResponse{
		Category:    monitor.HotCategoryUSETF,
		Sort:        sortBy,
		Page:        page,
		PageSize:    pageSize,
		Total:       len(items),
		HasMore:     end < len(items),
		Items:       items[start:end],
		GeneratedAt: time.Now(),
	}, nil
}

func (s *HotService) listAllSearchableItems(ctx context.Context, category monitor.HotCategory, sortBy monitor.HotSort) ([]monitor.HotItem, error) {
	cacheKey := hotSearchCacheKey(category, sortBy)
	if items, ok := s.loadCachedItems(cacheKey); ok {
		return items, nil
	}

	var (
		items []monitor.HotItem
		err   error
	)

	switch {
	case category == monitor.HotCategoryCNA || category == monitor.HotCategoryHK:
		items, err = s.fetchAllHotPages(ctx, func(ctx context.Context, page, pageSize int) (monitor.HotListResponse, error) {
			return s.listEastMoney(ctx, category, sortBy, page, pageSize)
		})
		if err != nil {
			items, err = s.loadPoolItems(ctx, category, sortBy)
		}
	case category == monitor.HotCategoryCNETF || category == monitor.HotCategoryHKETF || isUSHotCategory(category):
		items, err = s.loadPoolItems(ctx, category, sortBy)
	default:
		err = fmt.Errorf("不支持的热门分类: %s", category)
	}

	if err != nil {
		return nil, err
	}

	sortHotItems(items, sortBy)
	s.storeCachedItems(cacheKey, items)
	return cloneHotItems(items), nil
}

func (s *HotService) fetchAllHotPages(ctx context.Context, loadPage func(context.Context, int, int) (monitor.HotListResponse, error)) ([]monitor.HotItem, error) {
	all := make([]monitor.HotItem, 0, hotSearchFetchSize)
	for page := 1; ; page++ {
		resp, err := loadPage(ctx, page, hotSearchFetchSize)
		if err != nil {
			return nil, err
		}
		all = append(all, resp.Items...)
		if !resp.HasMore || len(resp.Items) == 0 {
			return all, nil
		}
	}
}

func (s *HotService) loadCachedItems(key string) ([]monitor.HotItem, bool) {
	s.mu.RLock()
	cached, ok := s.cache[key]
	s.mu.RUnlock()
	if !ok || time.Now().After(cached.expiresAt) {
		return nil, false
	}
	return cloneHotItems(cached.items), true
}

func (s *HotService) storeCachedItems(key string, items []monitor.HotItem) {
	s.mu.Lock()
	s.cache[key] = cachedHotPage{
		items:     cloneHotItems(items),
		total:     len(items),
		expiresAt: time.Now().Add(hotSearchCacheTTL),
	}
	s.mu.Unlock()
}

// listEastMoney 调用东方财富 clist 接口，适用于 CN-A、HK 分类。
func (s *HotService) listEastMoney(ctx context.Context, category monitor.HotCategory, sortBy monitor.HotSort, page, pageSize int) (monitor.HotListResponse, error) {
	fs, market, currency := resolveEastMoneyHotFilter(category)
	if fs == "" {
		return monitor.HotListResponse{}, fmt.Errorf("不支持的东方财富热门分类: %s", category)
	}

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
	params.Set("fs", fs)
	params.Set("fields", "f2,f3,f4,f5,f12,f13,f14,f20")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, datasource.URLWithQuery(datasource.EastMoneyHotAPI, params), nil)
	if err != nil {
		return monitor.HotListResponse{}, err
	}
	req.Header.Set("Referer", datasource.EastMoneyWebReferer)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/123.0.0.0 Safari/537.36")

	resp, err := s.client.Do(req)
	if err != nil {
		return monitor.HotListResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return monitor.HotListResponse{}, fmt.Errorf("东方财富热门请求失败: status %d", resp.StatusCode)
	}

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return monitor.HotListResponse{}, err
	}

	var parsed eastMoneyHotResponse
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return monitor.HotListResponse{}, err
	}
	if parsed.RC != 0 {
		return monitor.HotListResponse{}, fmt.Errorf("东方财富热门返回 rc=%d", parsed.RC)
	}

	items := make([]monitor.HotItem, 0, len(parsed.Data.Diff))
	for _, item := range parsed.Data.Diff {
		symbol := resolveEastMoneyHotSymbol(item.Code, item.MarketID, category)
		if symbol == "" {
			continue
		}
		items = append(items, monitor.HotItem{
			Symbol:        symbol,
			Name:          item.Name,
			Market:        market,
			Currency:      currency,
			CurrentPrice:  float64(item.CurrentPrice),
			Change:        float64(item.Change),
			ChangePercent: float64(item.ChangePercent),
			Volume:        float64(item.Volume),
			MarketCap:     float64(item.MarketCap),
			QuoteSource:   "东方财富",
			UpdatedAt:     time.Now(),
		})
	}

	return monitor.HotListResponse{
		Category:    category,
		Sort:        sortBy,
		Page:        page,
		PageSize:    pageSize,
		Total:       parsed.Data.Total,
		HasMore:     page*pageSize < parsed.Data.Total,
		Items:       items,
		GeneratedAt: time.Now(),
	}, nil
}

// listFromPool 使用预定义数据池 + 实时行情返回热门榜单分页结果。
func (s *HotService) listFromPool(ctx context.Context, category monitor.HotCategory, sortBy monitor.HotSort, page, pageSize int) (monitor.HotListResponse, error) {
	items, err := s.loadPoolItems(ctx, category, sortBy)
	if err != nil {
		return monitor.HotListResponse{}, err
	}

	start, end := paginateHotItems(len(items), page, pageSize)
	return monitor.HotListResponse{
		Category:    category,
		Sort:        sortBy,
		Page:        page,
		PageSize:    pageSize,
		Total:       len(items),
		HasMore:     end < len(items),
		Items:       items[start:end],
		GeneratedAt: time.Now(),
	}, nil
}

// loadPoolItems 从预定义数据池加载标的并获取实时行情。
func (s *HotService) loadPoolItems(ctx context.Context, category monitor.HotCategory, sortBy monitor.HotSort) ([]monitor.HotItem, error) {
	pool := hotConstituents[category]
	if len(pool) == 0 {
		return nil, fmt.Errorf("热门分类暂无可用数据池: %s", category)
	}

	items, err := s.loadHotItemsForSeeds(ctx, pool, "预置数据池")
	if err != nil {
		return nil, err
	}

	sortHotItems(items, sortBy)
	return items, nil
}

func (s *HotService) loadHotItemsForSeeds(ctx context.Context, seeds []hotSeed, fallbackSource string) ([]monitor.HotItem, error) {
	if len(seeds) == 0 {
		return []monitor.HotItem{}, nil
	}

	items, err := s.fetchPoolQuotes(ctx, seeds)
	if err != nil {
		return buildFallbackHotItems(seeds, fallbackSource), nil
	}

	return mergeHotItemsWithSeeds(items, seeds, fallbackSource), nil
}

// resolveEastMoneyHotFilter 将 HotCategory 映射为东方财富 clist 的 fs 参数、市场标签和货币。
func resolveEastMoneyHotFilter(category monitor.HotCategory) (fs, market, currency string) {
	switch category {
	case monitor.HotCategoryCNA:
		return "m:0 t:6,m:0 t:80,m:1 t:2,m:1 t:23", "CN-A", "CNY"
	case monitor.HotCategoryHK:
		return "m:128", "HK-MAIN", "HKD"
	default:
		return "", "", ""
	}
}

// resolveEastMoneyHotSymbol 根据东方财富返回的代码和市场 ID 生成标准股票代码。
func resolveEastMoneyHotSymbol(code string, marketID int, category monitor.HotCategory) string {
	code = normaliseEastMoneyCode(code, marketID)
	switch category {
	case monitor.HotCategoryCNA:
		switch marketID {
		case 1:
			return strings.ToUpper(code + ".SH")
		case 0:
			return strings.ToUpper(code + ".SZ")
		}
	case monitor.HotCategoryHK:
		return strings.ToUpper(code + ".HK")
	}
	return ""
}

// isCNHotCategory 判断分类是否属于 A 股市场（含 ETF）。
func isCNHotCategory(c monitor.HotCategory) bool {
	return c == monitor.HotCategoryCNA || c == monitor.HotCategoryCNETF
}

// isHKHotCategory 判断分类是否属于港股市场。
func isHKHotCategory(c monitor.HotCategory) bool {
	return c == monitor.HotCategoryHK || c == monitor.HotCategoryHKETF
}

// isUSHotCategory 判断分类是否属于美股市场。
func isUSHotCategory(c monitor.HotCategory) bool {
	switch c {
	case monitor.HotCategoryUSSP500, monitor.HotCategoryUSNasdaq, monitor.HotCategoryUSDow, monitor.HotCategoryUSETF:
		return true
	}
	return false
}

// normaliseHotCategory 把缺失或无效的分类回落到默认值。
func normaliseHotCategory(c monitor.HotCategory) monitor.HotCategory {
	switch c {
	case monitor.HotCategoryCNA, monitor.HotCategoryCNETF,
		monitor.HotCategoryHK, monitor.HotCategoryHKETF,
		monitor.HotCategoryUSSP500, monitor.HotCategoryUSNasdaq, monitor.HotCategoryUSDow, monitor.HotCategoryUSETF:
		return c
	}
	return monitor.HotCategoryCNA
}

// normaliseHotSort 把缺失或无效的排序字段回落到默认值。
func normaliseHotSort(s monitor.HotSort) monitor.HotSort {
	switch s {
	case monitor.HotSortVolume, monitor.HotSortGainers, monitor.HotSortLosers, monitor.HotSortMarketCap, monitor.HotSortPrice:
		return s
	}
	return monitor.HotSortVolume
}

// resolveEastMoneySort 把 HotSort 映射为东方财富 clist 的排序字段 ID 和排序方向。
func resolveEastMoneySort(sortBy monitor.HotSort) (fid string, po int) {
	switch sortBy {
	case monitor.HotSortGainers:
		return "f3", 1
	case monitor.HotSortLosers:
		return "f3", 0
	case monitor.HotSortMarketCap:
		return "f20", 1
	case monitor.HotSortPrice:
		return "f2", 1
	default: // volume
		return "f5", 1
	}
}

// normaliseEastMoneyCode 根据 marketID 补齐东方财富返回代码的前导零。
func normaliseEastMoneyCode(code string, marketID int) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	switch marketID {
	case 116, 128:
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

func normaliseHotKeyword(keyword string) string {
	return strings.ToLower(strings.TrimSpace(keyword))
}

func filterHotSeeds(seeds []hotSeed, keyword string) []hotSeed {
	keyword = normaliseHotKeyword(keyword)
	if keyword == "" {
		return append([]hotSeed(nil), seeds...)
	}

	filtered := make([]hotSeed, 0, len(seeds))
	for _, seed := range seeds {
		if strings.Contains(strings.ToLower(seed.Name), keyword) || strings.Contains(strings.ToLower(seed.Symbol), keyword) {
			filtered = append(filtered, seed)
		}
	}
	return filtered
}

func filterHotItems(items []monitor.HotItem, keyword string) []monitor.HotItem {
	keyword = normaliseHotKeyword(keyword)
	if keyword == "" {
		return cloneHotItems(items)
	}

	filtered := make([]monitor.HotItem, 0, len(items))
	for _, item := range items {
		if strings.Contains(strings.ToLower(item.Name), keyword) || strings.Contains(strings.ToLower(item.Symbol), keyword) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func paginateHotItems(total, page, pageSize int) (start, end int) {
	if total <= 0 {
		return 0, 0
	}
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = hotDefaultPageSize
	}

	start = (page - 1) * pageSize
	if start >= total {
		return total, total
	}

	end = start + pageSize
	if end > total {
		end = total
	}
	return start, end
}

func hotSearchCacheKey(category monitor.HotCategory, sortBy monitor.HotSort) string {
	return string(category) + "|" + string(sortBy)
}

func mergeHotSeeds(base, extra []hotSeed) []hotSeed {
	merged := append([]hotSeed(nil), base...)
	seen := make(map[string]struct{}, len(base))
	for _, seed := range base {
		seen[seed.Market+"|"+strings.ToUpper(seed.Symbol)] = struct{}{}
	}

	for _, seed := range extra {
		key := seed.Market + "|" + strings.ToUpper(seed.Symbol)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		merged = append(merged, seed)
	}
	return merged
}

func mergeHotItemsWithSeeds(items []monitor.HotItem, seeds []hotSeed, fallbackSource string) []monitor.HotItem {
	merged := cloneHotItems(items)
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		seen[item.Market+"|"+strings.ToUpper(item.Symbol)] = struct{}{}
	}

	now := time.Now()
	for _, seed := range seeds {
		key := seed.Market + "|" + strings.ToUpper(seed.Symbol)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		merged = append(merged, monitor.HotItem{
			Symbol:      seed.Symbol,
			Name:        seed.Name,
			Market:      seed.Market,
			Currency:    seed.Currency,
			QuoteSource: fallbackSource,
			UpdatedAt:   now,
		})
	}
	return merged
}

func buildFallbackHotItems(seeds []hotSeed, source string) []monitor.HotItem {
	items := make([]monitor.HotItem, 0, len(seeds))
	now := time.Now()
	for _, seed := range seeds {
		items = append(items, monitor.HotItem{
			Symbol:      seed.Symbol,
			Name:        seed.Name,
			Market:      seed.Market,
			Currency:    seed.Currency,
			QuoteSource: source,
			UpdatedAt:   now,
		})
	}
	return items
}

func cloneHotItems(items []monitor.HotItem) []monitor.HotItem {
	return append([]monitor.HotItem(nil), items...)
}

func sortHotItems(items []monitor.HotItem, sortBy monitor.HotSort) {
	sort.SliceStable(items, func(i, j int) bool {
		switch sortBy {
		case monitor.HotSortGainers:
			return items[i].ChangePercent > items[j].ChangePercent
		case monitor.HotSortLosers:
			return items[i].ChangePercent < items[j].ChangePercent
		case monitor.HotSortMarketCap:
			return items[i].MarketCap > items[j].MarketCap
		case monitor.HotSortPrice:
			return items[i].CurrentPrice > items[j].CurrentPrice
		default:
			return items[i].Volume > items[j].Volume
		}
	})
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (s *HotService) searchYahooUSSeeds(ctx context.Context, keyword string) ([]hotSeed, error) {
	parsed, err := fetchYahooSearch(ctx, s.client, keyword)
	if err != nil {
		return nil, err
	}

	seeds := make([]hotSeed, 0, len(parsed.Quotes))
	seen := make(map[string]struct{}, len(parsed.Quotes))
	for _, quote := range parsed.Quotes {
		if !isYahooETFQuote(quote.QuoteType, quote.TypeDisp) || !isLikelyUSExchange(quote.Exchange, quote.ExchDisp) {
			continue
		}

		symbol := strings.ToUpper(strings.TrimSpace(quote.Symbol))
		if symbol == "" {
			continue
		}

		if _, ok := seen[symbol]; ok {
			continue
		}
		seen[symbol] = struct{}{}
		seeds = append(seeds, hotSeed{
			Symbol:   symbol,
			Name:     firstNonEmpty(quote.LongName, quote.ShortName, symbol),
			Market:   "US-ETF",
			Currency: "USD",
		})
	}
	return seeds, nil
}

func fetchYahooSearch(ctx context.Context, client *http.Client, keyword string) (yahooSearchResponse, error) {
	if client == nil {
		client = &http.Client{}
	}

	params := url.Values{}
	params.Set("q", strings.TrimSpace(keyword))
	params.Set("quotesCount", "20")
	params.Set("newsCount", "0")
	params.Set("enableFuzzyQuery", "false")

	problems := make([]string, 0, len(datasource.YahooSearchHosts))
	for _, host := range datasource.YahooSearchHosts {
		parsed, err := fetchYahooSearchFromHost(ctx, client, host, params)
		if err == nil {
			return parsed, nil
		}
		problems = append(problems, fmt.Sprintf("%s: %v", host, err))
	}

	return yahooSearchResponse{}, collapseProblems(problems)
}

func fetchYahooSearchFromHost(ctx context.Context, client *http.Client, host string, params url.Values) (yahooSearchResponse, error) {
	query := make(url.Values, len(params))
	for key, values := range params {
		query[key] = append([]string(nil), values...)
	}

	requestURL := url.URL{
		Scheme:   "https",
		Host:     host,
		Path:     datasource.YahooSearchPath,
		RawQuery: query.Encode(),
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL.String(), nil)
	if err != nil {
		return yahooSearchResponse{}, err
	}
	request.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/123.0.0.0 Safari/537.36")
	request.Header.Set("Origin", datasource.YahooFinanceOrigin)
	request.Header.Set("Referer", datasource.YahooFinanceReferer)

	response, err := client.Do(request)
	if err != nil {
		return yahooSearchResponse{}, err
	}
	defer response.Body.Close()

	payload, err := io.ReadAll(response.Body)
	if err != nil {
		return yahooSearchResponse{}, err
	}

	if response.StatusCode != http.StatusOK {
		return yahooSearchResponse{}, fmt.Errorf("status %d", response.StatusCode)
	}

	var parsed yahooSearchResponse
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return yahooSearchResponse{}, err
	}
	return parsed, nil
}

func isYahooETFQuote(quoteType, typeDisp string) bool {
	quoteType = strings.ToUpper(strings.TrimSpace(quoteType))
	typeDisp = strings.ToUpper(strings.TrimSpace(typeDisp))
	return quoteType == "ETF" || strings.Contains(typeDisp, "ETF")
}

func isLikelyUSExchange(exchange, exchDisp string) bool {
	label := strings.ToUpper(strings.TrimSpace(exchange + " " + exchDisp))
	if label == "" {
		return true
	}

	for _, token := range []string{"NASDAQ", "NYSE", "ARCA", "ARCX", "BATS", "PCX"} {
		if strings.Contains(label, token) {
			return true
		}
	}
	return false
}
