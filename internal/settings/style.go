package settings

import (
	_ "embed"
	"fmt"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

//go:embed settings.css
var settingsCSS string

// LoadStyle applies the settings app stylesheet.
func LoadStyle() error {
	display := gdk.DisplayGetDefault()
	if display == nil {
		return fmt.Errorf("no default GDK display available")
	}

	provider := gtk.NewCSSProvider()
	provider.LoadFromString(settingsCSS)
	gtk.StyleContextAddProviderForDisplay(display, provider, gtk.STYLE_PROVIDER_PRIORITY_USER)

	return nil
}
