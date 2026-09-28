package widget

import (
	"phalune/internal/config"
	"phalune/internal/niri"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

type Widget interface {
	Root() gtk.Widgetter
	Destroy()
}

type Context struct {
	Config          *config.Config
	Niri            *niri.Service
	Output          string
	ShowOSD         func(icon, label string, value float64)
	TogglePowerMenu          func()
	OpenPowerMenu            func()
	ClosePowerMenu           func()
	ToggleControlCenter      func()
	OpenControlCenterSubpage func(page string)
	ToggleNotificationCenter func()
	OpenNotificationCenter   func()
	CloseNotificationCenter  func()
	NotifyStore              NotificationStore
	Privacy                  PrivacyMonitor
}

type NotificationStore interface {
	Count() int
	Subscribe(fn func()) func()
}

type PrivacyMonitor interface {
	Subscribe(fn func(mic, cam bool)) func()
}

type Factory func(ctx Context) (Widget, error)
