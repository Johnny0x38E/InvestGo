package monitor

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type DeveloperLogLevel string

const (
	DeveloperLogDebug DeveloperLogLevel = "debug"
	DeveloperLogInfo  DeveloperLogLevel = "info"
	DeveloperLogWarn  DeveloperLogLevel = "warn"
	DeveloperLogError DeveloperLogLevel = "error"
)

type DeveloperLogEntry struct {
	ID        string            `json:"id"`
	Source    string            `json:"source"`
	Scope     string            `json:"scope"`
	Level     DeveloperLogLevel `json:"level"`
	Message   string            `json:"message"`
	Timestamp time.Time         `json:"timestamp"`
}

type DeveloperLogSnapshot struct {
	Entries     []DeveloperLogEntry `json:"entries"`
	LogFilePath string              `json:"logFilePath"`
	GeneratedAt time.Time           `json:"generatedAt"`
}

// LogBook 负责聚合内存日志、文件日志和终端日志输出。
type LogBook struct {
	mu         sync.RWMutex
	entries    []DeveloperLogEntry
	maxEntries int
	sequence   atomic.Uint64
	file       *os.File
	filePath   string
	console    io.Writer
}

// NewLogBook 创建一个带固定容量的日志簿。
func NewLogBook(maxEntries int) *LogBook {
	if maxEntries <= 0 {
		maxEntries = 200
	}

	return &LogBook{
		entries:    make([]DeveloperLogEntry, 0, maxEntries),
		maxEntries: maxEntries,
	}
}

// ConfigureFile 配置日志文件输出目标。
func (b *LogBook) ConfigureFile(path string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if b.file != nil {
		_ = b.file.Close()
	}
	b.file = file
	b.filePath = path
	return nil
}

// Close 关闭当前日志文件句柄。
func (b *LogBook) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.file == nil {
		return nil
	}

	err := b.file.Close()
	b.file = nil
	return err
}

// Snapshot 返回带文件路径信息的日志快照。
func (b *LogBook) Snapshot(limit int) DeveloperLogSnapshot {
	return DeveloperLogSnapshot{
		Entries:     b.Entries(limit),
		LogFilePath: b.FilePath(),
		GeneratedAt: time.Now(),
	}
}

// Entries 返回最新的若干条日志记录，按时间倒序排列。
func (b *LogBook) Entries(limit int) []DeveloperLogEntry {
	b.mu.RLock()
	defer b.mu.RUnlock()

	total := len(b.entries)
	if total == 0 {
		return []DeveloperLogEntry{}
	}

	if limit <= 0 || limit > total {
		limit = total
	}

	result := make([]DeveloperLogEntry, 0, limit)
	for idx := total - 1; idx >= 0 && len(result) < limit; idx-- {
		result = append(result, b.entries[idx])
	}
	return result
}

// FilePath 返回当前日志文件路径。
func (b *LogBook) FilePath() string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.filePath
}

// Clear 清空内存日志，并在文件输出启用时清空日志文件内容。
func (b *LogBook) Clear() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.entries = b.entries[:0]
	if b.file == nil {
		return nil
	}

	if err := b.file.Truncate(0); err != nil {
		return err
	}
	_, err := b.file.Seek(0, io.SeekStart)
	return err
}

