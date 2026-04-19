package api

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"investgo/internal/marketdata"
	"investgo/internal/monitor"
	"investgo/internal/platform"
)

// Handler handles `/api/*` requests and coordinates backend services.
type Handler struct {
	store          *monitor.Store
	hot            *marketdata.HotService
	logs           *monitor.LogBook
	proxyTransport *platform.ProxyTransport
	routes         []route
}

const localeHeader = "X-InvestGo-Locale"

// clientLogRequest defines the JSON structure for log requests sent by the frontend.
type clientLogRequest struct {
	Source  string                    `json:"source"`
	Scope   string                    `json:"scope"`
	Level   monitor.DeveloperLogLevel `json:"level"`
	Message string                    `json:"message"`
}

type openExternalRequest struct {
	URL string `json:"url"`
}

type pinItemRequest struct {
	Pinned bool `json:"pinned"`
}

// NewHandler returns the unified API handler.
func NewHandler(store *monitor.Store, hot *marketdata.HotService, logs *monitor.LogBook, proxyTransport *platform.ProxyTransport) *Handler {
	handler := &Handler{
		store:          store,
		hot:            hot,
		logs:           logs,
		proxyTransport: proxyTransport,
	}
	handler.routes = handler.registerRoutes()
	return handler
}

// ServeHTTP strips the `/api` prefix uniformly and dispatches requests to registered routes.
func (h *Handler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")

	path := trimAPIPath(request.URL.Path)
	for _, route := range h.routes {
		params, ok := route.match(request.Method, path)
		if !ok {
			continue
		}
		route.handler(writer, request, params)
		return
	}

	writeError(writer, request, http.StatusNotFound, errNotFound(path))
}

// trimAPIPath trims the `/api` prefix registered by Wails into a relative path for internal routing.
func trimAPIPath(path string) string {
	trimmed := strings.TrimPrefix(path, "/api")
	if trimmed == "" {
		return "/"
	}
	return trimmed
}

// decodeJSON deserializes the request body into the target object and closes the body.
func decodeJSON(request *http.Request, target any) error {
	defer request.Body.Close()
	if err := json.NewDecoder(request.Body).Decode(target); err != nil {
		return &apiError{message: "Invalid JSON request body"}
	}
	return nil
}

// writeJSON writes a JSON response with the given status code.
func writeJSON(writer http.ResponseWriter, status int, payload any) {
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(payload)
}

// writeError encodes errors into a consistent JSON shape with a localized user message.
func writeError(writer http.ResponseWriter, request *http.Request, status int, err error) {
	debugMessage := strings.TrimSpace(err.Error())
	localizedMessage := monitor.LocalizeErrorMessage(requestLocale(request), debugMessage)

	payload := map[string]string{
		"error": localizedMessage,
	}
	if debugMessage != "" && debugMessage != localizedMessage {
		payload["debugError"] = debugMessage
	}

	writeJSON(writer, status, payload)
}

// errNotFound returns the error object used when an API route does not exist.
func errNotFound(path string) error {
	return &apiError{message: "API route not found: " + path}
}

// sanitiseDeveloperLogLevel falls back unknown log levels to info.
func sanitiseDeveloperLogLevel(level monitor.DeveloperLogLevel) monitor.DeveloperLogLevel {
	switch level {
	case monitor.DeveloperLogDebug, monitor.DeveloperLogInfo, monitor.DeveloperLogWarn, monitor.DeveloperLogError:
		return level
	default:
		return monitor.DeveloperLogInfo
	}
}

// sanitiseExternalURL validates and sanitizes external URL input, ensuring correct format and safe protocols.
func sanitiseExternalURL(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", &apiError{message: "URL must not be empty"}
	}

	parsed, err := url.Parse(value)
	if err != nil {
		return "", &apiError{message: "URL is invalid"}
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return "", &apiError{message: "Only http/https URLs are supported"}
	}
	if parsed.Host == "" {
		return "", &apiError{message: "URL is missing a host name"}
	}

	return parsed.String(), nil
}

