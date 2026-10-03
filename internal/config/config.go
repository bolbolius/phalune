package config

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"phalune/internal/logging"

	toml "github.com/pelletier/go-toml/v2"
)

var KnownWidgets = map[string]bool{
	"clock":         true,
	"workspaces":    true,
	"audio":         true,
	"battery":       true,
	"tray":          true,
	"bluetooth":     true,
	"wifi":          true,
	"network":       true,
	"keyboard":      true,
	"power":         true,
	"clipboard":     true,
	"notifications": true,
	"privacy":       true,
}

const customPrefix = "custom."

const defaultCommandTimeout = 10 * time.Second

// customWidgetName returns the config key of "custom.weather" -> "weather".
func customWidgetName(listName string) string {
	return strings.TrimPrefix(listName, customPrefix)
}

func isCustomWidget(listName string) bool {
	return strings.HasPrefix(listName, customPrefix) && len(listName) > len(customPrefix)
}

// ──────────────────────────── Top-level Config ────────────────────────────

type Config struct {
	Bar            BarConfig            `toml:"bar"`
	Theme          ThemeConfig          `toml:"theme"`
	Notifications  NotificationsConfig  `toml:"notifications"`
	OSD            OSDConfig            `toml:"osd"`
	ControlCenter  ControlCenterConfig  `toml:"control_center"`
	Launcher       LauncherConfig       `toml:"launcher"`
	Clipboard      ClipboardConfig      `toml:"clipboard"`
	WindowSwitcher WindowSwitcherConfig `toml:"window_switcher"`
	Screenshot     ScreenshotConfig     `toml:"screenshot"`
	LockScreen     LockScreenConfig     `toml:"lockscreen"`
	Session        SessionConfig        `toml:"session"`
	PowerMenu      PowerMenuConfig      `toml:"power_menu"`
	Logging        LogConfig            `toml:"logging"`
}

// ──────────────────────────── Theme ────────────────────────────

type ThemeConfig struct {
	// Name selects a built-in theme or a file in ~/.config/phalune/themes/<name>.toml.
	Name string `toml:"name"`
	// Values overrides individual color tokens.
	Values map[string]string `toml:"-"`

	RawValues map[string]any `toml:"values"`
}

// ──────────────────────────── Bar ────────────────────────────

type BarConfig struct {
	Height   int           `toml:"height"`
	Position string        `toml:"position"` // "top" or "bottom"
	Style    string        `toml:"style"`    // "bubble", "solid", "minimal"
	Left     SectionConfig `toml:"left"`
	Center   SectionConfig `toml:"center"`
	Right    SectionConfig `toml:"right"`

	// Per-widget configuration
	Workspaces WorkspacesConfig              `toml:"workspaces"`
	Clock      ClockConfig                   `toml:"clock"`
	Audio      AudioConfig                   `toml:"audio"`
	Battery    BatteryConfig                 `toml:"battery"`
	Tray       TrayConfig                    `toml:"tray"`
	Bluetooth  BluetoothConfig               `toml:"bluetooth"`
	Wifi       WifiConfig                    `toml:"wifi"`
	Keyboard   KeyboardConfig                `toml:"keyboard"`
	Power      PowerWidgetConfig             `toml:"power"`
	Custom     map[string]CustomWidgetConfig `toml:"custom"`
}

type SectionConfig struct {
	Widgets []string `toml:"widgets"`
}

// ──────────────────────────── Widget configs ────────────────────────────

type WorkspacesConfig struct {
	AllOutputs bool `toml:"all_outputs"`
}

type ClockConfig struct {
	Format   string   `toml:"format"`
	Interval Duration `toml:"interval"`
}

type AudioConfig struct {
	Step             int    `toml:"step"`               // Volume change per scroll tick (%)
	MaxVolume        int    `toml:"max_volume"`         // Maximum volume ceiling
	ScrollDebounceMs int    `toml:"scroll_debounce_ms"` // Scroll rate limit (ms)
	Format           string `toml:"format"`             // Volume label format
	MutedLabel       string `toml:"muted_label"`        // Text shown when muted
}

