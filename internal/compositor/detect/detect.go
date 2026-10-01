package detect

import (
	"log/slog"
	"os"

	"phalune/internal/compositor"
	"phalune/internal/compositor/hyprland"
	"phalune/internal/compositor/niri"
	"phalune/internal/compositor/sway"
)

func Detect() (compositor.Kind, compositor.Service, error) {
	if svc, ok := detectNiri(); ok {
		return compositor.Niri, svc, nil
	}
	if svc, ok := detectSway(); ok {
		return compositor.Sway, svc, nil
	}
	if svc, ok := detectHyprland(); ok {
		return compositor.Hyprland, svc, nil
	}
	return "", nil, compositor.ErrNoCompositor
}

func detectNiri() (compositor.Service, bool) {
	if os.Getenv("NIRI_SOCKET") == "" {
		return nil, false
	}
	client, err := niri.NewClient("")
	if err != nil {
		slog.Debug("compositor: niri socket found but client init failed", "error", err)
		return nil, false
	}
	svc := niri.NewService(client)
	if err := svc.Start(); err != nil {
		slog.Debug("compositor: niri event stream failed", "error", err)
		return nil, false
	}
	return svc, true
}

func detectSway() (compositor.Service, bool) {
	if os.Getenv("SWAYSOCK") == "" {
		return nil, false
	}
	svc := sway.New("")
	if err := svc.Ping(); err != nil {
		slog.Debug("compositor: sway socket found but IPC ping failed", "error", err)
		return nil, false
	}
	if err := svc.Start(); err != nil {
		slog.Debug("compositor: sway event stream failed", "error", err)
		return nil, false
	}
	return svc, true
}

func detectHyprland() (compositor.Service, bool) {
	if os.Getenv("HYPRLAND_INSTANCE_SIGNATURE") == "" {
		return nil, false
	}
	svc := hyprland.New()
	if err := svc.Ping(); err != nil {
		slog.Debug("compositor: hyprland signature found but IPC failed", "error", err)
		return nil, false
	}
	if err := svc.Start(); err != nil {
		slog.Debug("compositor: hyprland event stream failed", "error", err)
		return nil, false
	}
	return svc, true
}
