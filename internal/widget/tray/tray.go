package tray

import (
	"log/slog"
	"sync"

	"phalune/internal/tray"
	"phalune/internal/widget"
	"phalune/ui"

	"github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	glibv2 "github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

type Tray struct {
	box      *gtk.Box
	mgr      *tray.Manager
	items    map[string]*itemWidget
	iconSize int
	mu       sync.Mutex
}

type itemWidget struct {
	button  *gtk.Button
	icon    *gtk.Image
	popover *gtk.Popover
	item    *tray.TrayItem
}

func New(ctx widget.Context) (widget.Widget, error) {
	builder := gtk.NewBuilderFromString(ui.Tray)
	box := builder.GetObject("tray_box").Cast().(*gtk.Box)

	mgr, err := tray.GetManager()
	if err != nil {
		return nil, err
	}

	iconSize := 16
	if ctx.Config != nil && ctx.Config.Bar.Tray.IconSize > 0 {
		iconSize = ctx.Config.Bar.Tray.IconSize
	}

	t := &Tray{
		box:      box,
		mgr:      mgr,
		items:    make(map[string]*itemWidget),
		iconSize: iconSize,
	}

	mgr.AddListener(t)

	return t, nil
}

func (t *Tray) Root() gtk.Widgetter {
	return t.box
}

func (t *Tray) Destroy() {
	t.mgr.RemoveListener(t)

	t.mu.Lock()
	defer t.mu.Unlock()
	for _, w := range t.items {
		if w.popover != nil {
			w.popover.Popdown()
			w.popover.Unparent()
		}
	}
	t.items = make(map[string]*itemWidget)
}

func (t *Tray) OnItemAdded(item *tray.TrayItem) {
	glib.IdleAdd(func() {
		t.mu.Lock()
		if _, exists := t.items[item.Key]; exists {
			t.mu.Unlock()
			return
		}

		builder := gtk.NewBuilderFromString(ui.TrayItem)
		btn := builder.GetObject("tray_item_button").Cast().(*gtk.Button)
		icon := builder.GetObject("tray_item_icon").Cast().(*gtk.Image)

		popover := gtk.NewPopover()
		popover.SetParent(btn)
		popover.SetPosition(gtk.PosBottom)
		popover.AddCSSClass("tray-popover")

		w := &itemWidget{
			button:  btn,
			icon:    icon,
			popover: popover,
			item:    item,
		}
		t.items[item.Key] = w
		t.mu.Unlock()

		t.updateItemIcon(w)

		// Click interaction: opens sub-actions popover, or activates
		click := gtk.NewGestureClick()
		click.SetButton(0)
		click.ConnectReleased(func(n int, x, y float64) {
			b := click.CurrentButton()
			t.handleItemClick(w, b)
		})
		btn.AddController(click)

		if item.Title != "" {
			btn.SetTooltipText(item.Title)
		} else if item.Id != "" {
			btn.SetTooltipText(item.Id)
		}

		t.box.Append(btn)
	})
}

func (t *Tray) OnItemRemoved(key string) {
	glib.IdleAdd(func() {
		t.mu.Lock()
		w, ok := t.items[key]
		if !ok {
			t.mu.Unlock()
			return
		}
		delete(t.items, key)
		t.mu.Unlock()

		if w.popover != nil {
			w.popover.Popdown()
			w.popover.Unparent()
		}
		t.box.Remove(w.button)
	})
}

func (t *Tray) OnItemUpdated(item *tray.TrayItem) {
	glib.IdleAdd(func() {
		t.mu.Lock()
		w, ok := t.items[item.Key]
		t.mu.Unlock()
		if !ok {
			return
		}

		w.item = item
		t.updateItemIcon(w)
		if item.Title != "" {
			w.button.SetTooltipText(item.Title)
		}
	})
}

func (t *Tray) updateItemIcon(w *itemWidget) {
	it := w.item

	// Add custom search path if provided
	if it.IconThemePath != "" {
		display := gdk.DisplayGetDefault()
		if display != nil {
			theme := gtk.IconThemeGetForDisplay(display)
			if theme != nil {
				theme.AddSearchPath(it.IconThemePath)
			}
		}
	}

	if it.IconName != "" {
		w.icon.SetFromIconName(it.IconName)
		return
	}

	if len(it.IconPixmaps) > 0 {
		pixmap := it.IconPixmaps[0]
		// Choose the best size near iconSize if multiple exist
		for _, p := range it.IconPixmaps {
			if int(p.Width) >= t.iconSize && int(p.Width) <= t.iconSize*2 {
				pixmap = p
				break
			}
		}

		stride := uint(pixmap.Width * 4)
		if len(pixmap.Data) >= int(stride*uint(pixmap.Height)) {
			gbytes := glibv2.NewBytes(pixmap.Data)
			texture := gdk.NewMemoryTexture(int(pixmap.Width), int(pixmap.Height), gdk.MemoryA8R8G8B8, gbytes, stride)
			if texture != nil {
				w.icon.SetFromPaintable(texture)
				return
			}
		}
	}

	w.icon.SetFromIconName("application-x-executable-symbolic")
}

func (t *Tray) handleItemClick(w *itemWidget, button uint) {
	go func() {
		menuItems, err := w.item.GetMenu()
		if err != nil || len(menuItems) == 0 {
			// No D-Bus menu; trigger primary or secondary activation directly
			if button == gdk.BUTTON_SECONDARY {
				_ = w.item.ContextMenu(0, 0)
			} else {
				_ = w.item.Activate(0, 0)
			}
			return
		}

		glib.IdleAdd(func() {
			menuBox := gtk.NewBox(gtk.OrientationVertical, 2)
			menuBox.AddCSSClass("tray-menu")

			for _, mi := range menuItems {
				if mi.Type == "separator" {
					sep := gtk.NewSeparator(gtk.OrientationHorizontal)
					sep.AddCSSClass("tray-menu-separator")
					menuBox.Append(sep)
					continue
				}

				miBtn := gtk.NewButton()
				miBtn.AddCSSClass("tray-menu-item")

				row := gtk.NewBox(gtk.OrientationHorizontal, 8)
				if mi.ToggleType == "checkmark" {
					check := gtk.NewImage()
					if mi.ToggleState == 1 {
						check.SetFromIconName("object-select-symbolic")
					}
					check.SetPixelSize(14)
					row.Append(check)
				} else if mi.IconName != "" {
					img := gtk.NewImageFromIconName(mi.IconName)
					img.SetPixelSize(14)
					row.Append(img)
				}

				lbl := gtk.NewLabel(mi.Label)
				lbl.SetHAlign(gtk.AlignStart)
				lbl.SetHExpand(true)
				row.Append(lbl)

				miBtn.SetChild(row)
				if !mi.Enabled {
					miBtn.SetSensitive(false)
				}

				actionID := mi.ID
				miBtn.ConnectClicked(func() {
					w.popover.Popdown()
					go func() {
						if err := w.item.CallMenuEvent(actionID); err != nil {
							slog.Warn("tray menu action error", "error", err, "action_id", actionID)
						}
					}()
				})

				menuBox.Append(miBtn)
			}

			w.popover.SetChild(menuBox)
			w.popover.Popup()
		})
	}()
}
