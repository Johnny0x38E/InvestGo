package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"investgo/internal/monitor"
)

// route defines an API route including HTTP method, path template and handler.
type route struct {
	method  string
	pattern string
	handler routeHandler
}

// routeHandler defines the route handler signature with path parameters.
type routeHandler func(http.ResponseWriter, *http.Request, routeParams)

// routeParams stores path parameters extracted from the route template.
type routeParams map[string]string

// Value returns the path parameter value for the given name.
func (params routeParams) Value(name string) string {
	return params[name]
}

// registerRoutes registers all API routes in one place.
func (h *Handler) registerRoutes() []route {
	return []route{
		{method: http.MethodGet, pattern: "/state", handler: h.handleState},
		{method: http.MethodGet, pattern: "/overview", handler: h.handleOverview},
		{method: http.MethodGet, pattern: "/logs", handler: h.handleLogs},
		{method: http.MethodDelete, pattern: "/logs", handler: h.handleClearLogs},
		{method: http.MethodPost, pattern: "/client-logs", handler: h.handleClientLogs},
		{method: http.MethodGet, pattern: "/hot", handler: h.handleHot},
		{method: http.MethodGet, pattern: "/history", handler: h.handleHistory},
		{method: http.MethodPost, pattern: "/refresh", handler: h.handleRefresh},
		{method: http.MethodPost, pattern: "/open-external", handler: h.handleOpenExternal},
		{method: http.MethodPut, pattern: "/settings", handler: h.handleUpdateSettings},
		{method: http.MethodPost, pattern: "/items", handler: h.handleCreateItem},
		{method: http.MethodPut, pattern: "/items/{id}", handler: h.handleUpdateItem},
		{method: http.MethodPut, pattern: "/items/{id}/pin", handler: h.handlePinItem},
		{method: http.MethodDelete, pattern: "/items/{id}", handler: h.handleDeleteItem},
		{method: http.MethodPost, pattern: "/alerts", handler: h.handleCreateAlert},
		{method: http.MethodPut, pattern: "/alerts/{id}", handler: h.handleUpdateAlert},
		{method: http.MethodDelete, pattern: "/alerts/{id}", handler: h.handleDeleteAlert},
	}
}

// handleOverview returns the backend-computed analytics payload for the overview module.
func (h *Handler) handleOverview(writer http.ResponseWriter, request *http.Request, _ routeParams) {
	analytics, err := h.store.OverviewAnalytics(request.Context())
	if err != nil {
		writeError(writer, request, http.StatusBadGateway, err)
		return
	}
	writeJSON(writer, http.StatusOK, analytics)
}

// handleOpenExternal opens an external link using the platform default browser.
func (h *Handler) handleOpenExternal(writer http.ResponseWriter, request *http.Request, _ routeParams) {
	var payload openExternalRequest
	if err := decodeJSON(request, &payload); err != nil {
		writeError(writer, request, http.StatusBadRequest, err)
		return
	}

	targetURL, err := sanitiseExternalURL(payload.URL)
	if err != nil {
		writeError(writer, request, http.StatusBadRequest, err)
		return
	}

	if err := openExternalURL(targetURL); err != nil {
		writeError(writer, request, http.StatusInternalServerError, &apiError{message: "Failed to open external URL"})
		return
	}

	writeJSON(writer, http.StatusOK, map[string]bool{"ok": true})
}

// match checks whether this route matches the given HTTP method and path.
func (r route) match(method, path string) (routeParams, bool) {
	if method != r.method {
		return nil, false
	}
	return matchRoutePattern(r.pattern, path)
}

// matchRoutePattern matches a path against the template using simple `{name}` segment parameters.
func matchRoutePattern(pattern, path string) (routeParams, bool) {
	patternSegments := routeSegments(pattern)
	pathSegments := routeSegments(path)
	if len(patternSegments) != len(pathSegments) {
		return nil, false
	}

	params := make(routeParams)
	for index, segment := range patternSegments {
		if len(segment) >= 2 && strings.HasPrefix(segment, "{") && strings.HasSuffix(segment, "}") {
			params[segment[1:len(segment)-1]] = pathSegments[index]
			continue
		}
		if segment != pathSegments[index] {
			return nil, false
		}
	}

	return params, true
}

// routeSegments splits a path into segments for the matcher.
func routeSegments(path string) []string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "/")
}

