package monitor

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

type overviewHistoryLoader func(context.Context, WatchlistItem, HistoryInterval) (HistorySeries, error)

type overviewCalculator struct {
	fx              *FxRates
	displayCurrency string
	loadHistory     overviewHistoryLoader
}

type overviewTrendSeed struct {
	item         WatchlistItem
	firstBuyDate time.Time
	history      HistorySeries
	hasPosition  bool
}

func newOverviewCalculator(fx *FxRates, displayCurrency string, loadHistory overviewHistoryLoader) overviewCalculator {
	if strings.TrimSpace(displayCurrency) == "" {
		displayCurrency = "CNY"
	}
	return overviewCalculator{
		fx:              fx,
		displayCurrency: strings.ToUpper(strings.TrimSpace(displayCurrency)),
		loadHistory:     loadHistory,
	}
}

func (c overviewCalculator) Build(ctx context.Context, items []WatchlistItem) (OverviewAnalytics, error) {
	breakdown := c.buildBreakdown(items)
	trend, err := c.buildTrend(ctx, items)
	if err != nil {
		return OverviewAnalytics{}, err
	}
	return OverviewAnalytics{
		DisplayCurrency: c.displayCurrency,
		Breakdown:       breakdown,
		Trend:           trend,
		GeneratedAt:     time.Now(),
	}, nil
}

func (c overviewCalculator) buildBreakdown(items []WatchlistItem) []OverviewHoldingSlice {
	slices := make([]OverviewHoldingSlice, 0, len(items))
	var total float64

	for _, item := range items {
		value := c.convertValue(item.MarketValue(), item.Currency)
		if value <= 0 {
			continue
		}
		slices = append(slices, OverviewHoldingSlice{
			ItemID:   item.ID,
			Symbol:   item.Symbol,
			Name:     item.Name,
			Market:   item.Market,
			Currency: c.displayCurrency,
			Value:    value,
		})
		total += value
	}

	sort.Slice(slices, func(i, j int) bool {
		if slices[i].Value != slices[j].Value {
			return slices[i].Value > slices[j].Value
		}
		return slices[i].Symbol < slices[j].Symbol
	})
	for index := range slices {
		if total > 0 {
			slices[index].Weight = slices[index].Value / total
		}
	}
	return slices
}

func (c overviewCalculator) buildTrend(ctx context.Context, items []WatchlistItem) (OverviewTrend, error) {
	seeds := make([]overviewTrendSeed, 0, len(items))
	var problems []string
	var overallStart time.Time
	var overallEnd time.Time

	for _, item := range items {
		entries := validOverviewDCAEntries(item.DCAEntries)

		var firstBuy time.Time
		var hasPosition bool

		if len(entries) > 0 {
			firstBuy = entries[0].Date
			for _, entry := range entries[1:] {
				if entry.Date.Before(firstBuy) {
					firstBuy = entry.Date
				}
			}
		} else if item.Quantity > 0 {
			hasPosition = true
			if item.AcquiredAt != nil {
				firstBuy = *item.AcquiredAt
			}
			// If AcquiredAt is nil, firstBuy stays zero — overviewHistoryIntervalFor will
			// return HistoryRangeAll, and we anchor to the oldest history point below.
		} else {
			continue
		}

		historyInterval := overviewHistoryIntervalFor(firstBuy)
		history, err := c.loadHistory(ctx, item, historyInterval)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", item.Symbol, err))
			continue
		}
		if len(history.Points) == 0 {
			continue
		}

		// When no AcquiredAt was provided, anchor firstBuy to the oldest available history point
		// so that the item still participates in the trend with its full available history.
		if firstBuy.IsZero() {
			for _, p := range history.Points {
				if firstBuy.IsZero() || p.Timestamp.Before(firstBuy) {
					firstBuy = p.Timestamp
				}
			}
		}

		seeds = append(seeds, overviewTrendSeed{
			item:         item,
			firstBuyDate: normalizeTrendDay(firstBuy),
			history:      history,
			hasPosition:  hasPosition,
		})

		if overallStart.IsZero() || firstBuy.Before(overallStart) {
			overallStart = firstBuy
		}
		lastPointDay := normalizeTrendDay(history.Points[len(history.Points)-1].Timestamp)
		if overallEnd.IsZero() || lastPointDay.After(overallEnd) {
			overallEnd = lastPointDay
		}
	}

	if len(seeds) == 0 || overallStart.IsZero() || overallEnd.IsZero() {
		if len(problems) > 0 {
			return OverviewTrend{}, joinProblems(problems)
		}
		return OverviewTrend{}, nil
	}

	dates := collectTrendDates(normalizeTrendDay(overallStart), seeds)
	if len(dates) == 0 {
		return OverviewTrend{}, nil
	}
	series := make([]OverviewTrendSeries, 0, len(seeds))
	totalByDay := make([]float64, len(dates))

	for _, seed := range seeds {
		values := c.buildTrendValues(seed.item, dates, seed.history, seed.hasPosition)
		latestValue := values[len(values)-1]
		for index, value := range values {
			totalByDay[index] += value
		}
		series = append(series, OverviewTrendSeries{
			ItemID:       seed.item.ID,
			Symbol:       seed.item.Symbol,
			Name:         seed.item.Name,
			Market:       seed.item.Market,
			Currency:     c.displayCurrency,
			LatestValue:  latestValue,
			FirstBuyDate: seed.firstBuyDate,
			Values:       values,
		})
	}

	sort.Slice(series, func(i, j int) bool {
		if series[i].LatestValue != series[j].LatestValue {
			return series[i].LatestValue > series[j].LatestValue
		}
		return series[i].Symbol < series[j].Symbol
	})

	totalValue := totalByDay[len(totalByDay)-1]
	startDate := dates[0]
	endDate := dates[len(dates)-1]
	return OverviewTrend{
		StartDate:  &startDate,
		EndDate:    &endDate,
		Dates:      dates,
		Series:     series,
		TotalValue: totalValue,
	}, nil
}

