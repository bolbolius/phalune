package notify

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

const maxStoredNotifications = 100

// StoredItem is a serializable representation of a notification.
type StoredItem struct {
	ID        uint32    `json:"id"`
	AppName   string    `json:"app_name"`
	Summary   string    `json:"summary"`
	Body      string    `json:"body"`
	Icon      string    `json:"icon"`
	Urgency   Urgency   `json:"urgency"`
	Timestamp time.Time `json:"timestamp"`
	Actions   []Action  `json:"actions,omitempty"`
}

// GroupedNotifications groups notifications by app name.
type GroupedNotifications struct {
	AppName string
	Icon    string
	Items   []StoredItem
}

// Store manages notification history and disk persistence.
type Store struct {
	mu          sync.RWMutex
	filePath    string
	items       []StoredItem
	subscribers map[int]func()
	nextSubID   int
}

func defaultStorePath() string {
	cacheDir := os.Getenv("XDG_CACHE_HOME")
	if cacheDir == "" {
		home, _ := os.UserHomeDir()
		cacheDir = filepath.Join(home, ".cache")
	}
	return filepath.Join(cacheDir, "phalune", "notifications.json")
}

// NewStore initializes a notification store at the default or custom path.
func NewStore(customPath ...string) *Store {
	path := defaultStorePath()
	if len(customPath) > 0 && customPath[0] != "" {
		path = customPath[0]
	}

	s := &Store{
		filePath:    path,
		subscribers: make(map[int]func()),
	}
	_ = s.Load()
	return s
}

// Subscribe registers a listener called whenever the store changes.
// Returns an unsubscribe function.
func (s *Store) Subscribe(fn func()) func() {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.nextSubID
	s.nextSubID++
	s.subscribers[id] = fn

	return func() {
		s.mu.Lock()
		delete(s.subscribers, id)
		s.mu.Unlock()
	}
}

func (s *Store) notifySubscribers() {
	s.mu.RLock()
	subs := make([]func(), 0, len(s.subscribers))
	for _, fn := range s.subscribers {
		subs = append(subs, fn)
	}
	s.mu.RUnlock()

	for _, fn := range subs {
		fn()
	}
}

// Add appends or replaces a notification in history.
func (s *Store) Add(n Notification) {
	item := StoredItem{
		ID:        n.ID,
		AppName:   n.AppName,
		Summary:   n.Summary,
		Body:      n.Body,
		Icon:      n.Icon,
		Urgency:   n.Urgency,
		Timestamp: time.Now(),
		Actions:   n.Actions,
	}

	s.mu.Lock()
	// Replace existing item if matching ID
	replaced := false
	for i, it := range s.items {
		if it.ID == item.ID {
			s.items[i] = item
			replaced = true
			break
		}
	}
	if !replaced {
		s.items = append([]StoredItem{item}, s.items...)
		if len(s.items) > maxStoredNotifications {
			s.items = s.items[:maxStoredNotifications]
		}
	}
	s.mu.Unlock()

	_ = s.Save()
	s.notifySubscribers()
}

// Remove deletes a notification by ID.
func (s *Store) Remove(id uint32) {
	s.mu.Lock()
	for i, it := range s.items {
		if it.ID == id {
			s.items = append(s.items[:i], s.items[i+1:]...)
			break
		}
	}
	s.mu.Unlock()

	_ = s.Save()
	s.notifySubscribers()
}

// RemoveGroup deletes all notifications for a given application.
func (s *Store) RemoveGroup(appName string) {
	s.mu.Lock()
	filtered := s.items[:0]
	for _, it := range s.items {
		if it.AppName != appName {
			filtered = append(filtered, it)
		}
	}
	s.items = filtered
	s.mu.Unlock()

	_ = s.Save()
	s.notifySubscribers()
}

// Clear removes all stored notifications.
func (s *Store) Clear() {
	s.mu.Lock()
	s.items = nil
	s.mu.Unlock()

	_ = s.Save()
	s.notifySubscribers()
}

// Count returns the total number of notifications in history.
func (s *Store) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.items)
}

// All returns a copy of all stored notifications.
func (s *Store) All() []StoredItem {
	s.mu.RLock()
	defer s.mu.RUnlock()
	res := make([]StoredItem, len(s.items))
	copy(res, s.items)
	return res
}

// Grouped returns notifications grouped by application name, sorted newest first.
func (s *Store) Grouped() []GroupedNotifications {
	s.mu.RLock()
	defer s.mu.RUnlock()

	groupMap := make(map[string]*GroupedNotifications)
	var order []string

	for _, it := range s.items {
		name := it.AppName
		if name == "" {
			name = "System"
		}
		g, ok := groupMap[name]
		if !ok {
			g = &GroupedNotifications{
				AppName: name,
				Icon:    it.Icon,
			}
			groupMap[name] = g
			order = append(order, name)
		}
		g.Items = append(g.Items, it)
	}

	res := make([]GroupedNotifications, 0, len(order))
	for _, name := range order {
		res = append(res, *groupMap[name])
	}
	return res
}

// Load reads notification history from disk.
func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			s.items = nil
			return nil
		}
		return err
	}

	var items []StoredItem
	if err := json.Unmarshal(data, &items); err != nil {
		return err
	}

	// Sort newest first
	sort.Slice(items, func(i, j int) bool {
		return items[i].Timestamp.After(items[j].Timestamp)
	})

	if len(items) > maxStoredNotifications {
		items = items[:maxStoredNotifications]
	}
	s.items = items
	return nil
}

// Save writes notification history to disk.
func (s *Store) Save() error {
	s.mu.RLock()
	items := make([]StoredItem, len(s.items))
	copy(items, s.items)
	path := s.filePath
	s.mu.RUnlock()

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return err
	}

	tmpFile := path + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmpFile, path)
}
