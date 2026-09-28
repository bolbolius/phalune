package session

import (
	"context"
	"os/exec"
	"sync/atomic"
	"testing"
	"time"

	"phalune/internal/config"

	"github.com/godbus/dbus/v5"
)

func TestSessionLockUnlock(t *testing.T) {
	var lockedCalls int32
	var unlockedCalls int32

	cfg := config.SessionConfig{
		LockOnSleep: true,
	}

	mgr := New(cfg, nil, func() {
		atomic.AddInt32(&lockedCalls, 1)
	}, func() {
		atomic.AddInt32(&unlockedCalls, 1)
	})

	if mgr.IsLocked() {
		t.Errorf("expected initial state to be unlocked")
	}

	mgr.Lock()
	if !mgr.IsLocked() {
		t.Errorf("expected state to be locked")
	}

	mgr.Unlock()
	if mgr.IsLocked() {
		t.Errorf("expected state to be unlocked")
	}
}

func TestSessionPrepareForSleepSignal(t *testing.T) {
	var lockedCalls int32

	cfg := config.SessionConfig{
		LockOnSleep: true,
	}

	mgr := New(cfg, nil, func() {
		atomic.AddInt32(&lockedCalls, 1)
	}, nil)

	sigTrue := &dbus.Signal{
		Name: "org.freedesktop.login1.Manager.PrepareForSleep",
		Path: login1Path,
		Body: []any{true},
	}

	mgr.handleSignal(sigTrue)
	if !mgr.IsLocked() {
		t.Errorf("expected mgr to be locked on PrepareForSleep(true)")
	}

	// With LockOnSleep false, should not lock
	mgr.Unlock()
	mgr.UpdateConfig(config.SessionConfig{LockOnSleep: false})
	mgr.handleSignal(sigTrue)
	if mgr.IsLocked() {
		t.Errorf("expected mgr NOT to lock when LockOnSleep is false")
	}
}

func TestSessionLogindSignals(t *testing.T) {
	var lockedCalls int32
	var unlockedCalls int32

	mgr := New(config.SessionConfig{}, nil, func() {
		atomic.AddInt32(&lockedCalls, 1)
	}, func() {
		atomic.AddInt32(&unlockedCalls, 1)
	})

	sigLock := &dbus.Signal{
		Name: "org.freedesktop.login1.Session.Lock",
	}
	mgr.handleSignal(sigLock)
	if !mgr.IsLocked() {
		t.Errorf("expected locked on Session.Lock signal")
	}

	sigUnlock := &dbus.Signal{
		Name: "org.freedesktop.login1.Session.Unlock",
	}
	mgr.handleSignal(sigUnlock)
	if mgr.IsLocked() {
		t.Errorf("expected unlocked on Session.Unlock signal")
	}
}

func TestSessionCustomCommands(t *testing.T) {
	var executedCmd string
	cfg := config.SessionConfig{
		CommandLock:      "echo lock",
		CommandSuspend:   "echo suspend",
		CommandHibernate: "echo hibernate",
		CommandReboot:    "echo reboot",
		CommandPowerOff:  "echo poweroff",
		CommandLogout:    "echo logout",
	}

	mgr := New(cfg, nil, nil, nil)
	mgr.execCommand = func(name string, args ...string) *exec.Cmd {
		if len(args) >= 2 && args[0] == "-c" {
			executedCmd = args[1]
		}
		// Return a harmless echo command
		return exec.Command("true")
	}

	if err := mgr.Logout(); err != nil {
		t.Errorf("unexpected error on Logout: %v", err)
	}
	if executedCmd != "echo logout" {
		t.Errorf("expected 'echo logout', got %q", executedCmd)
	}

	if err := mgr.Suspend(); err != nil {
		t.Errorf("unexpected error on Suspend: %v", err)
	}
	if executedCmd != "echo suspend" {
		t.Errorf("expected 'echo suspend', got %q", executedCmd)
	}

	if err := mgr.Hibernate(); err != nil {
		t.Errorf("unexpected error on Hibernate: %v", err)
	}
	if executedCmd != "echo hibernate" {
		t.Errorf("expected 'echo hibernate', got %q", executedCmd)
	}

	if err := mgr.Reboot(); err != nil {
		t.Errorf("unexpected error on Reboot: %v", err)
	}
	if executedCmd != "echo reboot" {
		t.Errorf("expected 'echo reboot', got %q", executedCmd)
	}

	if err := mgr.PowerOff(); err != nil {
		t.Errorf("unexpected error on PowerOff: %v", err)
	}
	if executedCmd != "echo poweroff" {
		t.Errorf("expected 'echo poweroff', got %q", executedCmd)
	}
}

func TestSessionStartClose(t *testing.T) {
	mgr := New(config.SessionConfig{}, nil, nil, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	mgr.Start(ctx)
	mgr.Close()
}
