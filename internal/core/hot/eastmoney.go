package hot

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

	"investgo/internal/core/provider"
	"investgo/internal/core"
	"investgo/internal/core/endpoint"
)

// eastMoneySuggestResponse models the JSON envelope returned by the EastMoney suggest API.
type eastMoneySuggestResponse struct {
	QuotationCodeTable struct {
		Data []eastMoneySuggestItem `json:"Data"`
	} `json:"QuotationCodeTable"`
}

// eastMoneySuggestItem represents a single result from the EastMoney suggest API.
type eastMoneySuggestItem struct {
	Code             string `json:"Code"`
	Name             string `json:"Name"`
	MktNum           string `json:"MktNum"`
	SecurityTypeName string `json:"SecurityTypeName"`
}

// eastMoneyHotResponse models the JSON envelope returned by the EastMoney clist API.
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
	MarketID      int              `json:"f13"`
	Code          string           `json:"f12"`
	Name          string           `json:"f14"`
	CurrentPrice  provider.EmFloat `json:"f2"`
	ChangePercent provider.EmFloat `json:"f3"`
	Change        provider.EmFloat `json:"f4"`
	Volume        provider.EmFloat `json:"f5"`
	MarketCap     provider.EmFloat `json:"f20"`
}

// listEastMoney calls the EastMoney clist API, applicable to CN-A and HK categories.
func (s *HotService) listEastMoney(ctx context.Context, category core.HotCategory, sortBy core.HotSort, page, pageSize int) (core.HotListResponse, error) {
	fs, market, currency := resolveEastMoneyHotFilter(category)
	if fs == "" {
		return core.HotListResponse{}, fmt.Errorf("EastMoney hot category is unsupported: %s", category)
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

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.URLWithQuery(endpoint.EastMoneyHotAPI, params), nil)
	if err != nil {
		return core.HotListResponse{}, err
	}
	provider.SetEastMoneyHeaders(req, endpoint.EastMoneyWebReferer)

	resp, err := s.client.Do(req)
	if err != nil {
		return core.HotListResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return core.HotListResponse{}, fmt.Errorf("EastMoney hot request failed: status %d", resp.StatusCode)
	}

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return core.HotListResponse{}, err
	}

	var parsed eastMoneyHotResponse
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return core.HotListResponse{}, err
	}
	if parsed.RC != 0 {
		return core.HotListResponse{}, fmt.Errorf("EastMoney hot response returned rc=%d", parsed.RC)
	}

	items := make([]core.HotItem, 0, len(parsed.Data.Diff))
	for _, item := range parsed.Data.Diff {
		symbol := resolveEastMoneyHotSymbol(item.Code, item.MarketID, category)
		if symbol == "" {
			continue
		}
		items = append(items, core.HotItem{
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

	return core.HotListResponse{
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

// resolveEastMoneyHotFilter maps HotCategory to EastMoney clist fs parameter, market label and currency.
func resolveEastMoneyHotFilter(category core.HotCategory) (fs, market, currency string) {
	switch category {
	case core.HotCategoryCNA:
		return "m:0 t:6,m:0 t:80,m:1 t:2,m:1 t:23", "CN-A", "CNY"
	case core.HotCategoryHK:
		return "m:128", "HK-MAIN", "HKD"
	default:
		return "", "", ""
	}
}

// resolveEastMoneyHotSymbol generates a standard stock symbol from the EastMoney returned code and market ID.
func resolveEastMoneyHotSymbol(code string, marketID int, category core.HotCategory) string {
	code = normaliseEastMoneyCode(code, marketID)
	switch category {
	case core.HotCategoryCNA:
		switch marketID {
		case 1:
			return strings.ToUpper(code + ".SH")
		case 0:
			return strings.ToUpper(code + ".SZ")
		}
	case core.HotCategoryHK:
		return strings.ToUpper(code + ".HK")
	}
	return ""
}

// resolveEastMoneySort maps HotSort to EastMoney clist sort field ID and direction.
func resolveEastMoneySort(sortBy core.HotSort) (fid string, po int) {
	switch sortBy {
	case core.HotSortGainers:
		return "f3", 1
	case core.HotSortLosers:
		return "f3", 0
	case core.HotSortMarketCap:
		return "f20", 1
	case core.HotSortPrice:
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
		if len(code) < 5 && core.IsDigits(code) {
			return strings.Repeat("0", 5-len(code)) + code
		}
	case 0, 1:
		if len(code) < 6 && core.IsDigits(code) {
			return strings.Repeat("0", 6-len(code)) + code
		}
	}
	return code
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

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.EastMoneySuggestAPI+"?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	provider.SetEastMoneyHeaders(req, endpoint.EastMoneyWebReferer)

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
func (s *HotService) searchEastMoneySeeds(ctx context.Context, keyword string, category core.HotCategory) []hotSeed {
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
func eastMoneySuggestToSeed(item eastMoneySuggestItem, category core.HotCategory) (hotSeed, bool) {
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
