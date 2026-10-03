package osd

import (
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// layoutResult bundles the widgets an OSD template must provide. All
// styles serve the same state; only this tree differs.
type layoutResult struct {
	card     *gtk.Box
	icon     *gtk.Image
	label    *gtk.Label
	progress *gtk.ProgressBar
	value    *gtk.Label
}

// newOSDIcon returns the icon image shared by all templates.
func newOSDIcon() *gtk.Image {
	icon := gtk.NewImage()
	icon.SetPixelSize(24)
	icon.AddCSSClass("osd-icon")
	return icon
}

// buildOSDLayout constructs the widget tree for a templated style
// ("pill", "bar", "minimal"). The default style uses the Blueprint UI.
func buildOSDLayout(styleName string) (card *gtk.Box, icon *gtk.Image, label *gtk.Label, progress *gtk.ProgressBar, value *gtk.Label) {
	switch styleName {
	case "bar":
		// Horizontal strip: icon, progress, value. No text label.
		card = gtk.NewBox(gtk.OrientationHorizontal, 12)
		icon = newOSDIcon()
		progress = gtk.NewProgressBar()
		progress.SetVAlign(gtk.AlignCenter)
		progress.SetHExpand(true)
		progress.SetSizeRequest(240, 8)
		progress.AddCSSClass("osd-progress")
		value = gtk.NewLabel("0%")
		value.AddCSSClass("osd-value")
		value.SetVAlign(gtk.AlignCenter)

		card.Append(icon)
		card.Append(progress)
		card.Append(value)
		card.AddCSSClass("osd-card")
		card.AddCSSClass("osd-layout-bar")
		return card, icon, nil, progress, value

	case "minimal":
		// Icon over a short bar, no text at all.
		card = gtk.NewBox(gtk.OrientationVertical, 8)
		card.SetSizeRequest(80, 72)
		icon = newOSDIcon()
		icon.SetPixelSize(32)
		icon.SetHAlign(gtk.AlignCenter)
		progress = gtk.NewProgressBar()
		progress.SetSizeRequest(56, 5)
		progress.AddCSSClass("osd-progress")
		progress.SetHAlign(gtk.AlignCenter)

		card.Append(icon)
		card.Append(progress)
		card.AddCSSClass("osd-card")
		card.AddCSSClass("osd-layout-minimal")
		return card, icon, nil, progress, nil

	default: // "pill"
		// Same composition as the default template, Go-built.
		card = gtk.NewBox(gtk.OrientationHorizontal, 12)
		icon = newOSDIcon()
		label = gtk.NewLabel("")
		label.AddCSSClass("osd-label")
		progress = gtk.NewProgressBar()
		progress.SetVAlign(gtk.AlignCenter)
		progress.SetSizeRequest(140, 8)
		progress.AddCSSClass("osd-progress")
		value = gtk.NewLabel("")
		value.AddCSSClass("osd-value")

		card.Append(icon)
		card.Append(label)
		card.Append(progress)
		card.Append(value)
		card.AddCSSClass("osd-card")
		card.AddCSSClass("osd-layout-pill")
		card.SetHAlign(gtk.AlignCenter)
		card.SetVAlign(gtk.AlignCenter)
		return card, icon, label, progress, value
	}
}
