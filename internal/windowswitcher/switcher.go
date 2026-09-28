package windowswitcher

import (
	"log/slog"
	"sort"
	"strings"
	"sync"

	"phalune/internal/config"
	"phalune/internal/niri"
	"phalune/ui"

	"github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

type WindowSwitcher struct {
	window             *gtk.Window
	overlayBox         *gtk.Overlay
	card               *gtk.Box
	selectedTitleLabel *gtk.Label
	scrolled           *gtk.ScrolledWindow
	cardsBox           *gtk.Box
	emptyLabel         *gtk.Label

	niriSvc     *niri.Service
	cfg         config.WindowSwitcherConfig
	windows     []niri.Window
	cardButtons []*gtk.Button
	selectedIdx int
	isOpen      bool
	mu          sync.Mutex
}

func New(app *gtk.Application, cfg config.WindowSwitcherConfig, niriSvc *niri.Service) (*WindowSwitcher, error) {
	win := gtk.NewWindow()
	win.SetApplication(app)
	win.SetTitle("phalune-window-switcher")
	win.SetDecorated(false)
	win.AddCSSClass("window-switcher-window")

	if err := ConfigureSurface(win); err != nil {
		slog.Warn("windowswitcher: layer-surface configuration warning", "error", err)
	}

	builder := gtk.NewBuilderFromString(ui.WindowSwitcher)
	overlayBox := builder.GetObject("overlay_box").Cast().(*gtk.Overlay)
	card := builder.GetObject("card").Cast().(*gtk.Box)
	selectedTitleLabel := builder.GetObject("selected_title_label").Cast().(*gtk.Label)
	scrolled := builder.GetObject("scrolled_window").Cast().(*gtk.ScrolledWindow)
	cardsBox := builder.GetObject("cards_container").Cast().(*gtk.Box)
	emptyLabel := builder.GetObject("empty_label").Cast().(*gtk.Label)

	win.SetChild(overlayBox)

	ws := &WindowSwitcher{
		window:             win,
		overlayBox:         overlayBox,
		card:               card,
		selectedTitleLabel: selectedTitleLabel,
		scrolled:           scrolled,
		cardsBox:           cardsBox,
		emptyLabel:         emptyLabel,
		niriSvc:            niriSvc,
		cfg:                cfg,
	}

	ws.setupInteractivity()
	ws.setupKeyNavigation()

	return ws, nil
}

func (ws *WindowSwitcher) setupInteractivity() {
	click := gtk.NewGestureClick()
	click.ConnectReleased(func(n int, x, y float64) {
		pick := ws.overlayBox.Pick(x, y, gtk.PickDefault)
		if pick != nil {
			w := gtk.BaseWidget(pick)
			if w != nil && (w == &ws.card.Widget || w.IsAncestor(ws.card)) {
				return
			}
		}
		ws.Close()
	})
	ws.overlayBox.AddController(click)
}

func (ws *WindowSwitcher) setupKeyNavigation() {
	keyCtrl := gtk.NewEventControllerKey()
	keyCtrl.ConnectKeyPressed(func(keyval, keycode uint, state gdk.ModifierType) bool {
		switch keyval {
		case gdk.KEY_Escape:
			ws.Close()
			return true

		case gdk.KEY_Return, gdk.KEY_KP_Enter, gdk.KEY_space:
			ws.ActivateSelected()
			return true

		case gdk.KEY_Tab:
			if state.Has(gdk.ShiftMask) {
				ws.Prev()
			} else {
				ws.Next()
			}
			return true

		case gdk.KEY_ISO_Left_Tab:
			ws.Prev()
			return true

		case gdk.KEY_Right, gdk.KEY_Down, gdk.KEY_l, gdk.KEY_j:
			ws.Next()
			return true

		case gdk.KEY_Left, gdk.KEY_Up, gdk.KEY_h, gdk.KEY_k:
			ws.Prev()
			return true

		case gdk.KEY_1, gdk.KEY_2, gdk.KEY_3, gdk.KEY_4, gdk.KEY_5, gdk.KEY_6, gdk.KEY_7, gdk.KEY_8, gdk.KEY_9:
			idx := int(keyval - gdk.KEY_1)
			ws.ActivateIndex(idx)
			return true

		case gdk.KEY_KP_1, gdk.KEY_KP_2, gdk.KEY_KP_3, gdk.KEY_KP_4, gdk.KEY_KP_5, gdk.KEY_KP_6, gdk.KEY_KP_7, gdk.KEY_KP_8, gdk.KEY_KP_9:
			idx := int(keyval - gdk.KEY_KP_1)
			ws.ActivateIndex(idx)
			return true

		case gdk.KEY_q:
			ws.Close()
			return true
		}

		return false
	})

	ws.window.AddController(keyCtrl)
}

func (ws *WindowSwitcher) IsOpen() bool {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	return ws.isOpen
}

func (ws *WindowSwitcher) Toggle() {
	ws.mu.Lock()
	open := ws.isOpen
	ws.mu.Unlock()

	if open {
		ws.Close()
	} else {
		ws.Open()
	}
}

func (ws *WindowSwitcher) Open() {
	glib.IdleAdd(func() {
		ws.mu.Lock()
		defer ws.mu.Unlock()
		slog.Debug("windowswitcher: opening")
		ws.openLocked(false)
	})
}

func (ws *WindowSwitcher) openLocked(reverse bool) {
	var wins []niri.Window
	if ws.niriSvc != nil {
		all, err := ws.niriSvc.QueryWindows()
		if err != nil {
			slog.Warn("windowswitcher: query windows failed", "error", err)
		} else {
			var focusedWsID uint64
			for _, w := range ws.niriSvc.Workspaces() {
				if w.IsFocused {
					focusedWsID = w.ID
					break
				}
			}
			if focusedWsID == 0 {
				for _, w := range ws.niriSvc.Workspaces() {
					if w.IsActive {
						focusedWsID = w.ID
						break
					}
				}
			}

			if !ws.cfg.AllWorkspaces && focusedWsID != 0 {
				for _, win := range all {
					if win.WorkspaceID == focusedWsID {
						wins = append(wins, win)
					}
				}
			} else {
				wins = all
			}
		}
	}

	// Sort windows by actual position in Niri ribbon: workspace -> column -> row -> floating
	sort.Slice(wins, func(i, j int) bool {
		wi, wj := wins[i], wins[j]
		if wi.WorkspaceID != wj.WorkspaceID {
			return wi.WorkspaceID < wj.WorkspaceID
		}
		if wi.IsFloating != wj.IsFloating {
			return !wi.IsFloating
		}
		var colI, rowI int
		if wi.Layout != nil && len(wi.Layout.PosInScrollingLayout) >= 2 {
			colI = wi.Layout.PosInScrollingLayout[0]
			rowI = wi.Layout.PosInScrollingLayout[1]
		}
		var colJ, rowJ int
		if wj.Layout != nil && len(wj.Layout.PosInScrollingLayout) >= 2 {
			colJ = wj.Layout.PosInScrollingLayout[0]
			rowJ = wj.Layout.PosInScrollingLayout[1]
		}
		if colI != colJ {
			return colI < colJ
		}
		if rowI != rowJ {
			return rowI < rowJ
		}
		return wi.ID < wj.ID
	})

	ws.windows = wins
	ws.rebuildCards()

	if len(wins) == 0 {
		ws.selectedIdx = -1
		ws.emptyLabel.SetVisible(true)
		ws.scrolled.SetVisible(false)
		ws.selectedTitleLabel.SetVisible(false)
	} else {
		ws.emptyLabel.SetVisible(false)
		ws.scrolled.SetVisible(true)
		ws.selectedTitleLabel.SetVisible(true)

		initialIdx := 0
		for i, w := range wins {
			if w.IsFocused {
				if len(wins) > 1 {
					if reverse {
						initialIdx = (i - 1 + len(wins)) % len(wins)
					} else {
						initialIdx = (i + 1) % len(wins)
					}
				} else {
					initialIdx = i
				}
				break
			}
		}
		ws.selectedIdx = initialIdx
	}

	ws.isOpen = true
	ws.overlayBox.AddCSSClass("open")
	ws.card.AddCSSClass("open")
	ws.window.SetVisible(true)
	ws.window.Present()
	if len(wins) > 0 {
		ws.updateSelection()
	}
	slog.Debug("windowswitcher: presented", "window_count", len(wins), "selected_idx", ws.selectedIdx)
}

func (ws *WindowSwitcher) Close() {
	glib.IdleAdd(func() {
		ws.mu.Lock()
		defer ws.mu.Unlock()

		slog.Debug("windowswitcher: closing")
		ws.isOpen = false
		ws.overlayBox.RemoveCSSClass("open")
		ws.card.RemoveCSSClass("open")
		ws.window.SetVisible(false)
	})
}

func (ws *WindowSwitcher) Next() {
	glib.IdleAdd(func() {
		ws.mu.Lock()
		defer ws.mu.Unlock()

		if !ws.isOpen {
			ws.openLocked(false)
			return
		}
		if len(ws.windows) == 0 {
			return
		}
		ws.selectedIdx = (ws.selectedIdx + 1) % len(ws.windows)
		ws.updateSelection()
	})
}

func (ws *WindowSwitcher) Prev() {
	glib.IdleAdd(func() {
		ws.mu.Lock()
		defer ws.mu.Unlock()

		if !ws.isOpen {
			ws.openLocked(true)
			return
		}
		if len(ws.windows) == 0 {
			return
		}
		ws.selectedIdx = (ws.selectedIdx - 1 + len(ws.windows)) % len(ws.windows)
		ws.updateSelection()
	})
}

func (ws *WindowSwitcher) SelectIndex(idx int) {
	glib.IdleAdd(func() {
		ws.mu.Lock()
		defer ws.mu.Unlock()

		if idx < 0 || idx >= len(ws.windows) {
			return
		}
		ws.selectedIdx = idx
		ws.updateSelection()
	})
}

func (ws *WindowSwitcher) ActivateIndex(idx int) {
	ws.SelectIndex(idx)
	ws.ActivateSelected()
}

func (ws *WindowSwitcher) ActivateSelected() {
	glib.IdleAdd(func() {
		ws.mu.Lock()
		if !ws.isOpen {
			ws.mu.Unlock()
			return
		}

		targetID := uint64(0)
		hasTarget := false

		if ws.selectedIdx >= 0 && ws.selectedIdx < len(ws.windows) {
			targetID = ws.windows[ws.selectedIdx].ID
			hasTarget = true
		}
		ws.isOpen = false
		ws.overlayBox.RemoveCSSClass("open")
		ws.card.RemoveCSSClass("open")
		ws.window.SetVisible(false)
		ws.mu.Unlock()

		if hasTarget && ws.niriSvc != nil {
			go func(id uint64) {
				_ = ws.niriSvc.FocusWindow(id)
			}(targetID)
		}
	})
}

func (ws *WindowSwitcher) Destroy() {
	glib.IdleAdd(func() {
		ws.mu.Lock()
		defer ws.mu.Unlock()

		ws.isOpen = false
		if ws.window != nil {
			if ws.window.Realized() {
				ws.window.Destroy()
			}
			ws.window = nil
		}
	})
}

func (ws *WindowSwitcher) UpdateConfig(cfg config.WindowSwitcherConfig) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.cfg = cfg
}

