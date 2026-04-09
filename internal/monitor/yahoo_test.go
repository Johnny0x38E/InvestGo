package monitor

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestFetchYahooChartFallsBackToQuery2(t *testing.T) {
	t.Helper()

	client := &http.Client{
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			switch request.URL.Host {
			case "query1.finance.yahoo.com":
				return &http.Response{
					StatusCode: http.StatusForbidden,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`{"chart":{"result":null,"error":{"description":"Forbidden"}}}`)),
					Request:    request,
				}, nil
			case "query2.finance.yahoo.com":
				return &http.Response{
					StatusCode: http.StatusOK,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: io.NopCloser(strings.NewReader(`{
						"chart": {
							"result": [{
								"meta": {"currency":"USD","symbol":"AAPL","regularMarketPrice":190.5},
								"timestamp": [1710000000,1710086400],
								"indicators": {
									"quote": [{
										"open": [188.1,189.2],
										"high": [191.2,192.4],
										"low": [187.0,188.7],
										"close": [189.5,190.5],
										"volume": [1000,1200]
									}]
								}
							}],
							"error": null
						}
					}`)),
					Request: request,
				}, nil
			default:
				t.Fatalf("unexpected yahoo host: %s", request.URL.Host)
				return nil, nil
			}
		}),
	}

	params := url.Values{}
	params.Set("range", "6mo")
	params.Set("interval", "1d")

	parsed, err := fetchYahooChart(context.Background(), client, "AAPL", params)
	if err != nil {
		t.Fatalf("fetchYahooChart returned error: %v", err)
	}
	if len(parsed.Chart.Result) != 1 {
		t.Fatalf("unexpected result count: %d", len(parsed.Chart.Result))
	}
	if got := parsed.Chart.Result[0].Meta.Symbol; got != "AAPL" {
		t.Fatalf("unexpected symbol: %s", got)
	}
}

func TestFetchYahooChartSetsBrowserLikeHeaders(t *testing.T) {
	t.Helper()

	client := &http.Client{
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			if got := request.Header.Get("User-Agent"); !strings.Contains(got, "Mozilla/5.0") {
				t.Fatalf("unexpected User-Agent: %s", got)
			}
			if got := request.Header.Get("Referer"); got != "https://finance.yahoo.com/" {
				t.Fatalf("unexpected Referer: %s", got)
			}
			if got := request.Header.Get("Origin"); got != "https://finance.yahoo.com" {
				t.Fatalf("unexpected Origin: %s", got)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					"Content-Type": []string{"application/json"},
				},
				Body: io.NopCloser(strings.NewReader(`{
					"chart": {
						"result": [{
							"meta": {"currency":"USD","symbol":"AAPL","regularMarketPrice":190.5},
							"timestamp": [1710000000],
							"indicators": {
								"quote": [{
									"open": [188.1],
									"high": [191.2],
									"low": [187.0],
									"close": [190.5],
									"volume": [1000]
								}]
							}
						}],
						"error": null
					}
				}`)),
				Request: request,
			}, nil
		}),
	}

	params := url.Values{}
	params.Set("range", "5d")
	params.Set("interval", "1d")

	if _, err := fetchYahooChart(context.Background(), client, "AAPL", params); err != nil {
		t.Fatalf("fetchYahooChart returned error: %v", err)
	}
}
