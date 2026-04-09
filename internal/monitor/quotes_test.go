package monitor

import (
	"strings"
	"testing"
)

func TestResolveQuoteTarget(t *testing.T) {
	t.Run("A share", func(t *testing.T) {
		target, err := resolveQuoteTarget("600519.SH", "", "")
		if err != nil {
			t.Fatalf("resolveQuoteTarget returned error: %v", err)
		}
		if target.DisplaySymbol != "600519.SH" {
			t.Fatalf("unexpected symbol: %s", target.DisplaySymbol)
		}
		if target.Market != "A-Share" || target.Currency != "CNY" {
			t.Fatalf("unexpected market or currency: %+v", target)
		}
	})

	t.Run("Hong Kong", func(t *testing.T) {
		target, err := resolveQuoteTarget("700", "HK", "")
		if err != nil {
			t.Fatalf("resolveQuoteTarget returned error: %v", err)
		}
		if target.DisplaySymbol != "00700.HK" {
			t.Fatalf("unexpected symbol: %s", target.DisplaySymbol)
		}
		if target.TXCode != "hk00700" {
			t.Fatalf("unexpected tx code: %s", target.TXCode)
		}
	})

	t.Run("US ETF", func(t *testing.T) {
		target, err := resolveQuoteTarget("QQQ", "US ETF", "")
		if err != nil {
			t.Fatalf("resolveQuoteTarget returned error: %v", err)
		}
		if target.DisplaySymbol != "QQQ" || target.SinaCode != "gb_qqq" {
			t.Fatalf("unexpected target: %+v", target)
		}
		if target.Market != "US ETF" || target.Currency != "USD" {
			t.Fatalf("unexpected market or currency: %+v", target)
		}
	})
}

func TestParseTencentLine(t *testing.T) {
	fields := []string{
		"100", "TENCENT", "00700", "507.500", "489.200", "504.500",
		"0", "0", "0", "507.500", "0", "0", "0", "0", "0", "0", "0", "0", "0",
		"507.500", "0", "0", "0", "0", "0", "0", "0", "0", "0", "26375607.0",
		"2026/04/08 15:14:11", "18.300", "3.74", "509.000", "501.000", "507.500",
	}
	line := `v_r_hk00700="` + strings.Join(fields, "~") + `";`

	code, quote, err := parseTencentLine(line)
	if err != nil {
		t.Fatalf("parseTencentLine returned error: %v", err)
	}
	if code != "hk00700" {
		t.Fatalf("unexpected code: %s", code)
	}
	if quote.Name != "TENCENT" {
		t.Fatalf("unexpected name: %s", quote.Name)
	}
	if quote.CurrentPrice != 507.5 || quote.DayHigh != 509 || quote.DayLow != 501 {
		t.Fatalf("unexpected quote: %+v", quote)
	}
	if quote.Source != "Tencent" {
		t.Fatalf("unexpected source: %s", quote.Source)
	}
}

func TestParseSinaLineUS(t *testing.T) {
	line := `var hq_str_gb_aapl="APPLE,253.5000,-2.07,2026-04-08 09:30:12,-5.3600,256.1550,256.2000,245.7000,288.3600,168.1700,62148008,39359267,3721668989493,7.93,31.970000,0.00,0.00,0.00,0.00,14681139998,63,258.8800,2.12,5.38,Apr 07 07:59PM EDT,Apr 07 04:00PM EDT,258.8600,4463152,1,2026,15590068702.0000,259.2000,250.8246,1134750376.5076,253.4900,258.8600";`

	code, quote, err := parseSinaLine(line)
	if err != nil {
		t.Fatalf("parseSinaLine returned error: %v", err)
	}
	if code != "gb_aapl" {
		t.Fatalf("unexpected code: %s", code)
	}
	if quote.Name != "APPLE" {
		t.Fatalf("unexpected name: %s", quote.Name)
	}
	if quote.CurrentPrice != 253.5 || quote.PreviousClose != 258.86 {
		t.Fatalf("unexpected quote: %+v", quote)
	}
	if quote.Source != "Sina" {
		t.Fatalf("unexpected source: %s", quote.Source)
	}
}
