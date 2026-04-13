package monitor

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// DefaultQuoteSourceID 是默认启用的行情源标识。
const DefaultQuoteSourceID = "eastmoney"

const (
	DefaultCNQuoteSourceID = "eastmoney"
	DefaultHKQuoteSourceID = "eastmoney"
	DefaultUSQuoteSourceID = "eastmoney"
)

// Quote 表示前端仪表盘消费的统一行情结构。
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

// QuoteProvider 定义了实时行情 provider 的统一契约。
type QuoteProvider interface {
	Fetch(ctx context.Context, items []WatchlistItem) (map[string]Quote, error)
	Name() string
}

// QuoteSourceOption 描述一个可供用户选择的行情源。
type QuoteSourceOption struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Description      string   `json:"description"`
	SupportedMarkets []string `json:"supportedMarkets"`
}

// QuoteTarget 表示标的代码标准化后的通用结果。
type QuoteTarget struct {
	Key           string
	DisplaySymbol string
	Market        string
	Currency      string
}

// ResolveQuoteTarget 把标的标准化为系统内部统一使用的目标结构。
func ResolveQuoteTarget(item WatchlistItem) (QuoteTarget, error) {
	return resolveQuoteTarget(item.Symbol, item.Market, item.Currency)
}

// resolveQuoteTarget 根据代码、市场和币种推导统一的目标标识。
func resolveQuoteTarget(symbol, market, currency string) (QuoteTarget, error) {
	rawSymbol := strings.ToUpper(strings.TrimSpace(symbol))
	if rawSymbol == "" {
		return QuoteTarget{}, errors.New("股票代码不能为空")
	}

	market = normaliseMarketLabel(market)
	rawSymbol = strings.ReplaceAll(rawSymbol, " ", "")

	// 兼容不同输入风格，把用户输入收敛到少量标准形式后再分派。
	switch {
	case strings.HasPrefix(rawSymbol, "GB_"):
		ticker := strings.TrimPrefix(rawSymbol, "GB_")
		return buildUSTarget(ticker, market, currency)
	case strings.HasPrefix(rawSymbol, "HK") && isDigits(rawSymbol[2:]):
		return buildHKTarget(rawSymbol[2:], resolveHKMarket(market), currency)
	case strings.HasPrefix(rawSymbol, "SH") && isDigits(rawSymbol[2:]):
		num := rawSymbol[2:]
		return buildCNTarget(num, "SH", resolveCNMarket(num, market), currency)
	case strings.HasPrefix(rawSymbol, "SZ") && isDigits(rawSymbol[2:]):
		num := rawSymbol[2:]
		return buildCNTarget(num, "SZ", resolveCNMarket(num, market), currency)
	case strings.HasPrefix(rawSymbol, "BJ") && isDigits(rawSymbol[2:]):
		return buildBJTarget(rawSymbol[2:], currency)
	case strings.HasSuffix(rawSymbol, ".HK") && isDigits(strings.TrimSuffix(rawSymbol, ".HK")):
		return buildHKTarget(strings.TrimSuffix(rawSymbol, ".HK"), resolveHKMarket(market), currency)
	case strings.HasSuffix(rawSymbol, ".SH") && isDigits(strings.TrimSuffix(rawSymbol, ".SH")):
		num := strings.TrimSuffix(rawSymbol, ".SH")
		return buildCNTarget(num, "SH", resolveCNMarket(num, market), currency)
	case strings.HasSuffix(rawSymbol, ".SZ") && isDigits(strings.TrimSuffix(rawSymbol, ".SZ")):
		num := strings.TrimSuffix(rawSymbol, ".SZ")
		return buildCNTarget(num, "SZ", resolveCNMarket(num, market), currency)
	case strings.HasSuffix(rawSymbol, ".BJ") && isDigits(strings.TrimSuffix(rawSymbol, ".BJ")):
		return buildBJTarget(strings.TrimSuffix(rawSymbol, ".BJ"), currency)
	case isDigits(rawSymbol):
		return buildNumericTarget(rawSymbol, market, currency)
	case isUSSymbol(rawSymbol):
		return buildUSTarget(rawSymbol, market, currency)
	case strings.HasPrefix(rawSymbol, "US") && isUSSymbol(rawSymbol[2:]):
		return buildUSTarget(rawSymbol[2:], market, currency)
	default:
		return QuoteTarget{}, fmt.Errorf("无法识别股票代码: %s", rawSymbol)
	}
}

