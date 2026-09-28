package launcher

import (
	"fmt"
	"log/slog"

	"phalune/internal/config"
	"phalune/ui"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

type Launcher struct {
	window         *gtk.Window
	card           *gtk.Box
	searchEntry    *gtk.SearchEntry
	listBox        *gtk.ListBox
	scrolledWindow *gtk.ScrolledWindow
	noResultsLabel *gtk.Label

	pageSize int
	terminal string

	frecency *FrecencyStore
	allApps  []App
	current  []App
}

func New(app *gtk.Application, cfg config.LauncherConfig) (*Launcher, error) {
	win := gtk.NewWindow()
	win.SetApplication(app)
	win.SetTitle("phalune-launcher")
	win.SetDecorated(false)
	win.AddCSSClass("launcher-window")

	if err := ConfigureLauncherSurface(win); err != nil {
		return nil, fmt.Errorf("failed to configure launcher layer-surface: %w", err)
	}

	builder := gtk.NewBuilderFromString(ui.Launcher)
	overlayBox := builder.GetObject("overlay_box").Cast().(*gtk.Overlay)
	card := builder.GetObject("card").Cast().(*gtk.Box)
	searchEntry := builder.GetObject("search_entry").Cast().(*gtk.SearchEntry)
	listBox := builder.GetObject("list_box").Cast().(*gtk.ListBox)
	scrolled := builder.GetObject("scrolled_window").Cast().(*gtk.ScrolledWindow)
	noResults := builder.GetObject("no_results_label").Cast().(*gtk.Label)

	win.SetChild(overlayBox)

	pageSize := cfg.PageSize
	if pageSize <= 0 {
		pageSize = 6
	}

	frecency := NewFrecencyStoreWithParams(cfg.Frecency.HalfLifeDays, cfg.Frecency.MaxBoost)

	l := &Launcher{
		window:         win,
		card:           card,
		searchEntry:    searchEntry,
		listBox:        listBox,
		scrolledWindow: scrolled,
		noResultsLabel: noResults,
		pageSize:       pageSize,
		terminal:       cfg.Terminal,
		frecency:       frecency,
	}

	l.setupKeyNavigation()
	l.setupInteractivity(overlayBox, card)

	return l, nil
}

func (l *Launcher) setupInteractivity(overlayBox *gtk.Overlay, card *gtk.Box) {
	click := gtk.NewGestureClick()
	click.ConnectReleased(func(n int, x, y float64) {
		pick := overlayBox.Pick(x, y, gtk.PickDefault)
		if pick != nil {
			w := gtk.BaseWidget(pick)
			if w != nil && (w == &card.Widget || w.IsAncestor(card)) {
				return
			}
		}
		l.Close()
	})
	overlayBox.AddController(click)

	l.listBox.ConnectRowActivated(func(row *gtk.ListBoxRow) {
		idx := row.Index()
		if idx >= 0 && idx < len(l.current) {
			l.launch(l.current[idx])
		}
	})

	l.searchEntry.ConnectSearchChanged(func() {
		l.updateFilter()
	})
}

func (l *Launcher) setupKeyNavigation() {
	keyCtrl := gtk.NewEventControllerKey()
	keyCtrl.ConnectKeyPressed(func(keyval, keycode uint, state gdk.ModifierType) bool {
		switch keyval {
		case gdk.KEY_Escape:
			l.Close()
			return true

		case gdk.KEY_Return, gdk.KEY_KP_Enter:
			row := l.listBox.SelectedRow()
			if row != nil {
				idx := row.Index()
				if idx >= 0 && idx < len(l.current) {
					l.launch(l.current[idx])
					return true
				}
			}
			if len(l.current) > 0 {
				l.launch(l.current[0])
				return true
			}
			return true

		case gdk.KEY_Down:
			l.moveSelection(1)
			return true

		case gdk.KEY_Up:
			l.moveSelection(-1)
			return true

		case gdk.KEY_Page_Down:
			l.moveSelection(l.pageSize)
			return true

		case gdk.KEY_Page_Up:
			l.moveSelection(-l.pageSize)
			return true
		}

		if state.Has(gdk.ControlMask) {
			switch keyval {
			case gdk.KEY_n, gdk.KEY_j:
				l.moveSelection(1)
				return true
			case gdk.KEY_p, gdk.KEY_k:
				l.moveSelection(-1)
				return true
			}
		}

		return false
	})

	l.window.AddController(keyCtrl)
}

func (l *Launcher) moveSelection(delta int) {
	n := len(l.current)
	if n == 0 {
		return
	}

	curIdx := 0
	if sel := l.listBox.SelectedRow(); sel != nil {
		curIdx = sel.Index()
	}

	nextIdx := curIdx + delta
	if nextIdx < 0 {
		nextIdx = 0
	} else if nextIdx >= n {
		nextIdx = n - 1
	}

	if row := l.listBox.RowAtIndex(nextIdx); row != nil {
		l.listBox.SelectRow(row)
		row.GrabFocus()
		l.searchEntry.GrabFocus()
	}
}

func (l *Launcher) updateFilter() {
	query := l.searchEntry.Text()
	l.current = FilterApps(l.allApps, query, l.frecency)
	l.renderList()
}

func (l *Launcher) renderList() {
	for {
		row := l.listBox.RowAtIndex(0)
		if row == nil {
			break
		}
		l.listBox.Remove(row)
	}

	if len(l.current) == 0 {
		l.scrolledWindow.SetVisible(false)
		l.noResultsLabel.SetVisible(true)
		return
	}

	l.noResultsLabel.SetVisible(false)
	l.scrolledWindow.SetVisible(true)

	for _, app := range l.current {
		rowBuilder := gtk.NewBuilderFromString(ui.LauncherItem)
		row := rowBuilder.GetObject("launcher_item").Cast().(*gtk.ListBoxRow)
		icon := rowBuilder.GetObject("row_icon").Cast().(*gtk.Image)
		nameLabel := rowBuilder.GetObject("row_name").Cast().(*gtk.Label)
		descLabel := rowBuilder.GetObject("row_desc").Cast().(*gtk.Label)

		iconName := app.Icon
		if iconName == "" {
			iconName = "application-x-executable"
		}
		icon.SetFromIconName(iconName)
		nameLabel.SetText(app.Name)

		desc := app.GenericName
		if desc == "" {
			desc = app.Comment
		}
		if desc != "" {
			descLabel.SetText(desc)
		} else {
			descLabel.SetVisible(false)
		}

		l.listBox.Append(row)
	}

	if first := l.listBox.RowAtIndex(0); first != nil {
		l.listBox.SelectRow(first)
	}
}

func (l *Launcher) launch(app App) {
	l.frecency.RecordLaunch(app.ID)
	l.Close()

	go func() {
		if err := LaunchApp(app, l.terminal); err != nil {
			slog.Error("failed to launch application", "app", app.Name, "error", err)
		}
	}()
}

func (l *Launcher) Toggle() {
	if l.IsVisible() {
		l.Close()
	} else {
		l.Open()
	}
}

func (l *Launcher) Open() {
	apps, err := ScanApplications()
	if err == nil {
		l.allApps = apps
	}

	l.searchEntry.SetText("")
	l.updateFilter()

	l.window.Present()
	l.searchEntry.GrabFocus()
}

func (l *Launcher) Close() {
	l.window.SetVisible(false)
}

func (l *Launcher) IsVisible() bool {
	return l.window.IsVisible()
}

func (l *Launcher) UpdateConfig(cfg config.LauncherConfig) {
	if l == nil {
		return
	}
	pageSize := cfg.PageSize
	if pageSize <= 0 {
		pageSize = 6
	}
	l.pageSize = pageSize
	l.terminal = cfg.Terminal
	if l.frecency != nil {
		l.frecency.SetParams(cfg.Frecency.HalfLifeDays, cfg.Frecency.MaxBoost)
	}
}

func (l *Launcher) Destroy() {
	if l.window != nil {
		if l.window.Realized() {
			l.window.Destroy()
		}
		l.window = nil
	}
}
