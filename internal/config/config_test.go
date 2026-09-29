package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := Default()
	cfg.validate()

	if cfg.Bar.Height != 32 {
		t.Errorf("expected default height 32, got %d", cfg.Bar.Height)
	}

	if len(cfg.Bar.Left.Widgets) != 1 || cfg.Bar.Left.Widgets[0] != "workspaces" {
		t.Errorf("unexpected left widgets: %v", cfg.Bar.Left.Widgets)
	}

	if len(cfg.Bar.Center.Widgets) != 1 || cfg.Bar.Center.Widgets[0] != "clock" {
		t.Errorf("unexpected center widgets: %v", cfg.Bar.Center.Widgets)
	}

	expectedRight := []string{"privacy", "tray", "wifi", "bluetooth", "audio", "battery", "keyboard", "clipboard", "notifications", "power"}
	if len(cfg.Bar.Right.Widgets) != len(expectedRight) {
		t.Errorf("expected %d right widgets, got %d (%v)", len(expectedRight), len(cfg.Bar.Right.Widgets), cfg.Bar.Right.Widgets)
	} else {
		for i, w := range expectedRight {
			if cfg.Bar.Right.Widgets[i] != w {
				t.Errorf("expected right widget %d to be %q, got %q", i, w, cfg.Bar.Right.Widgets[i])
			}
		}
	}

	if cfg.Bar.Position != "top" {
		t.Errorf("expected default position top, got %s", cfg.Bar.Position)
	}

	if cfg.Bar.Audio.Step != 5 {
		t.Errorf("expected default audio step 5, got %d", cfg.Bar.Audio.Step)
	}
	if cfg.Bar.Audio.ScrollDebounceMs != 20 {
		t.Errorf("expected default scroll debounce 20, got %d", cfg.Bar.Audio.ScrollDebounceMs)
	}

	if cfg.Bar.Battery.LowThreshold != 15 {
		t.Errorf("expected default low threshold 15, got %d", cfg.Bar.Battery.LowThreshold)
	}

	if cfg.OSD.Timeout.Duration != 2*time.Second {
		t.Errorf("expected default OSD timeout 2s, got %v", cfg.OSD.Timeout.Duration)
	}
	if !cfg.OSD.Animate {
		t.Errorf("expected default OSD animate true, got %v", cfg.OSD.Animate)
	}
	if cfg.OSD.AnimationDurationMs != 120 {
		t.Errorf("expected default OSD animation duration 120, got %d", cfg.OSD.AnimationDurationMs)
	}

	if cfg.Notifications.TimeoutNormal.Duration != 5*time.Second {
		t.Errorf("expected default notify timeout 5s, got %v", cfg.Notifications.TimeoutNormal.Duration)
	}

	if cfg.Launcher.PageSize != 6 {
		t.Errorf("expected default page size 6, got %d", cfg.Launcher.PageSize)
	}

	if cfg.Bar.Bluetooth.ShowLabel {
		t.Errorf("expected default bluetooth show_label false")
	}
	if cfg.Bar.Bluetooth.HideUnavailable {
		t.Errorf("expected default bluetooth hide_unavailable false")
	}
	if cfg.Bar.Wifi.ShowLabel {
		t.Errorf("expected default wifi show_label false")
	}
	if cfg.Bar.Keyboard.Format != "%s" {
		t.Errorf("expected default keyboard format '%%s', got %q", cfg.Bar.Keyboard.Format)
	}
	if cfg.Bar.Keyboard.ShowIcon {
		t.Errorf("expected default keyboard show_icon false")
	}
}

func TestLoadNonExistent(t *testing.T) {
	nonExistent := filepath.Join(t.TempDir(), "nonexistent.toml")
	cfg, err := Load(nonExistent)
	if err != nil {
		t.Fatalf("expected load to succeed with defaults for non-existent file, got: %v", err)
	}
	if cfg.Bar.Height != 32 {
		t.Errorf("expected default height 32, got %d", cfg.Bar.Height)
	}
}

