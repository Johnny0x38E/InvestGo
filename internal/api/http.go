package api

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"investgo/internal/api/i18n"
	"investgo/internal/core"
	"investgo/internal/core/hot"
	"investgo/internal/core/store"
	"investgo/internal/logger"
	"investgo/internal/platform"
)

// Handler handles `/api/*` requests and coordinates backend services.
type Handler struct {
	store          *store.Store
	hot            *hot.HotService
	logs           *logger.LogBook
	proxyTransport *platform.ProxyTransport
	mux            *http.ServeMux // internal router (Go 1.22+ pattern matching)
}

const localeHeader = "X-InvestGo-Locale"

// clientLogRequest defines the JSON structure for log requests sent by the frontend.
type clientLogRequest struct {
	Source  string                   `json:"source"`
	Scope   string                   `json:"scope"`
	Level   logger.DeveloperLogLevel `json:"level"`
	Message string                   `json:"message"`
}

type openExternalRequest struct {
	URL string `json:"url"`
}

type pinItemRequest struct {
	Pinned bool `json:"pinned"`
}

// NewHandler returns the unified API handler.
func NewHandler(store *store.Store, hot *hot.HotService, logs *logger.LogBook, proxyTransport *platform.ProxyTransport) *Handler {
	h := &Handler{
		store:          store,
		hot:            hot,
		logs:           logs,
		proxyTransport: proxyTransport,
	}
	h.mux = h.buildMux()
	return h
}

// buildMux registers all API routes on an http.ServeMux.
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
	localizedMessage := i18n.LocalizeErrorMessage(requestLocale(request), debugMessage)

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
func sanitiseDeveloperLogLevel(level logger.DeveloperLogLevel) logger.DeveloperLogLevel {
	switch level {
	case logger.DeveloperLogDebug, logger.DeveloperLogInfo, logger.DeveloperLogWarn, logger.DeveloperLogError:
		return level
	default:
		return logger.DeveloperLogInfo
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

func localizeSnapshot(snapshot core.StateSnapshot, locale string) core.StateSnapshot {
	snapshot.Runtime.LastQuoteError = i18n.LocalizeErrorMessage(locale, snapshot.Runtime.LastQuoteError)
	snapshot.Runtime.LastFxError = i18n.LocalizeErrorMessage(locale, snapshot.Runtime.LastFxError)
	snapshot.Runtime.QuoteSource = localizeQuoteSourceSummary(locale, snapshot.Runtime.QuoteSource)
	snapshot.QuoteSources = localizeQuoteSourceOptions(locale, snapshot.QuoteSources)
	for index := range snapshot.Items {
		snapshot.Items[index].QuoteSource = localizeQuoteSourceName(locale, snapshot.Items[index].QuoteSource)
	}
	return snapshot
}

func localizeHistorySeries(series core.HistorySeries, locale string) core.HistorySeries {
	series.Source = localizeQuoteSourceName(locale, series.Source)
	return series
}

func localizeHotList(locale string, list core.HotListResponse) core.HotListResponse {
	for index := range list.Items {
		list.Items[index].QuoteSource = localizeQuoteSourceName(locale, list.Items[index].QuoteSource)
	}
	return list
}

func localizeQuoteSourceOptions(locale string, options []core.QuoteSourceOption) []core.QuoteSourceOption {
	localized := append([]core.QuoteSourceOption(nil), options...)
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
			return "东方财富"
		case "Yahoo Finance":
			return "雅虎财经"
		case "Sina Finance":
			return "新浪财经"
		case "Xueqiu":
			return "雪球"
		case "Tencent Finance":
			return "腾讯财经"
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
		return "覆盖 A 股、港股和美股，字段最完整，适合作为默认综合行情源。"
	case "yahoo":
		return "港股和美股覆盖较稳定，适合以海外市场为主的组合。"
	case "sina":
		return "A 股与境内 ETF 刷新较快，适合国内市场盯盘。"
	case "xueqiu":
		return "覆盖 A 股和港股，适合作为社区型补充来源。"
	case "tencent":
		return "腾讯财经提供 A 股、港股和美股的实时行情，并提供轻量 K 线接口作为补充。"
	case "alpha-vantage":
		return "适合美股和美股 ETF 的 API 型数据源，实时与历史都可走同一来源。"
	case "twelve-data":
		return "较稳定的美股与美股 ETF API 型数据源，适合统一实时和历史链路。"
	case "finnhub":
		return "面向美股与 ETF 的 API 数据源，适合统一接入实时价格和 K 线历史。"
	case "polygon":
		return "Polygon.io（Massive）提供的美股与 ETF API 数据源，适合高质量实时与历史链路。"
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
