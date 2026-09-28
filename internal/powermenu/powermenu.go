package powermenu

import (
	"sync"

	"phalune/internal/config"
	"phalune/ui"

	"github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

type SessionActions interface {
	Lock()
	Suspend() error
	Hibernate() error
	Logout() error
	Reboot() error
	PowerOff() error
}

type PowerMenu struct {
	window     *gtk.Window
	overlayBox *gtk.Overlay
	card       *gtk.Box

	lockBtn      *gtk.Button
	suspendBtn   *gtk.Button
	hibernateBtn *gtk.Button
	logoutBtn    *gtk.Button
	rebootBtn    *gtk.Button
	shutdownBtn  *gtk.Button
	cancelBtn    *gtk.Button

	sessionMgr SessionActions
	mu         sync.Mutex
	visible    bool
}

func New(app *gtk.Application, cfg config.PowerMenuConfig, sessionMgr SessionActions) (*PowerMenu, error) {
	win := gtk.NewWindow()
	win.SetApplication(app)
	win.SetTitle("phalune-powermenu")
	win.SetDecorated(false)
	win.AddCSSClass("powermenu-window")

	if err := ConfigurePowerMenuSurface(win); err != nil {
		// Log warning (e.g. in headless tests or without layer-shell support)
	}

	builder := gtk.NewBuilderFromString(ui.PowerMenu)
	overlayBox := builder.GetObject("overlay_box").Cast().(*gtk.Overlay)
	card := builder.GetObject("card").Cast().(*gtk.Box)

	lockBtn := builder.GetObject("lock_button").Cast().(*gtk.Button)
	suspendBtn := builder.GetObject("suspend_button").Cast().(*gtk.Button)
	hibernateBtn := builder.GetObject("hibernate_button").Cast().(*gtk.Button)
	logoutBtn := builder.GetObject("logout_button").Cast().(*gtk.Button)
	rebootBtn := builder.GetObject("reboot_button").Cast().(*gtk.Button)
	shutdownBtn := builder.GetObject("shutdown_button").Cast().(*gtk.Button)
	cancelBtn := builder.GetObject("cancel_button").Cast().(*gtk.Button)

	win.SetChild(overlayBox)

	pm := &PowerMenu{
		window:       win,
		overlayBox:   overlayBox,
		card:         card,
		lockBtn:      lockBtn,
		suspendBtn:   suspendBtn,
		hibernateBtn: hibernateBtn,
		logoutBtn:    logoutBtn,
		rebootBtn:    rebootBtn,
		shutdownBtn:  shutdownBtn,
		cancelBtn:    cancelBtn,
		sessionMgr:   sessionMgr,
	}

	if !cfg.ShowHibernate {
		hibernateBtn.SetVisible(false)
	}

	pm.setupInteractivity()

	return pm, nil
}

func (pm *PowerMenu) setupInteractivity() {
	// Dismiss on outside click
	click := gtk.NewGestureClick()
	click.ConnectReleased(func(n int, x, y float64) {
		pick := pm.overlayBox.Pick(x, y, gtk.PickDefault)
		if pick != nil {
			w := gtk.BaseWidget(pick)
			if w != nil && (w == &pm.card.Widget || w.IsAncestor(pm.card)) {
				return
			}
		}
		pm.Close()
	})
	pm.overlayBox.AddController(click)

	// Button actions
	pm.lockBtn.ConnectClicked(func() {
		pm.Close()
		if pm.sessionMgr != nil {
			pm.sessionMgr.Lock()
		}
	})

	pm.suspendBtn.ConnectClicked(func() {
		pm.Close()
		if pm.sessionMgr != nil {
			go func() {
				_ = pm.sessionMgr.Suspend()
			}()
		}
	})

	pm.hibernateBtn.ConnectClicked(func() {
		pm.Close()
		if pm.sessionMgr != nil {
			go func() {
				_ = pm.sessionMgr.Hibernate()
			}()
		}
	})

	pm.logoutBtn.ConnectClicked(func() {
		pm.Close()
		if pm.sessionMgr != nil {
			go func() {
				_ = pm.sessionMgr.Logout()
			}()
		}
	})

	pm.rebootBtn.ConnectClicked(func() {
		pm.Close()
		if pm.sessionMgr != nil {
			go func() {
				_ = pm.sessionMgr.Reboot()
			}()
		}
	})

	pm.shutdownBtn.ConnectClicked(func() {
		pm.Close()
		if pm.sessionMgr != nil {
			go func() {
				_ = pm.sessionMgr.PowerOff()
			}()
		}
	})

	pm.cancelBtn.ConnectClicked(func() {
		pm.Close()
	})

	// Keyboard shortcuts
	keyCtrl := gtk.NewEventControllerKey()
	keyCtrl.ConnectKeyPressed(func(keyval, keycode uint, state gdk.ModifierType) bool {
		switch keyval {
		case gdk.KEY_Escape:
			pm.Close()
			return true
		case gdk.KEY_l, gdk.KEY_L, gdk.KEY_1:
			pm.Close()
			if pm.sessionMgr != nil {
				pm.sessionMgr.Lock()
			}
			return true
		case gdk.KEY_s, gdk.KEY_S, gdk.KEY_2:
			pm.Close()
			if pm.sessionMgr != nil {
				go func() { _ = pm.sessionMgr.Suspend() }()
			}
			return true
		case gdk.KEY_h, gdk.KEY_H, gdk.KEY_3:
			pm.Close()
			if pm.sessionMgr != nil {
				go func() { _ = pm.sessionMgr.Hibernate() }()
			}
			return true
		case gdk.KEY_o, gdk.KEY_O, gdk.KEY_4:
			pm.Close()
			if pm.sessionMgr != nil {
				go func() { _ = pm.sessionMgr.Logout() }()
			}
			return true
		case gdk.KEY_r, gdk.KEY_R, gdk.KEY_5:
			pm.Close()
			if pm.sessionMgr != nil {
				go func() { _ = pm.sessionMgr.Reboot() }()
			}
			return true
		case gdk.KEY_p, gdk.KEY_P, gdk.KEY_6:
			pm.Close()
			if pm.sessionMgr != nil {
				go func() { _ = pm.sessionMgr.PowerOff() }()
			}
			return true
		}
		return false
	})
	pm.window.AddController(keyCtrl)
}

func (pm *PowerMenu) Open() {
	glib.IdleAdd(func() {
		pm.mu.Lock()
		pm.visible = true
		pm.mu.Unlock()

		pm.window.SetVisible(true)
		pm.window.Present()
		pm.lockBtn.GrabFocus()
	})
}

func (pm *PowerMenu) Close() {
	glib.IdleAdd(func() {
		pm.mu.Lock()
		pm.visible = false
		pm.mu.Unlock()

		pm.window.SetVisible(false)
	})
}

func (pm *PowerMenu) Toggle() {
	pm.mu.Lock()
	vis := pm.visible
	pm.mu.Unlock()

	if vis {
		pm.Close()
	} else {
		pm.Open()
	}
}

func (pm *PowerMenu) IsVisible() bool {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	return pm.visible
}

func (pm *PowerMenu) UpdateConfig(cfg config.PowerMenuConfig) {
	glib.IdleAdd(func() {
		pm.hibernateBtn.SetVisible(cfg.ShowHibernate)
	})
}

func (pm *PowerMenu) Destroy() {
	pm.Close()
	glib.IdleAdd(func() {
		if pm.window != nil {
			if pm.window.Realized() {
				pm.window.Destroy()
			}
			pm.window = nil
		}
	})
}
