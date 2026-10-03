package bar

import (
	"fmt"

	"phalune/internal/config"
	"phalune/internal/widget"
	"phalune/ui"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

type Bar struct {
	window    *gtk.Window
	centerBox *gtk.CenterBox
	widgets   []widget.Widget
}

func newBarSection(positionClass string) *gtk.Box {
	b := gtk.NewBuilderFromString(ui.BarSection)
	box := b.GetObject("bar_section").Cast().(*gtk.Box)
	box.AddCSSClass(positionClass)
	return box
}

func New(app *gtk.Application, monitor *gdk.Monitor, cfg *config.Config, registry *widget.Registry, widgetCtx widget.Context) (*Bar, error) {
	win := gtk.NewWindow()
	win.SetApplication(app)
	win.SetTitle("phalune-bar")
	win.SetDecorated(false)
	win.AddCSSClass("bar-window")

	height := cfg.Bar.Height
	if height <= 0 {
		height = 32
	}
	win.SetDefaultSize(-1, height)

	unit := height - 8
	if unit < 20 {
		unit = 20
	}
	textUnit := int(float64(unit) * 1.25)
	if textUnit < 32 {
		textUnit = 32
	}
	fontSize := 12
	if height <= 26 {
		fontSize = 11
	} else if height >= 38 {
		fontSize = 13
	}
	applyBarMetrics(height, unit, textUnit, fontSize)

	position := cfg.Bar.Position
	if position == "" {
		position = "top"
	}
	if err := ConfigureLayerSurface(win, monitor, height, position); err != nil {
		return nil, fmt.Errorf("failed to create layer-shell surface: %w", err)
	}

	builder := gtk.NewBuilderFromString(ui.Bar)
	centerBox := builder.GetObject("bar_root").Cast().(*gtk.CenterBox)

	leftBox := newBarSection("bar-left")
	centerBoxWidget := newBarSection("bar-center")
	rightBox := newBarSection("bar-right")

	centerBox.SetStartWidget(leftBox)
	centerBox.SetCenterWidget(centerBoxWidget)
	centerBox.SetEndWidget(rightBox)

	win.SetChild(centerBox)

	var createdWidgets []widget.Widget

	for _, name := range cfg.Bar.Left.Widgets {
		w, err := registry.Create(name, widgetCtx)
		if err != nil {
			return nil, fmt.Errorf("failed to instantiate widget %q in bar.left: %w", name, err)
		}
		leftBox.Append(w.Root())
		createdWidgets = append(createdWidgets, w)
	}

	for _, name := range cfg.Bar.Center.Widgets {
		w, err := registry.Create(name, widgetCtx)
		if err != nil {
			return nil, fmt.Errorf("failed to instantiate widget %q in bar.center: %w", name, err)
		}
		centerBoxWidget.Append(w.Root())
		createdWidgets = append(createdWidgets, w)
	}

	for _, name := range cfg.Bar.Right.Widgets {
		w, err := registry.Create(name, widgetCtx)
		if err != nil {
			return nil, fmt.Errorf("failed to instantiate widget %q in bar.right: %w", name, err)
		}
		rightBox.Append(w.Root())
		createdWidgets = append(createdWidgets, w)
	}

	return &Bar{
		window:    win,
		centerBox: centerBox,
		widgets:   createdWidgets,
	}, nil
}

func (b *Bar) Present() {
	b.window.Present()
}

func (b *Bar) Window() *gtk.Window {
	return b.window
}

func (b *Bar) Destroy() {
	for _, w := range b.widgets {
		w.Destroy()
	}
	b.widgets = nil

	if b.window != nil {
		if b.window.Realized() {
			b.window.Destroy()
		}
		b.window = nil
	}
}

var barMetricsProvider *gtk.CSSProvider

func applyBarMetrics(height, unit, textUnit, fontSize int) {
	dynamicCSS := fmt.Sprintf(":root {\n  --bar-height: %dpx;\n  --bar-unit: %dpx;\n  --bar-text-unit: %dpx;\n  --bar-font-size: %dpx;\n}\n", height, unit, textUnit, fontSize)
	if barMetricsProvider == nil {
		barMetricsProvider = gtk.NewCSSProvider()
		display := gdk.DisplayGetDefault()
		if display != nil {
			gtk.StyleContextAddProviderForDisplay(display, barMetricsProvider, gtk.STYLE_PROVIDER_PRIORITY_APPLICATION)
		}
	}
	barMetricsProvider.LoadFromString(dynamicCSS)
}
