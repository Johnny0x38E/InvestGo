package marketdata

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"investgo/internal/datasource"
	"investgo/internal/monitor"
)

// sinaBatchSize is the maximum number of symbols per Sina HTTP request to avoid
// overly long URLs that cause timeouts (e.g. 500 S&P 500 symbols in one URL).
const sinaBatchSize = 50

type SinaQuoteProvider struct {
	client *http.Client
}

func NewSinaQuoteProvider(client *http.Client) *SinaQuoteProvider {
	if client == nil {
		client = &http.Client{Timeout: 8 * time.Second}
	}
	return &SinaQuoteProvider{client: client}
}

func (p *SinaQuoteProvider) Name() string {
	return "Sina Finance"
}

func (p *SinaQuoteProvider) Fetch(ctx context.Context, items []monitor.WatchlistItem) (map[string]monitor.Quote, error) {
	targets, problems := collectQuoteTargets(items)
	quotes := make(map[string]monitor.Quote, len(targets))
	if len(targets) == 0 {
		return quotes, monitor.JoinProblems(problems)
	}

	itemByKey := make(map[string]monitor.WatchlistItem, len(targets))
	sinaCodes := make([]string, 0, len(targets))
	targetByCode := make(map[string]monitor.QuoteTarget, len(targets))
	for _, item := range items {
		target, err := monitor.ResolveQuoteTarget(item)
		if err != nil {
			continue
		}
		code, err := resolveSinaQuoteCode(target)
		if err != nil {
			problems = append(problems, err.Error())
			continue
		}
		itemByKey[target.Key] = item
		sinaCodes = append(sinaCodes, code)
		targetByCode[code] = target
	}

	if len(sinaCodes) == 0 {
		return quotes, monitor.JoinProblems(problems)
	}

	sinaHeaders := map[string]string{
		"Referer":    datasource.SinaFinanceReferer,
		"User-Agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
	}

	for _, batch := range chunkStrings(sinaCodes, sinaBatchSize) {
		text, err := fetchTextWithHeaders(ctx, p.client, datasource.SinaQuoteAPI+strings.Join(batch, ","), sinaHeaders, true)
		if err != nil {
			problems = append(problems, err.Error())
			continue
		}

		for _, line := range strings.Split(text, "\n") {
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
			quote, ok := buildSinaQuote(item, code, fields)
			if !ok {
				continue
			}
			quote.Symbol = target.DisplaySymbol
			quote.Market = target.Market
			quote.Currency = firstNonEmpty(quote.Currency, target.Currency)
			quotes[target.Key] = quote
		}
	}

	if len(quotes) == 0 && len(problems) == 0 {
		problems = append(problems, "Sina quote response is empty")
	}
	return quotes, monitor.JoinProblems(problems)
}

func resolveSinaQuoteCode(target monitor.QuoteTarget) (string, error) {
	switch {
	case strings.HasSuffix(target.Key, ".SH"):
		return "sh" + strings.TrimSuffix(target.Key, ".SH"), nil
	case strings.HasSuffix(target.Key, ".SZ"):
		return "sz" + strings.TrimSuffix(target.Key, ".SZ"), nil
	case strings.HasSuffix(target.Key, ".BJ"):
		return "bj" + strings.TrimSuffix(target.Key, ".BJ"), nil
	case strings.HasSuffix(target.Key, ".HK"):
		return "rt_hk" + strings.TrimSuffix(target.Key, ".HK"), nil
	case target.Market == "US-STOCK" || target.Market == "US-ETF":
		return "gb_" + strings.ToLower(target.DisplaySymbol), nil
	default:
		return "", fmt.Errorf("Sina does not support item: %s", target.DisplaySymbol)
	}
}

func parseSinaQuoteLine(line string) (string, []string, bool) {
	const prefix = "var hq_str_"
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
	return code, strings.Split(raw, ","), true
}

func buildSinaQuote(item monitor.WatchlistItem, code string, fields []string) (monitor.Quote, bool) {
	switch {
	case strings.HasPrefix(code, "sh") || strings.HasPrefix(code, "sz") || strings.HasPrefix(code, "bj"):
		if len(fields) < 6 {
			return monitor.Quote{}, false
		}
		name := partsAt(fields, 0)
		open := parseFloat(partsAt(fields, 1))
		previous := parseFloat(partsAt(fields, 2))
		current := parseFloat(partsAt(fields, 3))
		high := parseFloat(partsAt(fields, 4))
		low := parseFloat(partsAt(fields, 5))
		quote := buildQuote(firstNonEmpty(name, item.Name, item.Symbol), current, previous, open, high, low, time.Now(), "Sina Finance")
		quote.Currency = firstNonEmpty(item.Currency, "CNY")
		if len(fields) > 8 {
			quote.Volume = parseFloat(partsAt(fields, 8))
		}
		return quote, quote.CurrentPrice > 0
	case strings.HasPrefix(code, "rt_hk"):
		if len(fields) < 7 {
			return monitor.Quote{}, false
		}
		name := partsAt(fields, 1)
		open := parseFloat(partsAt(fields, 2))
		previous := parseFloat(partsAt(fields, 3))
		current := parseFloat(partsAt(fields, 6))
		high := parseFloat(partsAt(fields, 4))
		low := parseFloat(partsAt(fields, 5))
		updatedAt := parseTimestamp(partsAt(fields, 17) + " " + partsAt(fields, 18))
		if updatedAt.IsZero() {
			updatedAt = time.Now()
		}
		quote := buildQuote(firstNonEmpty(name, item.Name, item.Symbol), current, previous, open, high, low, updatedAt, "Sina Finance")
		quote.Currency = firstNonEmpty(item.Currency, "HKD")
		if len(fields) > 12 {
			quote.Volume = parseFloat(partsAt(fields, 12))
		}
		return quote, quote.CurrentPrice > 0
	case strings.HasPrefix(code, "gb_"):
		if len(fields) < 6 {
			return monitor.Quote{}, false
		}
		name := partsAt(fields, 0)
		current := parseFloat(partsAt(fields, 1))
		// field 2 = changePercent, field 4 = change amount (field 3 is the datetime string)
		changePercent := parseFloat(partsAt(fields, 2))
		change := parseFloat(partsAt(fields, 4))
		previous := current - change
		open := parseFloat(partsAt(fields, 5))
		high := parseFloat(partsAt(fields, 6))
		low := parseFloat(partsAt(fields, 7))
		quote := buildQuote(firstNonEmpty(name, item.Name, item.Symbol), current, previous, open, high, low, time.Now(), "Sina Finance")
		quote.Change = change
		quote.ChangePercent = changePercent
		quote.Currency = firstNonEmpty(item.Currency, "USD")
		if len(fields) > 12 {
			quote.Volume = parseFloat(partsAt(fields, 10))
			quote.MarketCap = parseFloat(partsAt(fields, 12))
		}
		return quote, quote.CurrentPrice > 0
	default:
		return monitor.Quote{}, false
	}
}
