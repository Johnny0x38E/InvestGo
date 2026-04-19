package marketdata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"sync"
	"time"

	"investgo/internal/datasource"
)

type yahooChartResponse struct {
	Chart struct {
		Result []struct {
			Meta struct {
				Currency string  `json:"currency"`
				Symbol   string  `json:"symbol"`
				Price    float64 `json:"regularMarketPrice"`
			} `json:"meta"`
			Timestamp  []int64 `json:"timestamp"`
			Indicators struct {
				Quote []struct {
					Open   []*float64 `json:"open"`
					High   []*float64 `json:"high"`
					Low    []*float64 `json:"low"`
					Close  []*float64 `json:"close"`
					Volume []*float64 `json:"volume"`
				} `json:"quote"`
			} `json:"indicators"`
		} `json:"result"`
		Error *struct {
			Description string `json:"description"`
		} `json:"error"`
	} `json:"chart"`
}

var yahooCookieJarOnce sync.Once
var yahooCookieJar http.CookieJar

func getYahooCookieJar() http.CookieJar {
	yahooCookieJarOnce.Do(func() {
		jar, err := cookiejar.New(nil)
		if err == nil {
			yahooCookieJar = jar
		}
	})
	return yahooCookieJar
}

func cloneYahooClient(client *http.Client) *http.Client {
	cloned := *client
	if cloned.Timeout == 0 {
		cloned.Timeout = 10 * time.Second
	}
	if cloned.Jar == nil {
		cloned.Jar = getYahooCookieJar()
	}
	return &cloned
}

func setYahooBrowserHeaders(request *http.Request, host string) {
	request.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15")
	request.Header.Set("Accept", "application/json,text/plain,*/*")
	request.Header.Set("Accept-Language", "en-US,en;q=0.9")
	request.Header.Set("Origin", datasource.YahooFinanceOrigin)
	request.Header.Set("Referer", datasource.YahooFinanceReferer)
	request.Header.Set("Sec-Fetch-Site", "same-site")
	request.Header.Set("Sec-Fetch-Mode", "cors")
	request.Header.Set("Sec-Fetch-Dest", "empty")
	request.Header.Set("Cache-Control", "no-cache")
	request.Header.Set("Pragma", "no-cache")
	request.Header.Set("Connection", "keep-alive")
	request.Host = host
}

func primeYahooSession(ctx context.Context, client *http.Client) error {
	if client == nil {
		return errors.New("client is nil")
	}
	if client.Jar == nil {
		return errors.New("cookie jar is not configured")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, datasource.YahooFinanceOrigin, nil)
	if err != nil {
		return err
	}
	setYahooBrowserHeaders(req, datasource.YahooFinanceDomain)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}

// fetchYahooChart polls multiple Yahoo Finance hosts for quote data, returning the first successful response or a combined error message.
func fetchYahooChart(ctx context.Context, client *http.Client, symbol string, params url.Values) (yahooChartResponse, error) {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}

	primedClient := cloneYahooClient(client)
	_ = primeYahooSession(ctx, primedClient)

	problems := make([]string, 0, len(datasource.YahooChartHosts))
	for _, host := range datasource.YahooChartHosts {
		parsed, err := fetchYahooChartFromHost(ctx, primedClient, host, symbol, params)
		if err == nil {
			return parsed, nil
		}
		problems = append(problems, fmt.Sprintf("%s: %v", host, err))
	}

	return yahooChartResponse{}, collapseProblems(problems)
}

// fetchYahooChartFromHost sends a request to the specified Yahoo Finance host, parses the response and handles possible errors.
func fetchYahooChartFromHost(ctx context.Context, client *http.Client, host, symbol string, params url.Values) (yahooChartResponse, error) {
	query := make(url.Values, len(params))
	for key, values := range params {
		query[key] = append([]string(nil), values...)
	}
	query.Set("corsDomain", datasource.YahooFinanceDomain)

	requestURL := url.URL{
		Scheme:   "https",
		Host:     host,
		Path:     datasource.YahooChartPathPrefix + url.PathEscape(symbol),
		RawQuery: query.Encode(),
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL.String(), nil)
	if err != nil {
		return yahooChartResponse{}, err
	}
	setYahooBrowserHeaders(request, host)

	response, err := client.Do(request)
	if err != nil {
		return yahooChartResponse{}, err
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return yahooChartResponse{}, err
	}

	if response.StatusCode != http.StatusOK {
		var parsed yahooChartResponse
		if err := json.Unmarshal(body, &parsed); err == nil && parsed.Chart.Error != nil {
			return yahooChartResponse{}, errors.New(parsed.Chart.Error.Description)
		}
		return yahooChartResponse{}, fmt.Errorf("status %d", response.StatusCode)
	}

	var parsed yahooChartResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return yahooChartResponse{}, err
	}
	if parsed.Chart.Error != nil {
		return yahooChartResponse{}, errors.New(parsed.Chart.Error.Description)
	}
	if len(parsed.Chart.Result) == 0 {
		return yahooChartResponse{}, errors.New("No results returned")
	}

	return parsed, nil
}
