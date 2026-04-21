package hot

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	ttlcache "investgo/internal/common/cache"
	"investgo/internal/core/marketdata"
	"investgo/internal/core"
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
	registry      *marketdata.Registry
	searchCache   *ttlcache.TTL[string, []core.HotItem]
	responseCache *ttlcache.TTL[string, core.HotListResponse]
}

// NewHotService creates a hot list service.
func NewHotService(client *http.Client, logger *slog.Logger, registry *marketdata.Registry) *HotService {
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
		searchCache:   ttlcache.NewTTL[string, []core.HotItem](),
		responseCache: ttlcache.NewTTL[string, core.HotListResponse](),
	}
}

// List returns the hot list for the given category and sort order.
func (s *HotService) List(ctx context.Context, category core.HotCategory, sortBy core.HotSort, keyword string, page, pageSize int, options HotListOptions) (core.HotListResponse, error) {
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

	var response core.HotListResponse
	var err error
	if keyword != "" {
		response, err = s.search(ctx, category, sortBy, keyword, page, pageSize, options)
	} else {
		switch {
		case category == core.HotCategoryCNA,
			category == core.HotCategoryCNETF,
			category == core.HotCategoryHK:
			response, err = s.listConfiguredCategory(ctx, category, sortBy, page, pageSize, options)
		case category == core.HotCategoryHKETF,
			isUSHotCategory(category):
			response, err = s.listFromPool(ctx, category, sortBy, page, pageSize, options)
		default:
			err = fmt.Errorf("Hot category is unsupported: %s", category)
		}
	}
	if err != nil {
		return core.HotListResponse{}, err
	}

	response.Cached = false
	expiresAt := s.storeCachedResponse(cacheKey, response, options.CacheTTL)
	response.CacheExpiresAt = ptrTime(expiresAt)
	return response, nil
}