func TestLoadCustomValid(t *testing.T) {
	content := `
[bar]
height = 40
position = "bottom"

[bar.left]
widgets = []

[bar.center]
widgets = ["clock"]

[bar.right]
widgets = ["workspaces"]

[bar.clock]
format = "15:04:05"
interval = "1m"

[bar.audio]
step = 2
max_volume = 150

[bar.battery]
low_threshold = 20
poll_interval = "1m"
`
	tmpFile := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(tmpFile)
	if err != nil {
		t.Fatalf("unexpected error loading config: %v", err)
	}

	if cfg.Bar.Height != 40 {
		t.Errorf("expected height 40, got %d", cfg.Bar.Height)
	}
	if cfg.Bar.Position != "bottom" {
		t.Errorf("expected position bottom, got %s", cfg.Bar.Position)
	}
	if len(cfg.Bar.Right.Widgets) != 1 || cfg.Bar.Right.Widgets[0] != "workspaces" {
		t.Errorf("unexpected right widgets: %v", cfg.Bar.Right.Widgets)
	}
	if cfg.Bar.Clock.Format != "15:04:05" {
		t.Errorf("expected format 15:04:05, got %s", cfg.Bar.Clock.Format)
	}
	if cfg.Bar.Clock.Interval.Duration != time.Minute {
		t.Errorf("expected interval 1m, got %v", cfg.Bar.Clock.Interval.Duration)
	}
	if cfg.Bar.Audio.Step != 2 {
		t.Errorf("expected audio step 2, got %d", cfg.Bar.Audio.Step)
	}
	if cfg.Bar.Audio.MaxVolume != 150 {
		t.Errorf("expected max volume 150, got %d", cfg.Bar.Audio.MaxVolume)
	}
	if cfg.Bar.Battery.LowThreshold != 20 {
		t.Errorf("expected low threshold 20, got %d", cfg.Bar.Battery.LowThreshold)
	}
	if cfg.Bar.Battery.PollInterval.Duration != time.Minute {
		t.Errorf("expected poll interval 1m, got %v", cfg.Bar.Battery.PollInterval.Duration)
	}
}

func TestUnknownWidgetWarning(t *testing.T) {
	// Unknown widgets are logged but don't cause errors anymore
	content := `
[bar]
height = 32

[bar.left]
widgets = ["nonexistent_widget"]

[bar.center]
widgets = []

[bar.right]
widgets = []
`
	tmpFile := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(tmpFile)
	if err != nil {
		t.Fatalf("expected load to succeed (unknown widgets log, not error), got: %v", err)
	}
	// The widget is still in the list — it will fail at creation time, not config time
	if len(cfg.Bar.Left.Widgets) != 1 || cfg.Bar.Left.Widgets[0] != "nonexistent_widget" {
		t.Errorf("expected unknown widget to remain in list, got: %v", cfg.Bar.Left.Widgets)
	}
}

func TestInvalidValuesClampToDefaults(t *testing.T) {
	content := `
[bar]
height = -5
position = "diagonal"

[bar.left]
widgets = []

[bar.center]
widgets = []

[bar.right]
widgets = []

[bar.audio]
step = -1
max_volume = 0
scroll_debounce_ms = -5

[bar.battery]
low_threshold = 200
`
	tmpFile := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(tmpFile)
	if err != nil {
		t.Fatalf("expected load to succeed with clamped values, got: %v", err)
	}

	if cfg.Bar.Height != 32 {
		t.Errorf("expected height clamped to 32, got %d", cfg.Bar.Height)
	}
	if cfg.Bar.Position != "top" {
		t.Errorf("expected position clamped to top, got %s", cfg.Bar.Position)
	}
	if cfg.Bar.Audio.Step != 5 {
		t.Errorf("expected audio step clamped to 5, got %d", cfg.Bar.Audio.Step)
	}
	if cfg.Bar.Audio.MaxVolume != 100 {
		t.Errorf("expected max volume clamped to 100, got %d", cfg.Bar.Audio.MaxVolume)
	}
	if cfg.Bar.Audio.ScrollDebounceMs != 20 {
		t.Errorf("expected scroll debounce clamped to 20, got %d", cfg.Bar.Audio.ScrollDebounceMs)
	}
	if cfg.Bar.Battery.LowThreshold != 15 {
		t.Errorf("expected low threshold clamped to 15, got %d", cfg.Bar.Battery.LowThreshold)
	}
}

