package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"investgo/internal/datasource"
	"investgo/internal/monitor"
)

// fetchYahooQuotesConcurrent fetches Yahoo quotes for a list of items using concurrent goroutines.
// Up to yahooHotConcurrency requests run in parallel, with results merged into a single map.
const yahooHotConcurrency = 10

// eastMoneyHotDiff represents the subset of fields returned by the EastMoney quote diff API used for hot fallback quotes and naming enrichment.
const eastMoneyHotBatchSize = 180

type hotSeed struct {
	Symbol   string
	Name     string
	Market   string
	Currency string
}

// fetchPoolQuotes requests real-time quotes in batch for the predefined hot category constituent pool and returns them in a unified format.
func (s *HotService) fetchPoolQuotes(ctx context.Context, seeds []hotSeed, sourceID string) ([]monitor.HotItem, error) {
	switch sourceID {
	case "yahoo":
		return s.fetchPoolQuotesYahoo(ctx, seeds)
	case "sina":
		return s.fetchPoolQuotesWithProvider(ctx, seeds, NewSinaQuoteProvider(s.client))
	case "xueqiu":
		return s.fetchPoolQuotesWithProvider(ctx, seeds, NewXueqiuQuoteProvider(s.client))
	case "eastmoney":
		return s.fetchPoolQuotesEastMoney(ctx, seeds)
	default:
		return nil, fmt.Errorf("Hot quote source is unsupported: %s", sourceID)
	}
}

func (s *HotService) enrichUSHotItemsWithEastMoneyNames(ctx context.Context, items []monitor.HotItem) ([]monitor.HotItem, error) {
	if len(items) == 0 {
		return []monitor.HotItem{}, nil
	}

	// EastMoney naming is used as a best-effort display enrichment for US rows.
	// It should not change the configured quote source or make the hot list fail
	// when the auxiliary naming lookup is unavailable.
	seeds := make([]hotSeed, 0, len(items))
	for _, item := range items {
		if item.Market != "US-STOCK" && item.Market != "US-ETF" {
			continue
		}
		seeds = append(seeds, hotSeed{
			Symbol:   item.Symbol,
			Name:     item.Name,
			Market:   item.Market,
			Currency: firstNonEmpty(item.Currency, "USD"),
		})
	}

	if len(seeds) == 0 {
		return items, nil
	}

	names, err := s.fetchEastMoneyPoolNames(ctx, seeds)
	if err != nil {
		return items, nil
	}

	enriched := append([]monitor.HotItem(nil), items...)
	for index := range enriched {
		key := enriched[index].Market + "|" + strings.ToUpper(enriched[index].Symbol)
		if name := strings.TrimSpace(names[key]); name != "" {
			enriched[index].Name = name
		}
	}
	return enriched, nil
}

func (s *HotService) fetchEastMoneyPoolNames(ctx context.Context, seeds []hotSeed) (map[string]string, error) {
	secids := make([]string, 0, len(seeds)*2)
	indexBySecID := make(map[string]hotSeed, len(seeds)*2)
	for _, seed := range seeds {
		ids, err := resolveAllPoolSecIDs(seed)
		if err != nil {
			continue
		}
		for _, secid := range ids {
			secids = append(secids, secid)
			indexBySecID[secid] = seed
		}
	}

	if len(secids) == 0 {
		return nil, fmt.Errorf("No quote symbols are available in the hot fallback pool")
	}

	diffs, err := s.fetchEastMoneyHotDiffs(ctx, secids, "f12,f13,f14")
	if err != nil {
		return nil, err
	}

	names := make(map[string]string, len(diffs))
	for _, item := range diffs {
		secid := fmt.Sprintf("%d.%s", item.MarketID, normaliseEastMoneyCode(item.Code, item.MarketID))
		seed, ok := indexBySecID[secid]
		if !ok {
			continue
		}
		name := strings.TrimSpace(item.Name)
		if name == "" {
			continue
		}
		names[seed.Market+"|"+strings.ToUpper(seed.Symbol)] = name
	}
	return names, nil
}

