package lockscreen

import (
	"log/slog"
	"os"
	"os/user"
	"sync"
	"time"

	"phalune/internal/config"
	"phalune/ui"

	"github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

type SessionActions interface {
	Suspend() error
	Reboot() error
	PowerOff() error
	SetLocked(bool)
}

type monitorWindow struct {
	window        *gtk.Window
	clockLabel    *gtk.Label
	dateLabel     *gtk.Label
	passwordEntry *gtk.PasswordEntry
	feedbackLabel *gtk.Label
	unlockBtn     *gtk.Button
}

type Manager struct {
	mu         sync.Mutex
	app        *gtk.Application
	cfg        config.LockScreenConfig
	sessionMgr SessionActions
	auth       Authenticator

	locked     bool
	verifying  bool
	windows    []*monitorWindow
	tickerStop chan struct{}
	username   string
	realName   string
}

func New(app *gtk.Application, cfg config.LockScreenConfig, sessionMgr SessionActions) *Manager {
	uName := os.Getenv("USER")
	rName := uName
	if u, err := user.Current(); err == nil {
		if u.Username != "" {
			uName = u.Username
		}
		if u.Name != "" {
			rName = u.Name
		} else {
			rName = uName
		}
	}

	timeFmt := cfg.TimeFormat
	if timeFmt == "" {
		timeFmt = "15:04"
	}
	dateFmt := cfg.DateFormat
	if dateFmt == "" {
		dateFmt = "Monday, January 2"
	}
	cfg.TimeFormat = timeFmt
	cfg.DateFormat = dateFmt

	return &Manager{
		app:        app,
		cfg:        cfg,
		sessionMgr: sessionMgr,
		auth:       NewUnixAuthenticator(cfg.AuthCommand),
		username:   uName,
		realName:   rName,
	}
}

func (m *Manager) SetAuthenticator(auth Authenticator) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.auth = auth
}

func (m *Manager) IsLocked() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.locked
}

func (m *Manager) Lock() {
	glib.IdleAdd(func() {
		m.mu.Lock()
		if m.locked {
			// Already locked, just present existing windows
			for _, mw := range m.windows {
				if mw.window != nil {
					mw.window.Present()
					if mw.passwordEntry != nil {
						mw.passwordEntry.GrabFocus()
					}
				}
			}
			m.mu.Unlock()
			return
		}
		m.locked = true
		m.mu.Unlock()

		if m.sessionMgr != nil {
			m.sessionMgr.SetLocked(true)
		}

		m.createWindows()
		m.startTicker()
	})
}

func (m *Manager) createWindows() {
	display := gdk.DisplayGetDefault()
	if display == nil {
		slog.Error("lockscreen: no default GDK display")
		return
	}

	monitorsList := display.Monitors()
	nMonitors := uint(0)
	if monitorsList != nil {
		nMonitors = monitorsList.NItems()
	}

	if nMonitors == 0 {
		mw := m.createMonitorWindow(nil)
		if mw != nil {
			m.windows = append(m.windows, mw)
			mw.window.Present()
			if mw.passwordEntry != nil {
				mw.passwordEntry.GrabFocus()
			}
		}
	} else {
		for i := uint(0); i < nMonitors; i++ {
			item := monitorsList.Item(i)
			if item == nil {
				continue
			}
			mon, ok := item.Cast().(*gdk.Monitor)
			if !ok {
				continue
			}
			mw := m.createMonitorWindow(mon)
			if mw != nil {
				m.windows = append(m.windows, mw)
				mw.window.Present()
				if i == 0 && mw.passwordEntry != nil {
					mw.passwordEntry.GrabFocus()
				}
			}
		}
	}

	m.updateClock()
}

func (m *Manager) createMonitorWindow(monitor *gdk.Monitor) *monitorWindow {
	win := gtk.NewWindow()
	if m.app != nil {
		win.SetApplication(m.app)
	}
	win.SetTitle("phalune-lockscreen")
	win.SetDecorated(false)
	win.AddCSSClass("lockscreen-window")

	if err := ConfigureLockScreenSurface(win, monitor); err != nil {
		slog.Error("lockscreen: failed to configure layer surface", "error", err)
	}

	builder := gtk.NewBuilderFromString(ui.LockScreen)
	overlayBox := builder.GetObject("overlay_box").Cast().(*gtk.Overlay)
	clockLabel := builder.GetObject("clock_label").Cast().(*gtk.Label)
	dateLabel := builder.GetObject("date_label").Cast().(*gtk.Label)
	userNameLabel := builder.GetObject("user_name").Cast().(*gtk.Label)
	passwordEntry := builder.GetObject("password_entry").Cast().(*gtk.PasswordEntry)
	feedbackLabel := builder.GetObject("feedback_label").Cast().(*gtk.Label)
	unlockBtn := builder.GetObject("unlock_button").Cast().(*gtk.Button)

	suspendBtn := builder.GetObject("suspend_btn").Cast().(*gtk.Button)
	rebootBtn := builder.GetObject("reboot_btn").Cast().(*gtk.Button)
	poweroffBtn := builder.GetObject("poweroff_btn").Cast().(*gtk.Button)

	displayName := m.realName
	if displayName == "" {
		displayName = m.username
	}
	userNameLabel.SetText(displayName)

	win.SetChild(overlayBox)

	mw := &monitorWindow{
		window:        win,
		clockLabel:    clockLabel,
		dateLabel:     dateLabel,
		passwordEntry: passwordEntry,
		feedbackLabel: feedbackLabel,
		unlockBtn:     unlockBtn,
	}

	// Action submit handlers
	submitAuth := func() {
		pwd := passwordEntry.Text()
		m.verifyPassword(pwd, mw)
	}

	passwordEntry.ConnectActivate(func() {
		submitAuth()
	})

	unlockBtn.ConnectClicked(func() {
		submitAuth()
	})

	// Keyboard shortcuts / interactivity on the window
	keyCtrl := gtk.NewEventControllerKey()
	keyCtrl.ConnectKeyPressed(func(keyval, keycode uint, state gdk.ModifierType) bool {
		switch keyval {
		case gdk.KEY_Escape:
			passwordEntry.SetText("")
			feedbackLabel.SetVisible(false)
			return true
		case gdk.KEY_Return, gdk.KEY_KP_Enter:
			submitAuth()
			return true
		default:
			// If key was pressed elsewhere, forward focus to password entry
			if !passwordEntry.HasFocus() {
				passwordEntry.GrabFocus()
			}
		}
		return false
	})
	win.AddController(keyCtrl)

	// Bottom action buttons
	suspendBtn.ConnectClicked(func() {
		if m.sessionMgr != nil {
			go func() {
				_ = m.sessionMgr.Suspend()
			}()
		}
	})

	rebootBtn.ConnectClicked(func() {
		if m.sessionMgr != nil {
			go func() {
				_ = m.sessionMgr.Reboot()
			}()
		}
	})

	poweroffBtn.ConnectClicked(func() {
		if m.sessionMgr != nil {
			go func() {
				_ = m.sessionMgr.PowerOff()
			}()
		}
	})

	return mw
}

