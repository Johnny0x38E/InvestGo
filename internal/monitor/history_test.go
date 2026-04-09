package monitor

import "testing"

func TestResolveYahooSymbol(t *testing.T) {
	testCases := []struct {
		name    string
		item    WatchlistItem
		want    string
		wantErr bool
	}{
		{
			name: "A share sh",
			item: WatchlistItem{Symbol: "600519.SH"},
			want: "600519.SS",
		},
		{
			name: "A share sz",
			item: WatchlistItem{Symbol: "000001.SZ"},
			want: "000001.SZ",
		},
		{
			name: "Hong Kong",
			item: WatchlistItem{Symbol: "00700.HK"},
			want: "0700.HK",
		},
		{
			name: "US",
			item: WatchlistItem{Symbol: "AAPL"},
			want: "AAPL",
		},
		{
			name:    "Beijing unsupported",
			item:    WatchlistItem{Symbol: "430047.BJ"},
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveYahooSymbol(tc.item)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveYahooSymbol returned error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("resolveYahooSymbol() = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestApplyHistorySummary(t *testing.T) {
	series := &HistorySeries{
		Points: []HistoryPoint{
			{Close: 10, High: 11, Low: 9},
			{Close: 12, High: 13, Low: 10},
			{Close: 11, High: 12, Low: 10.5},
		},
	}

	applyHistorySummary(series)

	if series.StartPrice != 10 || series.EndPrice != 11 {
		t.Fatalf("unexpected start/end price: %+v", series)
	}
	if series.High != 13 || series.Low != 9 {
		t.Fatalf("unexpected high/low: %+v", series)
	}
	if series.Change != 1 || series.ChangePercent != 10 {
		t.Fatalf("unexpected change summary: %+v", series)
	}
}
