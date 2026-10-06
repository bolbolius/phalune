package ipc

import (
	"log/slog"
	"sort"
	"sync"
)

// WidgetHub tracks live IPC widget state and the connection owning each
// widget. State outlives owners: when a daemon disconnects, the last
// pushed content stays on the bar, and a reconnecting daemon re-attaches
// through the watch handshake, which carries the persisted state.
type WidgetHub struct {
	mu        sync.RWMutex
	states    map[string]*WidgetState
	owners    map[string]chan *WidgetEvent
	watchers  map[string]map[uint64]func()
	nextWatch uint64
}

func NewWidgetHub() *WidgetHub {
	return &WidgetHub{
		states:   make(map[string]*WidgetState),
		owners:   make(map[string]chan *WidgetEvent),
		watchers: make(map[string]map[uint64]func()),
	}
}

const widgetEventBuffer = 16

// OnChange registers a callback fired (in the publishing goroutine) every
// time the state of id changes. Returns an unsubscribe function.
func (h *WidgetHub) OnChange(id string, fn func()) func() {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, ok := h.watchers[id]; !ok {
		h.watchers[id] = make(map[uint64]func())
	}
	subID := h.nextWatch
	h.nextWatch++
	h.watchers[id][subID] = fn

	return func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if set, ok := h.watchers[id]; ok {
			delete(set, subID)
			if len(set) == 0 {
				delete(h.watchers, id)
			}
		}
	}
}

// attach claims ownership of id. Returns false when another live
// connection already owns it.
func (h *WidgetHub) attach(id string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.owners[id]; ok {
		return false
	}
	h.owners[id] = make(chan *WidgetEvent, widgetEventBuffer)
	return true
}

// detach releases ownership of id.
func (h *WidgetHub) detach(id string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if ch, ok := h.owners[id]; ok {
		delete(h.owners, id)
		close(ch)
	}
}

// events returns the event channel for the owner of id.
func (h *WidgetHub) events(id string) <-chan *WidgetEvent {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.owners[id]
}

// State returns a copy of the current pushed state for id.
func (h *WidgetHub) State(id string) (WidgetState, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	st, ok := h.states[id]
	if !ok {
		return WidgetState{}, false
	}
	return *st, true
}

// All returns copies of every tracked widget state, sorted by id.
func (h *WidgetHub) All() []WidgetState {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]WidgetState, 0, len(h.states))
	for id, st := range h.states {
		cpy := *st
		cpy.ID = id
		out = append(out, cpy)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out
}

// Push stores new state and marks the widget visible.
func (h *WidgetHub) Push(st WidgetState) {
	h.mu.Lock()
	st.Visible = true
	if st.Percent != nil {
		p := clampPercentage(*st.Percent)
		st.Percent = &p
	}
	h.states[st.ID] = &st
	watchers := make([]func(), 0, len(h.watchers[st.ID]))
	for _, fn := range h.watchers[st.ID] {
		watchers = append(watchers, fn)
	}
	h.mu.Unlock()

	for _, fn := range watchers {
		fn()
	}
}

// Clear hides the widget; the daemon keeps ownership of the id.
func (h *WidgetHub) Clear(id string) {
	h.mu.Lock()
	if st, ok := h.states[id]; ok {
		st.Visible = false
	}
	watchers := make([]func(), 0, len(h.watchers[id]))
	for _, fn := range h.watchers[id] {
		watchers = append(watchers, fn)
	}
	h.mu.Unlock()

	for _, fn := range watchers {
		fn()
	}
}

// DispatchClick sends a user interaction back to the owning connection.
// Returns false when nobody owns the widget.
func (h *WidgetHub) DispatchClick(ev *WidgetEvent) bool {
	h.mu.RLock()
	ch, ok := h.owners[ev.ID]
	h.mu.RUnlock()
	if !ok {
		return false
	}

	select {
	case ch <- ev:
		return true
	default:
		slog.Warn("ipc: widget event dropped, daemon not reading", "widget", ev.ID)
		return false
	}
}

func (h *WidgetHub) closeAll() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for id, ch := range h.owners {
		close(ch)
		delete(h.owners, id)
	}
}

func clampPercentage(p float64) float64 {
	if p < 0 {
		return 0
	}
	if p > 100 {
		return 100
	}
	return p
}
