package monitor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

const (
	sinaQuoteURL = "http://hq.sinajs.cn/list=%s"
	txQuoteURL   = "http://qt.gtimg.cn/?q=%s"
)

type Quote struct {
	Symbol        string
	Name          string
	Market        string
	Currency      string
	CurrentPrice  float64
	PreviousClose float64
	OpenPrice     float64
	DayHigh       float64
	DayLow        float64
	Change        float64
	ChangePercent float64
	Source        string
	UpdatedAt     time.Time
}

type QuoteProvider interface {
	Fetch(ctx context.Context, items []WatchlistItem) (map[string]Quote, error)
	Name() string
}

type PublicQuoteProvider struct {
	client *http.Client
}

type quoteTarget struct {
	Key           string
	DisplaySymbol string
	Market        string
	Currency      string
	TXCode        string
	SinaCode      string
}

func NewPublicQuoteProvider(client *http.Client) *PublicQuoteProvider {
	if client == nil {
		client = &http.Client{Timeout: 8 * time.Second}
	}

	return &PublicQuoteProvider{client: client}
}

func (p *PublicQuoteProvider) Name() string {
	return "Tencent + Sina"
}

func ResolveQuoteTarget(item WatchlistItem) (quoteTarget, error) {
	return resolveQuoteTarget(item.Symbol, item.Market, item.Currency)
}

func (p *PublicQuoteProvider) Fetch(ctx context.Context, items []WatchlistItem) (map[string]Quote, error) {
	targets, problems := collectQuoteTargets(items)

	quotes := make(map[string]Quote, len(targets))
	if len(targets) == 0 {
		return quotes, collapseProblems(problems)
	}

	txTargets := make([]quoteTarget, 0, len(targets))
	for _, target := range targets {
		if target.TXCode != "" {
			txTargets = append(txTargets, target)
		}
	}

	txQuotes, txProblems := p.fetchTencent(ctx, txTargets)
	for key, quote := range txQuotes {
		quotes[key] = quote
	}
	problems = append(problems, txProblems...)

	sinaTargets := make([]quoteTarget, 0, len(targets))
	for key, target := range targets {
		if _, ok := quotes[key]; ok {
			continue
		}
		if target.SinaCode != "" {
			sinaTargets = append(sinaTargets, target)
			continue
		}
		problems = append(problems, fmt.Sprintf("没有可用行情代码: %s", target.DisplaySymbol))
	}

	sinaQuotes, sinaProblems := p.fetchSina(ctx, sinaTargets)
	for key, quote := range sinaQuotes {
		quotes[key] = quote
	}
	problems = append(problems, sinaProblems...)

	for key, target := range targets {
		if _, ok := quotes[key]; ok {
			continue
		}
		problems = append(problems, fmt.Sprintf("未收到 %s 的实时行情", target.DisplaySymbol))
	}

	return quotes, collapseProblems(problems)
}

func collectQuoteTargets(items []WatchlistItem) (map[string]quoteTarget, []string) {
	targets := make(map[string]quoteTarget, len(items))
	var problems []string

	for _, item := range items {
		target, err := ResolveQuoteTarget(item)
		if err != nil {
			problems = append(problems, err.Error())
			continue
		}
		targets[target.Key] = target
	}

	return targets, problems
}

func (p *PublicQuoteProvider) fetchTencent(ctx context.Context, targets []quoteTarget) (map[string]Quote, []string) {
	quotes := make(map[string]Quote, len(targets))
	if len(targets) == 0 {
		return quotes, nil
	}

	requestCodes := make([]string, 0, len(targets))
	indexByCode := make(map[string]quoteTarget, len(targets))
	for _, target := range targets {
		requestCode := target.TXCode
		if strings.HasPrefix(target.TXCode, "hk") {
			requestCode = "r_" + requestCode
		}
		requestCodes = append(requestCodes, requestCode)
		indexByCode[target.TXCode] = target
	}

	body, err := p.doRequest(ctx, fmt.Sprintf(txQuoteURL, url.QueryEscape(strings.Join(requestCodes, ","))), map[string]string{
		"Host":       "qt.gtimg.cn",
		"Referer":    "https://gu.qq.com/",
		"User-Agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/123.0.0.0 Safari/537.36",
	})
	if err != nil {
		return quotes, []string{fmt.Sprintf("腾讯行情请求失败: %v", err)}
	}

	lines := strings.Split(strings.TrimSpace(body), ";")
	var problems []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		rawCode, quote, err := parseTencentLine(line)
		if err != nil {
			problems = append(problems, fmt.Sprintf("腾讯行情解析失败: %v", err))
			continue
		}

		target, ok := indexByCode[rawCode]
		if !ok {
			continue
		}

		quote.Symbol = target.DisplaySymbol
		quote.Market = target.Market
		quote.Currency = target.Currency
		quotes[target.Key] = quote
	}

	return quotes, problems
}

