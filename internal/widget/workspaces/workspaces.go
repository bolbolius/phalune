package workspaces

import (
	"context"
	"fmt"
	"sort"

	"phalune/internal/niri"
	"phalune/internal/widget"
	"phalune/ui"

	"github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

type Workspaces struct {
	box         *gtk.Box
	niriSvc     *niri.Service
	output      string
	allOutputs  bool
	buttons     map[uint64]*gtk.Button
	order       []uint64
	unsubscribe func()
	cancel      context.CancelFunc
}

func New(ctx widget.Context) (widget.Widget, error) {
	if ctx.Niri == nil {
		return nil, fmt.Errorf("niri service is not available")
	}

	builder := gtk.NewBuilderFromString(ui.Workspaces)
	box := builder.GetObject("workspaces_container").Cast().(*gtk.Box)

	allOutputs := false
	if ctx.Config != nil {
		allOutputs = ctx.Config.Bar.Workspaces.AllOutputs
	}

	wCtx, cancel := context.WithCancel(context.Background())
	w := &Workspaces{
		box:        box,
		niriSvc:    ctx.Niri,
		output:     ctx.Output,
		allOutputs: allOutputs,
		buttons:    make(map[uint64]*gtk.Button),
		cancel:     cancel,
	}

	ch, unsub := ctx.Niri.Subscribe()
	w.unsubscribe = unsub

	go func() {
		for {
			select {
			case <-wCtx.Done():
				return
			case list, ok := <-ch:
				if !ok {
					return
				}
				workspacesCopy := list
				glib.IdleAdd(func() {
					w.updateWorkspaces(workspacesCopy)
				})
			}
		}
	}()

	return w, nil
}

func (w *Workspaces) updateWorkspaces(allWorkspaces []niri.Workspace) {
	var list []niri.Workspace
	for _, ws := range allWorkspaces {
		if !w.allOutputs && w.output != "" && ws.Output != nil && *ws.Output != w.output {
			continue
		}
		list = append(list, ws)
	}

	sort.Slice(list, func(i, j int) bool {
		if list[i].Idx != list[j].Idx {
			return list[i].Idx < list[j].Idx
		}
		return list[i].ID < list[j].ID
	})

	currentIDs := make(map[uint64]bool, len(list))
	for _, ws := range list {
		currentIDs[ws.ID] = true
	}

	for id, btn := range w.buttons {
		if !currentIDs[id] {
			w.box.Remove(btn)
			delete(w.buttons, id)
		}
	}

	orderChanged := len(w.order) != len(list)
	if !orderChanged {
		for i, ws := range list {
			if w.order[i] != ws.ID {
				orderChanged = true
				break
			}
		}
	}

	newOrder := make([]uint64, len(list))
	for i, ws := range list {
		newOrder[i] = ws.ID
		wsID := ws.ID
		displayName := ws.DisplayName()

		btn, exists := w.buttons[wsID]
		if !exists {
			btnBuilder := gtk.NewBuilderFromString(ui.WorkspaceButton)
			btn = btnBuilder.GetObject("workspace_button").Cast().(*gtk.Button)
			btn.SetLabel(displayName)
			btn.ConnectClicked(func() {
				_ = w.niriSvc.FocusWorkspace(wsID)
			})
			w.buttons[wsID] = btn
		} else {
			if btn.Label() != displayName {
				btn.SetLabel(displayName)
			}
		}

		if ws.IsFocused {
			btn.AddCSSClass("focused")
		} else {
			btn.RemoveCSSClass("focused")
		}

		if ws.IsActive {
			btn.AddCSSClass("active")
		} else {
			btn.RemoveCSSClass("active")
		}

		if ws.IsUrgent {
			btn.AddCSSClass("urgent")
		} else {
			btn.RemoveCSSClass("urgent")
		}
	}

	if orderChanged {
		for _, id := range w.order {
			if btn, ok := w.buttons[id]; ok {
				w.box.Remove(btn)
			}
		}
		for _, id := range newOrder {
			if btn, ok := w.buttons[id]; ok {
				w.box.Append(btn)
			}
		}
		w.order = newOrder
	}
}

func (w *Workspaces) Root() gtk.Widgetter {
	return w.box
}

func (w *Workspaces) Destroy() {
	if w.cancel != nil {
		w.cancel()
	}
	if w.unsubscribe != nil {
		w.unsubscribe()
	}
}
