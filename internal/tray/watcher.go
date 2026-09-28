package tray

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
	"github.com/godbus/dbus/v5/prop"
)

const (
	WatcherBusName   = "org.kde.StatusNotifierWatcher"
	WatcherPath      = "/StatusNotifierWatcher"
	WatcherInterface = "org.kde.StatusNotifierWatcher"
	ItemInterface    = "org.kde.StatusNotifierItem"
)

const watcherIntrospectionXML = `
<node>
  <interface name="org.kde.StatusNotifierWatcher">
    <method name="RegisterStatusNotifierItem">
      <arg name="service" type="s" direction="in"/>
    </method>
    <method name="RegisterStatusNotifierHost">
      <arg name="service" type="s" direction="in"/>
    </method>
    <property name="RegisteredStatusNotifierItems" type="as" access="read"/>
    <property name="IsStatusNotifierHostRegistered" type="b" access="read"/>
    <property name="ProtocolVersion" type="i" access="read"/>
    <signal name="StatusNotifierItemRegistered">
      <arg type="s"/>
    </signal>
    <signal name="StatusNotifierItemUnregistered">
      <arg type="s"/>
    </signal>
    <signal name="StatusNotifierHostRegistered"/>
  </interface>
  <interface name="org.freedesktop.DBus.Introspectable">
    <method name="Introspect">
      <arg name="data" type="s" direction="out"/>
    </method>
  </interface>
</node>
`

type watcherService struct {
	mgr   *Manager
	props *prop.Properties
}

func newManager() (*Manager, error) {
	conn, err := dbus.SessionBus()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to session bus: %w", err)
	}

	mgr := &Manager{
		conn:  conn,
		items: make(map[string]*TrayItem),
	}

	// Try claiming org.kde.StatusNotifierWatcher
	reply, err := conn.RequestName(WatcherBusName, dbus.NameFlagDoNotQueue)
	if err == nil && reply == dbus.RequestNameReplyPrimaryOwner {
		mgr.isWatcher = true
		if err := mgr.setupWatcherServer(); err != nil {
			slog.Warn("failed to initialize StatusNotifierWatcher server", "error", err)
		}
	} else {
		slog.Info("another StatusNotifierWatcher is running; registering as host")
		_ = mgr.setupHostClient()
	}

	go mgr.listenSignals()

	return mgr, nil
}

func (m *Manager) setupWatcherServer() error {
	svc := &watcherService{mgr: m}

	propsSpec := prop.Map{
		WatcherInterface: {
			"RegisteredStatusNotifierItems": {
				Value:    []string{},
				Writable: false,
				Emit:     prop.EmitTrue,
			},
			"IsStatusNotifierHostRegistered": {
				Value:    true,
				Writable: false,
				Emit:     prop.EmitTrue,
			},
			"ProtocolVersion": {
				Value:    int32(0),
				Writable: false,
				Emit:     prop.EmitTrue,
			},
		},
	}

	properties, err := prop.Export(m.conn, WatcherPath, propsSpec)
	if err != nil {
		return fmt.Errorf("failed to export properties: %w", err)
	}
	svc.props = properties

	if err := m.conn.Export(svc, WatcherPath, WatcherInterface); err != nil {
		return fmt.Errorf("failed to export watcher interface: %w", err)
	}

	introNode := &introspect.Node{
		Name: WatcherPath,
		Interfaces: []introspect.Interface{
			introspect.IntrospectData,
			prop.IntrospectData,
			{
				Name:    WatcherInterface,
				Methods: introspect.Methods(svc),
				Signals: []introspect.Signal{
					{
						Name: "StatusNotifierItemRegistered",
						Args: []introspect.Arg{{Type: "s"}},
					},
					{
						Name: "StatusNotifierItemUnregistered",
						Args: []introspect.Arg{{Type: "s"}},
					},
					{
						Name: "StatusNotifierHostRegistered",
					},
				},
			},
		},
	}

	if err := m.conn.Export(introspect.NewIntrospectable(introNode), WatcherPath, "org.freedesktop.DBus.Introspectable"); err != nil {
		return fmt.Errorf("failed to export introspectable: %w", err)
	}

	// Match NameOwnerChanged so we detect when items exit
	_ = m.conn.BusObject().Call("org.freedesktop.DBus.AddMatch", 0,
		"type='signal',sender='org.freedesktop.DBus',interface='org.freedesktop.DBus',member='NameOwnerChanged'")

	return nil
}

