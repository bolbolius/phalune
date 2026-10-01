package screenshot

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"phalune/internal/compositor"
)

type Mode string

const (
	ModeArea    Mode = "area"
	ModeWindow  Mode = "window"
	ModeDisplay Mode = "display"
)

type ToolPaths struct {
	Grim  string
	Slurp string
}

func ResolveTools() (*ToolPaths, error) {
	g, err := exec.LookPath("grim")
	if err != nil {
		return nil, fmt.Errorf("grim is required for screenshots: %w", err)
	}
	s, err := exec.LookPath("slurp")
	if err != nil {
		slog.Debug("screenshot: slurp not found; area mode unavailable", "error", err)
		s = ""
	}
	return &ToolPaths{Grim: g, Slurp: s}, nil
}

func (t *ToolPaths) supportArea() bool {
	return t != nil && t.Slurp != ""
}

func (t *ToolPaths) Capture(ctx context.Context, mode Mode, compositorSvc compositor.Service) ([]byte, error) {
	if t == nil || t.Grim == "" {
		return nil, fmt.Errorf("grim is not installed")
	}

	switch mode {
	case ModeWindow:
		return t.captureWindow(ctx, compositorSvc)
	case ModeDisplay:
		return t.captureDisplay(ctx)
	default:
		if t.supportArea() {
			return t.captureArea(ctx)
		}
		return t.captureDisplay(ctx)
	}
}

func (t *ToolPaths) captureArea(ctx context.Context) ([]byte, error) {
	selCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	out, err := exec.CommandContext(selCtx, t.Slurp).Output()
	if err != nil {
		if selCtx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("area selection timed out")
		}
		var ee *exec.ExitError
		if errors.As(err, &ee) && ee.ExitCode() == 1 {
			return nil, fmt.Errorf("selection cancelled")
		}
		return nil, fmt.Errorf("area selection: %w", err)
	}

	geometry := strings.TrimSpace(string(out))
	if geometry == "" {
		return nil, fmt.Errorf("empty area selection")
	}

	grimCtx, gcancel := context.WithTimeout(ctx, 30*time.Second)
	defer gcancel()
	return exec.CommandContext(grimCtx, t.Grim, "-t", "png", "-g", geometry, "-").Output()
}

func (t *ToolPaths) captureWindow(ctx context.Context, compositorSvc compositor.Service) ([]byte, error) {
	if compositorSvc == nil {
		return t.captureDisplay(ctx)
	}

	windows, err := compositorSvc.QueryWindows()
	if err != nil {
		return nil, fmt.Errorf("query focused window: %w", err)
	}

	var focused *compositor.Window
	for i := range windows {
		if windows[i].IsFocused {
			focused = &windows[i]
			break
		}
	}
	if focused == nil {
		return nil, fmt.Errorf("no focused window")
	}

	pos, size := focused.Pos, focused.Size
	if pos == nil || size == nil || size[0] <= 0 || size[1] <= 0 {
		return t.captureDisplay(ctx)
	}

	geometry := fmt.Sprintf("%d,%d %dx%d", int(pos[0]), int(pos[1]), int(size[0]), int(size[1]))

	grimCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return exec.CommandContext(grimCtx, t.Grim, "-t", "png", "-g", geometry, "-").Output()
}

func (t *ToolPaths) captureDisplay(ctx context.Context) ([]byte, error) {
	grimCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return exec.CommandContext(grimCtx, t.Grim, "-t", "png", "-").Output()
}

func Filename(now time.Time) string {
	return fmt.Sprintf("screenshot-%s.png", now.Format("20060102-150405"))
}

func Save(saveDir string, png []byte, now time.Time) (string, error) {
	dir := saveDir
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home dir: %w", err)
		}
		dir = filepath.Join(home, "Pictures", "Screenshots")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create screenshot dir %q: %w", dir, err)
	}

	path := filepath.Join(dir, Filename(now))
	if _, err := os.Stat(path); err == nil {
		for i := 1; i <= 10000; i++ {
			candidate := filepath.Join(dir, fmt.Sprintf("screenshot-%s-%d.png", now.Format("20060102-150405"), i))
			if _, err := os.Stat(candidate); err != nil {
				if os.IsNotExist(err) {
					path = candidate
					break
				}
				return "", fmt.Errorf("stat screenshot %q: %w", candidate, err)
			}
		}
	}

	if err := os.WriteFile(path, png, 0o644); err != nil {
		return "", fmt.Errorf("write screenshot %q: %w", path, err)
	}
	return path, nil
}
