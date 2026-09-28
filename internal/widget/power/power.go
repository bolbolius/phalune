package power

import (
	"phalune/internal/widget"
	"phalune/ui"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

type Power struct {
	box  *gtk.Box
	icon *gtk.Image
}

func New(ctx widget.Context) (widget.Widget, error) {
	builder := gtk.NewBuilderFromString(ui.Power)
	box := builder.GetObject("power_box").Cast().(*gtk.Box)
	icon := builder.GetObject("power_icon").Cast().(*gtk.Image)

	if ctx.Config != nil && ctx.Config.Bar.Power.Icon != "" {
		icon.SetFromIconName(ctx.Config.Bar.Power.Icon)
	}

	click := gtk.NewGestureClick()
	click.SetButton(0)
	click.ConnectReleased(func(n int, x, y float64) {
		btn := click.CurrentButton()
		if btn == gdk.BUTTON_PRIMARY || btn == gdk.BUTTON_SECONDARY {
			if ctx.TogglePowerMenu != nil {
				ctx.TogglePowerMenu()
			}
		}
	})
	box.AddController(click)

	return &Power{
		box:  box,
		icon: icon,
	}, nil
}

func (p *Power) Root() gtk.Widgetter {
	return p.box
}

func (p *Power) Destroy() {
}
