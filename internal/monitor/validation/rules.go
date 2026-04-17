package validation

import (
	"errors"
	"strings"

	"investgo/internal/monitor/domain"
)

// NormalizeTags removes empty tags and keeps the tag set unique.
func NormalizeTags(tags []string) []string {
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

// SanitizeAlert normalizes alert rules and performs basic validation.
func SanitizeAlert(input domain.AlertRule) (domain.AlertRule, error) {
	alert := input
	alert.Name = strings.TrimSpace(alert.Name)
	alert.ItemID = strings.TrimSpace(alert.ItemID)

	if alert.Name == "" {
		return domain.AlertRule{}, errors.New("Alert name is required")
	}
	if alert.ItemID == "" {
		return domain.AlertRule{}, errors.New("Alert must reference an item")
	}
	if alert.Condition != domain.AlertAbove && alert.Condition != domain.AlertBelow {
		return domain.AlertRule{}, errors.New("Alert condition is invalid")
	}
	if alert.Threshold <= 0 {
		return domain.AlertRule{}, errors.New("Alert threshold must be greater than 0")
	}

	return alert, nil
}

// NormalizeDCAEntries filters invalid entries, ensures IDs exist, and derives quantity plus weighted average cost.
func NormalizeDCAEntries(entries []domain.DCAEntry, newID func(string) string) ([]domain.DCAEntry, float64, float64) {
	if len(entries) == 0 {
		return nil, 0, 0
	}

	valid := make([]domain.DCAEntry, 0, len(entries))
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