func (ws *WindowSwitcher) rebuildCards() {
	for child := ws.cardsBox.FirstChild(); child != nil; child = ws.cardsBox.FirstChild() {
		ws.cardsBox.Remove(child)
	}
	ws.cardButtons = nil

	for i, win := range ws.windows {
		idx := i
		targetWin := win

		b := gtk.NewBuilderFromString(ui.WindowSwitcherCard)
		btn := b.GetObject("card_button").Cast().(*gtk.Button)
		appIcon := b.GetObject("app_icon").Cast().(*gtk.Image)
		appName := b.GetObject("app_name").Cast().(*gtk.Label)

		iconName := ResolveAppIcon(targetWin.AppID)
		appIcon.SetFromIconName(iconName)

		dispName := ResolveAppName(targetWin.AppID, targetWin.Title)
		appName.SetText(dispName)

		if targetWin.IsFocused {
			btn.AddCSSClass("currently-focused")
		}
		if targetWin.IsUrgent {
			btn.AddCSSClass("urgent")
		}

		btn.ConnectClicked(func() {
			ws.ActivateIndex(idx)
		})

		motion := gtk.NewEventControllerMotion()
		motion.ConnectEnter(func(x, y float64) {
			ws.SelectIndex(idx)
		})
		btn.AddController(motion)

		ws.cardsBox.Append(btn)
		ws.cardButtons = append(ws.cardButtons, btn)
	}
}

