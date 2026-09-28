package clock

import (
	"context"
	"time"

	"phalune/internal/widget"
	"phalune/ui"

	"github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

type Clock struct {
	label  *gtk.Label
	cancel context.CancelFunc
}

func New(ctx widget.Context) (widget.Widget, error) {
	format := ctx.Config.Bar.Clock.Format
	if format == "" {
		format = "15:04"
	}
	interval := ctx.Config.Bar.Clock.Interval.Duration
	if interval <= 0 {
		interval = time.Second
	}

	builder := gtk.NewBuilderFromString(ui.Clock)
	label := builder.GetObject("clock_label").Cast().(*gtk.Label)
	label.SetText(time.Now().Format(format))

	tickerCtx, cancel := context.WithCancel(context.Background())
	c := &Clock{
		label:  label,
		cancel: cancel,
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-tickerCtx.Done():
				return
			case <-ticker.C:
				text := time.Now().Format(format)
				glib.IdleAdd(func() {
					label.SetText(text)
				})
			}
		}
	}()

	return c, nil
}

func (c *Clock) Root() gtk.Widgetter {
	return c.label
}

func (c *Clock) Destroy() {
	if c.cancel != nil {
		c.cancel()
	}
}
