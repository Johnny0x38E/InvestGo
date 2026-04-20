package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	ttlcache "investgo/internal/cache"
	"investgo/internal/datasource"
	"investgo/internal/monitor"
)

const (
	hotDefaultPageSize = 20
	hotSearchFetchSize = 200
	defaultHotCacheTTL = 60 * time.Second
)

// HotListOptions carries request-scoped settings that affect hot list quote fetching.
type HotListOptions struct {
	CNQuoteSource string
	HKQuoteSource string
	USQuoteSource string
	CacheTTL      time.Duration
	BypassCache   bool
}

// HotService handles real-time data fetching and pagination for hot lists.
// Category membership may come from different upstream ranking sources, while
// displayed quote data should follow the configured market quote source.
type HotService struct {
	client        *http.Client
	log           *slog.Logger
	registry      *Registry
	searchCache   *ttlcache.TTL[string, []monitor.HotItem]
	responseCache *ttlcache.TTL[string, monitor.HotListResponse]
}

type eastMoneyHotResponse struct {
	RC   int `json:"rc"`
	Data struct {
		Total int                  `json:"total"`
		Diff  eastMoneyHotDiffList `json:"diff"`
	} `json:"data"`
}

type eastMoneyHotDiffList []eastMoneyHotDiff

func (l *eastMoneyHotDiffList) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*l = nil
		return nil
	}

	var asArray []eastMoneyHotDiff
	if err := json.Unmarshal(data, &asArray); err == nil {
		*l = asArray
		return nil
	}

	var asMap map[string]eastMoneyHotDiff
	if err := json.Unmarshal(data, &asMap); err != nil {
		return err
	}

	out := make([]eastMoneyHotDiff, 0, len(asMap))
	for _, item := range asMap {
		out = append(out, item)
	}
	*l = out
	return nil
}

