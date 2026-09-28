package notify

import (
	"fmt"

	"phalune/internal/config"

	"github.com/diamondburned/gotk4-layer-shell/pkg/gtk4layershell"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

func ConfigureNotifySurface(win *gtk.Window, cfg config.NotificationsConfig) error {
	if !gtk4layershell.IsSupported() {
		return fmt.Errorf("Wayland compositor does not support wlr-layer-shell protocol")
	}

	if !gtk4layershell.IsLayerWindow(win) {
		gtk4layershell.InitForWindow(win)
	}
	gtk4layershell.SetNamespace(win, "phalune-notify")
	gtk4layershell.SetLayer(win, gtk4layershell.LayerShellLayerOverlay)

	anchor := cfg.Anchor
	if anchor == "" {
		anchor = "top-right"
	}
	top, bottom, left, right := config.ParseAnchor(anchor)
	if !top && !bottom && !left && !right && anchor != "center" && anchor != "middle" {
		top = true
		right = true
	}

	gtk4layershell.SetAnchor(win, gtk4layershell.LayerShellEdgeTop, top)
	gtk4layershell.SetAnchor(win, gtk4layershell.LayerShellEdgeBottom, bottom)
	gtk4layershell.SetAnchor(win, gtk4layershell.LayerShellEdgeLeft, left)
	gtk4layershell.SetAnchor(win, gtk4layershell.LayerShellEdgeRight, right)

	defY := 16
	if cfg.MarginTop > 0 {
		defY = cfg.MarginTop
	} else if cfg.MarginBottom > 0 {
		defY = cfg.MarginBottom
	}

	defX := 16
	if cfg.MarginRight > 0 {
		defX = cfg.MarginRight
	} else if cfg.MarginLeft > 0 {
		defX = cfg.MarginLeft
	}

	if top {
		m := cfg.MarginTop
		if m <= 0 {
			m = defY
		}
		gtk4layershell.SetMargin(win, gtk4layershell.LayerShellEdgeTop, m)
	}
	if bottom {
		m := cfg.MarginBottom
		if m <= 0 {
			m = defY
		}
		gtk4layershell.SetMargin(win, gtk4layershell.LayerShellEdgeBottom, m)
	}
	if left {
		m := cfg.MarginLeft
		if m <= 0 {
			m = defX
		}
		gtk4layershell.SetMargin(win, gtk4layershell.LayerShellEdgeLeft, m)
	}
	if right {
		m := cfg.MarginRight
		if m <= 0 {
			m = defX
		}
		gtk4layershell.SetMargin(win, gtk4layershell.LayerShellEdgeRight, m)
	}

	gtk4layershell.SetExclusiveZone(win, -1)
	gtk4layershell.SetKeyboardMode(win, gtk4layershell.LayerShellKeyboardModeNone)

	return nil
}
