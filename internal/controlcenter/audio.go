package controlcenter

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/diamondburned/gotk4/pkg/core/glib"
)

var (
	reSinkInputHeader = regexp.MustCompile(`(?i)Sink\s+Input\s+#(\d+)`)
	rePropLine        = regexp.MustCompile(`^\s*([a-zA-Z0-9._-]+)\s*=\s*"(.*)"\s*$`)
)

type AudioStream struct {
	ID        int
	AppName   string
	MediaName string
	IconName  string
	Volume    int
	Muted     bool
}

type AudioController struct {
	mu               sync.Mutex
	inputVolume      float64
	inputMuted       bool
	streams          []AudioStream
	onInputChanged   func(vol float64, muted bool)
	onStreamsChanged func(streams []AudioStream)

	refreshTimer *time.Timer
	refreshMu    sync.Mutex
}

func NewAudioController() *AudioController {
	return &AudioController{}
}

func (ac *AudioController) SetOnInputChanged(fn func(vol float64, muted bool)) {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	ac.onInputChanged = fn
}

func (ac *AudioController) SetOnStreamsChanged(fn func(streams []AudioStream)) {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	ac.onStreamsChanged = fn
}

func (ac *AudioController) Start(ctx context.Context) {
	ac.Refresh()
	go ac.listenEvents(ctx)
}

func (ac *AudioController) triggerDebouncedRefresh() {
	ac.refreshMu.Lock()
	defer ac.refreshMu.Unlock()
	if ac.refreshTimer != nil {
		ac.refreshTimer.Stop()
	}
	ac.refreshTimer = time.AfterFunc(75*time.Millisecond, func() {
		ac.Refresh()
	})
}

func (ac *AudioController) Refresh() {
	go func() {
		vol, muted, err := QueryInputAudioState()
		if err == nil {
			ac.mu.Lock()
			ac.inputVolume = vol
			ac.inputMuted = muted
			cb := ac.onInputChanged
			ac.mu.Unlock()

			if cb != nil {
				glib.IdleAdd(func() {
					cb(vol, muted)
				})
			}
		}

		streams := QuerySinkInputs()
		ac.mu.Lock()
		ac.streams = streams
		cbStreams := ac.onStreamsChanged
		ac.mu.Unlock()

		if cbStreams != nil {
			glib.IdleAdd(func() {
				cbStreams(streams)
			})
		}
	}()
}

func (ac *AudioController) SetInputVolume(pct int) {
	if pct < 0 {
		pct = 0
	} else if pct > 100 {
		pct = 100
	}

	if wpctlPath != "" {
		volVal := fmt.Sprintf("%.2f", float64(pct)/100.0)
		_ = exec.Command(wpctlPath, "set-volume", "@DEFAULT_AUDIO_SOURCE@", volVal).Run()
	} else if pactlPath != "" {
		volVal := fmt.Sprintf("%d%%", pct)
		_ = exec.Command(pactlPath, "set-source-volume", "@DEFAULT_SOURCE@", volVal).Run()
	}
}

func (ac *AudioController) ToggleInputMute() {
	go func() {
		if wpctlPath != "" {
			_ = exec.Command(wpctlPath, "set-mute", "@DEFAULT_AUDIO_SOURCE@", "toggle").Run()
		} else if pactlPath != "" {
			_ = exec.Command(pactlPath, "set-source-mute", "@DEFAULT_SOURCE@", "toggle").Run()
		}

		vol, muted, err := QueryInputAudioState()
		if err == nil {
			ac.mu.Lock()
			ac.inputVolume = vol
			ac.inputMuted = muted
			cb := ac.onInputChanged
			ac.mu.Unlock()

			if cb != nil {
				glib.IdleAdd(func() {
					cb(vol, muted)
				})
			}
		}
	}()
}

func (ac *AudioController) SetSinkInputVolume(id int, pct int) {
	if pactlPath == "" {
		return
	}
	if pct < 0 {
		pct = 0
	} else if pct > 100 {
		pct = 100
	}

	arg := fmt.Sprintf("%d%%", pct)
	_ = exec.Command(pactlPath, "set-sink-input-volume", strconv.Itoa(id), arg).Run()
}