func (p *PublicQuoteProvider) fetchSina(ctx context.Context, targets []quoteTarget) (map[string]Quote, []string) {
	quotes := make(map[string]Quote, len(targets))
	if len(targets) == 0 {
		return quotes, nil
	}

	requestCodes := make([]string, 0, len(targets))
	indexByCode := make(map[string]quoteTarget, len(targets))
	for _, target := range targets {
		requestCodes = append(requestCodes, target.SinaCode)
		indexByCode[target.SinaCode] = target
	}

	body, err := p.doRequest(ctx, fmt.Sprintf(sinaQuoteURL, url.QueryEscape(strings.Join(requestCodes, ","))), map[string]string{
		"Host":       "hq.sinajs.cn",
		"Referer":    "https://finance.sina.com.cn/",
		"User-Agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/123.0.0.0 Safari/537.36",
	})
	if err != nil {
		return quotes, []string{fmt.Sprintf("新浪行情请求失败: %v", err)}
	}

	lines := strings.Split(strings.TrimSpace(body), "\n")
	var problems []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		rawCode, quote, err := parseSinaLine(line)
		if err != nil {
			problems = append(problems, fmt.Sprintf("新浪行情解析失败: %v", err))
			continue
		}

		target, ok := indexByCode[rawCode]
		if !ok {
			continue
		}

		quote.Symbol = target.DisplaySymbol
		quote.Market = target.Market
		quote.Currency = target.Currency
		quotes[target.Key] = quote
	}

	return quotes, problems
}

func (p *PublicQuoteProvider) doRequest(ctx context.Context, requestURL string, headers map[string]string) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return "", err
	}

	for key, value := range headers {
		request.Header.Set(key, value)
	}

	response, err := p.client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status %d", response.StatusCode)
	}

	reader := transform.NewReader(response.Body, simplifiedchinese.GB18030.NewDecoder())
	payload, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(bytes.TrimPrefix(payload, []byte{0xef, 0xbb, 0xbf}))), nil
}

func resolveQuoteTarget(symbol, market, currency string) (quoteTarget, error) {
	rawSymbol := strings.ToUpper(strings.TrimSpace(symbol))
	if rawSymbol == "" {
		return quoteTarget{}, errors.New("股票代码不能为空")
	}

	market = normaliseMarketLabel(market)
	rawSymbol = strings.ReplaceAll(rawSymbol, " ", "")

	switch {
	case strings.HasPrefix(rawSymbol, "GB_"):
		ticker := strings.TrimPrefix(rawSymbol, "GB_")
		return buildUSTarget(ticker, market, currency)
	case strings.HasPrefix(rawSymbol, "HK") && isDigits(rawSymbol[2:]):
		return buildHKTarget(rawSymbol[2:], currency)
	case strings.HasPrefix(rawSymbol, "SH") && isDigits(rawSymbol[2:]):
		return buildCNTarget(rawSymbol[2:], "SH", currency)
	case strings.HasPrefix(rawSymbol, "SZ") && isDigits(rawSymbol[2:]):
		return buildCNTarget(rawSymbol[2:], "SZ", currency)
	case strings.HasPrefix(rawSymbol, "BJ") && isDigits(rawSymbol[2:]):
		return buildBJTarget(rawSymbol[2:], currency)
	case strings.HasSuffix(rawSymbol, ".HK") && isDigits(strings.TrimSuffix(rawSymbol, ".HK")):
		return buildHKTarget(strings.TrimSuffix(rawSymbol, ".HK"), currency)
	case strings.HasSuffix(rawSymbol, ".SH") && isDigits(strings.TrimSuffix(rawSymbol, ".SH")):
		return buildCNTarget(strings.TrimSuffix(rawSymbol, ".SH"), "SH", currency)
	case strings.HasSuffix(rawSymbol, ".SZ") && isDigits(strings.TrimSuffix(rawSymbol, ".SZ")):
		return buildCNTarget(strings.TrimSuffix(rawSymbol, ".SZ"), "SZ", currency)
	case strings.HasSuffix(rawSymbol, ".BJ") && isDigits(strings.TrimSuffix(rawSymbol, ".BJ")):
		return buildBJTarget(strings.TrimSuffix(rawSymbol, ".BJ"), currency)
	case isDigits(rawSymbol):
		return buildNumericTarget(rawSymbol, market, currency)
	case isUSSymbol(rawSymbol):
		return buildUSTarget(rawSymbol, market, currency)
	case strings.HasPrefix(rawSymbol, "US") && isUSSymbol(rawSymbol[2:]):
		return buildUSTarget(rawSymbol[2:], market, currency)
	default:
		return quoteTarget{}, fmt.Errorf("无法识别股票代码: %s", rawSymbol)
	}
}

