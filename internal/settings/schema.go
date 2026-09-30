package settings

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"phalune/internal/config"
)

// Kind is the widget type used to render a row in the settings UI.
type Kind string

const (
	KindToggle Kind = "toggle"
	KindNumber Kind = "number"
	KindText   Kind = "text"
	KindChoice Kind = "choice"
)

// Row describes one editable config field.
type Row struct {
	// Key is the dotted TOML path: "bar.height", "clipboard.persist".
	Key string

	// Label is the display name.
	Label string

	// Hint is the short description shown under the label.
	Hint string

	// Kind selects the editor widget.
	Kind Kind

	// Choices limits valid values for KindChoice rows.
	Choices []string

	// Minimum/Maximum bound numeric rows (0 means unbounded).
	Min, Max int

	// Unit suffix for numeric rows (empty for none).
	Unit string
}

// Page groups rows under one sidebar entry.
type Page struct {
	// ID is the stable page identifier.
	ID string

	// Title shows in the sidebar and as the page header.
	Title string

	// Icon is the themed icon name (symbolic).
	Icon string

	// Rows listed in display order. Nested group headers use Group with no body.
	Rows []Row

	// Groups separates rows into titled groups: pair index with Titles.
	// Titles[i] heads Rows split per Cuts boundaries. Empty = single group.
	// Example: Cuts=[3] Titles=["General","Clock"] → Rows[0:3] then Rows[3:].
	Cuts   []int
	Titles []string
}

func (p *Page) groups() [][2]int {
	if len(p.Cuts) == 0 {
		return [][2]int{{0, len(p.Rows)}}
	}
	var out [][2]int
	start := 0
	for _, cut := range p.Cuts {
		out = append(out, [2]int{start, cut})
		start = cut
	}
	if start < len(p.Rows) {
		out = append(out, [2]int{start, len(p.Rows)})
	}
	return out
}

// Pages returns the full settings schema in sidebar order.
func Pages() []Page {
	return []Page{
		barPage(),
		toastsPage(),
		launcherPage(),
		clipboardPage(),
		windowSwitcherPage(),
		sessionPage(),
		loggingPage(),
	}
}

func barPage() Page {
	return Page{
		ID:    "bar",
		Title: "Bar",
		Icon:  "view-dual-symbolic",
		Rows: []Row{
			{Key: "bar.height", Label: "Height", Hint: "Bar thickness in pixels", Kind: KindNumber, Min: 16, Max: 96, Unit: "px"},
			{Key: "bar.position", Label: "Position", Hint: "Edge of the screen", Kind: KindChoice, Choices: []string{"top", "bottom"}},
			{Key: "bar.clock.format", Label: "Clock format", Hint: "Go time layout, e.g. 15:04 or 3:04 PM", Kind: KindText},
			{Key: "bar.audio.step", Label: "Volume step", Hint: "Percent per scroll tick", Kind: KindNumber, Min: 1, Max: 25, Unit: "%"},
			{Key: "bar.audio.max_volume", Label: "Max volume", Hint: "Volume ceiling", Kind: KindNumber, Min: 50, Max: 150, Unit: "%"},
			{Key: "bar.battery.low_threshold", Label: "Low battery", Hint: "Warning threshold", Kind: KindNumber, Min: 0, Max: 100, Unit: "%"},
			{Key: "bar.keyboard.format", Label: "Keyboard layout format", Hint: "printf-style: %s layout name", Kind: KindText},
			{Key: "bar.tray.icon_size", Label: "Tray icon size", Hint: "Pixel size", Kind: KindNumber, Min: 12, Max: 48, Unit: "px"},
		},
		Titles: []string{"Layout", "Clock", "Audio", "Battery", "Keyboard & Tray"},
		Cuts:   []int{2, 3, 5, 6, 8},
	}
}

func toastsPage() Page {
	return Page{
		ID:    "toasts",
		Title: "Notifications & OSD",
		Icon:  "system-lock-screen-symbolic",
		Rows: []Row{
			{Key: "notifications.anchor", Label: "Position", Hint: "Where toasts appear", Kind: KindChoice, Choices: []string{"top-right", "top-left", "bottom-right", "bottom-left", "top", "bottom", "center"}},
			{Key: "notifications.critical_sticky", Label: "Critical toasts persist", Hint: "Critical notifications never auto-dismiss", Kind: KindToggle},
			{Key: "notifications.timeout_low", Label: "Low urgency timeout", Hint: "Auto-dismiss delay", Kind: KindText},
			{Key: "notifications.timeout_normal", Label: "Normal timeout", Hint: "Auto-dismiss delay", Kind: KindText},
			{Key: "osd.anchor", Label: "OSD position", Hint: "Where on-screen displays appear", Kind: KindChoice, Choices: []string{"bottom", "top", "center", "bottom-left", "bottom-right", "top-left", "top-right"}},
			{Key: "osd.timeout", Label: "OSD timeout", Hint: "Auto-hide delay", Kind: KindText},
			{Key: "osd.animate", Label: "OSD animations", Hint: "Smooth value morphing", Kind: KindToggle},
			{Key: "screenshot.default_mode", Label: "Screenshot mode", Hint: "Fallback capture mode", Kind: KindChoice, Choices: []string{"area", "window", "display"}},
			{Key: "screenshot.save_dir", Label: "Screenshot folder", Hint: "Where saved shots go (empty = ~/Pictures/Screenshots)", Kind: KindText},
			{Key: "screenshot.toast_timeout", Label: "Preview timeout", Hint: "Screenshot toast auto-dismiss", Kind: KindText},
		},
		Titles: []string{"Notifications", "OSD", "Screenshots"},
		Cuts:   []int{4, 7},
	}
}

