package wifi

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"phalune/internal/controlcenter"
	"phalune/internal/widget"
	"phalune/ui"

	"github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/godbus/dbus/v5"
)

const (
	nmDest  = "org.freedesktop.NetworkManager"
	nmPath  = "/org/freedesktop/NetworkManager"
	nmIFace = "org.freedesktop.NetworkManager"
)

type Wifi struct {
	box             *gtk.Box
	icon            *gtk.Image
	label           *gtk.Label
	cancel          context.CancelFunc
	showLabel       bool
	hideUnavailable bool

	mu         sync.Mutex
	conn       *dbus.Conn
	devicePath dbus.ObjectPath
	enabled    bool
	available  bool
	ssid       string
	strength   uint8
}

func New(ctx widget.Context) (widget.Widget, error) {
	builder := gtk.NewBuilderFromString(ui.Wifi)
	box := builder.GetObject("wifi_box").Cast().(*gtk.Box)
	icon := builder.GetObject("wifi_icon").Cast().(*gtk.Image)
	label := builder.GetObject("wifi_label").Cast().(*gtk.Label)

	showLabel := true
	hideUnavailable := false
	if ctx.Config != nil {
		showLabel = ctx.Config.Bar.Wifi.ShowLabel
		hideUnavailable = ctx.Config.Bar.Wifi.HideUnavailable
	}

	if !showLabel {
		label.SetVisible(false)
	}

	// Click directly opens Wi-Fi subview in Control Center
	click := gtk.NewGestureClick()
	click.SetButton(0)
	click.ConnectReleased(func(n int, x, y float64) {
		btn := click.CurrentButton()
		if btn == gdk.BUTTON_PRIMARY || btn == gdk.BUTTON_SECONDARY {
			if ctx.OpenControlCenterSubpage != nil {
				ctx.OpenControlCenterSubpage("wifi")
			} else if ctx.ToggleControlCenter != nil {
				ctx.ToggleControlCenter()
			}
		}
	})
	box.AddController(click)

	bCtx, cancel := context.WithCancel(context.Background())
	w := &Wifi{
		box:             box,
		icon:            icon,
		label:           label,
		cancel:          cancel,
		showLabel:       showLabel,
		hideUnavailable: hideUnavailable,
	}

	go w.start(bCtx)

	return w, nil
}

func (w *Wifi) Root() gtk.Widgetter {
	return w.box
}

func (w *Wifi) Destroy() {
	if w.cancel != nil {
		w.cancel()
		w.cancel = nil
	}
}

func (w *Wifi) start(ctx context.Context) {
	conn, err := dbus.SystemBus()
	if err != nil {
		slog.Debug("wifi widget: system bus unavailable", "error", err)
		w.updateState(false, false, "", 0)
		return
	}

	w.mu.Lock()
	w.conn = conn
	w.mu.Unlock()

	w.updateDevice(conn)
	w.queryState(conn)

	ruleNM := fmt.Sprintf("type='signal',interface='org.freedesktop.DBus.Properties',member='PropertiesChanged',path='%s'", nmPath)
	conn.BusObject().Call("org.freedesktop.DBus.AddMatch", 0, ruleNM)

	w.mu.Lock()
	devPath := w.devicePath
	w.mu.Unlock()

	if devPath != "" {
		ruleDev := fmt.Sprintf("type='signal',interface='org.freedesktop.DBus.Properties',member='PropertiesChanged',path='%s'", devPath)
		conn.BusObject().Call("org.freedesktop.DBus.AddMatch", 0, ruleDev)
		ruleAccessPoints := fmt.Sprintf("type='signal',interface='org.freedesktop.NetworkManager.Device.Wireless',path='%s'", devPath)
		conn.BusObject().Call("org.freedesktop.DBus.AddMatch", 0, ruleAccessPoints)
	}

	ch := make(chan *dbus.Signal, 15)
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
			if sig.Path == nmPath {
				w.updateDevice(conn)
				w.queryState(conn)
			} else {
				w.mu.Lock()
				curDev := w.devicePath
				w.mu.Unlock()
				if curDev != "" && sig.Path == curDev {
					w.queryState(conn)
				}
			}
		}
	}
}

func (w *Wifi) updateDevice(conn *dbus.Conn) {
	obj := conn.Object(nmDest, nmPath)
	val, err := obj.GetProperty(nmIFace + ".AllDevices")
	if err != nil {
		return
	}
	paths, ok := val.Value().([]dbus.ObjectPath)
	if !ok {
		return
	}

	var foundPath dbus.ObjectPath
	for _, p := range paths {
		devObj := conn.Object(nmDest, p)
		typeVal, err := devObj.GetProperty("org.freedesktop.NetworkManager.Device.DeviceType")
		if err != nil {
			continue
		}
		if t, ok := typeVal.Value().(uint32); ok && t == 2 { // NM_DEVICE_TYPE_WIFI = 2
			foundPath = p
			break
		}
	}

	w.mu.Lock()
	w.devicePath = foundPath
	w.mu.Unlock()
}