type BatteryConfig struct {
	LowThreshold  int      `toml:"low_threshold"`  // Low battery CSS class threshold (%)
	FullThreshold int      `toml:"full_threshold"` // "Full" icon threshold (%)
	PollInterval  Duration `toml:"poll_interval"`  // Backup poll between kernel events
	Format        string   `toml:"format"`         // Battery label format
}

type TrayConfig struct {
	IconSize int `toml:"icon_size"` // Preferred icon pixel size
}

type BluetoothConfig struct {
	ShowLabel       bool `toml:"show_label"`       // Whether to show the text label ("On"/"Off")
	HideUnavailable bool `toml:"hide_unavailable"` // Hide widget if no Bluetooth adapter is found
}

type WifiConfig struct {
	ShowLabel       bool `toml:"show_label"`       // Whether to show the text label (SSID / "Off")
	HideUnavailable bool `toml:"hide_unavailable"` // Hide widget if no Wi-Fi interface is found
}

type KeyboardConfig struct {
	Format   string `toml:"format"`    // Display format for layout label (e.g. "%s")
	ShowIcon bool   `toml:"show_icon"` // Whether to show keyboard icon
}

type PowerWidgetConfig struct {
	Icon string `toml:"icon"` // Icon for the power bar widget
}

// CustomWidgetConfig defines a user script widget ("custom.<name>" in a bar section).
// Output is plain text or a line of JSON (return_type = "json") with text/tooltip/class keys.
type CustomWidgetConfig struct {
	Exec             string   `toml:"exec"`               // Command run by the shell
	Interval         Duration `toml:"interval"`           // Poll period; unset = run once
	Tail             bool     `toml:"tail"`               // Stream mode: read stdout line-by-line, ignore interval
	ReturnType       string   `toml:"return_type"`        // "text" (default) or "json"
	Format           string   `toml:"format"`             // Template; {text}/{tooltip}/{class} in json, {} in text
	Icon             string   `toml:"icon"`               // Optional icon name shown before the label
	HideEmpty        bool     `toml:"hide_empty"`         // Hide widget when text output is empty
	OnClick          string   `toml:"on_click"`           // Left-click command
	OnClickRight     string   `toml:"on_click_right"`     // Right-click command
	OnClickMiddle    string   `toml:"on_click_middle"`    // Middle-click command
	OnScrollUp       string   `toml:"on_scroll_up"`       // Scroll-up command
	OnScrollDown     string   `toml:"on_scroll_down"`     // Scroll-down command
	ScrollDebounceMs int      `toml:"scroll_debounce_ms"` // Scroll rate limit (ms); <= 0 uses default (100)
	CommandTimeout   Duration `toml:"command_timeout"`    // Per-command timeout (clicks/scrolls), default 10s
}

// ──────────────────────────── Lock Screen ────────────────────────────

type LockScreenConfig struct {
	TimeFormat  string `toml:"time_format"`  // Time format string (e.g. "15:04")
	DateFormat  string `toml:"date_format"`  // Date format string (e.g. "Monday, January 2")
	AuthCommand string `toml:"auth_command"` // Optional command for password authentication
}

// ──────────────────────────── Session ────────────────────────────

type SessionConfig struct {
	LockOnSleep      bool   `toml:"lock_on_sleep"`     // Auto-lock before sleep/suspend
	CommandLock      string `toml:"command_lock"`      // Optional custom lock command
	CommandLogout    string `toml:"command_logout"`    // Optional custom logout command
	CommandSuspend   string `toml:"command_suspend"`   // Optional custom suspend command
	CommandHibernate string `toml:"command_hibernate"` // Optional custom hibernate command
	CommandReboot    string `toml:"command_reboot"`    // Optional custom reboot command
	CommandPowerOff  string `toml:"command_poweroff"`  // Optional custom poweroff command
}

// ──────────────────────────── Power Menu ────────────────────────────

type PowerMenuConfig struct {
	ShowHibernate bool `toml:"show_hibernate"` // Show Hibernate action in the power menu
}

// ──────────────────────────── Notifications ────────────────────────────

