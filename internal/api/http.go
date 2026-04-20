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
	mux            *http.ServeMux // internal router (Go 1.22+ pattern matching)
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
	h := &Handler{
		store:          store,
		hot:            hot,
		logs:           logs,
		proxyTransport: proxyTransport,
	}
	h.mux = h.buildMux()
	return h
}

// buildMux registers all API routes on an http.ServeMux using Go 1.22+ method+pattern syntax.
// Path parameters (e.g. {id}) are retrieved via r.PathValue("id") inside handlers.
func (h *Handler) buildMux() *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /state", h.handleState)
	mux.HandleFunc("GET /overview", h.handleOverview)
	mux.HandleFunc("GET /logs", h.handleLogs)
	mux.HandleFunc("DELETE /logs", h.handleClearLogs)
	mux.HandleFunc("POST /client-logs", h.handleClientLogs)
	mux.HandleFunc("GET /hot", h.handleHot)
	mux.HandleFunc("GET /history", h.handleHistory)
	mux.HandleFunc("POST /refresh", h.handleRefresh)
	mux.HandleFunc("POST /open-external", h.handleOpenExternal)
	mux.HandleFunc("PUT /settings", h.handleUpdateSettings)
	mux.HandleFunc("POST /items", h.handleCreateItem)
	mux.HandleFunc("POST /items/{id}/refresh", h.handleRefreshItem)
	mux.HandleFunc("PUT /items/{id}", h.handleUpdateItem)
	mux.HandleFunc("PUT /items/{id}/pin", h.handlePinItem)
	mux.HandleFunc("DELETE /items/{id}", h.handleDeleteItem)
	mux.HandleFunc("POST /alerts", h.handleCreateAlert)
	mux.HandleFunc("PUT /alerts/{id}", h.handleUpdateAlert)
	mux.HandleFunc("DELETE /alerts/{id}", h.handleDeleteAlert)

	// Catch-all: return a JSON 404 for any unmatched path.
	mux.HandleFunc("/{path...}", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, r, http.StatusNotFound, errNotFound(r.URL.Path))
	})

	return mux
}

// ServeHTTP strips the `/api` prefix and delegates to the inner ServeMux.
func (h *Handler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")

	// Strip the /api prefix registered on the outer mux so inner patterns
	// are relative (e.g. "/api/items/x" becomes "/items/x").
	r2 := request.Clone(request.Context())
	r2.URL = new(url.URL)
	*r2.URL = *request.URL
	r2.URL.Path = trimAPIPath(request.URL.Path)
	h.mux.ServeHTTP(writer, r2)
}

// trimAPIPath strips the `/api` prefix registered by the outer mux.
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

// sanitiseExternalURL validates and sanitizes external URL input.
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
	replacements := []string{"EastMoney", "Yahoo Finance", "Sina Finance", "Xueqiu", "Tencent Finance", "Alpha Vantage", "Twelve Data", "Finnhub", "Polygon"}
	for _, name := range replacements {
		summary = strings.ReplaceAll(summary, name, localizeQuoteSourceName(locale, name))
	}
	return summary
}

func localizeQuoteSourceName(locale, name string) string {
	if strings.EqualFold(locale, "zh-CN") || strings.HasPrefix(strings.ToLower(locale), "zh") {
		switch name {
		case "EastMoney":
			return "\u4e1c\u65b9\u8d22\u5bcc"
		case "Yahoo Finance":
			return "\u96c5\u864e\u8d22\u7ecf"
		case "Sina Finance":
			return "\u65b0\u6d6a\u8d22\u7ecf"
		case "Xueqiu":
			return "\u96ea\u7403"
		case "Tencent Finance":
			return "\u817e\u8baf\u8d22\u7ecf"
		case "Alpha Vantage":
			return "Alpha Vantage"
		case "Twelve Data":
			return "Twelve Data"
		case "Finnhub":
			return "Finnhub"
		case "Polygon":
			return "Polygon"
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
		return "\u8986\u76d6 A \u80a1\u3001\u6e2f\u80a1\u548c\u7f8e\u80a1\uff0c\u5b57\u6bb5\u6700\u5b8c\u6574\uff0c\u9002\u5408\u4f5c\u4e3a\u9ed8\u8ba4\u7efc\u5408\u884c\u60c5\u6e90\u3002"
	case "yahoo":
		return "\u6e2f\u80a1\u548c\u7f8e\u80a1\u8986\u76d6\u8f83\u7a33\u5b9a\uff0c\u9002\u5408\u4ee5\u6d77\u5916\u5e02\u573a\u4e3a\u4e3b\u7684\u7ec4\u5408\u3002"
	case "sina":
		return "A \u80a1\u4e0e\u5883\u5185 ETF \u5237\u65b0\u8f83\u5feb\uff0c\u9002\u5408\u56fd\u5185\u5e02\u573a\u76ef\u76d8\u3002"
	case "xueqiu":
		return "\u8986\u76d6 A \u80a1\u548c\u6e2f\u80a1\uff0c\u9002\u5408\u4f5c\u4e3a\u793e\u533a\u578b\u8865\u5145\u6765\u6e90\u3002"
	case "tencent":
		return "\u817e\u8baf\u8d22\u7ecf\u63d0\u4f9b A \u80a1\u3001\u6e2f\u80a1\u548c\u7f8e\u80a1\u7684\u5b9e\u65f6\u884c\u60c5\uff0c\u5e76\u63d0\u4f9b\u8f7b\u91cf K \u7ebf\u63a5\u53e3\u4f5c\u4e3a\u8865\u5145\u3002"
	case "alpha-vantage":
		return "\u9002\u5408\u7f8e\u80a1\u548c\u7f8e\u80a1 ETF \u7684 API \u578b\u6570\u636e\u6e90\uff0c\u5b9e\u65f6\u4e0e\u5386\u53f2\u90fd\u53ef\u8d70\u540c\u4e00\u6765\u6e90\u3002"
	case "twelve-data":
		return "\u8f83\u7a33\u5b9a\u7684\u7f8e\u80a1\u4e0e\u7f8e\u80a1 ETF API \u578b\u6570\u636e\u6e90\uff0c\u9002\u5408\u7edf\u4e00\u5b9e\u65f6\u548c\u5386\u53f2\u94fe\u8def\u3002"
	case "finnhub":
		return "\u9762\u5411\u7f8e\u80a1\u4e0e ETF \u7684 API \u6570\u636e\u6e90\uff0c\u9002\u5408\u7edf\u4e00\u63a5\u5165\u5b9e\u65f6\u4ef7\u683c\u548c K \u7ebf\u5386\u53f2\u3002"
	case "polygon":
		return "Polygon.io\uff08Massive\uff09\u63d0\u4f9b\u7684\u7f8e\u80a1\u4e0e ETF API \u6570\u636e\u6e90\uff0c\u9002\u5408\u9ad8\u8d28\u91cf\u5b9e\u65f6\u4e0e\u5386\u53f2\u94fe\u8def\u3002"
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
