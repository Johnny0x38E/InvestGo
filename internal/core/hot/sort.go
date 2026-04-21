package hot

import (
	"sort"
	"strings"
	"time"

	"investgo/internal/core"
)

// sortHotItems sorts the hot instrument list in place according to the specified sort order.
func sortHotItems(items []core.HotItem, sortBy core.HotSort) {
	sort.SliceStable(items, func(i, j int) bool {
		switch sortBy {
		case core.HotSortGainers:
			return items[i].ChangePercent > items[j].ChangePercent
		case core.HotSortLosers:
			return items[i].ChangePercent < items[j].ChangePercent
		case core.HotSortMarketCap:
			return items[i].MarketCap > items[j].MarketCap
		case core.HotSortPrice:
			return items[i].CurrentPrice > items[j].CurrentPrice
		default:
			return items[i].Volume > items[j].Volume
		}
	})
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

// filterHotItems filters the core.HotItem list by keyword, matching items whose name or symbol contains the keyword.
// If the keyword is empty, returns a copy of the original list.
// Similar to filterHotSeeds, but operates on core.HotItem slices.
func filterHotItems(items []core.HotItem, keyword string) []core.HotItem {
	keyword = normaliseHotKeyword(keyword)
	if keyword == "" {
		return cloneHotItems(items)
	}

	filtered := make([]core.HotItem, 0, len(items))
	for _, item := range items {
		if strings.Contains(strings.ToLower(item.Name), keyword) || strings.Contains(strings.ToLower(item.Symbol), keyword) {
			filtered = append(filtered, item)
		}
	}
	return filtered
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

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func ptrTime(value time.Time) *time.Time {
	copy := value
	return &copy
}
