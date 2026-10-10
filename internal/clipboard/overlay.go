package clipboard

import (
	"fmt"
	"strings"
	"time"

	"phalune/ui"

	"github.com/diamondburned/gotk4-layer-shell/pkg/gtk4layershell"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// Overlay is the searchable clipboard history window.
type Overlay struct {
	window         *gtk.Window
	card           *gtk.Box
	searchEntry    *gtk.SearchEntry
	listBox        *gtk.ListBox
	scrolledWindow *gtk.ScrolledWindow
	noResultsLabel *gtk.Label
	clearButton    *gtk.Button

	watcher *Watcher
	current []*Entry
}

// NewOverlay builds the history overlay attached to the watcher.
func NewOverlay(app *gtk.Application, watcher *Watcher) (*Overlay, error) {
	win := gtk.NewWindow()
	win.SetApplication(app)
	win.SetTitle("phalune-clipboard")
	win.SetDecorated(false)
	win.AddCSSClass("clipboard-window")

	if !gtk4layershell.IsSupported() {
		return nil, fmt.Errorf("Wayland compositor does not support wlr-layer-shell protocol")
	}

	gtk4layershell.InitForWindow(win)
	gtk4layershell.SetNamespace(win, "phalune-clipboard")
	gtk4layershell.SetLayer(win, gtk4layershell.LayerShellLayerOverlay)

	gtk4layershell.SetAnchor(win, gtk4layershell.LayerShellEdgeTop, true)
	gtk4layershell.SetAnchor(win, gtk4layershell.LayerShellEdgeBottom, true)
	gtk4layershell.SetAnchor(win, gtk4layershell.LayerShellEdgeLeft, true)
	gtk4layershell.SetAnchor(win, gtk4layershell.LayerShellEdgeRight, true)
	gtk4layershell.SetExclusiveZone(win, -1)
	gtk4layershell.SetKeyboardMode(win, gtk4layershell.LayerShellKeyboardModeOnDemand)

	builder := gtk.NewBuilderFromString(ui.Clipboard)
	overlayBox := builder.GetObject("overlay_box").Cast().(*gtk.Overlay)
	card := builder.GetObject("card").Cast().(*gtk.Box)
	searchEntry := builder.GetObject("search_entry").Cast().(*gtk.SearchEntry)
	listBox := builder.GetObject("list_box").Cast().(*gtk.ListBox)
	scrolled := builder.GetObject("scrolled_window").Cast().(*gtk.ScrolledWindow)
	noResults := builder.GetObject("no_results_label").Cast().(*gtk.Label)
	clearBtn := builder.GetObject("clear_button").Cast().(*gtk.Button)

	win.SetChild(overlayBox)

	o := &Overlay{
		window:         win,
		card:           card,
		searchEntry:    searchEntry,
		listBox:        listBox,
		scrolledWindow: scrolled,
		noResultsLabel: noResults,
		clearButton:    clearBtn,
		watcher:        watcher,
	}

	o.setupInteractivity(overlayBox, card)
	watcher.OnChange(func() {
		if o.window.IsVisible() {
			o.refresh(false)
		}
	})

	return o, nil
}

func (o *Overlay) setupInteractivity(overlayBox *gtk.Overlay, card *gtk.Box) {
	// Outside click dismissal.
	click := gtk.NewGestureClick()
	click.ConnectReleased(func(n int, x, y float64) {
		pick := overlayBox.Pick(x, y, gtk.PickDefault)
		if pick != nil {
			w := gtk.BaseWidget(pick)
			if w != nil && (w == &card.Widget || w.IsAncestor(card)) {
				return
			}
		}
		o.Close()
	})
	overlayBox.AddController(click)

	keyCtrl := gtk.NewEventControllerKey()
	keyCtrl.ConnectKeyPressed(func(keyval, keycode uint, state gdk.ModifierType) bool {
		switch keyval {
		case gdk.KEY_Escape:
			o.Close()
			return true

		case gdk.KEY_Return, gdk.KEY_KP_Enter:
			if row := o.listBox.SelectedRow(); row != nil {
				o.activate(row.Index())
				return true
			}
			return true

		case gdk.KEY_Down:
			o.moveSelection(1)
			return true

		case gdk.KEY_Up:
			o.moveSelection(-1)
			return true
		}

		if state.Has(gdk.ControlMask) {
			switch keyval {
			case gdk.KEY_n, gdk.KEY_j:
				o.moveSelection(1)
				return true
			case gdk.KEY_p, gdk.KEY_k:
				o.moveSelection(-1)
				return true
			}
		}
		return false
	})
	o.window.AddController(keyCtrl)

	o.listBox.ConnectRowActivated(func(row *gtk.ListBoxRow) {
		o.activate(row.Index())
	})

	o.searchEntry.ConnectSearchChanged(func() {
		o.renderList(o.filtered())
	})

	o.clearButton.ConnectClicked(func() {
		o.watcher.Store().Clear()
		o.refresh(false)
	})
}

func (o *Overlay) filtered() []*Entry {
	query := o.searchEntry.Text()
	entries := o.watcher.Store().Entries()
	if query == "" {
		return entries
	}

	var out []*Entry
	q := lower(query)
	for _, e := range entries {
		if contains(lower(e.Preview), q) || contains(lower(e.Text), q) {
			out = append(out, e)
		}
	}
	return out
}

func (o *Overlay) refresh(resetQuery bool) {
	if resetQuery {
		o.searchEntry.SetText("")
	}
	o.renderList(o.filtered())
}

func (o *Overlay) renderList(entries []*Entry) {
	for {
		row := o.listBox.RowAtIndex(0)
		if row == nil {
			break
		}
		o.listBox.Remove(row)
	}

	o.current = entries

	if len(entries) == 0 {
		o.scrolledWindow.SetVisible(false)
		o.noResultsLabel.SetVisible(true)
		if strings.TrimSpace(o.searchEntry.Text()) == "" {
			o.noResultsLabel.SetText("Clipboard is empty")
		} else {
			o.noResultsLabel.SetText("No matching entries")
		}
		return
	}

	o.noResultsLabel.SetVisible(false)
	o.scrolledWindow.SetVisible(true)

	for _, e := range entries {
		rowBuilder := gtk.NewBuilderFromString(ui.ClipboardItem)
		row := rowBuilder.GetObject("clipboard_item").Cast().(*gtk.ListBoxRow)
		icon := rowBuilder.GetObject("row_icon").Cast().(*gtk.Image)
		nameLabel := rowBuilder.GetObject("row_name").Cast().(*gtk.Label)
		descLabel := rowBuilder.GetObject("row_desc").Cast().(*gtk.Label)
		delBtn := rowBuilder.GetObject("row_delete").Cast().(*gtk.Button)

		icon.SetFromIconName(iconFor(e))

		title, desc := describe(e)
		nameLabel.SetText(title)
		if desc != "" {
			descLabel.SetText(desc)
		} else {
			descLabel.SetVisible(false)
		}

		entry := e
		delBtn.ConnectClicked(func() {
			o.watcher.Store().Remove(entry.ID)
			o.refresh(false)
		})

		o.listBox.Append(row)
	}

	if first := o.listBox.RowAtIndex(0); first != nil {
		o.listBox.SelectRow(first)
	}
}

func (o *Overlay) activate(idx int) {
	if idx < 0 || idx >= len(o.current) {
		return
	}
	entry := o.current[idx]
	o.Close()
	o.watcher.Paste(entry)
}

func (o *Overlay) moveSelection(delta int) {
	n := len(o.current)
	if n == 0 {
		return
	}

	curIdx := 0
	if sel := o.listBox.SelectedRow(); sel != nil {
		curIdx = sel.Index()
	}

	nextIdx := curIdx + delta
	if nextIdx < 0 {
		nextIdx = 0
	} else if nextIdx >= n {
		nextIdx = n - 1
	}

	if row := o.listBox.RowAtIndex(nextIdx); row != nil {
		o.listBox.SelectRow(row)
	}
}

func iconFor(e *Entry) string {
	switch e.Kind {
	case KindImage:
		return "image-x-generic-symbolic"
	case KindFiles:
		return "folder-symbolic"
	default:
		return "edit-paste-symbolic"
	}
}

// describe returns (title, secondary) for an entry row.
func describe(e *Entry) (string, string) {
	switch e.Kind {
	case KindImage:
		return fmt.Sprintf("Image %dx%d", e.Width, e.Height), timeAgoString(e.CreatedAt)
	case KindFiles:
		uris := splitURIs(e.Text)
		first := ""
		if len(uris) > 0 {
			first = fileURIToPath(uris[0])
		}
		if len(uris) == 1 {
			return first, timeAgoString(e.CreatedAt)
		}
		return fmt.Sprintf("%d files", len(uris)), first
	default:
		title := e.Preview
		if title == "" {
			title = "(empty)"
		}
		return title, timeAgoString(e.CreatedAt)
	}
}

func (o *Overlay) Toggle() {
	if o.IsVisible() {
		o.Close()
	} else {
		o.Open()
	}
}

func (o *Overlay) Open() {
	o.refresh(true)
	o.window.Present()
	o.searchEntry.GrabFocus()
}

func (o *Overlay) Close() {
	o.window.SetVisible(false)
}

func (o *Overlay) IsVisible() bool {
	return o.window.IsVisible()
}

func (o *Overlay) Destroy() {
	if o.window != nil {
		if o.window.Realized() {
			o.window.Destroy()
		}
		o.window = nil
	}
}

// timeAgoString renders a coarse "X minutes ago" label.
func timeAgoString(ms int64) string {
	if ms == 0 {
		return ""
	}
	d := time.Now().UnixMilli() - ms
	if d < 0 {
		d = 0
	}
	switch {
	case d < 60_000:
		return "just now"
	case d < 3_600_000:
		return fmt.Sprintf("%d min ago", d/60_000)
	case d < 86_400_000:
		return fmt.Sprintf("%d hr ago", d/3_600_000)
	default:
		return fmt.Sprintf("%d days ago", d/86_400_000)
	}
}

func lower(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] >= 'A' && b[i] <= 'Z' {
			b[i] += 32
		}
	}
	return string(b)
}

func contains(haystack, needle string) bool {
	if needle == "" {
		return true
	}
	return indexOf(haystack, needle) >= 0
}

func indexOf(h, n string) int {
	for i := 0; i+len(n) <= len(h); i++ {
		if h[i:i+len(n)] == n {
			return i
		}
	}
	return -1
}