func launcherPage() Page {
	return Page{
		ID:    "launcher",
		Title: "Launcher",
		Icon:  "system-search-symbolic",
		Rows: []Row{
			{Key: "launcher.page_size", Label: "Page size", Hint: "Rows per launcher page", Kind: KindNumber, Min: 3, Max: 20},
			{Key: "launcher.terminal", Label: "Terminal", Hint: "Terminal emulator command (empty = auto-detect)", Kind: KindText},
			{Key: "launcher.frecency.half_life_days", Label: "Frecency decay", Hint: "Usage memory half-life in days", Kind: KindText},
			{Key: "launcher.frecency.max_boost", Label: "Frecency boost", Hint: "Maximum score bonus from recent use", Kind: KindText},
		},
		Titles: []string{"General", "Frecency"},
		Cuts:   []int{2},
	}
}

func clipboardPage() Page {
	return Page{
		ID:    "clipboard",
		Title: "Clipboard",
		Icon:  "edit-paste-symbolic",
		Rows: []Row{
			{Key: "clipboard.max_entries", Label: "History size", Hint: "Maximum entries kept", Kind: KindNumber, Min: 10, Max: 500},
			{Key: "clipboard.persist", Label: "Persist history", Hint: "Survive shell restarts", Kind: KindToggle},
			{Key: "clipboard.max_image_bytes", Label: "Image size limit", Hint: "Bytes; default 16 MiB", Kind: KindNumber, Min: 0, Max: 67108864},
			{Key: "clipboard.max_text_bytes", Label: "Text size limit", Hint: "Bytes; default 1 MiB", Kind: KindNumber, Min: 0, Max: 16777216},
		},
		Titles: []string{"History", "Size Limits"},
		Cuts:   []int{2},
	}
}

func windowSwitcherPage() Page {
	return Page{
		ID:    "window-switcher",
		Title: "Window Switcher",
		Icon:  "view-grid-symbolic",
		Rows: []Row{
			{Key: "window_switcher.all_workspaces", Label: "All workspaces", Hint: "Switch across every workspace", Kind: KindToggle},
		},
		Titles: []string{"General"},
	}
}

func sessionPage() Page {
	return Page{
		ID:    "session",
		Title: "Session & Locking",
		Icon:  "system-lock-screen-symbolic",
		Rows: []Row{
			{Key: "session.lock_on_sleep", Label: "Lock on sleep", Hint: "Auto-lock before suspend", Kind: KindToggle},
			{Key: "lockscreen.time_format", Label: "Lock screen time", Hint: "Go time layout", Kind: KindText},
			{Key: "lockscreen.date_format", Label: "Lock screen date", Hint: "Go time layout", Kind: KindText},
			{Key: "power_menu.show_hibernate", Label: "Hibernate option", Hint: "Show Hibernate in power menu", Kind: KindToggle},
		},
		Titles: []string{"Session", "Lock Screen", "Power Menu"},
		Cuts:   []int{1, 3},
	}
}

func loggingPage() Page {
	return Page{
		ID:    "logging",
		Title: "Logging",
		Icon:  "text-x-generic-symbolic",
		Rows: []Row{
			{Key: "logging.level", Label: "Level", Hint: "debug, info, warn or error", Kind: KindChoice, Choices: []string{"debug", "info", "warn", "error"}},
			{Key: "logging.notify", Label: "Logs as toasts", Hint: "Forward warnings to notifications", Kind: KindToggle},
			{Key: "logging.notify_level", Label: "Toast threshold", Hint: "Minimum level forwarded", Kind: KindChoice, Choices: []string{"debug", "info", "warn", "error"}},
		},
		Titles: []string{"Console", "Desktop Notifications"},
		Cuts:   []int{1},
	}
}

