package launcher

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCleanExec(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"firefox %u", "firefox"},
		{"gedit --new-window %F", "gedit --new-window"},
		{"code %f", "code"},
		{"foot", "foot"},
		{"vlc %U", "vlc"},
	}

	for _, c := range cases {
		got := CleanExec(c.input)
		if got != c.expected {
			t.Errorf("CleanExec(%q) = %q, expected %q", c.input, got, c.expected)
		}
	}
}

func TestFuzzyScore(t *testing.T) {
	score := FuzzyScore("ff", "firefox")
	if score <= 0 {
		t.Errorf("expected 'ff' to fuzzy match 'firefox', got %d", score)
	}

	scoreNone := FuzzyScore("xyz", "firefox")
	if scoreNone != 0 {
		t.Errorf("expected 'xyz' not to match 'firefox', got %d", scoreNone)
	}

	exact := FuzzyScore("term", "terminal")
	consecutive := FuzzyScore("trm", "terminal")
	if exact <= consecutive {
		t.Errorf("expected exact prefix to score higher than spread-out chars (%d <= %d)", exact, consecutive)
	}
}

func TestFilterApps(t *testing.T) {
	apps := []App{
		{ID: "firefox.desktop", Name: "Firefox Web Browser", GenericName: "Web Browser", Exec: "firefox"},
		{ID: "foot.desktop", Name: "Foot", GenericName: "Terminal Emulator", Exec: "foot"},
		{ID: "alacritty.desktop", Name: "Alacritty", GenericName: "Terminal", Exec: "alacritty"},
		{ID: "gimp.desktop", Name: "GNU Image Manipulation Program", GenericName: "Image Editor", Exec: "gimp"},
	}

	res := FilterApps(apps, "fire fox", nil)
	if len(res) == 0 || res[0].ID != "firefox.desktop" {
		t.Fatalf("expected 'fire fox' to match firefox, got: %+v", res)
	}

	resTerm := FilterApps(apps, "term", nil)
	if len(resTerm) < 2 {
		t.Fatalf("expected 'term' to match at least foot and alacritty via generic name, got: %+v", resTerm)
	}

	resFuzzy := FilterApps(apps, "ff", nil)
	if len(resFuzzy) == 0 || resFuzzy[0].ID != "firefox.desktop" {
		t.Fatalf("expected 'ff' to fuzzy match firefox, got: %+v", resFuzzy)
	}
}

func TestFrecencyStore(t *testing.T) {
	store := &FrecencyStore{
		data: make(map[string]FrecencyEntry),
	}

	if s := store.Score("unknown"); s != 0 {
		t.Errorf("expected score 0 for unknown app, got %v", s)
	}

	store.data["app1"] = FrecencyEntry{
		Count:    10,
		LastUsed: time.Now().UnixMilli(),
	}

	scoreRecent := store.Score("app1")
	if scoreRecent < 9.9 {
		t.Errorf("expected recent score ~10, got %v", scoreRecent)
	}

	store.data["app2"] = FrecencyEntry{
		Count:    10,
		LastUsed: time.Now().Add(-7 * 24 * time.Hour).UnixMilli(),
	}

	scoreWeekOld := store.Score("app2")
	if scoreWeekOld < 4.5 || scoreWeekOld > 5.5 {
		t.Errorf("expected score after 7 days to halve (~5), got %v", scoreWeekOld)
	}
}

func TestParseDesktopActions(t *testing.T) {
	content := `[Desktop Entry]
Type=Application
Name=Firefox
Exec=firefox %u
Icon=firefox
Actions=new-private-window;new-window;

[Desktop Action new-private-window]
Name=Open a Private Window
Icon=firefox-private
Exec=firefox --private-window %u

[Desktop Action new-window]
Name=Open a New Window
Exec=firefox --new-window %u

[Desktop Action orphan]
Name=Orphan Action
Exec=firefox --orphan
`
	path := filepath.Join(t.TempDir(), "firefox.desktop")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	app, ok := parseDesktopFile(path, "firefox.desktop")
	if !ok {
		t.Fatal("expected app to parse")
	}
	if len(app.Actions) != 2 {
		t.Fatalf("expected 2 actions (listed only), got %d: %+v", len(app.Actions), app.Actions)
	}

	priv := app.Actions[0]
	if priv.ID != "new-private-window" {
		t.Errorf("expected first action ID new-private-window, got %q", priv.ID)
	}
	if priv.Name != "Open a Private Window" {
		t.Errorf("unexpected action name %q", priv.Name)
	}
	if priv.CleanExec != "firefox --private-window" {
		t.Errorf("unexpected action CleanExec %q", priv.CleanExec)
	}
	if priv.Icon != "firefox-private" {
		t.Errorf("unexpected action icon %q", priv.Icon)
	}

	second := app.Actions[1]
	if second.ID != "new-window" || second.Name != "Open a New Window" {
		t.Errorf("unexpected second action %+v", second)
	}
}

