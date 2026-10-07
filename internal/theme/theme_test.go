package theme

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseBuiltinThemes(t *testing.T) {
	for _, name := range Builtins() {
		t.Run(name, func(t *testing.T) {
			th, err := Get(name)
			if err != nil {
				t.Fatalf("Get(%q): %v", name, err)
			}
			for _, key := range []string{"surface.rgb", "text", "accent"} {
				if _, ok := th.Colors[key]; !ok {
					t.Errorf("missing required token %q", key)
				}
			}
			if !strings.Contains(th.Colors["surface.rgb"], " ") {
				t.Errorf("surface must render as r g b triplet, got %q", th.Colors["surface.rgb"])
			}
			if !strings.HasPrefix(th.Colors["accent"], "#") {
				t.Errorf("accent must render as hex, got %q", th.Colors["accent"])
			}
		})
	}
}

func TestParseMissingRequired(t *testing.T) {
	_, err := Parse("bad", `
		[colors]
		text = "#ffffff"
	`)
	if err == nil || !strings.Contains(err.Error(), `missing required color "surface"`) {
		t.Fatalf("want missing-required error, got %v", err)
	}
}

func TestParseChannelTriplet(t *testing.T) {
	th, err := Parse("trip", `
		[colors]
		surface = [40, 42, 54]
		text = "#f8f8f2"
		accent = "#bd93f9"
	`)
	if err != nil {
		t.Fatal(err)
	}
	if th.Colors["surface.rgb"] != "40 42 54" {
		t.Errorf("surface triplet (under surface-rgb key) = %q, want %q", th.Colors["surface.rgb"], "40 42 54")
	}
}

func TestParseHexTripletCoercion(t *testing.T) {
	th, err := Parse("hex", `
		[colors]
		surface = "#1a1b26"
		text = "#c0caf5"
		accent = "#7aa2f7"
	`)
	if err != nil {
		t.Fatal(err)
	}
	if th.Colors["surface.rgb"] != "26 27 38" {
		t.Errorf("surface = %q, want %q", th.Colors["surface.rgb"], "26 27 38")
	}
}

func TestParseShorthandHex(t *testing.T) {
	th, err := Parse("hex", `
		[colors]
		surface = "#abc"
		text = "#fff"
		accent = "#00f"
	`)
	if err != nil {
		t.Fatal(err)
	}
	if th.Colors["surface.rgb"] != "170 187 204" {
		t.Errorf("shorthand surface = %q, want %q", th.Colors["surface.rgb"], "170 187 204")
	}
	if th.Colors["accent"] != "#00f" {
		t.Errorf("shorthand accent = %q, want #00f", th.Colors["accent"])
	}
}

func TestParseTripletAcceptsShorthandSurface(t *testing.T) {
	th, err := Parse("sh", `
		[colors]
		surface = "#abc"
		text = "#ffffff"
		accent = "#000000"
	`)
	if err != nil {
		t.Fatal(err)
	}
	if th.Colors["surface.rgb"] != "170 187 204" {
		t.Errorf("surface = %q, want %q", th.Colors["surface.rgb"], "170 187 204")
	}
}

func TestParseRejectsNonTripletInteger(t *testing.T) {
	_, err := Parse("bad", `
		[colors]
		surface = "#000000"
		text = "#ffffff"
		accent = 12
	`)
	if err == nil {
		t.Fatal("want error for integer accent")
	}
}

func TestClampOpacity(t *testing.T) {
	th, err := Parse("op", `
		opacity = { bar = 5.0, overlay = -1.0 }
		[colors]
		surface = "#000000"
		text = "#ffffff"
		accent = "#000000"
	`)
	if err != nil {
		t.Fatal(err)
	}
	if th.OpacityBar != 0.75 {
		t.Errorf("OpacityBar = %v, want default 0.75", th.OpacityBar)
	}
	if th.OpacityOverlay != 0.94 {
		t.Errorf("OpacityOverlay = %v, want default 0.94", th.OpacityOverlay)
	}
}

func TestCSSMergesDefaults(t *testing.T) {
	th := &Theme{Name: "t", Colors: map[string]string{"accent": "#ff79c6"}}
	css := th.CSS(map[string]string{"accent": "#7aa2f7", "text": "#c0caf5"}, nil)
	if !strings.Contains(css, "--accent: #ff79c6;") {
		t.Errorf("missing override: %s", css)
	}
	if !strings.Contains(css, "--text: #c0caf5;") {
		t.Errorf("missing fallback: %s", css)
	}
	if strings.Contains(css, "--text: #7aa2f7;") {
		t.Errorf("fallback must not leak: %s", css)
	}
}

