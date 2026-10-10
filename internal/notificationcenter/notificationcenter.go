package notificationcenter

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"phalune/internal/notify"
	"phalune/ui"

	"github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

type NotificationCenter struct {
	window     *gtk.Window
	overlayBox *gtk.Overlay
	card       *gtk.Box

	titleLabel     *gtk.Label
	countBadge     *gtk.Label
	dndButton      *gtk.Button
	clearButton    *gtk.Button
	closeButton    *gtk.Button
	scrolledWindow *gtk.ScrolledWindow
	listBox        *gtk.ListBox
	emptyLabel     *gtk.Label

	notifyMgr *notify.Manager
	store     *notify.Store

	undoBox   *gtk.Box
	undoBtn   *gtk.Button
	undoItems []notify.StoredItem
	undoTimer *time.Timer
	undoSeq   uint64

	mu          sync.Mutex
	visible     bool
	unsubscribe func()
}

func New(app *gtk.Application, notifyMgr *notify.Manager) (*NotificationCenter, error) {
	win := gtk.NewWindow()
	win.SetApplication(app)
	win.SetTitle("phalune-notification-center")
	win.SetDecorated(false)
	win.AddCSSClass("notification-center-window")

	if err := ConfigureNotificationCenterSurface(win); err != nil {
		return nil, fmt.Errorf("failed to configure notification center surface: %w", err)
	}

	builder := gtk.NewBuilderFromString(ui.NotificationCenter)
	overlayBox := builder.GetObject("overlay_box").Cast().(*gtk.Overlay)
	card := builder.GetObject("card").Cast().(*gtk.Box)

	titleLabel := builder.GetObject("title_label").Cast().(*gtk.Label)
	countBadge := builder.GetObject("count_badge").Cast().(*gtk.Label)
	dndBtn := builder.GetObject("dnd_button").Cast().(*gtk.Button)
	clearBtn := builder.GetObject("clear_button").Cast().(*gtk.Button)
	closeBtn := builder.GetObject("close_button").Cast().(*gtk.Button)
	scrolledWin := builder.GetObject("scrolled_window").Cast().(*gtk.ScrolledWindow)
	listBox := builder.GetObject("list_box").Cast().(*gtk.ListBox)
	emptyLabel := builder.GetObject("empty_label").Cast().(*gtk.Label)

	// Create floating undo bar inside notification center card
	undoBox := gtk.NewBox(gtk.OrientationHorizontal, 8)
	undoBox.AddCSSClass("notification-undo-bar")
	undoBox.SetHAlign(gtk.AlignCenter)
	undoBox.SetVisible(false)

	undoMsg := gtk.NewLabel("Notifications cleared")
	undoMsg.AddCSSClass("notification-undo-label")
	undoBtn := gtk.NewButtonWithLabel("Undo")
	undoBtn.AddCSSClass("notification-undo-btn")

	undoBox.Append(undoMsg)
	undoBox.Append(undoBtn)
	card.Append(undoBox)

	win.SetChild(overlayBox)

	nc := &NotificationCenter{
		window:         win,
		overlayBox:     overlayBox,
		card:           card,
		titleLabel:     titleLabel,
		countBadge:     countBadge,
		dndButton:      dndBtn,
		clearButton:    clearBtn,
		closeButton:    closeBtn,
		scrolledWindow: scrolledWin,
		listBox:        listBox,
		emptyLabel:     emptyLabel,
		undoBox:        undoBox,
		undoBtn:        undoBtn,
		notifyMgr:      notifyMgr,
	}

	if notifyMgr != nil {
		nc.store = notifyMgr.Store()
		if nc.store != nil {
			nc.unsubscribe = nc.store.Subscribe(func() {
				glib.IdleAdd(func() {
					nc.Render()
				})
			})
		}
	}

	nc.setupInteractivity()
	nc.Render()

	return nc, nil
}

