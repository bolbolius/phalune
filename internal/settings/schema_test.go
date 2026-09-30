package settings

import (
	"os"
	"testing"

	"phalune/internal/config"
)

// optionalEmpty rows legitimately read as "" (meaning "use default").
var optionalEmpty = map[string]bool{
	"launcher.terminal":   true,
	"screenshot.save_dir": true,
}

func TestAllRowsResolveAgainstConfig(t *testing.T) {
	cfgPath := os.Getenv("PHALUNE_TEST_CONFIG")
	if cfgPath == "" {
		t.Skip("no config fixture")
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range Pages() {
		for _, r := range p.Rows {
			if optionalEmpty[r.Key] {
				continue
			}
			if got := readString(cfg, r.Key); got == "" {
				t.Errorf("row %s returned empty string", r.Key)
			}
		}
	}
}

func TestAllRowsHaveUniqueKeys(t *testing.T) {
	seen := map[string]bool{}
	for _, p := range Pages() {
		for _, r := range p.Rows {
			if seen[r.Key] {
				t.Errorf("duplicate row key %q", r.Key)
			}
			seen[r.Key] = true
		}
		if seen[p.ID] {
			t.Errorf("duplicate page ID %q", p.ID)
		}
		seen[p.ID] = true
	}
}

func TestParseValueBounds(t *testing.T) {
	height := Row{Key: "bar.height", Kind: KindNumber, Min: 16, Max: 96}
	if _, err := ParseValue(height, "8"); err == nil {
		t.Error("expected error for below minimum")
	}
	if _, err := ParseValue(height, "500"); err == nil {
		t.Error("expected error for above maximum")
	}
	if _, err := ParseValue(height, "abc"); err == nil {
		t.Error("expected error for non-number")
	}
	if v, err := ParseValue(height, "48"); err != nil || v != "48" {
		t.Errorf("valid case failed: %q %v", v, err)
	}

	toggle := Row{Key: "x", Kind: KindToggle}
	if v, _ := ParseValue(toggle, "yes"); v != "true" {
		t.Errorf("toggle yes = %q", v)
	}
	if v, _ := ParseValue(toggle, "off"); v != "false" {
		t.Errorf("toggle off = %q", v)
	}

	choice := Row{Key: "x", Kind: KindChoice, Choices: []string{"a", "b"}}
	if _, err := ParseValue(choice, "c"); err == nil {
		t.Error("expected error for invalid choice")
	}

	dur := Row{Key: "osd.timeout", Kind: KindText}
	if _, err := ParseValue(dur, "not-a-duration"); err == nil {
		t.Error("expected duration error")
	}
	if _, err := ParseValue(dur, "4s"); err != nil {
		t.Errorf("valid duration failed: %v", err)
	}
}

var _ = os.Getenv
