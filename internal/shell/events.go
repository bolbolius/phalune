package shell

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"phalune/internal/compositor"
	"phalune/internal/mpris"
)

// startEventEmitters wires internal shell state changes onto the IPC
// event bus so external programs can subscribe ("phalune msg subscribe").
// Emitted topics: workspaces, volume, brightness, microphone, mpris,
// notifications, battery.
func (s *Shell) startEventEmitters(ctx context.Context) {
	if s.eventBus == nil {
		return
	}

	s.emitCompositorEvents(ctx)
	s.emitOSDEvents()
	s.emitNotificationEvents(ctx)
	s.emitBatteryEvents(ctx)
	s.emitMprisEvents(ctx)
}

// emitMprisEvents mirrors the active media player state onto the bus.
func (s *Shell) emitMprisEvents(ctx context.Context) {
	if s.controlCenter == nil {
		return
	}
	ctrl := s.controlCenter.Mpris()
	if ctrl == nil {
		return
	}

	ctrl.OnChange(func(active *mpris.PlayerState) {
		if active == nil {
			s.publish("mpris", map[string]any{"status": "Stopped"})
			return
		}
		s.publish("mpris", map[string]any{
			"player": strings.TrimPrefix(active.BusName, "org.mpris.MediaPlayer2."),
			"title":  active.Title,
			"artist": active.Artist,
			"album":  active.Album,
			"status": string(active.Status),
		})
	})
}

// publish is safe from any goroutine.
func (s *Shell) publish(topic string, data any) {
	if s.eventBus != nil {
		s.eventBus.Publish(topic, data)
	}
}

// emitCompositorEvents forwards workspace changes from the compositor
// service onto the bus.
func (s *Shell) emitCompositorEvents(ctx context.Context) {
	if s.compositorSvc == nil {
		return
	}

	ch, unsub := s.compositorSvc.Subscribe()

	go func() {
		defer unsub()
		var lastKey string
		for {
			select {
			case <-ctx.Done():
				return
			case list, ok := <-ch:
				if !ok {
					return
				}
				key := workspaceListKey(list)
				if key == lastKey {
					continue
				}
				lastKey = key
				s.publish("workspaces", workspacesEventData(list))
			}
		}
	}()
}

func workspaceListKey(ws []compositor.Workspace) string {
	var b strings.Builder
	for _, w := range ws {
		fmt.Fprintf(&b, "%d:%t:%t:%t;", w.ID, w.IsActive, w.IsUrgent, w.IsFocused)
	}
	return b.String()
}

func workspacesEventData(list []compositor.Workspace) map[string]any {
	workspaces := make([]map[string]any, 0, len(list))
	var active map[string]any

	for _, w := range list {
		entry := map[string]any{
			"id":      w.ID,
			"name":    w.DisplayName(),
			"output":  w.Output,
			"active":  w.IsActive,
			"urgent":  w.IsUrgent,
			"focused": w.IsFocused,
		}
		workspaces = append(workspaces, entry)
		if w.IsActive && active == nil {
			active = entry
		}
	}

	data := map[string]any{"workspaces": workspaces}
	if active != nil {
		data["active"] = active
	}
	return data
}

// emitOSDEvents reuses the OSD listeners' volume/brightness/mic change
// detection; every OSD show call becomes a bus event.
func (s *Shell) emitOSDEvents() {
	if s.osdMgr == nil {
		return
	}
	s.osdMgr.SetEventHook(func(icon, label string, value float64, text string) {
		switch label {
		case "Volume":
			s.publish("volume", map[string]any{
				"volume": int(value),
				"muted":  text == "Muted",
			})
		case "Brightness":
			s.publish("brightness", map[string]any{"brightness": int(value)})
		case "Microphone":
			s.publish("microphone", map[string]any{
				"volume": int(value),
				"muted":  text == "Muted",
			})
		}
	})
}

// emitNotificationEvents forwards notification store changes onto the bus.
func (s *Shell) emitNotificationEvents(ctx context.Context) {
	if s.notifyMgr == nil {
		return
	}
	store := s.notifyMgr.Store()
	if store == nil {
		return
	}

	store.Subscribe(func() {
		items := store.All()
		entries := make([]map[string]any, 0, len(items))
		for _, it := range items {
			entries = append(entries, map[string]any{
				"id":       it.ID,
				"app_name": it.AppName,
				"summary":  it.Summary,
				"urgency":  int(it.Urgency),
			})
		}
		s.publish("notifications", map[string]any{
			"count": len(entries),
			"items": entries,
		})
	})
}

// emitBatteryEvents publishes battery percentage on power_supply uevents.
// It subscribes its own kernel socket: the battery bar widget shares the
// same subscription technique, and events are cheap one-shot reads.
func (s *Shell) emitBatteryEvents(ctx context.Context) {
	go batteryEventLoop(ctx, s.publish)
}

// batteryEventLoop reads sysfs on kernel uevents; one netlink subscriber
// process-wide per purpose (deduped against the bar widget's own loop is
// acceptable: reads are a few bytes each).
func batteryEventLoop(ctx context.Context, publish func(topic string, data any)) {
	batPath := findBatteryPath()
	if batPath == "" {
		return
	}

	fd, err := syscall.Socket(syscall.AF_NETLINK, syscall.SOCK_DGRAM, syscall.NETLINK_KOBJECT_UEVENT)
	if err != nil {
		return
	}

	addr := &syscall.SockaddrNetlink{Family: syscall.AF_NETLINK, Groups: 1}
	if err := syscall.Bind(fd, addr); err != nil {
		_ = syscall.Close(fd)
		return
	}

	_ = syscall.SetNonblock(fd, true)
	file := os.NewFile(uintptr(fd), "netlink_battery_events")
	go func() {
		<-ctx.Done()
		_ = file.Close()
	}()
	defer file.Close()

	buf := make([]byte, 4096)
	last := -1

	if pct, err := readBatteryCapacity(batPath); err == nil {
		last = pct
		publish("battery", map[string]any{"percentage": pct})
	}

	for {
		n, err := file.Read(buf)
		if err != nil {
			return
		}
		if !strings.Contains(string(buf[:n]), "SUBSYSTEM=power_supply") {
			continue
		}
		pct, err := readBatteryCapacity(batPath)
		if err != nil || pct == last {
			continue
		}
		last = pct
		publish("battery", map[string]any{"percentage": pct})
	}
}

// findBatteryPath locates the first battery in sysfs.
func findBatteryPath() string {
	matches, err := filepath.Glob("/sys/class/power_supply/BAT*")
	if err == nil && len(matches) > 0 {
		return matches[0]
	}
	return ""
}

func readBatteryCapacity(batPath string) (int, error) {
	data, err := os.ReadFile(filepath.Join(batPath, "capacity"))
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(data)))
}
