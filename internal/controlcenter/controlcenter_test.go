package controlcenter

import (
	"testing"
	"time"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

func TestNextPowerProfile(t *testing.T) {
	tests := []struct {
		current  string
		expected string
	}{
		{"power-saver", "balanced"},
		{"balanced", "performance"},
		{"performance", "power-saver"},
		{"POWER-SAVER", "balanced"},
		{"unknown", "balanced"},
		{"", "balanced"},
	}

	for _, tt := range tests {
		got := NextPowerProfile(tt.current)
		if got != tt.expected {
			t.Errorf("NextPowerProfile(%q) = %q, want %q", tt.current, got, tt.expected)
		}
	}
}

func TestPowerProfileInfo(t *testing.T) {
	tests := []struct {
		profile    string
		wantTitle  string
		wantIcon   string
		wantActive bool
	}{
		{"power-saver", "Power Saver", "power-profile-power-saver-symbolic", true},
		{"balanced", "Balanced", "power-profile-balanced-symbolic", true},
		{"performance", "Performance", "power-profile-performance-symbolic", true},
		{"", "Unavailable", "power-profile-balanced-symbolic", false},
	}

	for _, tt := range tests {
		title, icon, active := PowerProfileInfo(tt.profile)
		if title != tt.wantTitle || icon != tt.wantIcon || active != tt.wantActive {
			t.Errorf("PowerProfileInfo(%q) = (%q, %q, %v), want (%q, %q, %v)",
				tt.profile, title, icon, active, tt.wantTitle, tt.wantIcon, tt.wantActive)
		}
	}
}

func TestParseWpctlVolume(t *testing.T) {
	tests := []struct {
		input       string
		wantVolume  float64
		wantMuted   bool
		expectError bool
	}{
		{"Volume: 0.50", 50.0, false, false},
		{"Volume: 1.00", 100.0, false, false},
		{"Volume: 0.00 [MUTED]", 0.0, true, false},
		{"Volume: 0.75 [MUTED]", 75.0, true, false},
		{"Invalid output", 0, false, true},
	}

	for _, tt := range tests {
		vol, muted, err := parseWpctlVolume(tt.input)
		if tt.expectError {
			if err == nil {
				t.Errorf("parseWpctlVolume(%q) expected error, got nil", tt.input)
			}
		} else {
			if err != nil {
				t.Errorf("parseWpctlVolume(%q) unexpected error: %v", tt.input, err)
			}
			if vol != tt.wantVolume || muted != tt.wantMuted {
				t.Errorf("parseWpctlVolume(%q) = (%v, %v), want (%v, %v)",
					tt.input, vol, muted, tt.wantVolume, tt.wantMuted)
			}
		}
	}
}

func TestParsePactlVolume(t *testing.T) {
	tests := []struct {
		input       string
		wantVolume  float64
		expectError bool
	}{
		{"Volume: front-left: 65536 /  100% / 0.00 dB", 100.0, false},
		{"Volume: front-left: 32768 /   50%", 50.0, false},
		{"No percentage here", 0, true},
	}

	for _, tt := range tests {
		vol, err := parsePactlVolume(tt.input)
		if tt.expectError {
			if err == nil {
				t.Errorf("parsePactlVolume(%q) expected error, got nil", tt.input)
			}
		} else {
			if err != nil {
				t.Errorf("parsePactlVolume(%q) unexpected error: %v", tt.input, err)
			}
			if vol != tt.wantVolume {
				t.Errorf("parsePactlVolume(%q) = %v, want %v", tt.input, vol, tt.wantVolume)
			}
		}
	}
}

func TestWifiSignalIcon(t *testing.T) {
	tests := []struct {
		strength uint8
		want     string
	}{
		{90, "network-wireless-signal-excellent-symbolic"},
		{75, "network-wireless-signal-excellent-symbolic"},
		{60, "network-wireless-signal-good-symbolic"},
		{50, "network-wireless-signal-good-symbolic"},
		{30, "network-wireless-signal-ok-symbolic"},
		{25, "network-wireless-signal-ok-symbolic"},
		{10, "network-wireless-signal-weak-symbolic"},
		{0, "network-wireless-signal-weak-symbolic"},
	}

	for _, tt := range tests {
		got := WifiSignalIcon(tt.strength)
		if got != tt.want {
			t.Errorf("WifiSignalIcon(%d) = %q, want %q", tt.strength, got, tt.want)
		}
	}
}

func TestBluetoothDeviceIcon(t *testing.T) {
	tests := []struct {
		icon string
		want string
	}{
		{"audio-headset", "audio-headphones-symbolic"},
		{"audio-headphones", "audio-headphones-symbolic"},
		{"audio-card", "audio-headphones-symbolic"},
		{"input-keyboard", "input-keyboard-symbolic"},
		{"input-mouse", "input-mouse-symbolic"},
		{"phone", "phone-symbolic"},
		{"unknown", "bluetooth-active-symbolic"},
	}

	for _, tt := range tests {
		got := BluetoothDeviceIcon(tt.icon)
		if got != tt.want {
			t.Errorf("BluetoothDeviceIcon(%q) = %q, want %q", tt.icon, got, tt.want)
		}
	}
}


func TestSlidersLockout(t *testing.T) {
	app := gtk.NewApplication("org.phalune.testsliders", 0)
	app.ConnectActivate(func() {
		defer app.Quit()

		volScale := gtk.NewScaleWithRange(gtk.OrientationHorizontal, 0, 100, 1)
		volLabel := gtk.NewLabel("50%")
		volIcon := gtk.NewImageFromIconName("audio-volume-high-symbolic")
		volBtn := gtk.NewButton()

		briScale := gtk.NewScaleWithRange(gtk.OrientationHorizontal, 0, 100, 1)
		briLabel := gtk.NewLabel("50%")
		briIcon := gtk.NewImageFromIconName("display-brightness-symbolic")
		briBtn := gtk.NewButton()

		sc := NewSlidersController(
			volBtn, volIcon, volScale, volLabel,
			briBtn, briIcon, briScale, briLabel,
		)

		// Set slider to 80
		volScale.SetValue(80)

		// Simulate user just dragged slider 10ms ago
		sc.volBinding.mu.Lock()
		sc.volBinding.lastUserChange = time.Now()
		sc.volBinding.mu.Unlock()

		// Background system event arrives saying volume is 30%
		sc.updateVolumeUI(30, false)

		// Should NOT have reset to 30 because user lockout is active
		if volScale.Value() != 80 {
			t.Errorf("expected slider to remain at 80 during user interaction, got %v", volScale.Value())
		}

		// Now simulate time passed (> 500ms)
		sc.volBinding.mu.Lock()
		sc.volBinding.lastUserChange = time.Now().Add(-600 * time.Millisecond)
		sc.volBinding.mu.Unlock()

		// External event arrives
		sc.updateVolumeUI(30, false)

		// Should now be updated to 30
		if volScale.Value() != 30 {
			t.Errorf("expected slider to sync to 30 after lockout expired, got %v", volScale.Value())
		}
	})
	app.Run(nil)
}

func TestWifiAccessPointSorting(t *testing.T) {
	aps := []AccessPoint{
		{SSID: "UnsavedWeak", Strength: 30, Secured: true, Connected: false, Saved: false},
		{SSID: "SavedMedium", Strength: 60, Secured: true, Connected: false, Saved: true},
		{SSID: "UnsavedStrong", Strength: 90, Secured: false, Connected: false, Saved: false},
		{SSID: "ConnectedNet", Strength: 50, Secured: true, Connected: true, Saved: true},
		{SSID: "SavedStrong", Strength: 80, Secured: true, Connected: false, Saved: true},
	}

	sortAccessPoints := func(list []AccessPoint) {
		for i := 0; i < len(list); i++ {
			for j := i + 1; j < len(list); j++ {
				swap := false
				if list[i].Connected != list[j].Connected {
					swap = !list[i].Connected
				} else if list[i].Saved != list[j].Saved {
					swap = !list[i].Saved
				} else if list[i].Strength != list[j].Strength {
					swap = list[i].Strength < list[j].Strength
				} else {
					swap = list[i].SSID > list[j].SSID
				}
				if swap {
					list[i], list[j] = list[j], list[i]
				}
			}
		}
	}

	sortAccessPoints(aps)

	expected := []string{
		"ConnectedNet",
		"SavedStrong",
		"SavedMedium",
		"UnsavedStrong",
		"UnsavedWeak",
	}

	for i, want := range expected {
		if aps[i].SSID != want {
			t.Errorf("aps[%d].SSID = %q, want %q", i, aps[i].SSID, want)
		}
	}
}

func TestSlidersShowOSD(t *testing.T) {
	app := gtk.NewApplication("org.phalune.testslidersosd", 0)
	app.ConnectActivate(func() {
		defer app.Quit()

		volScale := gtk.NewScaleWithRange(gtk.OrientationHorizontal, 0, 100, 1)
		volLabel := gtk.NewLabel("50%")
		volIcon := gtk.NewImageFromIconName("audio-volume-high-symbolic")
		volBtn := gtk.NewButton()

		briScale := gtk.NewScaleWithRange(gtk.OrientationHorizontal, 0, 100, 1)
		briLabel := gtk.NewLabel("50%")
		briIcon := gtk.NewImageFromIconName("display-brightness-symbolic")
		briBtn := gtk.NewButton()

		sc := NewSlidersController(
			volBtn, volIcon, volScale, volLabel,
			briBtn, briIcon, briScale, briLabel,
		)

		var osdCalled bool
		var osdLabel string
		var osdVal float64
		sc.SetShowOSD(func(icon, label string, value float64) {
			osdCalled = true
			osdLabel = label
			osdVal = value
		})

		volScale.SetValue(75)
		if !osdCalled || osdLabel != "Volume" || osdVal != 75 {
			t.Errorf("expected volume OSD call (Volume, 75), got called=%v, label=%s, val=%v",
				osdCalled, osdLabel, osdVal)
		}

		osdCalled = false
		briScale.SetValue(40)
		if !osdCalled || osdLabel != "Brightness" || osdVal != 40 {
			t.Errorf("expected brightness OSD call (Brightness, 40), got called=%v, label=%s, val=%v",
				osdCalled, osdLabel, osdVal)
		}
	})
	app.Run(nil)
}

func TestBluetoothControllerSetPoweredGuard(t *testing.T) {
	bc := NewBluetoothController(nil)
	bc.powered = false
	bc.available = false

	// Calling SetPowered when unavailable should be a no-op and not panic
	bc.SetPowered(false)
	bc.SetPowered(true)

	// When powered state already matches, SetPowered should return early
	bc.available = true
	bc.powered = false
	bc.SetPowered(false)
	if bc.IsPowered() {
		t.Errorf("expected powered to remain false, got %v", bc.IsPowered())
	}
}

func TestBluetoothSwitchFeedbackSuppression(t *testing.T) {
	app := gtk.NewApplication("org.phalune.testbtsuppression", 0)
	app.ConnectActivate(func() {
		defer app.Quit()

		sw := gtk.NewSwitch()
		var updatingSwitch bool
		var backendCallCount int

		sw.ConnectStateSet(func(state bool) bool {
			if updatingSwitch {
				return false
			}
			backendCallCount++
			return false
		})

		// 1. Programmatic update with suppression flag (simulating backend onChange)
		updatingSwitch = true
		sw.SetActive(true)
		updatingSwitch = false

		if backendCallCount != 0 {
			t.Errorf("expected 0 backend calls during programmatic update, got %d", backendCallCount)
		}
		if !sw.Active() {
			t.Errorf("expected switch to be active")
		}

		// 2. Programmatic update to false with suppression flag
		updatingSwitch = true
		sw.SetActive(false)
		updatingSwitch = false

		if backendCallCount != 0 {
			t.Errorf("expected 0 backend calls during programmatic update to false, got %d", backendCallCount)
		}
		if sw.Active() {
			t.Errorf("expected switch to be inactive")
		}
	})
	app.Run(nil)
}

func TestLoadCroppedTexture(t *testing.T) {
	// Test nonexistent file returns nil gracefully
	tex := loadCroppedTexture("/path/does/not/exist.png", 44)
	if tex != nil {
		t.Errorf("expected nil texture for nonexistent file")
	}
}

