package main

import (
	"log/slog"
	"os"

	"phalune/internal/config"
	"phalune/internal/logging"
	"phalune/internal/settings"

	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

func main() {
	if err := os.Setenv("GDK_BACKEND", "wayland"); err != nil {
		slog.Warn("failed to set GDK_BACKEND=wayland", "error", err)
	}

	cfgPath := settings.ConfigLoc()
	cfg, err := config.Load(cfgPath)
	if err != nil {
		cfg = config.Default()
	}

	logging.Init(logging.Config{
		Level:        cfg.Logging.Level,
		ConsoleLevel: cfg.Logging.ConsoleLevel,
	}, os.Stderr)

	app := gtk.NewApplication("org.phalune.settings", gio.ApplicationNonUnique)

	app.ConnectActivate(func() {
		if err := settings.LoadStyle(); err != nil {
			slog.Warn("failed to load settings stylesheet", "error", err)
		}

		w, err := settings.NewWindow(app)
		if err != nil {
			slog.Error("failed to build settings window", "error", err)
			os.Exit(1)
		}
		w.Show()
	})

	os.Exit(app.Run(os.Args))
}
