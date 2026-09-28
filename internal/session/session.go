package session

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"sync"

	"phalune/internal/config"
	"phalune/internal/niri"

	"github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/godbus/dbus/v5"
)

const (
	login1Dest  = "org.freedesktop.login1"
	login1Path  = "/org/freedesktop/login1"
	login1IFace = "org.freedesktop.login1.Manager"
)

type Manager struct {
	mu          sync.Mutex
	cfg         config.SessionConfig
	niriSvc     *niri.Service
	sysBus      *dbus.Conn
	onLock      func()
	onUnlock    func()
	isLocked    bool
	cancel      context.CancelFunc
	execCommand func(name string, arg ...string) *exec.Cmd
}

func New(cfg config.SessionConfig, niriSvc *niri.Service, onLock, onUnlock func()) *Manager {
	return &Manager{
		cfg:         cfg,
		niriSvc:     niriSvc,
		onLock:      onLock,
		onUnlock:    onUnlock,
		execCommand: exec.Command,
	}
}

func (m *Manager) Start(ctx context.Context) {
	m.mu.Lock()
	mCtx, cancel := context.WithCancel(ctx)
	m.cancel = cancel
	m.mu.Unlock()

	conn, err := dbus.SystemBus()
	if err != nil {
		slog.Debug("session: system D-Bus unavailable, using command fallbacks", "error", err)
		return
	}

	m.mu.Lock()
	m.sysBus = conn
	m.mu.Unlock()

	// Zero-polling subscription: listen for PrepareForSleep on org.freedesktop.login1.Manager
	ruleSleep := fmt.Sprintf("type='signal',interface='%s',member='PrepareForSleep',path='%s'", login1IFace, login1Path)
	_ = conn.BusObject().Call("org.freedesktop.DBus.AddMatch", 0, ruleSleep)

	// Also listen for Lock / Unlock signals on session objects
	ruleLock := "type='signal',interface='org.freedesktop.login1.Session',member='Lock'"
	_ = conn.BusObject().Call("org.freedesktop.DBus.AddMatch", 0, ruleLock)

	ruleUnlock := "type='signal',interface='org.freedesktop.login1.Session',member='Unlock'"
	_ = conn.BusObject().Call("org.freedesktop.DBus.AddMatch", 0, ruleUnlock)

	ch := make(chan *dbus.Signal, 10)
	conn.Signal(ch)

	go func() {
		defer conn.RemoveSignal(ch)
		for {
			select {
			case <-mCtx.Done():
				return
			case sig, ok := <-ch:
				if !ok {
					return
				}
				m.handleSignal(sig)
			}
		}
	}()
}

func (m *Manager) handleSignal(sig *dbus.Signal) {
	if sig == nil {
		return
	}

	switch sig.Name {
	case "org.freedesktop.login1.Manager.PrepareForSleep":
		if len(sig.Body) > 0 {
			if sleepStarting, ok := sig.Body[0].(bool); ok && sleepStarting {
				m.mu.Lock()
				lockOnSleep := m.cfg.LockOnSleep
				m.mu.Unlock()
				if lockOnSleep {
					slog.Info("session: system preparing for sleep, locking screen")
					m.Lock()
				}
			}
		}
	case "org.freedesktop.login1.Session.Lock":
		slog.Info("session: logind requested lock")
		m.Lock()
	case "org.freedesktop.login1.Session.Unlock":
		slog.Info("session: logind requested unlock")
		m.Unlock()
	}
}

func (m *Manager) Lock() {
	m.mu.Lock()
	m.isLocked = true
	cmdLock := m.cfg.CommandLock
	onLock := m.onLock
	bus := m.sysBus
	m.mu.Unlock()

	if cmdLock != "" {
		go func() {
			cmd := m.runShellCommand(cmdLock)
			_ = cmd.Run()
		}()
	}

	if onLock != nil {
		glib.IdleAdd(func() {
			onLock()
		})
	}

	if bus != nil {
		sessionID := os.Getenv("XDG_SESSION_ID")
		if sessionID != "" {
			go func() {
				obj := bus.Object(login1Dest, login1Path)
				_ = obj.Call(login1IFace+".LockSession", 0, sessionID)
			}()
		}
	}
}

func (m *Manager) Unlock() {
	m.mu.Lock()
	m.isLocked = false
	onUnlock := m.onUnlock
	bus := m.sysBus
	m.mu.Unlock()

	if onUnlock != nil {
		glib.IdleAdd(func() {
			onUnlock()
		})
	}

	if bus != nil {
		sessionID := os.Getenv("XDG_SESSION_ID")
		if sessionID != "" {
			go func() {
				obj := bus.Object(login1Dest, login1Path)
				_ = obj.Call(login1IFace+".UnlockSession", 0, sessionID)
			}()
		}
	}
}

func (m *Manager) IsLocked() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.isLocked
}

