package launcher

import (
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