func (m *Manager) setupHostClient() error {
	obj := m.conn.Object(WatcherBusName, WatcherPath)
	call := obj.Call(WatcherInterface+".RegisterStatusNotifierHost", 0, m.conn.Names()[0])
	if call.Err != nil {
		return call.Err
	}

	// Subscribe to Watcher signals
	_ = m.conn.BusObject().Call("org.freedesktop.DBus.AddMatch", 0,
		"type='signal',interface='org.kde.StatusNotifierWatcher'")

	// Fetch existing items
	variant, err := obj.GetProperty(WatcherInterface + ".RegisteredStatusNotifierItems")
	if err == nil {
		if items, ok := variant.Value().([]string); ok {
			for _, it := range items {
				m.registerItem(it)
			}
		}
	}

	return nil
}

func (s *watcherService) RegisterStatusNotifierItem(service string, sender dbus.Sender) *dbus.Error {
	normalized := NormalizeItemService(service, string(sender))
	s.mgr.registerItem(normalized)
	s.updateRegisteredProperty()
	_ = s.mgr.conn.Emit(WatcherPath, WatcherInterface+".StatusNotifierItemRegistered", normalized)
	return nil
}

func (s *watcherService) RegisterStatusNotifierHost(service string) *dbus.Error {
	_ = s.mgr.conn.Emit(WatcherPath, WatcherInterface+".StatusNotifierHostRegistered")
	return nil
}

func (s *watcherService) updateRegisteredProperty() {
	if s.props == nil {
		return
	}
	s.mgr.mu.RLock()
	var items []string
	for k := range s.mgr.items {
		items = append(items, k)
	}
	s.mgr.mu.RUnlock()
	_ = s.props.Set(WatcherInterface, "RegisteredStatusNotifierItems", dbus.MakeVariant(items))
}

func NormalizeItemService(service, sender string) string {
	if strings.HasPrefix(service, "/") {
		return sender + service
	}
	if !strings.Contains(service, "/") {
		return service + "/StatusNotifierItem"
	}
	return service
}

func (m *Manager) registerItem(service string) {
	idx := strings.Index(service, "/")
	if idx == -1 {
		return
	}
	busName := service[:idx]
	path := dbus.ObjectPath(service[idx:])

	m.mu.Lock()
	if _, exists := m.items[service]; exists {
		m.mu.Unlock()
		return
	}
	item := &TrayItem{
		Key:     service,
		BusName: busName,
		Path:    path,
	}
	m.items[service] = item
	m.mu.Unlock()

	// Listen for signals from this item
	_ = m.conn.BusObject().Call("org.freedesktop.DBus.AddMatch", 0,
		fmt.Sprintf("type='signal',sender='%s',interface='%s'", busName, ItemInterface))

	m.refreshItem(item)
	m.notifyAdded(item)
}

func (m *Manager) unregisterItem(service string) {
	m.mu.Lock()
	item, ok := m.items[service]
	if !ok {
		m.mu.Unlock()
		return
	}
	delete(m.items, service)
	m.mu.Unlock()

	if m.isWatcher {
		_ = m.conn.Emit(WatcherPath, WatcherInterface+".StatusNotifierItemUnregistered", service)
	}
	m.notifyRemoved(item.Key)
}

