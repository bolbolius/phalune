package lockscreen

import (
	"fmt"

	"github.com/diamondburned/gotk4-layer-shell/pkg/gtk4layershell"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// ConfigureLockScreenSurface configures a layer-shell overlay window to cover the screen
// with exclusive keyboard grab for lock screen security.
func ConfigureLockScreenSurface(win *gtk.Window, monitor *gdk.Monitor) error {
	if !gtk4layershell.IsSupported() {
		return fmt.Errorf("Wayland compositor does not support wlr-layer-shell protocol")
	}

	gtk4layershell.InitForWindow(win)
	gtk4layershell.SetNamespace(win, "phalune-lockscreen")
	gtk4layershell.SetLayer(win, gtk4layershell.LayerShellLayerOverlay)

	if monitor != nil {
		gtk4layershell.SetMonitor(win, monitor)
	}

	gtk4layershell.SetAnchor(win, gtk4layershell.LayerShellEdgeTop, true)
	gtk4layershell.SetAnchor(win, gtk4layershell.LayerShellEdgeBottom, true)
	gtk4layershell.SetAnchor(win, gtk4layershell.LayerShellEdgeLeft, true)
	gtk4layershell.SetAnchor(win, gtk4layershell.LayerShellEdgeRight, true)

	gtk4layershell.SetExclusiveZone(win, -1)
	gtk4layershell.SetKeyboardMode(win, gtk4layershell.LayerShellKeyboardModeExclusive)

	return nil
}
