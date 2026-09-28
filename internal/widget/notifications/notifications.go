package notifications

import (
	"fmt"
	"sync"

	"phalune/internal/widget"
	"phalune/ui"

	"github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

type Widget struct {
	box         *gtk.Box
	icon        *gtk.Image
	badge       *gtk.Label
	unsubscribe func()
	mu          sync.Mutex
}

func New(ctx widget.Context) (widget.Widget, error) {
	builder := gtk.NewBuilderFromString(ui.NotificationsWidget)
	box := builder.GetObject("notifications_box").Cast().(*gtk.Box)
	icon := builder.GetObject("notifications_icon").Cast().(*gtk.Image)
	badge := builder.GetObject("notifications_badge").Cast().(*gtk.Label)

	w := &Widget{
		box:   box,
		icon:  icon,
		badge: badge,
	}

	updateBadge := func() {
		if ctx.NotifyStore == nil {
			badge.SetVisible(false)
			return
		}
		c := ctx.NotifyStore.Count()
		if c > 0 {
			badge.SetText(fmt.Sprintf("%d", c))
			badge.SetVisible(true)
		} else {
			badge.SetVisible(false)
		}
	}

	if ctx.NotifyStore != nil {
		w.unsubscribe = ctx.NotifyStore.Subscribe(func() {
			glib.IdleAdd(func() {
				updateBadge()
			})
		})
	}
	updateBadge()

	click := gtk.NewGestureClick()
	click.ConnectReleased(func(n int, x, y float64) {
		if ctx.ToggleNotificationCenter != nil {
			ctx.ToggleNotificationCenter()
		}
	})
	box.AddController(click)

	return w, nil
}

func (w *Widget) Root() gtk.Widgetter {
	return w.box
}

func (w *Widget) Destroy() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.unsubscribe != nil {
		w.unsubscribe()
		w.unsubscribe = nil
	}
}
