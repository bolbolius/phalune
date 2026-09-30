package settings

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"phalune/internal/config"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

type widgetRow struct {
	row      Row
	entry    *gtk.Entry
	combo    *gtk.DropDown
	switcher *gtk.Switch
	errorLbl *gtk.Label
	starLbl  *gtk.Label
}

type rowWidgets struct {
	container gtk.Widgetter
	entry     *gtk.Entry
	combo     *gtk.DropDown
	switcher  *gtk.Switch
	errorLbl  *gtk.Label
	starLbl   *gtk.Label
}

type App struct {
	window   *gtk.ApplicationWindow
	sidebar  *gtk.ListBox
	stack    *gtk.Stack
	status   *gtk.Label
	saveBtn  *gtk.Button
	resetBtn *gtk.Button

	editor     *Editor
	cfg        *config.Config
	pages      []Page
	widgetRows [][]widgetRow
	configPath string
	isUpdating bool
}

// ConfigLoc returns the active configuration file path.
func ConfigLoc() string {
	if explicit := os.Getenv("PHALUNE_CONFIG"); explicit != "" {
		return explicit
	}
	if p, err := config.DefaultConfigPath(); err == nil {
		return p
	}
	return "config.toml"
}

func NewWindow(app *gtk.Application) (*App, error) {
	path := ConfigLoc()
	editor, err := NewEditor(path)
	if err != nil {
		return nil, err
	}
	cfg, err := config.Load(path)
	if err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	pages := Pages()
	w := &App{
		window:     gtk.NewApplicationWindow(app),
		editor:     editor,
		cfg:        cfg,
		pages:      pages,
		configPath: path,
	}
	w.window.AddCSSClass("settings-window")
	w.window.SetTitle("Settings")
	w.window.SetDefaultSize(920, 620)

	// Action buttons in headerbar (GNOME style)
	w.status = gtk.NewLabel("")
	w.status.AddCSSClass("settings-status-badge")
	w.status.SetVAlign(gtk.AlignCenter)

	w.resetBtn = gtk.NewButtonWithLabel("Reset")
	w.resetBtn.AddCSSClass("settings-reset-btn")
	w.resetBtn.SetVAlign(gtk.AlignCenter)
	w.resetBtn.SetVisible(false)
	w.resetBtn.ConnectClicked(func() { w.reload() })

	w.saveBtn = gtk.NewButtonWithLabel("Save & Apply")
	w.saveBtn.AddCSSClass("settings-save-btn")
	w.saveBtn.SetVAlign(gtk.AlignCenter)
	w.saveBtn.SetVisible(false)
	w.saveBtn.ConnectClicked(func() { w.onSave() })

	hb := buildHeaderBar("Settings", w.status, w.resetBtn, w.saveBtn)
	w.window.SetTitlebar(hb)

	hbox := gtk.NewBox(gtk.OrientationHorizontal, 0)
	hbox.AddCSSClass("settings-body")

	// Sidebar
	sideScrolled := gtk.NewScrolledWindow()
	sideScrolled.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)
	sideScrolled.AddCSSClass("sidebar-scroll")
	w.sidebar = gtk.NewListBox()
	w.sidebar.AddCSSClass("sidebar")
	w.sidebar.AddCSSClass("navigation-sidebar")
	w.sidebar.SetSelectionMode(gtk.SelectionSingle)
	sideScrolled.SetChild(w.sidebar)
	sideScrolled.SetSizeRequest(240, -1)
	sideScrolled.SetVExpand(true)
	sideScrolled.SetHExpand(false)

	// Stack of pages
	w.stack = gtk.NewStack()
	w.stack.SetTransitionType(gtk.StackTransitionTypeNone)
	w.stack.SetVExpand(true)
	w.stack.SetHExpand(true)

	hbox.Append(sideScrolled)
	hbox.Append(w.stack)

	w.window.SetChild(hbox)

	w.buildSidebar()
	w.buildPages()

	w.sidebar.ConnectRowSelected(func(row *gtk.ListBoxRow) {
		if row == nil {
			return
		}
		idx := row.Index()
		if idx >= 0 && idx < len(w.pages) {
			w.stack.SetVisibleChildName(w.pages[idx].ID)
		}
	})

	keyCtrl := gtk.NewEventControllerKey()
	keyCtrl.ConnectKeyPressed(func(keyval, keycode uint, state gdk.ModifierType) bool {
		if (state&gdk.ControlMask != 0) && (keyval == gdk.KEY_s || keyval == gdk.KEY_S) {
			if w.saveBtn.Visible() {
				w.onSave()
			}
			return true
		}
		return false
	})
	w.window.AddController(keyCtrl)

	// Ensure dirty indicators and buttons match initial state
	w.checkDirty()

	return w, nil
}

