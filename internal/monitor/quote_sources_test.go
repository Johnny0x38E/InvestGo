package monitor

import "testing"

func TestResolveEastMoneySecID(t *testing.T) {
	t.Run("A share", func(t *testing.T) {
		secid, err := resolveEastMoneySecID(quoteTarget{DisplaySymbol: "600519.SH"})
		if err != nil {
			t.Fatalf("resolveEastMoneySecID returned error: %v", err)
		}
		if secid != "1.600519" {
			t.Fatalf("unexpected secid: %s", secid)
		}
	})

	t.Run("Hong Kong", func(t *testing.T) {
		secid, err := resolveEastMoneySecID(quoteTarget{DisplaySymbol: "00700.HK"})
		if err != nil {
			t.Fatalf("resolveEastMoneySecID returned error: %v", err)
		}
		if secid != "116.00700" {
			t.Fatalf("unexpected secid: %s", secid)
		}
	})

	t.Run("US", func(t *testing.T) {
		secid, err := resolveEastMoneySecID(quoteTarget{DisplaySymbol: "QQQ"})
		if err != nil {
			t.Fatalf("resolveEastMoneySecID returned error: %v", err)
		}
		if secid != "105.QQQ" {
			t.Fatalf("unexpected secid: %s", secid)
		}
	})
}

func TestSanitiseSettingsPriceColorScheme(t *testing.T) {
	settings, err := sanitiseSettings(
		AppSettings{
			QuoteSource:            defaultQuoteSourceID,
			RefreshIntervalSeconds: 20,
			PriceColorScheme:       "intl",
		},
		AppSettings{},
		map[string]QuoteProvider{
			defaultQuoteSourceID: NewPublicQuoteProvider(nil),
		},
	)
	if err != nil {
		t.Fatalf("sanitiseSettings returned error: %v", err)
	}
	if settings.PriceColorScheme != "intl" {
		t.Fatalf("unexpected price color scheme: %s", settings.PriceColorScheme)
	}
}

func TestSanitiseSettingsDeveloperModeCanBeDisabled(t *testing.T) {
	settings, err := sanitiseSettings(
		AppSettings{
			QuoteSource:            defaultQuoteSourceID,
			RefreshIntervalSeconds: 20,
			DeveloperMode:          false,
		},
		AppSettings{
			QuoteSource:            defaultQuoteSourceID,
			RefreshIntervalSeconds: 20,
			DeveloperMode:          true,
		},
		map[string]QuoteProvider{
			defaultQuoteSourceID: NewPublicQuoteProvider(nil),
		},
	)
	if err != nil {
		t.Fatalf("sanitiseSettings returned error: %v", err)
	}
	if settings.DeveloperMode {
		t.Fatalf("expected developer mode to be disabled")
	}
}