// search filters the data pool by keyword. Each market uses a lightweight search approach:
// CN/HK uses EastMoney suggest API, US equities use local seed filtering + Yahoo search,
// US ETFs combine local pool + Yahoo search.
func (s *HotService) search(ctx context.Context, category core.HotCategory, sortBy core.HotSort, keyword string, page, pageSize int, options HotListOptions) (core.HotListResponse, error) {
	if category == core.HotCategoryUSETF {
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

	return core.HotListResponse{}, fmt.Errorf("Hot search is unsupported for category: %s", category)
}

// searchUSETFs handles US ETF search specially:
// filter from the pool first, then call the Yahoo Finance search API for more matches,
// merge and deduplicate, and fetch real-time quotes.
func (s *HotService) searchUSETFs(ctx context.Context, sortBy core.HotSort, keyword string, page, pageSize int, options HotListOptions) (core.HotListResponse, error) {
	seeds := filterHotSeeds(normalizedUSHotSeeds(core.HotCategoryUSETF, hotConstituents[core.HotCategoryUSETF]), keyword)

	remoteSeeds, err := s.searchYahooUSSeeds(ctx, keyword)
	if err == nil {
		seeds = mergeHotSeeds(seeds, remoteSeeds)
	}

	items, err := s.loadHotItemsForSeeds(ctx, seeds, options)
	if err != nil {
		return core.HotListResponse{}, err
	}

	sortHotItems(items, sortBy)
	start, end := paginateHotItems(len(items), page, pageSize)
	return core.HotListResponse{
		Category:    core.HotCategoryUSETF,
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
func (s *HotService) searchUSStocks(ctx context.Context, category core.HotCategory, sortBy core.HotSort, keyword string, page, pageSize int, options HotListOptions) (core.HotListResponse, error) {
	pool := normalizedUSHotSeeds(category, hotConstituents[category])

	// Filter seeds locally — no network I/O.
	seeds := filterHotSeeds(pool, keyword)

	// Call Yahoo search for broader coverage (e.g. name-based search).
	remoteSeeds, err := s.searchYahooUSStockSeeds(ctx, keyword, category)
	if err == nil && len(remoteSeeds) > 0 {
		seeds = mergeHotSeeds(seeds, remoteSeeds)
	}

	if len(seeds) == 0 {
		return core.HotListResponse{
			Category:    category,
			Sort:        sortBy,
			Page:        page,
			PageSize:    pageSize,
			Total:       0,
			HasMore:     false,
			Items:       []core.HotItem{},
			GeneratedAt: time.Now(),
		}, nil
	}

	// Only fetch quotes for the (small) set of matching seeds.
	items, err := s.loadHotItemsForSeeds(ctx, seeds, options)
	if err != nil {
		return core.HotListResponse{}, err
	}

	sortHotItems(items, sortBy)
	start, end := paginateHotItems(len(items), page, pageSize)
	return core.HotListResponse{
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
func (s *HotService) searchCNHK(ctx context.Context, category core.HotCategory, sortBy core.HotSort, keyword string, page, pageSize int, options HotListOptions) (core.HotListResponse, error) {
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
		return core.HotListResponse{
			Category:    category,
			Sort:        sortBy,
			Page:        page,
			PageSize:    pageSize,
			Total:       0,
			HasMore:     false,
			Items:       []core.HotItem{},
			GeneratedAt: time.Now(),
		}, nil
	}

	// Fetch quotes only for the small set of matching seeds.
	items, err := s.loadHotItemsForSeeds(ctx, seeds, options)
	if err != nil {
		return core.HotListResponse{}, err
	}

	sortHotItems(items, sortBy)
	start, end := paginateHotItems(len(items), page, pageSize)
	return core.HotListResponse{
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

// listFromPool returns paginated hot list results using the predefined data pool + real-time quotes.
func (s *HotService) listFromPool(ctx context.Context, category core.HotCategory, sortBy core.HotSort, page, pageSize int, options HotListOptions) (core.HotListResponse, error) {
	items, err := s.loadPoolItems(ctx, category, sortBy, options)
	if err != nil {
		return core.HotListResponse{}, err
	}

	start, end := paginateHotItems(len(items), page, pageSize)
	return core.HotListResponse{
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

func (s *HotService) listConfiguredCategory(ctx context.Context, category core.HotCategory, sortBy core.HotSort, page, pageSize int, options HotListOptions) (core.HotListResponse, error) {
	sourceID := resolveHotQuoteSource(category, options)

	if sourceID == "yahoo" {
		return core.HotListResponse{}, fmt.Errorf("Yahoo hot list is unsupported for category: %s", category)
	}

	if sourceSupportsCategoryList(sourceID, category) {
		return s.listCategoryBySource(ctx, sourceID, category, sortBy, page, pageSize)
	}

	return s.listConfiguredCategoryWithOverlay(ctx, category, sortBy, page, pageSize, options)
}

func (s *HotService) listConfiguredCategoryWithOverlay(ctx context.Context, category core.HotCategory, sortBy core.HotSort, page, pageSize int, options HotListOptions) (core.HotListResponse, error) {
	baseSource := membershipSourceForCategory(category)
	if baseSource == "" {
		return core.HotListResponse{}, fmt.Errorf("Hot quote source is unsupported: %s", resolveHotQuoteSource(category, options))
	}

	response, err := s.listCategoryBySource(ctx, baseSource, category, sortBy, page, pageSize)
	if err != nil {
		return core.HotListResponse{}, err
	}

	items, err := s.applyConfiguredQuotes(ctx, category, response.Items, options)
	if err != nil {
		return core.HotListResponse{}, err
	}
	sortHotItems(items, sortBy)
	response.Items = items
	response.GeneratedAt = time.Now()
	return response, nil
}

func (s *HotService) listCategoryBySource(ctx context.Context, sourceID string, category core.HotCategory, sortBy core.HotSort, page, pageSize int) (core.HotListResponse, error) {
	switch sourceID {
	case "eastmoney":
		return s.listEastMoney(ctx, category, sortBy, page, pageSize)
	case "sina":
		return s.listSina(ctx, category, sortBy, page, pageSize)
	case "xueqiu":
		return s.listXueqiu(ctx, category, sortBy, page, pageSize)
	default:
		return core.HotListResponse{}, fmt.Errorf("Hot quote source is unsupported: %s", sourceID)
	}
}

// loadPoolItems loads instruments from the predefined data pool and fetches real-time quotes.
func (s *HotService) loadPoolItems(ctx context.Context, category core.HotCategory, sortBy core.HotSort, options HotListOptions) ([]core.HotItem, error) {
	var pool []hotSeed
	if category == core.HotCategoryHKETF {
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
func (s *HotService) loadHotItemsForSeeds(ctx context.Context, seeds []hotSeed, options HotListOptions) ([]core.HotItem, error) {
	if len(seeds) == 0 {
		return []core.HotItem{}, nil
	}

	category := categoryForHotSeeds(seeds)
	sourceID := effectivePoolQuoteSource(category, resolveHotQuoteSource(category, options))
	return s.fetchPoolQuotes(ctx, seeds, sourceID, options)
}

// categoryForHotSeeds infers the HotCategory from the market field of the first seed.
func categoryForHotSeeds(seeds []hotSeed) core.HotCategory {
	if len(seeds) == 0 {
		return core.HotCategoryCNA
	}
	switch seeds[0].Market {
	case "US-STOCK":
		return core.HotCategoryUSSP500
	case "US-ETF":
		return core.HotCategoryUSETF
	case "HK-MAIN", "HK-GEM":
		return core.HotCategoryHK
	case "HK-ETF":
		return core.HotCategoryHKETF
	default:
		return core.HotCategoryCNA
	}
}