func TestGetRejectsTraversal(t *testing.T) {
	for _, name := range []string{"../etc", "sub/dir", "has space", "with.dot"} {
		if _, err := Get(name); err == nil {
			t.Errorf("Get(%q) should fail", name)
		}
	}
}

func TestGetFallbackOnUnknown(t *testing.T) {
	if _, err := Get("no-such-theme-xyz"); err == nil {
		t.Fatal("unknown theme should error so shell logs fallback")
	}
}

func TestTripletChannelRange(t *testing.T) {
	_, err := Parse("rng", `
		[colors]
		surface = [300, 0, 0]
		text = "#ffffff"
		accent = "#000000"
	`)
	if err == nil || !strings.Contains(err.Error(), "out of range") {
		t.Fatalf("want out-of-range error, got %v", err)
	}
}

func TestCSSPrecomposesSurfaces(t *testing.T) {
	th := &Theme{
		Name:           "t",
		OpacityBar:     0.8,
		OpacitySolid:   0.98,
		OpacityOverlay: 0.95,
		Colors:         map[string]string{"surface.rgb": "40 42 54"},
	}
	css := th.CSS(map[string]string{
		"surface":             "rgba(26, 27, 38, 0.75)",
		"surface-raised":      "rgba(26, 27, 38, 0.85)",
		"surface-solid":       "rgba(22, 22, 30, 0.98)",
		"surface-solid-color": "#16161e",
	}, nil)
	if !strings.Contains(css, "--surface: rgba(40, 42, 54, 0.80);") {
		t.Errorf("bar surface not precomposed: %s", css)
	}
	if !strings.Contains(css, "--surface-overlay: rgba(40, 42, 54, 0.95);") {
		t.Errorf("overlay surface not precomposed: %s", css)
	}
	if !strings.Contains(css, "--surface-solid-color: #282a36;") {
		t.Errorf("opaque hex twin missing: %s", css)
	}
	if !strings.Contains(css, "--surface-raised: rgba(26, 27, 38, 0.85);") {
		t.Errorf("undefined surface must keep default: %s", css)
	}
	if strings.Contains(css, "--surface.rgb") {
		t.Errorf("internal .rgb key leaked into CSS: %s", css)
	}
}

func TestDerivedRGBTriplets(t *testing.T) {
	// accent hex set by theme, accent-rgb absent -> derived from theme hex.
	th, err := Parse("derived", `
		[colors]
		surface = "#1a1b26"
		text = "#c0caf5"
		accent = "#8fd977"      # luna green: -rgb must follow, not stay blue
	`)
	if err != nil {
		t.Fatal(err)
	}
	css := th.CSS(map[string]string{
		"accent":     "#7aa2f7",
		"accent-rgb": "122 162 247",
		"text":       "#c0caf5",
	}, nil)
	if !strings.Contains(css, "--accent-rgb: 143 217 119;") {
		t.Errorf("accent-rgb not derived from theme accent: %s", css)
	}
	if !strings.Contains(css, "--accent: #8fd977;") {
		t.Errorf("accent hex lost: %s", css)
	}
}

func TestExplicitRGBWinsOverDerivation(t *testing.T) {
	th, err := Parse("explicit", `
		[colors]
		surface = "#1a1b26"
		text = "#c0caf5"
		accent = "#8fd977"
		accent-rgb = "1 2 3"
	`)
	if err != nil {
		t.Fatal(err)
	}
	css := th.CSS(map[string]string{"accent-rgb": "122 162 247"}, nil)
	if !strings.Contains(css, "--accent-rgb: 1 2 3;") {
		t.Errorf("explicit theme -rgb must win: %s", css)
	}
}

func TestDerivationOnlyForDeclaredPairs(t *testing.T) {
	th, err := Parse("scoped", `
		[colors]
		surface = "#1a1b26"
		text = "#c0caf5"
		accent = "#8fd977"
	`)
	if err != nil {
		t.Fatal(err)
	}
	// text-rgb NOT in defaults -> must not be synthesized
	css := th.CSS(map[string]string{"accent-rgb": "122 162 247"}, nil)
	if strings.Contains(css, "--text-rgb:") {
		t.Errorf("undeclared -rgb pair synthesized: %s", css)
	}
}