func (w *App) buildSidebar() {
	for _, p := range w.pages {
		row := gtk.NewListBoxRow()

		hbox := gtk.NewBox(gtk.OrientationHorizontal, 12)
		hbox.AddCSSClass("settings-sidebar-row")
		hbox.SetVAlign(gtk.AlignCenter)

		icon := gtk.NewImageFromIconName(p.Icon)
		icon.SetPixelSize(18)
		icon.AddCSSClass("settings-sidebar-icon")

		label := gtk.NewLabel(p.Title)
		label.AddCSSClass("settings-sidebar-label")
		label.SetHAlign(gtk.AlignStart)
		label.SetHExpand(true)

		hbox.Append(icon)
		hbox.Append(label)
		row.SetChild(hbox)

		w.sidebar.Append(row)
	}
}

func (w *App) buildPages() {
	w.widgetRows = make([][]widgetRow, len(w.pages))
	for i, p := range w.pages {
		w.widgetRows[i] = w.buildPage(p)
	}
	if n := w.sidebar.RowAtIndex(0); n != nil {
		w.sidebar.SelectRow(n)
	}

	// Wire change listeners to check dirty state
	for pi := range w.widgetRows {
		for _, wr := range w.widgetRows[pi] {
			if wr.entry != nil {
				wr.entry.ConnectChanged(func() {
					if !w.isUpdating {
						w.checkDirty()
					}
				})
			}
			if wr.switcher != nil {
				wr.switcher.NotifyProperty("active", func() {
					if !w.isUpdating {
						w.checkDirty()
					}
				})
			}
			if wr.combo != nil {
				wr.combo.NotifyProperty("selected", func() {
					if !w.isUpdating {
						w.checkDirty()
					}
				})
			}
		}
	}
}

func (w *App) buildPage(p Page) []widgetRow {
	scroll := gtk.NewScrolledWindow()
	scroll.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)
	scroll.AddCSSClass("page-scroll")

	outer := gtk.NewBox(gtk.OrientationVertical, 0)
	outer.SetHAlign(gtk.AlignCenter)
	outer.SetHExpand(true)
	outer.SetSizeRequest(620, -1)
	outer.AddCSSClass("page-column")

	title := gtk.NewLabel(p.Title)
	title.AddCSSClass("page-title")
	title.SetHAlign(gtk.AlignStart)
	outer.Append(title)

	rows := make([]widgetRow, 0, len(p.Rows))
	groups := p.groups()

	for gi, bounds := range groups {
		if gi < len(p.Titles) && p.Titles[gi] != "" {
			t := gtk.NewLabel(p.Titles[gi])
			t.AddCSSClass("settings-group-title")
			if gi == 0 {
				t.AddCSSClass("first")
			}
			t.SetHAlign(gtk.AlignStart)
			outer.Append(t)
		}

		card := gtk.NewBox(gtk.OrientationVertical, 0)
		card.AddCSSClass("settings-group")

		for ri := bounds[0]; ri < bounds[1] && ri < len(p.Rows); ri++ {
			wr := w.buildRow(p.Rows[ri])
			card.Append(wr.container)
			rows = append(rows, widgetRow{
				row:      p.Rows[ri],
				entry:    wr.entry,
				combo:    wr.combo,
				switcher: wr.switcher,
				errorLbl: wr.errorLbl,
				starLbl:  wr.starLbl,
			})
		}

		outer.Append(card)
	}

	content := gtk.NewBox(gtk.OrientationVertical, 0)
	content.AddCSSClass("page-content")
	content.Append(outer)

	scroll.SetChild(content)
	w.stack.AddNamed(scroll, p.ID)

	return rows
}