type eastMoneyHotDiff struct {
	MarketID      int     `json:"f13"`
	Code          string  `json:"f12"`
	Name          string  `json:"f14"`
	CurrentPrice  emFloat `json:"f2"`
	ChangePercent emFloat `json:"f3"`
	Change        emFloat `json:"f4"`
	Volume        emFloat `json:"f5"`
	MarketCap     emFloat `json:"f20"`
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

type eastMoneySuggestResponse struct {
	QuotationCodeTable struct {
		Data []eastMoneySuggestItem `json:"Data"`
	} `json:"QuotationCodeTable"`
}

type eastMoneySuggestItem struct {
	Code             string `json:"Code"`
	Name             string `json:"Name"`
	MktNum           string `json:"MktNum"`
	SecurityTypeName string `json:"SecurityTypeName"`
}

// NewHotService creates a hot list service.
func NewHotService(client *http.Client, logger *slog.Logger, registry *Registry) *HotService {
	if client == nil {
		client = &http.Client{Timeout: 12 * time.Second}
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &HotService{
		client:        client,
		log:           logger,
		registry:      registry,
		searchCache:   ttlcache.NewTTL[string, []monitor.HotItem](),
		responseCache: ttlcache.NewTTL[string, monitor.HotListResponse](),
	}
}

// List returns the hot list for the given category and sort order.
func (s *HotService) List(ctx context.Context, category monitor.HotCategory, sortBy monitor.HotSort, keyword string, page, pageSize int, options HotListOptions) (monitor.HotListResponse, error) {
	category = normaliseHotCategory(category)
	sortBy = normaliseHotSort(sortBy)
	keyword = normaliseHotKeyword(keyword)
	options = normaliseHotListOptions(options)
	page = maxInt(page, 1)
	if pageSize <= 0 {
		pageSize = hotDefaultPageSize
	}
	cacheKey := hotResponseCacheKey(category, sortBy, keyword, page, pageSize, options)
	if !options.BypassCache {
		if response, ok := s.loadCachedResponse(cacheKey); ok {
			return response, nil
		}
	}

	var response monitor.HotListResponse
	var err error
	if keyword != "" {
		response, err = s.search(ctx, category, sortBy, keyword, page, pageSize, options)
	} else {
		switch {
		case category == monitor.HotCategoryCNA,
			category == monitor.HotCategoryCNETF,
			category == monitor.HotCategoryHK:
			response, err = s.listConfiguredCategory(ctx, category, sortBy, page, pageSize, options)
		case category == monitor.HotCategoryHKETF,
			isUSHotCategory(category):
			response, err = s.listFromPool(ctx, category, sortBy, page, pageSize, options)
		default:
			err = fmt.Errorf("Hot category is unsupported: %s", category)
		}
	}
	if err != nil {
		return monitor.HotListResponse{}, err
	}

	response.Cached = false
	expiresAt := s.storeCachedResponse(cacheKey, response, options.CacheTTL)
	response.CacheExpiresAt = ptrTime(expiresAt)
	return response, nil
}

// search filters the data pool by keyword. Each market uses a lightweight search approach:
// CN/HK uses EastMoney suggest API, US equities use local seed filtering + Yahoo search,
// US ETFs combine local pool + Yahoo search.
func (s *HotService) search(ctx context.Context, category monitor.HotCategory, sortBy monitor.HotSort, keyword string, page, pageSize int, options HotListOptions) (monitor.HotListResponse, error) {
	if category == monitor.HotCategoryUSETF {
		return s.searchUSETFs(ctx, sortBy, keyword, page, pageSize, options)
	}

	// Pool-backed categories (US equities) filter seeds locally first, then use
	// Yahoo search for broader coverage (e.g. name search beyond local seed names).
	if isUSHotCategory(category) {
		return s.searchUSStocks(ctx, category, sortBy, keyword, page, pageSize, options)
	}

	// CN/HK categories use EastMoney suggest API for fast keyword search.
	if isCNHotCategory(category) || isHKHotCategory(category) {
		return s.searchCNHK(ctx, category, sortBy, keyword, page, pageSize, options)
	}

	return monitor.HotListResponse{}, fmt.Errorf("Hot search is unsupported for category: %s", category)
}

// searchUSETFs handles US ETF search specially:
// filter from the pool first, then call the Yahoo Finance search API for more matches,
// merge and deduplicate, and fetch real-time quotes.
func (s *HotService) searchUSETFs(ctx context.Context, sortBy monitor.HotSort, keyword string, page, pageSize int, options HotListOptions) (monitor.HotListResponse, error) {
	seeds := filterHotSeeds(normalizedUSHotSeeds(monitor.HotCategoryUSETF, hotConstituents[monitor.HotCategoryUSETF]), keyword)

	remoteSeeds, err := s.searchYahooUSSeeds(ctx, keyword)
	if err == nil {
		seeds = mergeHotSeeds(seeds, remoteSeeds)
	}

	items, err := s.loadHotItemsForSeeds(ctx, seeds, options)
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

// searchUSStocks handles keyword search for US equity categories.
// It first filters the local seed pool by name/symbol, then calls Yahoo search for
// broader coverage (e.g. matching by company name that may not be in the local seed names),
// merges and deduplicates, then fetches quotes for the combined matches.
func (s *HotService) searchUSStocks(ctx context.Context, category monitor.HotCategory, sortBy monitor.HotSort, keyword string, page, pageSize int, options HotListOptions) (monitor.HotListResponse, error) {
	pool := normalizedUSHotSeeds(category, hotConstituents[category])

	// Filter seeds locally — no network I/O.
	seeds := filterHotSeeds(pool, keyword)

	// Call Yahoo search for broader coverage (e.g. name-based search).
	remoteSeeds, err := s.searchYahooUSStockSeeds(ctx, keyword, category)
	if err == nil && len(remoteSeeds) > 0 {
		seeds = mergeHotSeeds(seeds, remoteSeeds)
	}

	if len(seeds) == 0 {
		return monitor.HotListResponse{
			Category:    category,
			Sort:        sortBy,
			Page:        page,
			PageSize:    pageSize,
			Total:       0,
			HasMore:     false,
			Items:       []monitor.HotItem{},
			GeneratedAt: time.Now(),
		}, nil
	}

	// Only fetch quotes for the (small) set of matching seeds.
	items, err := s.loadHotItemsForSeeds(ctx, seeds, options)
	if err != nil {
		return monitor.HotListResponse{}, err
	}

	sortHotItems(items, sortBy)
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

// searchCNHK handles keyword search for CN and HK categories using the EastMoney suggest API.
// This replaces the old fetch-all-then-filter approach that would download thousands of items.
func (s *HotService) searchCNHK(ctx context.Context, category monitor.HotCategory, sortBy monitor.HotSort, keyword string, page, pageSize int, options HotListOptions) (monitor.HotListResponse, error) {
	// Call EastMoney suggest API — single lightweight request, returns only matches.
	seeds := s.searchEastMoneySeeds(ctx, keyword, category)

	// Also try to filter from cached items (from previous normal browsing).
	if cachedItems, ok := s.loadCachedItems(hotSearchCacheKey(category, sortBy, resolveHotQuoteSource(category, options))); ok {
		cachedMatches := filterHotItems(cachedItems, keyword)
		for _, item := range cachedMatches {
			seeds = mergeHotSeeds(seeds, []hotSeed{{
				Symbol:   item.Symbol,
				Name:     item.Name,
				Market:   item.Market,
				Currency: item.Currency,
			}})
		}
	}

	if len(seeds) == 0 {
		return monitor.HotListResponse{
			Category:    category,
			Sort:        sortBy,
			Page:        page,
			PageSize:    pageSize,
			Total:       0,
			HasMore:     false,
			Items:       []monitor.HotItem{},
			GeneratedAt: time.Now(),
		}, nil
	}

	// Fetch quotes only for the small set of matching seeds.
	items, err := s.loadHotItemsForSeeds(ctx, seeds, options)
	if err != nil {
		return monitor.HotListResponse{}, err
	}

	sortHotItems(items, sortBy)
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

// listAllSearchableItems lists all searchable instruments for the given category, used for full-match search.
// For categories backed by upstream ranking APIs, fetch and cache the full list;
// for pool-backed categories, use the maintained local pool directly.
func (s *HotService) listAllSearchableItems(ctx context.Context, category monitor.HotCategory, sortBy monitor.HotSort, options HotListOptions) ([]monitor.HotItem, error) {
	cacheKey := hotSearchCacheKey(category, sortBy, resolveHotQuoteSource(category, options))
	if !options.BypassCache {
		if items, ok := s.loadCachedItems(cacheKey); ok {
			return items, nil
		}
	}

	var (
		items []monitor.HotItem
		err   error
	)

	switch {
	case category == monitor.HotCategoryCNA ||
		category == monitor.HotCategoryCNETF ||
		category == monitor.HotCategoryHK ||
		category == monitor.HotCategoryHKETF:
		items, err = s.fetchAllHotPages(ctx, func(ctx context.Context, page, pageSize int) (monitor.HotListResponse, error) {
			return s.listConfiguredCategory(ctx, category, sortBy, page, pageSize, options)
		})
	case isUSHotCategory(category):
		items, err = s.loadPoolItems(ctx, category, sortBy, options)
	default:
		err = fmt.Errorf("Hot category is unsupported: %s", category)
	}

	if err != nil {
		return nil, err
	}

	sortHotItems(items, sortBy)
	s.storeCachedItems(cacheKey, items, options.CacheTTL)
	return cloneHotItems(items), nil
}

// fetchAllHotPages fetches all hot instruments for the given category and sort order via the page loader until no more pages remain.
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

// loadCachedItems loads the hot instrument list from cache;
// returns false if cache is missing or expired.
func (s *HotService) loadCachedItems(key string) ([]monitor.HotItem, bool) {
	cached, _, ok := s.searchCache.Get(key)
	if !ok {
		return nil, false
	}
	return cloneHotItems(cached), true
}

// storeCachedItems stores the hot instrument list into cache and sets an expiration time.
func (s *HotService) storeCachedItems(key string, items []monitor.HotItem, ttl time.Duration) {
	if ttl <= 0 {
		ttl = defaultHotCacheTTL
	}
	s.searchCache.Set(key, cloneHotItems(items), ttl)
}

func (s *HotService) loadCachedResponse(key string) (monitor.HotListResponse, bool) {
	cached, expiresAt, ok := s.responseCache.Get(key)
	if !ok {
		return monitor.HotListResponse{}, false
	}
	response := cloneHotListResponse(cached)
	response.Cached = true
	response.CacheExpiresAt = ptrTime(expiresAt)
	return response, true
}

func (s *HotService) storeCachedResponse(key string, response monitor.HotListResponse, ttl time.Duration) time.Time {
	if ttl <= 0 {
		ttl = defaultHotCacheTTL
	}
	expiresAt := time.Now().Add(ttl)
	cached := cloneHotListResponse(response)
	cached.Cached = false
	cached.CacheExpiresAt = ptrTime(expiresAt)

	expiresAt = s.responseCache.Set(key, cached, ttl)

	return expiresAt
}

// listEastMoney calls the EastMoney clist API, applicable to CN-A and HK categories.
func (s *HotService) listEastMoney(ctx context.Context, category monitor.HotCategory, sortBy monitor.HotSort, page, pageSize int) (monitor.HotListResponse, error) {
	fs, market, currency := resolveEastMoneyHotFilter(category)
	if fs == "" {
		return monitor.HotListResponse{}, fmt.Errorf("EastMoney hot category is unsupported: %s", category)
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
	setEastMoneyHeaders(req, datasource.EastMoneyWebReferer)

	resp, err := s.client.Do(req)
	if err != nil {
		return monitor.HotListResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return monitor.HotListResponse{}, fmt.Errorf("EastMoney hot request failed: status %d", resp.StatusCode)
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
		return monitor.HotListResponse{}, fmt.Errorf("EastMoney hot response returned rc=%d", parsed.RC)
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
			QuoteSource:   "EastMoney",
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

// listFromPool returns paginated hot list results using the predefined data pool + real-time quotes.
func (s *HotService) listFromPool(ctx context.Context, category monitor.HotCategory, sortBy monitor.HotSort, page, pageSize int, options HotListOptions) (monitor.HotListResponse, error) {
	items, err := s.loadPoolItems(ctx, category, sortBy, options)
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

func (s *HotService) listConfiguredCategory(ctx context.Context, category monitor.HotCategory, sortBy monitor.HotSort, page, pageSize int, options HotListOptions) (monitor.HotListResponse, error) {
	sourceID := resolveHotQuoteSource(category, options)

	if sourceID == "yahoo" {
		return monitor.HotListResponse{}, fmt.Errorf("Yahoo hot list is unsupported for category: %s", category)
	}

	if sourceSupportsCategoryList(sourceID, category) {
		return s.listCategoryBySource(ctx, sourceID, category, sortBy, page, pageSize)
	}

	return s.listConfiguredCategoryWithOverlay(ctx, category, sortBy, page, pageSize, options)
}

func (s *HotService) listConfiguredCategoryWithOverlay(ctx context.Context, category monitor.HotCategory, sortBy monitor.HotSort, page, pageSize int, options HotListOptions) (monitor.HotListResponse, error) {
	baseSource := membershipSourceForCategory(category)
	if baseSource == "" {
		return monitor.HotListResponse{}, fmt.Errorf("Hot quote source is unsupported: %s", resolveHotQuoteSource(category, options))
	}

	response, err := s.listCategoryBySource(ctx, baseSource, category, sortBy, page, pageSize)
	if err != nil {
		return monitor.HotListResponse{}, err
	}

	items, err := s.applyConfiguredQuotes(ctx, category, response.Items, options)
	if err != nil {
		return monitor.HotListResponse{}, err
	}
	sortHotItems(items, sortBy)
	response.Items = items
	response.GeneratedAt = time.Now()
	return response, nil
}

func (s *HotService) listCategoryBySource(ctx context.Context, sourceID string, category monitor.HotCategory, sortBy monitor.HotSort, page, pageSize int) (monitor.HotListResponse, error) {
	switch sourceID {
	case "eastmoney":
		return s.listEastMoney(ctx, category, sortBy, page, pageSize)
	case "sina":
		return s.listSina(ctx, category, sortBy, page, pageSize)
	case "xueqiu":
		return s.listXueqiu(ctx, category, sortBy, page, pageSize)
	default:
		return monitor.HotListResponse{}, fmt.Errorf("Hot quote source is unsupported: %s", sourceID)
	}
}

// loadPoolItems loads instruments from the predefined data pool and fetches real-time quotes.
func (s *HotService) loadPoolItems(ctx context.Context, category monitor.HotCategory, sortBy monitor.HotSort, options HotListOptions) ([]monitor.HotItem, error) {
	var pool []hotSeed
	if category == monitor.HotCategoryHKETF {
		pool = hkETFConstituents
	} else {
		pool = normalizedUSHotSeeds(category, hotConstituents[category])
	}
	if len(pool) == 0 {
		return nil, fmt.Errorf("No available hot pool for category: %s", category)
	}

	items, err := s.loadHotItemsForSeeds(ctx, pool, options)
	if err != nil {
		return nil, err
	}

	sortHotItems(items, sortBy)
	return items, nil
}

// loadHotItemsForSeeds fetches real-time quotes for the given hotSeed list and returns only rows backed by live data.
func (s *HotService) loadHotItemsForSeeds(ctx context.Context, seeds []hotSeed, options HotListOptions) ([]monitor.HotItem, error) {
	if len(seeds) == 0 {
		return []monitor.HotItem{}, nil
	}

	category := categoryForHotSeeds(seeds)
	sourceID := effectivePoolQuoteSource(category, resolveHotQuoteSource(category, options))
	items, err := s.fetchPoolQuotes(ctx, seeds, sourceID, options)
	if err != nil {
		return nil, err
	}

	return items, nil
}

func categoryForHotSeeds(seeds []hotSeed) monitor.HotCategory {
	if len(seeds) == 0 {
		return monitor.HotCategoryCNA
	}
	switch seeds[0].Market {
	case "US-STOCK":
		return monitor.HotCategoryUSSP500
	case "US-ETF":
		return monitor.HotCategoryUSETF
	case "HK-MAIN", "HK-GEM":
		return monitor.HotCategoryHK
	case "HK-ETF":
		return monitor.HotCategoryHKETF
	default:
		return monitor.HotCategoryCNA
	}
}

// resolveEastMoneyHotFilter maps HotCategory to EastMoney clist fs parameter, market label and currency.
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

// resolveEastMoneyHotSymbol generates a standard stock symbol from the EastMoney returned code and market ID.
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

// isCNHotCategory checks whether the category belongs to the A-share market (including ETFs).
func isCNHotCategory(c monitor.HotCategory) bool {
	return c == monitor.HotCategoryCNA || c == monitor.HotCategoryCNETF
}

// isHKHotCategory checks whether the category belongs to the Hong Kong stock market.
func isHKHotCategory(c monitor.HotCategory) bool {
	return c == monitor.HotCategoryHK || c == monitor.HotCategoryHKETF
}

// isUSHotCategory checks whether the category belongs to the US stock market.
func isUSHotCategory(c monitor.HotCategory) bool {
	switch c {
	case monitor.HotCategoryUSSP500, monitor.HotCategoryUSNasdaq, monitor.HotCategoryUSDow, monitor.HotCategoryUSETF:
		return true
	}
	return false
}

func normaliseHotListOptions(options HotListOptions) HotListOptions {
	options.CNQuoteSource = normaliseHotQuoteSourceID(options.CNQuoteSource)
	options.HKQuoteSource = normaliseHotQuoteSourceID(options.HKQuoteSource)
	options.USQuoteSource = normaliseHotQuoteSourceID(options.USQuoteSource)
	if options.CacheTTL <= 0 {
		options.CacheTTL = defaultHotCacheTTL
	}
	return options
}

func normaliseHotQuoteSourceID(sourceID string) string {
	switch strings.ToLower(strings.TrimSpace(sourceID)) {
	case "yahoo":
		return "yahoo"
	case "alpha-vantage":
		return "alpha-vantage"
	case "twelve-data":
		return "twelve-data"
	case "finnhub":
		return "finnhub"
	case "polygon":
		return "polygon"
	case "sina":
		return "sina"
	case "xueqiu":
		return "xueqiu"
	case "tencent":
		return "tencent"
	default:
		return "eastmoney"
	}
}

func resolveHotQuoteSource(category monitor.HotCategory, options HotListOptions) string {
	if isCNHotCategory(category) {
		return options.CNQuoteSource
	}
	if isHKHotCategory(category) {
		return options.HKQuoteSource
	}
	if isUSHotCategory(category) {
		return options.USQuoteSource
	}
	return "eastmoney"
}

// effectivePoolQuoteSource applies per-category overrides for pool quote sources.
// For HK ETF, EastMoney push2 is unreliable; fall back to Tencent when the
// configured source is the eastmoney default and the user hasn't changed it.
func effectivePoolQuoteSource(category monitor.HotCategory, sourceID string) string {
	if category == monitor.HotCategoryHKETF && sourceID == "eastmoney" {
		return "tencent"
	}
	return sourceID
}

func membershipSourceForCategory(category monitor.HotCategory) string {
	switch category {
	case monitor.HotCategoryCNA, monitor.HotCategoryCNETF:
		return "sina"
	case monitor.HotCategoryHK, monitor.HotCategoryHKETF:
		return "xueqiu"
	default:
		return ""
	}
}

func sourceSupportsCategoryList(sourceID string, category monitor.HotCategory) bool {
	switch sourceID {
	case "eastmoney":
		return category == monitor.HotCategoryCNA || category == monitor.HotCategoryHK
	case "sina":
		return category == monitor.HotCategoryCNA || category == monitor.HotCategoryCNETF
	case "xueqiu":
		return category == monitor.HotCategoryCNA || category == monitor.HotCategoryCNETF || category == monitor.HotCategoryHK || category == monitor.HotCategoryHKETF
	default:
		return false
	}
}

func (s *HotService) applyConfiguredQuotes(ctx context.Context, category monitor.HotCategory, items []monitor.HotItem, options HotListOptions) ([]monitor.HotItem, error) {
	sourceID := resolveHotQuoteSource(category, options)

	// EastMoney is the default membership source — no overlay needed.
	if sourceID == "eastmoney" {
		return cloneHotItems(items), nil
	}

	// Look up the provider from the registry.
	var provider monitor.QuoteProvider
	if s.registry != nil {
		provider = s.registry.QuoteProvider(sourceID)
	}

	if provider != nil && hotItemsAlreadyUseSource(items, provider.Name()) {
		return cloneHotItems(items), nil
	}

	if provider == nil {
		return nil, fmt.Errorf("hot quote source is unsupported: %s", sourceID)
	}

	return s.applyProviderQuotes(ctx, items, provider)
}

func (s *HotService) applyProviderQuotes(ctx context.Context, items []monitor.HotItem, provider monitor.QuoteProvider) ([]monitor.HotItem, error) {
	if len(items) == 0 {
		return []monitor.HotItem{}, nil
	}
	watchItems := make([]monitor.WatchlistItem, 0, len(items))
	for _, item := range items {
		watchItems = append(watchItems, monitor.WatchlistItem{
			Symbol:   item.Symbol,
			Name:     item.Name,
			Market:   item.Market,
			Currency: item.Currency,
		})
	}
	quotes, err := provider.Fetch(ctx, watchItems)
	if err != nil {
		return nil, err
	}
	enriched := make([]monitor.HotItem, 0, len(items))
	for _, item := range items {
		target, err := monitor.ResolveQuoteTarget(monitor.WatchlistItem{Symbol: item.Symbol, Name: item.Name, Market: item.Market, Currency: item.Currency})
		if err != nil {
			continue
		}
		quote, ok := quotes[target.Key]
		if !ok {
			continue
		}
		item.Name = firstNonEmpty(quote.Name, item.Name)
		item.Currency = firstNonEmpty(quote.Currency, item.Currency)
		item.CurrentPrice = quote.CurrentPrice
		item.Change = quote.Change
		item.ChangePercent = quote.ChangePercent
		item.QuoteSource = quote.Source
		if quote.Volume > 0 {
			item.Volume = quote.Volume
		}
		if quote.MarketCap > 0 {
			item.MarketCap = quote.MarketCap
		}
		if !quote.UpdatedAt.IsZero() {
			item.UpdatedAt = quote.UpdatedAt
		}
		enriched = append(enriched, item)
	}
	if len(enriched) == 0 {
		return nil, fmt.Errorf("No live hot quotes are available from %s", provider.Name())
	}
	return enriched, nil
}

func hotItemsAlreadyUseSource(items []monitor.HotItem, source string) bool {
	if len(items) == 0 {
		return true
	}

	for _, item := range items {
		if strings.TrimSpace(item.QuoteSource) != source {
			return false
		}
	}
	return true
}

// normaliseHotCategory falls back missing or invalid categories to the default value.
func normaliseHotCategory(c monitor.HotCategory) monitor.HotCategory {
	switch c {
	case monitor.HotCategoryCNA, monitor.HotCategoryCNETF,
		monitor.HotCategoryHK, monitor.HotCategoryHKETF,
		monitor.HotCategoryUSSP500, monitor.HotCategoryUSNasdaq, monitor.HotCategoryUSDow, monitor.HotCategoryUSETF:
		return c
	}
	return monitor.HotCategoryCNA
}

// normaliseHotSort falls back missing or invalid sort fields to the default value.
func normaliseHotSort(s monitor.HotSort) monitor.HotSort {
	switch s {
	case monitor.HotSortVolume, monitor.HotSortGainers, monitor.HotSortLosers, monitor.HotSortMarketCap, monitor.HotSortPrice:
		return s
	}
	return monitor.HotSortVolume
}

// resolveEastMoneySort maps HotSort to EastMoney clist sort field ID and direction.
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

// normaliseEastMoneyCode pads leading zeros for the EastMoney returned code based on marketID.
func normaliseEastMoneyCode(code string, marketID int) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	switch marketID {
	case 116, 128:
		if len(code) < 5 && monitor.IsDigits(code) {
			return strings.Repeat("0", 5-len(code)) + code
		}
	case 0, 1:
		if len(code) < 6 && monitor.IsDigits(code) {
			return strings.Repeat("0", 6-len(code)) + code
		}
	}
	return code
}

func normaliseHotKeyword(keyword string) string {
	return strings.ToLower(strings.TrimSpace(keyword))
}

// filterHotSeeds filters the hotSeed list by keyword, matching items whose name or symbol contains the keyword.
// If the keyword is empty, returns a copy of the original list.
// hotSeed is an item from our predefined data pool containing basic market, symbol and name info,
// used for preliminary filtering during search to reduce the overhead of fetching real-time quotes.
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

// filterHotItems filters the monitor.HotItem list by keyword, matching items whose name or symbol contains the keyword.
// If the keyword is empty, returns a copy of the original list.
// Similar to filterHotSeeds, but operates on monitor.HotItem slices.
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

// hotSearchCacheKey generates the cache key for hot search based on category and sort order.
func hotSearchCacheKey(category monitor.HotCategory, sortBy monitor.HotSort, sourceID string) string {
	return string(category) + "|" + string(sortBy) + "|" + strings.TrimSpace(sourceID)
}

// mergeHotSeeds merges two hotSeed slices and returns a deduplicated new list.
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

func cloneHotItems(items []monitor.HotItem) []monitor.HotItem {
	return append([]monitor.HotItem(nil), items...)
}

func cloneHotListResponse(response monitor.HotListResponse) monitor.HotListResponse {
	response.Items = cloneHotItems(response.Items)
	if response.CacheExpiresAt != nil {
		response.CacheExpiresAt = ptrTime(*response.CacheExpiresAt)
	}
	return response
}

// sortHotItems sorts the hot instrument list in place according to the specified sort order.
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

func hotResponseCacheKey(category monitor.HotCategory, sortBy monitor.HotSort, keyword string, page, pageSize int, options HotListOptions) string {
	return strings.Join([]string{
		string(category),
		string(sortBy),
		keyword,
		strconv.Itoa(page),
		strconv.Itoa(pageSize),
		resolveHotQuoteSource(category, options),
	}, "|")
}

func ptrTime(value time.Time) *time.Time {
	copy := value
	return &copy
}

// searchYahooUSStockSeeds calls Yahoo Finance search API and returns US stock seeds matching the keyword.
func (s *HotService) searchYahooUSStockSeeds(ctx context.Context, keyword string, category monitor.HotCategory) ([]hotSeed, error) {
	parsed, err := fetchYahooSearch(ctx, s.client, keyword)
	if err != nil {
		return nil, err
	}

	seeds := make([]hotSeed, 0, len(parsed.Quotes))
	seen := make(map[string]struct{}, len(parsed.Quotes))
	for _, quote := range parsed.Quotes {
		quoteType := strings.ToUpper(strings.TrimSpace(quote.QuoteType))
		if quoteType != "EQUITY" && quoteType != "" {
			continue
		}
		if !isLikelyUSExchange(quote.Exchange, quote.ExchDisp) {
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
			Market:   "US-STOCK",
			Currency: "USD",
		})
	}
	return seeds, nil
}

// searchYahooUSSeeds fetches a list of US ETF instruments matching the keyword and filters for those likely listed on US exchanges.
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

// fetchEastMoneySuggest calls the EastMoney suggest API to search stocks by keyword (name or code).
// Returns up to the requested number of matching items across all markets.
func fetchEastMoneySuggest(ctx context.Context, client *http.Client, keyword string, count int) ([]eastMoneySuggestItem, error) {
	if client == nil {
		client = &http.Client{}
	}
	if count <= 0 {
		count = 30
	}

	params := url.Values{}
	params.Set("input", strings.TrimSpace(keyword))
	params.Set("type", "14")
	params.Set("token", "D43BF722C8E33BDC906FB84D85E326E8")
	params.Set("count", strconv.Itoa(count))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, datasource.EastMoneySuggestAPI+"?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	setEastMoneyHeaders(req, datasource.EastMoneyWebReferer)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("EastMoney suggest request failed: status %d", resp.StatusCode)
	}

	var parsed eastMoneySuggestResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	return parsed.QuotationCodeTable.Data, nil
}

// searchEastMoneySeeds calls the EastMoney suggest API and returns matching seeds for the given category.
func (s *HotService) searchEastMoneySeeds(ctx context.Context, keyword string, category monitor.HotCategory) []hotSeed {
	items, err := fetchEastMoneySuggest(ctx, s.client, keyword, 30)
	if err != nil {
		s.log.Warn("EastMoney suggest failed", "keyword", keyword, "error", err)
		return nil
	}

	seeds := make([]hotSeed, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		seed, ok := eastMoneySuggestToSeed(item, category)
		if !ok {
			continue
		}
		key := seed.Market + "|" + seed.Symbol
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		seeds = append(seeds, seed)
	}
	return seeds
}

// eastMoneySuggestToSeed converts an EastMoney suggest item to a hotSeed,
// returning false if the item does not belong to the given category.
func eastMoneySuggestToSeed(item eastMoneySuggestItem, category monitor.HotCategory) (hotSeed, bool) {
	code := strings.TrimSpace(item.Code)
	name := strings.TrimSpace(item.Name)
	if code == "" {
		return hotSeed{}, false
	}

	switch item.MktNum {
	case "1": // Shanghai
		if !isCNHotCategory(category) {
			return hotSeed{}, false
		}
		return hotSeed{Symbol: strings.ToUpper(code) + ".SH", Name: name, Market: "CN-A", Currency: "CNY"}, true
	case "0": // Shenzhen
		if !isCNHotCategory(category) {
			return hotSeed{}, false
		}
		return hotSeed{Symbol: strings.ToUpper(code) + ".SZ", Name: name, Market: "CN-A", Currency: "CNY"}, true
	case "128": // Hong Kong
		if !isHKHotCategory(category) {
			return hotSeed{}, false
		}
		return hotSeed{Symbol: strings.ToUpper(code) + ".HK", Name: name, Market: "HK-MAIN", Currency: "HKD"}, true
	default:
		return hotSeed{}, false
	}
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

	return yahooSearchResponse{}, monitor.JoinProblems(problems)
}

// fetchYahooSearchFromHost fetches search results from the specified Yahoo Search API host and parses them into the yahooSearchResponse struct.
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

// isLikelyUSExchange checks whether the given exchange info likely represents a US exchange based on simple matching of common US exchange identifiers.
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
