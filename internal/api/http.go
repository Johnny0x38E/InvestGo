package api

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"investgo/internal/marketdata"
	"investgo/internal/monitor"
)

// Handler handles `/api/*` requests and coordinates backend services.
type Handler struct {
	store  *monitor.Store
	hot    *marketdata.HotService
	logs   *monitor.LogBook
	routes []route
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
func NewHandler(store *monitor.Store, hot *marketdata.HotService, logs *monitor.LogBook) *Handler {
	handler := &Handler{
		store: store,
		hot:   hot,
		logs:  logs,
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
	return snapshot
}

// apiError represents a response error constructed internally by the API layer.
type apiError struct {
	message string
}

// Error implements the error interface.
func (e *apiError) Error() string {
	return e.message
}
