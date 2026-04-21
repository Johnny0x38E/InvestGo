package hot

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"investgo/internal/core"
)

// loadCachedItems loads the hot instrument list from cache;
// returns false if cache is missing or expired.
func (s *HotService) loadCachedItems(key string) ([]core.HotItem, bool) {
	cached, _, ok := s.searchCache.Get(key)
	if !ok {
		return nil, false
	}
	return cloneHotItems(cached), true
}

// storeCachedItems stores the hot instrument list into cache and sets an expiration time.
func (s *HotService) storeCachedItems(key string, items []core.HotItem, ttl time.Duration) {
	if ttl <= 0 {
		ttl = defaultHotCacheTTL
	}
	s.searchCache.Set(key, cloneHotItems(items), ttl)
}

func (s *HotService) loadCachedResponse(key string) (core.HotListResponse, bool) {
	cached, expiresAt, ok := s.responseCache.Get(key)
	if !ok {
		return core.HotListResponse{}, false
	}
	response := cloneHotListResponse(cached)
	response.Cached = true
	response.CacheExpiresAt = ptrTime(expiresAt)
	return response, true
}

func (s *HotService) storeCachedResponse(key string, response core.HotListResponse, ttl time.Duration) time.Time {
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

// listAllSearchableItems lists all searchable instruments for the given category, used for full-match search.
// For categories backed by upstream ranking APIs, fetch and cache the full list;
// for pool-backed categories, use the maintained local pool directly.
func (s *HotService) listAllSearchableItems(ctx context.Context, category core.HotCategory, sortBy core.HotSort, options HotListOptions) ([]core.HotItem, error) {
	cacheKey := hotSearchCacheKey(category, sortBy, resolveHotQuoteSource(category, options))
	if !options.BypassCache {
		if items, ok := s.loadCachedItems(cacheKey); ok {
			return items, nil
		}
	}

	var (
		items []core.HotItem
		err   error
	)

	switch {
	case category == core.HotCategoryCNA ||
		category == core.HotCategoryCNETF ||
		category == core.HotCategoryHK ||
		category == core.HotCategoryHKETF:
		items, err = s.fetchAllHotPages(ctx, func(ctx context.Context, page, pageSize int) (core.HotListResponse, error) {
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
func (s *HotService) fetchAllHotPages(ctx context.Context, loadPage func(context.Context, int, int) (core.HotListResponse, error)) ([]core.HotItem, error) {
	all := make([]core.HotItem, 0, hotSearchFetchSize)
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

// hotSearchCacheKey generates the cache key for hot search based on category and sort order.
func hotSearchCacheKey(category core.HotCategory, sortBy core.HotSort, sourceID string) string {
	return string(category) + "|" + string(sortBy) + "|" + strings.TrimSpace(sourceID)
}

func hotResponseCacheKey(category core.HotCategory, sortBy core.HotSort, keyword string, page, pageSize int, options HotListOptions) string {
	return strings.Join([]string{
		string(category),
		string(sortBy),
		keyword,
		strconv.Itoa(page),
		strconv.Itoa(pageSize),
		resolveHotQuoteSource(category, options),
	}, "|")
}

func cloneHotItems(items []core.HotItem) []core.HotItem {
	return append([]core.HotItem(nil), items...)
}

func cloneHotListResponse(response core.HotListResponse) core.HotListResponse {
	response.Items = cloneHotItems(response.Items)
	if response.CacheExpiresAt != nil {
		response.CacheExpiresAt = ptrTime(*response.CacheExpiresAt)
	}
	return response
}