func (m *Manager) updateClock() {
	now := time.Now()
	m.mu.Lock()
	timeFmt := m.cfg.TimeFormat
	dateFmt := m.cfg.DateFormat
	windows := m.windows
	m.mu.Unlock()

	timeStr := now.Format(timeFmt)
	dateStr := now.Format(dateFmt)

	for _, mw := range windows {
		if mw.clockLabel != nil {
			mw.clockLabel.SetText(timeStr)
		}
		if mw.dateLabel != nil {
			mw.dateLabel.SetText(dateStr)
		}
	}
}

func (m *Manager) startTicker() {
	m.stopTicker()

	stopCh := make(chan struct{})
	m.tickerStop = stopCh

	ticker := time.NewTicker(time.Second)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-stopCh:
				return
			case <-ticker.C:
				glib.IdleAdd(func() {
					m.mu.Lock()
					locked := m.locked
					m.mu.Unlock()
					if locked {
						m.updateClock()
					}
				})
			}
		}
	}()
}

func (m *Manager) stopTicker() {
	if m.tickerStop != nil {
		close(m.tickerStop)
		m.tickerStop = nil
	}
}

func (m *Manager) verifyPassword(password string, activeWindow *monitorWindow) {
	m.mu.Lock()
	if m.verifying {
		m.mu.Unlock()
		return
	}
	m.verifying = true
	auth := m.auth
	uName := m.username
	m.mu.Unlock()

	if activeWindow != nil && activeWindow.feedbackLabel != nil {
		activeWindow.feedbackLabel.SetText("Verifying…")
		activeWindow.feedbackLabel.RemoveCSSClass("lockscreen-error")
		activeWindow.feedbackLabel.SetVisible(true)
	}

	go func() {
		ok := false
		var authErr error
		if auth != nil {
			ok, authErr = auth.Authenticate(uName, password)
		}

		glib.IdleAdd(func() {
			m.mu.Lock()
			m.verifying = false
			m.mu.Unlock()

			if ok {
				slog.Info("lockscreen: authentication successful, unlocking")
				m.Unlock()
			} else {
				errMsg := "Incorrect password"
				if authErr != nil {
					slog.Warn("lockscreen: auth error", "error", authErr)
				}
				if activeWindow != nil {
					if activeWindow.feedbackLabel != nil {
						activeWindow.feedbackLabel.SetText(errMsg)
						activeWindow.feedbackLabel.AddCSSClass("lockscreen-error")
						activeWindow.feedbackLabel.SetVisible(true)
					}
					if activeWindow.passwordEntry != nil {
						activeWindow.passwordEntry.SetText("")
						activeWindow.passwordEntry.GrabFocus()
					}
				}
			}
		})
	}()
}

func (m *Manager) Unlock() {
	glib.IdleAdd(func() {
		m.mu.Lock()
		m.locked = false
		m.verifying = false
		windows := m.windows
		m.windows = nil
		m.mu.Unlock()

		m.stopTicker()

		for _, mw := range windows {
			if mw.window != nil {
				if mw.window.Realized() {
					mw.window.Destroy()
				}
				mw.window = nil
			}
		}

		if m.sessionMgr != nil {
			m.sessionMgr.SetLocked(false)
		}
	})
}

func (m *Manager) UpdateConfig(cfg config.LockScreenConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if cfg.TimeFormat == "" {
		cfg.TimeFormat = "15:04"
	}
	if cfg.DateFormat == "" {
		cfg.DateFormat = "Monday, January 2"
	}
	m.cfg = cfg
	if cfg.AuthCommand != "" {
		m.auth = NewUnixAuthenticator(cfg.AuthCommand)
	}
}

func (m *Manager) Destroy() {
	m.Unlock()
}
