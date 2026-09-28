package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"golang.org/x/sys/unix"
)

// Notifier defines the interface for delivering log messages to a notification sink.
type Notifier interface {
	NotifyLog(level slog.Level, title, message string)
}

// Logger defines a common logging interface for phalune components.
type Logger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
	Log(ctx context.Context, level slog.Level, msg string, args ...any)
	With(args ...any) *slog.Logger
}

// Config specifies the logging configuration.
type Config struct {
	Level        string
	ConsoleLevel string
	Notify       bool
	NotifyLevel  string
}

// Handler is a custom slog.Handler that routes log records to both console output
// and desktop notifications with independently configurable log levels.
type Handler struct {
	console       slog.Handler
	consoleLevel  *slog.LevelVar
	notifyLevel   *slog.LevelVar
	notifyEnabled *atomic.Bool
	notifier      *atomic.Pointer[Notifier]
	attrs         []slog.Attr
	groups        []string
}

var (
	defaultHandler *Handler
	handlerMu      sync.RWMutex
)

// ParseLevel parses a level string into a slog.Level.
func ParseLevel(s string) (slog.Level, error) {
	clean := strings.ToLower(strings.TrimSpace(s))
	switch clean {
	case "debug":
		return slog.LevelDebug, nil
	case "info", "":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		var l slog.Level
		if err := l.UnmarshalText([]byte(clean)); err == nil {
			return l, nil
		}
		return slog.LevelInfo, fmt.Errorf("unknown log level: %q (valid: debug, info, warn, error)", s)
	}
}

func isTerminal(w io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return false
	}
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	_, err := unix.IoctlGetTermios(int(f.Fd()), unix.TCGETS)
	return err == nil
}

type terminalHandler struct {
	w      io.Writer
	mu     *sync.Mutex
	level  *slog.LevelVar
	isTerm bool
	attrs  []slog.Attr
	groups []string
}

func newTerminalHandler(w io.Writer, level *slog.LevelVar) *terminalHandler {
	return &terminalHandler{
		w:      w,
		mu:     new(sync.Mutex),
		level:  level,
		isTerm: isTerminal(w),
	}
}

func (t *terminalHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= t.level.Level()
}

func (t *terminalHandler) Handle(ctx context.Context, r slog.Record) error {
	var buf strings.Builder

	// 1. Short timestamp: 15:04:05 (dimmed on terminal)
	timeStr := r.Time.Format("15:04:05")
	if t.isTerm {
		buf.WriteString("\033[90m")
		buf.WriteString(timeStr)
		buf.WriteString("\033[0m ")
	} else {
		buf.WriteString(timeStr)
		buf.WriteByte(' ')
	}

	// 2. Level Badge (colored on terminal)
	if t.isTerm {
		switch {
		case r.Level >= slog.LevelError:
			buf.WriteString("\033[1;31mERR\033[0m ")
		case r.Level >= slog.LevelWarn:
			buf.WriteString("\033[33mWRN\033[0m ")
		case r.Level >= slog.LevelInfo:
			buf.WriteString("\033[36mINF\033[0m ")
		default:
			buf.WriteString("\033[35mDBG\033[0m ")
		}
	} else {
		switch {
		case r.Level >= slog.LevelError:
			buf.WriteString("ERR ")
		case r.Level >= slog.LevelWarn:
			buf.WriteString("WRN ")
		case r.Level >= slog.LevelInfo:
			buf.WriteString("INF ")
		default:
			buf.WriteString("DBG ")
		}
	}

	// 3. Message
	buf.WriteString(r.Message)

	// 4. Attributes
	prefix := ""
	if len(t.groups) > 0 {
		prefix = strings.Join(t.groups, ".") + "."
	}

	writeAttr := func(a slog.Attr) {
		if a.Key == "" {
			return
		}
		buf.WriteByte(' ')
		if t.isTerm {
			buf.WriteString("\033[90m")
			buf.WriteString(prefix)
			buf.WriteString(a.Key)
			buf.WriteString("=\033[0m")
		} else {
			buf.WriteString(prefix)
			buf.WriteString(a.Key)
			buf.WriteByte('=')
		}
		valStr := formatAttrValue(a.Value.Resolve())
		buf.WriteString(valStr)
	}

	for _, a := range t.attrs {
		writeAttr(a)
	}
	r.Attrs(func(a slog.Attr) bool {
		writeAttr(a)
		return true
	})

	buf.WriteByte('\n')

	t.mu.Lock()
	defer t.mu.Unlock()
	_, err := t.w.Write([]byte(buf.String()))
	return err
}

