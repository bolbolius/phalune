package clipboard

import (
	"phalune/internal/widget"
	"phalune/ui"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

type Clipboard struct {
	box  *gtk.Box
	icon *gtk.Image
}

func New(ctx widget.Context) (widget.Widget, error) {
	builder := gtk.NewBuilderFromString(ui.ClipboardWidget)
	box := builder.GetObject("clipboard_box").Cast().(*gtk.Box)
	icon := builder.GetObject("clipboard_icon").Cast().(*gtk.Image)

	click := gtk.NewGestureClick()
	click.SetButton(0)
	click.ConnectReleased(func(n int, x, y float64) {
		btn := click.CurrentButton()
		if btn == gdk.BUTTON_PRIMARY || btn == gdk.BUTTON_SECONDARY {
			if ctx.ToggleClipboard != nil {
				ctx.ToggleClipboard()
			}
		}
	})
	box.AddController(click)

	return &Clipboard{
		box:  box,
		icon: icon,
	}, nil
}

func (c *Clipboard) Root() gtk.Widgetter {
	return c.box
}

func (c *Clipboard) Destroy() {
}
