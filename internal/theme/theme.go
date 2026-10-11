// Package theme loads and renders color palettes for phalune.
package theme

import (
	_ "embed"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
)

//go:embed themes/phalune.toml
var builtinPhalune string

//go:embed themes/tokyo-night.toml
var builtinTokyoNight string

//go:embed themes/dracula.toml
var builtinDracula string

// builtinRaw lists embedded themes by name.
var builtinRaw = map[string]string{
	"phalune":     builtinPhalune,
	"tokyo-night": builtinTokyoNight,
	"dracula":     builtinDracula,
}

// FromWallpaper is the special theme slug that extracts colors dynamically from the active wallpaper.
const FromWallpaper = "from-wallpaper"

// Builtins returns the names of embedded themes, sorted.
func Builtins() []string {
	names := make([]string, 0, len(builtinRaw))
	for name := range builtinRaw {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Available returns all selectable theme names (built-ins, from-wallpaper, and user themes), sorted.
func Available() []string {
	seen := make(map[string]bool)
	for _, b := range Builtins() {
		seen[b] = true
	}
	seen[FromWallpaper] = true
	extra := []string{FromWallpaper}

	dir := ""
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		dir = filepath.Join(xdg, "phalune", "themes")
	} else if home, err := os.UserHomeDir(); err == nil {
		dir = filepath.Join(home, ".config", "phalune", "themes")
	}
	if dir != "" {
		if entries, err := os.ReadDir(dir); err == nil {
			for _, e := range entries {
				name := e.Name()
				if e.IsDir() || !strings.HasSuffix(name, ".toml") {
					continue
				}
				slug := strings.TrimSuffix(name, ".toml")
				if slug == "" || !validSlug(slug) {
					continue
				}
				if !seen[slug] {
					seen[slug] = true
					extra = append(extra, slug)
				}
			}
		}
	}

	all := append(Builtins(), extra...)
	sort.Strings(all)
	return all
}

// validSlug checks whether a theme name is safe to use as a file slug.
func validSlug(slug string) bool {
	return slug == filepath.Base(slug) && !strings.ContainsAny(slug, "\\/. ")
}

// raw is the on-disk TOML shape of a theme file.
type raw struct {
	Name        string         `toml:"name"`
	Description string         `toml:"description"`
	Dark        bool           `toml:"dark"`
	Opacity     opacityRaw     `toml:"opacity"`
	Colors      map[string]any `toml:"colors"`
}

type opacityRaw struct {
	Bar     float64 `toml:"bar"`
	Overlay float64 `toml:"overlay"`
	Solid   float64 `toml:"solid"`
	Scrim   float64 `toml:"scrim"`
	Shadow  float64 `toml:"shadow"`
}

// Theme holds resolved palette colors and opacity settings.
type Theme struct {
	Name        string
	Description string
	Dark        bool

	OpacityBar     float64
	OpacityOverlay float64
	OpacitySolid   float64
	OpacityScrim   float64
	OpacityShadow  float64

	// Colors maps token name -> CSS color string.
	Colors map[string]string
}

// surfaceTokenRenames maps theme file color names to internal storage keys.
var surfaceTokenRenames = map[string]string{
	"surface":         "surface.rgb",
	"surface-raised":  "surface-raised.rgb",
	"surface-overlay": "surface-overlay.rgb",
	"surface-solid":   "surface-solid.rgb",
}

// surfaceNames lists precomposed surface roles in CSS token order.
var surfaceNames = []string{"surface", "surface-raised", "surface-overlay", "surface-solid"}

func isSurfaceToken(name string) bool {
	_, ok := surfaceTokenRenames[name]
	return ok
}

// Parse decodes a theme TOML document.
func Parse(name, data string) (*Theme, error) {
	var r raw
	if err := toml.Unmarshal([]byte(data), &r); err != nil {
		return nil, fmt.Errorf("theme %q: %w", name, err)
	}
	if len(r.Colors) == 0 {
		return nil, fmt.Errorf("theme %q: no [colors] table", name)
	}

	t := &Theme{
		Name:           r.Name,
		Description:    r.Description,
		Dark:           r.Dark,
		OpacityBar:     clamp01(r.Opacity.Bar, 0.75),
		OpacityOverlay: clamp01(r.Opacity.Overlay, 0.94),
		OpacitySolid:   clamp01(r.Opacity.Solid, 0.98),
		OpacityScrim:   clamp01(r.Opacity.Scrim, 0.55),
		OpacityShadow:  clamp01(r.Opacity.Shadow, 0.45),
		Colors:         make(map[string]string, len(r.Colors)),
	}
	if t.Name == "" {
		t.Name = name
	}

	// Required tokens.
	for _, key := range []string{"surface", "text", "accent"} {
		v, ok := r.Colors[key]
		if !ok {
			return nil, fmt.Errorf("theme %q: missing required color %q", name, key)
		}
		if err := encode(t.Colors, rename(key), v, isSurfaceToken(key)); err != nil {
			return nil, fmt.Errorf("theme %q: color %q: %w", name, key, err)
		}
	}
	// Explicit "-rgb" triplets accept hex or "r g b" strings.
	for key, v := range r.Colors {
		if isSurfaceToken(key) || key == "text" || key == "accent" {
			continue
		}
		if !strings.HasSuffix(rename(key), "-rgb") {
			continue
		}
		if s, ok := v.(string); ok && strings.Contains(s, " ") {
			if err := validateTriplet(s); err != nil {
				return nil, fmt.Errorf("theme %q: color %q: %w", name, key, err)
			}
			t.Colors[rename(key)] = s
			delete(r.Colors, key)
		}
	}
	for key, v := range r.Colors {
		if key == "surface" {
			continue
		}
		if err := encode(t.Colors, rename(key), v, isSurfaceToken(key)); err != nil {
			return nil, fmt.Errorf("theme %q: color %q: %w", name, key, err)
		}
	}
	return t, nil
}

// validateTriplet checks a "r g b" channel string.
func validateTriplet(s string) error {
	parts := strings.Fields(s)
	if len(parts) != 3 {
		return fmt.Errorf("expected 3 channels, got %d in %q", len(parts), s)
	}
	for i, p := range parts {
		n, err := strconv.ParseUint(p, 10, 8)
		if err != nil {
			return fmt.Errorf("channel %d out of range 0-255: %q", i, p)
		}
		_ = n
	}
	return nil
}

// rename maps a theme-file color name to its CSS token name.
func rename(key string) string {
	if renamed, ok := surfaceTokenRenames[key]; ok {
		return renamed
	}
	return key
}

func encode(dst map[string]string, key string, value any, triplet bool) error {
	switch v := value.(type) {
	case string:
		if triplet {
			channels, err := hexToTriplet(v)
			if err != nil {
				return err
			}
			dst[key] = channels
			return nil
		}
		if !isHexColor(v) {
			return fmt.Errorf("expected hex color like #7aa2f7, got %q", v)
		}
		dst[key] = strings.ToLower(v)
		return nil
	case int64: // toml integer: "r g b" triplet for surface tokens
		return fmt.Errorf("expected string or array, got integer")
	case []any:
		if !triplet {
			return fmt.Errorf("expected string hex color, got array")
		}
		if len(v) != 3 {
			return fmt.Errorf("expected 3 channels [r, g, b], got %d", len(v))
		}
		var buf [3]string
		for i, c := range v {
			f, ok := c.(int64)
			if !ok {
				return fmt.Errorf("channels must be integers 0-255")
			}
			if f < 0 || f > 255 {
				return fmt.Errorf("channel %d out of range 0-255: %d", i, f)
			}
			buf[i] = strconv.FormatInt(f, 10)
		}
		dst[key] = buf[0] + " " + buf[1] + " " + buf[2]
		return nil
	default:
		return fmt.Errorf("expected string or [r, g, b] array, got %T", value)
	}
}

func isHexColor(s string) bool {
	if !strings.HasPrefix(s, "#") {
		return false
	}
	n := len(s) - 1
	return n == 3 || n == 6 || n == 8
}

func hexToTriplet(s string) (string, error) {
	c := strings.TrimPrefix(s, "#")
	switch len(c) {
	case 3:
		c = string([]byte{c[0], c[0], c[1], c[1], c[2], c[2]})
	case 8:
		c = c[:6]
	}
	if len(c) != 6 {
		return "", fmt.Errorf("expected hex color, got %q", s)
	}
	v, err := strconv.ParseUint(c, 16, 32)
	if err != nil {
		return "", fmt.Errorf("expected hex color, got %q", s)
	}
	r := (v >> 16) & 0xff
	g := (v >> 8) & 0xff
	b := v & 0xff
	return strconv.FormatUint(r, 10) + " " + strconv.FormatUint(g, 10) + " " + strconv.FormatUint(b, 10), nil
}

func clamp01(v, fallback float64) float64 {
	if v < 0 || v > 1 {
		return fallback
	}
	return v
}

// DynamicProvider is an optional callback that supplies a dynamically generated theme (e.g. from active wallpaper).
var DynamicProvider func(name string) (*Theme, error)

// Get resolves a theme by name: dynamic provider, built-ins first, then user theme directory.
func Get(name string) (*Theme, error) {
	name = strings.TrimSpace(strings.ToLower(name))
	if name == "" {
		name = "phalune"
	}
	if !validSlug(name) {
		return nil, fmt.Errorf("invalid theme name %q", name)
	}

	if name == FromWallpaper && DynamicProvider != nil {
		if th, err := DynamicProvider(name); err == nil && th != nil {
			return th, nil
		}
	}

	if raw, ok := builtinRaw[name]; ok {
		return Parse(name, raw)
	}

	path, err := userThemePath(name)
	if err == nil {
		data, readErr := os.ReadFile(path)
		if readErr == nil {
			return Parse(name, string(data))
		}
	}

	// If from-wallpaper was requested but extraction failed, fall back to default theme.
	if name == FromWallpaper {
		if raw, ok := builtinRaw["phalune"]; ok {
			return Parse("phalune", raw)
		}
	}

	return nil, fmt.Errorf("theme %q not found (built-ins: %s)", name, strings.Join(Builtins(), ", "))
}

func userThemePath(name string) (string, error) {
	if name != filepath.Base(name) || strings.ContainsAny(name, "\\/. ") {
		return "", fmt.Errorf("invalid theme name %q", name)
	}
	var dir string
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		dir = filepath.Join(xdg, "phalune", "themes")
	} else if home, err := os.UserHomeDir(); err == nil {
		dir = filepath.Join(home, ".config", "phalune", "themes")
	} else {
		return "", fmt.Errorf("no config directory available")
	}
	return filepath.Join(dir, name+".toml"), nil
}

