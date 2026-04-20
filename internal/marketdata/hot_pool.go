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
const yahooHotConcurrency = 5

// eastMoneyHotDiff represents the subset of fields returned by the EastMoney quote diff API used for hot fallback quotes and naming enrichment.
const (
	eastMoneyHotBatchSize     = 50
	eastMoneyHotMaxSecIDChars = 180
)

const sinaPoolConcurrency = 4

type hotSeed struct {
	Symbol   string
	Name     string
	Market   string
	Currency string
}

// fetchPoolQuotes requests real-time quotes in batch for the predefined hot category constituent pool and returns them in a unified format.
func (s *HotService) fetchPoolQuotes(ctx context.Context, seeds []hotSeed, sourceID string, options HotListOptions) ([]monitor.HotItem, error) {
	// Sources with dedicated pool-fetching logic.
	switch sourceID {
	case "yahoo":
		return s.fetchPoolQuotesYahoo(ctx, seeds)
	case "eastmoney":
		return s.fetchPoolQuotesEastMoney(ctx, seeds)
	case "sina":
		if len(seeds) > 0 && (seeds[0].Market == "US-STOCK" || seeds[0].Market == "US-ETF") {
			return s.fetchPoolQuotesSina(ctx, seeds)
		}
	}

	// For all other sources (and Sina for non-US), look up the QuoteProvider
	// from the shared registry instead of constructing a new one.
	if s.registry != nil {
		if provider := s.registry.QuoteProvider(sourceID); provider != nil {
			return s.fetchPoolQuotesWithProvider(ctx, seeds, provider)
		}
	}

	return nil, fmt.Errorf("hot quote source is unsupported: %s", sourceID)
}

func (s *HotService) fetchPoolQuotesEastMoney(ctx context.Context, seeds []hotSeed) ([]monitor.HotItem, error) {
	if len(seeds) > 0 && (seeds[0].Market == "US-STOCK" || seeds[0].Market == "US-ETF") {
		return s.fetchUSPoolQuotesEastMoney(ctx, seeds)
	}

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

func (s *HotService) fetchUSPoolQuotesEastMoney(ctx context.Context, seeds []hotSeed) ([]monitor.HotItem, error) {
	params := url.Values{}
	params.Set("pn", "1")
	params.Set("pz", "20000")
	params.Set("po", "1")
	params.Set("np", "1")
	params.Set("fltt", "1")
	params.Set("invt", "2")
	params.Set("ut", "fa5fd1943c7b386f172d6893dbfba10b")
	params.Set("fid", "f3")
	params.Set("fs", "m:105,m:106,m:107")
	params.Set("dect", "1")
	params.Set("wbp2u", "|0|0|0|web")
	params.Set("fields", "f12,f13,f14,f1,f2,f4,f3,f152,f17,f28,f15,f16,f18,f20,f115")

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, datasource.URLWithQuery(datasource.EastMoneyUSHotAPI, params), nil)
	if err != nil {
		return nil, err
	}
	setEastMoneyHeaders(request, datasource.EastMoneyWebReferer+"center/gridlist.html#us_stocks")

	response, err := s.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("EastMoney hot request failed: status %d", response.StatusCode)
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
		return nil, fmt.Errorf("EastMoney hot response returned rc=%d", parsed.RC)
	}

	seedBySymbol := make(map[string]hotSeed, len(seeds))
	for _, seed := range seeds {
		seedBySymbol[normaliseEastMoneyUSPoolSymbol(seed.Symbol)] = seed
	}

	items := make([]monitor.HotItem, 0, len(seeds))
	seen := make(map[string]struct{}, len(seeds))
	for _, item := range parsed.Data.Diff {
		symbol := normaliseEastMoneyUSPoolSymbol(item.Code)
		seed, ok := seedBySymbol[symbol]
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
	if batchSize <= 0 {
		batchSize = 1
	}

	chunks := make([][]string, 0, (len(secids)+batchSize-1)/batchSize)
	current := make([]string, 0, min(batchSize, len(secids)))
	currentLen := 0
	for _, secid := range secids {
		nextLen := encodedSecIDQueryLength(currentLen, len(current), secid)
		if len(current) >= batchSize || (len(current) > 0 && nextLen > eastMoneyHotMaxSecIDChars) {
			chunks = append(chunks, current)
			current = make([]string, 0, min(batchSize, len(secids)))
			currentLen = 0
			nextLen = encodedSecIDQueryLength(0, 0, secid)
		}
		current = append(current, secid)
		currentLen = nextLen
	}
	if len(current) > 0 {
		chunks = append(chunks, current)
	}
	return chunks
}