func TestOverridesBeatThemeColors(t *testing.T) {
	th, err := Parse("ovr", `
		[colors]
		surface = "#1a1b26"
		text = "#c0caf5"
		accent = "#8fd977"
	`)
	if err != nil {
		t.Fatal(err)
	}
	css := th.CSS(map[string]string{"accent": "#7aa2f7", "accent-rgb": "122 162 247"}, map[string]string{"accent": "#ff79c6"})
	if !strings.Contains(css, "--accent: #ff79c6;") {
		t.Errorf("override lost to theme color: %s", css)
	}
	// derived -rgb follows the override, not the theme
	if !strings.Contains(css, "--accent-rgb: 255 121 198;") {
		t.Errorf("accent-rgb not derived from override: %s", css)
	}
}

func TestSurfaceOverridePrecomposes(t *testing.T) {
	th := &Theme{Name: "t", OpacityBar: 0.8, OpacitySolid: 0.98, Colors: map[string]string{
		"surface.rgb": "26 27 38",
	}}
	// config override surface = [40, 42, 54] arrives as "40 42 54" after
	// config.normalize(); hex form must coerce too.
	css := th.CSS(map[string]string{"surface": "rgba(26, 27, 38, 0.75)"}, map[string]string{"surface": "40 42 54"})
	if !strings.Contains(css, "--surface: rgba(40, 42, 54, 0.80);") {
		t.Errorf("override surface not precomposed: %s", css)
	}
	css2 := th.CSS(map[string]string{"surface": "rgba(26, 27, 38, 0.75)"}, map[string]string{"surface": "#ff00ff"})
	if !strings.Contains(css2, "--surface: rgba(255, 0, 255, 0.80);") {
		t.Errorf("hex surface override not coerced: %s", css2)
	}
	// -rgb pair must exist in defaults for precomposition; surface.rgb key
	// from the theme clobbers... but override arrives later and wins.
}

func TestInvalidSurfaceOverrideDropped(t *testing.T) {
	th := &Theme{Name: "t", OpacityBar: 0.8, Colors: map[string]string{"surface.rgb": "26 27 38"}}
	css := th.CSS(map[string]string{"surface": "rgba(26, 27, 38, 0.75)"}, map[string]string{"surface": "not-a-color"})
	if strings.Contains(css, "not-a-color") {
		t.Errorf("invalid override leaked into CSS: %s", css)
	}
	if !strings.Contains(css, "--surface: rgba(26, 27, 38, 0.80);") {
		t.Errorf("theme surface lost: %s", css)
	}
}

func TestAvailableIncludesUserThemes(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, "phalune", "themes"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"my-theme.toml", "not-theme.txt", "we-ird.toml"} {
		if err := os.WriteFile(filepath.Join(dir, "phalune", "themes", f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got := Available()
	hasMy := false
	for _, n := range got {
		if n == "my-theme" {
			hasMy = true
		}
		if n == "not-theme" {
			t.Errorf("invalid slug %q offered", n)
		}
	}
	if !hasMy {
		t.Errorf("user theme missing from Available: %v", got)
	}
	if !strings.Contains(strings.Join(got, ","), "phalune") {
		t.Errorf("builtins missing: %v", got)
	}
}

func TestGetRejectsSlugViolations(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if _, err := Get("UPPER"); err == nil {
		t.Error("non-lowercase slug should be rejected before file lookup")
	}
}

func TestFromWallpaperAvailable(t *testing.T) {
	avail := Available()
	found := false
	for _, n := range avail {
		if n == FromWallpaper {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected %q in Available(), got %v", FromWallpaper, avail)
	}
}

func TestFromWallpaperDynamicProvider(t *testing.T) {
	prev := DynamicProvider
	defer func() { DynamicProvider = prev }()

	DynamicProvider = func(name string) (*Theme, error) {
		return &Theme{
			Name:   FromWallpaper,
			Colors: map[string]string{"surface.rgb": "10 20 30", "text": "#ffffff", "accent": "#00ff00"},
		}, nil
	}

	th, err := Get(FromWallpaper)
	if err != nil {
		t.Fatalf("Get(from-wallpaper) failed: %v", err)
	}
	if th.Name != FromWallpaper || th.Colors["accent"] != "#00ff00" {
		t.Errorf("unexpected theme returned: %+v", th)
	}
}

func TestFromWallpaperFallbackWhenNoProvider(t *testing.T) {
	prev := DynamicProvider
	defer func() { DynamicProvider = prev }()
	DynamicProvider = nil

	th, err := Get(FromWallpaper)
	if err != nil {
		t.Fatalf("expected fallback to default theme, got error: %v", err)
	}
	if th.Name != "phalune" {
		t.Errorf("expected fallback to phalune, got %q", th.Name)
	}
}