func (c overviewCalculator) buildTrendValues(item WatchlistItem, dates []time.Time, history HistorySeries, hasPosition bool) []float64 {
	historyPoints := append([]HistoryPoint(nil), history.Points...)
	sort.Slice(historyPoints, func(i, j int) bool {
		return historyPoints[i].Timestamp.Before(historyPoints[j].Timestamp)
	})

	values := make([]float64, len(dates))

	if hasPosition {
		// Non-DCA holding: quantity is constant across the entire period.
		entryIndex := 0
		var lastClose float64
		for index, day := range dates {
			for entryIndex < len(historyPoints) && !normalizeTrendDay(historyPoints[entryIndex].Timestamp).After(day) {
				lastClose = historyPoints[entryIndex].Close
				entryIndex++
			}
			if lastClose <= 0 {
				continue
			}
			values[index] = c.convertValue(item.Quantity*lastClose, item.Currency)
		}
		return values
	}

	entries := validOverviewDCAEntries(item.DCAEntries)
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Date.Before(entries[j].Date)
	})
	entryIndex := 0
	historyIndex := 0
	var heldShares float64
	var lastClose float64

	for index, day := range dates {
		for entryIndex < len(entries) && !normalizeTrendDay(entries[entryIndex].Date).After(day) {
			heldShares += entries[entryIndex].Shares
			entryIndex++
		}
		for historyIndex < len(historyPoints) && !normalizeTrendDay(historyPoints[historyIndex].Timestamp).After(day) {
			lastClose = historyPoints[historyIndex].Close
			historyIndex++
		}
		if heldShares <= 0 || lastClose <= 0 {
			continue
		}
		values[index] = c.convertValue(heldShares*lastClose, item.Currency)
	}

	return values
}

func (c overviewCalculator) convertValue(value float64, fromCurrency string) float64 {
	fromCurrency = strings.ToUpper(strings.TrimSpace(fromCurrency))
	if c.fx == nil || fromCurrency == "" || fromCurrency == c.displayCurrency {
		return value
	}
	return c.fx.Convert(value, fromCurrency, c.displayCurrency)
}

func validOverviewDCAEntries(entries []DCAEntry) []DCAEntry {
	valid := make([]DCAEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.Amount > 0 && entry.Shares > 0 {
			valid = append(valid, entry)
		}
	}
	return valid
}

func normalizeTrendDay(value time.Time) time.Time {
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.UTC)
}

func overviewHistoryIntervalFor(firstBuy time.Time) HistoryInterval {
	age := time.Since(firstBuy)
	switch {
	case age <= 370*24*time.Hour:
		return HistoryRange1y
	case age <= (3*370)*24*time.Hour:
		return HistoryRange3y
	default:
		return HistoryRangeAll
	}
}

func enumerateTrendDays(start, end time.Time) []time.Time {
	if end.Before(start) {
		return nil
	}
	dates := make([]time.Time, 0, int(end.Sub(start).Hours()/24)+1)
	for day := start; !day.After(end); day = day.AddDate(0, 0, 1) {
		dates = append(dates, day)
	}
	return dates
}

func collectTrendDates(start time.Time, seeds []overviewTrendSeed) []time.Time {
	set := make(map[time.Time]struct{})
	for _, seed := range seeds {
		set[seed.firstBuyDate] = struct{}{}
		for _, point := range seed.history.Points {
			day := normalizeTrendDay(point.Timestamp)
			if day.Before(start) {
				continue
			}
			set[day] = struct{}{}
		}
	}

	if len(set) == 0 {
		return nil
	}

	dates := make([]time.Time, 0, len(set))
	for day := range set {
		dates = append(dates, day)
	}
	sort.Slice(dates, func(i, j int) bool {
		return dates[i].Before(dates[j])
	})
	return dates
}