func (m *Manager) SetLocked(locked bool) {
	m.mu.Lock()
	m.isLocked = locked
	bus := m.sysBus
	m.mu.Unlock()

	if bus != nil {
		sessionID := os.Getenv("XDG_SESSION_ID")
		if sessionID != "" {
			go func() {
				obj := bus.Object(login1Dest, login1Path)
				if locked {
					_ = obj.Call(login1IFace+".LockSession", 0, sessionID)
				} else {
					_ = obj.Call(login1IFace+".UnlockSession", 0, sessionID)
				}
			}()
		}
	}
}

func (m *Manager) Logout() error {
	m.mu.Lock()
	customCmd := m.cfg.CommandLogout
	m.mu.Unlock()

	if customCmd != "" {
		return m.runShellCommand(customCmd).Run()
	}

	// Try Niri quit action first if running under Niri
	if os.Getenv("NIRI_SOCKET") != "" {
		cmd := m.execCommand("niri", "msg", "action", "quit", "--skip-confirmation")
		if err := cmd.Run(); err == nil {
			return nil
		}
	}

	// Try loginctl terminate-session
	if sessID := os.Getenv("XDG_SESSION_ID"); sessID != "" {
		cmd := m.execCommand("loginctl", "terminate-session", sessID)
		if err := cmd.Run(); err == nil {
			return nil
		}
	}

	// Fallback to loginctl terminate-user
	if user := os.Getenv("USER"); user != "" {
		cmd := m.execCommand("loginctl", "terminate-user", user)
		if err := cmd.Run(); err == nil {
			return nil
		}
	}

	return fmt.Errorf("failed to terminate session")
}

func (m *Manager) Suspend() error {
	m.mu.Lock()
	lockOnSleep := m.cfg.LockOnSleep
	customCmd := m.cfg.CommandSuspend
	bus := m.sysBus
	m.mu.Unlock()

	if lockOnSleep {
		m.Lock()
	}

	if customCmd != "" {
		return m.runShellCommand(customCmd).Run()
	}

	if bus != nil {
		obj := bus.Object(login1Dest, login1Path)
		call := obj.Call(login1IFace+".Suspend", 0, true)
		if call.Err == nil {
			return nil
		}
	}

	// Fallbacks
	if err := m.execCommand("systemctl", "suspend").Run(); err == nil {
		return nil
	}
	return m.execCommand("loginctl", "suspend").Run()
}

func (m *Manager) Hibernate() error {
	m.mu.Lock()
	lockOnSleep := m.cfg.LockOnSleep
	customCmd := m.cfg.CommandHibernate
	bus := m.sysBus
	m.mu.Unlock()

	if lockOnSleep {
		m.Lock()
	}

	if customCmd != "" {
		return m.runShellCommand(customCmd).Run()
	}

	if bus != nil {
		obj := bus.Object(login1Dest, login1Path)
		call := obj.Call(login1IFace+".Hibernate", 0, true)
		if call.Err == nil {
			return nil
		}
	}

	// Fallbacks
	if err := m.execCommand("systemctl", "hibernate").Run(); err == nil {
		return nil
	}
	return m.execCommand("loginctl", "hibernate").Run()
}

func (m *Manager) Reboot() error {
	m.mu.Lock()
	customCmd := m.cfg.CommandReboot
	bus := m.sysBus
	m.mu.Unlock()

	if customCmd != "" {
		return m.runShellCommand(customCmd).Run()
	}

	if bus != nil {
		obj := bus.Object(login1Dest, login1Path)
		call := obj.Call(login1IFace+".Reboot", 0, true)
		if call.Err == nil {
			return nil
		}
	}

	// Fallbacks
	if err := m.execCommand("systemctl", "reboot").Run(); err == nil {
		return nil
	}
	return m.execCommand("loginctl", "reboot").Run()
}

func (m *Manager) PowerOff() error {
	m.mu.Lock()
	customCmd := m.cfg.CommandPowerOff
	bus := m.sysBus
	m.mu.Unlock()

	if customCmd != "" {
		return m.runShellCommand(customCmd).Run()
	}

	if bus != nil {
		obj := bus.Object(login1Dest, login1Path)
		call := obj.Call(login1IFace+".PowerOff", 0, true)
		if call.Err == nil {
			return nil
		}
	}

	// Fallbacks
	if err := m.execCommand("systemctl", "poweroff").Run(); err == nil {
		return nil
	}
	return m.execCommand("loginctl", "poweroff").Run()
}

func (m *Manager) UpdateConfig(cfg config.SessionConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cfg = cfg
}

func (m *Manager) Close() {
	m.mu.Lock()
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	if m.sysBus != nil {
		_ = m.sysBus.Close()
		m.sysBus = nil
	}
	m.mu.Unlock()
}

func (m *Manager) runShellCommand(cmdStr string) *exec.Cmd {
	return m.execCommand("sh", "-c", cmdStr)
}