// handleState returns the full state snapshot currently required by the frontend.
func (h *Handler) handleState(writer http.ResponseWriter, request *http.Request, _ routeParams) {
	writeJSON(writer, http.StatusOK, localizeSnapshot(h.store.Snapshot(), requestLocale(request)))
}

// handleLogs returns the developer log snapshot.
func (h *Handler) handleLogs(writer http.ResponseWriter, request *http.Request, _ routeParams) {
	limit, _ := strconv.Atoi(strings.TrimSpace(request.URL.Query().Get("limit")))
	if h.logs == nil {
		writeJSON(writer, http.StatusOK, monitor.DeveloperLogSnapshot{
			Entries:     []monitor.DeveloperLogEntry{},
			GeneratedAt: time.Now(),
		})
		return
	}

	writeJSON(writer, http.StatusOK, h.logs.Snapshot(limit))
}

// handleClearLogs clears persisted developer logs.
func (h *Handler) handleClearLogs(writer http.ResponseWriter, request *http.Request, _ routeParams) {
	if h.logs != nil {
		if err := h.logs.Clear(); err != nil {
			writeError(writer, request, http.StatusInternalServerError, err)
			return
		}
	}

	writeJSON(writer, http.StatusOK, map[string]bool{"ok": true})
}

// handleClientLogs accepts developer logs reported by the frontend.
func (h *Handler) handleClientLogs(writer http.ResponseWriter, request *http.Request, _ routeParams) {
	if h.logs == nil {
		writeJSON(writer, http.StatusOK, map[string]bool{"ok": true})
		return
	}

	var payload clientLogRequest
	if err := decodeJSON(request, &payload); err != nil {
		writeError(writer, request, http.StatusBadRequest, err)
		return
	}

	h.logs.Log(payload.Source, payload.Scope, sanitiseDeveloperLogLevel(payload.Level), payload.Message)
	writeJSON(writer, http.StatusOK, map[string]bool{"ok": true})
}

// handleHot returns the hot list for the given category and sort order.
func (h *Handler) handleHot(writer http.ResponseWriter, request *http.Request, _ routeParams) {
	if h.hot == nil {
		writeError(writer, request, http.StatusServiceUnavailable, &apiError{message: "Hot service is unavailable"})
		return
	}

	category := monitor.HotCategory(strings.TrimSpace(request.URL.Query().Get("category")))
	sortBy := monitor.HotSort(strings.TrimSpace(request.URL.Query().Get("sort")))
	keyword := strings.TrimSpace(request.URL.Query().Get("q"))
	page, _ := strconv.Atoi(strings.TrimSpace(request.URL.Query().Get("page")))
	pageSize, _ := strconv.Atoi(strings.TrimSpace(request.URL.Query().Get("pageSize")))
	list, err := h.hot.List(request.Context(), category, sortBy, keyword, page, pageSize)
	if err != nil {
		writeError(writer, request, http.StatusBadRequest, err)
		return
	}

	writeJSON(writer, http.StatusOK, list)
}

// handleHistory returns historical quotes for the given instrument and time range.
func (h *Handler) handleHistory(writer http.ResponseWriter, request *http.Request, _ routeParams) {
	itemID := strings.TrimSpace(request.URL.Query().Get("itemId"))
	interval := monitor.HistoryInterval(strings.TrimSpace(request.URL.Query().Get("interval")))
	if interval == "" {
		interval = monitor.HistoryRange1d
	}

	series, err := h.store.ItemHistory(request.Context(), itemID, interval)
	if err != nil {
		writeError(writer, request, http.StatusBadRequest, err)
		return
	}

	writeJSON(writer, http.StatusOK, series)
}

// handleRefresh triggers a full quote refresh.
func (h *Handler) handleRefresh(writer http.ResponseWriter, request *http.Request, _ routeParams) {
	snapshot, err := h.store.Refresh(request.Context())
	if err != nil {
		writeError(writer, request, http.StatusInternalServerError, err)
		return
	}

	writeJSON(writer, http.StatusOK, localizeSnapshot(snapshot, requestLocale(request)))
}

