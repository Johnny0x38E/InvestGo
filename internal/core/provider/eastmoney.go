// provider_eastmoney.go - EastMoney quote and history provider.
package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"investgo/internal/common/errs"
	"investgo/internal/core"
	"investgo/internal/core/endpoint"
)

// ── EastMoney quote provider ─────────────────────────────────────────────────────

type EastMoneyQuoteProvider struct {
	client *http.Client
}

type eastMoneyQuoteResponse struct {
	RC   int                `json:"rc"`
	Data EastMoneyQuoteData `json:"data"`
}

type eastMoneyStockQuoteResponse struct {
	RC   int                  `json:"rc"`
	Data *eastMoneyStockQuote `json:"data"`
}

type eastMoneyStockQuote struct {
	CurrentPrice  EmFloat `json:"f43"`
	OpenPrice     EmFloat `json:"f46"`
	Code          string  `json:"f57"`
	Name          string  `json:"f58"`
	PreviousClose EmFloat `json:"f60"`
	MarketCap     EmFloat `json:"f116"`
	Change        EmFloat `json:"f169"`
	ChangePercent EmFloat `json:"f170"`
	DayHigh       EmFloat `json:"f44"`
	DayLow        EmFloat `json:"f45"`
}

type EastMoneyQuoteData struct {
	Diff []EastMoneyQuoteDataDiff `json:"diff"`
}

type EastMoneyQuoteDataDiff struct {
	MarketID      int     `json:"f13"`
	Code          string  `json:"f12"`
	Name          string  `json:"f14"`
	CurrentPrice  EmFloat `json:"f2"`
	ChangePercent EmFloat `json:"f3"`
	Change        EmFloat `json:"f4"`
	DayHigh       EmFloat `json:"f15"`
	DayLow        EmFloat `json:"f16"`
	OpenPrice     EmFloat `json:"f17"`
	PreviousClose EmFloat `json:"f18"`
}

// eastMoneyBatchSize is the number of secids per batch for EastMoney quote requests.
const eastMoneyBatchSize = 50

// NewEastMoneyQuoteProvider creates an EastMoney real-time quote provider.
func NewEastMoneyQuoteProvider(client *http.Client) *EastMoneyQuoteProvider {
	if client == nil {
		client = &http.Client{Timeout: 8 * time.Second}
	}

	return &EastMoneyQuoteProvider{client: client}
}

// Name returns the display name of the EastMoney quote source.
func (p *EastMoneyQuoteProvider) Name() string {
	return "EastMoney"
}