func formatAttrValue(v slog.Value) string {
	s := v.String()
	if strings.ContainsAny(s, " \t\n\r\"=") || s == "" {
		return strconv.Quote(s)
	}
	return s
}

func (t *terminalHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &terminalHandler{
		w:      t.w,
		mu:     t.mu,
		level:  t.level,
		isTerm: t.isTerm,
		attrs:  append(slices.Clone(t.attrs), attrs...),
		groups: slices.Clone(t.groups),
	}
}

func (t *terminalHandler) WithGroup(name string) slog.Handler {
	return &terminalHandler{
		w:      t.w,
		mu:     t.mu,
		level:  t.level,
		isTerm: t.isTerm,
		attrs:  slices.Clone(t.attrs),
		groups: append(slices.Clone(t.groups), name),
	}
}

// NewHandler creates a new custom Handler with the given configuration and output writer.
func NewHandler(cfg Config, w io.Writer) *Handler {
	if w == nil {
		w = os.Stderr
	}

	cLevelStr := cfg.ConsoleLevel
	if cLevelStr == "" {
		cLevelStr = cfg.Level
	}
	cLevel, err := ParseLevel(cLevelStr)
	if err != nil {
		cLevel = slog.LevelInfo
	}

	nLevelStr := cfg.NotifyLevel
	if nLevelStr == "" {
		nLevelStr = "warn"
	}
	nLevel, err := ParseLevel(nLevelStr)
	if err != nil {
		nLevel = slog.LevelWarn
	}

	consoleLevelVar := new(slog.LevelVar)
	consoleLevelVar.Set(cLevel)

	notifyLevelVar := new(slog.LevelVar)
	notifyLevelVar.Set(nLevel)

	enabled := new(atomic.Bool)
	enabled.Store(cfg.Notify)

	consoleH := newTerminalHandler(w, consoleLevelVar)

	return &Handler{
		console:       consoleH,
		consoleLevel:  consoleLevelVar,
		notifyLevel:   notifyLevelVar,
		notifyEnabled: enabled,
		notifier:      new(atomic.Pointer[Notifier]),
	}
}

// Init configures the global slog default logger using NewHandler.
func Init(cfg Config, w io.Writer) *Handler {
	h := NewHandler(cfg, w)
	handlerMu.Lock()
	defaultHandler = h
	handlerMu.Unlock()

	slog.SetDefault(slog.New(h))
	return h
}

func (h *Handler) Enabled(ctx context.Context, level slog.Level) bool {
	if level >= h.consoleLevel.Level() {
		return true
	}
	if h.notifyEnabled.Load() && level >= h.notifyLevel.Level() {
		return true
	}
	return false
}

func (h *Handler) Handle(ctx context.Context, r slog.Record) error {
	if r.Level >= h.consoleLevel.Level() {
		_ = h.console.Handle(ctx, r)
	}

	if h.notifyEnabled.Load() && r.Level >= h.notifyLevel.Level() {
		if nPtr := h.notifier.Load(); nPtr != nil && *nPtr != nil {
			title := fmt.Sprintf("phalune [%s]", r.Level.String())
			body := r.Message

			var details []string
			for _, a := range h.attrs {
				details = append(details, fmt.Sprintf("%s=%v", a.Key, a.Value))
			}
			r.Attrs(func(a slog.Attr) bool {
				details = append(details, fmt.Sprintf("%s=%v", a.Key, a.Value))
				return true
			})
			if len(details) > 0 {
				body = fmt.Sprintf("%s (%s)", r.Message, strings.Join(details, ", "))
			}

			(*nPtr).NotifyLog(r.Level, title, body)
		}
	}
	return nil
}

