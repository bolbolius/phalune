package audio

import (
	"bufio"
	"context"
	"fmt"
	"math"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"phalune/internal/widget"
	"phalune/ui"

	"github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

var (
	rePercent    = regexp.MustCompile(`(\d+)%`)
	wpctlPath, _ = exec.LookPath("wpctl")
	pactlPath, _ = exec.LookPath("pactl")
)

type Audio struct {
	box    *gtk.Box
	icon   *gtk.Image
	label  *gtk.Label
	cancel context.CancelFunc

	step             int
	maxVolume        float64
	scrollDebounceMs time.Duration
	format           string
	mutedLabel       string
	showOSD          func(icon, label string, value float64)
	openSubpage      func(page string)

	mu          sync.Mutex
	volume      float64
	muted       bool
	lastScroll  time.Time
	lastUserVol time.Time
	stepCh      chan int
}

func New(ctx widget.Context) (widget.Widget, error) {
	builder := gtk.NewBuilderFromString(ui.Audio)
	box := builder.GetObject("audio_box").Cast().(*gtk.Box)
	icon := builder.GetObject("audio_icon").Cast().(*gtk.Image)
	label := builder.GetObject("audio_label").Cast().(*gtk.Label)

	step := ctx.Config.Bar.Audio.Step
	if step <= 0 {
		step = 5
	}
	maxVol := float64(ctx.Config.Bar.Audio.MaxVolume)
	if maxVol <= 0 {
		maxVol = 100
	}
	debounce := time.Duration(ctx.Config.Bar.Audio.ScrollDebounceMs) * time.Millisecond
	if debounce <= 0 {
		debounce = 20 * time.Millisecond
	}
	format := ctx.Config.Bar.Audio.Format
	if format == "" {
		format = "%d%%"
	}
	mutedLabel := ctx.Config.Bar.Audio.MutedLabel
	if mutedLabel == "" {
		mutedLabel = "Muted"
	}

	aCtx, cancel := context.WithCancel(context.Background())
	a := &Audio{
		box:              box,
		icon:             icon,
		label:            label,
		cancel:           cancel,
		step:             step,
		maxVolume:        maxVol,
		scrollDebounceMs: debounce,
		format:           format,
		mutedLabel:       mutedLabel,
		showOSD:          ctx.ShowOSD,
		openSubpage:      ctx.OpenControlCenterSubpage,
		stepCh:           make(chan int, 8),
	}

	a.setupInteractions()

	// Initial audio sync
	vol, muted, err := queryAudioState()
	if err == nil {
		a.updateUI(vol, muted)
	}

	// Single-flight worker applying volume changes sequentially
	go a.processStepWorker(aCtx)

	// Zero-polling audio subscriber
	go a.listenAudioEvents(aCtx)

	return a, nil
}

func (a *Audio) setupInteractions() {
	// Scroll interaction (scroll up to increase, scroll down to decrease)
	scroll := gtk.NewEventControllerScroll(gtk.EventControllerScrollVertical)
	scroll.ConnectScroll(func(dx, dy float64) bool {
		a.mu.Lock()
		now := time.Now()
		if now.Sub(a.lastScroll) < a.scrollDebounceMs {
			a.mu.Unlock()
			return true
		}
		a.lastScroll = now
		a.mu.Unlock()

		if dy < 0 {
			a.stepVolume(+a.step)
		} else if dy > 0 {
			a.stepVolume(-a.step)
		}
		return true
	})
	a.box.AddController(scroll)

	// Click interaction (left-click opens audio subpage, middle-click toggles mute)
	click := gtk.NewGestureClick()
	click.SetButton(0) // Receive all mouse buttons
	click.ConnectReleased(func(n int, x, y float64) {
		if click.CurrentButton() == gdk.BUTTON_MIDDLE {
			go a.toggleMute()
		} else if click.CurrentButton() == gdk.BUTTON_PRIMARY {
			if a.openSubpage != nil {
				a.openSubpage("audio")
			}
		}
	})
	a.box.AddController(click)
}

func (a *Audio) Root() gtk.Widgetter {
	return a.box
}

func (a *Audio) Destroy() {
	if a.cancel != nil {
		a.cancel()
		a.cancel = nil
	}
}

func (a *Audio) updateUI(volume float64, muted bool) {
	a.mu.Lock()
	if time.Since(a.lastUserVol) < 500*time.Millisecond {
		a.mu.Unlock()
		return
	}
	a.volume = volume
	a.muted = muted
	a.mu.Unlock()

	a.renderUI(volume, muted)
}

func (a *Audio) renderUI(volume float64, muted bool) {
	pct := int(math.Round(volume))
	if pct < 0 {
		pct = 0
	} else if float64(pct) > a.maxVolume {
		pct = int(a.maxVolume)
	}

	glib.IdleAdd(func() {
		a.icon.SetFromIconName(VolumeIconName(pct, muted))
		if muted {
			a.label.SetText(a.mutedLabel)
			a.box.AddCSSClass("muted")
		} else {
			a.label.SetText(fmt.Sprintf(a.format, pct))
			a.box.RemoveCSSClass("muted")
		}
	})
}

func VolumeIconName(pct int, muted bool) string {
	if muted || pct <= 0 {
		return "audio-volume-muted-symbolic"
	}
	if pct < 33 {
		return "audio-volume-low-symbolic"
	}
	if pct < 66 {
		return "audio-volume-medium-symbolic"
	}
	return "audio-volume-high-symbolic"
}

func (a *Audio) stepVolume(delta int) {
	a.mu.Lock()
	if a.muted {
		a.muted = false
	}
	cur := a.volume
	newVol := cur + float64(delta)
	if newVol > a.maxVolume {
		newVol = a.maxVolume
	} else if newVol < 0.0 {
		newVol = 0.0
	}

	// Strictly limit volume to maxVolume: do not exceed or allow upward step if already at or above maxVolume
	if cur >= a.maxVolume && delta > 0 {
		a.mu.Unlock()
		return
	}

	a.volume = newVol
	a.lastUserVol = time.Now()
	muted := a.muted
	a.mu.Unlock()

	// Instant optimistic UI update for butter-smooth scrolling
	a.renderUI(newVol, muted)
	if a.showOSD != nil {
		pct := int(math.Round(newVol))
		icon := VolumeIconName(pct, muted)
		a.showOSD(icon, "Volume", float64(pct))
	}

	select {
	case a.stepCh <- delta:
	default:
		select {
		case prev := <-a.stepCh:
			delta += prev
		default:
		}
		a.stepCh <- delta
	}
}

func (a *Audio) processStepWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case delta, ok := <-a.stepCh:
			if !ok {
				return
			}
			drained := false
			for !drained {
				select {
				case next, ok := <-a.stepCh:
					if !ok {
						return
					}
					delta += next
				default:
					drained = true
				}
			}

			a.applySystemVolume(delta)
			time.Sleep(15 * time.Millisecond)
		}
	}
}

