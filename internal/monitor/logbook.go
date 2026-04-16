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

// LogBook is responsible for aggregating in-memory logs, file logs, and terminal log output.
type LogBook struct {
	mu         sync.RWMutex
	entries    []DeveloperLogEntry
	maxEntries int
	sequence   atomic.Uint64
	file       *os.File
	filePath   string
	console    io.Writer
}

// NewLogBook creates a logbook with a fixed capacity.
func NewLogBook(maxEntries int) *LogBook {
	if maxEntries <= 0 {
		maxEntries = 200
	}

	return &LogBook{
		entries:    make([]DeveloperLogEntry, 0, maxEntries),
		maxEntries: maxEntries,
	}
}

// ConfigureFile configures the log file output destination.
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

// Close closes the current log file handle.
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

// Snapshot returns a log snapshot with file path information.
func (b *LogBook) Snapshot(limit int) DeveloperLogSnapshot {
	return DeveloperLogSnapshot{
		Entries:     b.Entries(limit),
		LogFilePath: b.FilePath(),
		GeneratedAt: time.Now(),
	}
}

// Entries returns the latest log records, sorted in reverse chronological order.
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

// FilePath returns the current log file path.
func (b *LogBook) FilePath() string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.filePath
}

// Clear clears in-memory logs and clears log file contents when file output is enabled.
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

// EnableConsole enables terminal log output.
func (b *LogBook) EnableConsole(writer io.Writer) {
	if b == nil {
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	b.console = writer
}

// Log writes a development log entry and synchronizes to memory, file, and terminal output.
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

	// In-memory logs use a fixed-length circular overwrite strategy to avoid unbounded growth.
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

// Debug writes a debug level log entry.
func (b *LogBook) Debug(source, scope, message string) {
	b.Log(source, scope, DeveloperLogDebug, message)
}

// Info writes an info level log entry.
func (b *LogBook) Info(source, scope, message string) {
	b.Log(source, scope, DeveloperLogInfo, message)
}

// Warn writes a warn level log entry.
func (b *LogBook) Warn(source, scope, message string) {
	b.Log(source, scope, DeveloperLogWarn, message)
}

// Error writes an error level log entry.
func (b *LogBook) Error(source, scope, message string) {
	b.Log(source, scope, DeveloperLogError, message)
}

// Writer returns a log writer that can bridge to standard io.Writer.
func (b *LogBook) Writer(source, scope string, level DeveloperLogLevel) io.Writer {
	return &logBookWriter{
		book:   b,
		source: source,
		scope:  scope,
		level:  level,
	}
}

// NewSlogLogger returns a logger that forwards slog records to the logbook.
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

// Write implements io.Writer interface and converts each line to a log record.
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

// Enabled returns whether the specified slog level should be logged.
func (h *logBookHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level.Level()
}

// Handle converts a slog record to development log format and writes to the logbook.
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

// WithAttrs returns a new handler with attached static attributes.
func (h *logBookHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := *h
	next.attrs = append(append([]slog.Attr(nil), h.attrs...), attrs...)
	return &next
}

// WithGroup returns a new handler with attached attribute groups.
func (h *logBookHandler) WithGroup(name string) slog.Handler {
	next := *h
	next.groups = append(append([]string(nil), h.groups...), name)
	return &next
}

// appendLogAttr expands slog attributes into key-value fragments suitable for writing to log text.
func appendLogAttr(parts *[]string, groups []string, attr slog.Attr) {
	attr.Value = attr.Value.Resolve()
	if attr.Key == "" && attr.Value.Kind() == slog.KindAny && attr.Value.Any() == nil {
		return
	}

	// slog groups need to be flattened into flat key paths to avoid nested structures losing semantics in logs.
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

// slogValueString converts slog.Value to a readable string representation.
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

// slogLevelToDeveloperLevel maps slog levels to application internal log levels.
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

// defaultString returns the given fallback value when the value is empty.
func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