// readString pulls a config field as its string rendering.
func readString(cfg *config.Config, key string) string {
	parts := strings.Split(key, ".")
	switch key {
	case "bar.height":
		return strconv.Itoa(cfg.Bar.Height)
	case "bar.position":
		return cfg.Bar.Position
	case "bar.clock.format":
		return cfg.Bar.Clock.Format
	case "bar.audio.step":
		return strconv.Itoa(cfg.Bar.Audio.Step)
	case "bar.audio.max_volume":
		return strconv.Itoa(cfg.Bar.Audio.MaxVolume)
	case "bar.battery.low_threshold":
		return strconv.Itoa(cfg.Bar.Battery.LowThreshold)
	case "bar.keyboard.format":
		return cfg.Bar.Keyboard.Format
	case "bar.tray.icon_size":
		return strconv.Itoa(cfg.Bar.Tray.IconSize)
	case "clipboard.max_entries":
		return strconv.Itoa(cfg.Clipboard.MaxEntries)
	case "clipboard.persist":
		return boolStr(cfg.Clipboard.Persist)
	case "clipboard.max_image_bytes":
		return strconv.Itoa(cfg.Clipboard.MaxImageBytes)
	case "clipboard.max_text_bytes":
		return strconv.Itoa(cfg.Clipboard.MaxTextBytes)
	case "launcher.page_size":
		return strconv.Itoa(cfg.Launcher.PageSize)
	case "launcher.terminal":
		return cfg.Launcher.Terminal
	case "launcher.frecency.half_life_days":
		return trimFloat(cfg.Launcher.Frecency.HalfLifeDays)
	case "launcher.frecency.max_boost":
		return trimFloat(cfg.Launcher.Frecency.MaxBoost)
	case "logging.level":
		return cfg.Logging.Level
	case "logging.notify":
		return boolStr(cfg.Logging.Notify)
	case "logging.notify_level":
		return cfg.Logging.NotifyLevel
	case "lockscreen.time_format":
		return cfg.LockScreen.TimeFormat
	case "lockscreen.date_format":
		return cfg.LockScreen.DateFormat
	case "notifications.anchor":
		return cfg.Notifications.Anchor
	case "notifications.critical_sticky":
		return boolStr(cfg.Notifications.CriticalSticky)
	case "notifications.timeout_low":
		return cfg.Notifications.TimeoutLow.Duration.String()
	case "notifications.timeout_normal":
		return cfg.Notifications.TimeoutNormal.Duration.String()
	case "osd.anchor":
		return cfg.OSD.Anchor
	case "osd.timeout":
		return cfg.OSD.Timeout.Duration.String()
	case "osd.animate":
		return boolStr(cfg.OSD.Animate)
	case "osd.animation_duration_ms":
		return strconv.Itoa(cfg.OSD.AnimationDurationMs)
	case "power_menu.show_hibernate":
		return boolStr(cfg.PowerMenu.ShowHibernate)
	case "screenshot.default_mode":
		return cfg.Screenshot.DefaultMode
	case "screenshot.save_dir":
		return cfg.Screenshot.SaveDir
	case "screenshot.toast_timeout":
		return cfg.Screenshot.ToastTimeout.Duration.String()
	case "session.lock_on_sleep":
		return boolStr(cfg.Session.LockOnSleep)
	case "window_switcher.all_workspaces":
		return boolStr(cfg.WindowSwitcher.AllWorkspaces)
	}
	_ = parts
	return ""
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func trimFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// ParseValue validates a raw text entry against the row constraints.
func ParseValue(row Row, value string) (parsed string, err error) {
	v := strings.TrimSpace(value)

	switch row.Kind {
	case KindChoice:
		for _, c := range row.Choices {
			if v == c {
				return v, nil
			}
		}
		return "", fmt.Errorf("must be one of: %s", strings.Join(row.Choices, ", "))

	case KindToggle:
		switch strings.ToLower(v) {
		case "true", "yes", "on", "1":
			return "true", nil
		case "false", "no", "off", "0", "":
			return "false", nil
		}
		return "", fmt.Errorf("must be true or false")

	case KindNumber:
		n, errErr := strconv.Atoi(v)
		if errErr != nil {
			return "", fmt.Errorf("must be a number")
		}
		if row.Min != 0 && n < row.Min {
			return "", fmt.Errorf("minimum %s", strconv.Itoa(row.Min))
		}
		if row.Max != 0 && n > row.Max {
			return "", fmt.Errorf("maximum %s", strconv.Itoa(row.Max))
		}
		return strconv.Itoa(n), nil

	default:
		if isDurationField(row.Key) {
			if _, err := time.ParseDuration(v); err != nil {
				return "", fmt.Errorf("invalid duration: use like \"5s\" or \"500ms\"")
			}
		}
		return v, nil
	}
}

func isDurationField(key string) bool {
	switch key {
	case "notifications.timeout_low", "notifications.timeout_normal",
		"osd.timeout", "screenshot.toast_timeout":
		return true
	}
	return false
}
