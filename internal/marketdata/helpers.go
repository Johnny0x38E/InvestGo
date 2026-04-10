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

// collectQuoteTargets 把输入标的转换为标准目标，并汇总解析失败信息。
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

// buildQuote 根据关键价格字段构造统一 Quote 对象。
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

// parseFloat 安全解析接口里的数值字段。
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

// collapseProblems 去重并合并多条错误信息。
func collapseProblems(problems []string) error {
	if len(problems) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(problems))
	uniq := make([]string, 0, len(problems))
	for _, problem := range problems {
		problem = strings.TrimSpace(problem)
		if problem == "" {
			continue
		}
		if _, exists := seen[problem]; exists {
			continue
		}
		seen[problem] = struct{}{}
		uniq = append(uniq, problem)
	}

	if len(uniq) == 0 {
		return nil
	}

	return errors.New(strings.Join(uniq, "；"))
}

// firstNonEmpty 返回第一个非空字符串。
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func partsAt(parts []string, index int) string {
	if index < 0 || index >= len(parts) {
		return ""
	}
	return parts[index]
}

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

func decodeGB18030Body(body io.Reader) (string, error) {
	reader := transform.NewReader(body, simplifiedchinese.GB18030.NewDecoder())
	payload, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(bytes.TrimPrefix(payload, []byte{0xef, 0xbb, 0xbf}))), nil
}

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

// isDigits 判断字符串是否全部由数字组成。
func isDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// isLetters 判断字符串是否全部由英文字母组成。
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

// UnmarshalJSON 兼容东方财富数值字段缺失时返回 "-" 的情况。
func (f *emFloat) UnmarshalJSON(data []byte) error {
	var value float64
	if err := json.Unmarshal(data, &value); err == nil {
		*f = emFloat(value)
		return nil
	}
	*f = 0
	return nil
}
