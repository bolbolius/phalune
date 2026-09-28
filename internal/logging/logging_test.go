package logging

import (
	"bytes"
	"log/slog"
	"strings"
	"sync"
	"testing"
)

type mockNotifier struct {
	mu    sync.Mutex
	calls []notificationCall
}

type notificationCall struct {
	Level   slog.Level
	Title   string
	Message string
}

func (m *mockNotifier) NotifyLog(level slog.Level, title, message string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, notificationCall{
		Level:   level,
		Title:   title,
		Message: message,
	})
}

func (m *mockNotifier) getCalls() []notificationCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	res := make([]notificationCall, len(m.calls))
	copy(res, m.calls)
	return res
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected slog.Level
		wantErr  bool
	}{
		{"debug", slog.LevelDebug, false},
		{"DEBUG", slog.LevelDebug, false},
		{"info", slog.LevelInfo, false},
		{"", slog.LevelInfo, false},
		{"warn", slog.LevelWarn, false},
		{"warning", slog.LevelWarn, false},
		{"error", slog.LevelError, false},
		{"invalid", slog.LevelInfo, true},
	}

	for _, tt := range tests {
		lvl, err := ParseLevel(tt.input)
		if tt.wantErr && err == nil {
			t.Errorf("ParseLevel(%q) expected error, got nil", tt.input)
		}
		if !tt.wantErr && err != nil {
			t.Errorf("ParseLevel(%q) unexpected error: %v", tt.input, err)
		}
		if lvl != tt.expected {
			t.Errorf("ParseLevel(%q) = %v, expected %v", tt.input, lvl, tt.expected)
		}
	}
}

func TestSeparateLogLevels(t *testing.T) {
	var buf bytes.Buffer
	notifier := &mockNotifier{}

	cfg := Config{
		ConsoleLevel: "debug",
		Notify:       true,
		NotifyLevel:  "error",
	}

	h := NewHandler(cfg, &buf)
	h.SetNotifier(notifier)
	logger := slog.New(h)

	// 1. Debug message: should appear in console, but NOT in notification
	logger.Debug("debug message", "key", "val")
	if !strings.Contains(buf.String(), "debug message") {
		t.Fatalf("expected debug message in console output, got: %s", buf.String())
	}
	if len(notifier.getCalls()) != 0 {
		t.Fatalf("expected 0 notifications for debug, got %d", len(notifier.getCalls()))
	}
	buf.Reset()

	// 2. Info message: should appear in console, but NOT in notification
	logger.Info("info message")
	if !strings.Contains(buf.String(), "info message") {
		t.Fatalf("expected info message in console output, got: %s", buf.String())
	}
	if len(notifier.getCalls()) != 0 {
		t.Fatalf("expected 0 notifications for info, got %d", len(notifier.getCalls()))
	}
	buf.Reset()

	// 3. Error message: should appear in BOTH console and notification
	logger.Error("critical issue occurred", "code", 500)
	if !strings.Contains(buf.String(), "critical issue occurred") {
		t.Fatalf("expected error message in console output, got: %s", buf.String())
	}
	calls := notifier.getCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 notification for error, got %d", len(calls))
	}
	if calls[0].Level != slog.LevelError {
		t.Errorf("expected level error, got %v", calls[0].Level)
	}
	if !strings.Contains(calls[0].Message, "critical issue occurred (code=500)") {
		t.Errorf("unexpected notification message: %s", calls[0].Message)
	}
}

func TestNotifyDisabled(t *testing.T) {
	var buf bytes.Buffer
	notifier := &mockNotifier{}

	cfg := Config{
		Level:       "info",
		Notify:      false,
		NotifyLevel: "error",
	}

	h := NewHandler(cfg, &buf)
	h.SetNotifier(notifier)
	logger := slog.New(h)

	logger.Error("error while notifications disabled")
	if !strings.Contains(buf.String(), "error while notifications disabled") {
		t.Fatalf("expected console output")
	}
	if len(notifier.getCalls()) != 0 {
		t.Fatalf("expected 0 notifications when notify is disabled")
	}

	// Dynamically enable notifications
	h.SetNotifyEnabled(true)
	logger.Error("error after notifications enabled")
	if len(notifier.getCalls()) != 1 {
		t.Fatalf("expected 1 notification after dynamically enabling")
	}
}

func TestDynamicLevelChange(t *testing.T) {
	var buf bytes.Buffer
	cfg := Config{
		Level: "warn",
	}
	h := NewHandler(cfg, &buf)
	logger := slog.New(h)

	logger.Info("should be dropped")
	if buf.Len() != 0 {
		t.Fatalf("expected no output for info when level is warn")
	}

	// Change console level to info
	h.SetConsoleLevel(slog.LevelInfo)
	logger.Info("should be logged now")
	if !strings.Contains(buf.String(), "should be logged now") {
		t.Fatalf("expected output after level lowered to info")
	}
}

func TestWithAttrsAndGroup(t *testing.T) {
	var buf bytes.Buffer
	notifier := &mockNotifier{}

	cfg := Config{
		Level:       "info",
		Notify:      true,
		NotifyLevel: "info",
	}
	h := NewHandler(cfg, &buf)
	h.SetNotifier(notifier)

	logger := slog.New(h).With("module", "ipc").WithGroup("subgroup")
	logger.Info("hello with attrs")

	calls := notifier.getCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(calls))
	}
	if !strings.Contains(calls[0].Message, "module=ipc") {
		t.Errorf("expected module=ipc in message, got: %s", calls[0].Message)
	}
}

func TestInitGlobal(t *testing.T) {
	var buf bytes.Buffer
	h := Init(Config{Level: "info"}, &buf)
	if h == nil {
		t.Fatal("expected non-nil handler from Init")
	}

	Info("global info test")
	if !strings.Contains(buf.String(), "global info test") {
		t.Fatalf("expected global info log in buffer")
	}
}

func TestTerminalHandlerFormat(t *testing.T) {
	var buf bytes.Buffer
	lvlVar := new(slog.LevelVar)
	lvlVar.Set(slog.LevelDebug)

	th := newTerminalHandler(&buf, lvlVar)
	th.isTerm = false // Plaintext test

	logger := slog.New(th)
	logger.Info("phalune ready", "workspace", "main", "count", 42, "reason", "started normally")

	out := buf.String()
	// Should contain INF badge
	if !strings.Contains(out, "INF phalune ready") {
		t.Errorf("expected clean format with badge, got: %s", out)
	}
	// Attributes should be space-separated and values with spaces should be quoted
	if !strings.Contains(out, "workspace=main count=42 reason=\"started normally\"") {
		t.Errorf("expected clean attributes formatting, got: %s", out)
	}

	// Test colored output
	buf.Reset()
	th.isTerm = true
	logger.Error("critical failure", "code", 500)
	coloredOut := buf.String()
	if !strings.Contains(coloredOut, "\033[1;31mERR\033[0m") {
		t.Errorf("expected bold red ERR badge in terminal output, got: %s", coloredOut)
	}
	if !strings.Contains(coloredOut, "critical failure") {
		t.Errorf("expected message in colored output, got: %s", coloredOut)
	}
}
