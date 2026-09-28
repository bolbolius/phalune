package tray

import (
	"sync"

	"github.com/godbus/dbus/v5"
)

type Pixmap struct {
	Width  int32
	Height int32
	Data   []byte
}

type TrayItem struct {
	Key           string          // e.g. ":1.50/StatusNotifierItem"
	BusName       string          // e.g. ":1.50"
	Path          dbus.ObjectPath // e.g. "/StatusNotifierItem"
	Id            string
	Title         string
	Status        string // Passive, Active, NeedsAttention
	IconName      string
	IconThemePath string
	IconPixmaps   []Pixmap
	MenuPath      dbus.ObjectPath
	ItemIsMenu    bool
}

type Listener interface {
	OnItemAdded(item *TrayItem)
	OnItemRemoved(key string)
	OnItemUpdated(item *TrayItem)
}

type Manager struct {
	mu        sync.RWMutex
	conn      *dbus.Conn
	items     map[string]*TrayItem
	listeners []Listener
	isWatcher bool
}

var (
	defaultMgr *Manager
	once       sync.Once
)

func GetManager() (*Manager, error) {
	var err error
	once.Do(func() {
		defaultMgr, err = newManager()
	})
	return defaultMgr, err
}

func (m *Manager) AddListener(l Listener) {
	m.mu.Lock()
	m.listeners = append(m.listeners, l)
	// Send current items snapshot to new listener
	items := make([]*TrayItem, 0, len(m.items))
	for _, it := range m.items {
		items = append(items, it)
	}
	m.mu.Unlock()

	for _, it := range items {
		l.OnItemAdded(it)
	}
}

func (m *Manager) RemoveListener(l Listener) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, existing := range m.listeners {
		if existing == l {
			m.listeners = append(m.listeners[:i], m.listeners[i+1:]...)
			break
		}
	}
}

func (m *Manager) Items() []*TrayItem {
	m.mu.RLock()
	defer m.mu.RUnlock()
	list := make([]*TrayItem, 0, len(m.items))
	for _, it := range m.items {
		list = append(list, it)
	}
	return list
}

func (m *Manager) notifyAdded(item *TrayItem) {
	m.mu.RLock()
	listeners := append([]Listener(nil), m.listeners...)
	m.mu.RUnlock()
	for _, l := range listeners {
		l.OnItemAdded(item)
	}
}

func (m *Manager) notifyRemoved(key string) {
	m.mu.RLock()
	listeners := append([]Listener(nil), m.listeners...)
	m.mu.RUnlock()
	for _, l := range listeners {
		l.OnItemRemoved(key)
	}
}

func (m *Manager) notifyUpdated(item *TrayItem) {
	m.mu.RLock()
	listeners := append([]Listener(nil), m.listeners...)
	m.mu.RUnlock()
	for _, l := range listeners {
		l.OnItemUpdated(item)
	}
}
