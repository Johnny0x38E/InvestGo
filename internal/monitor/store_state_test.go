package monitor

import "testing"

// TestSnapshotLockedReturnsNonNilCollections 验证快照里的集合字段始终为非 nil 切片。
func TestSnapshotLockedReturnsNonNilCollections(t *testing.T) {
	store := &Store{
		quoteProviders: map[string]QuoteProvider{
			DefaultQuoteSourceID: nil,
		},
		quoteSourceOptions: nil,
		state: persistedState{
			Items:  nil,
			Alerts: nil,
			Settings: AppSettings{
				RefreshIntervalSeconds: 60,
				QuoteSource:            DefaultQuoteSourceID,
				CNQuoteSource:          DefaultCNQuoteSourceID,
				HKQuoteSource:          DefaultHKQuoteSourceID,
				USQuoteSource:          DefaultUSQuoteSourceID,
				ThemeMode:              "system",
				ColorTheme:             "blue",
				FontPreset:             "system",
				AmountDisplay:          "full",
				CurrencyDisplay:        "symbol",
				PriceColorScheme:       "cn",
				Locale:                 "system",
			},
		},
	}

	store.normaliseLocked()
	snapshot := store.snapshotLocked()

	if snapshot.Items == nil {
		t.Fatal("expected snapshot items to be a non-nil slice")
	}
	if snapshot.Alerts == nil {
		t.Fatal("expected snapshot alerts to be a non-nil slice")
	}
	if snapshot.QuoteSources == nil {
		t.Fatal("expected snapshot quote sources to be a non-nil slice")
	}
}
