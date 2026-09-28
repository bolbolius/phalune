package osd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadBacklightPercent(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "osd-test-backlight-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := os.WriteFile(filepath.Join(tmpDir, "brightness"), []byte("50\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "max_brightness"), []byte("100\n"), 0644); err != nil {
		t.Fatal(err)
	}

	pct, err := readBacklightPercent(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pct != 50.0 {
		t.Errorf("expected 50.0%%, got %.1f%%", pct)
	}

	// Test boundary clamps
	_ = os.WriteFile(filepath.Join(tmpDir, "brightness"), []byte("120\n"), 0644)
	pct, err = readBacklightPercent(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pct != 100.0 {
		t.Errorf("expected 100.0%% clamp, got %.1f%%", pct)
	}
}

func TestReadCapsLockState(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "osd-test-caps-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dev1 := filepath.Join(tmpDir, "input0::capslock")
	_ = os.MkdirAll(dev1, 0755)
	_ = os.WriteFile(filepath.Join(dev1, "brightness"), []byte("0\n"), 0644)

	dev2 := filepath.Join(tmpDir, "input1::capslock")
	_ = os.MkdirAll(dev2, 0755)
	_ = os.WriteFile(filepath.Join(dev2, "brightness"), []byte("1\n"), 0644)

	st := readCapsLockState([]string{dev1, dev2})
	if st != 1 {
		t.Errorf("expected 1 (active), got %d", st)
	}

	_ = os.WriteFile(filepath.Join(dev2, "brightness"), []byte("0\n"), 0644)
	st = readCapsLockState([]string{dev1, dev2})
	if st != 0 {
		t.Errorf("expected 0 (inactive), got %d", st)
	}
}
