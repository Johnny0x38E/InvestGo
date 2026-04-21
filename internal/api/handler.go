package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"investgo/internal/logger"
	"investgo/internal/core/hot"
	"investgo/internal/core"
)

// handleOverview returns the backend-computed analytics payload for the overview module.
func (h *Handler) handleOverview(writer http.ResponseWriter, request *http.Request) {
	analytics, err := h.store.OverviewAnalytics(request.Context(), parseBoolQuery(request.URL.Query().Get("force")))
	if err != nil {
		writeError(writer, request, http.StatusBadGateway, err)
		return
	}
	writeJSON(writer, http.StatusOK, analytics)
}

// handleOpenExternal opens an external link using the platform default browser.
func (h *Handler) handleOpenExternal(writer http.ResponseWriter, request *http.Request) {
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

func parseBoolQuery(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

// handleState returns the full state snapshot currently required by the frontend.
func (h *Handler) handleState(writer http.ResponseWriter, request *http.Request) {
	writeJSON(writer, http.StatusOK, localizeSnapshot(h.store.Snapshot(), requestLocale(request)))
}

// handleLogs returns the developer log snapshot.
func (h *Handler) handleLogs(writer http.ResponseWriter, request *http.Request) {
	limit, _ := strconv.Atoi(strings.TrimSpace(request.URL.Query().Get("limit")))
	if h.logs == nil {
		writeJSON(writer, http.StatusOK, logger.DeveloperLogSnapshot{
			Entries:     []logger.DeveloperLogEntry{},
			GeneratedAt: time.Now(),
		})
		return
	}

	writeJSON(writer, http.StatusOK, h.logs.Snapshot(limit))
}

// handleClearLogs clears persisted developer logs.
func (h *Handler) handleClearLogs(writer http.ResponseWriter, request *http.Request) {
	if h.logs != nil {
		if err := h.logs.Clear(); err != nil {
			writeError(writer, request, http.StatusInternalServerError, err)
			return
		}
	}

	writeJSON(writer, http.StatusOK, map[string]bool{"ok": true})
}

// handleClientLogs accepts developer logs reported by the frontend.
func (h *Handler) handleClientLogs(writer http.ResponseWriter, request *http.Request) {
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
func (h *Handler) handleHot(writer http.ResponseWriter, request *http.Request) {
	if h.hot == nil {
		writeError(writer, request, http.StatusServiceUnavailable, &apiError{message: "Hot service is unavailable"})
		return
	}

	category := core.HotCategory(strings.TrimSpace(request.URL.Query().Get("category")))
	sortBy := core.HotSort(strings.TrimSpace(request.URL.Query().Get("sort")))
	keyword := strings.TrimSpace(request.URL.Query().Get("q"))
	page, _ := strconv.Atoi(strings.TrimSpace(request.URL.Query().Get("page")))
	pageSize, _ := strconv.Atoi(strings.TrimSpace(request.URL.Query().Get("pageSize")))
	options := hot.HotListOptions{}
	if h.store != nil {
		settings := h.store.CurrentSettings()
		options.CNQuoteSource = settings.CNQuoteSource
		options.HKQuoteSource = settings.HKQuoteSource
		options.USQuoteSource = settings.USQuoteSource
		options.CacheTTL = time.Duration(settings.HotCacheTTLSeconds) * time.Second
	}
	options.BypassCache = parseBoolQuery(request.URL.Query().Get("force"))

	list, err := h.hot.List(request.Context(), category, sortBy, keyword, page, pageSize, options)
	if err != nil {
		writeError(writer, request, http.StatusBadRequest, err)
		return
	}

	writeJSON(writer, http.StatusOK, localizeHotList(requestLocale(request), list))
}

// handleHistory returns historical quotes for the given instrument and time range.
func (h *Handler) handleHistory(writer http.ResponseWriter, request *http.Request) {
	itemID := strings.TrimSpace(request.URL.Query().Get("itemId"))
	interval := core.HistoryInterval(strings.TrimSpace(request.URL.Query().Get("interval")))
	if interval == "" {
		interval = core.HistoryRange1d
	}

	series, err := h.store.ItemHistory(request.Context(), itemID, interval, parseBoolQuery(request.URL.Query().Get("force")))
	if err != nil {
		writeError(writer, request, http.StatusBadRequest, err)
		return
	}

	writeJSON(writer, http.StatusOK, localizeHistorySeries(series, requestLocale(request)))
}

// handleRefresh triggers a full quote refresh.
func (h *Handler) handleRefresh(writer http.ResponseWriter, request *http.Request) {
	snapshot, err := h.store.Refresh(request.Context(), parseBoolQuery(request.URL.Query().Get("force")))
	if err != nil {
		writeError(writer, request, http.StatusInternalServerError, err)
		return
	}

	writeJSON(writer, http.StatusOK, localizeSnapshot(snapshot, requestLocale(request)))
}

// handleRefreshItem refreshes only the specified tracked item.
func (h *Handler) handleRefreshItem(writer http.ResponseWriter, request *http.Request) {
	snapshot, err := h.store.RefreshItem(request.Context(), request.PathValue("id"), parseBoolQuery(request.URL.Query().Get("force")))
	if err != nil {
		writeError(writer, request, http.StatusBadRequest, err)
		return
	}

	writeJSON(writer, http.StatusOK, localizeSnapshot(snapshot, requestLocale(request)))
}

// handleUpdateSettings updates application settings.
func (h *Handler) handleUpdateSettings(writer http.ResponseWriter, request *http.Request) {
	var settings core.AppSettings
	if err := decodeJSON(request, &settings); err != nil {
		writeError(writer, request, http.StatusBadRequest, err)
		return
	}

	snapshot, err := h.store.UpdateSettings(settings)
	if err != nil {
		writeError(writer, request, http.StatusBadRequest, err)
		return
	}
	if h.proxyTransport != nil {
		h.proxyTransport.Update(snapshot.Settings.ProxyMode, snapshot.Settings.ProxyURL)
	}

	writeJSON(writer, http.StatusOK, localizeSnapshot(snapshot, requestLocale(request)))
}

// handleCreateItem creates a new tracked item (watch-only or held position).
func (h *Handler) handleCreateItem(writer http.ResponseWriter, request *http.Request) {
	var item core.WatchlistItem
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
func (h *Handler) handleUpdateItem(writer http.ResponseWriter, request *http.Request) {
	var item core.WatchlistItem
	if err := decodeJSON(request, &item); err != nil {
		writeError(writer, request, http.StatusBadRequest, err)
		return
	}

	item.ID = request.PathValue("id")
	snapshot, err := h.store.UpsertItem(item)
	if err != nil {
		writeError(writer, request, http.StatusBadRequest, err)
		return
	}

	writeJSON(writer, http.StatusOK, localizeSnapshot(snapshot, requestLocale(request)))
}

// handleDeleteItem deletes the tracked item with the given ID.
func (h *Handler) handleDeleteItem(writer http.ResponseWriter, request *http.Request) {
	snapshot, err := h.store.DeleteItem(request.PathValue("id"))
	if err != nil {
		writeError(writer, request, http.StatusBadRequest, err)
		return
	}

	writeJSON(writer, http.StatusOK, localizeSnapshot(snapshot, requestLocale(request)))
}

// handlePinItem updates the pinned state of the tracked item with the given ID.
func (h *Handler) handlePinItem(writer http.ResponseWriter, request *http.Request) {
	var payload pinItemRequest
	if err := decodeJSON(request, &payload); err != nil {
		writeError(writer, request, http.StatusBadRequest, err)
		return
	}

	snapshot, err := h.store.SetItemPinned(request.PathValue("id"), payload.Pinned)
	if err != nil {
		writeError(writer, request, http.StatusBadRequest, err)
		return
	}

	writeJSON(writer, http.StatusOK, localizeSnapshot(snapshot, requestLocale(request)))
}

// handleCreateAlert creates a new price alert.
func (h *Handler) handleCreateAlert(writer http.ResponseWriter, request *http.Request) {
	var alert core.AlertRule
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
func (h *Handler) handleUpdateAlert(writer http.ResponseWriter, request *http.Request) {
	var alert core.AlertRule
	if err := decodeJSON(request, &alert); err != nil {
		writeError(writer, request, http.StatusBadRequest, err)
		return
	}

	alert.ID = request.PathValue("id")
	snapshot, err := h.store.UpsertAlert(alert)
	if err != nil {
		writeError(writer, request, http.StatusBadRequest, err)
		return
	}

	writeJSON(writer, http.StatusOK, localizeSnapshot(snapshot, requestLocale(request)))
}

// handleDeleteAlert deletes the price alert with the given ID.
func (h *Handler) handleDeleteAlert(writer http.ResponseWriter, request *http.Request) {
	snapshot, err := h.store.DeleteAlert(request.PathValue("id"))
	if err != nil {
		writeError(writer, request, http.StatusBadRequest, err)
		return
	}

	writeJSON(writer, http.StatusOK, localizeSnapshot(snapshot, requestLocale(request)))
}
