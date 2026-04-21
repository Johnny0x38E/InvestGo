package provider

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

	"investgo/internal/common/errs"
	"investgo/internal/core"
	"investgo/internal/core/endpoint"
)

type TencentQuoteProvider struct {
	client *http.Client
}

type TencentHistoryProvider struct {
	client *http.Client
}

// tencentKlineRow represents one OHLCV candlestick bar from the Tencent kline API.
// The upstream format is a JSON array: [date, open, close, high, low, volume, ...].
// Individual elements may be JSON strings ("16.88") or bare numbers (16.88).
type tencentKlineRow struct {
	Date   string
	Open   float64
	Close  float64
	High   float64
	Low    float64
	Volume float64
}

func (r *tencentKlineRow) UnmarshalJSON(data []byte) error {
	var elems []json.RawMessage
	if err := json.Unmarshal(data, &elems); err != nil {
		return err
	}
	if len(elems) < 6 {
		return fmt.Errorf("tencentKlineRow: expected at least 6 elements, got %d", len(elems))
	}
	r.Date = strings.Trim(string(elems[0]), `"`)
	r.Open = tencentParseRawFloat(elems[1])
	r.Close = tencentParseRawFloat(elems[2])
	r.High = tencentParseRawFloat(elems[3])
	r.Low = tencentParseRawFloat(elems[4])
	r.Volume = tencentParseRawFloat(elems[5])
	return nil
}

// tencentParseRawFloat parses a JSON value that may be either a bare number or a quoted string.
func tencentParseRawFloat(raw json.RawMessage) float64 {
	var n json.Number
	if err := json.Unmarshal(raw, &n); err == nil {
		f, _ := n.Float64()
		return f
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		f, _ := strconv.ParseFloat(s, 64)
		return f
	}
	return 0
}

// tencentKlinePayload is the per-symbol data block within tencentFQKlineResponse.
type tencentKlinePayload struct {
	Day    []tencentKlineRow   `json:"day"`
	Week   []tencentKlineRow   `json:"week"`
	Month  []tencentKlineRow   `json:"month"`
	QFQDay []tencentKlineRow   `json:"qfqday"`
	QT     map[string][]string `json:"qt"`
}

type tencentFQKlineResponse struct {
	Code int                            `json:"code"`
	Msg  string                         `json:"msg"`
	Data map[string]tencentKlinePayload `json:"data"`
}

const tencentBatchSize = 50

func NewTencentQuoteProvider(client *http.Client) *TencentQuoteProvider {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &TencentQuoteProvider{client: client}
}

func NewTencentHistoryProvider(client *http.Client) *TencentHistoryProvider {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &TencentHistoryProvider{client: client}
}

func (p *TencentQuoteProvider) Name() string { return "Tencent Finance" }

func (p *TencentQuoteProvider) Fetch(ctx context.Context, items []core.WatchlistItem) (map[string]core.Quote, error) {
	targets, problems := CollectQuoteTargets(items)
	quotes := make(map[string]core.Quote, len(targets))
	if len(targets) == 0 {
		return quotes, errs.JoinProblems(problems)
	}

	itemByKey := make(map[string]core.WatchlistItem, len(targets))
	queryCodes := make([]string, 0, len(targets))
	targetByCode := make(map[string]core.QuoteTarget, len(targets))
	for _, item := range items {
		target, err := core.ResolveQuoteTarget(item)
		if err != nil {
			continue
		}
		code, err := resolveTencentQuoteCode(target)
		if err != nil {
			problems = append(problems, err.Error())
			continue
		}
		itemByKey[target.Key] = item
		queryCodes = append(queryCodes, code)
		targetByCode[code] = target
	}

	if len(queryCodes) == 0 {
		return quotes, errs.JoinProblems(problems)
	}

	tencentHeaders := map[string]string{
		"Referer":    endpoint.TencentFinanceReferer,
		"User-Agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
	}

	for _, batch := range ChunkStrings(queryCodes, tencentBatchSize) {
		body, err := FetchTextWithHeaders(ctx, p.client, endpoint.TencentQuoteAPI+strings.Join(batch, ","), tencentHeaders, true)
		if err != nil {
			problems = append(problems, err.Error())
			continue
		}

		for line := range strings.SplitSeq(body, ";\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			code, fields, ok := parseTencentQuoteLine(line)
			if !ok {
				continue
			}
			target, ok := targetByCode[code]
			if !ok {
				continue
			}
			item := itemByKey[target.Key]
			quote, ok := buildTencentQuote(item, target, fields)
			if !ok {
				continue
			}
			quotes[target.Key] = quote
		}
	}

	if len(quotes) == 0 && len(problems) == 0 {
		problems = append(problems, "Tencent quote response is empty")
	}
	return quotes, errs.JoinProblems(problems)
}

