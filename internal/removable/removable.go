package removable

import (
	"context"
	"strings"
	"sync"

	"phalune/internal/notify"

	"github.com/godbus/dbus/v5"
)

type Monitor struct {
	notifyMgr *notify.Manager
	conn      *dbus.Conn
	mu        sync.Mutex
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	known     map[dbus.ObjectPath]string
}

func NewMonitor(notifyMgr *notify.Manager) *Monitor {
	return &Monitor{
		notifyMgr: notifyMgr,
		known:     make(map[dbus.ObjectPath]string),
	}
}

func (m *Monitor) Start() {
	conn, err := dbus.SystemBus()
	if err != nil {
		return
	}
	m.conn = conn

	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel

	rule := "type='signal',sender='org.freedesktop.UDisks2',path_namespace='/org/freedesktop/UDisks2',interface='org.freedesktop.DBus.ObjectManager'"
	call := conn.BusObject().CallWithContext(ctx, "org.freedesktop.DBus.AddMatch", 0, rule)
	if call.Err != nil {
		return
	}

	sigChan := make(chan *dbus.Signal, 10)
	conn.Signal(sigChan)

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case sig, ok := <-sigChan:
				if !ok {
					return
				}
				if sig == nil {
					continue
				}

				switch {
				case strings.HasSuffix(sig.Name, "InterfacesAdded"):
					m.handleAdded(sig)
				case strings.HasSuffix(sig.Name, "InterfacesRemoved"):
					m.handleRemoved(sig)
				}
			}
		}
	}()
}

func (m *Monitor) handleAdded(sig *dbus.Signal) {
	if len(sig.Body) < 2 {
		return
	}
	path, ok := sig.Body[0].(dbus.ObjectPath)
	if !ok {
		return
	}

	interfaces, ok := sig.Body[1].(map[string]map[string]dbus.Variant)
	if !ok {
		return
	}

	driveProps, hasDrive := interfaces["org.freedesktop.UDisks2.Drive"]
	if hasDrive {
		removable, _ := driveProps["Removable"].Value().(bool)
		bus, _ := driveProps["ConnectionBus"].Value().(string)

		if removable || bus == "usb" {
			vendor, _ := driveProps["Vendor"].Value().(string)
			model, _ := driveProps["Model"].Value().(string)

			name := strings.TrimSpace(vendor + " " + model)
			if name == "" {
				name = "Removable Storage"
			}

			m.mu.Lock()
			m.known[path] = name
			m.mu.Unlock()

			if m.notifyMgr != nil {
				m.notifyMgr.Show(notify.Notification{
					AppName: "Device Manager",
					Summary: "Storage Connected",
					Body:    name,
					Icon:    "drive-removable-media-symbolic",
					Urgency: notify.UrgencyNormal,
				})
			}
		}
	}
}

func (m *Monitor) handleRemoved(sig *dbus.Signal) {
	if len(sig.Body) < 1 {
		return
	}
	path, ok := sig.Body[0].(dbus.ObjectPath)
	if !ok {
		return
	}

	m.mu.Lock()
	name, found := m.known[path]
	if found {
		delete(m.known, path)
	}
	m.mu.Unlock()

	if found && m.notifyMgr != nil {
		m.notifyMgr.Show(notify.Notification{
			AppName: "Device Manager",
			Summary: "Storage Disconnected",
			Body:    name,
			Icon:    "drive-removable-media-symbolic",
			Urgency: notify.UrgencyLow,
		})
	}
}

func (m *Monitor) Stop() {
	if m.cancel != nil {
		m.cancel()
	}
	m.wg.Wait()
}
