package settings

import (
	"os"
	"path/filepath"
	"testing"

	"phalune/internal/config"
)

func write(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSetPreservesCommentsAndOrder(t *testing.T) {
	src := `# top comment
[bar]
height = 32

# battery comment
[bar.battery]
low_threshold = 15
`
	path := write(t, src)

	ed, err := NewEditor(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := ed.Set(allTestRows["bar.height"], "48"); err != nil {
		t.Fatal(err)
	}
	if err := ed.Save(); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(path)
	got := string(data)
	want := `# top comment
[bar]
height = 48

# battery comment
[bar.battery]
low_threshold = 15
`
	if got != want {
		t.Errorf("line-preserving write failed:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestSetQuotedString(t *testing.T) {
	src := "[bar.clock]\nformat = \"15:04\"\n"
	path := write(t, src)

	ed, _ := NewEditor(path)
	if err := ed.Set(allTestRows["bar.clock.format"], "%H:%M %d"); err != nil {
		t.Fatal(err)
	}
	_ = ed.Save()

	data, _ := os.ReadFile(path)
	got := string(data)
	want := "[bar.clock]\nformat = \"%H:%M %d\"\n"
	if got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestSetInsertsIntoExistingSection(t *testing.T) {
	src := "[clipboard]\nmax_entries = 30\n"
	path := write(t, src)

	ed, _ := NewEditor(path)
	if err := ed.Set(allTestRows["clipboard.persist"], "false"); err != nil {
		t.Fatal(err)
	}
	_ = ed.Save()

	data, _ := os.ReadFile(path)
	got := string(data)
	want := "[clipboard]\nmax_entries = 30\npersist = false\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestSetCreatesMissingSection(t *testing.T) {
	src := "[bar]\nheight = 32\n"
	path := write(t, src)

	ed, _ := NewEditor(path)
	if err := ed.Set(allTestRows["notifications.anchor"], "top-left"); err != nil {
		t.Fatal(err)
	}
	_ = ed.Save()

	data, _ := os.ReadFile(path)
	got := string(data)
	want := "[bar]\nheight = 32\n\n[notifications]\nanchor = \"top-left\"\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestSetNewFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")

	ed, err := NewEditor(path) // does not exist yet
	if err != nil {
		t.Fatal(err)
	}
	if err := ed.Set(allTestRows["bar.height"], "40"); err != nil {
		t.Fatal(err)
	}
	if err := ed.Save(); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(path)
	got := string(data)
	want := "[bar]\nheight = 40\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestIsNumeric(t *testing.T) {
	cases := map[string]bool{
		"32":    true,
		"-5":    true,
		"1.5":   true,
		"15:04": false,
		"top":   false,
		"":      false,
		"1.2.3": false,
	}
	for in, want := range cases {
		if got := isNumeric(in); got != want {
			t.Errorf("isNumeric(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestSplitJoinRoundTrip(t *testing.T) {
	src := "[a]\nx = 1\n\n[b]\ny = 2\n"
	ed, err := NewEditor(write(t, src))
	if err != nil {
		t.Fatal(err)
	}
	if err := ed.Save(); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(ed.Path())
	if string(data) != src {
		t.Errorf("round trip changed file:\ngot:\n%q\nwant:\n%q", string(data), src)
	}
}

func TestSetInsertsWithCommentsAndBlanks(t *testing.T) {
	src := `[clipboard]
# Keep top entries
max_entries = 30

# Image limit
max_image_bytes = 1000

[bar]
height = 32
`
	path := write(t, src)

	ed, _ := NewEditor(path)
	if err := ed.Set(allTestRows["clipboard.persist"], "true"); err != nil {
		t.Fatal(err)
	}
	_ = ed.Save()

	data, _ := os.ReadFile(path)
	got := string(data)
	want := `[clipboard]
# Keep top entries
max_entries = 30

# Image limit
max_image_bytes = 1000
persist = true

[bar]
height = 32
`
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestSetBackslashString(t *testing.T) {
	src := "[bar.keyboard]\nformat = \"layout: %s\"\n"
	path := write(t, src)

	ed, _ := NewEditor(path)
	if err := ed.Set(Row{Key: "bar.keyboard.format"}, `C:\path\"test"`); err != nil {
		t.Fatal(err)
	}
	_ = ed.Save()

	data, _ := os.ReadFile(path)
	got := string(data)
	want := "[bar.keyboard]\nformat = \"C:\\\\path\\\\\\\"test\\\"\"\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestSetWidgetsArray(t *testing.T) {
	src := "[bar.left]\nwidgets = [\"workspaces\"]\n"
	path := write(t, src)

	ed, _ := NewEditor(path)
	row := Row{Key: "bar.left.widgets", Kind: KindWidgets}

	parsed, err := ParseValue(row, "clock, wifi, custom.pomodoro, ipc:demo")
	if err != nil {
		t.Fatal(err)
	}
	if err := ed.Set(row, parsed); err != nil {
		t.Fatal(err)
	}
	if err := ed.Save(); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(path)
	want := "[bar.left]\nwidgets = [\"clock\", \"wifi\", \"custom.pomodoro\", \"ipc:demo\"]\n"
	if string(data) != want {
		t.Errorf("got:\n%s\nwant:\n%s", string(data), want)
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("written config does not parse: %v", err)
	}
	got := cfg.Bar.Left.Widgets
	if len(got) != 4 || got[0] != "clock" || got[3] != "ipc:demo" {
		t.Errorf("unexpected widgets: %q", got)
	}

	if _, err := ParseValue(row, "clock, bogus"); err == nil {
		t.Error("expected error for unknown widget")
	}
	if _, err := ParseValue(row, "custom., ipc:"); err == nil {
		t.Error("expected error for empty extension suffix")
	}
	if parsed, err := ParseValue(row, "  "); err != nil || parsed != "[]" {
		t.Errorf("empty list should clear to [], got %q, %v", parsed, err)
	}
}

var allTestRows = map[string]Row{
	"bar.height":               {Key: "bar.height"},
	"bar.clock.format":         {Key: "bar.clock.format"},
	"clipboard.persist":        {Key: "clipboard.persist"},
	"notifications.anchor":     {Key: "notifications.anchor"},
	"notifications.margin_top": {Key: "notifications.margin_top"},
}