func (nc *NotificationCenter) setupInteractivity() {
	nc.closeButton.ConnectClicked(func() {
		nc.Close()
	})

	nc.clearButton.ConnectClicked(func() {
		if nc.store == nil {
			return
		}
		items := nc.store.All()
		if len(items) == 0 {
			return
		}
		nc.mu.Lock()
		nc.undoSeq++
		seq := nc.undoSeq
		nc.undoItems = items
		if nc.undoTimer != nil {
			nc.undoTimer.Stop()
		}
		nc.undoTimer = time.AfterFunc(6*time.Second, func() {
			glib.IdleAdd(func() {
				nc.mu.Lock()
				if nc.undoSeq == seq {
					nc.undoItems = nil
					nc.undoBox.SetVisible(false)
				}
				nc.mu.Unlock()
			})
		})
		nc.mu.Unlock()

		nc.undoBox.SetVisible(true)
		if nc.notifyMgr != nil {
			nc.notifyMgr.ClearAll()
		} else {
			nc.store.Clear()
		}
	})

	nc.undoBtn.ConnectClicked(func() {
		nc.mu.Lock()
		if nc.undoTimer != nil {
			nc.undoTimer.Stop()
			nc.undoTimer = nil
		}
		items := nc.undoItems
		nc.undoItems = nil
		nc.mu.Unlock()

		nc.undoBox.SetVisible(false)
		if len(items) > 0 && nc.store != nil {
			nc.store.Restore(items)
		}
	})

	nc.dndButton.ConnectClicked(func() {
		if nc.notifyMgr != nil {
			next := !nc.notifyMgr.IsDND()
			nc.notifyMgr.SetDND(next)
			nc.updateDNDUI(next)
		}
	})

	// Backdrop click outside card closes notification center
	click := gtk.NewGestureClick()
	click.ConnectReleased(func(n int, x, y float64) {
		pick := nc.overlayBox.Pick(x, y, gtk.PickDefault)
		if pick != nil {
			w := gtk.BaseWidget(pick)
			if w != nil && (w == &nc.card.Widget || w.IsAncestor(nc.card)) {
				return
			}
		}
		nc.Close()
	})
	nc.overlayBox.AddController(click)

	keyCtrl := gtk.NewEventControllerKey()
	keyCtrl.ConnectKeyPressed(func(keyval, keycode uint, state gdk.ModifierType) bool {
		if keyval == gdk.KEY_Escape {
			nc.Close()
			return true
		}
		return false
	})
	nc.window.AddController(keyCtrl)
}

func (nc *NotificationCenter) updateDNDUI(enabled bool) {
	if enabled {
		nc.dndButton.SetIconName("notifications-disabled-symbolic")
		nc.dndButton.AddCSSClass("active")
	} else {
		nc.dndButton.SetIconName("preferences-system-notifications-symbolic")
		nc.dndButton.RemoveCSSClass("active")
	}
}

