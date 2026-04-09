package monitor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var yahooChartHosts = []string{
	"query1.finance.yahoo.com",
	"query2.finance.yahoo.com",
}

func fetchYahooChart(ctx context.Context, client *http.Client, symbol string, params url.Values) (yahooChartResponse, error) {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}

	problems := make([]string, 0, len(yahooChartHosts))
	for _, host := range yahooChartHosts {
		parsed, err := fetchYahooChartFromHost(ctx, client, host, symbol, params)
		if err == nil {
			return parsed, nil
		}
		problems = append(problems, fmt.Sprintf("%s: %v", host, err))
	}

	return yahooChartResponse{}, collapseProblems(problems)
}

func fetchYahooChartFromHost(ctx context.Context, client *http.Client, host, symbol string, params url.Values) (yahooChartResponse, error) {
	query := cloneURLValues(params)
	if query.Get("lang") == "" {
		query.Set("lang", "en-US")
	}
	if query.Get("region") == "" {
		query.Set("region", "US")
	}
	if query.Get("corsDomain") == "" {
		query.Set("corsDomain", "finance.yahoo.com")
	}

	requestURL := fmt.Sprintf(
		"https://%s/v8/finance/chart/%s?%s",
		host,
		url.PathEscape(symbol),
		query.Encode(),
	)

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return yahooChartResponse{}, err
	}
	setYahooRequestHeaders(request)

	response, err := client.Do(request)
	if err != nil {
		return yahooChartResponse{}, err
	}
	defer response.Body.Close()

	payload, err := io.ReadAll(response.Body)
	if err != nil {
		return yahooChartResponse{}, err
	}

	if response.StatusCode != http.StatusOK {
		message := extractYahooFailure(payload)
		if message == "" {
			message = fmt.Sprintf("status %d", response.StatusCode)
		} else {
			message = fmt.Sprintf("status %d: %s", response.StatusCode, message)
		}
		return yahooChartResponse{}, errors.New(message)
	}

	var parsed yahooChartResponse
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return yahooChartResponse{}, err
	}
	if parsed.Chart.Error != nil {
		return yahooChartResponse{}, errors.New(parsed.Chart.Error.Description)
	}
	if len(parsed.Chart.Result) == 0 {
		return yahooChartResponse{}, errors.New("返回空结果")
	}

	return parsed, nil
}

func setYahooRequestHeaders(request *http.Request) {
	request.Header.Set("Accept", "application/json, text/plain, */*")
	request.Header.Set("Accept-Language", "en-US,en;q=0.9,zh-CN;q=0.8,zh;q=0.7")
	request.Header.Set("Cache-Control", "no-cache")
	request.Header.Set("Origin", "https://finance.yahoo.com")
	request.Header.Set("Pragma", "no-cache")
	request.Header.Set("Referer", "https://finance.yahoo.com/")
	request.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/123.0.0.0 Safari/537.36")
}

func extractYahooFailure(payload []byte) string {
	var parsed yahooChartResponse
	if err := json.Unmarshal(payload, &parsed); err == nil && parsed.Chart.Error != nil {
		return strings.TrimSpace(parsed.Chart.Error.Description)
	}

	trimmed := strings.TrimSpace(string(payload))
	if trimmed == "" {
		return ""
	}
	if len(trimmed) > 180 {
		trimmed = trimmed[:180]
	}
	return trimmed
}

func cloneURLValues(values url.Values) url.Values {
	cloned := make(url.Values, len(values))
	for key, entries := range values {
		cloned[key] = append([]string(nil), entries...)
	}
	return cloned
}