func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newAttrs := append(slices.Clone(h.attrs), attrs...)
	return &Handler{
		console:       h.console.WithAttrs(attrs),
		consoleLevel:  h.consoleLevel,
		notifyLevel:   h.notifyLevel,
		notifyEnabled: h.notifyEnabled,
		notifier:      h.notifier,
		attrs:         newAttrs,
		groups:        slices.Clone(h.groups),
	}
}

func (h *Handler) WithGroup(name string) slog.Handler {
	newGroups := append(slices.Clone(h.groups), name)
	return &Handler{
		console:       h.console.WithGroup(name),
		consoleLevel:  h.consoleLevel,
		notifyLevel:   h.notifyLevel,
		notifyEnabled: h.notifyEnabled,
		notifier:      h.notifier,
		attrs:         slices.Clone(h.attrs),
		groups:        newGroups,
	}
}

func (h *Handler) SetNotifier(n Notifier) {
	if n == nil {
		h.notifier.Store(nil)
	} else {
		h.notifier.Store(&n)
	}
}

func (h *Handler) SetConsoleLevel(level slog.Level) {
	h.consoleLevel.Set(level)
}

func (h *Handler) ConsoleLevel() slog.Level {
	return h.consoleLevel.Level()
}

func (h *Handler) SetNotifyLevel(level slog.Level) {
	h.notifyLevel.Set(level)
}

func (h *Handler) NotifyLevel() slog.Level {
	return h.notifyLevel.Level()
}

func (h *Handler) SetNotifyEnabled(enabled bool) {
	h.notifyEnabled.Store(enabled)
}

func (h *Handler) IsNotifyEnabled() bool {
	return h.notifyEnabled.Load()
}

// Global accessor functions delegating to defaultHandler.

func SetNotifier(n Notifier) {
	handlerMu.RLock()
	defer handlerMu.RUnlock()
	if defaultHandler != nil {
		defaultHandler.SetNotifier(n)
	}
}

func SetConsoleLevel(level slog.Level) {
	handlerMu.RLock()
	defer handlerMu.RUnlock()
	if defaultHandler != nil {
		defaultHandler.SetConsoleLevel(level)
	}
}

func ConsoleLevel() slog.Level {
	handlerMu.RLock()
	defer handlerMu.RUnlock()
	if defaultHandler != nil {
		return defaultHandler.ConsoleLevel()
	}
	return slog.LevelInfo
}

func SetNotifyLevel(level slog.Level) {
	handlerMu.RLock()
	defer handlerMu.RUnlock()
	if defaultHandler != nil {
		defaultHandler.SetNotifyLevel(level)
	}
}

func NotifyLevel() slog.Level {
	handlerMu.RLock()
	defer handlerMu.RUnlock()
	if defaultHandler != nil {
		return defaultHandler.NotifyLevel()
	}
	return slog.LevelWarn
}

func SetNotifyEnabled(enabled bool) {
	handlerMu.RLock()
	defer handlerMu.RUnlock()
	if defaultHandler != nil {
		defaultHandler.SetNotifyEnabled(enabled)
	}
}

func IsNotifyEnabled() bool {
	handlerMu.RLock()
	defer handlerMu.RUnlock()
	if defaultHandler != nil {
		return defaultHandler.IsNotifyEnabled()
	}
	return false
}

// Convenience package-level logging functions.
func Debug(msg string, args ...any) { slog.Debug(msg, args...) }
func Info(msg string, args ...any)  { slog.Info(msg, args...) }
func Warn(msg string, args ...any)  { slog.Warn(msg, args...) }
func Error(msg string, args ...any) { slog.Error(msg, args...) }
