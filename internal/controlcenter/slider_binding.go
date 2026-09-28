package controlcenter

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

const defaultSliderLockout = 500 * time.Millisecond

type SliderBinding struct {
	mu sync.Mutex

	scale   *gtk.Scale
	label   *gtk.Label
	btn     *gtk.Button
	icon    *gtk.Image

	cloneScale *gtk.Scale
	cloneLabel *gtk.Label
	cloneBtn   *gtk.Button
	cloneIcon  *gtk.Image

	dragging        bool
	suppressEvent   bool
	lastPct         int
	lastUserChange  time.Time
	lockoutDuration time.Duration

	ch       chan int
	onApply  func(pct int)
	onMute   func()
	getIcon  func(pct int, muted bool) string
	muted    bool
	onNotify func(pct int)
}

type SliderConfig struct {
	Scale    *gtk.Scale
	Label    *gtk.Label
	Button   *gtk.Button
	Icon     *gtk.Image
	OnApply  func(pct int)
	OnMute   func()
	GetIcon  func(pct int, muted bool) string
	OnNotify func(pct int)
}

func NewSliderBinding(cfg SliderConfig) *SliderBinding {
	b := &SliderBinding{
		scale:           cfg.Scale,
		label:           cfg.Label,
		btn:             cfg.Button,
		icon:            cfg.Icon,
		lockoutDuration: defaultSliderLockout,
		ch:              make(chan int, 1),
		lastPct:         -1,
		onApply:         cfg.OnApply,
		onMute:          cfg.OnMute,
		getIcon:         cfg.GetIcon,
		onNotify:        cfg.OnNotify,
	}

	b.wirePrimary()
	return b
}

func (b *SliderBinding) jumpToPoint(scale *gtk.Scale, x float64) {
	w := float64(scale.Width())
	if w <= 0 {
		return
	}

	const sliderWidth = 16.0
	const half = sliderWidth / 2.0
	usable := w - sliderWidth
	if usable <= 0 {
		return
	}

	clampedX := x
	if clampedX < half {
		clampedX = half
	} else if clampedX > w-half {
		clampedX = w - half
	}

	frac := (clampedX - half) / usable
	lower := 0.0
	upper := 100.0
	if adj := scale.Adjustment(); adj != nil {
		lower = adj.Lower()
		upper = adj.Upper()
	}
	val := lower + frac*(upper-lower)
	scale.SetValue(val)
}

func (b *SliderBinding) wireScale(scale *gtk.Scale) {
	if scale == nil {
		return
	}

	onStart := func() {
		b.mu.Lock()
		b.dragging = true
		b.lastUserChange = time.Now()
		b.mu.Unlock()
		scale.AddCSSClass("dragging")
	}

	onEnd := func() {
		b.mu.Lock()
		b.dragging = false
		b.lastUserChange = time.Now()
		b.mu.Unlock()
		scale.RemoveCSSClass("dragging")
	}

	click := gtk.NewGestureClick()
	click.SetPropagationPhase(gtk.PhaseCapture)

	drag := gtk.NewGestureDrag()
	drag.SetPropagationPhase(gtk.PhaseCapture)

	click.Group(drag)

	click.ConnectPressed(func(n int, x, y float64) {
		click.SetState(gtk.EventSequenceClaimed)
		onStart()
		b.jumpToPoint(scale, x)
	})
	click.ConnectReleased(func(n int, x, y float64) {
		onEnd()
	})
	scale.AddController(click)

	drag.ConnectDragBegin(func(startX, startY float64) {
		drag.SetState(gtk.EventSequenceClaimed)
		onStart()
		b.jumpToPoint(scale, startX)
	})
	drag.ConnectDragUpdate(func(offsetX, offsetY float64) {
		drag.SetState(gtk.EventSequenceClaimed)
		if startX, _, ok := drag.StartPoint(); ok {
			b.jumpToPoint(scale, startX+offsetX)
		}
	})
	drag.ConnectDragEnd(func(offsetX, offsetY float64) {
		onEnd()
	})
	scale.AddController(drag)

	scale.ConnectChangeValue(func(scroll gtk.ScrollType, value float64) bool {
		if scroll == gtk.ScrollJump {
			return true
		}
		return false
	})

	scale.ConnectValueChanged(func() {
		b.onValueChanged(scale)
	})
}

