package shell

import (
	_ "embed"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"phalune/internal/theme"
)

//go:embed default.css
var defaultCSS string

//go:embed tokens.css
var tokensCSS string

//go:embed animations.css
var animationsCSS string

// CSSDefaults maps known token names to their default fallback values.
var CSSDefaults = map[string]string{
	"surface":             "rgba(26, 27, 38, 0.75)",
	"surface-raised":      "rgba(26, 27, 38, 0.85)",
	"surface-overlay":     "rgba(26, 27, 38, 0.94)",
	"surface-solid":       "rgba(22, 22, 30, 0.98)",
	"surface-solid-color": "#16161e",
	"text":                "#c0caf5",
	"text-dim":            "#a9b1d6",
	"text-faint":          "#787c99",
	"text-bright":         "#ffffff",
	"accent-rgb":          "122 162 247",
	"accent":              "#7aa2f7",
	"accent-hover":        "#89b4fa",
	"accent-active":       "#b4befe",
	"accent-contrast":     "#1a1b26",
	"accent-contrast-rgb": "26 27 38",
	"muted":               "#565f89",
	"error-rgb":           "247 118 142",
	"error":               "#f7768e",
	"error-bright":        "#ff9eaf",
	"success":             "#9ece6a",
	"warning":             "#e0af68",
	"hover-rgb":           "255 255 255",
	"shadow-rgb":          "0 0 0",
}

// ApplyDefaultCSS applies the built-in default stylesheet with no theme.
func ApplyDefaultCSS() error {
	return ApplyCSS(defaultCSS + "\n" + tokensCSS + "\n" + animationsCSS)
}

// LoadStyle builds and applies the combined stylesheet.
func LoadStyle(cfgThemeName string, overrides map[string]string) error {
	var b []byte
	b = append(b, defaultCSS...)
	b = append(b, '\n', '\n')
	b = append(b, tokensCSS...)
	b = append(b, '\n', '\n')
	b = append(b, themeBlock(cfgThemeName, overrides)...)
	b = append(b, '\n', '\n')
	b = append(b, animationsCSS...)
	b = append(b, '\n', '\n')

	if data, err := readUserConfig("style.css"); err == nil {
		b = append(b, data...)
		b = append(b, '\n')
	}

	return ApplyCSS(string(b))
}

// themeBlock renders the :root CSS block for the configured theme and overrides.
func themeBlock(cfgThemeName string, overrides map[string]string) []byte {
	validOverrides := make(map[string]string, len(overrides))
	for k, v := range overrides {
		if _, ok := CSSDefaults[k]; ok {
			validOverrides[k] = v
		} else {
			slog.Warn("theme: unknown token in [theme.values], ignoring", "token", k)
		}
	}

	th, err := theme.Get(cfgThemeName)
	if err != nil {
		slog.Warn("theme: falling back to built-in token defaults", "error", err)
		return []byte("/* theme fallback: token defaults unchanged */\n")
	}
	return []byte(th.CSS(CSSDefaults, validOverrides))
}

var userCSSProvider *gtk.CSSProvider

func ApplyCSS(cssData string) error {
	display := gdk.DisplayGetDefault()
	if display == nil {
		return fmt.Errorf("no default GDK display available")
	}

	if userCSSProvider == nil {
		userCSSProvider = gtk.NewCSSProvider()
		gtk.StyleContextAddProviderForDisplay(display, userCSSProvider, gtk.STYLE_PROVIDER_PRIORITY_USER)
	}
	userCSSProvider.LoadFromString(cssData)
	return nil
}

func readUserConfig(filename string) (string, error) {
	var dir string
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		dir = filepath.Join(xdg, "phalune")
	} else if home, err := os.UserHomeDir(); err == nil {
		dir = filepath.Join(home, ".config", "phalune")
	} else {
		return "", err
	}
	data, err := os.ReadFile(filepath.Join(dir, filename))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// DefaultCSS exposes the embedded component stylesheet for tooling.
func DefaultCSS() string { return defaultCSS }

// TokensCSS exposes the embedded token fallbacks for tooling.
func TokensCSS() string { return tokensCSS }
