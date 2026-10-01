package compositor

import (
	"sync"
)

type ServiceState struct {
	mu              sync.RWMutex
	workspaces      []Workspace
	subscribers     map[int]chan []Workspace
	nextSubID       int
	keyboardLayouts KeyboardLayouts
	kbSubscribers   map[int]chan KeyboardLayouts
	nextKbSubID     int
}

func NewServiceState() *ServiceState {
	return &ServiceState{
		subscribers:   make(map[int]chan []Workspace),
		kbSubscribers: make(map[int]chan KeyboardLayouts),
	}
}

func (s *ServiceState) SetWorkspaces(ws []Workspace) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.workspaces = ws
	current := make([]Workspace, len(ws))
	copy(current, ws)
	s.notifySubscribersLocked(current)
}

func (s *ServiceState) SetKeyboardLayouts(kb KeyboardLayouts) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.keyboardLayouts = kb
	s.notifyKeyboardSubscribersLocked(kb)
}

func (s *ServiceState) SetKeyboardIndex(idx int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.keyboardLayouts.CurrentIdx = idx
	current := s.keyboardLayouts
	s.notifyKeyboardSubscribersLocked(current)
}

func (s *ServiceState) notifySubscribersLocked(ws []Workspace) {
	for _, ch := range s.subscribers {
		select {
		case ch <- ws:
		default:
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

func (s *ServiceState) notifyKeyboardSubscribersLocked(kb KeyboardLayouts) {
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

func (s *ServiceState) Workspaces() []Workspace {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]Workspace, len(s.workspaces))
	copy(result, s.workspaces)
	return result
}

func (s *ServiceState) Subscribe() (<-chan []Workspace, func()) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := s.nextSubID
	s.nextSubID++
	ch := make(chan []Workspace, 1)
	s.subscribers[id] = ch

	current := make([]Workspace, len(s.workspaces))
	copy(current, s.workspaces)
	ch <- current

	unsubscribe := func() {
		s.mu.Lock()
		delete(s.subscribers, id)
		close(ch)
		s.mu.Unlock()
	}
	return ch, unsubscribe
}

func (s *ServiceState) KeyboardLayouts() KeyboardLayouts {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.keyboardLayouts
}

func (s *ServiceState) SubscribeKeyboard() (<-chan KeyboardLayouts, func()) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := s.nextKbSubID
	s.nextKbSubID++
	ch := make(chan KeyboardLayouts, 1)
	s.kbSubscribers[id] = ch

	current := s.keyboardLayouts
	ch <- current

	unsubscribe := func() {
		s.mu.Lock()
		delete(s.kbSubscribers, id)
		close(ch)
		s.mu.Unlock()
	}
	return ch, unsubscribe
}
