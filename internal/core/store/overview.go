package store

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"investgo/internal/common/errs"
	"investgo/internal/core"
	"investgo/internal/core/fx"
)

type overviewHistoryLoader func(context.Context, core.WatchlistItem, core.HistoryInterval) (core.HistorySeries, error)

type overviewCalculator struct {
	fx              *fx.FxRates
	displayCurrency string
	loadHistory     overviewHistoryLoader
}

type overviewTrendSeed struct {
	item         core.WatchlistItem
	firstBuyDate time.Time
	history      core.HistorySeries
	hasPosition  bool
}

type overviewTrendCandidate struct {
	item        core.WatchlistItem
	firstBuy    time.Time
	hasPosition bool
	interval    core.HistoryInterval
}

const overviewHistoryConcurrency = 4

func newOverviewCalculator(fx *fx.FxRates, displayCurrency string, loadHistory overviewHistoryLoader) overviewCalculator {
	if strings.TrimSpace(displayCurrency) == "" {
		displayCurrency = "CNY"
	}
	return overviewCalculator{
		fx:              fx,
		displayCurrency: strings.ToUpper(strings.TrimSpace(displayCurrency)),
		loadHistory:     loadHistory,
	}
}

func (c overviewCalculator) Build(ctx context.Context, items []core.WatchlistItem) (core.OverviewAnalytics, error) {
	// Breakdown and trend intentionally share the same normalized display
	// currency so the overview surface never mixes converted and raw values.
	breakdown := c.buildBreakdown(items)
	trend, err := c.buildTrend(ctx, items)
	if err != nil {
		return core.OverviewAnalytics{}, err
	}
	return core.OverviewAnalytics{
		DisplayCurrency: c.displayCurrency,
		Breakdown:       breakdown,
		Trend:           trend,
		GeneratedAt:     time.Now(),
	}, nil
}

func (c overviewCalculator) buildBreakdown(items []core.WatchlistItem) []core.OverviewHoldingSlice {
	slices := make([]core.OverviewHoldingSlice, 0, len(items))
	var total float64

	for _, item := range items {
		// Zero-value holdings do not add signal in the overview donut and would
		// only make the legend noisier.
		value := c.convertValue(item.MarketValue(), item.Currency)
		if value <= 0 {
			continue
		}
		slices = append(slices, core.OverviewHoldingSlice{
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

func (c overviewCalculator) buildTrend(ctx context.Context, items []core.WatchlistItem) (core.OverviewTrend, error) {
	candidates := make([]overviewTrendCandidate, 0, len(items))
	var problems []string

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

		candidates = append(candidates, overviewTrendCandidate{
			item:        item,
			firstBuy:    firstBuy,
			hasPosition: hasPosition,
			interval:    overviewHistoryIntervalFor(firstBuy),
		})
	}

	seeds, loadProblems := c.loadTrendSeeds(ctx, candidates)
	problems = append(problems, loadProblems...)

	var overallStart time.Time
	var overallEnd time.Time
	for _, seed := range seeds {
		if overallStart.IsZero() || seed.firstBuyDate.Before(overallStart) {
			overallStart = seed.firstBuyDate
		}
		lastPointDay := normalizeTrendDay(seed.history.Points[len(seed.history.Points)-1].Timestamp)
		if overallEnd.IsZero() || lastPointDay.After(overallEnd) {
			overallEnd = lastPointDay
		}
	}

	if len(seeds) == 0 || overallStart.IsZero() || overallEnd.IsZero() {
		if len(problems) > 0 {
			return core.OverviewTrend{}, errs.JoinProblems(problems)
		}
		return core.OverviewTrend{}, nil
	}

	dates := collectTrendDates(normalizeTrendDay(overallStart), seeds)
	if len(dates) == 0 {
		return core.OverviewTrend{}, nil
	}
	series := make([]core.OverviewTrendSeries, 0, len(seeds))
	totalByDay := make([]float64, len(dates))

	for _, seed := range seeds {
		values := c.buildTrendValues(seed.item, dates, seed.history, seed.hasPosition)
		latestValue := values[len(values)-1]
		for index, value := range values {
			totalByDay[index] += value
		}
		series = append(series, core.OverviewTrendSeries{
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
	return core.OverviewTrend{
		StartDate:  &startDate,
		EndDate:    &endDate,
		Dates:      dates,
		Series:     series,
		TotalValue: totalValue,
	}, nil
}

func (c overviewCalculator) loadTrendSeeds(ctx context.Context, candidates []overviewTrendCandidate) ([]overviewTrendSeed, []string) {
	if len(candidates) == 0 {
		return nil, nil
	}

	type result struct {
		seed    overviewTrendSeed
		problem string
	}

	sem := make(chan struct{}, overviewHistoryConcurrency)
	results := make(chan result, len(candidates))
	var wg sync.WaitGroup

	for _, candidate := range candidates {
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()

			history, err := c.loadHistory(ctx, candidate.item, candidate.interval)
			if err != nil {
				results <- result{problem: fmt.Sprintf("%s: %v", candidate.item.Symbol, err)}
				return
			}
			if len(history.Points) == 0 {
				return
			}

			firstBuy := candidate.firstBuy
			if firstBuy.IsZero() {
				for _, p := range history.Points {
					if firstBuy.IsZero() || p.Timestamp.Before(firstBuy) {
						firstBuy = p.Timestamp
					}
				}
			}

			results <- result{
				seed: overviewTrendSeed{
					item:         candidate.item,
					firstBuyDate: normalizeTrendDay(firstBuy),
					history:      history,
					hasPosition:  candidate.hasPosition,
				},
			}
		})
	}

	wg.Wait()
	close(results)

	seeds := make([]overviewTrendSeed, 0, len(candidates))
	problems := make([]string, 0)
	for result := range results {
		if result.problem != "" {
			problems = append(problems, result.problem)
			continue
		}
		if len(result.seed.history.Points) == 0 {
			continue
		}
		seeds = append(seeds, result.seed)
	}

	return seeds, problems
}

func (c overviewCalculator) buildTrendValues(item core.WatchlistItem, dates []time.Time, history core.HistorySeries, hasPosition bool) []float64 {
	historyPoints := append([]core.HistoryPoint(nil), history.Points...)
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

	// Replay DCA entries across the normalized day axis so the overview trend
	// reflects accumulated share count instead of a single synthetic entry.
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

func validOverviewDCAEntries(entries []core.DCAEntry) []core.DCAEntry {
	valid := make([]core.DCAEntry, 0, len(entries))
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

func overviewHistoryIntervalFor(firstBuy time.Time) core.HistoryInterval {
	age := time.Since(firstBuy)
	switch {
	case age <= 370*24*time.Hour:
		return core.HistoryRange1y
	case age <= (3*370)*24*time.Hour:
		return core.HistoryRange3y
	default:
		return core.HistoryRangeAll
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
