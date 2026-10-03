package controlcenter

import (
	"bufio"
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/godbus/dbus/v5"
)

var (
	rePercent    = regexp.MustCompile(`(\d+)%`)
	wpctlPath, _ = exec.LookPath("wpctl")
	pactlPath, _ = exec.LookPath("pactl")
)

type SlidersController struct {
	mu sync.Mutex

	volBinding *SliderBinding
	briBinding *SliderBinding

	backlightDev  string
	backlightName string
	maxBrightness int64
	systemBus     *dbus.Conn
	showOSD       func(icon, label string, value float64)
}

func (sc *SlidersController) SetShowOSD(fn func(icon, label string, value float64)) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.showOSD = fn
}

func NewSlidersController(
	volBtn *gtk.Button,
	volIcon *gtk.Image,
	volScale *gtk.Scale,
	volLabel *gtk.Label,
	briBtn *gtk.Button,
	briIcon *gtk.Image,
	briScale *gtk.Scale,
	briLabel *gtk.Label,
) *SlidersController {
	sc := &SlidersController{}

	sc.volBinding = NewSliderBinding(SliderConfig{
		Scale:   volScale,
		Label:   volLabel,
		Button:  volBtn,
		Icon:    volIcon,
		OnApply: sc.setSystemVolume,
		OnMute:  sc.toggleSystemMute,
		GetIcon: func(pct int, muted bool) string {
			if muted || pct <= 0 {
				return "audio-volume-muted-symbolic"
			} else if pct < 33 {
				return "audio-volume-low-symbolic"
			} else if pct < 66 {
				return "audio-volume-medium-symbolic"
			}
			return "audio-volume-high-symbolic"
		},
		OnNotify: func(pct int) {
			sc.mu.Lock()
			fn := sc.showOSD
			sc.mu.Unlock()
			if fn != nil {
				var icon string
				if pct <= 0 {
					icon = "audio-volume-muted-symbolic"
				} else if pct < 33 {
					icon = "audio-volume-low-symbolic"
				} else if pct < 66 {
					icon = "audio-volume-medium-symbolic"
				} else {
					icon = "audio-volume-high-symbolic"
				}
				fn(icon, "Volume", float64(pct))
			}
		},
	})

	sc.briBinding = NewSliderBinding(SliderConfig{
		Scale:   briScale,
		Label:   briLabel,
		Button:  briBtn,
		Icon:    briIcon,
		OnApply: sc.setSystemBrightness,
		GetIcon: func(pct int, muted bool) string {
			return "display-brightness-symbolic"
		},
		OnNotify: func(pct int) {
			sc.mu.Lock()
			fn := sc.showOSD
			sc.mu.Unlock()
			if fn != nil {
				fn("display-brightness-symbolic", "Brightness", float64(pct))
			}
		},
	})

	sc.initBacklight()
	return sc
}

func (sc *SlidersController) BindSubpageOutput(btn *gtk.Button, icon *gtk.Image, scale *gtk.Scale, label *gtk.Label) {
	sc.volBinding.BindClone(scale, label, btn, icon)
}

func (sc *SlidersController) isUserAdjustingVol() bool {
	return sc.volBinding.IsUserAdjusting()
}

func (sc *SlidersController) isUserAdjustingBri() bool {
	return sc.briBinding.IsUserAdjusting()
}

func (sc *SlidersController) initBacklight() {
	matches, err := filepath.Glob("/sys/class/backlight/*")
	if err == nil && len(matches) > 0 {
		sc.backlightDev = matches[0]
		// Prefer native driver over acpi_video
		for _, m := range matches {
			if !strings.HasPrefix(filepath.Base(m), "acpi_video") {
				sc.backlightDev = m
				break
			}
		}
		sc.backlightName = filepath.Base(sc.backlightDev)

		maxPath := filepath.Join(sc.backlightDev, "max_brightness")
		if data, err := os.ReadFile(maxPath); err == nil {
			sc.maxBrightness, _ = strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
		}
	}
}

func (sc *SlidersController) Start(ctx context.Context) {
	conn, err := dbus.SystemBus()
	if err == nil {
		sc.mu.Lock()
		sc.systemBus = conn
		sc.mu.Unlock()
	}

	// Read initial values
	sc.syncInitialValues()

	sc.volBinding.StartWorker(ctx)
	sc.briBinding.StartWorker(ctx)

	// Zero-polling audio listener via pactl subscribe
	go sc.listenAudioEvents(ctx)

	// Zero-polling backlight listener via Netlink KOBJECT_UEVENT
	go sc.listenBacklightEvents(ctx)
}

func (sc *SlidersController) Sync() {
	go sc.syncInitialValues()
}

func (sc *SlidersController) syncInitialValues() {
	vol, muted, err := queryAudioState()
	if err == nil {
		glib.IdleAdd(func() {
			sc.updateVolumeUI(vol, muted)
		})
	}

	if sc.backlightDev != "" {
		bri, err := readBacklightPercent(sc.backlightDev)
		if err == nil {
			glib.IdleAdd(func() {
				sc.updateBrightnessUI(bri)
			})
		}
	}
}

func (sc *SlidersController) updateVolumeUI(volume float64, muted bool) {
	pct := int(math.Round(volume))
	sc.volBinding.SetValue(pct, muted)
}

func (sc *SlidersController) updateBrightnessUI(percent float64) {
	pct := int(math.Round(percent))
	sc.briBinding.SetValue(pct, false)
}

