package shell

import (
	"context"
	"fmt"
	"log/slog"
	"reflect"
	"sync"

	"phalune/internal/clipboard"
	"phalune/internal/compositor"
	"phalune/internal/config"
	"phalune/internal/controlcenter"
	"phalune/internal/launcher"
	"phalune/internal/lockscreen"
	"phalune/internal/notificationcenter"
	"phalune/internal/notify"
	"phalune/internal/osd"
	"phalune/internal/polkit"
	"phalune/internal/powermenu"
	"phalune/internal/privacy"
	"phalune/internal/removable"
	"phalune/internal/screenshot"
	"phalune/internal/session"
	"phalune/internal/shell/bar"
	"phalune/internal/widget"
	"phalune/internal/windowswitcher"

	"github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

type Shell struct {
	app                *gtk.Application
	cfg                *config.Config
	registry           *widget.Registry
	compositorSvc      compositor.Service
	barsByConnector    map[string]*bar.Bar
	fallbackBar        *bar.Bar
	launcher           *launcher.Launcher
	controlCenter      *controlcenter.ControlCenter
	notificationCenter *notificationcenter.NotificationCenter
	osdMgr             *osd.OSD
	osdListeners       *osd.ListenerController
	notifyMgr          *notify.Manager
	dbusServer         *notify.DBusServer
	privacyMonitor     *privacy.Monitor
	removableMonitor   *removable.Monitor
	ccCancel           context.CancelFunc
	sessionMgr         *session.Manager
	lockscreenMgr      *lockscreen.Manager
	powerMenu          *powermenu.PowerMenu
	windowSwitcher     *windowswitcher.WindowSwitcher
	clipboardWatcher   *clipboard.Watcher
	clipboardOverlay   *clipboard.Overlay
	screenshotSvc      *screenshot.Service
	screenshotToast    *screenshot.Toast
	polkitAgent        *polkit.Agent
	sessionCancel      context.CancelFunc
	configReloader     func()
	mu                 sync.Mutex
}

func New(app *gtk.Application, cfg *config.Config, registry *widget.Registry, compositorSvc compositor.Service) *Shell {
	return &Shell{
		app:             app,
		cfg:             cfg,
		registry:        registry,
		compositorSvc:   compositorSvc,
		barsByConnector: make(map[string]*bar.Bar),
	}
}

func (s *Shell) ToggleLauncher() {
	if s.launcher != nil {
		s.launcher.Toggle()
	}
}

func (s *Shell) OpenLauncher() {
	if s.launcher != nil {
		s.launcher.Open()
	}
}

func (s *Shell) CloseLauncher() {
	if s.launcher != nil {
		s.launcher.Close()
	}
}

func (s *Shell) ToggleControlCenter() {
	if s.controlCenter != nil {
		s.controlCenter.Toggle()
	}
}

func (s *Shell) OpenControlCenter() {
	if s.controlCenter != nil {
		s.controlCenter.Open()
	}
}

func (s *Shell) CloseControlCenter() {
	if s.controlCenter != nil {
		s.controlCenter.Close()
	}
}

func (s *Shell) OpenControlCenterSubpage(page string) {
	if s.controlCenter != nil {
		s.controlCenter.OpenSubpage(page)
	}
}

func (s *Shell) ToggleNotificationCenter() {
	if s.notificationCenter != nil {
		s.notificationCenter.Toggle()
	}
}

func (s *Shell) OpenNotificationCenter() {
	if s.notificationCenter != nil {
		s.notificationCenter.Open()
	}
}

func (s *Shell) CloseNotificationCenter() {
	if s.notificationCenter != nil {
		s.notificationCenter.Close()
	}
}

func (s *Shell) TogglePowerMenu() {
	if s.powerMenu != nil {
		s.powerMenu.Toggle()
	}
}

func (s *Shell) ToggleClipboard() {
	if s.clipboardOverlay != nil {
		s.clipboardOverlay.Toggle()
	}
}

func (s *Shell) OpenClipboard() {
	if s.clipboardOverlay != nil {
		s.clipboardOverlay.Open()
	}
}

func (s *Shell) CloseClipboard() {
	if s.clipboardOverlay != nil {
		s.clipboardOverlay.Close()
	}
}

func (s *Shell) CaptureScreenshot(mode string) {
	if s.screenshotSvc != nil {
		s.screenshotSvc.Capture(mode)
	}
}

func (s *Shell) OpenPowerMenu() {
	if s.powerMenu != nil {
		s.powerMenu.Open()
	}
}

func (s *Shell) ClosePowerMenu() {
	if s.powerMenu != nil {
		s.powerMenu.Close()
	}
}

func (s *Shell) ToggleWindowSwitcher() {
	if s.windowSwitcher != nil {
		s.windowSwitcher.Toggle()
	}
}

func (s *Shell) OpenWindowSwitcher() {
	if s.windowSwitcher != nil {
		s.windowSwitcher.Open()
	}
}

func (s *Shell) CloseWindowSwitcher() {
	if s.windowSwitcher != nil {
		s.windowSwitcher.Close()
	}
}

func (s *Shell) NextWindow() {
	if s.windowSwitcher != nil {
		s.windowSwitcher.Next()
	}
}

func (s *Shell) PrevWindow() {
	if s.windowSwitcher != nil {
		s.windowSwitcher.Prev()
	}
}

func (s *Shell) Lock() {
	if s.sessionMgr != nil {
		s.sessionMgr.Lock()
	} else if s.lockscreenMgr != nil {
		s.lockscreenMgr.Lock()
	}
}

func (s *Shell) Unlock() {
	if s.sessionMgr != nil {
		s.sessionMgr.Unlock()
	} else if s.lockscreenMgr != nil {
		s.lockscreenMgr.Unlock()
	}
}

func (s *Shell) IsLocked() bool {
	if s.lockscreenMgr != nil {
		return s.lockscreenMgr.IsLocked()
	}
	if s.sessionMgr != nil {
		return s.sessionMgr.IsLocked()
	}
	return false
}

type Status struct {
	Running       bool   `json:"running"`
	Locked        bool   `json:"locked"`
	Compositor    string `json:"compositor,omitempty"`
	Bars          int    `json:"bars"`
	DND           bool   `json:"dnd"`
	Notifications int    `json:"notifications"`
}

func (s *Shell) Status() Status {
	s.mu.Lock()
	barsCount := len(s.barsByConnector)
	if barsCount == 0 && s.fallbackBar != nil {
		barsCount = 1
	}
	compKind := ""
	if s.compositorSvc != nil {
		compKind = string(s.compositorSvc.Kind())
	}
	dnd := false
	notifs := 0
	if s.notifyMgr != nil {
		dnd = s.notifyMgr.IsDND()
		if store := s.notifyMgr.Store(); store != nil {
			notifs = store.Count()
		}
	}
	s.mu.Unlock()

	return Status{
		Running:       true,
		Locked:        s.IsLocked(),
		Compositor:    compKind,
		Bars:          barsCount,
		DND:           dnd,
		Notifications: notifs,
	}
}

func (s *Shell) Suspend() error {
	if s.sessionMgr != nil {
		return s.sessionMgr.Suspend()
	}
	return fmt.Errorf("session manager not available")
}

func (s *Shell) Hibernate() error {
	if s.sessionMgr != nil {
		return s.sessionMgr.Hibernate()
	}
	return fmt.Errorf("session manager not available")
}

func (s *Shell) Reboot() error {
	if s.sessionMgr != nil {
		return s.sessionMgr.Reboot()
	}
	return fmt.Errorf("session manager not available")
}

func (s *Shell) PowerOff() error {
	if s.sessionMgr != nil {
		return s.sessionMgr.PowerOff()
	}
	return fmt.Errorf("session manager not available")
}

func (s *Shell) Logout() error {
	if s.sessionMgr != nil {
		return s.sessionMgr.Logout()
	}
	return fmt.Errorf("session manager not available")
}

func (s *Shell) ReloadConfig() {
	s.mu.Lock()
	reloader := s.configReloader
	s.mu.Unlock()

	if reloader != nil {
		reloader()
		return
	}
	glib.IdleAdd(func() {
		if err := LoadStyle(s.cfg.Theme.Name, s.cfg.Theme.Values); err != nil {
			slog.Warn("launcher reload: style reload warning", "error", err)
		}
	})
}

func (s *Shell) SetConfigReloader(fn func()) {
	s.mu.Lock()
	s.configReloader = fn
	s.mu.Unlock()
}

func (s *Shell) ShowOSD(icon, label string, value float64) {
	if s.osdMgr != nil {
		s.osdMgr.Show(icon, label, value)
	}
}

func (s *Shell) TestOSD() {
	if s.osdMgr != nil {
		s.osdMgr.Show("audio-volume-high", "Volume", 70)
	}
}

func (s *Shell) Notify(n notify.Notification) {
	if s.notifyMgr != nil {
		s.notifyMgr.Show(n)
		return
	}

	// Notifications are not owned by phalune; safely forward to external daemon asynchronously
	go func() {
		_, _ = notify.SendDBus(n)
	}()
}

// NotifyLog implements logging.Notifier to display log messages as notifications.
func (s *Shell) NotifyLog(level slog.Level, title, message string) {
	urgency := notify.UrgencyNormal
	icon := "dialog-information"
	if level >= slog.LevelError {
		urgency = notify.UrgencyCritical
		icon = "dialog-error"
	} else if level >= slog.LevelWarn {
		urgency = notify.UrgencyNormal
		icon = "dialog-warning"
	}

	s.Notify(notify.Notification{
		AppName: "phalune",
		Summary: title,
		Body:    message,
		Icon:    icon,
		Urgency: urgency,
	})
}

func (s *Shell) TestNotify() {
	s.Notify(notify.Notification{
		AppName: "phalune",
		Summary: "Test Notification",
		Body:    "This is a test notification via org.freedesktop.Notifications.",
		Icon:    "dialog-information",
	})
}

func (s *Shell) Start() error {
	display := gdk.DisplayGetDefault()
	if display == nil {
		return fmt.Errorf("no default GDK display available")
	}

	osdInstance, err := osd.New(s.app, s.cfg.OSD)
	if err != nil {
		return fmt.Errorf("failed to create OSD: %w", err)
	}
	s.osdMgr = osdInstance
	s.osdListeners = osd.StartListeners(osdInstance)

	s.sessionMgr = session.New(s.cfg.Session, s.compositorSvc, func() {
		if s.lockscreenMgr != nil {
			s.lockscreenMgr.Lock()
		}
	}, func() {
		if s.lockscreenMgr != nil {
			s.lockscreenMgr.Unlock()
		}
	})
	sessionCtx, sessionCancel := context.WithCancel(context.Background())
	s.sessionCancel = sessionCancel
	s.sessionMgr.Start(sessionCtx)

	s.lockscreenMgr = lockscreen.New(s.app, s.cfg.LockScreen, s.sessionMgr)

	pmInstance, err := powermenu.New(s.app, s.cfg.PowerMenu, s.sessionMgr)
	if err != nil {
		slog.Warn("failed to create power menu", "error", err)
	} else {
		s.powerMenu = pmInstance
	}

	wsInstance, err := windowswitcher.New(s.app, s.cfg.WindowSwitcher, s.compositorSvc)
	if err != nil {
		slog.Warn("failed to create window switcher", "error", err)
	} else {
		s.windowSwitcher = wsInstance
	}

	// Notifications
	if notify.IsServiceOwned() {
		slog.Info("notification service is owned by another daemon; skipping phalune notification service")
	} else {
		notifyInstance, err := notify.New(s.app, s.cfg.Notifications)
		if err != nil {
			slog.Warn("notification UI warning", "error", err)
		} else {
			dbusSvr, err := notify.StartDBusServer(notifyInstance)
			if err != nil {
				slog.Warn("notification D-Bus service warning; leaving notification service disabled", "error", err)
				notifyInstance = nil
			} else {
				s.notifyMgr = notifyInstance
				s.dbusServer = dbusSvr
			}
		}
	}

	// Removable storage (USB/drive) notifications
	s.removableMonitor = removable.NewMonitor(s.notifyMgr)
	s.removableMonitor.Start()

	// Privacy monitor (microphone and camera usage)
	s.privacyMonitor = privacy.NewMonitor(s.ShowOSD)
	s.privacyMonitor.Start()

	// Notification Center
	ncInstance, err := notificationcenter.New(s.app, s.notifyMgr)
	if err != nil {
		slog.Warn("failed to create notification center", "error", err)
	} else {
		s.notificationCenter = ncInstance
	}

	// Control Center
	ccInstance, err := controlcenter.New(s.app, s.notifyMgr, s.cfg.ControlCenter)
	if err != nil {
		return fmt.Errorf("failed to create control center: %w", err)
	}
	s.controlCenter = ccInstance
	if s.osdMgr != nil && s.controlCenter != nil {
		s.controlCenter.SetShowOSD(func(icon, label string, value float64) {
			s.osdMgr.Show(icon, label, value)
		})
	}
	ccCtx, ccCancel := context.WithCancel(context.Background())
	s.ccCancel = ccCancel
	s.controlCenter.Start(ccCtx)

	launch, err := launcher.New(s.app, s.cfg.Launcher)
	if err != nil {
		return fmt.Errorf("failed to create launcher: %w", err)
	}
	launch.SetShellCommands(&launcher.ShellCommands{
		Reboot:    s.Reboot,
		PowerOff:  s.PowerOff,
		Suspend:   s.Suspend,
		Hibernate: s.Hibernate,
		Logout:    s.Logout,
		Lock:      s.Lock,
		Reload:    s.ReloadConfig,
		PowerMenu: s.TogglePowerMenu,
		Clipboard: s.ToggleClipboard,
	})
	s.launcher = launch

	// Clipboard history watcher + overlay.
	if s.cfg.Clipboard.MaxEntries != 0 || s.cfg.Clipboard.Persist || s.cfg.Clipboard.MaxImageBytes != 0 {
		clipWatcher, err := clipboard.NewWatcher(s.cfg.Clipboard)
		if err != nil {
			slog.Warn("shell: clipboard watcher unavailable", "error", err)
		} else {
			s.clipboardWatcher = clipWatcher
			overlay, err := clipboard.NewOverlay(s.app, clipWatcher)
			if err != nil {
				slog.Warn("shell: clipboard overlay unavailable", "error", err)
				clipWatcher.Stop()
				s.clipboardWatcher = nil
			} else {
				s.clipboardOverlay = overlay
			}
		}
	}

	// Screenshot tooling
	if scTools, toolsErr := screenshot.ResolveTools(); toolsErr != nil {
		slog.Warn("failed to resolve screenshot tools", "error", toolsErr)
	} else {
		scToast, err := screenshot.NewToast(s.app, s.cfg.Screenshot)
		if err != nil {
			slog.Warn("failed to create screenshot toast", "error", err)
		} else {
			scSvc, err := screenshot.New(s.cfg.Screenshot, s.compositorSvc, scToast, scTools)
			if err != nil {
				slog.Warn("failed to create screenshot service", "error", err)
				scToast.Destroy()
			} else {
				s.screenshotSvc = scSvc
				s.screenshotToast = scToast
			}
		}
	}

	// Polkit authentication agent
	pkAgent, err := polkit.New(s.app)
	if err != nil {
		slog.Warn("shell: polkit agent initialization failed", "error", err)
	} else if err := pkAgent.Start(); err != nil {
		slog.Warn("shell: polkit agent registration failed", "error", err)
	} else {
		s.polkitAgent = pkAgent
	}

	// Top bars with multi-monitor hotplug
	s.syncBars()
	monitorsList := display.Monitors()
	if monitorsList != nil {
		monitorsList.Connect("items-changed", func(pos, rem, add uint) {
			s.syncBars()
		})
	}

	return nil
}

func (s *Shell) buildWidgetContext(monitor *gdk.Monitor) widget.Context {
	connector := ""
	if monitor != nil {
		connector = monitor.Connector()
	}

	var store widget.NotificationStore
	if s.notifyMgr != nil {
		store = s.notifyMgr.Store()
	}

	return widget.Context{
		Config:                   s.cfg,
		Compositor:               s.compositorSvc,
		Output:                   connector,
		ShowOSD:                  s.ShowOSD,
		TogglePowerMenu:          s.TogglePowerMenu,
		OpenPowerMenu:            s.OpenPowerMenu,
		ClosePowerMenu:           s.ClosePowerMenu,
		ToggleControlCenter:      s.ToggleControlCenter,
		OpenControlCenterSubpage: s.OpenControlCenterSubpage,
		ToggleNotificationCenter: s.ToggleNotificationCenter,
		OpenNotificationCenter:   s.OpenNotificationCenter,
		CloseNotificationCenter:  s.CloseNotificationCenter,
		ToggleClipboard:          s.ToggleClipboard,
		OpenClipboard:            s.OpenClipboard,
		CloseClipboard:           s.CloseClipboard,
		NotifyStore:              store,
		Privacy:                  s.privacyMonitor,
	}
}

func (s *Shell) syncBars() {
	glib.IdleAdd(func() {
		display := gdk.DisplayGetDefault()
		if display == nil {
			return
		}
		monitorsList := display.Monitors()
		if monitorsList == nil {
			return
		}
		nMonitors := monitorsList.NItems()

		currentMonitors := make(map[string]*gdk.Monitor)
		for i := uint(0); i < nMonitors; i++ {
			item := monitorsList.Item(i)
			if item == nil {
				continue
			}
			m, ok := item.Cast().(*gdk.Monitor)
			if ok {
				currentMonitors[m.Connector()] = m
			}
		}

		s.mu.Lock()
		defer s.mu.Unlock()

		for conn, b := range s.barsByConnector {
			if _, exists := currentMonitors[conn]; !exists {
				b.Destroy()
				delete(s.barsByConnector, conn)
			}
		}

		for conn, m := range currentMonitors {
			if _, exists := s.barsByConnector[conn]; !exists {
				ctx := s.buildWidgetContext(m)
				b, err := bar.New(s.app, m, s.cfg, s.registry, ctx)
				if err == nil {
					s.barsByConnector[conn] = b
					b.Present()
				} else {
					slog.Error("failed to create bar for monitor", "connector", conn, "error", err)
				}
			}
		}

		if len(currentMonitors) == 0 && s.fallbackBar == nil {
			ctx := s.buildWidgetContext(nil)
			b, err := bar.New(s.app, nil, s.cfg, s.registry, ctx)
			if err == nil {
				s.fallbackBar = b
				b.Present()
			}
		} else if len(currentMonitors) > 0 && s.fallbackBar != nil {
			s.fallbackBar.Destroy()
			s.fallbackBar = nil
		}
	})
}

func (s *Shell) Stop() {
	if s.ccCancel != nil {
		s.ccCancel()
		s.ccCancel = nil
	}

	if s.sessionCancel != nil {
		s.sessionCancel()
		s.sessionCancel = nil
	}

	if s.sessionMgr != nil {
		s.sessionMgr.Close()
		s.sessionMgr = nil
	}

	if s.lockscreenMgr != nil {
		s.lockscreenMgr.Destroy()
		s.lockscreenMgr = nil
	}

	if s.powerMenu != nil {
		s.powerMenu.Destroy()
		s.powerMenu = nil
	}

	if s.controlCenter != nil {
		s.controlCenter.Destroy()
		s.controlCenter = nil
	}

	if s.notificationCenter != nil {
		s.notificationCenter.Destroy()
		s.notificationCenter = nil
	}

	if s.launcher != nil {
		s.launcher.Destroy()
		s.launcher = nil
	}

	if s.clipboardOverlay != nil {
		s.clipboardOverlay.Destroy()
		s.clipboardOverlay = nil
	}
	if s.clipboardWatcher != nil {
		s.clipboardWatcher.Stop()
		s.clipboardWatcher = nil
	}

	if s.screenshotSvc != nil {
		s.screenshotSvc = nil
	}
	if s.screenshotToast != nil {
		s.screenshotToast.Destroy()
		s.screenshotToast = nil
	}

	if s.polkitAgent != nil {
		s.polkitAgent.Stop()
		s.polkitAgent = nil
	}

	if s.windowSwitcher != nil {
		s.windowSwitcher.Destroy()
		s.windowSwitcher = nil
	}

	if s.removableMonitor != nil {
		s.removableMonitor.Stop()
		s.removableMonitor = nil
	}

	if s.privacyMonitor != nil {
		s.privacyMonitor.Stop()
		s.privacyMonitor = nil
	}

	if s.osdListeners != nil {
		s.osdListeners.Stop()
		s.osdListeners = nil
	}

	if s.osdMgr != nil {
		s.osdMgr.Destroy()
		s.osdMgr = nil
	}

	if s.dbusServer != nil {
		_ = s.dbusServer.Close()
		s.dbusServer = nil
	}

	if s.notifyMgr != nil {
		s.notifyMgr.Destroy()
		s.notifyMgr = nil
	}

	s.mu.Lock()
	for conn, b := range s.barsByConnector {
		b.Destroy()
		delete(s.barsByConnector, conn)
	}
	if s.fallbackBar != nil {
		s.fallbackBar.Destroy()
		s.fallbackBar = nil
	}
	s.mu.Unlock()
}

// Reload re-applies configuration on the fly. It MUST be called on the GTK main loop.
func (s *Shell) Reload(newCfg *config.Config) error {
	if newCfg == nil {
		return fmt.Errorf("new config is nil")
	}
	oldCfg := s.cfg
	s.cfg = newCfg

	if err := LoadStyle(newCfg.Theme.Name, newCfg.Theme.Values); err != nil {
		slog.Warn("reload: style reload warning", "error", err)
	}

	if s.osdMgr != nil {
		if err := s.osdMgr.UpdateConfig(newCfg.OSD); err != nil {
			slog.Warn("reload: OSD config update warning", "error", err)
		}
	}

	if s.notifyMgr != nil {
		if err := s.notifyMgr.UpdateConfig(newCfg.Notifications); err != nil {
			slog.Warn("reload: notifications config update warning", "error", err)
		}
	}

	if s.launcher != nil {
		s.launcher.UpdateConfig(newCfg.Launcher)
	}

	if s.controlCenter != nil {
		s.controlCenter.UpdateConfig(newCfg.ControlCenter)
	}

	if s.clipboardWatcher != nil {
		s.clipboardWatcher.SetConfig(newCfg.Clipboard)
	}

	if s.screenshotSvc != nil {
		s.screenshotSvc.UpdateConfig(newCfg.Screenshot)
	}

	if s.sessionMgr != nil {
		s.sessionMgr.UpdateConfig(newCfg.Session)
	}
	if s.lockscreenMgr != nil {
		s.lockscreenMgr.UpdateConfig(newCfg.LockScreen)
	}
	if s.powerMenu != nil {
		s.powerMenu.UpdateConfig(newCfg.PowerMenu)
	}
	if s.windowSwitcher != nil {
		s.windowSwitcher.UpdateConfig(newCfg.WindowSwitcher)
	}

	if oldCfg == nil || !reflect.DeepEqual(oldCfg.Bar, newCfg.Bar) {
		s.mu.Lock()
		for conn, b := range s.barsByConnector {
			b.Destroy()
			delete(s.barsByConnector, conn)
		}
		if s.fallbackBar != nil {
			s.fallbackBar.Destroy()
			s.fallbackBar = nil
		}
		s.mu.Unlock()

		s.syncBars()
	}

	slog.Info("shell: configuration hot-reloaded successfully")
	return nil
}
