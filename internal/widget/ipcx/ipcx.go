// Package ipcx implements bar widgets driven by external programs through
// the Phalune IPC socket: an external daemon claims a widget id ("widget
// watch" on the socket) and pushes state; user interactions travel back
// over the same connection. This file contains the bar-side renderer.
package ipcx

import (
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"phalune/internal/config"
	"phalune/internal/ipc"
	"phalune/internal/widget"
	"phalune/ui"

	"github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// Widget renders the bar pill for one "ipc:<id>" layout entry. State
// arrives through the shared WidgetHub, which IPC daemons update over
// the socket.
type Widget struct {
	box      *gtk.Box
	icon     *gtk.Image
	progress *gtk.LevelBar
	label    *gtk.Label
	popover  *gtk.Popover

	id  string
	hub *ipc.WidgetHub
	cfg config.IPCWidgetConfig

	mu           sync.Mutex
	onDestroy    func()
	currentClass string
}

// New builds the pill and subscribes to hub updates for its id.
func New(ctx widget.Context) (widget.Widget, error) {
	if ctx.WidgetHub == nil {
		return nil, fmt.Errorf("ipc widget %q requires the widget hub", ctx.WidgetName)
	}

	builder := gtk.NewBuilderFromString(ui.IPCPill)
	box := builder.GetObject("ipc_pill").Cast().(*gtk.Box)
	icon := builder.GetObject("ipc_icon").Cast().(*gtk.Image)
	progress := builder.GetObject("ipc_progress").Cast().(*gtk.LevelBar)
	label := builder.GetObject("ipc_label").Cast().(*gtk.Label)

	box.AddCSSClass("widget-ipc-" + ctx.WidgetName)

	var cfg config.IPCWidgetConfig
	if ctx.Config != nil && ctx.Config.Bar.IPC != nil {
		cfg = ctx.Config.Bar.IPC[ctx.WidgetName]
	}

	w := &Widget{
		box:      box,
		icon:     icon,
		progress: progress,
		label:    label,
		id:       ctx.WidgetName,
		hub:      ctx.WidgetHub,
		cfg:      cfg,
	}
	w.setInteractions()

	if snapshot, ok := w.hub.State(w.id); ok {
		w.render(snapshot)
	} else {
		box.SetVisible(false)
	}

	unsubscribe := w.hub.OnChange(w.id, func() {
		st, ok := w.hub.State(w.id)
		if !ok {
			return
		}
		glib.IdleAdd(func() { w.render(st) })
	})
	w.onDestroy = unsubscribe

	return w, nil
}

func (w *Widget) Root() gtk.Widgetter { return w.box }

func (w *Widget) Destroy() {
	w.mu.Lock()
	unsubscribe := w.onDestroy
	w.onDestroy = nil
	w.mu.Unlock()

	if unsubscribe != nil {
		unsubscribe()
	}
}

// render applies one pushed state. Main thread only.
func (w *Widget) render(st ipc.WidgetState) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.box == nil {
		return
	}

	if !st.Visible {
		if w.popover != nil {
			w.popover.Popdown()
		}
		w.box.SetVisible(false)
		return
	}

	w.label.SetMarkup(st.Text)

	iconName := st.Icon
	if iconName != "" {
		w.icon.SetFromIconName(iconName)
	}
	w.icon.SetVisible(iconName != "")

	if st.Percent != nil {
		w.progress.SetValue(*st.Percent)
	}
	w.progress.SetVisible(st.Percent != nil)

	w.syncPopover(st.Popover)
	w.setDynamicClass(st.Class)

	tooltip := st.Tooltip
	if tooltip == "" {
		tooltip = w.cfg.Tooltip
	}
	w.box.SetTooltipText(tooltip)
	w.box.SetVisible(true)
}

// syncPopover rebuilds the flyout when the daemon declares or changes
// one. nil removes it; clicks then surface as widget_clicked events.
func (w *Widget) syncPopover(spec *ipc.PopoverSpec) {
	if spec == nil {
		if w.popover != nil {
			w.popover.Popdown()
			w.popover.Unparent()
			w.popover = nil
		}
		return
	}

	if w.popover == nil {
		w.popover = gtk.NewPopover()
		w.popover.SetParent(w.box)
		w.popover.SetPosition(gtk.PosBottom)
		w.popover.AddCSSClass("ipc-popover")
	}

	child := buildPopoverContent(spec, w.id, w.popover, w.dispatch)
	w.popover.SetChild(child)
}

