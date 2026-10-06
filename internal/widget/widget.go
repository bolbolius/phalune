package widget

import (
	"phalune/internal/compositor"
	"phalune/internal/config"
	"phalune/internal/ipc"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

type Widget interface {
	Root() gtk.Widgetter
	Destroy()
}

type Context struct {
	Config     *config.Config
	Compositor compositor.Service
	Output     string
	// WidgetName is the exact name from the bar section list ("custom.weather"
	// → "weather"). Factory implementations that serve several names must use it.
	WidgetName string
	// WidgetHub receives state pushed by external programs for "ipc:<id>"
	// widgets; nil when the hub is unavailable.
	WidgetHub                *ipc.WidgetHub
	ShowOSD                  func(icon, label string, value float64)
	TogglePowerMenu          func()
	OpenPowerMenu            func()
	ClosePowerMenu           func()
	ToggleControlCenter      func()
	OpenControlCenterSubpage func(page string)
	ToggleNotificationCenter func()
	OpenNotificationCenter   func()
	CloseNotificationCenter  func()
	ToggleClipboard          func()
	OpenClipboard            func()
	CloseClipboard           func()
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