func (ac *AudioController) ToggleSinkInputMute(id int) {
	if pactlPath == "" {
		return
	}

	_ = exec.Command(pactlPath, "set-sink-input-mute", strconv.Itoa(id), "toggle").Run()
	ac.triggerDebouncedRefresh()
}

func (ac *AudioController) listenEvents(ctx context.Context) {
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
			if strings.Contains(line, "sink-input") || strings.Contains(line, "source") {
				ac.triggerDebouncedRefresh()
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

func QueryInputAudioState() (float64, bool, error) {
	if wpctlPath != "" {
		out, err := exec.Command(wpctlPath, "get-volume", "@DEFAULT_AUDIO_SOURCE@").Output()
		if err == nil {
			vol, muted, err := parseWpctlVolume(string(out))
			if err == nil {
				return vol, muted, nil
			}
		}
	}

	if pactlPath != "" {
		volOut, err := exec.Command(pactlPath, "get-source-volume", "@DEFAULT_SOURCE@").Output()
		if err == nil {
			vol, err := parsePactlVolume(string(volOut))
			if err == nil {
				muteOut, _ := exec.Command(pactlPath, "get-source-mute", "@DEFAULT_SOURCE@").Output()
				muted := strings.Contains(strings.ToLower(string(muteOut)), "mute: yes")
				return vol, muted, nil
			}
		}
	}

	return 0, false, fmt.Errorf("no input audio tool available")
}

func QuerySinkInputs() []AudioStream {
	if pactlPath == "" {
		return nil
	}
	out, err := exec.Command(pactlPath, "list", "sink-inputs").Output()
	if err != nil {
		return nil
	}
	return ParseSinkInputs(string(out))
}

func ParseSinkInputs(raw string) []AudioStream {
	var streams []AudioStream
	lines := strings.Split(raw, "\n")

	var cur *AudioStream
	var curProps map[string]string

	finishCurrent := func() {
		if cur == nil {
			return
		}
		// Resolve app name
		if name, ok := curProps["application.name"]; ok && name != "" {
			cur.AppName = name
		} else if desc, ok := curProps["device.description"]; ok && desc != "" {
			cur.AppName = desc
		} else if node, ok := curProps["node.name"]; ok && node != "" {
			cur.AppName = node
		} else if bin, ok := curProps["application.process.binary"]; ok && bin != "" {
			cur.AppName = strings.Title(bin)
		} else if cur.MediaName != "" {
			cur.AppName = cur.MediaName
		} else {
			cur.AppName = fmt.Sprintf("Stream #%d", cur.ID)
		}

		// Resolve media name
		if m, ok := curProps["media.name"]; ok && m != "" && m != cur.AppName {
			cur.MediaName = m
		}

		// Resolve icon name
		if icon, ok := curProps["application.icon_name"]; ok && icon != "" {
			cur.IconName = icon
		} else if bin, ok := curProps["application.process.binary"]; ok && bin != "" {
			cur.IconName = strings.ToLower(bin)
		} else {
			cur.IconName = "audio-speakers-symbolic"
		}

		streams = append(streams, *cur)
		cur = nil
		curProps = nil
	}

	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if m := reSinkInputHeader.FindStringSubmatch(trim); len(m) > 1 {
			finishCurrent()
			id, _ := strconv.Atoi(m[1])
			cur = &AudioStream{
				ID:       id,
				Volume:   100,
				IconName: "audio-speakers-symbolic",
			}
			curProps = make(map[string]string)
			continue
		}

		if cur == nil {
			continue
		}

		if strings.HasPrefix(trim, "Mute:") {
			cur.Muted = strings.Contains(strings.ToLower(trim), "yes")
		} else if strings.HasPrefix(trim, "Volume:") {
			if m := rePercent.FindStringSubmatch(trim); len(m) > 1 {
				v, _ := strconv.Atoi(m[1])
				cur.Volume = v
			}
		} else if pm := rePropLine.FindStringSubmatch(line); len(pm) > 2 {
			curProps[pm[1]] = pm[2]
		}
	}

	finishCurrent()
	return streams
}

func InputVolumeIconName(pct int, muted bool) string {
	if muted || pct <= 0 {
		return "microphone-disabled-symbolic"
	}
	if pct < 33 {
		return "audio-input-microphone-symbolic"
	}
	if pct < 66 {
		return "audio-input-microphone-symbolic"
	}
	return "audio-input-microphone-symbolic"
}
