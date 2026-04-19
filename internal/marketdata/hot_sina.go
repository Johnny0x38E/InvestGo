package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"investgo/internal/monitor"
)

const (
	sinaReferer  = "https://finance.sina.com.cn/"
	sinaHotAPI   = "https://vip.stock.finance.sina.com.cn/quotes_service/api/json_v2.php/Market_Center.getHQNodeData"
	sinaCountAPI = "https://vip.stock.finance.sina.com.cn/quotes_service/api/json_v2.php/Market_Center.getHQNodeStockCount"
)

// sinaHotItem mirrors a single element from the Sina Finance ranking JSON array.
type sinaHotItem struct {
	Symbol        string  `json:"symbol"`
	Code          string  `json:"code"`
	Name          string  `json:"name"`
	Trade         string  `json:"trade"`
	PriceChange   float64 `json:"pricechange"`
	ChangePercent float64 `json:"changepercent"`
	Volume        float64 `json:"volume"`
	MktCap        float64 `json:"mktcap"`
}

// listSina fetches a hot list page from Sina Finance.
// Supports monitor.HotCategoryCNA and monitor.HotCategoryCNETF; other categories return an error.
func (s *HotService) listSina(ctx context.Context, category monitor.HotCategory, sortBy monitor.HotSort, page, pageSize int) (monitor.HotListResponse, error) {
	if category != monitor.HotCategoryCNA && category != monitor.HotCategoryCNETF {
		return monitor.HotListResponse{}, fmt.Errorf("Sina hot category is unsupported: %s", category)
	}

	var node, market string
	switch category {
	case monitor.HotCategoryCNA:
		node = "hs_a"
		market = "CN-A"
	case monitor.HotCategoryCNETF:
		node = "etf_hq_fund"
		market = "CN-ETF"
	default:
		return monitor.HotListResponse{}, fmt.Errorf("Sina hot category is unsupported: %s", category)
	}

	s.log.Info("hot list: using Sina Finance ranking", "category", category, "sort", sortBy, "page", page)

	sortField, asc := resolveSinaSort(sortBy)

	// Fetch total count.
	total, err := s.fetchSinaCount(ctx, node)
	if err != nil {
		return monitor.HotListResponse{}, fmt.Errorf("Sina count request failed: %w", err)
	}

	// Fetch page data.
	params := url.Values{}
	params.Set("page", strconv.Itoa(page))
	params.Set("num", strconv.Itoa(pageSize))
	params.Set("sort", sortField)
	params.Set("asc", strconv.Itoa(asc))
	params.Set("node", node)
	params.Set("_s_r_a", "auto")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sinaHotAPI+"?"+params.Encode(), nil)
	if err != nil {
		return monitor.HotListResponse{}, err
	}
	req.Header.Set("Referer", sinaReferer)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")

	resp, err := s.client.Do(req)
	if err != nil {
		return monitor.HotListResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return monitor.HotListResponse{}, fmt.Errorf("Sina hot request failed: status %d", resp.StatusCode)
	}

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return monitor.HotListResponse{}, err
	}

	var parsed []sinaHotItem
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return monitor.HotListResponse{}, fmt.Errorf("Sina hot response parse error: %w", err)
	}

	items := make([]monitor.HotItem, 0, len(parsed))
	for _, raw := range parsed {
		symbol := convertSinaSymbol(raw.Symbol)
		if symbol == "" {
			continue
		}
		price, _ := strconv.ParseFloat(raw.Trade, 64)
		items = append(items, monitor.HotItem{
			Symbol:        symbol,
			Name:          raw.Name,
			Market:        market,
			Currency:      "CNY",
			CurrentPrice:  price,
			Change:        raw.PriceChange,
			ChangePercent: raw.ChangePercent,
			Volume:        raw.Volume,
			MarketCap:     raw.MktCap * 10000, // mktcap is in 万元
			QuoteSource:   "Sina",
			UpdatedAt:     time.Now(),
		})
	}

	return monitor.HotListResponse{
		Category:    category,
		Sort:        sortBy,
		Page:        page,
		PageSize:    pageSize,
		Total:       total,
		HasMore:     page*pageSize < total,
		Items:       items,
		GeneratedAt: time.Now(),
	}, nil
}
// fetchSinaCount retrieves the total number of instruments from Sina for the given node.
// The API returns a quoted number string like "5505".
func (s *HotService) fetchSinaCount(ctx context.Context, node string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sinaCountAPI+"?node="+node, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Referer", sinaReferer)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")

	resp, err := s.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("Sina count request failed: status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	// Response is a quoted number, e.g. "5505" — unmarshal as a JSON string.
	var numStr string
	if err := json.Unmarshal(body, &numStr); err != nil {
		return 0, fmt.Errorf("Sina count parse error: %w", err)
	}
	return strconv.Atoi(numStr)
}

// resolveSinaSort maps a HotSort value to the corresponding Sina API sort field
// and ascending flag (0 = descending, 1 = ascending).
func resolveSinaSort(sortBy monitor.HotSort) (field string, asc int) {
	switch sortBy {
	case monitor.HotSortGainers:
		return "changepercent", 0
	case monitor.HotSortLosers:
		return "changepercent", 1
	case monitor.HotSortMarketCap:
		return "mktcap", 0
	case monitor.HotSortPrice:
		return "trade", 0
	default: // volume
		return "volume", 0
	}
}

// convertSinaSymbol converts a Sina-style symbol like "sh601778" to our
// canonical format "601778.SH". Returns "" for unrecognised prefixes.
func convertSinaSymbol(symbol string) string {
	if len(symbol) < 3 {
		return ""
	}
	prefix := strings.ToLower(symbol[:2])
	code := symbol[2:]
	switch prefix {
	case "sh":
		return code + ".SH"
	case "sz":
		return code + ".SZ"
	case "bj":
		return code + ".BJ"
	default:
		return ""
	}
}