func buildNumericTarget(rawSymbol, market, currency string) (quoteTarget, error) {
	switch market {
	case "HK":
		return buildHKTarget(rawSymbol, currency)
	case "BJ":
		return buildBJTarget(rawSymbol, currency)
	}

	if len(rawSymbol) == 5 && market == "" {
		return buildHKTarget(rawSymbol, currency)
	}

	switch rawSymbol[0] {
	case '6', '9':
		return buildCNTarget(rawSymbol, "SH", currency)
	case '0', '2', '3':
		return buildCNTarget(rawSymbol, "SZ", currency)
	case '4', '8':
		return buildBJTarget(rawSymbol, currency)
	default:
		return quoteTarget{}, fmt.Errorf("无法识别数字代码归属市场: %s", rawSymbol)
	}
}

func buildCNTarget(rawSymbol, exchange, currency string) (quoteTarget, error) {
	if len(rawSymbol) != 6 {
		return quoteTarget{}, fmt.Errorf("A 股代码应为 6 位: %s", rawSymbol)
	}

	exchange = strings.ToUpper(exchange)
	code := strings.ToLower(exchange) + rawSymbol
	return quoteTarget{
		Key:           strings.ToUpper(rawSymbol + "." + exchange),
		DisplaySymbol: strings.ToUpper(rawSymbol + "." + exchange),
		Market:        "A-Share",
		Currency:      defaultCurrency(currency, "CNY"),
		TXCode:        code,
		SinaCode:      code,
	}, nil
}

func buildBJTarget(rawSymbol, currency string) (quoteTarget, error) {
	if len(rawSymbol) != 6 {
		return quoteTarget{}, fmt.Errorf("北交所代码应为 6 位: %s", rawSymbol)
	}

	code := "bj" + rawSymbol
	return quoteTarget{
		Key:           strings.ToUpper(rawSymbol + ".BJ"),
		DisplaySymbol: strings.ToUpper(rawSymbol + ".BJ"),
		Market:        "BJ",
		Currency:      defaultCurrency(currency, "CNY"),
		SinaCode:      code,
	}, nil
}

func buildHKTarget(rawSymbol, currency string) (quoteTarget, error) {
	if !isDigits(rawSymbol) {
		return quoteTarget{}, fmt.Errorf("港股代码必须为数字: %s", rawSymbol)
	}
	if len(rawSymbol) > 5 {
		return quoteTarget{}, fmt.Errorf("港股代码长度异常: %s", rawSymbol)
	}

	padded := rawSymbol
	if len(padded) < 5 {
		padded = strings.Repeat("0", 5-len(padded)) + padded
	}
	code := "hk" + padded
	return quoteTarget{
		Key:           strings.ToUpper(padded + ".HK"),
		DisplaySymbol: strings.ToUpper(padded + ".HK"),
		Market:        "HK",
		Currency:      defaultCurrency(currency, "HKD"),
		TXCode:        code,
		SinaCode:      code,
	}, nil
}