func TestTypeMismatchFallsBackToDefaults(t *testing.T) {
	// height is a string instead of int — TOML parse error
	content := `
[bar]
height = "not_a_number"
`
	tmpFile := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(tmpFile)
	if err != nil {
		t.Fatalf("expected load to succeed with full defaults, got: %v", err)
	}

	if cfg.Bar.Height != 32 {
		t.Errorf("expected full default height 32 after type mismatch, got %d", cfg.Bar.Height)
	}
}

func TestLoggingConfig(t *testing.T) {
	content := `
[bar]
height = 32

[bar.left]
widgets = []

[bar.center]
widgets = []

[bar.right]
widgets = []

[logging]
level = "debug"
console_level = "info"
notify = true
notify_level = "error"
`
	tmpFile := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(tmpFile)
	if err != nil {
		t.Fatalf("unexpected error loading logging config: %v", err)
	}

	if cfg.Logging.Level != "debug" {
		t.Errorf("expected level debug, got %s", cfg.Logging.Level)
	}
	if cfg.Logging.ConsoleLevel != "info" {
		t.Errorf("expected console_level info, got %s", cfg.Logging.ConsoleLevel)
	}
	if !cfg.Logging.Notify {
		t.Errorf("expected notify true, got %v", cfg.Logging.Notify)
	}
	if cfg.Logging.NotifyLevel != "error" {
		t.Errorf("expected notify_level error, got %s", cfg.Logging.NotifyLevel)
	}
}

func TestInvalidLogLevelClamps(t *testing.T) {
	content := `
[logging]
level = "super_verbose"
`
	tmpFile := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(tmpFile)
	if err != nil {
		t.Fatalf("expected load to succeed, got: %v", err)
	}

	if cfg.Logging.Level != "info" {
		t.Errorf("expected level clamped to info, got %s", cfg.Logging.Level)
	}
}

