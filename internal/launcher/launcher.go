package launcher

import (
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"sync"

	"phalune/internal/config"
	"phalune/internal/style"
	"phalune/ui"

	"github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"
)

type Launcher struct {
	window       *gtk.Window
	overlayBox   *gtk.Overlay
	card         *gtk.Box
	searchEntry  *gtk.SearchEntry
	contentStack *gtk.Stack
	listBox      *gtk.ListBox
	gridFlowBox  *gtk.FlowBox
	frequentBox  *gtk.Box

	chipSettings   *gtk.Button
	chipTerminal   *gtk.Button
	chipScreenshot *gtk.Button
	chipLock       *gtk.Button
	chipsBox       *gtk.Box
	footerHints    *gtk.Box

	pageSize int
	terminal string

	currentStyle  string
	frecency      *FrecencyStore
	commands      []commandEntry
	rawCommands   *ShellCommands
	allApps       []App
	appsMu        sync.RWMutex
	scanActive    bool
	current       []Result
	frequentApps  []App
	activePopover *gtk.Popover
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
	contentStack := builder.GetObject("content_stack").Cast().(*gtk.Stack)
	listBox := builder.GetObject("list_box").Cast().(*gtk.ListBox)
	gridFlowBox := builder.GetObject("grid_flowbox").Cast().(*gtk.FlowBox)
	frequentBox := builder.GetObject("frequent_section").Cast().(*gtk.Box)

	chipSettings := builder.GetObject("chip_settings").Cast().(*gtk.Button)
	chipTerminal := builder.GetObject("chip_terminal").Cast().(*gtk.Button)
	chipScreenshot := builder.GetObject("chip_screenshot").Cast().(*gtk.Button)
	chipLock := builder.GetObject("chip_lock").Cast().(*gtk.Button)
	chipsBox := builder.GetObject("chips_box").Cast().(*gtk.Box)
	footerHints := builder.GetObject("footer_hints").Cast().(*gtk.Box)

	win.SetChild(overlayBox)

	pageSize := cfg.PageSize
	if pageSize <= 0 {
		pageSize = 6
	}

	frecency := NewFrecencyStoreWithParams(cfg.Frecency.HalfLifeDays, cfg.Frecency.MaxBoost)

	l := &Launcher{
		window:         win,
		overlayBox:     overlayBox,
		card:           card,
		searchEntry:    searchEntry,
		contentStack:   contentStack,
		listBox:        listBox,
		gridFlowBox:    gridFlowBox,
		frequentBox:    frequentBox,
		chipSettings:   chipSettings,
		chipTerminal:   chipTerminal,
		chipScreenshot: chipScreenshot,
		chipLock:       chipLock,
		chipsBox:       chipsBox,
		footerHints:    footerHints,
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
			l.launchResult(l.current[idx], false)
		}
	})

	l.gridFlowBox.ConnectChildActivated(func(child *gtk.FlowBoxChild) {
		idx := child.Index()
		if idx >= 0 && idx < len(l.frequentApps) {
			l.launch(l.frequentApps[idx])
		}
	})

	l.searchEntry.ConnectSearchChanged(func() {
		l.updateFilter()
	})

	l.chipSettings.ConnectClicked(func() {
		l.Close()
		go func() {
			cmd := exec.Command("phalune-settings")
			if err := cmd.Start(); err != nil {
				slog.Warn("launcher: failed to start phalune-settings", "error", err)
			}
		}()
	})

	l.chipTerminal.ConnectClicked(func() {
		l.Close()
		go func() {
			if err := execParts([]string{}, true, l.terminal); err != nil {
				slog.Warn("launcher: failed to start terminal", "error", err)
			}
		}()
	})

	l.chipScreenshot.ConnectClicked(func() {
		l.Close()
		if l.rawCommands != nil && l.rawCommands.Screenshot != nil {
			l.rawCommands.Screenshot()
		} else {
			slog.Warn("launcher: screenshot service unavailable")
		}
	})

	l.chipLock.ConnectClicked(func() {
		l.Close()
		if l.rawCommands != nil && l.rawCommands.Lock != nil {
			l.rawCommands.Lock()
		} else {
			slog.Warn("launcher: lock service unavailable")
		}
	})
}

