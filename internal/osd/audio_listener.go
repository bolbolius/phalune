package osd

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
)

var (
	rePactlPercent = regexp.MustCompile(`(\d+)%`)
	wpctlPath, _   = exec.LookPath("wpctl")
	pactlPath, _   = exec.LookPath("pactl")
)

type audioState struct {
	volume int
	muted  bool
}

func parseWpctlVolume(out string) (float64, bool, error) {
	// e.g. "Volume: 0.30" or "Volume: 0.30 [MUTED]"
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
	return val * 100.0, muted, nil
}

func parsePactlVolume(out string) (float64, error) {
	m := rePactlPercent.FindStringSubmatch(out)
	if len(m) < 2 {
		return 0, fmt.Errorf("no percentage in pactl output: %q", out)
	}
	return strconv.ParseFloat(m[1], 64)
}

func parsePactlMute(out string) bool {
	return strings.Contains(strings.ToLower(out), "mute: yes")
}

func querySink() (audioState, error) {
	if wpctlPath != "" {
		out, err := exec.Command(wpctlPath, "get-volume", "@DEFAULT_AUDIO_SINK@").Output()
		if err == nil {
			vol, muted, err := parseWpctlVolume(string(out))
			if err == nil {
				return audioState{volume: int(math.Round(vol)), muted: muted}, nil
			}
		}
	}

	if pactlPath != "" {
		volOut, err := exec.Command(pactlPath, "get-sink-volume", "@DEFAULT_SINK@").Output()
		if err == nil {
			vol, err := parsePactlVolume(string(volOut))
			if err == nil {
				muteOut, _ := exec.Command(pactlPath, "get-sink-mute", "@DEFAULT_SINK@").Output()
				muted := parsePactlMute(string(muteOut))
				return audioState{volume: int(math.Round(vol)), muted: muted}, nil
			}
		}
	}

	return audioState{}, fmt.Errorf("no audio tool available to query sink")
}

func querySource() (audioState, error) {
	if wpctlPath != "" {
		out, err := exec.Command(wpctlPath, "get-volume", "@DEFAULT_AUDIO_SOURCE@").Output()
		if err == nil {
			vol, muted, err := parseWpctlVolume(string(out))
			if err == nil {
				return audioState{volume: int(math.Round(vol)), muted: muted}, nil
			}
		}
	}

	if pactlPath != "" {
		volOut, err := exec.Command(pactlPath, "get-source-volume", "@DEFAULT_SOURCE@").Output()
		if err == nil {
			vol, err := parsePactlVolume(string(volOut))
			if err == nil {
				muteOut, _ := exec.Command(pactlPath, "get-source-mute", "@DEFAULT_SOURCE@").Output()
				muted := parsePactlMute(string(muteOut))
				return audioState{volume: int(math.Round(vol)), muted: muted}, nil
			}
		}
	}

	return audioState{}, fmt.Errorf("no audio tool available to query source")
}

func parsePactlSubscribeFacility(line string) string {
	fields := strings.Fields(line)
	for i, f := range fields {
		if f == "on" && i+1 < len(fields) {
			return fields[i+1]
		}
	}
	return ""
}

func runAudioListener(ctx context.Context, o *OSD) {
	var (
		mu         sync.Mutex
		lastSink   audioState
		lastSource audioState
		hasSink    bool
		hasSource  bool
	)

	checkSink := func(initial bool) {
		if !initial && o.IsInDirectUserLockout() {
			return
		}

		st, err := querySink()
		if err != nil {
			return
		}

		if !initial && o.IsInDirectUserLockout() {
			return
		}

		mu.Lock()
		if !hasSink {
			hasSink = true
			lastSink = st
			mu.Unlock()
			if initial {
				return
			}
		} else {
			if st == lastSink {
				mu.Unlock()
				return
			}
			lastSink = st
			mu.Unlock()
		}

		if st.muted {
			o.ShowCustomExternal("audio-volume-muted-symbolic", "Volume", 0, "Muted")
		} else {
			var icon string
			if st.volume <= 0 {
				icon = "audio-volume-muted-symbolic"
			} else if st.volume < 33 {
				icon = "audio-volume-low-symbolic"
			} else if st.volume < 66 {
				icon = "audio-volume-medium-symbolic"
			} else {
				icon = "audio-volume-high-symbolic"
			}
			o.ShowExternal(icon, "Volume", float64(st.volume))
		}
	}

	checkSource := func(initial bool) {
		if !initial && o.IsInDirectUserLockout() {
			return
		}

		st, err := querySource()
		if err != nil {
			return
		}

		if !initial && o.IsInDirectUserLockout() {
			return
		}

		mu.Lock()
		if !hasSource {
			hasSource = true
			lastSource = st
			mu.Unlock()
			if initial {
				return
			}
		} else {
			if st == lastSource {
				mu.Unlock()
				return
			}
			lastSource = st
			mu.Unlock()
		}

		if st.muted {
			o.ShowCustomExternal("microphone-sensitivity-muted-symbolic", "Microphone", 0, "Muted")
		} else {
			o.ShowExternal("audio-input-microphone-symbolic", "Microphone", float64(st.volume))
		}
	}

	// Capture initial state without showing OSD popup
	checkSink(true)
	checkSource(true)

	sinkCh := make(chan struct{}, 1)
	sourceCh := make(chan struct{}, 1)

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-sinkCh:
				time.Sleep(10 * time.Millisecond)
				drained := false
				for !drained {
					select {
					case <-sinkCh:
					default:
						drained = true
					}
				}
				checkSink(false)
			}
		}
	}()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-sourceCh:
				time.Sleep(10 * time.Millisecond)
				drained := false
				for !drained {
					select {
					case <-sourceCh:
					default:
						drained = true
					}
				}
				checkSource(false)
			}
		}
	}()

	// Real-time event subscription via pactl subscribe (zero polling)
	for {
		if ctx.Err() != nil {
			return
		}

		cmd := exec.CommandContext(ctx, "pactl", "subscribe")
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
			switch parsePactlSubscribeFacility(line) {
			case "sink":
				select {
				case sinkCh <- struct{}{}:
				default:
				}
			case "source":
				select {
				case sourceCh <- struct{}{}:
				default:
				}
			case "server":
				select {
				case sinkCh <- struct{}{}:
				default:
				}
				select {
				case sourceCh <- struct{}{}:
				default:
				}
			}
		}

		close(stopCh)
		_ = cmd.Wait()

		// If pactl subscribe exited (e.g. audio server restart), wait before reconnecting
		select {
		case <-ctx.Done():
			return
		case <-time.After(1 * time.Second):
		}
	}
}
