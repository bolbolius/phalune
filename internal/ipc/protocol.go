package ipc

import (
	"fmt"
	"os"
	"path/filepath"
)

type Request struct {
	Action string            `json:"action"`
	Args   map[string]string `json:"args,omitempty"`
}

type Response struct {
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

const (
	ActionToggleLauncher           = "toggle-launcher"
	ActionOpenLauncher             = "open-launcher"
	ActionCloseLauncher            = "close-launcher"
	ActionToggleControlCenter      = "toggle-control-center"
	ActionOpenControlCenter        = "open-control-center"
	ActionCloseControlCenter       = "close-control-center"
	ActionOpenWifi                 = "open-wifi"
	ActionOpenBluetooth            = "open-bluetooth"
	ActionToggleNotificationCenter = "toggle-notifications"
	ActionOpenNotificationCenter   = "open-notifications"
	ActionCloseNotificationCenter  = "close-notifications"
	ActionReloadStyle              = "reload-style"
	ActionPing                     = "ping"
	ActionShowOSD                  = "osd"
	ActionTestOSD                  = "test-osd"
	ActionNotify                   = "notify"
	ActionTestNotify               = "test-notify"
	ActionSetLogLevel              = "set-log-level"
	ActionToggleNotifyLogs         = "toggle-notify-logs"
	ActionReloadConfig             = "reload-config"
	ActionReload                   = "reload"
	ActionTogglePowerMenu          = "toggle-power-menu"
	ActionOpenPowerMenu            = "open-power-menu"
	ActionClosePowerMenu           = "close-power-menu"
	ActionWindowSwitcher           = "window-switcher"
	ActionToggleWindowSwitcher     = "toggle-window-switcher"
	ActionOpenWindowSwitcher       = "open-window-switcher"
	ActionCloseWindowSwitcher      = "close-window-switcher"
	ActionNextWindow               = "next-window"
	ActionPrevWindow               = "prev-window"
	ActionLock                     = "lock"
	ActionUnlock                   = "unlock"
	ActionIsLocked                 = "is-locked"
	ActionSuspend                  = "suspend"
	ActionHibernate                = "hibernate"
	ActionReboot                   = "reboot"
	ActionPowerOff                 = "poweroff"
	ActionLogout                   = "logout"
)

func DefaultSocketPath() string {
	if runtimeDir := os.Getenv("XDG_RUNTIME_DIR"); runtimeDir != "" {
		return filepath.Join(runtimeDir, "phalune.sock")
	}
	return filepath.Join(os.TempDir(), fmt.Sprintf("phalune-%d.sock", os.Getuid()))
}