// Fetch requests EastMoney real-time quotes in batch and maps them to the standard Quote structure.
func (p *EastMoneyQuoteProvider) Fetch(ctx context.Context, items []core.WatchlistItem) (map[string]core.Quote, error) {
	targets, problems := CollectQuoteTargets(items)
	quotes := make(map[string]core.Quote, len(targets))
	if len(targets) == 0 {
		return quotes, errs.JoinProblems(problems)
	}

	itemByTargetKey := make(map[string]core.WatchlistItem, len(items))
	for _, item := range items {
		target, err := core.ResolveQuoteTarget(item)
		if err != nil {
			continue
		}
		itemByTargetKey[target.Key] = item
	}

	// EastMoney queries by secid in batch, so map standard targets to secids first.
	secids := make([]string, 0, len(targets)*2)
	indexBySecID := make(map[string]core.QuoteTarget, len(targets)*2)
	secidsByTargetKey := make(map[string][]string, len(targets))
	for _, target := range targets {
		if target.Market == "US-STOCK" || target.Market == "US-ETF" {
			continue
		}
		ids, err := ResolveAllEastMoneySecIDs(target)
		if err != nil {
			problems = append(problems, err.Error())
			continue
		}
		for _, secid := range ids {
			secids = append(secids, secid)
			indexBySecID[secid] = target
		}
		secidsByTargetKey[target.Key] = append(secidsByTargetKey[target.Key], ids...)
	}

	if len(secids) == 0 {
	} else {
		diffs, fetchErr := p.fetchDiffs(ctx, secids)
		if fetchErr == nil {
			for _, item := range diffs {
				secid := fmt.Sprintf("%d.%s", item.MarketID, NormaliseEastMoneyCode(item.Code, item.MarketID))
				target, ok := indexBySecID[secid]
				if !ok {
					continue
				}

				quote := BuildQuote(
					item.Name,
					float64(item.CurrentPrice),
					float64(item.PreviousClose),
					float64(item.OpenPrice),
					float64(item.DayHigh),
					float64(item.DayLow),
					time.Now(),
					p.Name(),
				)
				quote.Symbol = target.DisplaySymbol
				quote.Market = target.Market
				quote.Currency = target.Currency
				if item.ChangePercent != 0 {
					quote.ChangePercent = float64(item.ChangePercent)
				}
				if item.Change != 0 {
					quote.Change = float64(item.Change)
				}
				quotes[target.Key] = quote
			}
		}

		for key, target := range targets {
			if target.Market == "US-STOCK" || target.Market == "US-ETF" {
				continue
			}
			if _, ok := quotes[key]; ok {
				continue
			}
			secids := secidsByTargetKey[key]
			if len(secids) > 0 {
				problems = append(problems, fmt.Sprintf("Did not receive EastMoney quote for %s (%s)", target.DisplaySymbol, secids[0]))
			}
		}

		if fetchErr != nil {
			for key, target := range targets {
				if target.Market == "US-STOCK" || target.Market == "US-ETF" {
					continue
				}
				if _, ok := quotes[key]; !ok {
					problems = append(problems, fetchErr.Error())
					break
				}
			}
		}
	}

	missingUSItems := make([]core.WatchlistItem, 0, len(itemByTargetKey))
	usProblems := make(map[string]error)
	for _, target := range targets {
		if target.Market != "US-STOCK" && target.Market != "US-ETF" {
			continue
		}
		quote, err := p.fetchUSQuote(ctx, target)
		if err != nil {
			usProblems[target.Key] = err
			continue
		}
		quotes[target.Key] = quote
	}

	for key, target := range targets {
		if _, ok := quotes[key]; ok {
			continue
		}
		if target.Market != "US-STOCK" && target.Market != "US-ETF" {
			continue
		}
		item, ok := itemByTargetKey[key]
		if !ok {
			continue
		}
		missingUSItems = append(missingUSItems, item)
	}

	if len(missingUSItems) > 0 {
		yahooQuotes, yahooErr := NewYahooQuoteProvider(p.client).Fetch(ctx, missingUSItems)
		for key, quote := range yahooQuotes {
			if _, exists := quotes[key]; !exists {
				quotes[key] = quote
			}
		}
		if yahooErr != nil {
			problems = append(problems, yahooErr.Error())
		}
	}

	for key, err := range usProblems {
		if _, ok := quotes[key]; ok {
			continue
		}
		problems = append(problems, err.Error())
	}

	return quotes, errs.JoinProblems(problems)
}

func (p *EastMoneyQuoteProvider) fetchUSQuote(ctx context.Context, target core.QuoteTarget) (core.Quote, error) {
	secids, err := ResolveAllEastMoneySecIDs(target)
	if err != nil {
		return core.Quote{}, err
	}

	var problems []string
	for _, secid := range secids {
		quote, quoteErr := p.fetchUSQuoteBySecID(ctx, target, secid)
		if quoteErr == nil {
			return quote, nil
		}
		problems = append(problems, quoteErr.Error())
	}

	return core.Quote{}, errs.JoinProblems(problems)
}

