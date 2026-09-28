package osd

import (
	"fmt"
	"math"
	"sync"
	"time"

	"phalune/internal/config"
	"phalune/ui"

	"github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

type OSD struct {
	window              *gtk.Window
	icon                *gtk.Image
	label               *gtk.Label
	progress            *gtk.ProgressBar
	valueLbl            *gtk.Label
	timeout             time.Duration
	animate             bool
	animationDurationMs int

	mu             sync.Mutex
	timer          *time.Timer
	suppressFn     func() bool
	lastDirectUser time.Time

	// Cached UI state to prevent redundant GTK redraws and icon reloads
	lastIcon      string
	lastLabel     string
	lastValueText string
	lastFraction  float64
	isVisible     bool

	// Animation interpolation state
	currentFraction float64
	targetFraction  float64
	isAnimating     bool
	tickID          uint

	// Coalesced update state to avoid queue buildup during rapid events
	hasPending    bool
	pendingIcon   string
	pendingLabel  string
	pendingVal    float64
	pendingCustom string
}

func (o *OSD) SetSuppressFunc(fn func() bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.suppressFn = fn
}

func (o *OSD) IsInDirectUserLockout() bool {
	if o == nil {
		return false
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	return time.Since(o.lastDirectUser) < 150*time.Millisecond
}

func New(app *gtk.Application, cfg config.OSDConfig) (*OSD, error) {
	win := gtk.NewWindow()
	win.SetApplication(app)
	win.SetTitle("phalune-osd")
	win.SetDecorated(false)
	win.AddCSSClass("osd-window")

	if err := ConfigureOSDSurface(win, cfg); err != nil {
		return nil, fmt.Errorf("failed to configure OSD surface: %w", err)
	}

	builder := gtk.NewBuilderFromString(ui.OSD)
	card := builder.GetObject("osd_card").Cast().(*gtk.Box)
	icon := builder.GetObject("osd_icon").Cast().(*gtk.Image)
	label := builder.GetObject("osd_label").Cast().(*gtk.Label)
	progress := builder.GetObject("osd_progress").Cast().(*gtk.ProgressBar)
	valueLbl := builder.GetObject("osd_value").Cast().(*gtk.Label)

	win.SetChild(card)

	timeout := cfg.Timeout.Duration
	if timeout <= 0 {
		timeout = 2 * time.Second
	}

	animDuration := cfg.AnimationDurationMs
	if animDuration <= 0 {
		animDuration = 120
	}

	return &OSD{
		window:              win,
		icon:                icon,
		label:               label,
		progress:            progress,
		valueLbl:            valueLbl,
		timeout:             timeout,
		animate:             cfg.Animate,
		animationDurationMs: animDuration,
		lastFraction:        -1.0,
		currentFraction:     -1.0,
	}, nil
}

func (o *OSD) UpdateConfig(cfg config.OSDConfig) error {
	if o == nil {
		return nil
	}
	o.mu.Lock()
	timeout := cfg.Timeout.Duration
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	o.timeout = timeout
	o.animate = cfg.Animate
	if cfg.AnimationDurationMs > 0 {
		o.animationDurationMs = cfg.AnimationDurationMs
	}
	win := o.window
	o.mu.Unlock()

	if win != nil {
		return ConfigureOSDSurface(win, cfg)
	}
	return nil
}

func (o *OSD) Show(iconName string, labelText string, value float64) {
	if o == nil {
		return
	}
	o.mu.Lock()
	o.lastDirectUser = time.Now()
	o.mu.Unlock()
	o.ShowCustom(iconName, labelText, value, fmt.Sprintf("%d%%", int(value)))
}

func (o *OSD) ShowExternal(iconName string, labelText string, value float64) {
	if o == nil || o.IsInDirectUserLockout() {
		return
	}
	o.ShowCustom(iconName, labelText, value, fmt.Sprintf("%d%%", int(value)))
}

func (o *OSD) ShowCustomExternal(iconName string, labelText string, value float64, customText string) {
	if o == nil || o.IsInDirectUserLockout() {
		return
	}
	o.ShowCustom(iconName, labelText, value, customText)
}

func (o *OSD) ShowCustom(iconName string, labelText string, value float64, customText string) {
	if o == nil {
		return
	}

	o.mu.Lock()
	if o.suppressFn != nil && o.suppressFn() {
		o.mu.Unlock()
		return
	}

	if value < 0 {
		value = 0
	} else if value > 100 {
		value = 100
	}
	if iconName == "" {
		iconName = "dialog-information"
	}

	o.pendingIcon = iconName
	o.pendingLabel = labelText
	o.pendingVal = value
	o.pendingCustom = customText

	// Reset auto-dismiss timer on every update so it stays visible while sliding/scrolling
	if o.timer != nil {
		o.timer.Stop()
	}
	o.timer = time.AfterFunc(o.timeout, func() {
		glib.IdleAdd(func() {
			o.mu.Lock()
			defer o.mu.Unlock()
			if o.window != nil && o.isVisible {
				o.stopAnimation()
				o.window.SetVisible(false)
				o.isVisible = false
				o.lastIcon = ""
				o.lastLabel = ""
				o.lastValueText = ""
				o.lastFraction = -1.0
				o.currentFraction = -1.0
			}
		})
	})

	if o.hasPending {
		o.mu.Unlock()
		return
	}
	o.hasPending = true
	o.mu.Unlock()

	glib.IdleAdd(func() {
		o.mu.Lock()
		if o.window == nil {
			o.hasPending = false
			o.mu.Unlock()
			return
		}
		if o.suppressFn != nil && o.suppressFn() {
			o.hasPending = false
			o.mu.Unlock()
			return
		}

		icon := o.pendingIcon
		label := o.pendingLabel
		val := o.pendingVal
		custom := o.pendingCustom
		o.hasPending = false
		o.mu.Unlock()

		wasVisible := o.isVisible
		if !o.isVisible {
			o.window.SetVisible(true)
			o.window.Present()
			o.isVisible = true
		}

		// Only re-resolve icon from theme when the icon name actually changes
		if o.lastIcon != icon {
			o.icon.SetFromIconName(icon)
			o.lastIcon = icon
		}

		// Only recalculate text layout when label changes
		if o.lastLabel != label {
			o.label.SetText(label)
			o.lastLabel = label
		}

		// Only update custom value text when changed
		if o.lastValueText != custom {
			o.valueLbl.SetText(custom)
			o.lastValueText = custom
		}

		fraction := val / 100.0
		if o.lastFraction != fraction {
			o.lastFraction = fraction
			if !wasVisible || !o.animate {
				// Initial appearance or animation disabled: snap immediately
				o.stopAnimation()
				o.currentFraction = fraction
				o.targetFraction = fraction
				o.progress.SetFraction(fraction)
			} else {
				// While visible and scrolling: smoothly morph towards fraction
				o.animateTo(fraction)
			}
		}
	})
}

func (o *OSD) animateTo(targetFraction float64) {
	o.targetFraction = targetFraction
	if o.isAnimating {
		return
	}

	o.isAnimating = true
	durationMs := float64(o.animationDurationMs)
	if durationMs <= 0 {
		durationMs = 120
	}
	alpha := 1.0 - math.Pow(0.005, 16.0/durationMs)
	if alpha < 0.25 {
		alpha = 0.25
	} else if alpha > 0.6 {
		alpha = 0.6
	}

	o.tickID = o.progress.AddTickCallback(func(w gtk.Widgetter, clock gdk.FrameClocker) bool {
		o.mu.Lock()
		if !o.isAnimating || o.window == nil {
			o.isAnimating = false
			o.tickID = 0
			o.mu.Unlock()
			return false
		}

		diff := o.targetFraction - o.currentFraction
		if math.Abs(diff) < 0.002 {
			o.currentFraction = o.targetFraction
			curr := o.currentFraction
			o.isAnimating = false
			o.tickID = 0
			o.mu.Unlock()
			o.progress.SetFraction(curr)
			return false
		}

		o.currentFraction += diff * alpha
		curr := o.currentFraction
		o.mu.Unlock()

		o.progress.SetFraction(curr)
		return true
	})
}

func (o *OSD) stopAnimation() {
	if o.isAnimating && o.tickID != 0 {
		o.progress.RemoveTickCallback(o.tickID)
	}
	o.isAnimating = false
	o.tickID = 0
}

func (o *OSD) Destroy() {
	o.mu.Lock()
	if o.timer != nil {
		o.timer.Stop()
		o.timer = nil
	}
	o.hasPending = false
	o.stopAnimation()
	o.mu.Unlock()

	if o.window != nil {
		if o.window.Realized() {
			o.window.Destroy()
		}
		o.window = nil
	}
}
