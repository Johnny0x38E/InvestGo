package monitor

import "testing"

// TestResolveQuoteTarget 验证不同市场代码都能被标准化为统一目标。
func TestResolveQuoteTarget(t *testing.T) {
	t.Run("A share", func(t *testing.T) {
		target, err := resolveQuoteTarget("600519.SH", "", "")
		if err != nil {
			t.Fatalf("resolveQuoteTarget returned error: %v", err)
		}
		if target.DisplaySymbol != "600519.SH" {
			t.Fatalf("unexpected symbol: %s", target.DisplaySymbol)
		}
		if target.Market != "CN-A" || target.Currency != "CNY" {
			t.Fatalf("unexpected market or currency: %+v", target)
		}
	})

	t.Run("Hong Kong", func(t *testing.T) {
		target, err := resolveQuoteTarget("700", "HK-MAIN", "")
		if err != nil {
			t.Fatalf("resolveQuoteTarget returned error: %v", err)
		}
		if target.DisplaySymbol != "00700.HK" {
			t.Fatalf("unexpected symbol: %s", target.DisplaySymbol)
		}
		if target.Market != "HK-MAIN" || target.Currency != "HKD" {
			t.Fatalf("unexpected market or currency: %+v", target)
		}
	})

	t.Run("US ETF", func(t *testing.T) {
		target, err := resolveQuoteTarget("QQQ", "US-ETF", "")
		if err != nil {
			t.Fatalf("resolveQuoteTarget returned error: %v", err)
		}
		if target.DisplaySymbol != "QQQ" || target.Key != "QQQ" {
			t.Fatalf("unexpected target: %+v", target)
		}
		if target.Market != "US-ETF" || target.Currency != "USD" {
			t.Fatalf("unexpected market or currency: %+v", target)
		}
	})
}