func (p *EastMoneyQuoteProvider) fetchUSQuoteBySecID(ctx context.Context, target core.QuoteTarget, secid string) (core.Quote, error) {
	params := url.Values{}
	params.Set("secid", secid)
	params.Set("ut", "fa5fd1943c7b386f172d6893dbfba10b")
	params.Set("fields", "f43,f57,f58,f169,f170,f46,f44,f45,f60,f116")

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.URLWithQuery(endpoint.EastMoneyStockAPI, params), nil)
	if err != nil {
		return core.Quote{}, err
	}
	SetEastMoneyHeaders(request, endpoint.EastMoneyWebReferer)

	response, err := p.client.Do(request)
	if err != nil {
		return core.Quote{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return core.Quote{}, fmt.Errorf("EastMoney quote request failed: status %d", response.StatusCode)
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return core.Quote{}, err
	}

	var parsed eastMoneyStockQuoteResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return core.Quote{}, err
	}
	if parsed.RC != 0 || parsed.Data == nil {
		return core.Quote{}, fmt.Errorf("Did not receive EastMoney quote for %s (%s)", target.DisplaySymbol, secid)
	}

	quote := BuildQuote(
		FirstNonEmpty(parsed.Data.Name, target.DisplaySymbol),
		scaleEastMoneyPrice(parsed.Data.CurrentPrice),
		scaleEastMoneyPrice(parsed.Data.PreviousClose),
		scaleEastMoneyPrice(parsed.Data.OpenPrice),
		scaleEastMoneyPrice(parsed.Data.DayHigh),
		scaleEastMoneyPrice(parsed.Data.DayLow),
		time.Now(),
		p.Name(),
	)
	quote.Symbol = target.DisplaySymbol
	quote.Market = target.Market
	quote.Currency = target.Currency
	quote.Change = scaleEastMoneyPrice(parsed.Data.Change)
	quote.ChangePercent = scaleEastMoneyPercent(parsed.Data.ChangePercent)
	return quote, nil
}

func scaleEastMoneyPrice(value EmFloat) float64 {
	return float64(value) / 1000
}

func scaleEastMoneyPercent(value EmFloat) float64 {
	return float64(value) / 100
}

func (p *EastMoneyQuoteProvider) fetchDiffs(ctx context.Context, secids []string) ([]EastMoneyQuoteDataDiff, error) {
	diffs := make([]EastMoneyQuoteDataDiff, 0, len(secids))
	for _, batch := range ChunkSecIDs(secids, eastMoneyBatchSize, 1<<30) {
		batchDiffs, err := p.fetchDiffBatchAdaptive(ctx, batch)
		if err != nil {
			return nil, err
		}
		diffs = append(diffs, batchDiffs...)
	}
	return diffs, nil
}

func (p *EastMoneyQuoteProvider) fetchDiffBatchAdaptive(ctx context.Context, secids []string) ([]EastMoneyQuoteDataDiff, error) {
	if len(secids) == 0 {
		return nil, nil
	}

	diffs, err := p.fetchDiffBatch(ctx, secids)
	if err == nil {
		return diffs, nil
	}
	if len(secids) == 1 {
		return nil, err
	}

	mid := len(secids) / 2
	left, leftErr := p.fetchDiffBatchAdaptive(ctx, secids[:mid])
	right, rightErr := p.fetchDiffBatchAdaptive(ctx, secids[mid:])
	if leftErr != nil && rightErr != nil {
		return nil, errs.JoinProblems([]string{leftErr.Error(), rightErr.Error()})
	}
	if leftErr != nil {
		return append([]EastMoneyQuoteDataDiff(nil), right...), nil
	}
	if rightErr != nil {
		return append([]EastMoneyQuoteDataDiff(nil), left...), nil
	}
	return append(left, right...), nil
}

func (p *EastMoneyQuoteProvider) fetchDiffBatch(ctx context.Context, secids []string) ([]EastMoneyQuoteDataDiff, error) {
	params := url.Values{}
	params.Set("fltt", "2")
	params.Set("invt", "2")
	params.Set("np", "1")
	params.Set("ut", "bd1d9ddb04089700cf9c27f6f7426281")
	params.Set("fields", "f2,f3,f4,f12,f13,f14,f15,f16,f17,f18")
	params.Set("secids", strings.Join(secids, ","))

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.URLWithQuery(endpoint.EastMoneyQuoteAPI, params), nil)
	if err != nil {
		return nil, err
	}
	SetEastMoneyHeaders(request, endpoint.EastMoneyWebReferer)

	response, err := p.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("EastMoney quote request failed: status %d", response.StatusCode)
	}

	payload, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}

	var parsed eastMoneyQuoteResponse
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return nil, err
	}
	if parsed.RC != 0 {
		return nil, fmt.Errorf("EastMoney quote response returned rc=%d", parsed.RC)
	}

	return parsed.Data.Diff, nil
}