func (a *Audio) applySystemVolume(delta int) {
	if delta == 0 {
		return
	}
	if wpctlPath != "" {
		sign := "+"
		val := delta
		if delta < 0 {
			sign = "-"
			val = -delta
		}
		arg := fmt.Sprintf("%d%%%s", val, sign)
		limitArg := fmt.Sprintf("%.2f", a.maxVolume/100.0)
		_ = exec.Command(wpctlPath, "set-volume", "-l", limitArg, "@DEFAULT_AUDIO_SINK@", arg).Run()
		return
	}
	if pactlPath != "" {
		arg := fmt.Sprintf("%+d%%", delta)
		_ = exec.Command(pactlPath, "set-sink-volume", "@DEFAULT_SINK@", arg).Run()
	}
}

func (a *Audio) toggleMute() {
	a.mu.Lock()
	a.muted = !a.muted
	muted := a.muted
	vol := a.volume
	a.mu.Unlock()

	a.renderUI(vol, muted)
	if a.showOSD != nil {
		pct := int(math.Round(vol))
		icon := VolumeIconName(pct, muted)
		a.showOSD(icon, "Volume", float64(pct))
	}

	if wpctlPath != "" {
		_ = exec.Command(wpctlPath, "set-mute", "@DEFAULT_AUDIO_SINK@", "toggle").Run()
		return
	}
	if pactlPath != "" {
		_ = exec.Command(pactlPath, "set-sink-mute", "@DEFAULT_SINK@", "toggle").Run()
	}
}