func (sc *SlidersController) setSystemVolume(pct int) {
	if wpctlPath != "" {
		volVal := fmt.Sprintf("%.2f", float64(pct)/100.0)
		_ = exec.Command(wpctlPath, "set-volume", "@DEFAULT_AUDIO_SINK@", volVal).Run()
		return
	}
	if pactlPath != "" {
		volVal := fmt.Sprintf("%d%%", pct)
		_ = exec.Command(pactlPath, "set-sink-volume", "@DEFAULT_SINK@", volVal).Run()
	}
}

func (sc *SlidersController) toggleSystemMute() {
	if wpctlPath != "" {
		_ = exec.Command(wpctlPath, "set-mute", "@DEFAULT_AUDIO_SINK@", "toggle").Run()
	} else if pactlPath != "" {
		_ = exec.Command(pactlPath, "set-sink-mute", "@DEFAULT_SINK@", "toggle").Run()
	}

	vol, muted, err := queryAudioState()
	if err == nil {
		pct := int(math.Round(vol))
		glib.IdleAdd(func() {
			sc.updateVolumeUI(vol, muted)
		})
		if sc.showOSD != nil {
			if muted {
				sc.showOSD("audio-volume-muted-symbolic", "Volume", 0)
			} else {
				var icon string
				if pct <= 0 {
					icon = "audio-volume-muted-symbolic"
				} else if pct < 33 {
					icon = "audio-volume-low-symbolic"
				} else if pct < 66 {
					icon = "audio-volume-medium-symbolic"
				} else {
					icon = "audio-volume-high-symbolic"
				}
				sc.showOSD(icon, "Volume", float64(pct))
			}
		}
	}
}

func (sc *SlidersController) setSystemBrightness(pct int) {
	if sc.maxBrightness <= 0 || sc.backlightName == "" {
		return
	}

	target := uint32(math.Round(float64(pct) / 100.0 * float64(sc.maxBrightness)))
	if target == 0 && pct > 0 {
		target = 1
	}

	// Direct sysfs write if accessible
	briFile := filepath.Join(sc.backlightDev, "brightness")
	if err := os.WriteFile(briFile, []byte(strconv.FormatUint(uint64(target), 10)), 0644); err == nil {
		return
	}

	sc.mu.Lock()
	bus := sc.systemBus
	sc.mu.Unlock()

	if bus != nil {
		obj := bus.Object("org.freedesktop.login1", "/org/freedesktop/login1/session/self")
		_ = obj.Call("org.freedesktop.login1.Session.SetBrightness", 0, "backlight", sc.backlightName, target)
	}
}

func (sc *SlidersController) listenAudioEvents(ctx context.Context) {
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
						// Drop event if user is actively adjusting volume
						if sc.isUserAdjustingVol() {
							break
						}

						vol, muted, err := queryAudioState()
						if err == nil {
							glib.IdleAdd(func() {
								sc.updateVolumeUI(vol, muted)
							})
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

func (sc *SlidersController) listenBacklightEvents(ctx context.Context) {
	fd, err := syscall.Socket(syscall.AF_NETLINK, syscall.SOCK_DGRAM, syscall.NETLINK_KOBJECT_UEVENT)
	if err != nil {
		return
	}

	addr := &syscall.SockaddrNetlink{
		Family: syscall.AF_NETLINK,
		Groups: 1, // kernel broadcast group
	}
	if err := syscall.Bind(fd, addr); err != nil {
		_ = syscall.Close(fd)
		return
	}

	_ = syscall.SetNonblock(fd, true)

	file := os.NewFile(uintptr(fd), "netlink")
	defer file.Close()

	go func() {
		<-ctx.Done()
		_ = file.Close()
	}()

	buf := make([]byte, 4096)
	for {
		n, err := file.Read(buf)
		if err != nil {
			return
		}

		msg := string(buf[:n])
		if strings.Contains(msg, "SUBSYSTEM=backlight") {
			if sc.isUserAdjustingBri() {
				continue
			}
			if sc.backlightDev != "" {
				pct, err := readBacklightPercent(sc.backlightDev)
				if err == nil {
					glib.IdleAdd(func() {
						sc.updateBrightnessUI(pct)
					})
				}
			}
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
	return val * 100.0, muted, nil
}

func parsePactlVolume(out string) (float64, error) {
	m := rePercent.FindStringSubmatch(out)
	if len(m) < 2 {
		return 0, fmt.Errorf("no percentage in pactl output: %q", out)
	}
	return strconv.ParseFloat(m[1], 64)
}

func readBacklightPercent(dev string) (float64, error) {
	curData, err := os.ReadFile(filepath.Join(dev, "brightness"))
	if err != nil {
		return 0, err
	}
	cur, err := strconv.ParseInt(strings.TrimSpace(string(curData)), 10, 64)
	if err != nil {
		return 0, err
	}

	maxData, err := os.ReadFile(filepath.Join(dev, "max_brightness"))
	if err != nil {
		return 0, err
	}
	maxVal, err := strconv.ParseInt(strings.TrimSpace(string(maxData)), 10, 64)
	if err != nil || maxVal <= 0 {
		return 0, err
	}

	pct := math.Round((float64(cur) / float64(maxVal)) * 100.0)
	if pct < 0 {
		pct = 0
	} else if pct > 100 {
		pct = 100
	}
	return pct, nil
}
