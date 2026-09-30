package settings

import (
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

func buildHeaderBar(title string, status *gtk.Label, resetBtn, saveBtn *gtk.Button) *gtk.HeaderBar {
	hb := gtk.NewHeaderBar()
	hb.SetShowTitleButtons(true)

	lbl := gtk.NewLabel(title)
	lbl.AddCSSClass("title")
	hb.SetTitleWidget(lbl)

	if status != nil {
		status.SetVAlign(gtk.AlignCenter)
		hb.PackEnd(status)
	}
	if resetBtn != nil {
		resetBtn.SetVAlign(gtk.AlignCenter)
		hb.PackEnd(resetBtn)
	}
	if saveBtn != nil {
		saveBtn.SetVAlign(gtk.AlignCenter)
		hb.PackEnd(saveBtn)
	}

	return hb
}