func (w *App) buildRow(row Row) rowWidgets {
	hbox := gtk.NewBox(gtk.OrientationHorizontal, 16)
	hbox.AddCSSClass("settings-row")
	hbox.SetHAlign(gtk.AlignFill)
	hbox.SetHExpand(true)

	vbox := gtk.NewBox(gtk.OrientationVertical, 2)
	vbox.SetHAlign(gtk.AlignStart)
	vbox.SetHExpand(true)
	vbox.SetVAlign(gtk.AlignCenter)

	titleBox := gtk.NewBox(gtk.OrientationHorizontal, 6)
	titleBox.SetHAlign(gtk.AlignStart)
	titleBox.SetVAlign(gtk.AlignCenter)

	lbl := gtk.NewLabel(row.Label)
	lbl.AddCSSClass("settings-row-label")
	lbl.SetHAlign(gtk.AlignStart)
	titleBox.Append(lbl)

	starLbl := gtk.NewLabel("*")
	starLbl.AddCSSClass("settings-row-star")
	starLbl.SetVisible(false)
	titleBox.Append(starLbl)

	vbox.Append(titleBox)

	if row.Hint != "" {
		hint := gtk.NewLabel(row.Hint)
		hint.AddCSSClass("settings-row-hint")
		hint.SetHAlign(gtk.AlignStart)
		vbox.Append(hint)
	}

	errLbl := gtk.NewLabel("")
	errLbl.AddCSSClass("settings-row-error")
	errLbl.SetHAlign(gtk.AlignStart)
	errLbl.SetVisible(false)
	vbox.Append(errLbl)

	out := rowWidgets{container: hbox, errorLbl: errLbl, starLbl: starLbl}

	switch row.Kind {
	case KindToggle:
		sw := gtk.NewSwitch()
		sw.AddCSSClass("settings-row-switch")
		sw.SetVAlign(gtk.AlignCenter)
		sw.SetHAlign(gtk.AlignEnd)
		sw.SetActive(strings.ToLower(w.currentValue(row)) == "true")
		out.switcher = sw
		hbox.Append(vbox)
		hbox.Append(sw)

	case KindChoice:
		combo := gtk.NewDropDownFromStrings(stringsToGStrv(row.Choices))
		combo.AddCSSClass("settings-row-combo")
		combo.SetVAlign(gtk.AlignCenter)
		combo.SetHAlign(gtk.AlignEnd)
		cur := w.currentValue(row)
		for i, c := range row.Choices {
			if c == cur {
				combo.SetSelected(uint(i))
				break
			}
		}
		out.combo = combo
		hbox.Append(vbox)
		hbox.Append(combo)

	default:
		entry := gtk.NewEntry()
		entry.AddCSSClass("settings-row-entry")
		entry.SetVAlign(gtk.AlignCenter)
		entry.SetSizeRequest(200, -1)
		entry.SetText(w.currentValue(row))
		entry.SetHExpand(false)
		entry.ConnectChanged(func() {
			if _, err := ParseValue(row, entry.Text()); err != nil {
				errLbl.SetText(err.Error())
				errLbl.SetVisible(true)
				entry.AddCSSClass("error")
			} else {
				errLbl.SetVisible(false)
				entry.RemoveCSSClass("error")
			}
		})
		out.entry = entry

		if row.Unit != "" {
			entry.AddCSSClass("has-unit")

			unitLbl := gtk.NewLabel(row.Unit)
			unitLbl.AddCSSClass("settings-entry-unit")
			unitLbl.SetVAlign(gtk.AlignCenter)
			unitLbl.SetHAlign(gtk.AlignEnd)
			unitLbl.SetMarginEnd(10)
			unitLbl.SetCanTarget(false)
			unitLbl.SetCanFocus(false)

			overlay := gtk.NewOverlay()
			overlay.SetChild(entry)
			overlay.AddOverlay(unitLbl)
			overlay.SetVAlign(gtk.AlignCenter)
			overlay.SetHAlign(gtk.AlignEnd)
			overlay.SetSizeRequest(200, -1)

			hbox.Append(vbox)
			hbox.Append(overlay)
		} else {
			entry.SetHAlign(gtk.AlignEnd)
			hbox.Append(vbox)
			hbox.Append(entry)
		}
	}

	return out
}

func (w *App) currentValue(row Row) string {
	return readString(w.cfg, row.Key)
}

func (w *App) rowCurrentValue(wr widgetRow) string {
	switch {
	case wr.switcher != nil:
		return boolStr(wr.switcher.Active())
	case wr.combo != nil:
		sel := int(wr.combo.Selected())
		if sel >= 0 && sel < len(wr.row.Choices) {
			return wr.row.Choices[sel]
		}
	case wr.entry != nil:
		return wr.entry.Text()
	}
	return ""
}