func TestDurationParsing(t *testing.T) {
	content := `
[osd]
timeout = "3s"
margin_bottom = 100

[notifications]
timeout_low = "2s"
timeout_normal = "10s"
`
	tmpFile := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(tmpFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.OSD.Timeout.Duration != 3*time.Second {
		t.Errorf("expected OSD timeout 3s, got %v", cfg.OSD.Timeout.Duration)
	}
	if cfg.OSD.MarginBottom != 100 {
		t.Errorf("expected margin bottom 100, got %d", cfg.OSD.MarginBottom)
	}
	if cfg.Notifications.TimeoutLow.Duration != 2*time.Second {
		t.Errorf("expected notify timeout low 2s, got %v", cfg.Notifications.TimeoutLow.Duration)
	}
	if cfg.Notifications.TimeoutNormal.Duration != 10*time.Second {
		t.Errorf("expected notify timeout normal 10s, got %v", cfg.Notifications.TimeoutNormal.Duration)
	}
}

func TestLoadExampleConfig(t *testing.T) {
	examplePath := filepath.Join("..", "..", "example", "config.toml")
	cfg, err := Load(examplePath)
	if err != nil {
		t.Fatalf("failed to load example/config.toml: %v", err)
	}
	if cfg.Logging.Level != "info" {
		t.Errorf("expected level info, got %s", cfg.Logging.Level)
	}
	if cfg.Bar.Audio.Step != 5 {
		t.Errorf("expected audio step 5, got %d", cfg.Bar.Audio.Step)
	}
}

func TestPartialConfigMergesWithDefaults(t *testing.T) {
	// Only specify one field — everything else should be defaults
	content := `
[bar.audio]
step = 10
`
	tmpFile := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(tmpFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Bar.Audio.Step != 10 {
		t.Errorf("expected step 10, got %d", cfg.Bar.Audio.Step)
	}
	// Defaults preserved
	if cfg.Bar.Audio.MaxVolume != 100 {
		t.Errorf("expected default max_volume 100, got %d", cfg.Bar.Audio.MaxVolume)
	}
	if cfg.Bar.Height != 32 {
		t.Errorf("expected default height 32, got %d", cfg.Bar.Height)
	}
	if cfg.Bar.Clock.Format != "15:04" {
		t.Errorf("expected default clock format, got %s", cfg.Bar.Clock.Format)
	}
}

func TestLauncherConfig(t *testing.T) {
	content := `
[launcher]
page_size = 10
terminal = "kitty"

[launcher.frecency]
half_life_days = 14.0
max_boost = 50.0
`
	tmpFile := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(tmpFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Launcher.PageSize != 10 {
		t.Errorf("expected page size 10, got %d", cfg.Launcher.PageSize)
	}
	if cfg.Launcher.Terminal != "kitty" {
		t.Errorf("expected terminal kitty, got %s", cfg.Launcher.Terminal)
	}
	if cfg.Launcher.Frecency.HalfLifeDays != 14.0 {
		t.Errorf("expected half life 14, got %f", cfg.Launcher.Frecency.HalfLifeDays)
	}
	if cfg.Launcher.Frecency.MaxBoost != 50.0 {
		t.Errorf("expected max boost 50, got %f", cfg.Launcher.Frecency.MaxBoost)
	}
}

func TestParseAnchor(t *testing.T) {
	cases := []struct {
		input  string
		top    bool
		bottom bool
		left   bool
		right  bool
	}{
		{"top", true, false, false, false},
		{"bottom", false, true, false, false},
		{"top-right", true, false, false, true},
		{"top_left", true, false, true, false},
		{"bottom-right", false, true, false, true},
		{"bottom left", false, true, true, false},
		{"center", false, false, false, false},
		{"middle", false, false, false, false},
		{"LEFT", false, false, true, false},
		{"RIGHT", false, false, false, true},
	}

	for _, tc := range cases {
		top, bottom, left, right := ParseAnchor(tc.input)
		if top != tc.top || bottom != tc.bottom || left != tc.left || right != tc.right {
			t.Errorf("ParseAnchor(%q) = (%v, %v, %v, %v); expected (%v, %v, %v, %v)",
				tc.input, top, bottom, left, right, tc.top, tc.bottom, tc.left, tc.right)
		}
	}
}

func TestAnchorConfig(t *testing.T) {
	content := `
[osd]
anchor = "top"
margin_top = 48
margin_bottom = 20

[notifications]
anchor = "bottom-left"
margin_bottom = 32
margin_left = 24
`
	tmpFile := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(tmpFile)
	if err != nil {
		t.Fatalf("unexpected error loading config: %v", err)
	}

	if cfg.OSD.Anchor != "top" {
		t.Errorf("expected osd anchor top, got %s", cfg.OSD.Anchor)
	}
	if cfg.OSD.MarginTop != 48 {
		t.Errorf("expected osd margin_top 48, got %d", cfg.OSD.MarginTop)
	}
	if cfg.Notifications.Anchor != "bottom-left" {
		t.Errorf("expected notifications anchor bottom-left, got %s", cfg.Notifications.Anchor)
	}
	if cfg.Notifications.MarginBottom != 32 {
		t.Errorf("expected notifications margin_bottom 32, got %d", cfg.Notifications.MarginBottom)
	}
	if cfg.Notifications.MarginLeft != 24 {
		t.Errorf("expected notifications margin_left 24, got %d", cfg.Notifications.MarginLeft)
	}
}

func TestBluetoothAndKeyboardConfig(t *testing.T) {
	content := `
[bar]
height = 32

[bar.left]
widgets = ["keyboard"]

[bar.right]
widgets = ["bluetooth"]

[bar.bluetooth]
show_label = false
hide_unavailable = true

[bar.keyboard]
format = "[%s]"
show_icon = false
`
	tmpFile := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(tmpFile)
	if err != nil {
		t.Fatalf("unexpected error loading config: %v", err)
	}

	if cfg.Bar.Bluetooth.ShowLabel != false {
		t.Errorf("expected bluetooth show_label false, got %v", cfg.Bar.Bluetooth.ShowLabel)
	}
	if cfg.Bar.Bluetooth.HideUnavailable != true {
		t.Errorf("expected bluetooth hide_unavailable true, got %v", cfg.Bar.Bluetooth.HideUnavailable)
	}
	if cfg.Bar.Keyboard.Format != "[%s]" {
		t.Errorf("expected keyboard format '[%%s]', got %q", cfg.Bar.Keyboard.Format)
	}
	if cfg.Bar.Keyboard.ShowIcon != false {
		t.Errorf("expected keyboard show_icon false, got %v", cfg.Bar.Keyboard.ShowIcon)
	}
}

func TestWifiConfig(t *testing.T) {
	content := `
[bar]
height = 32

[bar.right]
widgets = ["wifi"]

[bar.wifi]
show_label = false
hide_unavailable = true
`
	tmpFile := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(tmpFile)
	if err != nil {
		t.Fatalf("unexpected error loading config: %v", err)
	}

	if cfg.Bar.Wifi.ShowLabel != false {
		t.Errorf("expected wifi show_label false, got %v", cfg.Bar.Wifi.ShowLabel)
	}
	if cfg.Bar.Wifi.HideUnavailable != true {
		t.Errorf("expected wifi hide_unavailable true, got %v", cfg.Bar.Wifi.HideUnavailable)
	}
}

func TestSessionAndPowerConfig(t *testing.T) {
	content := `
[bar]
height = 32

[bar.right]
widgets = ["power"]

[bar.power]
icon = "system-shutdown"

[lockscreen]
time_format = "03:04 PM"
date_format = "2006-01-02"
auth_command = "echo pass"

[session]
lock_on_sleep = false
command_lock = "custom-lock"
command_suspend = "custom-suspend"
command_reboot = "custom-reboot"
command_poweroff = "custom-poweroff"
command_logout = "custom-logout"

[power_menu]
show_hibernate = false
`
	tmpFile := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(tmpFile)
	if err != nil {
		t.Fatalf("unexpected error loading config: %v", err)
	}

	if cfg.Bar.Power.Icon != "system-shutdown" {
		t.Errorf("expected power icon 'system-shutdown', got %q", cfg.Bar.Power.Icon)
	}
	if cfg.LockScreen.TimeFormat != "03:04 PM" {
		t.Errorf("expected lockscreen time format '03:04 PM', got %q", cfg.LockScreen.TimeFormat)
	}
	if cfg.LockScreen.DateFormat != "2006-01-02" {
		t.Errorf("expected lockscreen date format '2006-01-02', got %q", cfg.LockScreen.DateFormat)
	}
	if cfg.LockScreen.AuthCommand != "echo pass" {
		t.Errorf("expected auth command 'echo pass', got %q", cfg.LockScreen.AuthCommand)
	}
	if cfg.Session.LockOnSleep != false {
		t.Errorf("expected lock_on_sleep false, got %v", cfg.Session.LockOnSleep)
	}
	if cfg.Session.CommandLock != "custom-lock" {
		t.Errorf("expected command_lock 'custom-lock', got %q", cfg.Session.CommandLock)
	}
	if cfg.PowerMenu.ShowHibernate != false {
		t.Errorf("expected show_hibernate false, got %v", cfg.PowerMenu.ShowHibernate)
	}
}