func TestFilterResultsCommands(t *testing.T) {
	cmds := commandList(&ShellCommands{
		Reboot:   func() error { return nil },
		PowerOff: func() error { return nil },
		Lock:     func() {},
	})

	res := FilterResults(nil, cmds, ":reboot", nil)
	if len(res) == 0 {
		t.Fatal("expected :reboot to match reboot command")
	}
	if res[0].Command == nil || res[0].Command.Name != "reboot" {
		t.Fatalf("expected reboot command first, got %+v", res[0])
	}

	// Alias match
	resAlias := FilterResults(nil, cmds, ":shutdown", nil)
	if len(resAlias) == 0 || resAlias[0].Command == nil || resAlias[0].Command.Name != "poweroff" {
		t.Fatalf("expected :shutdown alias to match poweroff, got %+v", resAlias)
	}

	// Bare colon shows all commands
	resAll := FilterResults(nil, cmds, ":", nil)
	if len(resAll) != len(cmds) {
		t.Fatalf("expected %d commands for ':', got %d", len(cmds), len(resAll))
	}

	// Non-colon query returns app results, no commands
	apps := []App{{ID: "firefox.desktop", Name: "Firefox", Exec: "firefox"}}
	resApp := FilterResults(apps, cmds, "fir", nil)
	if len(resApp) == 0 || resApp[0].App == nil {
		t.Fatalf("expected app result for 'fir', got %+v", resApp)
	}
}

func TestFilterResultsSubActions(t *testing.T) {
	firefox := App{
		ID:   "firefox.desktop",
		Name: "Firefox",
		Exec: "firefox",
		Actions: []DesktopAction{
			{ID: "new-private-window", Name: "New Private Window", Exec: "firefox --private-window"},
			{ID: "new-window", Name: "New Window", Exec: "firefox --new-window"},
		},
	}
	apps := []App{firefox}

	// Action keyword matches the parent app
	res := FilterResults(apps, nil, "private", nil)
	if len(res) == 0 || res[0].App == nil || res[0].App.ID != "firefox.desktop" {
		t.Fatalf("expected 'private' to match parent app firefox, got %+v", res)
	}

	// Combined query "firefox private" also lands on the parent app
	resCombo := FilterResults(apps, nil, "firefox private", nil)
	if len(resCombo) == 0 || resCombo[0].App == nil || resCombo[0].App.ID != "firefox.desktop" {
		t.Fatalf("expected 'firefox private' to match parent app firefox, got %+v", resCombo)
	}

	// Desktop actions are retained on the App struct
	if len(res[0].App.Actions) != 2 {
		t.Fatalf("expected 2 actions on matched app, got %d", len(res[0].App.Actions))
	}
}

func TestResultAccessors(t *testing.T) {
	cmd := commandEntry{Name: "lock", Description: "Lock screen", Icon: "system-lock-screen-symbolic"}
	r := Result{Command: &cmd}
	if r.Title() != ":lock" {
		t.Errorf("expected :lock title, got %q", r.Title())
	}
	if r.Subtitle() != "Lock screen" {
		t.Errorf("unexpected subtitle %q", r.Subtitle())
	}
	if r.IconName() != "system-lock-screen-symbolic" {
		t.Errorf("unexpected icon %q", r.IconName())
	}

	app := App{ID: "a.desktop", Name: "App", Icon: "app-icon"}
	ra := Result{App: &app}
	if ra.Title() != "App" || ra.Subtitle() != "" || ra.IconName() != "app-icon" {
		t.Errorf("unexpected app result %+v", ra)
	}

	rc := Result{Calculation: "42"}
	if rc.Title() != "42" || rc.Subtitle() != "Calculator result (Press Enter to copy)" || rc.IconName() != "accessories-calculator-symbolic" {
		t.Errorf("unexpected calc result %+v", rc)
	}

	rs := Result{ShellCmd: "htop"}
	if rs.Title() != "> htop" || rs.Subtitle() != "Run command in terminal" || rs.IconName() != "utilities-terminal-symbolic" {
		t.Errorf("unexpected shell result %+v", rs)
	}
}

func TestFilterResultsMathAndShell(t *testing.T) {
	// Math evaluation
	resMath := FilterResults(nil, nil, "128 * 4", nil)
	if len(resMath) != 1 || resMath[0].Calculation != "512" {
		t.Fatalf("expected math result 512, got %+v", resMath)
	}

	// Shell command evaluation with '>'
	resShell := FilterResults(nil, nil, "> btop", nil)
	if len(resShell) != 1 || resShell[0].ShellCmd != "btop" {
		t.Fatalf("expected shell cmd 'btop', got %+v", resShell)
	}
}