// resolveEastMoneySecID converts a standard target to the secid required by the EastMoney API (returns the first match).
// Note: US stocks default to 105 (NASDAQ), but may actually be on 106 (NYSE) or 107 (NYSE Arca).
// For batch quote requests, prefer resolveAllEastMoneySecIDs to cover all US exchanges.
func resolveEastMoneySecID(target core.QuoteTarget) (string, error) {
	ids, err := ResolveAllEastMoneySecIDs(target)
	if err != nil {
		return "", err
	}
	return ids[0], nil
}

// resolveAllEastMoneySecIDs converts a standard target to all possible secids required by the EastMoney API.
// For US stocks, since the same ticker may list on NASDAQ (105), NYSE (106), or NYSE Arca (107),
// all three variants are returned to ensure the correct exchange is hit.
func ResolveAllEastMoneySecIDs(target core.QuoteTarget) ([]string, error) {
	symbol := target.DisplaySymbol
	market := target.Market

	switch market {
	case "CN-A", "CN-GEM", "CN-STAR", "CN-ETF":
		if strings.HasSuffix(symbol, ".SH") {
			return []string{"1." + strings.TrimSuffix(symbol, ".SH")}, nil
		}
		if strings.HasSuffix(symbol, ".SZ") {
			return []string{"0." + strings.TrimSuffix(symbol, ".SZ")}, nil
		}
		return nil, fmt.Errorf("A-share / ETF symbol format is invalid: %s", symbol)
	case "CN-BJ":
		return nil, fmt.Errorf("Realtime quotes are not supported for Beijing Exchange symbols in EastMoney: %s", symbol)
	case "HK-MAIN", "HK-GEM", "HK-ETF":
		if strings.HasSuffix(symbol, ".HK") {
			return []string{"116." + strings.TrimSuffix(symbol, ".HK")}, nil
		}
		return nil, fmt.Errorf("Hong Kong symbol format is invalid: %s", symbol)
	case "US-STOCK", "US-ETF":
		var ticker string
		if IsLetters(symbol) {
			ticker = symbol
		} else if strings.Contains(symbol, "-") {
			ticker = strings.ReplaceAll(symbol, "-", ".")
		} else {
			return nil, fmt.Errorf("US symbol format is invalid: %s", symbol)
		}
		// 105=NASDAQ, 106=NYSE, 107=NYSE Arca — request all three to cover every exchange
		return []string{"105." + ticker, "106." + ticker, "107." + ticker}, nil
	default:
		return nil, fmt.Errorf("Market type is unsupported: %s", market)
	}
}

// ── EastMoney K-line chart provider ──────────────────────────────────────────

// chinaLocation defines the China time zone for parsing timestamps returned by EastMoney.
// EastMoney's K-line API returns timestamps in China time, which must be parsed with this location.
var chinaLocation = time.FixedZone("CST", 8*3600)

// EastMoneyChartProvider fetches historical quote data via the EastMoney K-line API.
// The app currently uses this API as the unified historical chart data source.
type EastMoneyChartProvider struct {
	client *http.Client
}

type eastMoneyKlineResponse struct {
	RC   int    `json:"rc"`
	Info string `json:"info,omitempty"`
	Data *struct {
		Code   string   `json:"code"`
		Market int      `json:"market"`
		Name   string   `json:"name"`
		KLines []string `json:"klines"`
	} `json:"data"`
}

