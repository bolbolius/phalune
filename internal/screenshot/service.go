package screenshot

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"phalune/internal/config"
	"phalune/internal/niri"

	"github.com/diamondburned/gotk4/pkg/core/glib"
)

type Service struct {
	tools   *ToolPaths
	toast   *Toast
	niriSvc *niri.Service

	cfg config.ScreenshotConfig
}

func New(cfg config.ScreenshotConfig, niriSvc *niri.Service, toast *Toast, tools *ToolPaths) (*Service, error) {
	if tools == nil {
		var err error
		tools, err = ResolveTools()
		if err != nil {
			return nil, err
		}
	}
	return &Service{
		tools:   tools,
		toast:   toast,
		niriSvc: niriSvc,
		cfg:     cfg,
	}, nil
}

func (s *Service) UpdateConfig(cfg config.ScreenshotConfig) {
	if s == nil {
		return
	}
	s.cfg = cfg
	if s.toast != nil {
		s.toast.UpdateConfig(cfg)
	}
}

func (s *Service) Capture(modeArg string) {
	if s == nil || s.tools == nil {
		return
	}

	mode := s.resolveMode(modeArg)
	tools := s.tools
	niriSvc := s.niriSvc

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()

		png, err := tools.Capture(ctx, mode, niriSvc)
		if err != nil {
			slog.Warn("failed to capture screenshot", "error", err)
			return
		}

		shotPath := writeTempPNG(png)

		glib.IdleAdd(func() {
			if s.toast == nil {
				return
			}
			s.toast.Show(png, shotPath)
		})
	}()
}

func (s *Service) resolveMode(arg string) Mode {
	normalized := strings.ToLower(strings.TrimSpace(arg))
	switch normalized {
	case string(ModeArea):
		return ModeArea
	case string(ModeWindow):
		return ModeWindow
	case string(ModeDisplay), "screen", "monitor", "fullscreen":
		return ModeDisplay
	}

	def := strings.ToLower(strings.TrimSpace(s.cfg.DefaultMode))
	switch def {
	case string(ModeWindow):
		return ModeWindow
	case string(ModeDisplay), "screen", "monitor", "fullscreen":
		return ModeDisplay
	default:
		return ModeArea
	}
}

func (s *Service) ModeAvailable(mode Mode) bool {
	if s == nil || s.tools == nil {
		return false
	}
	switch mode {
	case ModeArea:
		return s.tools.supportArea()
	default:
		return s.tools.Grim != ""
	}
}