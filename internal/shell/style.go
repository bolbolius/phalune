package shell

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

//go:embed default.css
var defaultCSS string

//go:embed animations.css
var animationsCSS string

func ApplyDefaultCSS() error {
	return ApplyCSS(defaultCSS + "\n" + animationsCSS)
}

func LoadStyle() error {
	css := defaultCSS
	if data, err := readUserConfig("style.css"); err == nil {
		css = data
	}

	anim := animationsCSS
	if data, err := readUserConfig("animations.css"); err == nil {
		anim = data
	}

	return ApplyCSS(css + "\n" + anim)
}

func ApplyCSS(cssData string) error {
	display := gdk.DisplayGetDefault()
	if display == nil {
		return fmt.Errorf("no default GDK display available")
	}

	provider := gtk.NewCSSProvider()
	provider.LoadFromString(cssData)

	gtk.StyleContextAddProviderForDisplay(display, provider, gtk.STYLE_PROVIDER_PRIORITY_USER)
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