func (m *Manager) refreshItem(item *TrayItem) {
	obj := m.conn.Object(item.BusName, item.Path)

	if id, err := obj.GetProperty(ItemInterface + ".Id"); err == nil {
		if s, ok := id.Value().(string); ok {
			item.Id = s
		}
	}
	if title, err := obj.GetProperty(ItemInterface + ".Title"); err == nil {
		if s, ok := title.Value().(string); ok {
			item.Title = s
		}
	}
	if status, err := obj.GetProperty(ItemInterface + ".Status"); err == nil {
		if s, ok := status.Value().(string); ok {
			item.Status = s
		}
	}
	if iconName, err := obj.GetProperty(ItemInterface + ".IconName"); err == nil {
		if s, ok := iconName.Value().(string); ok {
			item.IconName = s
		}
	}
	if themePath, err := obj.GetProperty(ItemInterface + ".IconThemePath"); err == nil {
		if s, ok := themePath.Value().(string); ok {
			item.IconThemePath = s
		}
	}
	if menu, err := obj.GetProperty(ItemInterface + ".Menu"); err == nil {
		if p, ok := menu.Value().(dbus.ObjectPath); ok {
			item.MenuPath = p
		}
	}
	if isMenu, err := obj.GetProperty(ItemInterface + ".ItemIsMenu"); err == nil {
		if b, ok := isMenu.Value().(bool); ok {
			item.ItemIsMenu = b
		}
	}

	if pixmaps, err := obj.GetProperty(ItemInterface + ".IconPixmap"); err == nil {
		item.IconPixmaps = parsePixmaps(pixmaps.Value())
	}
}

func parsePixmaps(val interface{}) []Pixmap {
	var list []Pixmap
	// dbus unmarshals a(iiay) as []interface{} or []struct
	rows, ok := val.([]interface{})
	if !ok {
		return list
	}
	for _, r := range rows {
		if slice, ok := r.([]interface{}); ok && len(slice) >= 3 {
			w, _ := slice[0].(int32)
			h, _ := slice[1].(int32)
			data, _ := slice[2].([]byte)
			if w > 0 && h > 0 && len(data) > 0 {
				list = append(list, Pixmap{Width: w, Height: h, Data: data})
			}
		}
	}
	return list
}

func (m *Manager) listenSignals() {
	ch := make(chan *dbus.Signal, 64)
	m.conn.Signal(ch)

	for sig := range ch {
		if sig == nil {
			return
		}

		switch sig.Name {
		case "org.freedesktop.DBus.NameOwnerChanged":
			if len(sig.Body) >= 3 {
				name, _ := sig.Body[0].(string)
				newOwner, _ := sig.Body[2].(string)
				if newOwner == "" {
					// Check if any items belong to this bus name
					m.mu.RLock()
					var toRemove []string
					for k, it := range m.items {
						if it.BusName == name {
							toRemove = append(toRemove, k)
						}
					}
					m.mu.RUnlock()

					for _, k := range toRemove {
						m.unregisterItem(k)
					}
				}
			}

		case WatcherInterface + ".StatusNotifierItemRegistered":
			if len(sig.Body) >= 1 {
				if s, ok := sig.Body[0].(string); ok {
					m.registerItem(s)
				}
			}

		case WatcherInterface + ".StatusNotifierItemUnregistered":
			if len(sig.Body) >= 1 {
				if s, ok := sig.Body[0].(string); ok {
					m.unregisterItem(s)
				}
			}

		case ItemInterface + ".NewIcon",
			ItemInterface + ".NewTitle",
			ItemInterface + ".NewStatus",
			ItemInterface + ".NewMenu",
			ItemInterface + ".NewToolTip":
			sender := sig.Sender
			m.mu.RLock()
			var matched *TrayItem
			for _, it := range m.items {
				if it.BusName == sender {
					matched = it
					break
				}
			}
			m.mu.RUnlock()

			if matched != nil {
				m.refreshItem(matched)
				m.notifyUpdated(matched)
			}
		}
	}
}
