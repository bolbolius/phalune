package battery

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"phalune/internal/widget"
	"phalune/ui"

	"github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

type Battery struct {
	box           *gtk.Box
	icon          *gtk.Image
	label         *gtk.Label
	cancel        context.CancelFunc
	batteryPath   string
	lowThreshold  int
	fullThreshold int
	pollInterval  time.Duration
	format        string
}

func New(ctx widget.Context) (widget.Widget, error) {
	builder := gtk.NewBuilderFromString(ui.Battery)
	box := builder.GetObject("battery_box").Cast().(*gtk.Box)
	icon := builder.GetObject("battery_icon").Cast().(*gtk.Image)
	label := builder.GetObject("battery_label").Cast().(*gtk.Label)

	batPath := findBatteryPath()
	if batPath == "" {
		// Desktop or environment with no battery; gracefully hide widget
		box.SetVisible(false)
	}

	lowThresh := ctx.Config.Bar.Battery.LowThreshold
	if lowThresh <= 0 {
		lowThresh = 15
	}
	fullThresh := ctx.Config.Bar.Battery.FullThreshold
	if fullThresh <= 0 {
		fullThresh = 98
	}
	pollInt := ctx.Config.Bar.Battery.PollInterval.Duration
	if pollInt <= 0 {
		pollInt = 30 * time.Second
	}
	format := ctx.Config.Bar.Battery.Format
	if format == "" {
		format = "%d%%"
	}

	bCtx, cancel := context.WithCancel(context.Background())
	b := &Battery{
		box:           box,
		icon:          icon,
		label:         label,
		cancel:        cancel,
		batteryPath:   batPath,
		lowThreshold:  lowThresh,
		fullThreshold: fullThresh,
		pollInterval:  pollInt,
		format:        format,
	}

	click := gtk.NewGestureClick()
	click.SetButton(1)
	click.ConnectReleased(func(n int, x, y float64) {
		if ctx.OpenControlCenterSubpage != nil {
			ctx.OpenControlCenterSubpage("power")
		}
	})
	box.AddController(click)

	if batPath != "" {
		b.refresh()
		go b.listenEvents(bCtx)
	}

	return b, nil
}

func (b *Battery) Root() gtk.Widgetter {
	return b.box
}

func (b *Battery) Destroy() {
	if b.cancel != nil {
		b.cancel()
		b.cancel = nil
	}
}

func (b *Battery) refresh() {
	if b.batteryPath == "" {
		return
	}

	capVal, status, err := readBatteryInfo(b.batteryPath)
	if err != nil {
		return
	}

	iconName := BatteryIconNameWithThreshold(capVal, status, b.fullThreshold)

	glib.IdleAdd(func() {
		b.icon.SetFromIconName(iconName)
		b.label.SetText(fmt.Sprintf(b.format, capVal))

		if status == "Charging" {
			b.box.AddCSSClass("charging")
			b.box.RemoveCSSClass("low")
		} else {
			b.box.RemoveCSSClass("charging")
			if capVal <= b.lowThreshold {
				b.box.AddCSSClass("low")
			} else {
				b.box.RemoveCSSClass("low")
			}
		}
	})
}

func (b *Battery) listenEvents(ctx context.Context) {
	// Zero-polling Netlink KOBJECT_UEVENT listener for instant power supply notifications
	go func() {
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
		file := os.NewFile(uintptr(fd), "netlink_power")
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
			if strings.Contains(msg, "SUBSYSTEM=power_supply") {
				b.refresh()
			}
		}
	}()

	// Low-frequency tick to refresh slowly draining percentage between kernel uevents
	ticker := time.NewTicker(b.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			b.refresh()
		}
	}
}

func findBatteryPath() string {
	matches, err := filepath.Glob("/sys/class/power_supply/BAT*")
	if err == nil && len(matches) > 0 {
		return matches[0]
	}

	supplies, err := filepath.Glob("/sys/class/power_supply/*")
	if err == nil {
		for _, s := range supplies {
			typeData, err := os.ReadFile(filepath.Join(s, "type"))
			if err == nil && strings.TrimSpace(string(typeData)) == "Battery" {
				return s
			}
		}
	}

	return ""
}

func readBatteryInfo(batPath string) (int, string, error) {
	capData, err := os.ReadFile(filepath.Join(batPath, "capacity"))
	if err != nil {
		return 0, "", err
	}
	capVal, err := strconv.Atoi(strings.TrimSpace(string(capData)))
	if err != nil {
		return 0, "", err
	}

	status := "Discharging"
	statusData, err := os.ReadFile(filepath.Join(batPath, "status"))
	if err == nil {
		status = strings.TrimSpace(string(statusData))
	}

	if capVal < 0 {
		capVal = 0
	} else if capVal > 100 {
		capVal = 100
	}

	return capVal, status, nil
}

func BatteryIconName(pct int, status string) string {
	return BatteryIconNameWithThreshold(pct, status, 98)
}

func BatteryIconNameWithThreshold(pct int, status string, fullThreshold int) string {
	isCharging := status == "Charging"
	if pct >= fullThreshold {
		if isCharging {
			return "battery-level-100-charged-symbolic"
		}
		return "battery-level-100-symbolic"
	}

	level := (pct / 10) * 10
	if level < 0 {
		level = 0
	}

	if isCharging {
		return fmt.Sprintf("battery-level-%d-charging-symbolic", level)
	}

	return fmt.Sprintf("battery-level-%d-symbolic", level)
}
