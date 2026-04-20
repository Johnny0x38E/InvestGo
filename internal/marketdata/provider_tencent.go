package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"investgo/internal/datasource"
	"investgo/internal/monitor"
)

type TencentQuoteProvider struct {
	client *http.Client
}

type TencentHistoryProvider struct {
	client *http.Client
}

type tencentFQKlineResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data map[string]struct {
		Day    [][]any `json:"day"`
		Week   [][]any `json:"week"`
		Month  [][]any `json:"month"`
		QFQDay [][]any `json:"qfqday"`
		QT     map[string][]string `json:"qt"`
	} `json:"data"`
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

func (p *TencentQuoteProvider) Fetch(ctx context.Context, items []monitor.WatchlistItem) (map[string]monitor.Quote, error) {
	targets, problems := collectQuoteTargets(items)
	quotes := make(map[string]monitor.Quote, len(targets))
	if len(targets) == 0 {
		return quotes, monitor.JoinProblems(problems)
	}

	itemByKey := make(map[string]monitor.WatchlistItem, len(targets))
	queryCodes := make([]string, 0, len(targets))
	targetByCode := make(map[string]monitor.QuoteTarget, len(targets))
	for _, item := range items {
		target, err := monitor.ResolveQuoteTarget(item)
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
		return quotes, monitor.JoinProblems(problems)
	}

	tencentHeaders := map[string]string{
		"Referer":    datasource.TencentFinanceReferer,
		"User-Agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
	}

	for _, batch := range chunkStrings(queryCodes, tencentBatchSize) {
		body, err := fetchTextWithHeaders(ctx, p.client, datasource.TencentQuoteAPI+strings.Join(batch, ","), tencentHeaders, true)
		if err != nil {
			problems = append(problems, err.Error())
			continue
		}

		for _, line := range strings.Split(body, ";\n") {
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
	return quotes, monitor.JoinProblems(problems)
}

func (p *TencentHistoryProvider) Name() string { return "Tencent Finance" }

func (p *TencentHistoryProvider) Fetch(ctx context.Context, item monitor.WatchlistItem, interval monitor.HistoryInterval) (monitor.HistorySeries, error) {
	target, err := monitor.ResolveQuoteTarget(item)
	if err != nil {
		return monitor.HistorySeries{}, err
	}

	codeCandidates, err := resolveTencentHistoryCodes(target)
	if err != nil {
		return monitor.HistorySeries{}, err
	}

	period, begin, end, count, qfq, err := resolveTencentHistoryParams(interval)
	if err != nil {
		return monitor.HistorySeries{}, err
	}

	var problems []string
	for _, code := range codeCandidates {
		series, fetchErr := p.fetchHistoryWithCode(ctx, item, target, code, interval, period, begin, end, count, qfq)
		if fetchErr == nil {
			return series, nil
		}
		problems = append(problems, fetchErr.Error())
	}

	return monitor.HistorySeries{}, monitor.JoinProblems(problems)
}

func (p *TencentHistoryProvider) fetchHistoryWithCode(
	ctx context.Context,
	item monitor.WatchlistItem,
	target monitor.QuoteTarget,
	code string,
	interval monitor.HistoryInterval,
	period, begin, end, count, qfq string,
) (monitor.HistorySeries, error) {
	params := url.Values{}
	params.Set("param", strings.Join([]string{code, period, begin, end, count, qfq}, ","))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, datasource.TencentFQKlineAPI+"?"+params.Encode(), nil)
	if err != nil {
		return monitor.HistorySeries{}, err
	}
	req.Header.Set("Referer", datasource.TencentFinanceReferer)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")

	resp, err := p.client.Do(req)
	if err != nil {
		return monitor.HistorySeries{}, fmt.Errorf("Tencent history request failed for %s: %w", code, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return monitor.HistorySeries{}, fmt.Errorf("Tencent history request failed for %s: status %d", code, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return monitor.HistorySeries{}, fmt.Errorf("Tencent history request failed for %s: %w", code, err)
	}

	var parsed tencentFQKlineResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return monitor.HistorySeries{}, fmt.Errorf("Tencent history decode failed for %s: %w", code, err)
	}
	if parsed.Code != 0 {
		return monitor.HistorySeries{}, fmt.Errorf("Tencent history response returned code=%d for %s", parsed.Code, code)
	}

	payload, ok := parsed.Data[code]
	if !ok {
		return monitor.HistorySeries{}, fmt.Errorf("Tencent history response is empty for %s", code)
	}

	rows := selectTencentHistoryRows(payload, period, qfq)
	points := parseTencentHistoryRows(rows)
	if len(points) == 0 {
		return monitor.HistorySeries{}, fmt.Errorf("Tencent history response is empty for %s", code)
	}

	series := monitor.HistorySeries{
		Symbol:      item.Symbol,
		Name:        firstNonEmpty(resolveTencentHistoryName(payload, code), item.Name, item.Symbol),
		Market:      item.Market,
		Currency:    firstNonEmpty(item.Currency, target.Currency),
		Interval:    interval,
		Source:      p.Name(),
		Points:      points,
		GeneratedAt: time.Now(),
	}
	applyHistorySummary(&series)
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

func buildTencentQuote(item monitor.WatchlistItem, target monitor.QuoteTarget, fields []string) (monitor.Quote, bool) {
	if len(fields) < 38 {
		return monitor.Quote{}, false
	}

	name := firstNonEmpty(partsAt(fields, 1), item.Name, target.DisplaySymbol)
	current := parseFloat(partsAt(fields, 3))
	previous := parseFloat(partsAt(fields, 4))
	open := parseFloat(partsAt(fields, 5))
	high := parseFloat(partsAt(fields, 33))
	low := parseFloat(partsAt(fields, 34))
	updatedAt := parseTimestamp(partsAt(fields, 30))
	if updatedAt.IsZero() {
		updatedAt = time.Now()
	}

	quote := buildQuote(name, current, previous, open, high, low, updatedAt, "Tencent Finance")
	quote.Symbol = target.DisplaySymbol
	quote.Market = target.Market
	quote.Currency = firstNonEmpty(partsAt(fields, 35), item.Currency, target.Currency)
	quote.Change = parseFloat(partsAt(fields, 31))
	quote.ChangePercent = parseFloat(partsAt(fields, 32))

	// Volume: field 36. CN markets report in shou (lots of 100 shares); US/HK are in shares.
	if vol := parseFloat(partsAt(fields, 36)); vol > 0 {
		switch target.Market {
		case "CN-A", "CN-GEM", "CN-STAR", "CN-ETF":
			quote.Volume = vol * 100
		default:
			quote.Volume = vol
		}
	}
	// MarketCap: field 44 is in yi (hundred-millions) of local currency; convert to raw units.
	if mc := parseFloat(partsAt(fields, 44)); mc > 0 {
		quote.MarketCap = mc * 1e8
	}

	return quote, quote.CurrentPrice > 0
}

func resolveTencentQuoteCode(target monitor.QuoteTarget) (string, error) {
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

func resolveTencentHistoryCodes(target monitor.QuoteTarget) ([]string, error) {
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

func resolveTencentHistoryParams(interval monitor.HistoryInterval) (period, begin, end, count, qfq string, err error) {
	now := time.Now()
	switch interval {
	case monitor.HistoryRange1w, monitor.HistoryRange1mo, monitor.HistoryRange1y:
		return "day", now.AddDate(-1, 0, 0).Format("2006-01-02"), now.Format("2006-01-02"), "500", "qfq", nil
	case monitor.HistoryRange3y, monitor.HistoryRangeAll:
		return "week", now.AddDate(-5, 0, 0).Format("2006-01-02"), now.Format("2006-01-02"), "500", "qfq", nil
	default:
		return "", "", "", "", "", fmt.Errorf("Tencent does not support history interval: %s", interval)
	}
}

func selectTencentHistoryRows(payload struct {
	Day    [][]any `json:"day"`
	Week   [][]any `json:"week"`
	Month  [][]any `json:"month"`
	QFQDay [][]any `json:"qfqday"`
	QT     map[string][]string `json:"qt"`
}, period, qfq string) [][]any {
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

func parseTencentHistoryRows(rows [][]any) []monitor.HistoryPoint {
	points := make([]monitor.HistoryPoint, 0, len(rows))
	for _, row := range rows {
		if len(row) < 6 {
			continue
		}
		ts, err := time.ParseInLocation("2006-01-02", fmt.Sprint(row[0]), time.Local)
		if err != nil {
			continue
		}
		open := parseFloat(fmt.Sprint(row[1]))
		closePrice := parseFloat(fmt.Sprint(row[2]))
		high := parseFloat(fmt.Sprint(row[3]))
		low := parseFloat(fmt.Sprint(row[4]))
		volume := parseFloat(fmt.Sprint(row[5]))
		if closePrice <= 0 {
			continue
		}
		points = append(points, monitor.HistoryPoint{
			Timestamp: ts,
			Open:      open,
			High:      high,
			Low:       low,
			Close:     closePrice,
			Volume:    volume,
		})
	}
	return points
}

func resolveTencentHistoryName(payload struct {
	Day    [][]any `json:"day"`
	Week   [][]any `json:"week"`
	Month  [][]any `json:"month"`
	QFQDay [][]any `json:"qfqday"`
	QT     map[string][]string `json:"qt"`
}, code string) string {
	if qt, ok := payload.QT[code]; ok {
		return firstNonEmpty(partsAt(qt, 1), partsAt(qt, 46))
	}
	return ""
}