func (p *TencentHistoryProvider) Name() string { return "Tencent Finance" }

func (p *TencentHistoryProvider) Fetch(ctx context.Context, item core.WatchlistItem, interval core.HistoryInterval) (core.HistorySeries, error) {
	target, err := core.ResolveQuoteTarget(item)
	if err != nil {
		return core.HistorySeries{}, err
	}

	codeCandidates, err := resolveTencentHistoryCodes(target)
	if err != nil {
		return core.HistorySeries{}, err
	}

	period, begin, end, count, qfq, err := resolveTencentHistoryParams(interval)
	if err != nil {
		return core.HistorySeries{}, err
	}

	var problems []string
	for _, code := range codeCandidates {
		series, fetchErr := p.fetchHistoryWithCode(ctx, item, target, code, interval, period, begin, end, count, qfq)
		if fetchErr == nil {
			return series, nil
		}
		problems = append(problems, fetchErr.Error())
	}

	return core.HistorySeries{}, errs.JoinProblems(problems)
}

func (p *TencentHistoryProvider) fetchHistoryWithCode(
	ctx context.Context,
	item core.WatchlistItem,
	target core.QuoteTarget,
	code string,
	interval core.HistoryInterval,
	period, begin, end, count, qfq string,
) (core.HistorySeries, error) {
	params := url.Values{}
	params.Set("param", strings.Join([]string{code, period, begin, end, count, qfq}, ","))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.TencentFQKlineAPI+"?"+params.Encode(), nil)
	if err != nil {
		return core.HistorySeries{}, err
	}
	req.Header.Set("Referer", endpoint.TencentFinanceReferer)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")

	resp, err := p.client.Do(req)
	if err != nil {
		return core.HistorySeries{}, fmt.Errorf("Tencent history request failed for %s: %w", code, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return core.HistorySeries{}, fmt.Errorf("Tencent history request failed for %s: status %d", code, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return core.HistorySeries{}, fmt.Errorf("Tencent history request failed for %s: %w", code, err)
	}

	var parsed tencentFQKlineResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return core.HistorySeries{}, fmt.Errorf("Tencent history decode failed for %s: %w", code, err)
	}
	if parsed.Code != 0 {
		return core.HistorySeries{}, fmt.Errorf("Tencent history response returned code=%d for %s", parsed.Code, code)
	}

	payload, ok := parsed.Data[code]
	if !ok {
		return core.HistorySeries{}, fmt.Errorf("Tencent history response is empty for %s", code)
	}

	rows := selectTencentHistoryRows(payload, period, qfq)
	points := parseTencentHistoryRows(rows)
	if len(points) == 0 {
		return core.HistorySeries{}, fmt.Errorf("Tencent history response is empty for %s", code)
	}

	series := core.HistorySeries{
		Symbol:      item.Symbol,
		Name:        FirstNonEmpty(resolveTencentHistoryName(payload, code), item.Name, item.Symbol),
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

func parseTencentQuoteLine(line string) (string, []string, bool) {
	const prefix = "v_"
	if !strings.HasPrefix(line, prefix) {
		return "", nil, false
	}
	eq := strings.Index(line, "=")
	if eq <= len(prefix) {
		return "", nil, false
	}
	code := strings.TrimSpace(line[len(prefix):eq])
	raw := strings.TrimSpace(strings.TrimSuffix(line[eq+1:], ";"))
	raw = strings.Trim(raw, "\"")
	if raw == "" {
		return code, nil, false
	}
	return code, strings.Split(raw, "~"), true
}

func buildTencentQuote(item core.WatchlistItem, target core.QuoteTarget, fields []string) (core.Quote, bool) {
	if len(fields) < 38 {
		return core.Quote{}, false
	}

	name := FirstNonEmpty(PartsAt(fields, 1), item.Name, target.DisplaySymbol)
	current := ParseFloat(PartsAt(fields, 3))
	previous := ParseFloat(PartsAt(fields, 4))
	open := ParseFloat(PartsAt(fields, 5))
	high := ParseFloat(PartsAt(fields, 33))
	low := ParseFloat(PartsAt(fields, 34))
	updatedAt := ParseTimestamp(PartsAt(fields, 30))
	if updatedAt.IsZero() {
		updatedAt = time.Now()
	}

	quote := BuildQuote(name, current, previous, open, high, low, updatedAt, "Tencent Finance")
	quote.Symbol = target.DisplaySymbol
	quote.Market = target.Market
	quote.Currency = FirstNonEmpty(PartsAt(fields, 35), item.Currency, target.Currency)
	quote.Change = ParseFloat(PartsAt(fields, 31))
	quote.ChangePercent = ParseFloat(PartsAt(fields, 32))

	// Volume: field 36. CN markets report in shou (lots of 100 shares); US/HK are in shares.
	if vol := ParseFloat(PartsAt(fields, 36)); vol > 0 {
		switch target.Market {
		case "CN-A", "CN-GEM", "CN-STAR", "CN-ETF":
			quote.Volume = vol * 100
		default:
			quote.Volume = vol
		}
	}
	// MarketCap: field 44 is in yi (hundred-millions) of local currency; convert to raw units.
	if mc := ParseFloat(PartsAt(fields, 44)); mc > 0 {
		quote.MarketCap = mc * 1e8
	}

	return quote, quote.CurrentPrice > 0
}

func resolveTencentQuoteCode(target core.QuoteTarget) (string, error) {
	switch target.Market {
	case "CN-A", "CN-GEM", "CN-STAR", "CN-ETF":
		if strings.HasSuffix(target.DisplaySymbol, ".SH") {
			return "sh" + strings.TrimSuffix(target.DisplaySymbol, ".SH"), nil
		}
		if strings.HasSuffix(target.DisplaySymbol, ".SZ") {
			return "sz" + strings.TrimSuffix(target.DisplaySymbol, ".SZ"), nil
		}
	case "HK-MAIN", "HK-GEM", "HK-ETF":
		if strings.HasSuffix(target.DisplaySymbol, ".HK") {
			return "hk" + strings.TrimSuffix(target.DisplaySymbol, ".HK"), nil
		}
	case "US-STOCK", "US-ETF":
		return "us" + strings.ReplaceAll(target.DisplaySymbol, "-", "."), nil
	}
	return "", fmt.Errorf("Tencent does not support item: %s", target.DisplaySymbol)
}

func resolveTencentHistoryCodes(target core.QuoteTarget) ([]string, error) {
	switch target.Market {
	case "CN-A", "CN-GEM", "CN-STAR", "CN-ETF":
		if strings.HasSuffix(target.DisplaySymbol, ".SH") {
			return []string{"sh" + strings.TrimSuffix(target.DisplaySymbol, ".SH")}, nil
		}
		if strings.HasSuffix(target.DisplaySymbol, ".SZ") {
			return []string{"sz" + strings.TrimSuffix(target.DisplaySymbol, ".SZ")}, nil
		}
	case "HK-MAIN", "HK-GEM", "HK-ETF":
		if strings.HasSuffix(target.DisplaySymbol, ".HK") {
			return []string{"hk" + strings.TrimSuffix(target.DisplaySymbol, ".HK")}, nil
		}
	case "US-STOCK", "US-ETF":
		symbol := strings.ReplaceAll(target.DisplaySymbol, "-", ".")
		return []string{"us" + symbol + ".OQ", "us" + symbol + ".N"}, nil
	}
	return nil, fmt.Errorf("Tencent does not support market: %s", target.DisplaySymbol)
}

func resolveTencentHistoryParams(interval core.HistoryInterval) (period, begin, end, count, qfq string, err error) {
	now := time.Now()
	switch interval {
	case core.HistoryRange1w, core.HistoryRange1mo, core.HistoryRange1y:
		return "day", now.AddDate(-1, 0, 0).Format(time.DateOnly), now.Format(time.DateOnly), "500", "qfq", nil
	case core.HistoryRange3y, core.HistoryRangeAll:
		return "week", now.AddDate(-5, 0, 0).Format(time.DateOnly), now.Format(time.DateOnly), "500", "qfq", nil
	default:
		return "", "", "", "", "", fmt.Errorf("Tencent does not support history interval: %s", interval)
	}
}

func selectTencentHistoryRows(payload tencentKlinePayload, period, qfq string) []tencentKlineRow {
	if period == "day" && qfq == "qfq" && len(payload.QFQDay) > 0 {
		return payload.QFQDay
	}
	switch period {
	case "week":
		return payload.Week
	case "month":
		return payload.Month
	default:
		return payload.Day
	}
}

func parseTencentHistoryRows(rows []tencentKlineRow) []core.HistoryPoint {
	points := make([]core.HistoryPoint, 0, len(rows))
	for _, row := range rows {
		ts, err := time.ParseInLocation(time.DateOnly, row.Date, time.Local)
		if err != nil {
			continue
		}
		if row.Close <= 0 {
			continue
		}
		points = append(points, core.HistoryPoint{
			Timestamp: ts,
			Open:      row.Open,
			High:      row.High,
			Low:       row.Low,
			Close:     row.Close,
			Volume:    row.Volume,
		})
	}
	return points
}

func resolveTencentHistoryName(payload tencentKlinePayload, code string) string {
	if qt, ok := payload.QT[code]; ok {
		return FirstNonEmpty(PartsAt(qt, 1), PartsAt(qt, 46))
	}
	return ""
}
