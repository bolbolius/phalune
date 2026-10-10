package ipc

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Request struct {
	Action string            `json:"action"`
	Args   map[string]string `json:"args,omitempty"`
}

type Response struct {
	OK      bool            `json:"ok"`
	Message string          `json:"message,omitempty"`
	Error   string          `json:"error,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
}

const (
	ActionStatus                   = "status"
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
	ActionScreenshot               = "screenshot"
	ActionReloadConfig             = "reload-config"
	ActionReload                   = "reload"
	ActionTogglePowerMenu          = "toggle-power-menu"
	ActionOpenPowerMenu            = "open-power-menu"
	ActionClosePowerMenu           = "close-power-menu"
	ActionToggleClipboard          = "toggle-clipboard"
	ActionOpenClipboard            = "open-clipboard"
	ActionCloseClipboard           = "close-clipboard"
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
	ActionVolumeUp                 = "volume-up"
	ActionVolumeDown               = "volume-down"
	ActionVolumeMute               = "volume-mute"
	ActionVolumeSet                = "volume-set"
	ActionBrightnessUp             = "brightness-up"
	ActionBrightnessDown           = "brightness-down"
	ActionBrightnessSet            = "brightness-set"
	ActionWidgetPush               = "widget-push"
	ActionWidgetClear              = "widget-clear"
	ActionWidgetWatch              = "widget-watch"
	ActionSubscribeEvents          = "subscribe"
)

// widgetPrefix marks an IPC widget reference in a bar section list
// ("ipc:pomodoro" renders the state pushed to id "pomodoro").
const widgetPrefix = "ipc:"

// WidgetState is a bar widget state pushed by an external program.
// Text supports Pango markup; Class accepts several space-separated CSS
// tokens. Percentage shows a small progress bar when set. Popover
// declares the flyout UI opened on click (nil = default click events
// only).
type WidgetState struct {
	ID      string       `json:"id"`
	Text    string       `json:"text,omitempty"`
	Tooltip string       `json:"tooltip,omitempty"`
	Class   string       `json:"class,omitempty"`
	Icon    string       `json:"icon,omitempty"`
	Percent *float64     `json:"percentage,omitempty"`
	Popover *PopoverSpec `json:"popover,omitempty"`
	Visible bool         `json:"visible,omitempty"`
}

// PopoverSpec declares a flyout menu for an IPC widget.
type PopoverSpec struct {
	Title string       `json:"title,omitempty"`
	Rows  []PopoverRow `json:"rows,omitempty"`
}

// PopoverRowKind enumerates the supported declarative row types.
const (
	PopoverRowLabel     = "label"
	PopoverRowProgress  = "progress"
	PopoverRowButtonRow = "button_row"
	PopoverRowList      = "list"
)

// PopoverRow is one declarative row inside a popover.
type PopoverRow struct {
	Type    string          `json:"type"`
	Label   string          `json:"label,omitempty"`
	Value   float64         `json:"value,omitempty"` // progress 0-1
	Buttons []PopoverButton `json:"buttons,omitempty"`
	Items   []string        `json:"items,omitempty"`
}

// PopoverButton is a clickable popover action; ID travels back to the
// daemon as {"event": "popover_action", "id": ..., "action": id}.
type PopoverButton struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Class string `json:"class,omitempty"`
}

// WidgetEvent is delivered to the owning connection when the user
// interacts with an IPC widget.
type WidgetEvent struct {
	Event  string `json:"event"`
	ID     string `json:"id"`
	Action string `json:"action,omitempty"`
	Button uint   `json:"button,omitempty"`
}

func mustJSON(v any) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return data
}

// rawAction supports legacy bare-string requests (no JSON envelope): the
// line itself is treated as the action name.
func rawAction(line string) Request {
	return Request{Action: line}
}

// ParseWidgetID extracts the id from an "ipc:<id>" section list name.
func ParseWidgetID(listName string) (string, bool) {
	id := strings.TrimPrefix(listName, widgetPrefix)
	if id == listName || id == "" {
		return "", false
	}
	return id, true
}

// ValidWidgetID reports whether id is safe as a config key and socket id.
func ValidWidgetID(id string) bool {
	if id == "" || len(id) > 64 {
		return false
	}
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
		default:
			return false
		}
	}
	return true
}

// ParseEventTopics splits a comma-separated topic filter list.
func ParseEventTopics(raw string) []string {
	var topics []string
	for _, t := range strings.Split(raw, ",") {
		if t = strings.ToLower(strings.TrimSpace(t)); t != "" {
			topics = append(topics, t)
		}
	}
	return topics
}

// SortedTopics orders topic names for stable status output.
func SortedTopics(topics []string) []string {
	out := append([]string(nil), topics...)
	sort.Strings(out)
	return out
}

func DefaultSocketPath() string {
	if runtimeDir := os.Getenv("XDG_RUNTIME_DIR"); runtimeDir != "" {
		return filepath.Join(runtimeDir, "phalune.sock")
	}
	return filepath.Join(os.TempDir(), fmt.Sprintf("phalune-%d.sock", os.Getuid()))
}
