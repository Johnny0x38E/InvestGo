package marketdata

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"investgo/internal/monitor"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

// collectQuoteTargets converts input items into standard targets and collects any resolution failures.
func collectQuoteTargets(items []monitor.WatchlistItem) (map[string]monitor.QuoteTarget, []string) {
	targets := make(map[string]monitor.QuoteTarget, len(items))
	var problems []string

	for _, item := range items {
		target, err := monitor.ResolveQuoteTarget(item)
		if err != nil {
			problems = append(problems, err.Error())
			continue
		}
		targets[target.Key] = target
	}

	return targets, problems
}

// buildQuote constructs a unified Quote object from the key price fields.
func buildQuote(name string, current, previous, open, high, low float64, updatedAt time.Time, source string) monitor.Quote {
	change := 0.0
	changePercent := 0.0
	if previous > 0 {
		change = current - previous
		changePercent = change / previous * 100
	}

	return monitor.Quote{
		Name:          strings.TrimSpace(name),
		CurrentPrice:  current,
		PreviousClose: previous,
		OpenPrice:     open,
		DayHigh:       high,
		DayLow:        low,
		Change:        change,
		ChangePercent: changePercent,
		Source:        source,
		UpdatedAt:     updatedAt,
	}
}

// parseFloat safely parses numeric fields from API responses.
func parseFloat(raw string) float64 {
	clean := strings.TrimSpace(strings.NewReplacer("\"", "", ";", "", ",", "").Replace(raw))
	if clean == "" || clean == "-" {
		return 0
	}

	value, err := strconv.ParseFloat(clean, 64)
	if err != nil {
		return 0
	}
	return value
}

func firstNonEmptyFloat(left, right float64) float64 {
	if left > 0 {
		return left
	}
	return right
}

// firstNonEmpty returns the first non-empty string.
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

// partsAt safely accesses an element in a string slice, returning an empty string on out-of-bounds access.
func partsAt(parts []string, index int) string {
	if index < 0 || index >= len(parts) {
		return ""
	}
	return parts[index]
}

// parseTimestamp safely parses time fields from API responses, supporting several common formats.
func parseTimestamp(raw string) time.Time {
	candidate := strings.TrimSpace(strings.NewReplacer("/", "-", "\"", "", ";", "").Replace(raw))
	if candidate == "" {
		return time.Time{}
	}

	layouts := []string{
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"20060102150405",
	}

	for _, layout := range layouts {
		if parsed, err := time.ParseInLocation(layout, candidate, time.Local); err == nil {
			return parsed
		}
	}

	return time.Time{}
}

// decodeGB18030Body converts a GB18030-encoded response body into a UTF-8 string.
func decodeGB18030Body(body io.Reader) (string, error) {
	reader := transform.NewReader(body, simplifiedchinese.GB18030.NewDecoder())
	payload, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(bytes.TrimPrefix(payload, []byte{0xef, 0xbb, 0xbf}))), nil
}

// fetchTextWithHeaders makes a GET request with custom headers and returns the response text.
// Optionally decodes GB18030-encoded responses.
func fetchTextWithHeaders(ctx context.Context, client *http.Client, requestURL string, headers map[string]string, decodeGB18030 bool) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return "", err
	}

	for key, value := range headers {
		request.Header.Set(key, value)
	}

	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return "", errors.New("unexpected status " + strconv.Itoa(response.StatusCode))
	}

	if decodeGB18030 {
		return decodeGB18030Body(response.Body)
	}

	payload, err := io.ReadAll(response.Body)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(payload)), nil
}

// isLetters checks whether a string consists entirely of English letters.
func isLetters(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') {
			return false
		}
	}
	return true
}

type emFloat float64

// UnmarshalJSON handles EastMoney numeric fields returning "-" when missing.
func (f *emFloat) UnmarshalJSON(data []byte) error {
	var value float64
	if err := json.Unmarshal(data, &value); err == nil {
		*f = emFloat(value)
		return nil
	}
	*f = 0
	return nil
}

// setEastMoneyHeaders sets comprehensive browser-like request headers required by EastMoney APIs.
// Without these headers, EastMoney servers may close the connection immediately (EOF).
func setEastMoneyHeaders(req *http.Request, referer string) {
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Cache-Control", "no-cache")
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
}

// ── History helpers ─────────────────────────────────────────────────────────

// applyHistorySummary calculates the gain/loss summary and period high/low from the history point series.
func applyHistorySummary(series *monitor.HistorySeries) {
	if len(series.Points) == 0 {
		return
	}

	series.StartPrice = series.Points[0].Close
	series.EndPrice = series.Points[len(series.Points)-1].Close
	series.High = series.Points[0].High
	series.Low = series.Points[0].Low

	for _, point := range series.Points {
		if point.High > series.High {
			series.High = point.High
		}
		if point.Low < series.Low {
			series.Low = point.Low
		}
	}

	series.Change = series.EndPrice - series.StartPrice
	if series.StartPrice > 0 {
		series.ChangePercent = series.Change / series.StartPrice * 100
	}
}

// minInt returns the smaller of two integers.
func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

// chunkStrings splits a string slice into chunks of at most the given size.
func chunkStrings(items []string, size int) [][]string {
	if len(items) == 0 {
		return nil
	}
	if size <= 0 {
		size = 1
	}
	chunks := make([][]string, 0, (len(items)+size-1)/size)
	for i := 0; i < len(items); i += size {
		end := i + size
		if end > len(items) {
			end = len(items)
		}
		chunks = append(chunks, items[i:end])
	}
	return chunks
}

// trimHistoryPoints trims history points to the given window, preserving original chronological order.
func trimHistoryPoints(points []monitor.HistoryPoint, window time.Duration) []monitor.HistoryPoint {
	if window <= 0 || len(points) == 0 {
		return points
	}

	latest := points[len(points)-1].Timestamp
	if latest.IsZero() {
		return append([]monitor.HistoryPoint(nil), points...)
	}

	cutoff := latest.Add(-window)
	start := 0
	for start < len(points) && points[start].Timestamp.Before(cutoff) {
		start++
	}
	if start >= len(points) {
		return nil
	}
	return append([]monitor.HistoryPoint(nil), points[start:]...)
}

// historyTrimWindow returns the trim duration for the given history interval.
func historyTrimWindow(interval monitor.HistoryInterval) time.Duration {
	switch interval {
	case monitor.HistoryRange1h:
		return time.Hour
	case monitor.HistoryRange1d:
		return 24 * time.Hour
	case monitor.HistoryRange1w:
		return 7 * 24 * time.Hour
	case monitor.HistoryRange1mo:
		return 30 * 24 * time.Hour
	case monitor.HistoryRange1y:
		return 365 * 24 * time.Hour
	case monitor.HistoryRange3y:
		return 3 * 365 * 24 * time.Hour
	default:
		return 0
	}
}

// parseUSAPITimestamp parses timestamp strings commonly returned by US market data APIs.
func parseUSAPITimestamp(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02"} {
		if parsed, err := time.ParseInLocation(layout, raw, time.Local); err == nil {
			return parsed
		}
	}
	return time.Time{}
}