func (nc *NotificationCenter) Render() {
	if nc.listBox == nil {
		return
	}

	for row := nc.listBox.RowAtIndex(0); row != nil; row = nc.listBox.RowAtIndex(0) {
		nc.listBox.Remove(row)
	}

	if nc.store == nil {
		nc.emptyLabel.SetVisible(true)
		nc.scrolledWindow.SetVisible(false)
		nc.countBadge.SetVisible(false)
		return
	}

	items := nc.store.All()
	count := len(items)

	if count == 0 {
		nc.emptyLabel.SetText("No new notifications\nYou're all caught up")
		nc.emptyLabel.SetVisible(true)
		nc.scrolledWindow.SetVisible(false)
		nc.countBadge.SetVisible(false)
		return
	}

	nc.emptyLabel.SetVisible(false)
	nc.scrolledWindow.SetVisible(true)
	nc.countBadge.SetText(fmt.Sprintf("%d", count))
	nc.countBadge.SetVisible(true)

	for _, it := range items {
		item := it
		builder := gtk.NewBuilderFromString(ui.NotificationHistoryItem)
		itemBox := builder.GetObject("item_box").Cast().(*gtk.Box)
		icon := builder.GetObject("item_icon").Cast().(*gtk.Image)
		appName := builder.GetObject("app_name").Cast().(*gtk.Label)
		timeLabel := builder.GetObject("time_label").Cast().(*gtk.Label)
		dismissBtn := builder.GetObject("dismiss_button").Cast().(*gtk.Button)
		summaryLabel := builder.GetObject("summary_label").Cast().(*gtk.Label)
		bodyLabel := builder.GetObject("body_label").Cast().(*gtk.Label)
		actionsBox := builder.GetObject("actions_box").Cast().(*gtk.Box)

		if strings.HasPrefix(item.Icon, "/") || strings.HasPrefix(item.Icon, "file://") {
			icon.SetFromFile(strings.TrimPrefix(item.Icon, "file://"))
		} else if item.Icon != "" {
			icon.SetFromIconName(item.Icon)
		} else {
			icon.SetFromIconName("dialog-information")
		}

		name := item.AppName
		if name == "" {
			name = "System"
		}
		appName.SetText(name)
		timeLabel.SetText(formatRelativeTime(item.Timestamp))
		summaryLabel.SetText(item.Summary)

		if item.Body != "" {
			if strings.Contains(item.Body, "<") && strings.Contains(item.Body, ">") {
				bodyLabel.SetMarkup(item.Body)
			} else {
				bodyLabel.SetText(item.Body)
			}
			bodyLabel.SetVisible(true)
		}

		dismissBtn.ConnectClicked(func() {
			if nc.store != nil {
				nc.store.Remove(item.ID)
			}
			if nc.notifyMgr != nil {
				nc.notifyMgr.Dismiss(item.ID, notify.CloseReasonDismissedByUser)
			}
		})

		if len(item.Actions) > 0 {
			for _, act := range item.Actions {
				if act.Key == "default" {
					continue
				}
				btn := gtk.NewButtonWithLabel(act.Label)
				btn.AddCSSClass("notify-action-btn")
				actionKey := act.Key
				btn.ConnectClicked(func() {
					if nc.notifyMgr != nil {
						nc.notifyMgr.InvokeAction(item.ID, actionKey)
					}
					if nc.store != nil {
						nc.store.Remove(item.ID)
					}
				})
				actionsBox.Append(btn)
			}
			actionsBox.SetVisible(true)
		}

		nc.listBox.Append(itemBox)
	}
}

func formatRelativeTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	if d < 1*time.Minute {
		return "Just now"
	}
	if d < 1*time.Hour {
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	return t.Format("Jan 02")
}

func (nc *NotificationCenter) Open() {
	glib.IdleAdd(func() {
		nc.mu.Lock()
		nc.visible = true
		nc.mu.Unlock()

		if nc.notifyMgr != nil {
			nc.updateDNDUI(nc.notifyMgr.IsDND())
		}
		nc.Render()
		nc.window.Present()
	})
}

func (nc *NotificationCenter) Close() {
	glib.IdleAdd(func() {
		nc.mu.Lock()
		nc.visible = false
		nc.mu.Unlock()

		nc.window.SetVisible(false)
	})
}

func (nc *NotificationCenter) Toggle() {
	nc.mu.Lock()
	vis := nc.visible
	nc.mu.Unlock()

	if vis {
		nc.Close()
	} else {
		nc.Open()
	}
}

func (nc *NotificationCenter) IsVisible() bool {
	nc.mu.Lock()
	defer nc.mu.Unlock()
	return nc.visible
}

func (nc *NotificationCenter) Destroy() {
	nc.Close()
	if nc.unsubscribe != nil {
		nc.unsubscribe()
	}
	glib.IdleAdd(func() {
		if nc.window != nil {
			if nc.window.Realized() {
				nc.window.Destroy()
			}
			nc.window = nil
		}
	})
}