// CSS renders the theme as a :root CSS block with custom properties.
// DefaultTokens is the shared token fallback map.
var DefaultTokens = map[string]string{
	"surface":             "rgba(16, 19, 21, 0.75)",
	"surface-raised":      "rgba(21, 25, 29, 0.85)",
	"surface-overlay":     "rgba(21, 25, 29, 0.94)",
	"surface-solid":       "rgba(11, 14, 17, 0.98)",
	"surface-solid-color": "#0b0e11",
	"text":                "#e8eae6",
	"text-dim":            "#a8ada6",
	"text-faint":          "#8a938a",
	"text-bright":         "#ffffff",
	"accent-rgb":          "134 184 155",
	"accent":              "#86b89b",
	"accent-hover":        "#9ccbaf",
	"accent-active":       "#6fa687",
	"accent-contrast":     "#0d1712",
	"accent-contrast-rgb": "13 23 18",
	"muted":               "#6f7a72",
	"error-rgb":           "224 108 117",
	"error":               "#e06c75",
	"error-bright":        "#eb828a",
	"success":             "#7fb98a",
	"success-rgb":         "127 185 138",
	"warning":             "#e5c463",
	"warning-rgb":         "229 196 99",
	"hover-rgb":           "134 184 155",
	"shadow-rgb":          "6 8 10",
}

