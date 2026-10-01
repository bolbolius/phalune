package screenshot

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"phalune/internal/compositor"
	"phalune/internal/config"
)

func TestResolveModeExplicit(t *testing.T) {
	s := &Service{cfg: defaultSvcCfg()}
	cases := map[string]Mode{
		"area":    ModeArea,
		"AREA":    ModeArea,
		"window":  ModeWindow,
		"display": ModeDisplay,
		"screen":  ModeDisplay,
		"monitor": ModeDisplay,
	}
	for in, want := range cases {
		if got := s.resolveMode(in); got != want {
			t.Errorf("resolveMode(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestResolveModeDefault(t *testing.T) {
	cases := map[string]Mode{
		"area":    ModeArea,
		"window":  ModeWindow,
		"display": ModeDisplay,
		"":        ModeArea,
		"bogus":   ModeArea,
	}
	for def, want := range cases {
		cfg := defaultSvcCfg()
		cfg.DefaultMode = def
		s := &Service{cfg: cfg}
		if got := s.resolveMode(""); got != want {
			t.Errorf("default %q: resolveMode(\"\") = %q, want %q", def, got, want)
		}
	}
}

func TestWindowTileGeometry(t *testing.T) {
	w4 := &compositor.Window{Pos: []float64{100, 200}, Size: []float64{800, 600}}
	pos, size := w4.Pos, w4.Size
	if pos == nil || size == nil {
		t.Fatal("expected geometry, got nil")
	}
	if pos[0] != 100 || pos[1] != 200 || size[0] != 800 || size[1] != 600 {
		t.Errorf("unexpected geometry: pos=%v size=%v", pos, size)
	}

	w2 := &compositor.Window{Size: []float64{1920, 1080}}
	if w2.Size[0] != 1920 || w2.Size[1] != 1080 {
		t.Errorf("unexpected 2-element geometry: pos=%v size=%v", w2.Pos, w2.Size)
	}

	short := &compositor.Window{Size: []float64{100}}
	if len(short.Size) != 1 {
		t.Errorf("expected short size preserved, got %v", short.Size)
	}
}

func TestWindowTileGeometryFormatting(t *testing.T) {
	w := compositor.Window{Pos: []float64{100, 200}, Size: []float64{800, 600}}
	geom := fmt.Sprintf("%d,%d %dx%d", int(w.Pos[0]), int(w.Pos[1]), int(w.Size[0]), int(w.Size[1]))
	if geom != "100,200 800x600" {
		t.Errorf("unexpected geometry: %q", geom)
	}

	w2 := compositor.Window{Pos: []float64{0, 0}, Size: []float64{1920, 1080}}
	geom2 := fmt.Sprintf("%d,%d %dx%d", int(w2.Pos[0]), int(w2.Pos[1]), int(w2.Size[0]), int(w2.Size[1]))
	if geom2 != "0,0 1920x1080" {
		t.Errorf("unexpected geometry: %q", geom2)
	}
}

func TestFilenameFormat(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 34, 56, 0, time.UTC)
	got := Filename(now)
	if want := "screenshot-20260930-123456.png"; got != want {
		t.Errorf("Filename = %q, want %q", got, want)
	}
}

func TestSaveCreatesDirAndFile(t *testing.T) {
	dir := t.TempDir()
	saveDir := filepath.Join(dir, "nested", "shots")
	png := []byte("fake-png-bytes")

	path, err := Save(saveDir, png, time.Date(2026, 9, 30, 12, 34, 56, 0, time.UTC))
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	if filepath.Dir(path) != saveDir {
		t.Errorf("expected file in %q, got %q", saveDir, filepath.Dir(path))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read saved file: %v", err)
	}
	if string(data) != string(png) {
		t.Errorf("saved content mismatch: %q", data)
	}
}

func TestSaveDoesNotClobber(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 9, 30, 12, 34, 56, 0, time.UTC)

	first, err := Save(dir, []byte("one"), now)
	if err != nil {
		t.Fatalf("first save: %v", err)
	}
	second, err := Save(dir, []byte("two"), now)
	if err != nil {
		t.Fatalf("second save: %v", err)
	}
	if first == second {
		t.Errorf("second save clobbered first at %q", first)
	}

	data, err := os.ReadFile(first)
	if err != nil {
		t.Fatalf("read first: %v", err)
	}
	if string(data) != "one" {
		t.Errorf("first file was overwritten: %q", data)
	}
}

func defaultSvcCfg() config.ScreenshotConfig {
	return config.ScreenshotConfig{
		DefaultMode:  "area",
		ToastTimeout: config.Duration{Duration: 6 * time.Second},
	}
}
