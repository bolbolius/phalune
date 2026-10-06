package wallpaper

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"phalune/internal/config"
)

func TestSupportedExtensions(t *testing.T) {
	for _, ext := range []string{".png", ".jpg", ".jpeg", ".webp", ".svg", ".bmp", ".gif", ".avif"} {
		if !supportedExts[ext] {
			t.Errorf("expected extension %s to be supported", ext)
		}
	}
	if supportedExts[".txt"] || supportedExts[".mp4"] {
		t.Error("unexpected extension recognized as image")
	}
}

func TestListImageFiles(t *testing.T) {
	dir := t.TempDir()

	// Write mock images
	_ = os.WriteFile(filepath.Join(dir, "wall1.png"), []byte("png"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "wall2.jpg"), []byte("jpg"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "note.txt"), []byte("text"), 0o644)

	files := listImageFiles(dir)
	if len(files) != 2 {
		t.Fatalf("expected 2 image files, got %d", len(files))
	}
}

func TestManagerResolvePath(t *testing.T) {
	cfg := config.WallpaperConfig{
		Enabled: true,
		Path:    "/default/wallpaper.png",
		Outputs: map[string]string{
			"DP-1": "/custom/dp1.png",
		},
	}
	mgr := New(nil, cfg)

	if got := mgr.resolvePath("DP-1"); got != "/custom/dp1.png" {
		t.Errorf("expected custom path for DP-1, got %s", got)
	}
	if got := mgr.resolvePath("HDMI-A-1"); got != "/default/wallpaper.png" {
		t.Errorf("expected default path for HDMI-A-1, got %s", got)
	}
}

func TestManagerLifecycleDisabled(t *testing.T) {
	cfg := config.WallpaperConfig{
		Enabled:  false,
		Path:     "/test.png",
		Interval: config.Duration{Duration: 10 * time.Minute},
	}
	mgr := New(nil, cfg)
	mgr.Start()
	if len(mgr.surfaces) != 0 {
		t.Errorf("expected 0 surfaces when disabled, got %d", len(mgr.surfaces))
	}
	mgr.Stop()
}
