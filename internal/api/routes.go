package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"investgo/internal/monitor"
)

type route struct {
	method  string
	pattern string
	handler routeHandler
}

// routeHandler 定义了带路径参数的路由处理函数签名。
type routeHandler func(http.ResponseWriter, *http.Request, routeParams)

// routeParams 保存从路由模板中提取出来的路径参数。
type routeParams map[string]string

// Value 返回指定名称的路径参数值。
func (params routeParams) Value(name string) string {
	return params[name]
}

// registerRoutes 统一注册 API 路由。
func (h *Handler) registerRoutes() []route {
	return []route{
		{method: http.MethodGet, pattern: "/state", handler: h.handleState},
		{method: http.MethodGet, pattern: "/logs", handler: h.handleLogs},
		{method: http.MethodDelete, pattern: "/logs", handler: h.handleClearLogs},
		{method: http.MethodPost, pattern: "/client-logs", handler: h.handleClientLogs},
		{method: http.MethodGet, pattern: "/hot", handler: h.handleHot},
		{method: http.MethodGet, pattern: "/history", handler: h.handleHistory},
		{method: http.MethodPost, pattern: "/refresh", handler: h.handleRefresh},
		{method: http.MethodPut, pattern: "/settings", handler: h.handleUpdateSettings},
		{method: http.MethodPost, pattern: "/items", handler: h.handleCreateItem},
		{method: http.MethodPut, pattern: "/items/{id}", handler: h.handleUpdateItem},
		{method: http.MethodDelete, pattern: "/items/{id}", handler: h.handleDeleteItem},
		{method: http.MethodPost, pattern: "/alerts", handler: h.handleCreateAlert},
		{method: http.MethodPut, pattern: "/alerts/{id}", handler: h.handleUpdateAlert},
		{method: http.MethodDelete, pattern: "/alerts/{id}", handler: h.handleDeleteAlert},
	}
}

// match 判断当前路由是否匹配给定的请求方法和路径。
func (r route) match(method, path string) (routeParams, bool) {
	if method != r.method {
		return nil, false
	}
	return matchRoutePattern(r.pattern, path)
}

// matchRoutePattern 按简单的 `{name}` 单段参数规则匹配路径模板。
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

// routeSegments 把路径按层级拆分成匹配器使用的段列表。
func routeSegments(path string) []string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "/")
}

// handleState 返回前端当前依赖的完整状态快照。
func (h *Handler) handleState(writer http.ResponseWriter, _ *http.Request, _ routeParams) {
	writeJSON(writer, http.StatusOK, h.store.Snapshot())
}

// handleLogs 返回开发日志快照。
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

// handleClearLogs 清空已持久化的开发日志。
func (h *Handler) handleClearLogs(writer http.ResponseWriter, _ *http.Request, _ routeParams) {
	if h.logs != nil {
		if err := h.logs.Clear(); err != nil {
			writeError(writer, http.StatusInternalServerError, err)
			return
		}
	}

	writeJSON(writer, http.StatusOK, map[string]bool{"ok": true})
}

// handleClientLogs 接收前端上报的开发日志。
func (h *Handler) handleClientLogs(writer http.ResponseWriter, request *http.Request, _ routeParams) {
	if h.logs == nil {
		writeJSON(writer, http.StatusOK, map[string]bool{"ok": true})
		return
	}

	var payload clientLogRequest
	if err := decodeJSON(request, &payload); err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}

	h.logs.Log(payload.Source, payload.Scope, sanitiseDeveloperLogLevel(payload.Level), payload.Message)
	writeJSON(writer, http.StatusOK, map[string]bool{"ok": true})
}

// handleHot 返回指定分类和排序方式的热门榜单。
func (h *Handler) handleHot(writer http.ResponseWriter, request *http.Request, _ routeParams) {
	if h.hot == nil {
		writeError(writer, http.StatusServiceUnavailable, &apiError{message: "热门服务不可用"})
		return
	}

	category := monitor.HotCategory(strings.TrimSpace(request.URL.Query().Get("category")))
	sortBy := monitor.HotSort(strings.TrimSpace(request.URL.Query().Get("sort")))
	keyword := strings.TrimSpace(request.URL.Query().Get("q"))
	page, _ := strconv.Atoi(strings.TrimSpace(request.URL.Query().Get("page")))
	pageSize, _ := strconv.Atoi(strings.TrimSpace(request.URL.Query().Get("pageSize")))
	list, err := h.hot.List(request.Context(), category, sortBy, keyword, page, pageSize)
	if err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}

	writeJSON(writer, http.StatusOK, list)
}