type NotificationsConfig struct {
	Style          string   `toml:"style"`           // "bubbles", "compact"
	Anchor         string   `toml:"anchor"`          // "top-right", "top-left", "bottom-right", "bottom-left", "top", "bottom", "center"
	TimeoutLow     Duration `toml:"timeout_low"`     // Auto-dismiss for low urgency
	TimeoutNormal  Duration `toml:"timeout_normal"`  // Auto-dismiss for normal urgency
	CriticalSticky bool     `toml:"critical_sticky"` // Critical notifications never auto-dismiss
	MarginTop      int      `toml:"margin_top"`      // Pixels from top edge
	MarginRight    int      `toml:"margin_right"`    // Pixels from right edge
	MarginBottom   int      `toml:"margin_bottom"`   // Pixels from bottom edge
	MarginLeft     int      `toml:"margin_left"`     // Pixels from left edge
}

// ──────────────────────────── OSD ────────────────────────────

type OSDConfig struct {
	Style               string   `toml:"style"`                 // "pill", "bar", "minimal"
	Anchor              string   `toml:"anchor"`                // "bottom", "top", "center", "bottom-left", "bottom-right", "top-left", "top-right"
	Timeout             Duration `toml:"timeout"`               // Auto-hide duration
	MarginBottom        int      `toml:"margin_bottom"`         // Pixels from bottom edge
	MarginTop           int      `toml:"margin_top"`            // Pixels from top edge
	MarginLeft          int      `toml:"margin_left"`           // Pixels from left edge
	MarginRight         int      `toml:"margin_right"`          // Pixels from right edge
	Animate             bool     `toml:"animate"`               // Enable smooth morph animation on progress changes
	AnimationDurationMs int      `toml:"animation_duration_ms"` // Duration of smooth animation in ms
}

// ──────────────────────────── Control Center ────────────────────────────

type ControlCenterConfig struct {
	Style string `toml:"style"` // "cards", "compact"
}

// ──────────────────────────── Launcher ────────────────────────────

type LauncherConfig struct {
	Style    string         `toml:"style"`     // "centered", "fullscreen", "compact"
	PageSize int            `toml:"page_size"` // Rows jumped on Page Up/Down
	Terminal string         `toml:"terminal"`  // Preferred terminal emulator (empty = auto-detect)
	Frecency FrecencyConfig `toml:"frecency"`
}

type FrecencyConfig struct {
	HalfLifeDays float64 `toml:"half_life_days"` // Decay half-life in days
	MaxBoost     float64 `toml:"max_boost"`      // Max search score bonus
}

// ──────────────────────────── Clipboard ────────────────────────────

type ClipboardConfig struct {
	MaxEntries    int      `toml:"max_entries"`
	Persist       bool     `toml:"persist"`
	MaxImageBytes int      `toml:"max_image_bytes"`
	MaxTextBytes  int      `toml:"max_text_bytes"`
	IgnoredApps   []string `toml:"ignored_apps"`
	Blacklist     []string `toml:"blacklist"`
}

// ──────────────────────────── Window Switcher ────────────────────────────

type WindowSwitcherConfig struct {
	AllWorkspaces bool `toml:"all_workspaces"`
}

// ──────────────────────────── Screenshot ────────────────────────────

type ScreenshotConfig struct {
	SaveDir      string   `toml:"save_dir"`      // Destination for saved screenshots (empty = ~/Pictures/Screenshots)
	DefaultMode  string   `toml:"default_mode"`  // "area", "window" or "display" fallback
	ToastTimeout Duration `toml:"toast_timeout"` // Preview toast auto-dismiss
}

// ──────────────────────────── Logging ────────────────────────────

type LogConfig struct {
	Level        string `toml:"level"`
	ConsoleLevel string `toml:"console_level"`
	Notify       bool   `toml:"notify"`
	NotifyLevel  string `toml:"notify_level"`
}

// ──────────────────────────── Defaults ────────────────────────────