func (s *HotService) fetchPoolQuotesEastMoney(ctx context.Context, seeds []hotSeed) ([]monitor.HotItem, error) {
	secids := make([]string, 0, len(seeds)*2)
	indexBySecID := make(map[string]hotSeed, len(seeds)*2)
	for _, seed := range seeds {
		ids, err := resolveAllPoolSecIDs(seed)
		if err != nil {
			continue
		}
		for _, secid := range ids {
			secids = append(secids, secid)
			indexBySecID[secid] = seed
		}
	}

	if len(secids) == 0 {
		return nil, fmt.Errorf("No quote symbols are available in the hot fallback pool")
	}

	// US pools expand quickly because each ticker fans out to several exchange
	// guesses. Chunking keeps the request URL below the point where EastMoney
	// starts returning upstream 502 responses.
	diffs, err := s.fetchEastMoneyHotDiffs(ctx, secids, "f2,f3,f4,f5,f12,f13,f14,f20")
	if err != nil {
		return nil, err
	}

	items := make([]monitor.HotItem, 0, len(diffs))
	seen := make(map[string]struct{}, len(diffs))
	for _, item := range diffs {
		secid := fmt.Sprintf("%d.%s", item.MarketID, normaliseEastMoneyCode(item.Code, item.MarketID))
		seed, ok := indexBySecID[secid]
		if !ok {
			continue
		}

		key := seed.Market + "|" + seed.Symbol
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}

		items = append(items, monitor.HotItem{
			Symbol:        seed.Symbol,
			Name:          firstNonEmpty(item.Name, seed.Name),
			Market:        seed.Market,
			Currency:      seed.Currency,
			CurrentPrice:  float64(item.CurrentPrice),
			Change:        float64(item.Change),
			ChangePercent: float64(item.ChangePercent),
			Volume:        float64(item.Volume),
			MarketCap:     float64(item.MarketCap),
			QuoteSource:   "EastMoney",
			UpdatedAt:     time.Now(),
		})
	}

	if len(items) == 0 {
		return nil, fmt.Errorf("Hot fallback quote response is empty")
	}

	return items, nil
}

func (s *HotService) fetchEastMoneyHotDiffs(ctx context.Context, secids []string, fields string) ([]eastMoneyHotDiff, error) {
	diffs := make([]eastMoneyHotDiff, 0, len(secids))
	for _, batch := range chunkSecIDs(secids, eastMoneyHotBatchSize) {
		batchDiffs, err := s.fetchEastMoneyHotDiffBatch(ctx, batch, fields)
		if err != nil {
			return nil, err
		}
		diffs = append(diffs, batchDiffs...)
	}
	return diffs, nil
}

func chunkSecIDs(secids []string, batchSize int) [][]string {
	if len(secids) == 0 {
		return nil
	}

	chunks := make([][]string, 0, (len(secids)+batchSize-1)/batchSize)
	for start := 0; start < len(secids); start += batchSize {
		end := min(start+batchSize, len(secids))
		chunks = append(chunks, secids[start:end])
	}
	return chunks
}

func (s *HotService) fetchEastMoneyHotDiffBatch(ctx context.Context, secids []string, fields string) ([]eastMoneyHotDiff, error) {
	// Keep the single-batch request focused on transport and decoding so the
	// caller can reason about chunking and aggregation separately.
	params := url.Values{}
	params.Set("fltt", "2")
	params.Set("invt", "2")
	params.Set("np", "1")
	params.Set("ut", "bd1d9ddb04089700cf9c27f6f7426281")
	params.Set("fields", fields)
	params.Set("secids", strings.Join(secids, ","))

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, datasource.URLWithQuery(datasource.EastMoneyQuoteAPI, params), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Referer", datasource.EastMoneyWebReferer)
	request.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/123.0.0.0 Safari/537.36")

	response, err := s.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	payload, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Hot fallback quote request failed: status %d", response.StatusCode)
	}

	var parsed eastMoneyHotResponse
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return nil, err
	}
	if parsed.RC != 0 {
		return nil, fmt.Errorf("Hot fallback quote response returned rc=%d", parsed.RC)
	}

	return parsed.Data.Diff, nil
}

func (s *HotService) fetchPoolQuotesYahoo(ctx context.Context, seeds []hotSeed) ([]monitor.HotItem, error) {
	if len(seeds) == 0 {
		return nil, fmt.Errorf("Hot fallback quote response is empty")
	}

	// Build WatchlistItem list for all seeds.
	items := make([]monitor.WatchlistItem, 0, len(seeds))
	for _, seed := range seeds {
		items = append(items, monitor.WatchlistItem{
			Symbol:   seed.Symbol,
			Name:     seed.Name,
			Market:   seed.Market,
			Currency: seed.Currency,
		})
	}

	// Fetch Yahoo quotes concurrently in small batches.
	quotes, err := s.fetchYahooQuotesConcurrent(ctx, items)
	if err != nil {
		return nil, err
	}

	// Map results back to HotItem list, deduplicating by target key.
	hotItems := make([]monitor.HotItem, 0, len(quotes))
	seen := make(map[string]struct{}, len(quotes))
	for _, seed := range seeds {
		item := monitor.WatchlistItem{
			Symbol:   seed.Symbol,
			Name:     seed.Name,
			Market:   seed.Market,
			Currency: seed.Currency,
		}
		target, err := monitor.ResolveQuoteTarget(item)
		if err != nil {
			continue
		}
		if _, exists := seen[target.Key]; exists {
			continue
		}
		quote, ok := quotes[target.Key]
		if !ok {
			continue
		}
		seen[target.Key] = struct{}{}

		hotItems = append(hotItems, monitor.HotItem{
			Symbol:        seed.Symbol,
			Name:          firstNonEmpty(quote.Name, seed.Name),
			Market:        seed.Market,
			Currency:      firstNonEmpty(quote.Currency, seed.Currency),
			CurrentPrice:  quote.CurrentPrice,
			Change:        quote.Change,
			ChangePercent: quote.ChangePercent,
			QuoteSource:   quote.Source,
			Volume:        0, // Yahoo chart API does not provide aggregate daily volume for batch quotes
			MarketCap:     0, // Yahoo chart API does not provide market cap
			UpdatedAt:     quote.UpdatedAt,
		})
	}

	if len(hotItems) == 0 {
		return nil, fmt.Errorf("Hot fallback quote response is empty")
	}

	return hotItems, nil
}

