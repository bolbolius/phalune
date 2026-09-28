package battery

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBatteryIconName(t *testing.T) {
	tests := []struct {
		pct      int
		status   string
		expected string
	}{
		{100, "Full", "battery-level-100-symbolic"},
		{99, "Charging", "battery-level-100-charged-symbolic"},
		{85, "Discharging", "battery-level-80-symbolic"},
		{85, "Charging", "battery-level-80-charging-symbolic"},
		{15, "Discharging", "battery-level-10-symbolic"},
		{5, "Discharging", "battery-level-0-symbolic"},
	}

	for _, tt := range tests {
		got := BatteryIconName(tt.pct, tt.status)
		if got != tt.expected {
			t.Errorf("BatteryIconName(%d, %q) = %q, want %q", tt.pct, tt.status, got, tt.expected)
		}
	}
}

func TestReadBatteryInfo(t *testing.T) {
	tmpDir := t.TempDir()
	capFile := filepath.Join(tmpDir, "capacity")
	statusFile := filepath.Join(tmpDir, "status")

	if err := os.WriteFile(capFile, []byte("78\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statusFile, []byte("Discharging\n"), 0644); err != nil {
		t.Fatal(err)
	}

	capVal, status, err := readBatteryInfo(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capVal != 78 {
		t.Errorf("expected cap 78, got %d", capVal)
	}
	if status != "Discharging" {
		t.Errorf("expected status 'Discharging', got %q", status)
	}
}
