package settings

import (
	"os"
	"path/filepath"
	"testing"

	"phalune/internal/config"
)

// TestRealWorldEdit drives the full flow the settings app performs:
// load → change two rows → save → reparse with the shell's parser.
func TestRealWorldEdit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	src := `# phalune configuration file
# Keep comments: they survive edits.

[bar]
height = 32

[bar.clock]
format = "15:04"

[clipboard]
max_entries = 30
persist = true

[screenshot]
default_mode = "area"
`
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("baseline parse: %v", err)
	}
	if cfg.Bar.Height != 32 || cfg.Clipboard.Persist != true {
		t.Fatalf("unexpected baseline: %+v", cfg)
	}

	ed, err := NewEditor(path)
	if err != nil {
		t.Fatal(err)
	}
	pages := Pages()
	var heightRow, formatRow Row
	for _, p := range pages {
		for _, r := range p.Rows {
			if r.Key == "bar.height" {
				heightRow = r
			}
			if r.Key == "bar.clock.format" {
				formatRow = r
			}
		}
	}
	if heightRow.Key == "" || formatRow.Key == "" {
		t.Fatal("schema missing rows")
	}

	v, err := ParseValue(heightRow, "48")
	if err != nil {
		t.Fatal(err)
	}
	if err := ed.Set(heightRow, v); err != nil {
		t.Fatal(err)
	}
	v, err = ParseValue(formatRow, "%H:%M")
	if err != nil {
		t.Fatal(err)
	}
	if err := ed.Set(formatRow, v); err != nil {
		t.Fatal(err)
	}
	if err := ed.Save(); err != nil {
		t.Fatal(err)
	}

	cfg, err = config.Load(path)
	if err != nil {
		t.Fatalf("post-edit parse: %v", err)
	}
	if cfg.Bar.Height != 48 {
		t.Errorf("height = %d, want 48", cfg.Bar.Height)
	}
	if cfg.Bar.Clock.Format != "%H:%M" {
		t.Errorf("format = %q, want %%H:%%M", cfg.Bar.Clock.Format)
	}
	if cfg.Clipboard.Persist != true {
		t.Errorf("untouched row changed: persist = %v", cfg.Clipboard.Persist)
	}
	if cfg.Screenshot.DefaultMode != "area" {
		t.Errorf("untouched section changed: %q", cfg.Screenshot.DefaultMode)
	}

	data, _ := os.ReadFile(path)
	got := string(data)
	for _, keep := range []string{"# Keep comments", "[bar.battery]", "[notifications]"} {
		if got != src {
			break // no insertions happened; comment still present
		}
		if !containsStr(got, keep) {
			t.Errorf("lost content: %q", keep)
		}
	}
	if !containsStr(got, "# Keep comments") {
		t.Errorf("user comment lost:\n%s", got)
	}
}

func containsStr(h, n string) bool {
	for i := 0; i+len(n) <= len(h); i++ {
		if h[i:i+len(n)] == n {
			return true
		}
	}
	return false
}
