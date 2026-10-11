package osd

import (
	"fmt"
	"math"
	"sync"
	"time"

	"phalune/internal/config"
	"phalune/internal/style"
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
	styleName           string
	timeout             time.Duration
	animate             bool
	animationDurationMs int

	mu             sync.Mutex
	timer          *time.Timer
	suppressFn     func() bool
	lastDirectUser time.Time
	eventHook      func(icon, label string, value float64, customText string)

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

// SetEventHook registers a callback fired for every OSD show, including
// suppressed ones. Runs on the caller goroutine; used by the shell to
// mirror state onto the IPC event bus.
func (o *OSD) SetEventHook(fn func(icon, label string, value float64, customText string)) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.eventHook = fn
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

	styleName, styleClass := style.Resolve(style.OSD, cfg.Style)

	card, icon, label, progress, valueLbl := createOSDWidgets(styleName)
	card.AddCSSClass(styleClass)

	win.SetChild(card)

	timeout := cfg.Timeout.Duration
	if timeout <= 0 {
		timeout = 2 * time.Second
	}

	animDuration := cfg.AnimationDurationMs
	if animDuration <= 0 {
		animDuration = 120
	}

	o := &OSD{
		window:              win,
		icon:                icon,
		label:               label,
		progress:            progress,
		valueLbl:            valueLbl,
		styleName:           styleName,
		timeout:             timeout,
		animate:             cfg.Animate,
		animationDurationMs: animDuration,
		lastFraction:        -1.0,
		currentFraction:     -1.0,
	}

	// Hovering the pill holds it on screen; leaving re-arms the timeout.
	hover := gtk.NewEventControllerMotion()
	hover.ConnectEnter(func(_, _ float64) {
		o.holdDismiss()
	})
	hover.ConnectLeave(func() {
		o.releaseDismiss()
	})
	card.AddController(hover)

	return o, nil
}

// armDismissLocked restarts the auto-dismiss timer. Callers must hold o.mu.
func (o *OSD) armDismissLocked() {
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
}

// holdDismiss pauses auto-dismiss while hovered.
func (o *OSD) holdDismiss() {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.timer != nil {
		o.timer.Stop()
		o.timer = nil
	}
}

func (o *OSD) releaseDismiss() {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.isVisible && o.timer == nil {
		o.armDismissLocked()
	}
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
	styleName, styleClass := style.Resolve(style.OSD, cfg.Style)
	styleChanged := styleName != o.styleName
	if styleChanged {
		// Set under lock so rapid reloads don't queue duplicate rebuilds.
		o.styleName = styleName
	}
	o.mu.Unlock()

	// Style swaps rebuild the widget tree; state (fraction etc.) re-syncs
	// on the next Show call.
	if styleChanged {
		glib.IdleAdd(func() {
			o.rebuildChild(styleName, styleClass)
		})
	}

	if win != nil {
		return ConfigureOSDSurface(win, cfg)
	}
	return nil
}

// rebuildChild swaps the OSD layout tree for a new style. Main thread only.
func (o *OSD) rebuildChild(styleName, styleClass string) {
	o.mu.Lock()
	if o.window == nil {
		o.mu.Unlock()
		return
	}
	card, icon, label, progress, valueLbl := createOSDWidgets(styleName)
	card.AddCSSClass(styleClass)

	// SetChild implicitly unparents any previous child.
	o.window.SetChild(card)

	o.icon, o.label, o.progress, o.valueLbl = icon, label, progress, valueLbl
	o.lastIcon, o.lastLabel, o.lastValueText = "", "", ""
	o.stopAnimation()
	o.currentFraction, o.targetFraction, o.lastFraction = -1.0, -1.0, -1.0
	o.mu.Unlock()
}

// createOSDWidgets builds the widget tree for a style: template-based when
// one exists, otherwise the default Blueprint UI. Main thread only.
func createOSDWidgets(styleName string) (card *gtk.Box, icon *gtk.Image, label *gtk.Label, progress *gtk.ProgressBar, valueLbl *gtk.Label) {
	if style.HasTemplate(style.OSD, styleName) {
		return buildOSDLayout(styleName)
	}
	builder := gtk.NewBuilderFromString(ui.OSD)
	return builder.GetObject("osd_card").Cast().(*gtk.Box),
		builder.GetObject("osd_icon").Cast().(*gtk.Image),
		builder.GetObject("osd_label").Cast().(*gtk.Label),
		builder.GetObject("osd_progress").Cast().(*gtk.ProgressBar),
		builder.GetObject("osd_value").Cast().(*gtk.Label)
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
	o.armDismissLocked()

	hook := o.eventHook
	isPending := o.hasPending
	if !isPending {
		o.hasPending = true
	}
	o.mu.Unlock()

	if hook != nil {
		hook(iconName, labelText, value, customText)
	}
	if isPending {
		return
	}

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
		if o.label != nil && o.lastLabel != label {
			o.label.SetText(label)
			o.lastLabel = label
		}

		// Only update custom value text when changed
		if o.valueLbl != nil && o.lastValueText != custom {
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