func (s *HotService) fetchPoolQuotesWithProvider(ctx context.Context, seeds []hotSeed, provider monitor.QuoteProvider) ([]monitor.HotItem, error) {
	if len(seeds) == 0 {
		return nil, fmt.Errorf("Hot fallback quote response is empty")
	}
	if provider == nil {
		return nil, fmt.Errorf("Hot quote provider is not configured")
	}

	items := make([]monitor.WatchlistItem, 0, len(seeds))
	for _, seed := range seeds {
		items = append(items, monitor.WatchlistItem{
			Symbol:   seed.Symbol,
			Name:     seed.Name,
			Market:   seed.Market,
			Currency: seed.Currency,
		})
	}

	quotes, err := provider.Fetch(ctx, items)
	if err != nil {
		return nil, err
	}

	hotItems := make([]monitor.HotItem, 0, len(quotes))
	seen := make(map[string]struct{}, len(quotes))
	for _, seed := range seeds {
		item := monitor.WatchlistItem{
			Symbol:   seed.Symbol,
			Name:     seed.Name,
			Market:   seed.Market,
			Currency: seed.Currency,
		}
		target, err := monitor.ResolveQuoteTarget(item)
		if err != nil {
			continue
		}
		if _, exists := seen[target.Key]; exists {
			continue
		}
		quote, ok := quotes[target.Key]
		if !ok {
			continue
		}
		seen[target.Key] = struct{}{}

		hotItems = append(hotItems, monitor.HotItem{
			Symbol:        seed.Symbol,
			Name:          firstNonEmpty(quote.Name, seed.Name),
			Market:        seed.Market,
			Currency:      firstNonEmpty(quote.Currency, seed.Currency),
			CurrentPrice:  quote.CurrentPrice,
			Change:        quote.Change,
			ChangePercent: quote.ChangePercent,
			QuoteSource:   quote.Source,
			Volume:        0,
			MarketCap:     0,
			UpdatedAt:     quote.UpdatedAt,
		})
	}

	if len(hotItems) == 0 {
		return nil, fmt.Errorf("Hot fallback quote response is empty")
	}

	return hotItems, nil
}

func (s *HotService) fetchYahooQuotesConcurrent(ctx context.Context, items []monitor.WatchlistItem) (map[string]monitor.Quote, error) {
	provider := NewYahooQuoteProvider(s.client)

	type result struct {
		quotes map[string]monitor.Quote
		err    error
	}

	results := make([]result, len(items))
	sem := make(chan struct{}, yahooHotConcurrency)
	var wg sync.WaitGroup

	for i, item := range items {
		wg.Add(1)
		go func(idx int, it monitor.WatchlistItem) {
			defer wg.Done()
			sem <- struct{}{}        // acquire
			defer func() { <-sem }() // release

			q, err := provider.Fetch(ctx, []monitor.WatchlistItem{it})
			results[idx] = result{quotes: q, err: err}
		}(i, item)
	}

	wg.Wait()

	merged := make(map[string]monitor.Quote, len(items))
	var problems []string
	for i, r := range results {
		if r.err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", items[i].Symbol, r.err))
			continue
		}
		for k, v := range r.quotes {
			merged[k] = v
		}
	}

	if len(merged) == 0 {
		return nil, fmt.Errorf("all Yahoo quote requests failed: %s", strings.Join(problems, "; "))
	}

	return merged, nil
}

// resolveAllPoolSecIDs returns all possible secids for the seed instrument.
// For US stocks, it returns the 105/106/107 variants to cover NASDAQ, NYSE and NYSE Arca.
func resolveAllPoolSecIDs(seed hotSeed) ([]string, error) {
	target, err := monitor.ResolveQuoteTarget(monitor.WatchlistItem{
		Symbol:   seed.Symbol,
		Market:   seed.Market,
		Currency: seed.Currency,
	})
	if err != nil {
		return nil, err
	}
	return resolveAllEastMoneySecIDs(target)
}
