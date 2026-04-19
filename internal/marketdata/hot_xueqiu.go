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

const xueqiuScreenerAPI = "https://xueqiu.com/service/v5/stock/screener/quote/list"

// xueqiuScreenerResponse models the JSON envelope returned by the Xueqiu
// stock screener list API.  Numeric fields use pointer types because the
// upstream response may contain JSON null for any of them.
type xueqiuScreenerResponse struct {
	Data struct {
		Count int `json:"count"`
		List  []struct {
			Symbol        string   `json:"symbol"`
			Name          string   `json:"name"`
			Current       *float64 `json:"current"`
			Chg           *float64 `json:"chg"`
			Percent       *float64 `json:"percent"`
			Volume        *float64 `json:"volume"`
			Amount        *float64 `json:"amount"`
			MarketCapital *float64 `json:"market_capital"`
		} `json:"list"`
	} `json:"data"`
	ErrorCode        int    `json:"error_code"`
	ErrorDescription string `json:"error_description"`
}

// listXueqiu fetches a page of hot-list items from the Xueqiu screener API.
// It supports the HK and CN-A categories; other categories return an error.
func (s *HotService) listXueqiu(ctx context.Context, category monitor.HotCategory, sortBy monitor.HotSort, page, pageSize int) (monitor.HotListResponse, error) {
	market, typ, mkt, currency, err := resolveXueqiuMarket(category)
	if err != nil {
		return monitor.HotListResponse{}, err
	}

	orderBy, order := resolveXueqiuSort(sortBy)

	s.log.Info("hot list: using Xueqiu ranking", "category", category, "sort", sortBy, "page", page)

	params := url.Values{}
	params.Set("page", strconv.Itoa(page))
	params.Set("size", strconv.Itoa(pageSize))
	params.Set("order", order)
	params.Set("order_by", orderBy)
	params.Set("market", market)
	params.Set("type", typ)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, xueqiuScreenerAPI+"?"+params.Encode(), nil)
	if err != nil {
		return monitor.HotListResponse{}, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")

	resp, err := s.client.Do(req)
	if err != nil {
		return monitor.HotListResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return monitor.HotListResponse{}, fmt.Errorf("Xueqiu screener request failed: status %d", resp.StatusCode)
	}

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return monitor.HotListResponse{}, err
	}

	var parsed xueqiuScreenerResponse
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return monitor.HotListResponse{}, err
	}
	if parsed.ErrorCode != 0 {
		return monitor.HotListResponse{}, fmt.Errorf("Xueqiu screener error %d: %s", parsed.ErrorCode, parsed.ErrorDescription)
	}

	items := make([]monitor.HotItem, 0, len(parsed.Data.List))
	for _, item := range parsed.Data.List {
		price := derefFloat64(item.Current)
		if price == 0 {
			continue
		}

		symbol := convertXueqiuSymbol(item.Symbol, category)
		if symbol == "" {
			continue
		}

		items = append(items, monitor.HotItem{
			Symbol:        symbol,
			Name:          item.Name,
			Market:        mkt,
			Currency:      currency,
			CurrentPrice:  price,
			Change:        derefFloat64(item.Chg),
			ChangePercent: derefFloat64(item.Percent),
			Volume:        derefFloat64(item.Volume),
			MarketCap:     derefFloat64(item.MarketCapital),
			QuoteSource:   "Xueqiu",
			UpdatedAt:     time.Now(),
		})
	}

	total := parsed.Data.Count
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

// resolveXueqiuSort maps a HotSort value to the Xueqiu order_by and order
// query parameters.
func resolveXueqiuSort(sortBy monitor.HotSort) (orderBy, order string) {
	switch sortBy {
	case monitor.HotSortGainers:
		return "percent", "desc"
	case monitor.HotSortLosers:
		return "percent", "asc"
	case monitor.HotSortMarketCap:
		return "market_capital", "desc"
	case monitor.HotSortPrice:
		return "current", "desc"
	default: // volume
		return "volume", "desc"
	}
}

// resolveXueqiuMarket maps a HotCategory to the Xueqiu market/type query
// parameters and the display market string and currency used in HotItem.
func resolveXueqiuMarket(category monitor.HotCategory) (market, typ string, mkt string, currency string, err error) {
	switch category {
	case monitor.HotCategoryHK:
		return "HK", "hk", "HK-MAIN", "HKD", nil
	case monitor.HotCategoryHKETF:
		return "HK", "hk_etf", "HK-ETF", "HKD", nil
	case monitor.HotCategoryCNA:
		return "CN", "sh_sz", "CN-A", "CNY", nil
	case monitor.HotCategoryCNETF:
		return "CN", "sh_sz_etf", "CN-ETF", "CNY", nil
	default:
		return "", "", "", "", fmt.Errorf("Xueqiu screener does not support category: %s", category)
	}
}

// convertXueqiuSymbol converts a Xueqiu symbol to the application's
// canonical format.
//
//   - HK: "00700" → "00700.HK"
//   - CN: "SH601778" → "601778.SH", "SZ300058" → "300058.SZ"
func convertXueqiuSymbol(raw string, category monitor.HotCategory) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	switch category {
	case monitor.HotCategoryHK, monitor.HotCategoryHKETF:
		return raw + ".HK"
	case monitor.HotCategoryCNA, monitor.HotCategoryCNETF:
		if len(raw) > 2 {
			prefix := strings.ToUpper(raw[:2])
			code := raw[2:]
			if prefix == "SH" || prefix == "SZ" {
				return code + "." + prefix
			}
		}
		return raw
	default:
		return raw
	}
}

// derefFloat64 safely dereferences a *float64, returning 0 if the pointer is nil.
func derefFloat64(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}