func (b *SliderBinding) wirePrimary() {
	b.wireScale(b.scale)

	if b.btn != nil && b.onMute != nil {
		b.btn.ConnectClicked(func() {
			go b.onMute()
		})
	}
}

func (b *SliderBinding) BindClone(scale *gtk.Scale, label *gtk.Label, btn *gtk.Button, icon *gtk.Image) {
	b.mu.Lock()
	b.cloneScale = scale
	b.cloneLabel = label
	b.cloneBtn = btn
	b.cloneIcon = icon
	b.mu.Unlock()

	b.wireScale(scale)

	if btn != nil && b.onMute != nil {
		btn.ConnectClicked(func() {
			go b.onMute()
		})
	}
}

func (b *SliderBinding) onValueChanged(source *gtk.Scale) {
	b.mu.Lock()
	if b.suppressEvent {
		b.mu.Unlock()
		return
	}
	b.mu.Unlock()

	val := source.Value()
	pct := int(math.Round(val))
	if pct < 0 {
		pct = 0
	} else if pct > 100 {
		pct = 100
	}

	b.mu.Lock()
	if pct == b.lastPct {
		b.mu.Unlock()
		return
	}
	b.lastPct = pct
	b.lastUserChange = time.Now()
	b.suppressEvent = true
	b.mu.Unlock()

	// Optimistic UI updates
	if b.scale != nil && b.scale != source {
		b.scale.SetValue(float64(pct))
	}
	if b.cloneScale != nil && b.cloneScale != source {
		b.cloneScale.SetValue(float64(pct))
	}

	txt := fmt.Sprintf("%d%%", pct)
	if b.label != nil {
		b.label.SetText(txt)
	}
	if b.cloneLabel != nil {
		b.cloneLabel.SetText(txt)
	}

	b.mu.Lock()
	b.suppressEvent = false
	muted := b.muted
	b.mu.Unlock()

	if b.getIcon != nil {
		iconName := b.getIcon(pct, muted)
		if b.icon != nil {
			b.icon.SetFromIconName(iconName)
		}
		if b.cloneIcon != nil {
			b.cloneIcon.SetFromIconName(iconName)
		}
	}

	if b.onNotify != nil {
		b.onNotify(pct)
	}

	select {
	case b.ch <- pct:
	default:
		select {
		case <-b.ch:
		default:
		}
		b.ch <- pct
	}
}

func (b *SliderBinding) SetValue(pct int, muted bool) {
	if pct < 0 {
		pct = 0
	} else if pct > 100 {
		pct = 100
	}

	b.mu.Lock()
	b.muted = muted

	if b.getIcon != nil {
		iconName := b.getIcon(pct, muted)
		if b.icon != nil {
			b.icon.SetFromIconName(iconName)
		}
		if b.cloneIcon != nil {
			b.cloneIcon.SetFromIconName(iconName)
		}
	}

	if b.dragging || time.Since(b.lastUserChange) < b.lockoutDuration {
		b.mu.Unlock()
		return
	}

	b.lastPct = pct
	b.suppressEvent = true
	b.mu.Unlock()

	txt := fmt.Sprintf("%d%%", pct)
	if b.scale != nil {
		b.scale.SetValue(float64(pct))
	}
	if b.cloneScale != nil {
		b.cloneScale.SetValue(float64(pct))
	}
	if b.label != nil {
		b.label.SetText(txt)
	}
	if b.cloneLabel != nil {
		b.cloneLabel.SetText(txt)
	}

	b.mu.Lock()
	b.suppressEvent = false
	b.mu.Unlock()
}

func (b *SliderBinding) IsUserAdjusting() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.dragging || time.Since(b.lastUserChange) < b.lockoutDuration
}

func (b *SliderBinding) StartWorker(ctx context.Context) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case pct, ok := <-b.ch:
				if !ok {
					return
				}
				// Drain intermediate values to always apply latest
				drained := false
				for !drained {
					select {
					case next, ok := <-b.ch:
						if !ok {
							return
						}
						pct = next
					default:
						drained = true
					}
				}

				if b.onApply != nil {
					b.onApply(pct)
				}

				select {
				case <-ctx.Done():
					return
				case <-time.After(25 * time.Millisecond):
				}
			}
		}
	}()
}