// buildNumericTarget 处理纯数字代码，并按市场语义推导最终目标。
func buildNumericTarget(rawSymbol, market, currency string) (QuoteTarget, error) {
	switch market {
	case "HK-MAIN", "HK-GEM", "HK-ETF":
		return buildHKTarget(rawSymbol, market, currency)
	case "HK":
		return buildHKTarget(rawSymbol, "HK-MAIN", currency)
	case "CN-BJ", "BJ":
		return buildBJTarget(rawSymbol, currency)
	}

	if len(rawSymbol) == 5 && market == "" {
		return buildHKTarget(rawSymbol, "HK-MAIN", currency)
	}

	if len(rawSymbol) != 6 {
		return QuoteTarget{}, fmt.Errorf("无法识别数字代码归属市场: %s", rawSymbol)
	}

	// 6 位数字代码可能是 A 股、ETF 或北交所，需要结合前缀规则推断。
	if market == "CN-GEM" || market == "CN-STAR" || market == "CN-ETF" {
		_, exchange, err := inferCNMarketAndExchange(rawSymbol)
		if err != nil {
			return QuoteTarget{}, err
		}
		return buildCNTarget(rawSymbol, exchange, market, currency)
	}

	detectedMarket, exchange, err := inferCNMarketAndExchange(rawSymbol)
	if err != nil {
		return QuoteTarget{}, err
	}
	return buildCNTarget(rawSymbol, exchange, detectedMarket, currency)
}

// buildCNTarget 构造沪深市场标的的标准目标。
func buildCNTarget(rawSymbol, exchange, market, currency string) (QuoteTarget, error) {
	if len(rawSymbol) != 6 {
		return QuoteTarget{}, fmt.Errorf("A 股代码应为 6 位: %s", rawSymbol)
	}

	exchange = strings.ToUpper(exchange)
	if market == "" {
		market = "CN-A"
	}
	return QuoteTarget{
		Key:           rawSymbol + "." + exchange,
		DisplaySymbol: rawSymbol + "." + exchange,
		Market:        market,
		Currency:      defaultCurrency(currency, "CNY"),
	}, nil
}

// buildBJTarget 构造北交所标的的标准目标。
func buildBJTarget(rawSymbol, currency string) (QuoteTarget, error) {
	if len(rawSymbol) != 6 {
		return QuoteTarget{}, fmt.Errorf("北交所代码应为 6 位: %s", rawSymbol)
	}

	return QuoteTarget{
		Key:           rawSymbol + ".BJ",
		DisplaySymbol: rawSymbol + ".BJ",
		Market:        "CN-BJ",
		Currency:      defaultCurrency(currency, "CNY"),
	}, nil
}

// buildHKTarget 构造港股标的的标准目标。
func buildHKTarget(rawSymbol, market, currency string) (QuoteTarget, error) {
	if !isDigits(rawSymbol) {
		return QuoteTarget{}, fmt.Errorf("港股代码必须为数字: %s", rawSymbol)
	}
	if len(rawSymbol) > 5 {
		return QuoteTarget{}, fmt.Errorf("港股代码长度异常: %s", rawSymbol)
	}

	// 港股接口要求 5 位代码，不足时统一左侧补零。
	padded := rawSymbol
	if len(padded) < 5 {
		padded = strings.Repeat("0", 5-len(padded)) + padded
	}
	if market == "" {
		market = "HK-MAIN"
	}
	return QuoteTarget{
		Key:           padded + ".HK",
		DisplaySymbol: padded + ".HK",
		Market:        market,
		Currency:      defaultCurrency(currency, "HKD"),
	}, nil
}

