package monitor

import "testing"

func TestSnapshotLockedReturnsNonNilCollections(t *testing.T) {
	store := &Store{
		quoteProviders: map[string]QuoteProvider{
			defaultQuoteSourceID: nil,
		},
		quoteSourceOptions: nil,
		state: persistedState{
			Items:  nil,
			Alerts: nil,
			Settings: AppSettings{
				PriceMode:              "live",
				RefreshIntervalSeconds: 20,
				QuoteSource:            defaultQuoteSourceID,
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