// handleUpdateSettings updates application settings.
func (h *Handler) handleUpdateSettings(writer http.ResponseWriter, request *http.Request, _ routeParams) {
	var settings monitor.AppSettings
	if err := decodeJSON(request, &settings); err != nil {
		writeError(writer, request, http.StatusBadRequest, err)
		return
	}

	snapshot, err := h.store.UpdateSettings(settings)
	if err != nil {
		writeError(writer, request, http.StatusBadRequest, err)
		return
	}

	writeJSON(writer, http.StatusOK, localizeSnapshot(snapshot, requestLocale(request)))
}

// handleCreateItem creates a new tracked item (watch-only or held position).
func (h *Handler) handleCreateItem(writer http.ResponseWriter, request *http.Request, _ routeParams) {
	var item monitor.WatchlistItem
	if err := decodeJSON(request, &item); err != nil {
		writeError(writer, request, http.StatusBadRequest, err)
		return
	}

	snapshot, err := h.store.UpsertItem(item)
	if err != nil {
		writeError(writer, request, http.StatusBadRequest, err)
		return
	}

	writeJSON(writer, http.StatusOK, localizeSnapshot(snapshot, requestLocale(request)))
}

// handleUpdateItem updates the tracked item with the given ID.
func (h *Handler) handleUpdateItem(writer http.ResponseWriter, request *http.Request, params routeParams) {
	var item monitor.WatchlistItem
	if err := decodeJSON(request, &item); err != nil {
		writeError(writer, request, http.StatusBadRequest, err)
		return
	}

	item.ID = params.Value("id")
	snapshot, err := h.store.UpsertItem(item)
	if err != nil {
		writeError(writer, request, http.StatusBadRequest, err)
		return
	}

	writeJSON(writer, http.StatusOK, localizeSnapshot(snapshot, requestLocale(request)))
}

// handleDeleteItem deletes the tracked item with the given ID.
func (h *Handler) handleDeleteItem(writer http.ResponseWriter, request *http.Request, params routeParams) {
	snapshot, err := h.store.DeleteItem(params.Value("id"))
	if err != nil {
		writeError(writer, request, http.StatusBadRequest, err)
		return
	}

	writeJSON(writer, http.StatusOK, localizeSnapshot(snapshot, requestLocale(request)))
}

// handlePinItem updates the pinned state of the tracked item with the given ID.
func (h *Handler) handlePinItem(writer http.ResponseWriter, request *http.Request, params routeParams) {
	var payload pinItemRequest
	if err := decodeJSON(request, &payload); err != nil {
		writeError(writer, request, http.StatusBadRequest, err)
		return
	}

	snapshot, err := h.store.SetItemPinned(params.Value("id"), payload.Pinned)
	if err != nil {
		writeError(writer, request, http.StatusBadRequest, err)
		return
	}

	writeJSON(writer, http.StatusOK, localizeSnapshot(snapshot, requestLocale(request)))
}

// handleCreateAlert creates a new price alert.
func (h *Handler) handleCreateAlert(writer http.ResponseWriter, request *http.Request, _ routeParams) {
	var alert monitor.AlertRule
	if err := decodeJSON(request, &alert); err != nil {
		writeError(writer, request, http.StatusBadRequest, err)
		return
	}

	snapshot, err := h.store.UpsertAlert(alert)
	if err != nil {
		writeError(writer, request, http.StatusBadRequest, err)
		return
	}

	writeJSON(writer, http.StatusOK, localizeSnapshot(snapshot, requestLocale(request)))
}

// handleUpdateAlert updates the price alert with the given ID.
func (h *Handler) handleUpdateAlert(writer http.ResponseWriter, request *http.Request, params routeParams) {
	var alert monitor.AlertRule
	if err := decodeJSON(request, &alert); err != nil {
		writeError(writer, request, http.StatusBadRequest, err)
		return
	}

	alert.ID = params.Value("id")
	snapshot, err := h.store.UpsertAlert(alert)
	if err != nil {
		writeError(writer, request, http.StatusBadRequest, err)
		return
	}

	writeJSON(writer, http.StatusOK, localizeSnapshot(snapshot, requestLocale(request)))
}

// handleDeleteAlert deletes the price alert with the given ID.
func (h *Handler) handleDeleteAlert(writer http.ResponseWriter, request *http.Request, params routeParams) {
	snapshot, err := h.store.DeleteAlert(params.Value("id"))
	if err != nil {
		writeError(writer, request, http.StatusBadRequest, err)
		return
	}

	writeJSON(writer, http.StatusOK, localizeSnapshot(snapshot, requestLocale(request)))
}
