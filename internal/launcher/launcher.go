package launcher

import (
	"fmt"
	"log/slog"

	"phalune/internal/config"
	"phalune/internal/style"
	"phalune/ui"

	"github.com/diamondburned/gotk4/pkg/core/glib"
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
	commands []commandEntry
	allApps  []App
	current  []Result
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
		commands:       commandList(nil),
	}

	styleName, styleClass := style.Resolve(style.Launcher, cfg.Style)
	l.applyStyle(styleName, styleClass)

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
			l.launchResult(l.current[idx])
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
					l.launchResult(l.current[idx])
					return true
				}
			}
			if len(l.current) > 0 {
				l.launchResult(l.current[0])
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
	l.current = FilterResults(l.allApps, l.commands, query, l.frecency)
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

	for _, res := range l.current {
		rowBuilder := gtk.NewBuilderFromString(ui.LauncherItem)
		row := rowBuilder.GetObject("launcher_item").Cast().(*gtk.ListBoxRow)
		icon := rowBuilder.GetObject("row_icon").Cast().(*gtk.Image)
		nameLabel := rowBuilder.GetObject("row_name").Cast().(*gtk.Label)
		descLabel := rowBuilder.GetObject("row_desc").Cast().(*gtk.Label)

		icon.SetFromIconName(res.IconName())
		nameLabel.SetText(res.Title())

		if desc := res.Subtitle(); desc != "" {
			descLabel.SetText(desc)
		} else {
			descLabel.SetVisible(false)
		}

		if res.Command != nil {
			row.AddCSSClass("launcher-row-command")
		} else if res.Action != nil {
			row.AddCSSClass("launcher-row-action")
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

func (l *Launcher) launchResult(res Result) {
	switch {
	case res.Command != nil:
		cmd := *res.Command
		l.Close()
		go func() {
			if err := cmd.Run(); err != nil {
				slog.Warn("launcher command failed", "command", cmd.Name, "error", err)
			}
		}()

	case res.Action != nil && res.App != nil:
		l.frecency.RecordLaunch(res.App.ID)
		l.Close()
		app := *res.App
		action := *res.Action
		go func() {
			if err := LaunchDesktopAction(app, action, l.terminal); err != nil {
				slog.Error("failed to launch desktop action", "app", app.Name, "action", action.Name, "error", err)
			}
		}()

	default:
		if res.App != nil {
			l.launch(*res.App)
		}
	}
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

	styleName, styleClass := style.Resolve(style.Launcher, cfg.Style)
	glib.IdleAdd(func() {
		l.applyStyle(styleName, styleClass)
	})
}

func (l *Launcher) applyStyle(styleName, styleClass string) {
	if l.card == nil {
		return
	}
	for _, s := range []string{"launcher-style-centered", "launcher-style-fullscreen", "launcher-style-compact", "launcher-style-default"} {
		l.card.RemoveCSSClass(s)
	}
	l.card.AddCSSClass(styleClass)

	switch styleName {
	case "fullscreen":
		l.card.SetSizeRequest(-1, -1)
		l.card.SetHAlign(gtk.AlignFill)
		l.card.SetVAlign(gtk.AlignFill)
		l.card.SetHExpand(true)
		l.card.SetVExpand(true)
		l.card.SetMarginTop(32)
		l.card.SetMarginBottom(32)
		l.card.SetMarginStart(48)
		l.card.SetMarginEnd(48)
	case "compact":
		l.card.SetHAlign(gtk.AlignCenter)
		l.card.SetVAlign(gtk.AlignCenter)
		l.card.SetHExpand(false)
		l.card.SetVExpand(false)
		l.card.SetSizeRequest(380, 320)
		l.card.SetMarginTop(0)
		l.card.SetMarginBottom(0)
		l.card.SetMarginStart(0)
		l.card.SetMarginEnd(0)
	default: // "centered"
		l.card.SetHAlign(gtk.AlignCenter)
		l.card.SetVAlign(gtk.AlignCenter)
		l.card.SetHExpand(false)
		l.card.SetVExpand(false)
		l.card.SetSizeRequest(520, 500)
		l.card.SetMarginTop(0)
		l.card.SetMarginBottom(0)
		l.card.SetMarginStart(0)
		l.card.SetMarginEnd(0)
	}
}

func (l *Launcher) SetShellCommands(cmds *ShellCommands) {
	if l == nil {
		return
	}
	l.commands = commandList(cmds)
	if l.window != nil && l.window.IsVisible() {
		l.updateFilter()
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
