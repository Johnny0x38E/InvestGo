package monitor

import (
	"errors"
	"strings"
)

// normalizeTags removes empty tags and deduplicates the set while preserving order.
func normalizeTags(tags []string) []string {
	seen := make(map[string]struct{}, len(tags))
	result := make([]string, 0, len(tags))
	for _, tag := range tags {
		clean := strings.TrimSpace(tag)
		if clean == "" {
			continue
		}
		if _, exists := seen[clean]; exists {
			continue
		}
		seen[clean] = struct{}{}
		result = append(result, clean)
	}
	return result
}

// sanitiseAlertRule normalizes an alert rule and validates its fields.
func sanitiseAlertRule(input AlertRule) (AlertRule, error) {
	alert := input
	alert.Name = strings.TrimSpace(alert.Name)
	alert.ItemID = strings.TrimSpace(alert.ItemID)

	if alert.Name == "" {
		return AlertRule{}, errors.New("Alert name is required")
	}
	if alert.ItemID == "" {
		return AlertRule{}, errors.New("Alert must reference an item")
	}
	if alert.Condition != AlertAbove && alert.Condition != AlertBelow {
		return AlertRule{}, errors.New("Alert condition is invalid")
	}
	if alert.Threshold <= 0 {
		return AlertRule{}, errors.New("Alert threshold must be greater than 0")
	}
	return alert, nil
}

// normalizeDCAEntries filters out invalid entries, ensures every entry has an ID,
// and returns the cleaned slice together with total shares and weighted-average cost.
func normalizeDCAEntries(entries []DCAEntry, newID func(string) string) ([]DCAEntry, float64, float64) {
	if len(entries) == 0 {
		return nil, 0, 0
	}

	valid := make([]DCAEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.Shares <= 0 || entry.Amount <= 0 {
			continue
		}
		if entry.ID == "" {
			entry.ID = newID("dca")
		}
		valid = append(valid, entry)
	}

	totalShares := 0.0
	totalEffectiveCost := 0.0
	for _, entry := range valid {
		totalShares += entry.Shares
		effectiveCost := entry.Price * entry.Shares
		if entry.Price <= 0 {
			effectiveCost = entry.Amount - entry.Fee
			if effectiveCost < 0 {
				effectiveCost = 0
			}
		}
		totalEffectiveCost += effectiveCost
	}

	averageCost := 0.0
	if totalShares > 0 {
		averageCost = totalEffectiveCost / totalShares
	}
	return valid, totalShares, averageCost
}
