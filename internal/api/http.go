package api

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"investgo/internal/marketdata"
	"investgo/internal/monitor"
)

// Handler 负责承接 `/api/*` 请求并协调各个后端服务。
type Handler struct {
	store  *monitor.Store
	hot    *marketdata.HotService
	logs   *monitor.LogBook
	routes []route
}

// clientLogRequest 定义了前端发送日志请求的 JSON 结构。
type clientLogRequest struct {
	Source  string                    `json:"source"`
	Scope   string                    `json:"scope"`
	Level   monitor.DeveloperLogLevel `json:"level"`
	Message string                    `json:"message"`
}

type openExternalRequest struct {
	URL string `json:"url"`
}

// NewHandler 返回统一的 API 处理器。
func NewHandler(store *monitor.Store, hot *marketdata.HotService, logs *monitor.LogBook) *Handler {
	handler := &Handler{
		store: store,
		hot:   hot,
		logs:  logs,
	}
	handler.routes = handler.registerRoutes()
	return handler
}

// ServeHTTP 负责统一裁剪 `/api` 前缀，并按已注册路由分发请求。
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

	writeError(writer, http.StatusNotFound, errNotFound(path))
}

// trimAPIPath 把 Wails 注册的 `/api` 前缀裁剪成内部路由使用的相对路径。
func trimAPIPath(path string) string {
	trimmed := strings.TrimPrefix(path, "/api")
	if trimmed == "" {
		return "/"
	}
	return trimmed
}

// decodeJSON 把请求体反序列化到目标对象，并负责关闭请求体。
func decodeJSON(request *http.Request, target any) error {
	defer request.Body.Close()
	return json.NewDecoder(request.Body).Decode(target)
}

// writeJSON 按指定状态码输出 JSON 响应。
func writeJSON(writer http.ResponseWriter, status int, payload any) {
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(payload)
}

// writeError 把错误统一编码成 `{error: ...}` 结构输出。
func writeError(writer http.ResponseWriter, status int, err error) {
	writeJSON(writer, status, map[string]string{
		"error": err.Error(),
	})
}

// errNotFound 返回接口不存在时使用的错误对象。
func errNotFound(path string) error {
	return &apiError{message: "接口不存在: " + path}
}

// sanitiseDeveloperLogLevel 把未知日志级别回落为 info。
func sanitiseDeveloperLogLevel(level monitor.DeveloperLogLevel) monitor.DeveloperLogLevel {
	switch level {
	case monitor.DeveloperLogDebug, monitor.DeveloperLogInfo, monitor.DeveloperLogWarn, monitor.DeveloperLogError:
		return level
	default:
		return monitor.DeveloperLogInfo
	}
}

func sanitiseExternalURL(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", &apiError{message: "链接不能为空"}
	}

	parsed, err := url.Parse(value)
	if err != nil {
		return "", &apiError{message: "链接格式无效"}
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return "", &apiError{message: "仅支持 http/https 链接"}
	}
	if parsed.Host == "" {
		return "", &apiError{message: "链接缺少主机名"}
	}

	return parsed.String(), nil
}

// apiError 表示 API 层内部构造的响应错误。
type apiError struct {
	message string
}

// Error 实现 error 接口。
func (e *apiError) Error() string {
	return e.message
}
