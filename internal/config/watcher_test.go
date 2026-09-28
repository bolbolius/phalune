package config

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestWatcher_DirectModify(t *testing.T) {
	tmpDir := t.TempDir()
	cfgFile := filepath.Join(tmpDir, "config.toml")

	initial := `
[bar]
height = 30
`
	if err := os.WriteFile(cfgFile, []byte(initial), 0644); err != nil {
		t.Fatal(err)
	}

	reloadCh := make(chan *Config, 5)
	w, err := StartWatcher(cfgFile, func(newCfg *Config) {
		reloadCh <- newCfg
	})
	if err != nil {
		t.Fatalf("failed to start watcher: %v", err)
	}
	defer w.Stop()

	w.SetDebounceDelay(20 * time.Millisecond)

	// Modify the file
	updated := `
[bar]
height = 42
`
	if err := os.WriteFile(cfgFile, []byte(updated), 0644); err != nil {
		t.Fatal(err)
	}

	select {
	case cfg := <-reloadCh:
		if cfg.Bar.Height != 42 {
			t.Errorf("expected height 42 after reload, got %d", cfg.Bar.Height)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for config reload on modify")
	}
}

func TestWatcher_AtomicRename(t *testing.T) {
	tmpDir := t.TempDir()
	cfgFile := filepath.Join(tmpDir, "config.toml")

	initial := `
[bar]
height = 28
`
	if err := os.WriteFile(cfgFile, []byte(initial), 0644); err != nil {
		t.Fatal(err)
	}

	reloadCh := make(chan *Config, 5)
	w, err := StartWatcher(cfgFile, func(newCfg *Config) {
		reloadCh <- newCfg
	})
	if err != nil {
		t.Fatalf("failed to start watcher: %v", err)
	}
	defer w.Stop()

	w.SetDebounceDelay(20 * time.Millisecond)

	// Simulate Vim / atomic save: write to temp file then rename over target
	tmpSave := filepath.Join(tmpDir, "config.toml.tmp")
	updated := `
[bar]
height = 50
`
	if err := os.WriteFile(tmpSave, []byte(updated), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmpSave, cfgFile); err != nil {
		t.Fatal(err)
	}

	select {
	case cfg := <-reloadCh:
		if cfg.Bar.Height != 50 {
			t.Errorf("expected height 50 after atomic rename reload, got %d", cfg.Bar.Height)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for config reload on atomic rename")
	}

	// Verify subsequent modifications still trigger after inode change
	subsequent := `
[bar]
height = 55
`
	if err := os.WriteFile(cfgFile, []byte(subsequent), 0644); err != nil {
		t.Fatal(err)
	}

	select {
	case cfg := <-reloadCh:
		if cfg.Bar.Height != 55 {
			t.Errorf("expected height 55 after subsequent write, got %d", cfg.Bar.Height)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for subsequent config reload after inode replacement")
	}
}

func TestWatcher_DebounceCoalesces(t *testing.T) {
	tmpDir := t.TempDir()
	cfgFile := filepath.Join(tmpDir, "config.toml")

	initial := `
[bar]
height = 20
`
	if err := os.WriteFile(cfgFile, []byte(initial), 0644); err != nil {
		t.Fatal(err)
	}

	var reloadCount int32
	w, err := StartWatcher(cfgFile, func(newCfg *Config) {
		atomic.AddInt32(&reloadCount, 1)
	})
	if err != nil {
		t.Fatalf("failed to start watcher: %v", err)
	}
	defer w.Stop()

	w.SetDebounceDelay(50 * time.Millisecond)

	// Rapidly write multiple times within debounce window
	for i := 0; i < 5; i++ {
		_ = os.WriteFile(cfgFile, []byte(initial), 0644)
		time.Sleep(5 * time.Millisecond)
	}

	time.Sleep(150 * time.Millisecond)

	count := atomic.LoadInt32(&reloadCount)
	if count != 1 {
		t.Errorf("expected exactly 1 coalesced reload, got %d", count)
	}
}