// buildUSTarget 构造美股或美股 ETF 的标准目标。
func buildUSTarget(rawSymbol, market, currency string) (QuoteTarget, error) {
	if !isUSSymbol(rawSymbol) {
		return QuoteTarget{}, fmt.Errorf("美股代码格式无效: %s", rawSymbol)
	}

	label := "US-STOCK"
	if market == "US-ETF" || market == "US ETF" {
		label = "US-ETF"
	}

	ticker := normaliseUSSymbol(rawSymbol)
	return QuoteTarget{
		Key:           ticker,
		DisplaySymbol: ticker,
		Market:        label,
		Currency:      defaultCurrency(currency, "USD"),
	}, nil
}

// normaliseMarketLabel 把市场标签兼容值收敛为系统内部的标准枚举。
func normaliseMarketLabel(market string) string {
	switch strings.ToUpper(strings.TrimSpace(market)) {
	case "A-SHARE", "ASHARE", "CN", "A", "CN-A":
		return "CN-A"
	case "CN-GEM", "GEM":
		return "CN-GEM"
	case "CN-STAR", "STAR":
		return "CN-STAR"
	case "CN-ETF", "CNETF":
		return "CN-ETF"
	case "CN-BJ", "BJ":
		return "CN-BJ"
	case "HK", "H-SHARE":
		return "HK-MAIN"
	case "HK-MAIN", "HK-GEM", "HK-ETF":
		return strings.TrimSpace(market)
	case "US", "NASDAQ", "NYSE", "US-NYQ":
		return "US-STOCK"
	case "US ETF", "ETF", "US-ETF":
		return "US-ETF"
	case "US-STOCK":
		return "US-STOCK"
	default:
		return strings.TrimSpace(market)
	}
}

// inferCNMarketAndExchange 根据 6 位数字代码推断 A 股市场和交易所。
func inferCNMarketAndExchange(rawSymbol string) (market, exchange string, err error) {
	if len(rawSymbol) != 6 {
		return "", "", fmt.Errorf("A股/基金代码应为 6 位: %s", rawSymbol)
	}
	if strings.HasPrefix(rawSymbol, "688") || strings.HasPrefix(rawSymbol, "689") {
		return "CN-STAR", "SH", nil
	}
	if rawSymbol[0] == '6' || rawSymbol[0] == '9' {
		return "CN-A", "SH", nil
	}
	if rawSymbol[0] == '5' {
		return "CN-ETF", "SH", nil
	}
	if rawSymbol[0] == '3' {
		return "CN-GEM", "SZ", nil
	}
	if strings.HasPrefix(rawSymbol, "15") || strings.HasPrefix(rawSymbol, "16") {
		return "CN-ETF", "SZ", nil
	}
	if rawSymbol[0] == '0' || rawSymbol[0] == '1' || rawSymbol[0] == '2' {
		return "CN-A", "SZ", nil
	}
	if rawSymbol[0] == '4' || rawSymbol[0] == '8' {
		return "CN-BJ", "BJ", nil
	}
	return "", "", fmt.Errorf("无法识别A股/ETF代码: %s", rawSymbol)
}

// resolveCNMarket 在已有存储值和代码推断结果之间确定最终的 A 股市场类型。
func resolveCNMarket(code, storedMarket string) string {
	switch storedMarket {
	case "CN-GEM", "CN-STAR", "CN-ETF":
		return storedMarket
	}
	market, _, err := inferCNMarketAndExchange(code)
	if err != nil {
		return "CN-A"
	}
	return market
}

// resolveHKMarket 返回规范化后的港股市场类型。
func resolveHKMarket(storedMarket string) string {
	switch storedMarket {
	case "HK-GEM", "HK-ETF":
		return storedMarket
	}
	return "HK-MAIN"
}

// defaultCurrency 返回标准化后的币种；若为空则使用回退值。
func defaultCurrency(currency, fallback string) string {
	currency = strings.ToUpper(strings.TrimSpace(currency))
	if currency == "" {
		return fallback
	}
	return currency
}

// isDigits 判断字符串是否全部由数字组成。
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

// isUSSymbol 判断字符串是否符合美股代码字符集。
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

// normaliseUSSymbol 把美股代码转换为系统内部使用的标准格式。
func normaliseUSSymbol(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, ".", "-")
	return value
}
