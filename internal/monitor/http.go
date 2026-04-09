package monitor

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type API struct {
	store *Store
	hot   *HotService
	logs  *LogBook
}

type clientLogRequest struct {
	Source  string            `json:"source"`
	Scope   string            `json:"scope"`
	Level   DeveloperLogLevel `json:"level"`
	Message string            `json:"message"`
}

// NewAPI 把前端需要的后端能力拼成统一的 HTTP 入口。
func NewAPI(store *Store, hot *HotService, logs *LogBook) *API {
	return &API{store: store, hot: hot, logs: logs}
}

// ServeHTTP 只负责路由和 JSON 编解码，业务规则继续下沉到 Store / HotService。
func (a *API) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")

	path := strings.TrimPrefix(request.URL.Path, "/api")
	switch {
	case path == "/state" && request.Method == http.MethodGet:
		writeJSON(writer, http.StatusOK, a.store.Snapshot())
	case path == "/logs" && request.Method == http.MethodGet:
		limit, _ := strconv.Atoi(strings.TrimSpace(request.URL.Query().Get("limit")))
		if a.logs == nil {
			writeJSON(writer, http.StatusOK, DeveloperLogSnapshot{
				Entries:     []DeveloperLogEntry{},
				GeneratedAt: time.Now(),
			})
			return
		}
		writeJSON(writer, http.StatusOK, a.logs.Snapshot(limit))
	case path == "/logs" && request.Method == http.MethodDelete:
		if a.logs != nil {
			if err := a.logs.Clear(); err != nil {
				writeError(writer, http.StatusInternalServerError, err)
				return
			}
		}
		writeJSON(writer, http.StatusOK, map[string]bool{"ok": true})
	case path == "/client-logs" && request.Method == http.MethodPost:
		if a.logs == nil {
			writeJSON(writer, http.StatusOK, map[string]bool{"ok": true})
			return
		}
		var payload clientLogRequest
		if err := decodeJSON(request, &payload); err != nil {
			writeError(writer, http.StatusBadRequest, err)
			return
		}
		a.logs.Log(payload.Source, payload.Scope, sanitiseDeveloperLogLevel(payload.Level), payload.Message)
		writeJSON(writer, http.StatusOK, map[string]bool{"ok": true})
	case path == "/hot" && request.Method == http.MethodGet:
		if a.hot == nil {
			writeError(writer, http.StatusServiceUnavailable, &apiError{message: "热门服务不可用"})
			return
		}
		category := HotCategory(strings.TrimSpace(request.URL.Query().Get("category")))
		sortBy := HotSort(strings.TrimSpace(request.URL.Query().Get("sort")))
		page, _ := strconv.Atoi(strings.TrimSpace(request.URL.Query().Get("page")))
		pageSize, _ := strconv.Atoi(strings.TrimSpace(request.URL.Query().Get("pageSize")))
		list, err := a.hot.List(request.Context(), category, sortBy, page, pageSize)
		if err != nil {
			writeError(writer, http.StatusBadRequest, err)
			return
		}
		writeJSON(writer, http.StatusOK, list)
	case path == "/history" && request.Method == http.MethodGet:
		itemID := strings.TrimSpace(request.URL.Query().Get("itemId"))
		interval := HistoryInterval(strings.TrimSpace(request.URL.Query().Get("interval")))
		if interval == "" {
			interval = HistoryDay
		}
		series, err := a.store.ItemHistory(request.Context(), itemID, interval)
		if err != nil {
			writeError(writer, http.StatusBadRequest, err)
			return
		}
		writeJSON(writer, http.StatusOK, series)
	case path == "/refresh" && request.Method == http.MethodPost:
		snapshot, err := a.store.Refresh(request.Context())
		if err != nil {
			writeError(writer, http.StatusInternalServerError, err)
			return
		}
		writeJSON(writer, http.StatusOK, snapshot)
	case path == "/settings" && request.Method == http.MethodPut:
		var settings AppSettings
		if err := decodeJSON(request, &settings); err != nil {
			writeError(writer, http.StatusBadRequest, err)
			return
		}
		snapshot, err := a.store.UpdateSettings(settings)
		if err != nil {
			writeError(writer, http.StatusBadRequest, err)
			return
		}
		writeJSON(writer, http.StatusOK, snapshot)
	case path == "/items" && request.Method == http.MethodPost:
		var item WatchlistItem
		if err := decodeJSON(request, &item); err != nil {
			writeError(writer, http.StatusBadRequest, err)
			return
		}
		snapshot, err := a.store.UpsertItem(item)
		if err != nil {
			writeError(writer, http.StatusBadRequest, err)
			return
		}
		writeJSON(writer, http.StatusOK, snapshot)
	case strings.HasPrefix(path, "/items/") && request.Method == http.MethodPut:
		var item WatchlistItem
		if err := decodeJSON(request, &item); err != nil {
			writeError(writer, http.StatusBadRequest, err)
			return
		}
		item.ID = strings.TrimPrefix(path, "/items/")
		snapshot, err := a.store.UpsertItem(item)
		if err != nil {
			writeError(writer, http.StatusBadRequest, err)
			return
		}
		writeJSON(writer, http.StatusOK, snapshot)
	case strings.HasPrefix(path, "/items/") && request.Method == http.MethodDelete:
		snapshot, err := a.store.DeleteItem(strings.TrimPrefix(path, "/items/"))
		if err != nil {
			writeError(writer, http.StatusBadRequest, err)
			return
		}
		writeJSON(writer, http.StatusOK, snapshot)
	case path == "/alerts" && request.Method == http.MethodPost:
		var alert AlertRule
		if err := decodeJSON(request, &alert); err != nil {
			writeError(writer, http.StatusBadRequest, err)
			return
		}
		snapshot, err := a.store.UpsertAlert(alert)
		if err != nil {
			writeError(writer, http.StatusBadRequest, err)
			return
		}
		writeJSON(writer, http.StatusOK, snapshot)
	case strings.HasPrefix(path, "/alerts/") && request.Method == http.MethodPut:
		var alert AlertRule
		if err := decodeJSON(request, &alert); err != nil {
			writeError(writer, http.StatusBadRequest, err)
			return
		}
		alert.ID = strings.TrimPrefix(path, "/alerts/")
		snapshot, err := a.store.UpsertAlert(alert)
		if err != nil {
			writeError(writer, http.StatusBadRequest, err)
			return
		}
		writeJSON(writer, http.StatusOK, snapshot)
	case strings.HasPrefix(path, "/alerts/") && request.Method == http.MethodDelete:
		snapshot, err := a.store.DeleteAlert(strings.TrimPrefix(path, "/alerts/"))
		if err != nil {
			writeError(writer, http.StatusBadRequest, err)
			return
		}
		writeJSON(writer, http.StatusOK, snapshot)
	default:
		writeError(writer, http.StatusNotFound, errNotFound(path))
	}
}

func decodeJSON(request *http.Request, target any) error {
	defer request.Body.Close()
	return json.NewDecoder(request.Body).Decode(target)
}

func writeJSON(writer http.ResponseWriter, status int, payload any) {
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(payload)
}

func writeError(writer http.ResponseWriter, status int, err error) {
	writeJSON(writer, status, map[string]string{
		"error": err.Error(),
	})
}

func errNotFound(path string) error {
	return &apiError{message: "接口不存在: " + path}
}

func sanitiseDeveloperLogLevel(level DeveloperLogLevel) DeveloperLogLevel {
	switch level {
	case DeveloperLogDebug, DeveloperLogInfo, DeveloperLogWarn, DeveloperLogError:
		return level
	default:
		return DeveloperLogInfo
	}
}

type apiError struct {
	message string
}

func (e *apiError) Error() string {
	return e.message
}