// EnableConsole 启用终端日志输出。
func (b *LogBook) EnableConsole(writer io.Writer) {
	if b == nil {
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	b.console = writer
}

// Log 写入一条开发日志，并同步到内存、文件和终端输出。
func (b *LogBook) Log(source, scope string, level DeveloperLogLevel, message string) {
	if b == nil {
		return
	}

	clean := strings.TrimSpace(message)
	if clean == "" {
		return
	}

	entry := DeveloperLogEntry{
		ID:        fmt.Sprintf("log-%06d", b.sequence.Add(1)),
		Source:    defaultString(strings.TrimSpace(source), "backend"),
		Scope:     defaultString(strings.TrimSpace(scope), "app"),
		Level:     level,
		Message:   clean,
		Timestamp: time.Now(),
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	// 内存日志采用固定长度环形覆盖策略，避免无限增长。
	if len(b.entries) == b.maxEntries {
		copy(b.entries, b.entries[1:])
		b.entries[len(b.entries)-1] = entry
	} else {
		b.entries = append(b.entries, entry)
	}

	if b.file != nil {
		_, _ = fmt.Fprintf(
			b.file,
			"%s [%s] %s/%s %s\n",
			entry.Timestamp.Format(time.RFC3339),
			strings.ToUpper(string(entry.Level)),
			entry.Source,
			entry.Scope,
			entry.Message,
		)
	}

	if b.console != nil {
		_, _ = fmt.Fprintf(
			b.console,
			"%s [%s] %s/%s %s\n",
			entry.Timestamp.Format(time.RFC3339),
			strings.ToUpper(string(entry.Level)),
			entry.Source,
			entry.Scope,
			entry.Message,
		)
	}
}

// Debug 写入一条 debug 级别日志。
func (b *LogBook) Debug(source, scope, message string) {
	b.Log(source, scope, DeveloperLogDebug, message)
}

// Info 写入一条 info 级别日志。
func (b *LogBook) Info(source, scope, message string) {
	b.Log(source, scope, DeveloperLogInfo, message)
}

// Warn 写入一条 warn 级别日志。
func (b *LogBook) Warn(source, scope, message string) {
	b.Log(source, scope, DeveloperLogWarn, message)
}

// Error 写入一条 error 级别日志。
func (b *LogBook) Error(source, scope, message string) {
	b.Log(source, scope, DeveloperLogError, message)
}

// Writer 返回一个可桥接标准 io.Writer 的日志写入器。
func (b *LogBook) Writer(source, scope string, level DeveloperLogLevel) io.Writer {
	return &logBookWriter{
		book:   b,
		source: source,
		scope:  scope,
		level:  level,
	}
}

// NewSlogLogger 返回一个把 slog 记录转发到日志簿的 logger。
func (b *LogBook) NewSlogLogger(source string, level slog.Level) *slog.Logger {
	levelVar := new(slog.LevelVar)
	levelVar.Set(level)
	return slog.New(&logBookHandler{
		book:   b,
		source: defaultString(strings.TrimSpace(source), "system"),
		level:  levelVar,
	})
}

type logBookWriter struct {
	book   *LogBook
	source string
	scope  string
	level  DeveloperLogLevel
}

// Write 实现 io.Writer 接口，并把每一行转成日志记录。
func (w *logBookWriter) Write(payload []byte) (int, error) {
	message := strings.TrimSpace(string(payload))
	if message == "" {
		return len(payload), nil
	}

	for line := range strings.SplitSeq(message, "\n") {
		clean := strings.TrimSpace(line)
		if clean == "" {
			continue
		}
		w.book.Log(w.source, w.scope, w.level, clean)
	}

	return len(payload), nil
}

type logBookHandler struct {
	book   *LogBook
	source string
	level  *slog.LevelVar
	attrs  []slog.Attr
	groups []string
}

// Enabled 返回指定 slog 级别是否应被记录。
func (h *logBookHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level.Level()
}

// Handle 把一条 slog 记录转换为开发日志格式后写入日志簿。
func (h *logBookHandler) Handle(_ context.Context, record slog.Record) error {
	parts := make([]string, 0, 8)
	for _, attr := range h.attrs {
		appendLogAttr(&parts, h.groups, attr)
	}
	record.Attrs(func(attr slog.Attr) bool {
		appendLogAttr(&parts, h.groups, attr)
		return true
	})

	message := strings.TrimSpace(record.Message)
	if len(parts) > 0 {
		message = strings.TrimSpace(message + " | " + strings.Join(parts, " "))
	}

	h.book.Log(h.source, "wails", slogLevelToDeveloperLevel(record.Level), message)
	return nil
}

// WithAttrs 返回一个附加静态属性的新 handler。
func (h *logBookHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := *h
	next.attrs = append(append([]slog.Attr(nil), h.attrs...), attrs...)
	return &next
}

// WithGroup 返回一个附加属性分组的新 handler。
func (h *logBookHandler) WithGroup(name string) slog.Handler {
	next := *h
	next.groups = append(append([]string(nil), h.groups...), name)
	return &next
}

// appendLogAttr 把 slog 属性展开为适合写入日志文本的键值片段。
func appendLogAttr(parts *[]string, groups []string, attr slog.Attr) {
	attr.Value = attr.Value.Resolve()
	if attr.Key == "" && attr.Value.Kind() == slog.KindAny && attr.Value.Any() == nil {
		return
	}

	// slog 的 group 需要拍平成扁平键路径，避免嵌套结构在日志里丢失语义。
	if attr.Value.Kind() == slog.KindGroup {
		nextGroups := groups
		if attr.Key != "" {
			nextGroups = append(append([]string(nil), groups...), attr.Key)
		}
		for _, child := range attr.Value.Group() {
			appendLogAttr(parts, nextGroups, child)
		}
		return
	}

	keyParts := append([]string(nil), groups...)
	if attr.Key != "" {
		keyParts = append(keyParts, attr.Key)
	}
	key := strings.Join(keyParts, ".")
	if key == "" {
		key = "attr"
	}

	*parts = append(*parts, fmt.Sprintf("%s=%s", key, slogValueString(attr.Value)))
}

// slogValueString 把 slog.Value 转成可读字符串表示。
func slogValueString(value slog.Value) string {
	switch value.Kind() {
	case slog.KindString:
		return value.String()
	case slog.KindBool:
		if value.Bool() {
			return "true"
		}
		return "false"
	case slog.KindInt64:
		return fmt.Sprintf("%d", value.Int64())
	case slog.KindUint64:
		return fmt.Sprintf("%d", value.Uint64())
	case slog.KindFloat64:
		return fmt.Sprintf("%g", value.Float64())
	case slog.KindDuration:
		return value.Duration().String()
	case slog.KindTime:
		return value.Time().Format(time.RFC3339)
	default:
		return fmt.Sprint(value.Any())
	}
}

// slogLevelToDeveloperLevel 把 slog 级别映射为应用内部日志级别。
func slogLevelToDeveloperLevel(level slog.Level) DeveloperLogLevel {
	switch {
	case level < slog.LevelInfo:
		return DeveloperLogDebug
	case level < slog.LevelWarn:
		return DeveloperLogInfo
	case level < slog.LevelError:
		return DeveloperLogWarn
	default:
		return DeveloperLogError
	}
}

// defaultString 在值为空时返回给定的回退值。
func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