func (w *App) checkDirty() {
	anyDirty := false

	for pi := range w.pages {
		for _, wr := range w.widgetRows[pi] {
			orig := readString(w.cfg, wr.row.Key)
			cur := w.rowCurrentValue(wr)

			dirty := false
			if parsed, err := ParseValue(wr.row, cur); err == nil {
				dirty = (parsed != orig)
			} else {
				dirty = (cur != orig)
			}

			if wr.starLbl != nil {
				wr.starLbl.SetVisible(dirty)
			}
			if dirty {
				anyDirty = true
			}
		}
	}

	w.saveBtn.SetVisible(anyDirty)
	w.resetBtn.SetVisible(anyDirty)
	if anyDirty {
		w.setStatus("", false)
	}
}

func (w *App) reload() {
	cfg, err := config.Load(w.configPath)
	if err != nil {
		w.setStatus("Reload failed: "+err.Error(), false)
		return
	}
	w.cfg = cfg
	w.isUpdating = true
	for pi := range w.pages {
		for ri, wr := range w.widgetRows[pi] {
			val := readString(w.cfg, wr.row.Key)
			switch {
			case wr.switcher != nil:
				w.widgetRows[pi][ri].switcher.SetActive(strings.ToLower(val) == "true")
			case wr.combo != nil:
				for i, c := range wr.row.Choices {
					if c == val {
						w.widgetRows[pi][ri].combo.SetSelected(uint(i))
						break
					}
				}
			case wr.entry != nil:
				w.widgetRows[pi][ri].entry.SetText(val)
			}
		}
	}
	w.isUpdating = false
	w.checkDirty()
	w.setStatus("Reloaded", false)
}

func (w *App) collect() ([]struct {
	Row   Row
	Value string
	Old   string
}, error) {
	var out []struct {
		Row   Row
		Value string
		Old   string
	}

	for pi := range w.pages {
		for _, wr := range w.widgetRows[pi] {
			old := readString(w.cfg, wr.row.Key)
			v := w.rowCurrentValue(wr)

			parsed, err := ParseValue(wr.row, v)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", wr.row.Label, err)
			}
			if parsed != old {
				out = append(out, struct {
					Row   Row
					Value string
					Old   string
				}{wr.row, parsed, old})
			}
		}
	}
	return out, nil
}

func (w *App) onSave() {
	changes, err := w.collect()
	if err != nil {
		w.setStatus(err.Error(), false)
		return
	}
	if len(changes) == 0 {
		w.setStatus("No changes", false)
		return
	}

	for _, c := range changes {
		if err := w.editor.Set(c.Row, c.Value); err != nil {
			w.setStatus("Edit failed: "+err.Error(), false)
			return
		}
	}
	if err := w.editor.Save(); err != nil {
		w.setStatus("Save failed: "+err.Error(), false)
		return
	}

	reloaded, err := w.reloadShell()
	if err != nil {
		w.setStatus("Saved (shell reload failed: "+err.Error()+")", false)
		return
	}

	w.cfg, err = config.Load(w.configPath)
	if err != nil {
		w.setStatus("Saved (config now invalid: "+err.Error()+")", false)
		return
	}
	w.checkDirty()
	if reloaded {
		w.setStatus(fmt.Sprintf("Saved %d change(s)", len(changes)), true)
	} else {
		w.setStatus(fmt.Sprintf("Saved %d change(s) (shell offline)", len(changes)), true)
	}
}

func (w *App) reloadShell() (bool, error) {
	resp, err := ipcSend("reload-config")
	if err != nil {
		slog.Debug("settings: shell reload skipped", "error", err)
		return false, nil
	}
	if resp.errMsg != "" {
		return false, fmt.Errorf("%s", resp.errMsg)
	}
	return true, nil
}

func (w *App) setStatus(text string, saved bool) {
	if w.status == nil {
		return
	}
	w.status.SetText(text)
	w.status.RemoveCSSClass("saved")
	w.status.RemoveCSSClass("error")
	if text == "" {
		w.status.SetVisible(false)
		return
	}
	w.status.SetVisible(true)
	if saved {
		w.status.AddCSSClass("saved")
	} else if strings.Contains(strings.ToLower(text), "fail") || strings.Contains(strings.ToLower(text), "invalid") {
		w.status.AddCSSClass("error")
	}
}

func (w *App) Show() { w.window.SetVisible(true) }

func stringsToGStrv(items []string) []string { return items }