func Default() *Config {
	return &Config{
		Theme: ThemeConfig{
			Name: "phalune",
		},
		Bar: BarConfig{
			Height:   32,
			Position: "top",
			Left:     SectionConfig{Widgets: []string{"workspaces"}},
			Center:   SectionConfig{Widgets: []string{"clock"}},
			Right:    SectionConfig{Widgets: []string{"privacy", "tray", "wifi", "bluetooth", "audio", "battery", "keyboard", "clipboard", "notifications", "power"}},
			Workspaces: WorkspacesConfig{
				AllOutputs: false,
			},
			Clock: ClockConfig{
				Format:   "15:04",
				Interval: Duration{time.Second},
			},
			Audio: AudioConfig{
				Step:             5,
				MaxVolume:        100,
				ScrollDebounceMs: 20,
				Format:           "%d%%",
				MutedLabel:       "Muted",
			},
			Battery: BatteryConfig{
				LowThreshold:  15,
				FullThreshold: 98,
				PollInterval:  Duration{30 * time.Second},
				Format:        "%d%%",
			},
			Tray: TrayConfig{
				IconSize: 16,
			},
			Bluetooth: BluetoothConfig{
				ShowLabel:       false,
				HideUnavailable: false,
			},
			Wifi: WifiConfig{
				ShowLabel:       false,
				HideUnavailable: false,
			},
			Keyboard: KeyboardConfig{
				Format:   "%s",
				ShowIcon: false,
			},
			Power: PowerWidgetConfig{
				Icon: "system-shutdown-symbolic",
			},
		},
		Notifications: NotificationsConfig{
			Anchor:         "top-right",
			TimeoutLow:     Duration{3 * time.Second},
			TimeoutNormal:  Duration{5 * time.Second},
			CriticalSticky: true,
			MarginTop:      16,
			MarginRight:    16,
		},
		OSD: OSDConfig{
			Anchor:              "bottom",
			Timeout:             Duration{2 * time.Second},
			MarginBottom:        64,
			Animate:             true,
			AnimationDurationMs: 120,
		},
		ControlCenter: ControlCenterConfig{},
		Launcher: LauncherConfig{
			PageSize: 6,
			Terminal: "",
			Frecency: FrecencyConfig{
				HalfLifeDays: 7.0,
				MaxBoost:     25.0,
			},
		},
		WindowSwitcher: WindowSwitcherConfig{
			AllWorkspaces: true,
		},
		Screenshot: ScreenshotConfig{
			DefaultMode:  "area",
			ToastTimeout: Duration{6 * time.Second},
		},
		Clipboard: ClipboardConfig{
			MaxEntries:    30,
			Persist:       true,
			MaxImageBytes: 16 << 20, // 16 MiB
			MaxTextBytes:  1 << 20,  // 1 MiB
		},
		LockScreen: LockScreenConfig{
			TimeFormat: "15:04",
			DateFormat: "Monday, January 2",
		},
		Session: SessionConfig{
			LockOnSleep: true,
		},
		PowerMenu: PowerMenuConfig{
			ShowHibernate: true,
		},
		Logging: LogConfig{
			Level:       "info",
			Notify:      false,
			NotifyLevel: "warn",
		},
	}
}

func DefaultConfigPath() (string, error) {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "phalune", "config.toml"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("user home dir: %w", err)
	}
	return filepath.Join(home, ".config", "phalune", "config.toml"), nil
}

func Load(path string) (*Config, error) {
	target := path
	if target == "" {
		p, err := DefaultConfigPath()
		if err != nil {
			return nil, err
		}
		target = p
	}

	data, err := os.ReadFile(target)
	if err != nil {
		if os.IsNotExist(err) {
			cfg := Default()
			cfg.validate()
			return cfg, nil
		}
		return nil, fmt.Errorf("read config %q: %w", target, err)
	}

	cfg := Default()
	if err := toml.Unmarshal(data, cfg); err != nil {
		// Type mismatch or parse error: log and fall back to full defaults.
		slog.Warn("config parse error; falling back to defaults", "path", target, "error", err)
		cfg = Default()
	}

	cfg.validate()
	return cfg, nil
}

