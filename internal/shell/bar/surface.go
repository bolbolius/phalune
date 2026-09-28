package bar

import (
	"fmt"

	"github.com/diamondburned/gotk4-layer-shell/pkg/gtk4layershell"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

func ConfigureLayerSurface(win *gtk.Window, monitor *gdk.Monitor, height int, position string) error {
	if !gtk4layershell.IsSupported() {
		return fmt.Errorf("Wayland compositor does not support wlr-layer-shell protocol")
	}

	gtk4layershell.InitForWindow(win)
	gtk4layershell.SetNamespace(win, "phalune-bar")
	gtk4layershell.SetLayer(win, gtk4layershell.LayerShellLayerTop)

	if monitor != nil {
		gtk4layershell.SetMonitor(win, monitor)
	}

	isBottom := position == "bottom"
	gtk4layershell.SetAnchor(win, gtk4layershell.LayerShellEdgeTop, !isBottom)
	gtk4layershell.SetAnchor(win, gtk4layershell.LayerShellEdgeBottom, isBottom)
	gtk4layershell.SetAnchor(win, gtk4layershell.LayerShellEdgeLeft, true)
	gtk4layershell.SetAnchor(win, gtk4layershell.LayerShellEdgeRight, true)

	if height > 0 {
		gtk4layershell.SetExclusiveZone(win, height)
	} else {
		gtk4layershell.AutoExclusiveZoneEnable(win)
	}

	gtk4layershell.SetKeyboardMode(win, gtk4layershell.LayerShellKeyboardModeNone)

	return nil
}