func requestLocale(request *http.Request) string {
	if request == nil {
		return "en-US"
	}

	if locale := strings.TrimSpace(request.Header.Get(localeHeader)); locale != "" {
		return locale
	}
	if locale := strings.TrimSpace(request.Header.Get("Accept-Language")); locale != "" {
		return locale
	}
	return "en-US"
}

func localizeSnapshot(snapshot monitor.StateSnapshot, locale string) monitor.StateSnapshot {
	snapshot.Runtime.LastQuoteError = monitor.LocalizeErrorMessage(locale, snapshot.Runtime.LastQuoteError)
	snapshot.Runtime.LastFxError = monitor.LocalizeErrorMessage(locale, snapshot.Runtime.LastFxError)
	snapshot.Runtime.QuoteSource = localizeQuoteSourceSummary(locale, snapshot.Runtime.QuoteSource)
	snapshot.QuoteSources = localizeQuoteSourceOptions(locale, snapshot.QuoteSources)
	for index := range snapshot.Items {
		snapshot.Items[index].QuoteSource = localizeQuoteSourceName(locale, snapshot.Items[index].QuoteSource)
	}
	return snapshot
}

func localizeHistorySeries(series monitor.HistorySeries, locale string) monitor.HistorySeries {
	series.Source = localizeQuoteSourceName(locale, series.Source)
	return series
}

func localizeHotList(locale string, list monitor.HotListResponse) monitor.HotListResponse {
	for index := range list.Items {
		list.Items[index].QuoteSource = localizeQuoteSourceName(locale, list.Items[index].QuoteSource)
	}
	return list
}

func localizeQuoteSourceOptions(locale string, options []monitor.QuoteSourceOption) []monitor.QuoteSourceOption {
	localized := append([]monitor.QuoteSourceOption(nil), options...)
	for index := range localized {
		localized[index].Name = localizeQuoteSourceName(locale, localized[index].Name)
		localized[index].Description = localizeQuoteSourceDescription(locale, localized[index].ID, localized[index].Description)
	}
	return localized
}

func localizeQuoteSourceSummary(locale, summary string) string {
	replacements := []string{"EastMoney", "Yahoo Finance", "Sina Finance", "Xueqiu", "Alpha Vantage", "Twelve Data"}
	for _, name := range replacements {
		summary = strings.ReplaceAll(summary, name, localizeQuoteSourceName(locale, name))
	}
	return summary
}

func localizeQuoteSourceName(locale, name string) string {
	if strings.EqualFold(locale, "zh-CN") || strings.HasPrefix(strings.ToLower(locale), "zh") {
		switch name {
		case "EastMoney":
			return "东方财富"
		case "Yahoo Finance":
			return "雅虎财经"
		case "Sina Finance":
			return "新浪财经"
		case "Xueqiu":
			return "雪球"
		case "Alpha Vantage":
			return "Alpha Vantage"
		case "Twelve Data":
			return "Twelve Data"
		}
	}
	return name
}

func localizeQuoteSourceDescription(locale, sourceID, fallback string) string {
	if !(strings.EqualFold(locale, "zh-CN") || strings.HasPrefix(strings.ToLower(locale), "zh")) {
		return fallback
	}

	switch strings.ToLower(strings.TrimSpace(sourceID)) {
	case "eastmoney":
		return "覆盖 A 股、港股和美股，字段最完整，适合作为默认综合行情源。"
	case "yahoo":
		return "港股和美股覆盖较稳定，适合以海外市场为主的组合。"
	case "sina":
		return "A 股与境内 ETF 刷新较快，适合国内市场盯盘。"
	case "xueqiu":
		return "覆盖 A 股和港股，适合作为社区型补充来源。"
	case "alpha-vantage":
		return "适合美股和美股 ETF 的 API 型数据源，实时与历史都可走同一来源。"
	case "twelve-data":
		return "较稳定的美股与美股 ETF API 型数据源，适合统一实时和历史链路。"
	default:
		return fallback
	}
}

// apiError represents a response error constructed internally by the API layer.
type apiError struct {
	message string
}

// Error implements the error interface.
func (e *apiError) Error() string {
	return e.message
}
