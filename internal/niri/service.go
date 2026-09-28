package niri

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

type Service struct {
	client *Client

	mu              sync.RWMutex
	workspaces      []Workspace
	subscribers     map[int]chan []Workspace
	nextSubID       int
	keyboardLayouts KeyboardLayouts
	kbSubscribers   map[int]chan KeyboardLayouts
	nextKbSubID     int

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	conn   net.Conn
}

func NewService(client *Client) *Service {
	ctx, cancel := context.WithCancel(context.Background())
	return &Service{
		client:        client,
		subscribers:   make(map[int]chan []Workspace),
		kbSubscribers: make(map[int]chan KeyboardLayouts),
		ctx:           ctx,
		cancel:        cancel,
	}
}

func (s *Service) Start() error {
	initial, err := s.client.QueryWorkspaces()
	if err != nil {
		return fmt.Errorf("failed to fetch initial workspaces from Niri: %w", err)
	}

	s.mu.Lock()
	s.workspaces = make([]Workspace, len(initial))
	copy(s.workspaces, initial)
	s.mu.Unlock()

	if kb, kbErr := s.client.QueryKeyboardLayouts(); kbErr == nil && kb != nil {
		s.mu.Lock()
		s.keyboardLayouts = *kb
		s.mu.Unlock()
	}

	conn, scanner, err := s.client.OpenEventStream()
	if err != nil {
		return fmt.Errorf("failed to start Niri event stream: %w", err)
	}
	s.conn = conn

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer conn.Close()

		for scanner.Scan() {
			select {
			case <-s.ctx.Done():
				return
			default:
			}

			line := scanner.Bytes()
			if len(line) == 0 {
				continue
			}

			var ev Event
			if err := json.Unmarshal(line, &ev); err != nil {
				continue
			}

			s.handleEvent(&ev)
		}
	}()

	return nil
}

func (s *Service) handleEvent(ev *Event) {
	s.mu.Lock()
	changed := false

	if ev.WorkspacesChanged != nil {
		s.workspaces = make([]Workspace, len(ev.WorkspacesChanged.Workspaces))
		copy(s.workspaces, ev.WorkspacesChanged.Workspaces)
		changed = true
	} else if ev.WorkspaceActivated != nil {
		targetID := ev.WorkspaceActivated.ID
		focused := ev.WorkspaceActivated.Focused

		var targetOutput *string
		for _, ws := range s.workspaces {
			if ws.ID == targetID {
				targetOutput = ws.Output
				break
			}
		}

		for i := range s.workspaces {
			ws := &s.workspaces[i]

			if targetOutput != nil && ws.Output != nil && *ws.Output == *targetOutput {
				if ws.ID == targetID {
					if !ws.IsActive {
						ws.IsActive = true
						changed = true
					}
				} else {
					if ws.IsActive {
						ws.IsActive = false
						changed = true
					}
				}
			} else if ws.ID == targetID {
				if !ws.IsActive {
					ws.IsActive = true
					changed = true
				}
			}

			if focused {
				if ws.ID == targetID {
					if !ws.IsFocused {
						ws.IsFocused = true
						changed = true
					}
				} else {
					if ws.IsFocused {
						ws.IsFocused = false
						changed = true
					}
				}
			} else if ws.ID == targetID {
				if ws.IsFocused {
					ws.IsFocused = false
					changed = true
				}
			}
		}
	}

	var current []Workspace
	if changed {
		current = make([]Workspace, len(s.workspaces))
		copy(current, s.workspaces)
	}
	s.mu.Unlock()

	if changed {
		s.notifySubscribers(current)
	}

	if ev.KeyboardLayoutsChanged != nil {
		s.mu.Lock()
		s.keyboardLayouts = ev.KeyboardLayoutsChanged.KeyboardLayouts
		currentKb := s.keyboardLayouts
		s.mu.Unlock()
		s.notifyKeyboardSubscribers(currentKb)
	} else if ev.KeyboardLayoutSwitched != nil {
		s.mu.Lock()
		s.keyboardLayouts.CurrentIdx = ev.KeyboardLayoutSwitched.Idx
		currentKb := s.keyboardLayouts
		s.mu.Unlock()
		s.notifyKeyboardSubscribers(currentKb)
	}
}

func (s *Service) notifySubscribers(ws []Workspace) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, ch := range s.subscribers {
		select {
		case ch <- ws:
		default:
			// Drain old event and send latest to prevent blocking
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- ws:
			default:
			}
		}
	}
}

func (s *Service) Workspaces() []Workspace {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]Workspace, len(s.workspaces))
	copy(result, s.workspaces)
	return result
}

func (s *Service) Subscribe() (<-chan []Workspace, func()) {
	s.mu.Lock()
	id := s.nextSubID
	s.nextSubID++
	ch := make(chan []Workspace, 1)
	s.subscribers[id] = ch

	current := make([]Workspace, len(s.workspaces))
	copy(current, s.workspaces)
	ch <- current
	s.mu.Unlock()

	unsubscribe := func() {
		s.mu.Lock()
		delete(s.subscribers, id)
		close(ch)
		s.mu.Unlock()
	}

	return ch, unsubscribe
}

func (s *Service) notifyKeyboardSubscribers(kb KeyboardLayouts) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, ch := range s.kbSubscribers {
		select {
		case ch <- kb:
		default:
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- kb:
			default:
			}
		}
	}
}

func (s *Service) KeyboardLayouts() KeyboardLayouts {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.keyboardLayouts
}

func (s *Service) SubscribeKeyboard() (<-chan KeyboardLayouts, func()) {
	s.mu.Lock()
	id := s.nextKbSubID
	s.nextKbSubID++
	ch := make(chan KeyboardLayouts, 1)
	s.kbSubscribers[id] = ch

	current := s.keyboardLayouts
	ch <- current
	s.mu.Unlock()

	unsubscribe := func() {
		s.mu.Lock()
		delete(s.kbSubscribers, id)
		close(ch)
		s.mu.Unlock()
	}

	return ch, unsubscribe
}

func (s *Service) SwitchLayoutNext() error {
	if s == nil || s.client == nil {
		return fmt.Errorf("niri client not available")
	}
	return s.client.SwitchLayoutNext()
}

func (s *Service) FocusWorkspace(id uint64) error {
	return s.client.FocusWorkspace(id)
}

func (s *Service) QueryWindows() ([]Window, error) {
	if s == nil || s.client == nil {
		return nil, fmt.Errorf("niri client not available")
	}
	return s.client.QueryWindows()
}

func (s *Service) FocusWindow(id uint64) error {
	if s == nil || s.client == nil {
		return fmt.Errorf("niri client not available")
	}
	return s.client.FocusWindow(id)
}

func (s *Service) Close() error {
	s.cancel()
	if s.conn != nil {
		if closer, ok := s.conn.(io.Closer); ok {
			_ = closer.Close()
		}
	}
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
	}
	return nil
}