func (l *Launcher) setupKeyNavigation() {
	keyCtrl := gtk.NewEventControllerKey()
	keyCtrl.SetPropagationPhase(gtk.PhaseCapture)
	keyCtrl.ConnectKeyPressed(func(keyval, keycode uint, state gdk.ModifierType) bool {
		switch keyval {
		case gdk.KEY_Escape:
			if l.activePopover != nil {
				l.closeActivePopover()
				return true
			}
			l.Close()
			return true

		case gdk.KEY_Tab:
			if l.activePopover != nil {
				l.closeActivePopover()
				return true
			}
			row := l.listBox.SelectedRow()
			if row != nil {
				idx := row.Index()
				if idx >= 0 && idx < len(l.current) {
					res := l.current[idx]
					if res.App != nil && len(res.App.Actions) > 0 {
						l.showAppActionsPopover(row, *res.App)
						return true
					}
				}
			}
			return false

		case gdk.KEY_Return, gdk.KEY_KP_Enter:
			inTerminal := state.Has(gdk.ControlMask)
			row := l.listBox.SelectedRow()
			if row != nil {
				idx := row.Index()
				if idx >= 0 && idx < len(l.current) {
					l.launchResult(l.current[idx], inTerminal)
					return true
				}
			}
			if len(l.current) > 0 {
				l.launchResult(l.current[0], inTerminal)
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

	curIdx := -1
	if sel := l.listBox.SelectedRow(); sel != nil {
		curIdx = sel.Index()
	}

	nextIdx := curIdx + delta

	// If at the top row and pressing Up, return selection and focus to search entry
	if delta < 0 && (curIdx <= 0 || nextIdx < 0) {
		l.listBox.UnselectAll()
		l.searchEntry.GrabFocus()
		pos := len(l.searchEntry.Text())
		l.searchEntry.SetPosition(pos)
		return
	}

	if nextIdx < 0 {
		nextIdx = 0
	} else if nextIdx >= n {
		nextIdx = n - 1
	}

	if row := l.listBox.RowAtIndex(nextIdx); row != nil {
		l.listBox.SelectRow(row)
		row.GrabFocus()
	}
}

func (l *Launcher) updateFilter() {
	query := strings.TrimSpace(l.searchEntry.Text())
	apps := l.getApps()

	// Compact mode is a pure command HUD: no canvas, always results view
	if l.currentStyle == "compact" {
		l.contentStack.SetVisibleChildName("results")
		if query == "" {
			// Show top frecency apps directly in the list
			sorted := SortAppsHybrid(apps, l.frecency)
			l.current = make([]Result, len(sorted))
			for i := range sorted {
				l.current[i] = Result{App: &sorted[i]}
			}
		} else {
			l.current = FilterResults(apps, l.commands, query, l.frecency)
		}
		l.renderList()
		return
	}

	if query == "" {
		l.contentStack.SetVisibleChildName("canvas")
		l.renderFrequentGrid()
		return
	}

	l.contentStack.SetVisibleChildName("results")
	l.current = FilterResults(l.allApps, l.commands, query, l.frecency)
	l.renderList()
}

func (l *Launcher) renderFrequentGrid() {
	for {
		child := l.gridFlowBox.ChildAtIndex(0)
		if child == nil {
			break
		}
		l.gridFlowBox.Remove(child)
	}

	limit := 8
	iconPixelSize := 36
	if l.currentStyle == "fullscreen" {
		limit = 18
		iconPixelSize = 48
		l.gridFlowBox.SetMinChildrenPerLine(6)
		l.gridFlowBox.SetMaxChildrenPerLine(6)
	} else {
		l.gridFlowBox.SetMinChildrenPerLine(4)
		l.gridFlowBox.SetMaxChildrenPerLine(4)
	}

	apps := l.getApps()
	sorted := SortAppsHybrid(apps, l.frecency)
	if len(sorted) < limit {
		limit = len(sorted)
	}
	l.frequentApps = make([]App, limit)
	copy(l.frequentApps, sorted[:limit])

	if l.frequentBox != nil {
		l.frequentBox.SetVisible(len(l.frequentApps) > 0)
	}

	for _, app := range l.frequentApps {
		itemBox := gtk.NewBox(gtk.OrientationVertical, 6)
		itemBox.AddCSSClass("launcher-grid-item")
		itemBox.SetHAlign(gtk.AlignCenter)
		itemBox.SetVAlign(gtk.AlignCenter)

		iconName := app.Icon
		if iconName == "" {
			iconName = "application-x-executable"
		}
		img := gtk.NewImageFromIconName(iconName)
		img.SetPixelSize(iconPixelSize)
		img.AddCSSClass("launcher-grid-icon")
		itemBox.Append(img)

		lbl := gtk.NewLabel(app.Name)
		lbl.AddCSSClass("launcher-grid-label")
		lbl.SetEllipsize(pango.EllipsizeEnd)
		lbl.SetMaxWidthChars(14)
		lbl.SetHAlign(gtk.AlignCenter)
		itemBox.Append(lbl)

		if len(app.Actions) > 0 {
			targetApp := app
			boxWidget := itemBox
			rightClick := gtk.NewGestureClick()
			rightClick.SetButton(3)
			rightClick.ConnectReleased(func(n int, x, y float64) {
				l.showAppActionsPopover(boxWidget, targetApp)
			})
			itemBox.AddController(rightClick)
		}

		l.gridFlowBox.Append(itemBox)
	}
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
		l.contentStack.SetVisibleChildName("empty")
		return
	}

	l.contentStack.SetVisibleChildName("results")

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
		} else if res.Calculation != "" {
			row.AddCSSClass("launcher-row-calc")
		} else if res.ShellCmd != "" {
			row.AddCSSClass("launcher-row-shell")
		}

		if res.App != nil && len(res.App.Actions) > 0 {
			targetApp := *res.App
			rowWidget := row
			rightClick := gtk.NewGestureClick()
			rightClick.SetButton(3)
			rightClick.ConnectReleased(func(n int, x, y float64) {
				l.showAppActionsPopover(rowWidget, targetApp)
			})
			row.AddController(rightClick)
		}

		l.listBox.Append(row)
	}

	if first := l.listBox.RowAtIndex(0); first != nil {
		l.listBox.SelectRow(first)
	}
}

func (l *Launcher) closeActivePopover() {
	if l.activePopover != nil {
		l.activePopover.Popdown()
		l.activePopover.Unparent()
		l.activePopover = nil
	}
}

func (l *Launcher) showAppActionsPopover(parent gtk.Widgetter, app App) {
	l.closeActivePopover()

	if len(app.Actions) == 0 {
		return
	}

	pop := gtk.NewPopover()
	pop.SetParent(parent)
	pop.SetPosition(gtk.PosBottom)
	pop.SetHasArrow(true)
	pop.AddCSSClass("launcher-actions-popover")

	vbox := gtk.NewBox(gtk.OrientationVertical, 2)
	vbox.AddCSSClass("launcher-actions-box")

	header := gtk.NewLabel(app.Name + " Actions")
	header.AddCSSClass("launcher-actions-header")
	header.SetHAlign(gtk.AlignStart)
	vbox.Append(header)

	for _, action := range app.Actions {
		act := action
		btn := gtk.NewButton()
		btn.AddCSSClass("launcher-action-btn")

		btnBox := gtk.NewBox(gtk.OrientationHorizontal, 8)
		iconName := act.Icon
		if iconName == "" {
			iconName = app.Icon
		}
		if iconName == "" {
			iconName = "application-x-executable-symbolic"
		}
		img := gtk.NewImageFromIconName(iconName)
		img.SetPixelSize(16)
		btnBox.Append(img)

		lbl := gtk.NewLabel(act.Name)
		lbl.AddCSSClass("launcher-action-label")
		lbl.SetHAlign(gtk.AlignStart)
		btnBox.Append(lbl)

		btn.SetChild(btnBox)
		btn.ConnectClicked(func() {
			l.closeActivePopover()
			l.Close()
			l.frecency.RecordLaunch(app.ID)
			go func() {
				if err := LaunchDesktopAction(app, act, l.terminal); err != nil {
					slog.Error("failed to launch desktop action", "app", app.Name, "action", act.Name, "error", err)
				}
			}()
		})

		vbox.Append(btn)
	}

	pop.SetChild(vbox)
	pop.ConnectClosed(func() {
		l.closeActivePopover()
	})

	l.activePopover = pop
	pop.Popup()
}

func (l *Launcher) launch(app App) {
	l.closeActivePopover()
	l.frecency.RecordLaunch(app.ID)
	l.Close()

	go func() {
		if err := LaunchApp(app, l.terminal); err != nil {
			slog.Error("failed to launch application", "app", app.Name, "error", err)
		}
	}()
}

func (l *Launcher) launchResult(res Result, forceTerminal bool) {
	l.closeActivePopover()
	switch {
	case res.Calculation != "":
		display := gdk.DisplayGetDefault()
		if display != nil {
			clip := display.Clipboard()
			if clip != nil {
				clip.SetText(res.Calculation)
			}
		}
		l.Close()

	case res.ShellCmd != "":
		l.Close()
		go func() {
			parts, err := splitCommandLine(res.ShellCmd)
			if err != nil || len(parts) == 0 {
				parts = []string{"sh", "-c", res.ShellCmd}
			}
			if err := execParts(parts, true, l.terminal); err != nil {
				slog.Warn("launcher: shell command failed", "cmd", res.ShellCmd, "error", err)
			}
		}()

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
			termPref := l.terminal
			if forceTerminal {
				action.Terminal = new(bool)
				*action.Terminal = true
			}
			if err := LaunchDesktopAction(app, action, termPref); err != nil {
				slog.Error("failed to launch desktop action", "app", app.Name, "action", action.Name, "error", err)
			}
		}()

	default:
		if res.App != nil {
			if forceTerminal {
				app := *res.App
				app.Terminal = true
				l.launch(app)
			} else {
				l.launch(*res.App)
			}
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
	if len(l.getApps()) == 0 {
		// First open scans synchronously; later opens use the cache.
		if apps, err := ScanApplications(); err == nil {
			l.setApps(apps)
		}
	} else {
		go l.rescan()
	}

	l.searchEntry.SetText("")
	l.updateFilter()

	l.window.Present()
	l.searchEntry.GrabFocus()
}

// getApps returns a snapshot of the cached application list.
func (l *Launcher) getApps() []App {
	l.appsMu.RLock()
	defer l.appsMu.RUnlock()
	return append([]App(nil), l.allApps...)
}

func (l *Launcher) setApps(apps []App) {
	l.appsMu.Lock()
	l.allApps = apps
	l.appsMu.Unlock()
}

// rescan refreshes the app cache in the background.
func (l *Launcher) rescan() {
	l.appsMu.Lock()
	if l.scanActive {
		l.appsMu.Unlock()
		return
	}
	l.scanActive = true
	l.appsMu.Unlock()
	defer func() {
		l.appsMu.Lock()
		l.scanActive = false
		l.appsMu.Unlock()
	}()

	apps, err := ScanApplications()
	if err != nil {
		return
	}
	l.setApps(apps)
	glib.IdleAdd(func() {
		if l.window.IsVisible() {
			l.updateFilter()
		}
	})
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
	l.currentStyle = styleName
	for _, s := range []string{"launcher-style-centered", "launcher-style-fullscreen", "launcher-style-compact", "launcher-style-default"} {
		l.card.RemoveCSSClass(s)
	}
	l.card.AddCSSClass(styleClass)

	switch styleName {
	case "fullscreen":
		if l.overlayBox != nil {
			l.overlayBox.AddCSSClass("launcher-overlay-fullscreen")
		}
		if l.chipsBox != nil {
			l.chipsBox.SetVisible(true)
		}
		if l.footerHints != nil {
			l.footerHints.SetVisible(true)
		}
		l.card.SetSizeRequest(-1, -1)
		l.card.SetHAlign(gtk.AlignFill)
		l.card.SetVAlign(gtk.AlignFill)
		l.card.SetHExpand(true)
		l.card.SetVExpand(true)
		l.card.SetMarginTop(48)
		l.card.SetMarginBottom(48)
		l.card.SetMarginStart(80)
		l.card.SetMarginEnd(80)
	case "compact":
		if l.overlayBox != nil {
			l.overlayBox.RemoveCSSClass("launcher-overlay-fullscreen")
		}
		// Compact mode: pure fast command bar
		if l.chipsBox != nil {
			l.chipsBox.SetVisible(false)
		}
		if l.footerHints != nil {
			l.footerHints.SetVisible(false)
		}
		l.card.SetHAlign(gtk.AlignCenter)
		l.card.SetVAlign(gtk.AlignCenter)
		l.card.SetHExpand(false)
		l.card.SetVExpand(false)
		l.card.SetSizeRequest(420, 360)
		l.card.SetMarginTop(0)
		l.card.SetMarginBottom(0)
		l.card.SetMarginStart(0)
		l.card.SetMarginEnd(0)
	default: // "centered"
		if l.overlayBox != nil {
			l.overlayBox.RemoveCSSClass("launcher-overlay-fullscreen")
		}
		if l.chipsBox != nil {
			l.chipsBox.SetVisible(true)
		}
		if l.footerHints != nil {
			l.footerHints.SetVisible(true)
		}
		l.card.SetHAlign(gtk.AlignCenter)
		l.card.SetVAlign(gtk.AlignCenter)
		l.card.SetHExpand(false)
		l.card.SetVExpand(false)
		l.card.SetSizeRequest(560, 520)
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
	l.rawCommands = cmds
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
