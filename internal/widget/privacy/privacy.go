package privacy

import (
	"sync"

	"phalune/internal/widget"
	"phalune/ui"

	"github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

type Widget struct {
	box         *gtk.Box
	micIcon     *gtk.Image
	camIcon     *gtk.Image
	unsubscribe func()
	mu          sync.Mutex
}

func New(ctx widget.Context) (widget.Widget, error) {
	builder := gtk.NewBuilderFromString(ui.PrivacyWidget)
	box := builder.GetObject("privacy_box").Cast().(*gtk.Box)
	micIcon := builder.GetObject("mic_icon").Cast().(*gtk.Image)
	camIcon := builder.GetObject("camera_icon").Cast().(*gtk.Image)

	w := &Widget{
		box:     box,
		micIcon: micIcon,
		camIcon: camIcon,
	}

	update := func(mic, cam bool) {
		micIcon.SetVisible(mic)
		camIcon.SetVisible(cam)
		box.SetVisible(mic || cam)
	}

	if ctx.Privacy != nil {
		w.unsubscribe = ctx.Privacy.Subscribe(func(mic, cam bool) {
			glib.IdleAdd(func() {
				update(mic, cam)
			})
		})
	}

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