func (a *Audio) listenAudioEvents(ctx context.Context) {
	if pactlPath == "" {
		return
	}

	for {
		if ctx.Err() != nil {
			return
		}

		cmd := exec.CommandContext(ctx, pactlPath, "subscribe")
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
				continue
			}
		}

		if err := cmd.Start(); err != nil {
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
				continue
			}
		}

		stopCh := make(chan struct{})
		go func() {
			select {
			case <-ctx.Done():
			case <-stopCh:
			}
			if stdout != nil {
				_ = stdout.Close()
			}
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
		}()

		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			fields := strings.Fields(line)
			for i, f := range fields {
				if f == "on" && i+1 < len(fields) {
					fac := fields[i+1]
					if fac == "sink" || fac == "server" {
						a.mu.Lock()
						inLockout := time.Since(a.lastUserVol) < 500*time.Millisecond
						a.mu.Unlock()
						if inLockout {
							break
						}
						vol, muted, err := queryAudioState()
						if err == nil {
							a.updateUI(vol, muted)
						}
					}
					break
				}
			}
		}

		close(stopCh)
		_ = cmd.Wait()

		select {
		case <-ctx.Done():
			return
		case <-time.After(1 * time.Second):
		}
	}
}

func queryAudioState() (float64, bool, error) {
	if wpctlPath != "" {
		out, err := exec.Command(wpctlPath, "get-volume", "@DEFAULT_AUDIO_SINK@").Output()
		if err == nil {
			vol, muted, err := parseWpctlVolume(string(out))
			if err == nil {
				return vol, muted, nil
			}
		}
	}

	if pactlPath != "" {
		volOut, err := exec.Command(pactlPath, "get-sink-volume", "@DEFAULT_SINK@").Output()
		if err == nil {
			vol, err := parsePactlVolume(string(volOut))
			if err == nil {
				muteOut, _ := exec.Command(pactlPath, "get-sink-mute", "@DEFAULT_SINK@").Output()
				muted := strings.Contains(strings.ToLower(string(muteOut)), "mute: yes")
				return vol, muted, nil
			}
		}
	}

	return 0, false, fmt.Errorf("no audio tool available")
}

func parseWpctlVolume(out string) (float64, bool, error) {
	trimmed := strings.TrimSpace(out)
	if !strings.HasPrefix(trimmed, "Volume:") {
		return 0, false, fmt.Errorf("unexpected wpctl output: %q", out)
	}

	muted := strings.Contains(trimmed, "[MUTED]")
	cleaned := strings.TrimPrefix(trimmed, "Volume:")
	cleaned = strings.ReplaceAll(cleaned, "[MUTED]", "")
	cleaned = strings.TrimSpace(cleaned)

	val, err := strconv.ParseFloat(cleaned, 64)
	if err != nil {
		return 0, false, err
	}
	return math.Round(val*10000.0) / 100.0, muted, nil
}

func parsePactlVolume(out string) (float64, error) {
	m := rePercent.FindStringSubmatch(out)
	if len(m) < 2 {
		return 0, fmt.Errorf("no percentage in pactl output: %q", out)
	}
	return strconv.ParseFloat(m[1], 64)
}
