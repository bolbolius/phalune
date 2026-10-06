package wallpaper

import (
	"fmt"

	"github.com/diamondburned/gotk4-layer-shell/pkg/gtk4layershell"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// ConfigureSurface configures a GTK window as a background layer-shell surface
// that anchors to all 4 edges behind all other surfaces.
func ConfigureSurface(win *gtk.Window, monitor *gdk.Monitor) error {
	if !gtk4layershell.IsSupported() {
		return fmt.Errorf("Wayland compositor does not support wlr-layer-shell protocol")
	}

	gtk4layershell.InitForWindow(win)
	gtk4layershell.SetNamespace(win, "phalune-wallpaper")
	gtk4layershell.SetLayer(win, gtk4layershell.LayerShellLayerBackground)

	if monitor != nil {
		gtk4layershell.SetMonitor(win, monitor)
	}

	gtk4layershell.SetAnchor(win, gtk4layershell.LayerShellEdgeTop, true)
	gtk4layershell.SetAnchor(win, gtk4layershell.LayerShellEdgeBottom, true)
	gtk4layershell.SetAnchor(win, gtk4layershell.LayerShellEdgeLeft, true)
	gtk4layershell.SetAnchor(win, gtk4layershell.LayerShellEdgeRight, true)

	gtk4layershell.SetExclusiveZone(win, -1)
	gtk4layershell.SetKeyboardMode(win, gtk4layershell.LayerShellKeyboardModeNone)

	return nil
}