func buildUSTarget(rawSymbol, market, currency string) (quoteTarget, error) {
	if !isUSSymbol(rawSymbol) {
		return quoteTarget{}, fmt.Errorf("美股代码格式无效: %s", rawSymbol)
	}

	label := "US"
	if market == "US ETF" {
		label = "US ETF"
	}

	ticker := normaliseUSSymbol(rawSymbol)
	return quoteTarget{
		Key:           ticker,
		DisplaySymbol: ticker,
		Market:        label,
		Currency:      defaultCurrency(currency, "USD"),
		SinaCode:      "gb_" + strings.ToLower(ticker),
	}, nil
}

func parseTencentLine(line string) (string, Quote, error) {
	left, right, found := strings.Cut(line, "=")
	if !found {
		return "", Quote{}, fmt.Errorf("unexpected payload: %s", line)
	}

	code := strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(left), "v_"), "r_")
	parts := splitClean(strings.Trim(right, "\";"), "~")
	if len(parts) < 6 {
		return "", Quote{}, fmt.Errorf("字段不足: %s", code)
	}

	current := parseFloat(parts[3])
	previous := parseFloat(parts[4])
	open := parseFloat(parts[5])

	high := 0.0
	low := 0.0
	if len(parts) > 34 {
		high = parseFloat(parts[33])
		low = parseFloat(parts[34])
	} else if len(parts) > 33 {
		high = parseFloat(parts[32])
		low = parseFloat(parts[33])
	}

	return code, buildQuote(
		parts[1],
		current,
		previous,
		open,
		high,
		low,
		parseTencentTime(parts),
		"Tencent",
	), nil
}

func parseSinaLine(line string) (string, Quote, error) {
	left, right, found := strings.Cut(line, "=")
	if !found {
		return "", Quote{}, fmt.Errorf("unexpected payload: %s", line)
	}

	code := strings.TrimPrefix(strings.TrimSpace(left), "var hq_str_")
	payload := strings.Trim(right, "\";")
	if payload == "" {
		return "", Quote{}, fmt.Errorf("空行情: %s", code)
	}

	parts := splitClean(payload, ",")
	switch {
	case strings.HasPrefix(code, "gb_"):
		quote, err := parseSinaUSQuote(parts)
		return code, quote, err
	case strings.HasPrefix(code, "hk"):
		quote, err := parseSinaHKQuote(parts)
		return code, quote, err
	case strings.HasPrefix(code, "sh"), strings.HasPrefix(code, "sz"), strings.HasPrefix(code, "bj"):
		quote, err := parseSinaCNQuote(parts)
		return code, quote, err
	default:
		return "", Quote{}, fmt.Errorf("不支持的新浪代码: %s", code)
	}
}

func parseSinaUSQuote(parts []string) (Quote, error) {
	if len(parts) < 8 {
		return Quote{}, errors.New("美股字段不足")
	}

	previous := 0.0
	switch {
	case len(parts) > 35:
		previous = parseFloat(parts[35])
	case len(parts) > 26:
		previous = parseFloat(parts[26])
	default:
		previous = parseFloat(parts[len(parts)-1])
	}

	updatedAt := parseTimestamp(firstNonEmpty(partsAt(parts, 3), partsAt(parts, 24)))
	return buildQuote(
		partsAt(parts, 0),
		parseFloat(partsAt(parts, 1)),
		previous,
		parseFloat(partsAt(parts, 5)),
		parseFloat(partsAt(parts, 6)),
		parseFloat(partsAt(parts, 7)),
		updatedAt,
		"Sina",
	), nil
}

func parseSinaHKQuote(parts []string) (Quote, error) {
	if len(parts) < 19 {
		return Quote{}, errors.New("港股字段不足")
	}

	updatedAt := parseTimestamp(strings.TrimSpace(parts[17]) + " " + strings.TrimSpace(parts[18]))
	return buildQuote(
		parts[1],
		parseFloat(parts[6]),
		parseFloat(parts[3]),
		parseFloat(parts[2]),
		parseFloat(parts[4]),
		parseFloat(parts[5]),
		updatedAt,
		"Sina",
	), nil
}