func (ws *WindowSwitcher) updateSelection() {
	for i, btn := range ws.cardButtons {
		if i == ws.selectedIdx {
			btn.AddCSSClass("selected")
			btn.GrabFocus()
		} else {
			btn.RemoveCSSClass("selected")
		}
	}
	if ws.selectedIdx >= 0 && ws.selectedIdx < len(ws.windows) {
		title := ws.windows[ws.selectedIdx].Title
		if title == "" {
			title = ResolveAppName(ws.windows[ws.selectedIdx].AppID, "")
		}
		ws.selectedTitleLabel.SetText(title)
		ws.selectedTitleLabel.SetVisible(true)
	} else {
		ws.selectedTitleLabel.SetVisible(false)
	}
}

func ResolveAppIcon(appID string) string {
	clean := strings.TrimSpace(appID)
	if clean == "" {
		return "application-x-executable-symbolic"
	}

	display := gdk.DisplayGetDefault()
	if display != nil {
		theme := gtk.IconThemeGetForDisplay(display)
		if theme != nil {
			if theme.HasIcon(clean) {
				return clean
			}
			lower := strings.ToLower(clean)
			if theme.HasIcon(lower) {
				return lower
			}

			parts := strings.Split(clean, ".")
			if len(parts) > 1 {
				for j := len(parts) - 1; j >= 0; j-- {
					p := strings.ToLower(parts[j])
					if p != "desktop" && p != "exe" && theme.HasIcon(p) {
						return p
					}
				}
			}
		}
	}

	return "application-x-executable-symbolic"
}

func ResolveAppName(appID, title string) string {
	clean := strings.TrimSpace(appID)
	if clean != "" {
		parts := strings.Split(clean, ".")
		candidate := parts[len(parts)-1]
		if strings.EqualFold(candidate, "desktop") && len(parts) > 1 {
			candidate = parts[len(parts)-2]
		}
		if len(candidate) > 0 {
			return strings.ToUpper(candidate[:1]) + candidate[1:]
		}
	}

	if title != "" {
		if len(title) > 28 {
			return title[:28] + "…"
		}
		return title
	}

	return "Window"
}
