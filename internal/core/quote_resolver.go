package core

import (
	"errors"
	"fmt"
	"strings"
)

type quoteAffixRule struct {
	token    string
	exchange string
}

var quotePrefixRules = []quoteAffixRule{
	{token: "HK", exchange: "HK"},
	{token: "SH", exchange: "SH"},
	{token: "SZ", exchange: "SZ"},
	{token: "BJ", exchange: "BJ"},
}

var quoteSuffixRules = []quoteAffixRule{
	{token: ".HK", exchange: "HK"},
	{token: ".SH", exchange: "SH"},
	{token: ".SZ", exchange: "SZ"},
	{token: ".BJ", exchange: "BJ"},
}

// ResolveQuoteTarget resolves a WatchlistItem into its canonical QuoteTarget.
func ResolveQuoteTarget(item WatchlistItem) (QuoteTarget, error) {
	return resolveQuoteTarget(item.Symbol, item.Market, item.Currency)
}

// resolveQuoteTarget normalizes a raw symbol, market, and currency string into a canonical QuoteTarget.
func resolveQuoteTarget(symbol, market, currency string) (QuoteTarget, error) {
	rawSymbol := strings.ToUpper(strings.TrimSpace(symbol))
	if rawSymbol == "" {
		return QuoteTarget{}, errors.New("Symbol is required")
	}

	market = normaliseMarketLabel(market)
	rawSymbol = strings.ReplaceAll(rawSymbol, " ", "")

	// Normalize the many input formats a user may provide to a small set of canonical forms before dispatching.
	if target, ok, err := resolveExplicitQuoteTarget(rawSymbol, market, currency); ok {
		return target, err
	}

	if IsDigits(rawSymbol) {
		return buildNumericTarget(rawSymbol, market, currency)
	}
	if isUSSymbol(rawSymbol) {
		return buildUSTarget(rawSymbol, market, currency)
	}
	if strings.HasPrefix(rawSymbol, "US") && isUSSymbol(rawSymbol[2:]) {
		return buildUSTarget(rawSymbol[2:], market, currency)
	}

	return QuoteTarget{}, fmt.Errorf("Unrecognized symbol: %s", rawSymbol)
}

func resolveExplicitQuoteTarget(rawSymbol, market, currency string) (QuoteTarget, bool, error) {
	if ticker, ok := strings.CutPrefix(rawSymbol, "GB_"); ok && isUSSymbol(ticker) {
		target, err := buildUSTarget(ticker, market, currency)
		return target, true, err
	}

	if target, ok, err := resolveAffixedQuoteTarget(rawSymbol, market, currency, quotePrefixRules, strings.CutPrefix); ok {
		return target, true, err
	}
	if target, ok, err := resolveAffixedQuoteTarget(rawSymbol, market, currency, quoteSuffixRules, strings.CutSuffix); ok {
		return target, true, err
	}

	return QuoteTarget{}, false, nil
}

func resolveAffixedQuoteTarget(
	rawSymbol, market, currency string,
	rules []quoteAffixRule,
	trim func(string, string) (string, bool),
) (QuoteTarget, bool, error) {
	for _, rule := range rules {
		code, ok := trim(rawSymbol, rule.token)
		if !ok || !IsDigits(code) {
			continue
		}

		target, err := buildExchangeTarget(code, rule.exchange, market, currency)
		return target, true, err
	}

	return QuoteTarget{}, false, nil
}

func buildExchangeTarget(rawSymbol, exchange, market, currency string) (QuoteTarget, error) {
	switch exchange {
	case "HK":
		return buildHKTarget(rawSymbol, resolveHKMarket(market), currency)
	case "SH", "SZ":
		return buildCNTarget(rawSymbol, exchange, resolveCNMarket(rawSymbol, market), currency)
	case "BJ":
		return buildBJTarget(rawSymbol, currency)
	default:
		return QuoteTarget{}, fmt.Errorf("Unsupported exchange: %s", exchange)
	}
}

// buildNumericTarget processes pure numeric codes and derives the final target based on market semantics.
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
		return QuoteTarget{}, fmt.Errorf("Cannot infer market for numeric symbol: %s", rawSymbol)
	}

	// 6-digit numeric codes may be A-shares, ETFs, or Beijing Exchange; need to infer based on prefix rules.
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

// buildCNTarget constructs the standard target for Shanghai/Shenzhen market items.
func buildCNTarget(rawSymbol, exchange, market, currency string) (QuoteTarget, error) {
	if len(rawSymbol) != 6 {
		return QuoteTarget{}, fmt.Errorf("A-share symbol must be 6 digits: %s", rawSymbol)
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

// buildBJTarget constructs the standard target for Beijing Exchange items.
func buildBJTarget(rawSymbol, currency string) (QuoteTarget, error) {
	if len(rawSymbol) != 6 {
		return QuoteTarget{}, fmt.Errorf("Beijing Exchange symbol must be 6 digits: %s", rawSymbol)
	}

	return QuoteTarget{
		Key:           rawSymbol + ".BJ",
		DisplaySymbol: rawSymbol + ".BJ",
		Market:        "CN-BJ",
		Currency:      defaultCurrency(currency, "CNY"),
	}, nil
}

// buildHKTarget constructs the standard target for Hong Kong stock items.
func buildHKTarget(rawSymbol, market, currency string) (QuoteTarget, error) {
	if !IsDigits(rawSymbol) {
		return QuoteTarget{}, fmt.Errorf("Hong Kong symbol must be numeric: %s", rawSymbol)
	}
	if len(rawSymbol) > 5 {
		return QuoteTarget{}, fmt.Errorf("Hong Kong symbol length is invalid: %s", rawSymbol)
	}

	// Hong Kong stock API requires 5-digit codes; pad with zeros on the left when insufficient.
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

// buildUSTarget constructs the standard target for US stocks or US ETFs.
func buildUSTarget(rawSymbol, market, currency string) (QuoteTarget, error) {
	if !isUSSymbol(rawSymbol) {
		return QuoteTarget{}, fmt.Errorf("US symbol is invalid: %s", rawSymbol)
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

// normaliseMarketLabel maps any recognized market label variant to the canonical internal market identifier.
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

// inferCNMarketAndExchange infers A-share market and exchange based on 6-digit numeric code.
func inferCNMarketAndExchange(rawSymbol string) (market, exchange string, err error) {
	if len(rawSymbol) != 6 {
		return "", "", fmt.Errorf("A-share / fund symbol must be 6 digits: %s", rawSymbol)
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
	return "", "", fmt.Errorf("Cannot recognize A-share / ETF symbol: %s", rawSymbol)
}

// resolveCNMarket determines the final A-share market type between stored values and code inference results.
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

// resolveHKMarket returns the normalized Hong Kong stock market type.
func resolveHKMarket(storedMarket string) string {
	switch storedMarket {
	case "HK-GEM", "HK-ETF":
		return storedMarket
	}
	return "HK-MAIN"
}

// defaultCurrency returns the normalized currency; if empty, uses the fallback value.
func defaultCurrency(currency, fallback string) string {
	currency = strings.ToUpper(strings.TrimSpace(currency))
	if currency == "" {
		return fallback
	}
	return currency
}

// IsDigits reports whether s consists entirely of ASCII digit characters.
func IsDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// isUSSymbol checks whether a string matches the US stock code character set.
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

// normaliseUSSymbol converts US stock codes to the standard format used internally by the system.
func normaliseUSSymbol(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, ".", "-")
	return value
}