// validate clamps out-of-range values back to their defaults and logs warnings.
// It never returns an error — invalid config never crashes the shell.
func (c *Config) validate() {
	d := Default()

	if strings.TrimSpace(c.Theme.Name) == "" {
		c.Theme.Name = d.Theme.Name
	}
	c.Theme.Name = strings.ToLower(strings.TrimSpace(c.Theme.Name))
	c.Theme.normalize()

	if c.Bar.Height <= 0 {
		slog.Warn("config: bar.height must be > 0, using default", "got", c.Bar.Height, "default", d.Bar.Height)
		c.Bar.Height = d.Bar.Height
	}

	if c.Bar.Position != "top" && c.Bar.Position != "bottom" {
		slog.Warn("config: bar.position must be \"top\" or \"bottom\", using default", "got", c.Bar.Position, "default", d.Bar.Position)
		c.Bar.Position = d.Bar.Position
	}

	for name, cw := range c.Bar.Custom {
		if cw.Exec == "" {
			slog.Warn("config: custom widget has no exec, ignoring", "widget", name)
			delete(c.Bar.Custom, name)
			continue
		}
		if !cw.Tail && cw.Interval.Duration < 0 {
			slog.Warn("config: custom widget interval must be positive, running once", "widget", name, "got", cw.Interval.Duration)
			cw.Interval = Duration{}
			c.Bar.Custom[name] = cw
		}
		if rt := strings.ToLower(strings.TrimSpace(cw.ReturnType)); rt != "" && rt != "json" && rt != "text" {
			slog.Warn("config: custom widget return_type must be \"text\" or \"json\", using text", "widget", name, "got", cw.ReturnType)
			cw.ReturnType = ""
			c.Bar.Custom[name] = cw
		}
		if cw.CommandTimeout.Duration < 0 {
			cw.CommandTimeout = Duration{defaultCommandTimeout}
			c.Bar.Custom[name] = cw
		}
	}

	for section, list := range map[string][]string{
		"bar.left":   c.Bar.Left.Widgets,
		"bar.center": c.Bar.Center.Widgets,
		"bar.right":  c.Bar.Right.Widgets,
	} {
		for _, name := range list {
			if isCustomWidget(name) {
				if _, ok := c.Bar.Custom[customWidgetName(name)]; !ok {
					slog.Warn("config: custom widget has no [bar.custom.<name>] section, ignoring", "widget", name, "section", section)
				}
				continue
			}
			if !KnownWidgets[name] {
				slog.Warn("config: unknown widget in section, ignoring", "widget", name, "section", section)
			}
		}
	}

	if c.Bar.Clock.Format == "" {
		c.Bar.Clock.Format = d.Bar.Clock.Format
	}
	if c.Bar.Clock.Interval.Duration <= 0 {
		slog.Warn("config: bar.clock.interval must be > 0, using default", "got", c.Bar.Clock.Interval, "default", d.Bar.Clock.Interval)
		c.Bar.Clock.Interval = d.Bar.Clock.Interval
	}

	if c.Bar.Audio.Step <= 0 {
		slog.Warn("config: bar.audio.step must be > 0, using default", "got", c.Bar.Audio.Step, "default", d.Bar.Audio.Step)
		c.Bar.Audio.Step = d.Bar.Audio.Step
	}
	if c.Bar.Audio.MaxVolume <= 0 {
		slog.Warn("config: bar.audio.max_volume must be > 0, using default", "got", c.Bar.Audio.MaxVolume, "default", d.Bar.Audio.MaxVolume)
		c.Bar.Audio.MaxVolume = d.Bar.Audio.MaxVolume
	}
	if c.Bar.Audio.ScrollDebounceMs < 0 {
		c.Bar.Audio.ScrollDebounceMs = d.Bar.Audio.ScrollDebounceMs
	}
	if c.Bar.Audio.Format == "" {
		c.Bar.Audio.Format = d.Bar.Audio.Format
	}
	if c.Bar.Audio.MutedLabel == "" {
		c.Bar.Audio.MutedLabel = d.Bar.Audio.MutedLabel
	}

	if c.Bar.Battery.LowThreshold < 0 || c.Bar.Battery.LowThreshold > 100 {
		slog.Warn("config: bar.battery.low_threshold must be 0-100, using default", "got", c.Bar.Battery.LowThreshold, "default", d.Bar.Battery.LowThreshold)
		c.Bar.Battery.LowThreshold = d.Bar.Battery.LowThreshold
	}
	if c.Bar.Battery.FullThreshold < 0 || c.Bar.Battery.FullThreshold > 100 {
		slog.Warn("config: bar.battery.full_threshold must be 0-100, using default", "got", c.Bar.Battery.FullThreshold, "default", d.Bar.Battery.FullThreshold)
		c.Bar.Battery.FullThreshold = d.Bar.Battery.FullThreshold
	}
	if c.Bar.Battery.PollInterval.Duration <= 0 {
		c.Bar.Battery.PollInterval = d.Bar.Battery.PollInterval
	}
	if c.Bar.Battery.Format == "" {
		c.Bar.Battery.Format = d.Bar.Battery.Format
	}

	if c.Bar.Tray.IconSize <= 0 {
		c.Bar.Tray.IconSize = d.Bar.Tray.IconSize
	}

	if c.Bar.Keyboard.Format == "" {
		c.Bar.Keyboard.Format = d.Bar.Keyboard.Format
	}

	if c.Bar.Power.Icon == "" {
		c.Bar.Power.Icon = d.Bar.Power.Icon
	}

	// Pluggable UI styles: invalid names fall back to default inside the
	// component; here only whitespace is normalized.
	cfgs := []struct{ ptr *string }{
		{&c.Bar.Style}, {&c.ControlCenter.Style}, {&c.Launcher.Style},
		{&c.OSD.Style}, {&c.Notifications.Style},
	}
	for i := range cfgs {
		*cfgs[i].ptr = strings.ToLower(strings.TrimSpace(*cfgs[i].ptr))
	}

	if c.LockScreen.TimeFormat == "" {
		c.LockScreen.TimeFormat = d.LockScreen.TimeFormat
	}
	if c.LockScreen.DateFormat == "" {
		c.LockScreen.DateFormat = d.LockScreen.DateFormat
	}

	if c.Notifications.Anchor == "" {
		c.Notifications.Anchor = d.Notifications.Anchor
	}
	if c.Notifications.TimeoutLow.Duration < 0 {
		c.Notifications.TimeoutLow = d.Notifications.TimeoutLow
	}
	if c.Notifications.TimeoutNormal.Duration < 0 {
		c.Notifications.TimeoutNormal = d.Notifications.TimeoutNormal
	}
	if c.Notifications.MarginTop < 0 {
		c.Notifications.MarginTop = d.Notifications.MarginTop
	}
	if c.Notifications.MarginRight < 0 {
		c.Notifications.MarginRight = d.Notifications.MarginRight
	}
	if c.Notifications.MarginBottom < 0 {
		c.Notifications.MarginBottom = 0
	}
	if c.Notifications.MarginLeft < 0 {
		c.Notifications.MarginLeft = 0
	}

	if c.OSD.Anchor == "" {
		c.OSD.Anchor = d.OSD.Anchor
	}
	if c.OSD.Timeout.Duration <= 0 {
		c.OSD.Timeout = d.OSD.Timeout
	}
	if c.OSD.MarginBottom < 0 {
		c.OSD.MarginBottom = d.OSD.MarginBottom
	}
	if c.OSD.MarginTop < 0 {
		c.OSD.MarginTop = 0
	}
	if c.OSD.MarginLeft < 0 {
		c.OSD.MarginLeft = 0
	}
	if c.OSD.MarginRight < 0 {
		c.OSD.MarginRight = 0
	}
	if c.OSD.AnimationDurationMs <= 0 {
		c.OSD.AnimationDurationMs = d.OSD.AnimationDurationMs
	}

	if c.Launcher.PageSize <= 0 {
		c.Launcher.PageSize = d.Launcher.PageSize
	}
	if c.Launcher.Frecency.HalfLifeDays <= 0 {
		c.Launcher.Frecency.HalfLifeDays = d.Launcher.Frecency.HalfLifeDays
	}
	if c.Launcher.Frecency.MaxBoost < 0 {
		c.Launcher.Frecency.MaxBoost = d.Launcher.Frecency.MaxBoost
	}

	if c.Clipboard.MaxEntries <= 0 {
		c.Clipboard.MaxEntries = d.Clipboard.MaxEntries
	}
	if c.Clipboard.MaxImageBytes <= 0 {
		c.Clipboard.MaxImageBytes = d.Clipboard.MaxImageBytes
	}
	if c.Clipboard.MaxTextBytes <= 0 {
		c.Clipboard.MaxTextBytes = d.Clipboard.MaxTextBytes
	}

	mode := strings.ToLower(strings.TrimSpace(c.Screenshot.DefaultMode))
	switch mode {
	case "area", "window", "display", "screen", "monitor":
		c.Screenshot.DefaultMode = mode
	case "":
		c.Screenshot.DefaultMode = d.Screenshot.DefaultMode
	default:
		slog.Warn("config: screenshot.default_mode must be \"area\", \"window\" or \"display\", using default", "got", c.Screenshot.DefaultMode, "default", d.Screenshot.DefaultMode)
		c.Screenshot.DefaultMode = d.Screenshot.DefaultMode
	}
	if c.Screenshot.ToastTimeout.Duration <= 0 {
		c.Screenshot.ToastTimeout = d.Screenshot.ToastTimeout
	}

	if c.Logging.Level != "" {
		if _, err := logging.ParseLevel(c.Logging.Level); err != nil {
			slog.Warn("config: invalid logging.level, using default", "got", c.Logging.Level, "default", d.Logging.Level, "error", err)
			c.Logging.Level = d.Logging.Level
		}
	}
	if c.Logging.ConsoleLevel != "" {
		if _, err := logging.ParseLevel(c.Logging.ConsoleLevel); err != nil {
			slog.Warn("config: invalid logging.console_level, using default", "got", c.Logging.ConsoleLevel, "default", d.Logging.ConsoleLevel, "error", err)
			c.Logging.ConsoleLevel = d.Logging.ConsoleLevel
		}
	}
	if c.Logging.NotifyLevel != "" {
		if _, err := logging.ParseLevel(c.Logging.NotifyLevel); err != nil {
			slog.Warn("config: invalid logging.notify_level, using default", "got", c.Logging.NotifyLevel, "default", d.Logging.NotifyLevel, "error", err)
			c.Logging.NotifyLevel = d.Logging.NotifyLevel
		}
	}
}

