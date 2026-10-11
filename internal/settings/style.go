package settings

import (
	_ "embed"
	"fmt"
	"log/slog"

	"phalune/internal/theme"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

//go:embed settings.css
var settingsCSS string

// LoadStyle applies the theme block, then the settings stylesheet.
func LoadStyle(themeName string, overrides map[string]string) error {
	display := gdk.DisplayGetDefault()
	if display == nil {
		return fmt.Errorf("no default GDK display available")
	}

	th, err := theme.Get(themeName)
	var tokens string
	if err != nil {
		slog.Warn("settings: unknown theme, using token defaults", "theme", themeName)
		tokens = theme.TokensCSS()
	} else {
		valid := make(map[string]string, len(overrides))
		for k, v := range overrides {
			if _, ok := theme.DefaultTokens[k]; ok {
				valid[k] = v
			} else {
				slog.Warn("settings: unknown theme token override, ignoring", "token", k)
			}
		}
		tokens = th.CSS(theme.DefaultTokens, valid)
	}

	provider := gtk.NewCSSProvider()
	provider.LoadFromString(tokens + "\n" + settingsCSS)
	gtk.StyleContextAddProviderForDisplay(display, provider, gtk.STYLE_PROVIDER_PRIORITY_USER)

	return nil
}