// TokensCSS renders the fallback :root block.
func TokensCSS() string {
	keys := make([]string, 0, len(DefaultTokens))
	for k := range DefaultTokens {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString(":root {\n")
	for _, k := range keys {
		b.WriteString("    --")
		b.WriteString(k)
		b.WriteString(": ")
		b.WriteString(DefaultTokens[k])
		b.WriteString(";\n")
	}
	b.WriteString("}\n")
	return b.String()
}

func (t *Theme) CSS(cssDefaults, overrides map[string]string) string {
	merged := make(map[string]string, len(cssDefaults)+len(t.Colors)+len(overrides)+4)
	for k, v := range cssDefaults {
		merged[k] = v
	}
	for k, v := range t.Colors {
		merged[k] = v
	}
	// Overrides win over both the theme and defaults.
	for k, v := range overrides {
		key, value, ok := normalizeOverride(k, v)
		if !ok {
			continue
		}
		merged[key] = value
	}

	explicit := make(map[string]bool, len(t.Colors)+len(overrides))
	for k := range t.Colors {
		explicit[k] = true
	}
	for k, v := range overrides {
		key, _, ok := normalizeOverride(k, v)
		if ok {
			explicit[key] = true
		}
	}

	if _, ok := merged["surface-overlay.rgb"]; !ok {
		if s, ok := merged["surface.rgb"]; ok {
			merged["surface-overlay.rgb"] = s
		} else if s, ok := merged["surface-solid.rgb"]; ok {
			merged["surface-overlay.rgb"] = s
		}
	}

	for _, surface := range surfaceNames {
		triplet, ok := merged[surface+".rgb"]
		if !ok {
			continue
		}
		delete(merged, surface+".rgb")
		alpha := t.OpacityBar
		if surface == "surface-raised" {
			alpha = (t.OpacityBar + t.OpacitySolid) / 2
		} else if surface == "surface-overlay" {
			alpha = t.OpacityOverlay
		} else if surface == "surface-solid" {
			alpha = t.OpacitySolid
		}
		merged[surface] = "rgba(" +
			strings.ReplaceAll(triplet, " ", ", ") + ", " +
			formatAlpha(alpha) + ")"
		if hex, ok := tripletToHex(triplet); ok {
			merged["surface-solid-color"] = hex
		}
	}

	deriveRGBTriplets(merged, cssDefaults, explicit)

	keys := make([]string, 0, len(merged))
	for k := range merged {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	b.WriteString("/* Generated by phalune theme: ")
	b.WriteString(t.Name)
	b.WriteString(" */\n:root {\n")
	for _, k := range keys {
		b.WriteString("    --")
		b.WriteString(k)
		b.WriteString(": ")
		b.WriteString(merged[k])
		b.WriteString(";\n")
	}
	b.WriteString("}\n")
	return b.String()
}

// normalizeOverride maps surface override keys to internal .rgb form.
func normalizeOverride(key, value string) (string, string, bool) {
	if isSurfaceToken(key) {
		if strings.Contains(value, " ") {
			return key + ".rgb", value, true
		}
		if triplet, err := hexToTriplet(value); err == nil {
			if t := strings.TrimPrefix(value, "#"); len(t) == 8 {
				slog.Warn("theme: 8-digit hex alpha channel ignored, using opaque RGB", "token", key)
			}
			return key + ".rgb", triplet, true
		}
		return key, value, false
	}
	return key, value, true
}

// deriveRGBTriplets populates missing "<token>-rgb" triplets from hex colors.
func deriveRGBTriplets(merged, cssDefaults map[string]string, explicit map[string]bool) {
	for k, v := range merged {
		if strings.HasSuffix(k, "-rgb") {
			continue
		}
		rgbKey := k + "-rgb"
		if _, wanted := cssDefaults[rgbKey]; !wanted {
			continue
		}
		if _, isExplicit := explicit[rgbKey]; isExplicit {
			continue
		}
		if !isHexColor(v) {
			continue
		}
		triplet, err := hexToTriplet(v)
		if err != nil {
			continue
		}
		merged[rgbKey] = triplet
	}
}

func formatAlpha(v float64) string {
	return strconv.FormatFloat(v, 'f', 2, 64)
}

func tripletToHex(triplet string) (string, bool) {
	parts := strings.Fields(triplet)
	if len(parts) != 3 {
		return "", false
	}
	var buf [3]byte
	for i, p := range parts {
		n, err := strconv.ParseUint(p, 10, 8)
		if err != nil {
			return "", false
		}
		buf[i] = byte(n)
	}
	return fmt.Sprintf("#%02x%02x%02x", buf[0], buf[1], buf[2]), true
}