// ParseAnchor parses an anchor string (e.g. "top-right", "bottom", "center") into edge booleans.
func ParseAnchor(anchor string) (top, bottom, left, right bool) {
	norm := strings.ToLower(strings.TrimSpace(anchor))
	norm = strings.ReplaceAll(norm, "_", "-")
	norm = strings.ReplaceAll(norm, " ", "-")

	if strings.Contains(norm, "top") {
		top = true
	}
	if strings.Contains(norm, "bottom") {
		bottom = true
	}
	if strings.Contains(norm, "left") {
		left = true
	}
	if strings.Contains(norm, "right") {
		right = true
	}
	return
}

// normalize folds RawValues into Values: hex strings pass through,
// 3-channel int arrays become "r g b" strings.
func (t *ThemeConfig) normalize() {
	t.Values = make(map[string]string, len(t.RawValues))
	for key, v := range t.RawValues {
		switch val := v.(type) {
		case string:
			t.Values[key] = val
		case []any:
			if len(val) != 3 {
				slog.Warn("config: theme.values array must have 3 channels, ignoring", "token", key)
				continue
			}
			bad := false
			b := make([]string, 3)
			for i, c := range val {
				n, ok := channel255(c)
				if !ok {
					bad = true
					break
				}
				b[i] = strconv.FormatInt(n, 10)
			}
			if bad {
				slog.Warn("config: theme.values channels must be integers 0-255, ignoring", "token", key)
				continue
			}
			t.Values[key] = strings.Join(b, " ")
		default:
			slog.Warn("config: theme.values token must be a string or [r, g, b], ignoring", "token", key)
		}
	}
	t.RawValues = nil
}

// channel255 converts a decoded channel value to an integer in 0-255.
func channel255(v any) (int64, bool) {
	switch n := v.(type) {
	case int64:
		if n < 0 || n > 255 {
			return 0, false
		}
		return n, true
	case int:
		if n < 0 || n > 255 {
			return 0, false
		}
		return int64(n), true
	case float64:
		i := int64(n)
		if n != float64(i) || i < 0 || i > 255 {
			return 0, false
		}
		return i, true
	}
	return 0, false
}
