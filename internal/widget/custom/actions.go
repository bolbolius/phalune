package custom

import (
	"log/slog"
	"os/exec"
	"syscall"
	"time"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// detachedProcAttr puts the action in its own session so the shell never
// becomes its parent-job target and no signal reaches it from the shell.
func detachedProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}

// defaultScrollDebounce rate-limits scroll-triggered commands.
const defaultScrollDebounce = 100 * time.Millisecond

// setupInteractions binds mouse buttons and scroll directions to user commands.
func (w *Widget) setupInteractions() {
	click := gtk.NewGestureClick()
	click.SetButton(0)
	click.ConnectReleased(func(int, float64, float64) {
		var cmd string
		switch click.CurrentButton() {
		case gdk.BUTTON_PRIMARY:
			cmd = w.cfg.OnClick
		case gdk.BUTTON_SECONDARY:
			cmd = w.cfg.OnClickRight
		case gdk.BUTTON_MIDDLE:
			cmd = w.cfg.OnClickMiddle
		}
		if cmd != "" {
			go w.runAction(cmd)
		}
	})
	w.box.AddController(click)

	if w.cfg.OnScrollUp == "" && w.cfg.OnScrollDown == "" {
		return
	}

	scroll := gtk.NewEventControllerScroll(gtk.EventControllerScrollVertical)
	scroll.ConnectScroll(func(_, dy float64) bool {
		if !w.scrollAllow() {
			return true
		}
		cmd := w.cfg.OnScrollDown
		if dy < 0 {
			cmd = w.cfg.OnScrollUp
		}
		if cmd != "" {
			go w.runAction(cmd)
			return true
		}
		return false
	})
	w.box.AddController(scroll)
}

// scrollAllow debounces scroll events, dropping those inside the window.
func (w *Widget) scrollAllow() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	now := time.Now()
	if now.Sub(w.lastScroll) < w.scrollDebounce() {
		return false
	}
	w.lastScroll = now
	return true
}

func (w *Widget) scrollDebounce() time.Duration {
	if w.cfg.ScrollDebounceMs > 0 {
		return time.Duration(w.cfg.ScrollDebounceMs) * time.Millisecond
	}
	return defaultScrollDebounce
}

// runAction launches a user command detached: GUI apps (pavucontrol,
// xdg-open) must outlive the timeout, which only guards the spawn.
func (w *Widget) runAction(cmd string) {
	run := exec.Command("sh", "-c", cmd)
	run.SysProcAttr = detachedProcAttr()
	if err := run.Start(); err != nil {
		slog.Warn("custom widget action failed to launch", "widget", w.name, "command", cmd, "error", err)
		return
	}
	go func() { _ = run.Wait() }()
}
