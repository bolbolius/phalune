package custom

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"phalune/internal/config"
	"phalune/internal/widget"

	"github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"
)

// execTimeout bounds a single command execution in poll/single-run mode.
const execTimeout = 30 * time.Second

// restartDelay waits before relaunching a failed tail process.
const restartDelay = 2 * time.Second

// payload is the optional JSON schema (return_type = "json").
type payload struct {
	Text    string `json:"text"`
	Tooltip string `json:"tooltip"`
	Class   string `json:"class"`
}

type Widget struct {
	box   *gtk.Box
	icon  *gtk.Image
	label *gtk.Label

	cfg    config.CustomWidgetConfig
	name   string
	cancel context.CancelFunc

	mu      sync.Mutex
	lastCmd *exec.Cmd

	lastScroll time.Time // guards scroll debounce

	currentClass string // last JSON-provided CSS class, main thread only
}

func New(ctx widget.Context) (widget.Widget, error) {
	cfg, ok := ctx.Config.Bar.Custom[ctx.WidgetName]
	if !ok {
		return nil, fmt.Errorf("no [bar.custom.%s] section", ctx.WidgetName)
	}

	box := gtk.NewBox(gtk.OrientationHorizontal, 0)
	icon := gtk.NewImage()
	label := gtk.NewLabel("")
	label.SetHAlign(gtk.AlignStart)
	label.SetEllipsize(pango.EllipsizeEnd)

	box.Append(icon)
	box.Append(label)
	box.AddCSSClass("widget-box")
	box.AddCSSClass("widget-pill")
	box.AddCSSClass("widget-custom")
	box.AddCSSClass("widget-custom-" + ctx.WidgetName)
	icon.AddCSSClass("widget-icon")
	label.AddCSSClass("widget-label")
	if cfg.Icon != "" {
		icon.SetFromIconName(cfg.Icon)
	} else {
		icon.SetVisible(false)
	}
	if cfg.HideEmpty {
		box.SetVisible(false)
	}

	wCtx, cancel := context.WithCancel(context.Background())
	w := &Widget{
		box:    box,
		icon:   icon,
		label:  label,
		cfg:    cfg,
		name:   ctx.WidgetName,
		cancel: cancel,
	}
	w.setupInteractions()

	go func() {
		switch {
		case cfg.Tail:
			w.runTail(wCtx)
		case cfg.Interval.Duration > 0:
			w.runPoll(wCtx)
		default:
			w.runOnce(wCtx)
		}
	}()

	return w, nil
}

func (w *Widget) Root() gtk.Widgetter { return w.box }

func (w *Widget) Destroy() {
	if w.cancel != nil {
		w.cancel()
		w.cancel = nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.lastCmd != nil && w.lastCmd.Process != nil {
		_ = w.lastCmd.Process.Kill()
	}
}

func (w *Widget) runOnce(ctx context.Context) {
	w.execOnce(ctx)
}

func (w *Widget) runPoll(ctx context.Context) {
	w.execOnce(ctx)
	ticker := time.NewTicker(w.cfg.Interval.Duration)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.execOnce(ctx)
		}
	}
}

// runTail relaunches the stream if the process exits, so a crashing
// script never permanently kills the widget.
func (w *Widget) runTail(ctx context.Context) {
	for ctx.Err() == nil {
		w.streamOnce(ctx)
		if ctx.Err() != nil {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(restartDelay):
		}
	}
}

func (w *Widget) execOnce(ctx context.Context) {
	runCtx, cancel := context.WithTimeout(ctx, execTimeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, "sh", "-c", w.cfg.Exec)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	w.trackCmd(cmd)
	out, err := cmd.Output()
	w.trackCmd(nil)

	if ctx.Err() != nil {
		return
	}
	text := strings.TrimSpace(string(out))
	w.apply(text)
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || text == "" {
			slog.Warn("custom widget exec failed", "widget", w.name, "error", err)
		}
	}
}

func (w *Widget) streamOnce(ctx context.Context) {
	cmd := exec.Command("sh", "-c", w.cfg.Exec)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		slog.Warn("custom widget pipe failed", "widget", w.name, "error", err)
		return
	}
	if err := cmd.Start(); err != nil {
		slog.Warn("custom widget start failed", "widget", w.name, "error", err)
		return
	}
	w.trackCmd(cmd)
	defer func() {
		if cmd.Process != nil {
			// Kill the whole process group: children spawning daemons
			// (e.g. journalctl) would otherwise outlive the kill and keep
			// the stdout pipe open.
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		_ = cmd.Wait()
		w.trackCmd(nil)
	}()

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		if ctx.Err() != nil {
			return
		}
		w.apply(strings.TrimSpace(scanner.Text()))
	}
	if err := scanner.Err(); err != nil {
		slog.Debug("custom widget stream ended", "widget", w.name, "error", err)
	}
}

func (w *Widget) trackCmd(cmd *exec.Cmd) {
	w.mu.Lock()
	w.lastCmd = cmd
	w.mu.Unlock()
}

// apply renders raw output per return_type and format.
// apply is called from worker goroutines; all widget mutation happens
// inside glib.IdleAdd on the GTK main thread.
func (w *Widget) apply(raw string) {
	p := payload{Text: raw}
	if w.isJSON() {
		if err := json.Unmarshal([]byte(raw), &p); err != nil {
			slog.Warn("custom widget: invalid JSON output", "widget", w.name, "error", err)
			return
		}
	}

	text := w.cfg.Format
	if text == "" {
		if w.isJSON() {
			text = "{text}"
		} else {
			text = "{}"
		}
	}
	if w.isJSON() {
		text = strings.ReplaceAll(text, "{text}", p.Text)
		text = strings.ReplaceAll(text, "{tooltip}", p.Tooltip)
		text = strings.ReplaceAll(text, "{class}", p.Class)
	} else {
		// Plain mode shows the last line of multi-line output.
		line := raw
		if i := strings.LastIndexByte(raw, '\n'); i >= 0 {
			line = strings.TrimSpace(raw[i+1:])
		}
		text = strings.ReplaceAll(text, "{}", line)
	}

	visible := !(w.cfg.HideEmpty && strings.TrimSpace(p.Text) == "")
	tooltip := p.Tooltip
	class := p.Class

	glib.IdleAdd(func() {
		if w.box == nil {
			return
		}
		w.label.SetText(text)
		w.box.SetVisible(visible)
		w.box.SetTooltipText(tooltip)
		w.setDynamicClass(class)
	})
}

// setDynamicClass swaps the JSON-provided classes. A payload may carry
// several space-separated tokens ("active urgent"); each is applied
// individually since GTK rejects CSS classes containing spaces.
// It must run on the main thread.
func (w *Widget) setDynamicClass(classes string) {
	if classes == w.currentClass {
		return
	}
	for _, c := range strings.Fields(w.currentClass) {
		w.box.RemoveCSSClass(c)
	}
	for _, c := range strings.Fields(classes) {
		w.box.AddCSSClass(c)
	}
	w.currentClass = classes
}

func (w *Widget) isJSON() bool {
	return strings.EqualFold(strings.TrimSpace(w.cfg.ReturnType), "json")
}
