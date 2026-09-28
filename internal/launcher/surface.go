package launcher

import (
	"fmt"

	"github.com/diamondburned/gotk4-layer-shell/pkg/gtk4layershell"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

func ConfigureLauncherSurface(win *gtk.Window) error {
	if !gtk4layershell.IsSupported() {
		return fmt.Errorf("Wayland compositor does not support wlr-layer-shell protocol")
	}

	gtk4layershell.InitForWindow(win)
	gtk4layershell.SetNamespace(win, "phalune-launcher")
	gtk4layershell.SetLayer(win, gtk4layershell.LayerShellLayerOverlay)

	gtk4layershell.SetAnchor(win, gtk4layershell.LayerShellEdgeTop, true)
	gtk4layershell.SetAnchor(win, gtk4layershell.LayerShellEdgeBottom, true)
	gtk4layershell.SetAnchor(win, gtk4layershell.LayerShellEdgeLeft, true)
	gtk4layershell.SetAnchor(win, gtk4layershell.LayerShellEdgeRight, true)

	gtk4layershell.SetExclusiveZone(win, -1)
	gtk4layershell.SetKeyboardMode(win, gtk4layershell.LayerShellKeyboardModeOnDemand)

	return nil
}