type eastMoneyHistorySpec struct {
	klt        int           // candlestick period (101=daily, 102=weekly, 103=monthly, 60=60min)
	beg        string        // start date YYYYMMDD; "0" means earliest
	end        string        // end date YYYYMMDD
	lmt        int           // max number of candlesticks to return (0=unlimited)
	intraday   bool          // whether it is minute-level (timestamp includes hour:minute:second)
	trimWindow time.Duration // trim to the most recent duration (0=do not trim)
}

// NewEastMoneyChartProvider creates an EastMoney historical quote provider.
func NewEastMoneyChartProvider(client *http.Client) *EastMoneyChartProvider {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &EastMoneyChartProvider{client: client}
}

// Name returns the display name of the EastMoney history source.
func (p *EastMoneyChartProvider) Name() string {
	return "EastMoney"
}

// Fetch fetches historical quote data via the EastMoney K-line API.
func (p *EastMoneyChartProvider) Fetch(ctx context.Context, item core.WatchlistItem, interval core.HistoryInterval) (core.HistorySeries, error) {
	target, err := core.ResolveQuoteTarget(item)
	if err != nil {
		return core.HistorySeries{}, fmt.Errorf("EastMoney history failed to resolve item %s: %w", item.Symbol, err)
	}

	// EastMoney uses different klt, date windows and trimming strategies for different intervals.
	spec, err := eastMoneyHistorySpecFor(interval)
	if err != nil {
		return core.HistorySeries{}, err
	}

	secids, err := ResolveAllEastMoneySecIDs(target)
	if err != nil {
		return core.HistorySeries{}, fmt.Errorf("EastMoney history failed to resolve secid: %w", err)
	}

	var problems []string
	for _, secid := range secids {
		series, fetchErr := p.fetchWithSecID(ctx, item, target, interval, spec, secid)
		if fetchErr == nil {
			return series, nil
		}
		problems = append(problems, fetchErr.Error())
	}
	return core.HistorySeries{}, errs.JoinProblems(problems)
}

