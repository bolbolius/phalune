package niri

import (
	"encoding/json"
	"fmt"
	"net"
	"os/exec"
	"sync"
	"time"

	"phalune/internal/compositor"
)

// Service implements compositor.Service for niri over its JSON-RPC socket.
type Service struct {
	*compositor.ServiceState

	client *Client
	done   chan struct{}

	conn net.Conn
	wg   sync.WaitGroup
}

// NewService builds a niri-backed compositor service.
func NewService(client *Client) *Service {
	return &Service{
		client:       client,
		ServiceState: compositor.NewServiceState(),
		done:         make(chan struct{}),
	}
}

func (s *Service) Kind() compositor.Kind { return compositor.Niri }

func (s *Service) Start() error {
	initial, err := s.client.QueryWorkspaces()
	if err != nil {
		return fmt.Errorf("failed to fetch initial workspaces from Niri: %w", err)
	}

	ws := make([]compositor.Workspace, len(initial))
	for i, w := range initial {
		ws[i] = toCompositorWorkspace(w)
	}
	s.ServiceState.SetWorkspaces(ws)

	if kb, kbErr := s.client.QueryKeyboardLayouts(); kbErr == nil && kb != nil {
		s.ServiceState.SetKeyboardLayouts(toCompositorKeyboardLayouts(*kb))
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
			case <-s.done:
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
	if ev.WorkspacesChanged != nil {
		ws := make([]compositor.Workspace, len(ev.WorkspacesChanged.Workspaces))
		for i, w := range ev.WorkspacesChanged.Workspaces {
			ws[i] = toCompositorWorkspace(w)
		}
		s.ServiceState.SetWorkspaces(ws)
	} else if ev.WorkspaceActivated != nil {
		targetID := ev.WorkspaceActivated.ID
		focused := ev.WorkspaceActivated.Focused

		ws := s.ServiceState.Workspaces()
		var targetOutput string
		for _, w := range ws {
			if w.ID == targetID {
				targetOutput = w.Output
				break
			}
		}

		for i := range ws {
			ww := &ws[i]

			if targetOutput != "" && ww.Output == targetOutput {
				if ww.ID == targetID {
					ww.IsActive = true
				} else {
					ww.IsActive = false
				}
			} else if ww.ID == targetID {
				ww.IsActive = true
			}

			if focused {
				if ww.ID == targetID {
					ww.IsFocused = true
				} else {
					ww.IsFocused = false
				}
			} else if ww.ID == targetID {
				ww.IsFocused = false
			}
		}
		s.ServiceState.SetWorkspaces(ws)
	}

	if ev.KeyboardLayoutsChanged != nil {
		s.ServiceState.SetKeyboardLayouts(toCompositorKeyboardLayouts(ev.KeyboardLayoutsChanged.KeyboardLayouts))
	} else if ev.KeyboardLayoutSwitched != nil {
		s.ServiceState.SetKeyboardIndex(ev.KeyboardLayoutSwitched.Idx)
	}
}

func (s *Service) SwitchLayoutNext() error {
	if s == nil || s.client == nil {
		return fmt.Errorf("compositor service not available")
	}
	if err := s.client.SwitchLayoutNext(); err == nil {
		return nil
	}
	// Fallback to niri CLI (socket-only clients cannot always switch).
	cmd := exec.Command("niri", "msg", "action", "switch-layout", "next")
	return cmd.Run()
}

func (s *Service) QueryWindows() ([]compositor.Window, error) {
	if s == nil || s.client == nil {
		return nil, fmt.Errorf("compositor service not available")
	}
	wins, err := s.client.QueryWindows()
	if err != nil {
		return nil, err
	}
	return toCompositorWindows(wins), nil
}

func toCompositorWindows(wins []Window) []compositor.Window {
	out := make([]compositor.Window, len(wins))
	for i, w := range wins {
		out[i] = toCompositorWindow(w)
	}
	return out
}

func (s *Service) FocusWorkspace(id uint64) error {
	if s == nil || s.client == nil {
		return fmt.Errorf("compositor service not available")
	}
	return s.client.FocusWorkspace(id)
}

func (s *Service) FocusWindow(id uint64) error {
	if s == nil || s.client == nil {
		return fmt.Errorf("compositor service not available")
	}
	return s.client.FocusWindow(id)
}

// Close stops the event stream and releases resources.
func (s *Service) Close() error {
	if s == nil {
		return nil
	}
	close(s.done)
	if s.conn != nil {
		_ = s.conn.Close()
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

var _ compositor.Service = (*Service)(nil)
