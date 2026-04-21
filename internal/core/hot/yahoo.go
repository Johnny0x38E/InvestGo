package hot

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"investgo/internal/core/provider"
	"investgo/internal/common/errs"
	"investgo/internal/core"
	"investgo/internal/core/endpoint"
)

// yahooSearchResponse models the JSON envelope returned by the Yahoo Finance search API.
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

// searchYahooUSStockSeeds calls Yahoo Finance search API and returns US stock seeds matching the keyword.
func (s *HotService) searchYahooUSStockSeeds(ctx context.Context, keyword string, category core.HotCategory) ([]hotSeed, error) {
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
			Name:     provider.FirstNonEmpty(quote.LongName, quote.ShortName, symbol),
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
			Name:     provider.FirstNonEmpty(quote.LongName, quote.ShortName, symbol),
			Market:   "US-ETF",
			Currency: "USD",
		})
	}
	return seeds, nil
}

// fetchYahooSearch queries the Yahoo Finance search API across all configured hosts and
// returns the first successful response. If all hosts fail, the errors are combined and returned.
func fetchYahooSearch(ctx context.Context, client *http.Client, keyword string) (yahooSearchResponse, error) {
	if client == nil {
		client = &http.Client{}
	}

	params := url.Values{}
	params.Set("q", strings.TrimSpace(keyword))
	params.Set("quotesCount", "20")
	params.Set("newsCount", "0")
	params.Set("enableFuzzyQuery", "false")

	problems := make([]string, 0, len(endpoint.YahooSearchHosts))
	for _, host := range endpoint.YahooSearchHosts {
		parsed, err := fetchYahooSearchFromHost(ctx, client, host, params)
		if err == nil {
			return parsed, nil
		}
		problems = append(problems, fmt.Sprintf("%s: %v", host, err))
	}

	return yahooSearchResponse{}, errs.JoinProblems(problems)
}

// fetchYahooSearchFromHost fetches search results from the specified Yahoo Search API host
// and parses them into the yahooSearchResponse struct.
func fetchYahooSearchFromHost(ctx context.Context, client *http.Client, host string, params url.Values) (yahooSearchResponse, error) {
	// Copy params to avoid mutating the shared slice across concurrent calls.
	query := make(url.Values, len(params))
	for key, values := range params {
		query[key] = append([]string(nil), values...)
	}

	requestURL := url.URL{
		Scheme:   "https",
		Host:     host,
		Path:     endpoint.YahooSearchPath,
		RawQuery: query.Encode(),
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL.String(), nil)
	if err != nil {
		return yahooSearchResponse{}, err
	}
	request.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/123.0.0.0 Safari/537.36")
	request.Header.Set("Origin", endpoint.YahooFinanceOrigin)
	request.Header.Set("Referer", endpoint.YahooFinanceReferer)

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

// isYahooETFQuote reports whether the Yahoo Finance quote type fields indicate an ETF.
func isYahooETFQuote(quoteType, typeDisp string) bool {
	quoteType = strings.ToUpper(strings.TrimSpace(quoteType))
	typeDisp = strings.ToUpper(strings.TrimSpace(typeDisp))
	return quoteType == "ETF" || strings.Contains(typeDisp, "ETF")
}

// isLikelyUSExchange reports whether the given exchange fields likely represent a US-listed
// instrument based on well-known US exchange identifiers.
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