func parseSinaCNQuote(parts []string) (Quote, error) {
	if len(parts) < 32 {
		return Quote{}, errors.New("A 股字段不足")
	}

	updatedAt := parseTimestamp(strings.TrimSpace(parts[30]) + " " + strings.TrimSpace(parts[31]))
	return buildQuote(
		parts[0],
		parseFloat(parts[3]),
		parseFloat(parts[2]),
		parseFloat(parts[1]),
		parseFloat(parts[4]),
		parseFloat(parts[5]),
		updatedAt,
		"Sina",
	), nil
}

func buildQuote(name string, current, previous, open, high, low float64, updatedAt time.Time, source string) Quote {
	change := 0.0
	changePercent := 0.0
	if previous > 0 {
		change = current - previous
		changePercent = change / previous * 100
	}

	return Quote{
		Name:          strings.TrimSpace(name),
		CurrentPrice:  current,
		PreviousClose: previous,
		OpenPrice:     open,
		DayHigh:       high,
		DayLow:        low,
		Change:        change,
		ChangePercent: changePercent,
		Source:        source,
		UpdatedAt:     updatedAt,
	}
}

func parseTencentTime(parts []string) time.Time {
	candidates := []string{
		partsAt(parts, 30),
		partsAt(parts, 29),
	}
	for _, candidate := range candidates {
		if ts := parseTimestamp(candidate); !ts.IsZero() {
			return ts
		}
	}
	return time.Now()
}

func parseTimestamp(raw string) time.Time {
	candidate := strings.TrimSpace(strings.NewReplacer("/", "-", "\"", "", ";", "").Replace(raw))
	if candidate == "" {
		return time.Time{}
	}

	layouts := []string{
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"20060102150405",
	}

	for _, layout := range layouts {
		if parsed, err := time.ParseInLocation(layout, candidate, time.Local); err == nil {
			return parsed
		}
	}

	return time.Time{}
}

func parseFloat(raw string) float64 {
	clean := strings.TrimSpace(strings.NewReplacer("\"", "", ";", "", ",", "").Replace(raw))
	if clean == "" || clean == "-" {
		return 0
	}

	value, err := strconv.ParseFloat(clean, 64)
	if err != nil {
		return 0
	}
	return value
}

func normaliseMarketLabel(market string) string {
	switch strings.ToUpper(strings.TrimSpace(market)) {
	case "A-SHARE", "ASHARE", "CN", "A":
		return "A-Share"
	case "HK", "H-SHARE":
		return "HK"
	case "US", "NASDAQ", "NYSE":
		return "US"
	case "US ETF", "ETF":
		return "US ETF"
	case "BJ":
		return "BJ"
	default:
		return strings.TrimSpace(market)
	}
}

func splitClean(raw, sep string) []string {
	parts := strings.Split(raw, sep)
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		result = append(result, strings.TrimSpace(part))
	}
	return result
}

func defaultCurrency(currency, fallback string) string {
	currency = strings.ToUpper(strings.TrimSpace(currency))
	if currency == "" {
		return fallback
	}
	return currency
}

func isDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func isLetters(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') {
			return false
		}
	}
	return true
}

func isUSSymbol(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		switch {
		case r >= 'A' && r <= 'Z':
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '-' || r == '.':
		default:
			return false
		}
	}
	return true
}

func normaliseUSSymbol(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, ".", "-")
	return value
}

func collapseProblems(problems []string) error {
	if len(problems) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(problems))
	uniq := make([]string, 0, len(problems))
	for _, problem := range problems {
		problem = strings.TrimSpace(problem)
		if problem == "" {
			continue
		}
		if _, exists := seen[problem]; exists {
			continue
		}
		seen[problem] = struct{}{}
		uniq = append(uniq, problem)
	}

	if len(uniq) == 0 {
		return nil
	}

	return errors.New(strings.Join(uniq, "；"))
}

func partsAt(parts []string, index int) string {
	if index < 0 || index >= len(parts) {
		return ""
	}
	return parts[index]
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