func (p *EastMoneyChartProvider) fetchWithSecID(
	ctx context.Context,
	item core.WatchlistItem,
	target core.QuoteTarget,
	interval core.HistoryInterval,
	spec eastMoneyHistorySpec,
	secid string,
) (core.HistorySeries, error) {

	params := url.Values{}
	params.Set("secid", secid)
	params.Set("ut", "bd1d9ddb04089700cf9c27f6f7426281")
	params.Set("fields1", "f1,f2,f3,f4,f5,f6")
	params.Set("fields2", "f51,f52,f53,f54,f55,f56,f57")
	params.Set("klt", strconv.Itoa(spec.klt))
	params.Set("fqt", "1")
	params.Set("beg", spec.beg)
	params.Set("end", spec.end)
	if spec.lmt > 0 {
		params.Set("lmt", strconv.Itoa(spec.lmt))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		endpoint.URLWithQuery(endpoint.EastMoneyHistoryAPI, params), nil)
	if err != nil {
		return core.HistorySeries{}, fmt.Errorf("EastMoney history request failed for %s: %w", secid, err)
	}
	SetEastMoneyHeaders(req, endpoint.EastMoneyWebReferer)

	resp, err := p.client.Do(req)
	if err != nil {
		return core.HistorySeries{}, fmt.Errorf("EastMoney history request failed for %s: %w", secid, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return core.HistorySeries{}, fmt.Errorf("EastMoney history request failed for %s: status %d", secid, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return core.HistorySeries{}, fmt.Errorf("EastMoney history request failed for %s: %w", secid, err)
	}

	var parsed eastMoneyKlineResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return core.HistorySeries{}, fmt.Errorf("EastMoney history decode failed for %s: %w", secid, err)
	}
	if parsed.RC != 0 {
		return core.HistorySeries{}, fmt.Errorf("EastMoney history response returned rc=%d for %s", parsed.RC, secid)
	}
	if parsed.Data == nil || len(parsed.Data.KLines) == 0 {
		return core.HistorySeries{}, fmt.Errorf("EastMoney history response is empty for %s", secid)
	}

	points := parseEastMoneyKlines(parsed.Data.KLines, spec.intraday)
	if len(points) == 0 {
		return core.HistorySeries{}, errors.New("EastMoney history contains no valid price points")
	}

	if spec.trimWindow > 0 {
		points = TrimHistoryPoints(points, spec.trimWindow)
	}
	if len(points) == 0 {
		return core.HistorySeries{}, errors.New("EastMoney history contains no valid price points after trimming")
	}

	series := core.HistorySeries{
		Symbol:      item.Symbol,
		Name:        FirstNonEmpty(parsed.Data.Name, item.Name, item.Symbol),
		Market:      item.Market,
		Currency:    FirstNonEmpty(item.Currency, target.Currency),
		Interval:    interval,
		Source:      p.Name(),
		Points:      points,
		GeneratedAt: time.Now(),
	}
	ApplyHistorySummary(&series)
	return series, nil
}

// eastMoneyHistorySpecFor maps a chart interval to EastMoney K-line request parameters.
func eastMoneyHistorySpecFor(interval core.HistoryInterval) (eastMoneyHistorySpec, error) {
	now := time.Now()
	end := now.AddDate(0, 0, 1).Format("20060102")

	switch interval {
	case core.HistoryRange1h:
		return eastMoneyHistorySpec{klt: 60, beg: now.AddDate(0, 0, -5).Format("20060102"), end: end, lmt: 50, intraday: true, trimWindow: time.Hour}, nil
	case core.HistoryRange1d:
		return eastMoneyHistorySpec{klt: 60, beg: now.AddDate(0, 0, -5).Format("20060102"), end: end, lmt: 50, intraday: true, trimWindow: 24 * time.Hour}, nil
	case core.HistoryRange1w:
		return eastMoneyHistorySpec{klt: 101, beg: now.AddDate(0, 0, -14).Format("20060102"), end: end, lmt: 10}, nil
	case core.HistoryRange1mo:
		return eastMoneyHistorySpec{klt: 101, beg: now.AddDate(0, -2, 0).Format("20060102"), end: end, lmt: 35}, nil
	case core.HistoryRange1y:
		return eastMoneyHistorySpec{klt: 101, beg: now.AddDate(-1, -1, 0).Format("20060102"), end: end, lmt: 270}, nil
	case core.HistoryRange3y:
		return eastMoneyHistorySpec{klt: 102, beg: now.AddDate(-3, -1, 0).Format("20060102"), end: end, lmt: 160}, nil
	case core.HistoryRangeAll:
		return eastMoneyHistorySpec{klt: 103, beg: "0", end: "20500101", lmt: 999}, nil
	default:
		return eastMoneyHistorySpec{}, fmt.Errorf("EastMoney does not support history interval: %s", interval)
	}
}

// parseEastMoneyKlines parses the EastMoney K-line string list into a slice of history points.
// K-line field order: date, open, close, high, low, volume, turnover (comma-separated).
func parseEastMoneyKlines(klines []string, intraday bool) []core.HistoryPoint {
	layout := time.DateOnly
	if intraday {
		layout = time.DateTime
	}

	points := make([]core.HistoryPoint, 0, len(klines))
	for _, kline := range klines {
		parts := strings.SplitN(kline, ",", 8)
		if len(parts) < 6 {
			continue
		}
		t, err := time.ParseInLocation(layout, strings.TrimSpace(parts[0]), chinaLocation)
		if err != nil {
			continue
		}

		open := ParseFloat(parts[1])
		closePrice := ParseFloat(parts[2])
		high := ParseFloat(parts[3])
		low := ParseFloat(parts[4])
		volume := ParseFloat(parts[5])

		if closePrice <= 0 {
			continue
		}

		points = append(points, core.HistoryPoint{
			Timestamp: t,
			Open:      open,
			High:      high,
			Low:       low,
			Close:     closePrice,
			Volume:    volume,
		})
	}
	return points
}

// NormaliseEastMoneyCode pads leading zeros for the EastMoney returned code based on marketID.
func NormaliseEastMoneyCode(code string, marketID int) string {
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