func encodedSecIDQueryLength(currentLen, currentCount int, secid string) int {
	nextLen := currentLen + len(secid)
	if currentCount > 0 {
		// Commas become `%2C` in url.Values.Encode().
		nextLen += 3
	}
	return nextLen
}

func normaliseEastMoneyUSPoolSymbol(symbol string) string {
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	return strings.ReplaceAll(symbol, ".", "-")
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
	setEastMoneyHeaders(request, datasource.EastMoneyWebReferer)

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

func (s *HotService) fetchPoolQuotesSina(ctx context.Context, seeds []hotSeed) ([]monitor.HotItem, error) {
	if len(seeds) == 0 {
		return nil, fmt.Errorf("Hot fallback quote response is empty")
	}

	items := make([]monitor.WatchlistItem, 0, len(seeds))
	targetByCode := make(map[string]monitor.QuoteTarget, len(seeds))
	itemByKey := make(map[string]monitor.WatchlistItem, len(seeds))
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
		code, err := resolveSinaQuoteCode(target)
		if err != nil {
			continue
		}
		items = append(items, item)
		targetByCode[code] = target
		itemByKey[target.Key] = item
	}

	codes := mapsKeys(targetByCode)
	if len(codes) == 0 {
		return nil, fmt.Errorf("No quote symbols are available in the hot fallback pool")
	}

	sinaHeaders := map[string]string{
		"Referer":    datasource.SinaFinanceReferer,
		"User-Agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
	}

	// Fetch in concurrent batches to avoid overly long URLs that cause timeouts.
	type batchResult struct {
		text string
		err  error
	}

	batches := chunkStrings(codes, sinaBatchSize)
	results := make([]batchResult, len(batches))
	sem := make(chan struct{}, sinaPoolConcurrency)
	var wg sync.WaitGroup

	for i, batch := range batches {
		wg.Add(1)
		go func(idx int, batch []string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			text, err := fetchTextWithHeaders(ctx, s.client, datasource.SinaQuoteAPI+strings.Join(batch, ","), sinaHeaders, true)
			results[idx] = batchResult{text: text, err: err}
		}(i, batch)
	}
	wg.Wait()

	hotItems := make([]monitor.HotItem, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	var anySuccess bool

	for _, result := range results {
		if result.err != nil {
			continue
		}
		anySuccess = true
		for _, line := range strings.Split(result.text, "\n") {
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
			hotItem, ok := buildSinaHotItem(item, code, fields)
			if !ok {
				continue
			}
			if _, exists := seen[target.Key]; exists {
				continue
			}
			seen[target.Key] = struct{}{}
			hotItems = append(hotItems, hotItem)
		}
	}

	if !anySuccess {
		// Return the first batch error if none succeeded.
		for _, result := range results {
			if result.err != nil {
				return nil, result.err
			}
		}
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
			Volume:        quote.Volume,
			MarketCap:     quote.MarketCap,
			UpdatedAt:     quote.UpdatedAt,
		})
	}

	if len(hotItems) == 0 {
		return nil, fmt.Errorf("Hot fallback quote response is empty")
	}

	return hotItems, nil
}

func (s *HotService) fetchYahooQuotesConcurrent(ctx context.Context, items []monitor.WatchlistItem) (map[string]monitor.Quote, error) {
	var provider monitor.QuoteProvider
	if s.registry != nil {
		provider = s.registry.QuoteProvider("yahoo")
	}
	if provider == nil {
		provider = NewYahooQuoteProvider(s.client)
	}

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

func buildSinaHotItem(item monitor.WatchlistItem, code string, fields []string) (monitor.HotItem, bool) {
	quote, ok := buildSinaQuote(item, code, fields)
	if !ok {
		return monitor.HotItem{}, false
	}

	hotItem := monitor.HotItem{
		Symbol:        item.Symbol,
		Name:          firstNonEmpty(quote.Name, item.Name, item.Symbol),
		Market:        item.Market,
		Currency:      firstNonEmpty(quote.Currency, item.Currency),
		CurrentPrice:  quote.CurrentPrice,
		Change:        quote.Change,
		ChangePercent: quote.ChangePercent,
		QuoteSource:   quote.Source,
		UpdatedAt:     quote.UpdatedAt,
	}

	if strings.HasPrefix(code, "gb_") {
		hotItem.Volume = parseFloat(partsAt(fields, 10))
		hotItem.MarketCap = parseFloat(partsAt(fields, 12))
	}

	return hotItem, true
}

func mapsKeys[T any](value map[string]T) []string {
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	return keys
}
