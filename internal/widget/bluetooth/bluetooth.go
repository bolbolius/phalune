package bluetooth

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"phalune/internal/widget"
	"phalune/ui"

	"github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/godbus/dbus/v5"
)

const (
	bluezDest  = "org.bluez"
	bluezPath  = "/org/bluez/hci0"
	bluezIFace = "org.bluez.Adapter1"
)

type Bluetooth struct {
	box             *gtk.Box
	icon            *gtk.Image
	label           *gtk.Label
	cancel          context.CancelFunc
	showLabel       bool
	hideUnavailable bool

	mu        sync.Mutex
	conn      *dbus.Conn
	powered   bool
	available bool
}

func New(ctx widget.Context) (widget.Widget, error) {
	builder := gtk.NewBuilderFromString(ui.Bluetooth)
	box := builder.GetObject("bluetooth_box").Cast().(*gtk.Box)
	icon := builder.GetObject("bluetooth_icon").Cast().(*gtk.Image)
	label := builder.GetObject("bluetooth_label").Cast().(*gtk.Label)

	showLabel := true
	hideUnavailable := false
	if ctx.Config != nil {
		showLabel = ctx.Config.Bar.Bluetooth.ShowLabel
		hideUnavailable = ctx.Config.Bar.Bluetooth.HideUnavailable
	}

	if !showLabel {
		label.SetVisible(false)
	}

	// Click directly opens Bluetooth subview in Control Center
	click := gtk.NewGestureClick()
	click.SetButton(0)
	click.ConnectReleased(func(n int, x, y float64) {
		btn := click.CurrentButton()
		if btn == gdk.BUTTON_PRIMARY || btn == gdk.BUTTON_SECONDARY {
			if ctx.OpenControlCenterSubpage != nil {
				ctx.OpenControlCenterSubpage("bluetooth")
			} else if ctx.ToggleControlCenter != nil {
				ctx.ToggleControlCenter()
			}
		}
	})
	box.AddController(click)

	bCtx, cancel := context.WithCancel(context.Background())
	b := &Bluetooth{
		box:             box,
		icon:            icon,
		label:           label,
		cancel:          cancel,
		showLabel:       showLabel,
		hideUnavailable: hideUnavailable,
	}

	go b.start(bCtx)

	return b, nil
}

func (b *Bluetooth) Root() gtk.Widgetter {
	return b.box
}

func (b *Bluetooth) Destroy() {
	if b.cancel != nil {
		b.cancel()
		b.cancel = nil
	}
}

func (b *Bluetooth) start(ctx context.Context) {
	conn, err := dbus.SystemBus()
	if err != nil {
		slog.Debug("bluetooth: system bus unavailable", "error", err)
		b.updateState(false, false)
		return
	}

	b.mu.Lock()
	b.conn = conn
	b.mu.Unlock()

	// Initial adapter query
	obj := conn.Object(bluezDest, bluezPath)
	val, err := obj.GetProperty(bluezIFace + ".Powered")
	if err == nil {
		if powered, ok := val.Value().(bool); ok {
			b.updateState(powered, true)
		} else {
			b.updateState(false, true)
		}
	} else {
		b.updateState(false, false)
	}

	// Zero-polling signal match for property changes on /org/bluez/hci0
	rule := fmt.Sprintf("type='signal',interface='org.freedesktop.DBus.Properties',member='PropertiesChanged',path='%s'", bluezPath)
	conn.BusObject().Call("org.freedesktop.DBus.AddMatch", 0, rule)

	ch := make(chan *dbus.Signal, 10)
	conn.Signal(ch)
	defer conn.RemoveSignal(ch)

	for {
		select {
		case <-ctx.Done():
			return
		case sig, ok := <-ch:
			if !ok {
				return
			}
			if sig.Path == bluezPath && len(sig.Body) >= 2 {
				if iface, ok := sig.Body[0].(string); ok && iface == bluezIFace {
					if changed, ok := sig.Body[1].(map[string]dbus.Variant); ok {
						if v, exists := changed["Powered"]; exists {
							if p, ok := v.Value().(bool); ok {
								b.updateState(p, true)
							}
						}
					}
				}
			}
		}
	}
}

func (b *Bluetooth) updateState(powered, available bool) {
	b.mu.Lock()
	b.powered = powered
	b.available = available
	hideUnavail := b.hideUnavailable
	showLabel := b.showLabel
	b.mu.Unlock()

	glib.IdleAdd(func() {
		if !available {
			if hideUnavail {
				b.box.SetVisible(false)
			} else {
				b.box.SetVisible(true)
				b.box.RemoveCSSClass("active")
				b.box.RemoveCSSClass("disabled")
				b.box.AddCSSClass("unavailable")
				b.icon.SetFromIconName("bluetooth-disabled-symbolic")
				if showLabel {
					b.label.SetLabel("Unavailable")
				}
				b.box.SetTooltipText("Bluetooth: Unavailable")
			}
			return
		}

		b.box.SetVisible(true)
		b.box.RemoveCSSClass("unavailable")

		if powered {
			b.box.RemoveCSSClass("disabled")
			b.box.AddCSSClass("active")
			b.icon.SetFromIconName("bluetooth-active-symbolic")
			if showLabel {
				b.label.SetLabel("On")
			}
			b.box.SetTooltipText("Bluetooth: On")
		} else {
			b.box.RemoveCSSClass("active")
			b.box.AddCSSClass("disabled")
			b.icon.SetFromIconName("bluetooth-disabled-symbolic")
			if showLabel {
				b.label.SetLabel("Off")
			}
			b.box.SetTooltipText("Bluetooth: Off")
		}
	})
}
