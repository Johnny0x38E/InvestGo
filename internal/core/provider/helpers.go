package provider

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

	"investgo/internal/core"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

// CollectQuoteTargets converts input items into standard targets and collects any resolution failures.
func CollectQuoteTargets(items []core.WatchlistItem) (map[string]core.QuoteTarget, []string) {
	targets := make(map[string]core.QuoteTarget, len(items))
	var problems []string

	for _, item := range items {
		target, err := core.ResolveQuoteTarget(item)
		if err != nil {
			problems = append(problems, err.Error())
			continue
		}
		targets[target.Key] = target
	}

	return targets, problems
}

// BuildQuote constructs a unified Quote object from the key price fields.
func BuildQuote(name string, current, previous, open, high, low float64, updatedAt time.Time, source string) core.Quote {
	change := 0.0
	changePercent := 0.0
	if previous > 0 {
		change = current - previous
		changePercent = change / previous * 100
	}

	return core.Quote{
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

// ParseFloat safely parses numeric fields from API responses.
func ParseFloat(raw string) float64 {
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

func FirstNonEmptyFloat(left, right float64) float64 {
	if left > 0 {
		return left
	}
	return right
}

// FirstNonEmpty returns the first non-empty string.
func FirstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

// PartsAt safely accesses an element in a string slice, returning an empty string on out-of-bounds access.
func PartsAt(parts []string, index int) string {
	if index < 0 || index >= len(parts) {
		return ""
	}
	return parts[index]
}

// ParseTimestamp safely parses time fields from API responses, supporting several common formats.
func ParseTimestamp(raw string) time.Time {
	candidate := strings.TrimSpace(strings.NewReplacer("/", "-", "\"", "", ";", "").Replace(raw))
	if candidate == "" {
		return time.Time{}
	}

	layouts := []string{
		time.DateTime,
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

// FetchTextWithHeaders makes a GET request with custom headers and returns the response text.
// Optionally decodes GB18030-encoded responses.
func FetchTextWithHeaders(ctx context.Context, client *http.Client, requestURL string, headers map[string]string, decodeGB18030 bool) (string, error) {
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
		return DecodeGB18030Body(response.Body)
	}

	payload, err := io.ReadAll(response.Body)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(payload)), nil
}

// DecodeGB18030Body converts a GB18030-encoded response body into a UTF-8 string.
func DecodeGB18030Body(body io.Reader) (string, error) {
	reader := transform.NewReader(body, simplifiedchinese.GB18030.NewDecoder())
	payload, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(bytes.TrimPrefix(payload, []byte{0xef, 0xbb, 0xbf}))), nil
}

// IsLetters checks whether a string consists entirely of English letters.
func IsLetters(value string) bool {
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

type EmFloat float64

// UnmarshalJSON handles EastMoney numeric fields returning "-" when missing.
func (f *EmFloat) UnmarshalJSON(data []byte) error {
	var value float64
	if err := json.Unmarshal(data, &value); err == nil {
		*f = EmFloat(value)
		return nil
	}
	*f = 0
	return nil
}

// SetEastMoneyHeaders sets comprehensive browser-like request headers required by EastMoney APIs.
// Without these headers, EastMoney servers may close the connection immediately (EOF).
func SetEastMoneyHeaders(req *http.Request, referer string) {
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

// ApplyHistorySummary calculates the gain/loss summary and period high/low from the history point series.
func ApplyHistorySummary(series *core.HistorySeries) {
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

// MinInt returns the smaller of two integers.
func MinInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

// ChunkStrings splits a string slice into chunks of at most the given size.
func ChunkStrings(items []string, size int) [][]string {
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
			end = min(i+size, len(items))
		}
		chunks = append(chunks, items[i:end])
	}
	return chunks
}

// TrimHistoryPoints trims history points to the given window, preserving original chronological order.
func TrimHistoryPoints(points []core.HistoryPoint, window time.Duration) []core.HistoryPoint {
	if window <= 0 || len(points) == 0 {
		return points
	}

	latest := points[len(points)-1].Timestamp
	if latest.IsZero() {
		return append([]core.HistoryPoint(nil), points...)
	}

	cutoff := latest.Add(-window)
	start := 0
	for start < len(points) && points[start].Timestamp.Before(cutoff) {
		start++
	}
	if start >= len(points) {
		return nil
	}
	return append([]core.HistoryPoint(nil), points[start:]...)
}

// HistoryTrimWindow returns the trim duration for the given history interval.
func HistoryTrimWindow(interval core.HistoryInterval) time.Duration {
	switch interval {
	case core.HistoryRange1h:
		return time.Hour
	case core.HistoryRange1d:
		return 24 * time.Hour
	case core.HistoryRange1w:
		return 7 * 24 * time.Hour
	case core.HistoryRange1mo:
		return 30 * 24 * time.Hour
	case core.HistoryRange1y:
		return 365 * 24 * time.Hour
	case core.HistoryRange3y:
		return 3 * 365 * 24 * time.Hour
	default:
		return 0
	}
}

// ParseUSAPITimestamp parses timestamp strings commonly returned by US market data APIs.
func ParseUSAPITimestamp(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.DateTime, time.DateOnly} {
		if parsed, err := time.ParseInLocation(layout, raw, time.Local); err == nil {
			return parsed
		}
	}
	return time.Time{}
}

// ChunkSecIDs splits a slice of EastMoney security IDs into chunks that respect
// both a count limit and a maximum encoded query-string length.
func ChunkSecIDs(secids []string, batchSize int, maxChars int) [][]string {
	if len(secids) == 0 {
		return nil
	}
	if batchSize <= 0 {
		batchSize = 1
	}

	chunks := make([][]string, 0, (len(secids)+batchSize-1)/batchSize)
	current := make([]string, 0, min(batchSize, len(secids)))
	currentLen := 0
	for _, secid := range secids {
		nextLen := currentLen + len(secid)
		if len(current) > 0 {
			nextLen += 3 // commas become %2C in url.Values.Encode()
		}
		if len(current) >= batchSize || (len(current) > 0 && nextLen > maxChars) {
			chunks = append(chunks, current)
			current = make([]string, 0, min(batchSize, len(secids)))
			currentLen = 0
			nextLen = len(secid)
		}
		current = append(current, secid)
		currentLen = nextLen
	}
	if len(current) > 0 {
		chunks = append(chunks, current)
	}
	return chunks
}
