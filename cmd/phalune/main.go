package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"reflect"
	"strings"
	"syscall"
	"time"

	"phalune/internal/compositor"
	"phalune/internal/compositor/detect"
	"phalune/internal/config"
	"phalune/internal/ipc"
	"phalune/internal/logging"
	"phalune/internal/notify"
	"phalune/internal/shell"
	"phalune/internal/widget"
	"phalune/internal/widget/audio"
	"phalune/internal/widget/battery"
	"phalune/internal/widget/bluetooth"
	widgetClipboard "phalune/internal/widget/clipboard"
	"phalune/internal/widget/clock"
	widgetCustom "phalune/internal/widget/custom"
	widgetIPCX "phalune/internal/widget/ipcx"
	"phalune/internal/widget/keyboard"
	widgetNotifications "phalune/internal/widget/notifications"
	"phalune/internal/widget/power"
	widgetPrivacy "phalune/internal/widget/privacy"
	"phalune/internal/widget/tray"
	"phalune/internal/widget/wifi"
	"phalune/internal/widget/workspaces"

	"github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "msg" {
		os.Exit(ipc.RunClient(os.Args[2:]))
	}

	// Layer-shell strictly requires the Wayland backend.
	if err := os.Setenv("GDK_BACKEND", "wayland"); err != nil {
		fmt.Fprintf(os.Stderr, "failed to set GDK_BACKEND=wayland: %v\n", err)
	}

	configPath := flag.String("config", "", "path to config.toml")
	logLevelFlag := flag.String("log-level", "", "console log level (debug, info, warn, error)")
	notifyLogsFlag := flag.Bool("notify-logs", false, "show logs as desktop notifications")
	notifyLogLevelFlag := flag.String("notify-log-level", "", "notification log level (debug, info, warn, error)")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	if *logLevelFlag != "" {
		cfg.Logging.ConsoleLevel = *logLevelFlag
	}
	if *notifyLogsFlag {
		cfg.Logging.Notify = true
	}
	if *notifyLogLevelFlag != "" {
		cfg.Logging.NotifyLevel = *notifyLogLevelFlag
	}

	logHandler := logging.Init(logging.Config{
		Level:        cfg.Logging.Level,
		ConsoleLevel: cfg.Logging.ConsoleLevel,
		Notify:       cfg.Logging.Notify,
		NotifyLevel:  cfg.Logging.NotifyLevel,
	}, os.Stderr)

	var compositorSvc compositor.Service
	compositorKind, svc, err := detect.Detect()
	if err != nil {
		slog.Warn("compositor: no backend detected; bar widgets will be disabled", "error", err)
	} else {
		compositorSvc = svc
		slog.Info("compositor: backend detected", "kind", string(compositorKind))
	}

	reg := widget.NewRegistry()
	reg.Register("clock", clock.New)
	reg.Register("workspaces", workspaces.New)
	reg.Register("audio", audio.New)
	reg.Register("battery", battery.New)
	reg.Register("tray", tray.New)
	reg.Register("bluetooth", bluetooth.New)
	reg.Register("wifi", wifi.New)
	reg.Register("network", wifi.New)
	reg.Register("keyboard", keyboard.New)
	reg.Register("power", power.New)
	reg.Register("clipboard", widgetClipboard.New)
	reg.Register("notifications", widgetNotifications.New)
	reg.Register("privacy", widgetPrivacy.New)
	reg.RegisterPrefix("custom.", widgetCustom.New)

	eventBus := ipc.NewEventBus()
	widgetHub := ipc.NewWidgetHub()
	reg.RegisterPrefix("ipc:", widgetIPCX.New)

	app := gtk.NewApplication("org.phalune.shell", gio.ApplicationNonUnique)

	var sh *shell.Shell
	var ipcServer *ipc.Server
	var cfgWatcher *config.Watcher

	var lastReloadTime time.Time
	var lastReloadCfg *config.Config

	doReload := func(newCfg *config.Config) error {
		if newCfg == nil {
			var err error
			newCfg, err = config.Load(*configPath)
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}
		}

		now := time.Now()
		if lastReloadCfg != nil && now.Sub(lastReloadTime) < 500*time.Millisecond && reflect.DeepEqual(lastReloadCfg, newCfg) {
			slog.Debug("config: ignoring duplicate reload request")
			return nil
		}
		lastReloadTime = now
		lastReloadCfg = newCfg

		// Update logging configuration if specified
		if newCfg.Logging.ConsoleLevel != "" {
			if lvl, err := logging.ParseLevel(newCfg.Logging.ConsoleLevel); err == nil {
				logHandler.SetConsoleLevel(lvl)
			}
		}
		if newCfg.Logging.NotifyLevel != "" {
			if lvl, err := logging.ParseLevel(newCfg.Logging.NotifyLevel); err == nil {
				logHandler.SetNotifyLevel(lvl)
			}
		}
		logHandler.SetNotifyEnabled(newCfg.Logging.Notify)

		if sh != nil {
			return sh.Reload(newCfg)
		}
		return nil
	}

	ipcHandler := func(req ipc.Request) ipc.Response {
		respCh := make(chan ipc.Response, 1)
		glib.IdleAdd(func() {
			if sh == nil {
				respCh <- ipc.Response{OK: false, Error: "shell not ready"}
				return
			}
			switch req.Action {
			case ipc.ActionToggleLauncher, "launcher-toggle":
				sh.ToggleLauncher()
				respCh <- ipc.Response{OK: true, Message: "launcher toggled"}
			case ipc.ActionOpenLauncher, "launcher-open":
				sh.OpenLauncher()
				respCh <- ipc.Response{OK: true, Message: "launcher opened"}
			case ipc.ActionCloseLauncher, "launcher-close":
				sh.CloseLauncher()
				respCh <- ipc.Response{OK: true, Message: "launcher closed"}
			case ipc.ActionToggleControlCenter, "control-center-toggle":
				sh.ToggleControlCenter()
				respCh <- ipc.Response{OK: true, Message: "control center toggled"}
			case ipc.ActionOpenControlCenter, "control-center-open":
				sh.OpenControlCenter()
				respCh <- ipc.Response{OK: true, Message: "control center opened"}
			case ipc.ActionCloseControlCenter, "control-center-close":
				sh.CloseControlCenter()
				respCh <- ipc.Response{OK: true, Message: "control center closed"}
			case ipc.ActionOpenWifi, "wifi-open":
				sh.OpenControlCenterSubpage("wifi")
				respCh <- ipc.Response{OK: true, Message: "wifi subview opened"}
			case ipc.ActionOpenBluetooth, "bluetooth-open":
				sh.OpenControlCenterSubpage("bluetooth")
				respCh <- ipc.Response{OK: true, Message: "bluetooth subview opened"}
			case ipc.ActionToggleNotificationCenter, "notification-center-toggle", "notifications-toggle":
				sh.ToggleNotificationCenter()
				respCh <- ipc.Response{OK: true, Message: "notification center toggled"}
			case ipc.ActionOpenNotificationCenter, "notification-center-open", "notifications-open":
				sh.OpenNotificationCenter()
				respCh <- ipc.Response{OK: true, Message: "notification center opened"}
			case ipc.ActionCloseNotificationCenter, "notification-center-close", "notifications-close":
				sh.CloseNotificationCenter()
				respCh <- ipc.Response{OK: true, Message: "notification center closed"}
			case ipc.ActionTogglePowerMenu, "power-menu-toggle", "powermenu-toggle":
				sh.TogglePowerMenu()
				respCh <- ipc.Response{OK: true, Message: "power menu toggled"}
			case ipc.ActionOpenPowerMenu, "power-menu-open", "powermenu-open":
				sh.OpenPowerMenu()
				respCh <- ipc.Response{OK: true, Message: "power menu opened"}
			case ipc.ActionClosePowerMenu, "power-menu-close", "powermenu-close":
				sh.ClosePowerMenu()
				respCh <- ipc.Response{OK: true, Message: "power menu closed"}
			case ipc.ActionToggleClipboard, "clipboard-toggle", "clipboard":
				sh.ToggleClipboard()
				respCh <- ipc.Response{OK: true, Message: "clipboard toggled"}
			case ipc.ActionOpenClipboard, "clipboard-open":
				sh.OpenClipboard()
				respCh <- ipc.Response{OK: true, Message: "clipboard opened"}
			case ipc.ActionCloseClipboard, "clipboard-close":
				sh.CloseClipboard()
				respCh <- ipc.Response{OK: true, Message: "clipboard closed"}
			case ipc.ActionWindowSwitcher, "alt-tab", "switch-window":
				sub := req.Args["action"]
				switch sub {
				case "prev", "backward", "back":
					sh.PrevWindow()
					respCh <- ipc.Response{OK: true, Message: "switched to previous window"}
				case "next", "forward":
					sh.NextWindow()
					respCh <- ipc.Response{OK: true, Message: "switched to next window"}
				case "close", "cancel":
					sh.CloseWindowSwitcher()
					respCh <- ipc.Response{OK: true, Message: "window switcher closed"}
				case "open":
					sh.OpenWindowSwitcher()
					respCh <- ipc.Response{OK: true, Message: "window switcher opened"}
				case "toggle", "":
					sh.ToggleWindowSwitcher()
					respCh <- ipc.Response{OK: true, Message: "window switcher toggled"}
				default:
					sh.ToggleWindowSwitcher()
					respCh <- ipc.Response{OK: true, Message: "window switcher toggled"}
				}
			case ipc.ActionToggleWindowSwitcher, "window-switcher-toggle", "window-toggle":
				sh.ToggleWindowSwitcher()
				respCh <- ipc.Response{OK: true, Message: "window switcher toggled"}
			case ipc.ActionOpenWindowSwitcher, "window-switcher-open", "window-open":
				sh.OpenWindowSwitcher()
				respCh <- ipc.Response{OK: true, Message: "window switcher opened"}
			case ipc.ActionCloseWindowSwitcher, "window-switcher-close", "window-close":
				sh.CloseWindowSwitcher()
				respCh <- ipc.Response{OK: true, Message: "window switcher closed"}
			case ipc.ActionNextWindow, "window-next":
				sh.NextWindow()
				respCh <- ipc.Response{OK: true, Message: "switched to next window"}
			case ipc.ActionPrevWindow, "window-prev":
				sh.PrevWindow()
				respCh <- ipc.Response{OK: true, Message: "switched to previous window"}
			case ipc.ActionLock, "lock-screen", "screen-lock":
				sh.Lock()
				respCh <- ipc.Response{OK: true, Message: "screen locked"}
			case ipc.ActionUnlock, "unlock-screen":
				sh.Unlock()
				respCh <- ipc.Response{OK: true, Message: "screen unlocked"}
			case ipc.ActionStatus, "shell-status":
				st := sh.Status()
				data, _ := json.Marshal(st)
				comp := st.Compositor
				if comp == "" {
					comp = "none"
				}
				msg := fmt.Sprintf("status: running\nlocked: %v\ncompositor: %s\nbars: %d\ndnd: %v\nnotifications: %d",
					st.Locked, comp, st.Bars, st.DND, st.Notifications)
				respCh <- ipc.Response{
					OK:      true,
					Message: msg,
					Data:    data,
				}
			case ipc.ActionIsLocked, "lock-status":
				locked := sh.IsLocked()
				data, _ := json.Marshal(map[string]any{"locked": locked})
				respCh <- ipc.Response{
					OK:      true,
					Message: fmt.Sprintf("%v", locked),
					Data:    data,
				}
			case ipc.ActionSuspend, "sleep":
				go func() { _ = sh.Suspend() }()
				respCh <- ipc.Response{OK: true, Message: "suspending"}
			case ipc.ActionHibernate:
				go func() { _ = sh.Hibernate() }()
				respCh <- ipc.Response{OK: true, Message: "hibernating"}
			case ipc.ActionReboot, "restart":
				go func() { _ = sh.Reboot() }()
				respCh <- ipc.Response{OK: true, Message: "rebooting"}
			case ipc.ActionPowerOff, "shutdown":
				go func() { _ = sh.PowerOff() }()
				respCh <- ipc.Response{OK: true, Message: "shutting down"}
			case ipc.ActionLogout, "exit-session":
				go func() { _ = sh.Logout() }()
				respCh <- ipc.Response{OK: true, Message: "logging out"}
			case ipc.ActionScreenshot, "screenshot-area", "screenshot-window", "screenshot-display", "screenshot-screen":
				mode := req.Args["mode"]
				if mode == "" && strings.HasPrefix(req.Action, "screenshot-") {
					mode = strings.TrimPrefix(req.Action, "screenshot-")
				}
				if mode == "screen" {
					mode = "display"
				}
				sh.CaptureScreenshot(mode)
				respCh <- ipc.Response{OK: true, Message: "screenshot capture initiated"}
			case ipc.ActionTestOSD, "osd-test":
				sh.TestOSD()
				respCh <- ipc.Response{OK: true, Message: "OSD test shown"}
			case ipc.ActionShowOSD:
				osdType := req.Args["type"]
				valStr := req.Args["value"]
				var val float64
				var icon, label string
				if osdType == "brightness" {
					icon = "display-brightness"
					label = "Brightness"
				} else {
					icon = "audio-volume-high"
					label = "Volume"
				}
				if valStr != "" {
					fmt.Sscanf(valStr, "%f", &val)
				}
				if label == "Volume" && val <= 0 {
					icon = "audio-volume-muted"
				}
				sh.ShowOSD(icon, label, val)
				respCh <- ipc.Response{OK: true, Message: fmt.Sprintf("OSD %s set to %.0f%%", label, val)}
			case ipc.ActionTestNotify, "notify-test":
				sh.TestNotify()
				respCh <- ipc.Response{OK: true, Message: "notification test sent"}
			case ipc.ActionNotify:
				summary := req.Args["summary"]
				body := req.Args["body"]
				icon := req.Args["icon"]
				appName := req.Args["app"]
				if appName == "" {
					appName = "phalune"
				}
				if summary == "" {
					summary = "Notification"
				}
				urgency := notify.UrgencyNormal
				if u := req.Args["urgency"]; u == "critical" {
					urgency = notify.UrgencyCritical
				} else if u == "low" {
					urgency = notify.UrgencyLow
				}
				sh.Notify(notify.Notification{
					AppName: appName,
					Summary: summary,
					Body:    body,
					Icon:    icon,
					Urgency: urgency,
				})
				respCh <- ipc.Response{OK: true, Message: "notification sent"}
			case ipc.ActionReloadStyle:
				if err := shell.LoadStyle(cfg.Theme.Name, cfg.Theme.Values); err != nil {
					respCh <- ipc.Response{OK: false, Error: err.Error()}
				} else {
					respCh <- ipc.Response{OK: true, Message: "style reloaded"}
				}
			case ipc.ActionReloadConfig, ipc.ActionReload:
				if err := doReload(nil); err != nil {
					respCh <- ipc.Response{OK: false, Error: err.Error()}
				} else {
					respCh <- ipc.Response{OK: true, Message: "config and shell reloaded"}
				}
			case ipc.ActionSetLogLevel:
				lvlStr := req.Args["level"]
				if lvlStr != "" {
					lvl, err := logging.ParseLevel(lvlStr)
					if err != nil {
						respCh <- ipc.Response{OK: false, Error: err.Error()}
						return
					}
					logHandler.SetConsoleLevel(lvl)
				}
				if nLvlStr := req.Args["notify_level"]; nLvlStr != "" {
					nLvl, err := logging.ParseLevel(nLvlStr)
					if err != nil {
						respCh <- ipc.Response{OK: false, Error: err.Error()}
						return
					}
					logHandler.SetNotifyLevel(nLvl)
				}
				respCh <- ipc.Response{
					OK:      true,
					Message: fmt.Sprintf("log levels: console=%s notify=%s (notify_enabled=%v)", logHandler.ConsoleLevel(), logHandler.NotifyLevel(), logHandler.IsNotifyEnabled()),
				}
			case ipc.ActionToggleNotifyLogs:
				var newState bool
				stateArg := req.Args["state"]
				switch stateArg {
				case "on", "true", "1", "enable":
					newState = true
				case "off", "false", "0", "disable":
					newState = false
				default:
					newState = !logHandler.IsNotifyEnabled()
				}
				logHandler.SetNotifyEnabled(newState)
				respCh <- ipc.Response{
					OK:      true,
					Message: fmt.Sprintf("notify logs enabled: %v (level=%s)", newState, logHandler.NotifyLevel()),
				}
			case ipc.ActionPing:
				data, _ := json.Marshal(map[string]any{"ping": "pong"})
				respCh <- ipc.Response{OK: true, Message: "pong", Data: data}
			case ipc.ActionWidgetPush:
				id := req.Args["id"]
				if !ipc.ValidWidgetID(id) {
					respCh <- ipc.Response{OK: false, Error: "invalid or missing widget id"}
					return
				}
				var st ipc.WidgetState
				if raw := req.Args["json"]; raw != "" {
					if err := json.Unmarshal([]byte(raw), &st); err != nil {
						respCh <- ipc.Response{OK: false, Error: fmt.Sprintf("invalid state JSON: %v", err)}
						return
					}
				} else {
					st = ipc.WidgetState{
						ID:      id,
						Text:    req.Args["text"],
						Tooltip: req.Args["tooltip"],
						Class:   req.Args["class"],
						Icon:    req.Args["icon"],
					}
					if raw := req.Args["percentage"]; raw != "" {
						var pct float64
						if _, err := fmt.Sscanf(raw, "%g", &pct); err != nil {
							respCh <- ipc.Response{OK: false, Error: fmt.Sprintf("invalid percentage %q", raw)}
							return
						}
						st.Percent = &pct
					}
				}
				st.ID = id
				widgetHub.Push(st)
				respCh <- ipc.Response{OK: true, Message: "widget pushed"}
			case ipc.ActionWidgetClear:
				id := req.Args["id"]
				if !ipc.ValidWidgetID(id) {
					respCh <- ipc.Response{OK: false, Error: "invalid or missing widget id"}
					return
				}
				widgetHub.Clear(id)
				respCh <- ipc.Response{OK: true, Message: "widget cleared"}
			default:
				respCh <- ipc.Response{OK: false, Error: fmt.Sprintf("unknown command %q", req.Action)}
			}
		})
		return <-respCh
	}

	app.ConnectActivate(func() {
		if err := shell.LoadStyle(cfg.Theme.Name, cfg.Theme.Values); err != nil {
			slog.Warn("style warning", "error", err)
		}

		sh = shell.New(app, cfg, reg, compositorSvc, eventBus, widgetHub)
		logging.SetNotifier(sh)
		sh.SetConfigReloader(func() {
			glib.IdleAdd(func() {
				if err := doReload(nil); err != nil {
					slog.Error("config reload failed", "error", err)
				}
			})
		})

		if err := sh.Start(); err != nil {
			slog.Error("failed to start shell", "error", err)
			app.Quit()
			return
		}

		server, err := ipc.NewServer("", ipcHandler, eventBus, widgetHub)
		if err != nil {
			slog.Warn("ipc server error", "error", err)
		} else {
			ipcServer = server
		}

		watcher, err := config.StartWatcher(*configPath, func(newCfg *config.Config) {
			glib.IdleAdd(func() {
				_ = doReload(newCfg)
			})
		})
		if err != nil {
			slog.Warn("failed to start config inotify watcher", "error", err)
		} else {
			cfgWatcher = watcher
		}
	})

	app.ConnectShutdown(func() {
		if cfgWatcher != nil {
			cfgWatcher.Stop()
			cfgWatcher = nil
		}
		if ipcServer != nil {
			_ = ipcServer.Close()
		}
		if sh != nil {
			sh.Stop()
		}
		if compositorSvc != nil {
			_ = compositorSvc.Close()
		}
	})

	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		glib.IdleAdd(func() {
			app.Quit()
		})

		select {
		case <-sigCh:
			os.Exit(0)
		case <-time.After(1500 * time.Millisecond):
			os.Exit(0)
		}
	}()

	os.Exit(app.Run([]string{os.Args[0]}))
}
