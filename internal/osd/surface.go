package osd

import (
	"fmt"

	"phalune/internal/config"

	"github.com/diamondburned/gotk4-layer-shell/pkg/gtk4layershell"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

func ConfigureOSDSurface(win *gtk.Window, cfg config.OSDConfig) error {
	if !gtk4layershell.IsSupported() {
		return fmt.Errorf("Wayland compositor does not support wlr-layer-shell protocol")
	}

	if !gtk4layershell.IsLayerWindow(win) {
		gtk4layershell.InitForWindow(win)
	}
	gtk4layershell.SetNamespace(win, "phalune-osd")
	gtk4layershell.SetLayer(win, gtk4layershell.LayerShellLayerOverlay)

	anchor := cfg.Anchor
	if anchor == "" {
		anchor = "bottom"
	}
	top, bottom, left, right := config.ParseAnchor(anchor)
	// If anchor wasn't explicitly center/middle and matched nothing, default to bottom
	if !top && !bottom && !left && !right && anchor != "center" && anchor != "middle" {
		bottom = true
	}

	gtk4layershell.SetAnchor(win, gtk4layershell.LayerShellEdgeTop, top)
	gtk4layershell.SetAnchor(win, gtk4layershell.LayerShellEdgeBottom, bottom)
	gtk4layershell.SetAnchor(win, gtk4layershell.LayerShellEdgeLeft, left)
	gtk4layershell.SetAnchor(win, gtk4layershell.LayerShellEdgeRight, right)

	defMargin := 64
	if cfg.MarginBottom > 0 {
		defMargin = cfg.MarginBottom
	} else if cfg.MarginTop > 0 {
		defMargin = cfg.MarginTop
	}

	if top {
		m := cfg.MarginTop
		if m <= 0 {
			m = defMargin
		}
		gtk4layershell.SetMargin(win, gtk4layershell.LayerShellEdgeTop, m)
	}
	if bottom {
		m := cfg.MarginBottom
		if m <= 0 {
			m = defMargin
		}
		gtk4layershell.SetMargin(win, gtk4layershell.LayerShellEdgeBottom, m)
	}
	if left {
		m := cfg.MarginLeft
		if m <= 0 && cfg.MarginRight > 0 {
			m = cfg.MarginRight
		}
		if m > 0 {
			gtk4layershell.SetMargin(win, gtk4layershell.LayerShellEdgeLeft, m)
		}
	}
	if right {
		m := cfg.MarginRight
		if m <= 0 && cfg.MarginLeft > 0 {
			m = cfg.MarginLeft
		}
		if m > 0 {
			gtk4layershell.SetMargin(win, gtk4layershell.LayerShellEdgeRight, m)
		}
	}

	gtk4layershell.SetExclusiveZone(win, -1)
	gtk4layershell.SetKeyboardMode(win, gtk4layershell.LayerShellKeyboardModeNone)

	return nil
}