// handleHistory 返回指定标的和时间区间的历史行情。
func (h *Handler) handleHistory(writer http.ResponseWriter, request *http.Request, _ routeParams) {
	itemID := strings.TrimSpace(request.URL.Query().Get("itemId"))
	interval := monitor.HistoryInterval(strings.TrimSpace(request.URL.Query().Get("interval")))
	if interval == "" {
		interval = monitor.HistoryRange1d
	}

	series, err := h.store.ItemHistory(request.Context(), itemID, interval)
	if err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}

	writeJSON(writer, http.StatusOK, series)
}

// handleRefresh 触发一次全量行情刷新。
func (h *Handler) handleRefresh(writer http.ResponseWriter, request *http.Request, _ routeParams) {
	snapshot, err := h.store.Refresh(request.Context())
	if err != nil {
		writeError(writer, http.StatusInternalServerError, err)
		return
	}

	writeJSON(writer, http.StatusOK, snapshot)
}

// handleUpdateSettings 更新应用设置。
func (h *Handler) handleUpdateSettings(writer http.ResponseWriter, request *http.Request, _ routeParams) {
	var settings monitor.AppSettings
	if err := decodeJSON(request, &settings); err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}

	snapshot, err := h.store.UpdateSettings(settings)
	if err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}

	writeJSON(writer, http.StatusOK, snapshot)
}

// handleCreateItem 新增一个自选标的。
func (h *Handler) handleCreateItem(writer http.ResponseWriter, request *http.Request, _ routeParams) {
	var item monitor.WatchlistItem
	if err := decodeJSON(request, &item); err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}

	snapshot, err := h.store.UpsertItem(item)
	if err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}

	writeJSON(writer, http.StatusOK, snapshot)
}

// handleUpdateItem 更新指定 ID 的自选标的。
func (h *Handler) handleUpdateItem(writer http.ResponseWriter, request *http.Request, params routeParams) {
	var item monitor.WatchlistItem
	if err := decodeJSON(request, &item); err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}

	item.ID = params.Value("id")
	snapshot, err := h.store.UpsertItem(item)
	if err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}

	writeJSON(writer, http.StatusOK, snapshot)
}

// handleDeleteItem 删除指定 ID 的自选标的。
func (h *Handler) handleDeleteItem(writer http.ResponseWriter, _ *http.Request, params routeParams) {
	snapshot, err := h.store.DeleteItem(params.Value("id"))
	if err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}

	writeJSON(writer, http.StatusOK, snapshot)
}

// handleCreateAlert 新增一个价格提醒。
func (h *Handler) handleCreateAlert(writer http.ResponseWriter, request *http.Request, _ routeParams) {
	var alert monitor.AlertRule
	if err := decodeJSON(request, &alert); err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}

	snapshot, err := h.store.UpsertAlert(alert)
	if err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}

	writeJSON(writer, http.StatusOK, snapshot)
}

// handleUpdateAlert 更新指定 ID 的价格提醒。
func (h *Handler) handleUpdateAlert(writer http.ResponseWriter, request *http.Request, params routeParams) {
	var alert monitor.AlertRule
	if err := decodeJSON(request, &alert); err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}

	alert.ID = params.Value("id")
	snapshot, err := h.store.UpsertAlert(alert)
	if err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}

	writeJSON(writer, http.StatusOK, snapshot)
}

// handleDeleteAlert 删除指定 ID 的价格提醒。
func (h *Handler) handleDeleteAlert(writer http.ResponseWriter, _ *http.Request, params routeParams) {
	snapshot, err := h.store.DeleteAlert(params.Value("id"))
	if err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}

	writeJSON(writer, http.StatusOK, snapshot)
}