// buildPopoverContent constructs the declarative popover tree.
func buildPopoverContent(spec *ipc.PopoverSpec, widgetID string, popover *gtk.Popover, dispatch func(*ipc.WidgetEvent)) *gtk.Box {
	content := gtk.NewBox(gtk.OrientationVertical, 6)
	content.AddCSSClass("ipc-popover-content")

	appendLabel := func(text string, class string) {
		lbl := gtk.NewLabel(text)
		lbl.SetMarkup(text)
		lbl.SetHAlign(gtk.AlignStart)
		if class != "" {
			lbl.AddCSSClass(class)
		}
		content.Append(lbl)
	}

	if spec.Title != "" {
		appendLabel(spec.Title, "ipc-popover-title")
	}

	for _, row := range spec.Rows {
		switch row.Type {
		case ipc.PopoverRowProgress:
			if row.Label != "" {
				appendLabel(row.Label, "ipc-popover-label")
			}
			progress := gtk.NewProgressBar()
			progress.SetFraction(clampFraction(row.Value))
			progress.AddCSSClass("ipc-popover-progress")
			content.Append(progress)

		case ipc.PopoverRowButtonRow:
			btnRow := gtk.NewBox(gtk.OrientationHorizontal, 6)
			btnRow.AddCSSClass("ipc-popover-buttons")
			for _, btn := range row.Buttons {
				if btn.ID == "" {
					continue
				}
				actionID := btn.ID
				button := gtk.NewButton()
				button.SetLabel(btn.Label)
				button.ConnectClicked(func() {
					if popover != nil {
						popover.Popdown()
					}
					dispatch(&ipc.WidgetEvent{Event: "popover_action", ID: widgetID, Action: actionID})
				})
				if btn.Class != "" {
					button.AddCSSClass(btn.Class)
				}
				btnRow.Append(button)
			}
			content.Append(btnRow)

		case ipc.PopoverRowList:
			list := gtk.NewLabel("")
			list.SetMarkup(strings.Join(wrapListItems(row.Items), "\n"))
			list.AddCSSClass("ipc-popover-list")
			list.SetHAlign(gtk.AlignStart)
			content.Append(list)

		default:
			if row.Label != "" {
				appendLabel(row.Label, "ipc-popover-label")
			}
		}
	}
	return content
}

// wrapListItems renders popover list items as bullet lines.
func wrapListItems(items []string) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, "• "+it)
	}
	return out
}

func clampFraction(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// setDynamicClass swaps pushed CSS classes; space-separated tokens apply
// individually since GTK rejects class names containing spaces.
func (w *Widget) setDynamicClass(class string) {
	if class == w.currentClass {
		return
	}
	for _, c := range strings.Fields(w.currentClass) {
		w.box.RemoveCSSClass(c)
	}
	for _, c := range strings.Fields(class) {
		w.box.AddCSSClass(c)
	}
	w.currentClass = class
}

// setInteractions handles pill clicks: opens the popover if declared,
// or forwards the click event back to the widget owner.
func (w *Widget) setInteractions() {
	click := gtk.NewGestureClick()
	click.SetButton(0)
	click.ConnectReleased(func(int, float64, float64) {
		w.mu.Lock()
		pop := w.popover
		w.mu.Unlock()
		if pop != nil {
			pop.Popup()
			return
		}
		w.dispatch(&ipc.WidgetEvent{
			Event:  "widget_clicked",
			ID:     w.id,
			Button: click.CurrentButton(),
		})
	})
	w.box.AddController(click)
}

func (w *Widget) dispatch(ev *ipc.WidgetEvent) {
	w.mu.Lock()
	hub := w.hub
	w.mu.Unlock()

	if hub == nil {
		return
	}
	if !hub.DispatchClick(ev) {
		slog.Debug("ipc: widget click dropped, no owner", "widget", w.id)
	}
}
