package monitor

// effectiveDCAEntryPrice returns the normalized buy price used across item and dialog summaries.
func effectiveDCAEntryPrice(entry DCAEntry) float64 {
	if entry.Price > 0 && entry.Shares > 0 {
		return entry.Price
	}
	if entry.Shares <= 0 {
		return 0
	}
	netAmount := entry.Amount - entry.Fee
	if netAmount < 0 {
		netAmount = 0
	}
	return netAmount / entry.Shares
}

// decorateDCAEntries populates the EffectivePrice field for each entry in the slice.
func decorateDCAEntries(entries []DCAEntry) []DCAEntry {
	if len(entries) == 0 {
		return nil
	}

	result := append([]DCAEntry(nil), entries...)
	for index := range result {
		result[index].EffectivePrice = effectiveDCAEntryPrice(result[index])
	}
	return result
}

// buildDCASummary aggregates all valid DCA entries on an item into a DCASummary.
// Returns nil when no valid entries (amount > 0 and shares > 0) are present.
func buildDCASummary(item WatchlistItem) *DCASummary {
	validEntries := make([]DCAEntry, 0, len(item.DCAEntries))
	for _, entry := range item.DCAEntries {
		if entry.Amount > 0 && entry.Shares > 0 {
			validEntries = append(validEntries, entry)
		}
	}
	if len(validEntries) == 0 {
		return nil
	}

	summary := &DCASummary{
		Count: len(validEntries),
	}
	var totalEffectiveCost float64
	for _, entry := range validEntries {
		summary.TotalAmount += entry.Amount
		summary.TotalShares += entry.Shares
		summary.TotalFees += entry.Fee
		totalEffectiveCost += effectiveDCAEntryPrice(entry) * entry.Shares
	}
	if summary.TotalShares > 0 {
		summary.AverageCost = totalEffectiveCost / summary.TotalShares
	}
	if item.CurrentPrice > 0 {
		summary.HasCurrentPrice = true
		summary.CurrentValue = summary.TotalShares * item.CurrentPrice
		summary.PnL = summary.CurrentValue - totalEffectiveCost
		if totalEffectiveCost > 0 {
			summary.PnLPct = summary.PnL / totalEffectiveCost * 100
		}
	}

	return summary
}

// buildPositionSummary computes derived position metrics for a tracked item.
func buildPositionSummary(item WatchlistItem) *PositionSummary {
	summary := &PositionSummary{
		CostBasis:        item.CostBasis(),
		MarketValue:      item.MarketValue(),
		UnrealisedPnL:    item.UnrealisedPnL(),
		UnrealisedPnLPct: item.UnrealisedPnLPct(),
		HasPosition:      item.Quantity > 0,
	}
	return summary
}

// decorateItemDerived attaches all server-computed derived fields to an item before it is sent to the frontend.
func decorateItemDerived(item WatchlistItem) WatchlistItem {
	item.DCAEntries = decorateDCAEntries(item.DCAEntries)
	item.DCASummary = buildDCASummary(item)
	item.Position = buildPositionSummary(item)
	return item
}

// buildMarketSnapshot computes the market and position metrics overlay for a chart series response.
func buildMarketSnapshot(item WatchlistItem, series HistorySeries) *MarketSnapshot {
	livePrice := item.CurrentPrice
	if livePrice <= 0 {
		livePrice = series.EndPrice
	}

	hasLiveAndSeries := item.CurrentPrice > 0 && series.StartPrice > 0
	effectiveChange := series.Change
	effectiveChangePct := series.ChangePercent
	if hasLiveAndSeries {
		effectiveChange = item.CurrentPrice - series.StartPrice
		effectiveChangePct = 0
		if series.StartPrice > 0 {
			effectiveChangePct = effectiveChange / series.StartPrice * 100
		}
	} else if effectiveChange == 0 {
		effectiveChange = item.Change
		effectiveChangePct = item.ChangePercent
	}

	previousClose := item.PreviousClose
	if previousClose <= 0 {
		previousClose = series.StartPrice
	}
	openPrice := item.OpenPrice
	if openPrice <= 0 && len(series.Points) > 0 {
		openPrice = series.Points[0].Open
	}
	rangeHigh := series.High
	if rangeHigh <= 0 {
		rangeHigh = item.DayHigh
	}
	rangeLow := series.Low
	if rangeLow <= 0 {
		rangeLow = item.DayLow
	}

	amplitudePct := 0.0
	if previousClose > 0 && rangeHigh > 0 && rangeLow > 0 {
		amplitudePct = (rangeHigh - rangeLow) / previousClose * 100
	}

	positionValue := 0.0
	positionBaseline := 0.0
	positionPnL := 0.0
	positionPnLPct := 0.0
	if item.Quantity > 0 {
		positionValue = item.Quantity * livePrice
		positionBaseline = item.Quantity * item.CostPrice
		positionPnL = positionValue - positionBaseline
		if positionBaseline > 0 {
			positionPnLPct = positionPnL / positionBaseline * 100
		}
	}

	return &MarketSnapshot{
		LivePrice:          livePrice,
		EffectiveChange:    effectiveChange,
		EffectiveChangePct: effectiveChangePct,
		PreviousClose:      previousClose,
		OpenPrice:          openPrice,
		RangeHigh:          rangeHigh,
		RangeLow:           rangeLow,
		AmplitudePct:       amplitudePct,
		PositionValue:      positionValue,
		PositionBaseline:   positionBaseline,
		PositionPnL:        positionPnL,
		PositionPnLPct:     positionPnLPct,
	}
}
