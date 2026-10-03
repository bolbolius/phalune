// Package style resolves per-component UI style names from config.
// A style controls presentation and layout only; every style serves the
// same underlying state. Component code must not branch on style names,
// except to pick a layout template when one exists.
package style

import (
	"log/slog"
	"strings"
)

// Components that expose pluggable styles.
const (
	Bar           = "bar"
	ControlCenter = "control-center"
	Launcher      = "launcher"
	OSD           = "osd"
	Notifications = "notifications"
)

// styles maps component → set of valid style names ("" key = default).
var styles = map[string]map[string]bool{
	Bar:           {"": true, "bubble": true, "solid": true, "minimal": true},
	ControlCenter: {"": true, "cards": true, "compact": true},
	Launcher:      {"": true, "centered": true, "fullscreen": true, "compact": true},
	OSD:           {"": true, "pill": true, "bar": true, "minimal": true},
	Notifications: {"": true, "bubbles": true, "compact": true},
}

// Valid reports whether name is a registered style for component.
func Valid(component, name string) bool {
	set, ok := styles[component]
	if !ok {
		return false
	}
	return set[strings.ToLower(strings.TrimSpace(name))]
}

// Resolve validates name and returns the canonical style name and the CSS
// class to attach to the component root ("<component>-style-<name>").
// Invalid names fall back to "" with a warning; the shell never crashes
// over styles.
func Resolve(component, name string) (style, class string) {
	name = strings.ToLower(strings.TrimSpace(name))
	if !Valid(component, name) {
		slog.Warn("style: unknown style, using default", "component", component, "got", name)
		name = ""
	}
	if name == "" {
		return "", component + "-style-default"
	}
	return name, component + "-style-" + name
}

// HasTemplate reports whether the component ships a dedicated layout
// template for the style. Components without one render via CSS classes
// only.
func HasTemplate(component, name string) bool {
	return templates[component+":"+name]
}

// templates records which (component, style) pairs ship a dedicated
// layout template.
var templates = map[string]bool{
	OSD + ":pill":    true,
	OSD + ":bar":     true,
	OSD + ":minimal": true,
}