func (w *Wifi) queryState(conn *dbus.Conn) {
	w.mu.Lock()
	devPath := w.devicePath
	w.mu.Unlock()

	if devPath == "" {
		w.updateState(false, false, "", 0)
		return
	}

	obj := conn.Object(nmDest, nmPath)
	val, err := obj.GetProperty(nmIFace + ".WirelessEnabled")
	if err != nil {
		w.updateState(false, false, "", 0)
		return
	}
	enabled, _ := val.Value().(bool)

	if !enabled {
		w.updateState(false, true, "", 0)
		return
	}

	activeSSID := w.queryActiveWiFiSSID(conn)
	strength := w.queryActiveStrength(conn, devPath, activeSSID)

	w.updateState(true, true, activeSSID, strength)
}

func (w *Wifi) queryActiveWiFiSSID(conn *dbus.Conn) string {
	obj := conn.Object(nmDest, nmPath)
	val, err := obj.GetProperty(nmIFace + ".ActiveConnections")
	if err != nil {
		return ""
	}

	paths, ok := val.Value().([]dbus.ObjectPath)
	if !ok {
		return ""
	}

	for _, p := range paths {
		connObj := conn.Object(nmDest, p)
		typeVal, err := connObj.GetProperty("org.freedesktop.NetworkManager.Connection.Active.Type")
		if err != nil {
			continue
		}
		if tStr, ok := typeVal.Value().(string); ok && tStr == "802-11-wireless" {
			idVal, err := connObj.GetProperty("org.freedesktop.NetworkManager.Connection.Active.Id")
			if err == nil {
				if idStr, ok := idVal.Value().(string); ok && idStr != "" {
					return idStr
				}
			}
		}
	}
	return ""
}

func (w *Wifi) queryActiveStrength(conn *dbus.Conn, devPath dbus.ObjectPath, activeSSID string) uint8 {
	if activeSSID == "" {
		return 0
	}
	devObj := conn.Object(nmDest, devPath)
	val, err := devObj.GetProperty("org.freedesktop.NetworkManager.Device.Wireless.ActiveAccessPoint")
	if err != nil {
		return 50
	}
	apPath, ok := val.Value().(dbus.ObjectPath)
	if !ok || apPath == "/" || apPath == "" {
		return 50
	}

	apObj := conn.Object(nmDest, apPath)
	sVal, err := apObj.GetProperty("org.freedesktop.NetworkManager.AccessPoint.Strength")
	if err == nil {
		if st, ok := sVal.Value().(byte); ok {
			return uint8(st)
		}
	}
	return 50
}

func (w *Wifi) updateState(enabled, available bool, ssid string, strength uint8) {
	w.mu.Lock()
	w.enabled = enabled
	w.available = available
	w.ssid = ssid
	w.strength = strength
	hideUnavail := w.hideUnavailable
	showLabel := w.showLabel
	w.mu.Unlock()

	glib.IdleAdd(func() {
		if !available {
			if hideUnavail {
				w.box.SetVisible(false)
			} else {
				w.box.SetVisible(true)
				w.box.RemoveCSSClass("active")
				w.box.RemoveCSSClass("disabled")
				w.box.RemoveCSSClass("disconnected")
				w.box.AddCSSClass("unavailable")
				w.icon.SetFromIconName("network-wireless-symbolic")
				if showLabel {
					w.label.SetLabel("Unavailable")
				}
				w.box.SetTooltipText("Wi-Fi: Unavailable")
			}
			return
		}

		w.box.SetVisible(true)
		w.box.RemoveCSSClass("unavailable")

		if !enabled {
			w.box.RemoveCSSClass("active")
			w.box.RemoveCSSClass("disconnected")
			w.box.AddCSSClass("disabled")
			w.icon.SetFromIconName("network-wireless-disabled-symbolic")
			if showLabel {
				w.label.SetLabel("Off")
			}
			w.box.SetTooltipText("Wi-Fi: Off (Click to open)")
			return
		}

		w.box.RemoveCSSClass("disabled")

		if ssid != "" {
			w.box.RemoveCSSClass("disconnected")
			w.box.AddCSSClass("active")
			w.icon.SetFromIconName(controlcenter.WifiSignalIcon(strength))
			if showLabel {
				display := ssid
				if len(display) > 16 {
					display = strings.TrimSpace(display[:14]) + "…"
				}
				w.label.SetLabel(display)
			}
			w.box.SetTooltipText(fmt.Sprintf("Wi-Fi: %s (%d%%)\nClick to manage networks", ssid, strength))
		} else {
			w.box.RemoveCSSClass("active")
			w.box.AddCSSClass("disconnected")
			w.icon.SetFromIconName("network-wireless-symbolic")
			if showLabel {
				w.label.SetLabel("Disconnected")
			}
			w.box.SetTooltipText("Wi-Fi: Disconnected (Click to connect)")
		}
	})
}
