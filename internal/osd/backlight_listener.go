package osd

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
)

func readSysfsInt(path string) (int64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
}

func findBacklightDevice() string {
	matches, err := filepath.Glob("/sys/class/backlight/*")
	if err != nil || len(matches) == 0 {
		return ""
	}
	// Prefer native drivers over acpi firmware fallbacks
	for _, m := range matches {
		base := filepath.Base(m)
		if !strings.HasPrefix(base, "acpi_video") {
			return m
		}
	}
	return matches[0]
}

func findKbdBacklightDevice() string {
	patterns := []string{
		"/sys/class/leds/*::kbd_backlight",
		"/sys/class/leds/*kbd_backlight*",
		"/sys/class/leds/*kbd*",
	}
	for _, pat := range patterns {
		matches, err := filepath.Glob(pat)
		if err == nil && len(matches) > 0 {
			return matches[0]
		}
	}
	return ""
}

func findCapsLockDevices() []string {
	matches, err := filepath.Glob("/sys/class/leds/*capslock*")
	if err != nil {
		return nil
	}
	return matches
}

func readBacklightPercent(dev string) (float64, error) {
	cur, err := readSysfsInt(filepath.Join(dev, "brightness"))
	if err != nil {
		return 0, err
	}
	maxVal, err := readSysfsInt(filepath.Join(dev, "max_brightness"))
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

func readCapsLockState(devices []string) int64 {
	for _, dev := range devices {
		val, err := readSysfsInt(filepath.Join(dev, "brightness"))
		if err == nil && val > 0 {
			return 1
		}
	}
	return 0
}

func runBacklightListener(ctx context.Context, o *OSD) {
	backlightDev := findBacklightDevice()
	kbdDev := findKbdBacklightDevice()
	capsDevs := findCapsLockDevices()

	var (
		mu             sync.Mutex
		lastBrightness float64 = -1
		lastKbd        float64 = -1
		lastCaps       int64   = -1
	)

	checkBacklight := func(initial bool) {
		if backlightDev == "" {
			return
		}
		pct, err := readBacklightPercent(backlightDev)
		if err != nil {
			return
		}

		mu.Lock()
		defer mu.Unlock()

		if lastBrightness < 0 {
			lastBrightness = pct
			if initial {
				return
			}
		}

		if pct == lastBrightness {
			return
		}
		lastBrightness = pct

		o.Show("display-brightness-symbolic", "Brightness", pct)
	}

	checkKbd := func(initial bool) {
		if kbdDev == "" {
			return
		}
		pct, err := readBacklightPercent(kbdDev)
		if err != nil {
			return
		}

		mu.Lock()
		defer mu.Unlock()

		if lastKbd < 0 {
			lastKbd = pct
			if initial {
				return
			}
		}

		if pct == lastKbd {
			return
		}
		lastKbd = pct

		o.Show("keyboard-brightness-symbolic", "Keyboard", pct)
	}

	checkCaps := func(initial bool) {
		if len(capsDevs) == 0 {
			return
		}
		st := readCapsLockState(capsDevs)

		mu.Lock()
		defer mu.Unlock()

		if lastCaps < 0 {
			lastCaps = st
			if initial {
				return
			}
		}

		if st == lastCaps {
			return
		}
		lastCaps = st

		if st == 1 {
			o.ShowCustom("input-keyboard-symbolic", "Caps Lock", 100, "ON")
		} else {
			o.ShowCustom("input-keyboard-symbolic", "Caps Lock", 0, "OFF")
		}
	}

	// Capture initial state without showing OSD
	checkBacklight(true)
	checkKbd(true)
	checkCaps(true)

	// Listen for kernel netlink uevents (pure event subscription, zero polling)
	fd, err := syscall.Socket(syscall.AF_NETLINK, syscall.SOCK_RAW, syscall.NETLINK_KOBJECT_UEVENT)
	if err != nil {
		return
	}

	if err := syscall.Bind(fd, &syscall.SockaddrNetlink{Family: syscall.AF_NETLINK, Groups: 1}); err != nil {
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

	buf := make([]byte, 2048)
	for {
		n, err := file.Read(buf)
		if err != nil {
			return
		}
		msg := string(buf[:n])
		if strings.Contains(msg, "backlight") || strings.Contains(msg, "leds") {
			checkBacklight(false)
			checkKbd(false)
			checkCaps(false)
		}
	}
}
