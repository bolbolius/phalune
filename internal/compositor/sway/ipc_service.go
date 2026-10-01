package sway

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"

	"phalune/internal/compositor"
)

type Service struct {
	*compositor.ServiceState

	socketPath string
	client     *Client

	mu       sync.Mutex
	eventCxn *eventConn
	stop     chan struct{}
	stopped  bool
	cancel   func()
}

func New(socketPath string) *Service {
	return &Service{
		ServiceState: compositor.NewServiceState(),
		socketPath:   socketPath,
		stop:         make(chan struct{}),
	}
}

func (s *Service) Kind() compositor.Kind { return compositor.Sway }

func (s *Service) Ping() error {
	c, err := dial(s.socketPath)
	if err != nil {
		return err
	}
	defer c.Close()
	_, err = c.raw(msgGetVersion, "")
	return err
}

func (s *Service) Start() error {
	c, err := dial(s.socketPath)
	if err != nil {
		return err
	}

	s.client = &Client{conn: c}

	if err := s.refreshWorkspaces(); err != nil {
		slog.Warn("sway: initial workspace query failed", "error", err)
	}

	ecConn, err := dial(s.socketPath)
	if err != nil {
		_ = c.Close()
		return fmt.Errorf("sway: open event connection: %w", err)
	}
	ec, err := ecConn.events(eventWorkspace, eventWindow)
	if err != nil {
		_ = ecConn.Close()
		_ = c.Close()
		return fmt.Errorf("sway: subscribe events: %w", err)
	}
	s.eventCxn = ec

	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel

	go s.dispatchLoop(ctx, ec)
	return nil
}

func (s *Service) dispatchLoop(ctx context.Context, ec *eventConn) {
	defer slog.Debug("sway: event loop stopped")
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stop:
			return
		case ev, ok := <-ec.ch:
			if !ok {
				return
			}
			s.handleEvent(ev)
		}
	}
}

func (s *Service) refreshWorkspaces() error {
	if s.client == nil {
		return fmt.Errorf("sway client not connected")
	}
	ws, err := s.client.Workspaces()
	if err != nil {
		return err
	}
	s.ServiceState.SetWorkspaces(ws)
	return nil
}

func (s *Service) handleEvent(ev event) {
	switch ev.Change {
	case "empty", "focus", "init", "move", "reload", "rename", "restored", "urgent",
		"new", "close", "title", "fullscreen_mode", "floating", "mark":
		s.refresh()
	default:
		slog.Debug("sway: unhandled event", "change", ev.Change)
	}
}

func (s *Service) refresh() {
	if err := s.refreshWorkspaces(); err != nil {
		slog.Debug("sway: workspace refresh failed", "error", err)
	}
}

type Client struct {
	conn *conn
}

func (c *Client) Workspaces() ([]compositor.Workspace, error) {
	body, err := c.conn.raw(msgGetWorkspaces, "")
	if err != nil {
		return nil, err
	}

	var raws []swayWorkspace
	if err := json.Unmarshal(body, &raws); err != nil {
		return nil, fmt.Errorf("decode sway workspaces: %w", err)
	}

	out := make([]compositor.Workspace, 0, len(raws))
	for _, r := range raws {
		out = append(out, r.toCompositor())
	}
	return out, nil
}

func (c *Client) Tree() (*swayNode, error) {
	body, err := c.conn.raw(msgGetTree, "")
	if err != nil {
		return nil, err
	}
	var root swayNode
	if err := json.Unmarshal(body, &root); err != nil {
		return nil, fmt.Errorf("decode sway tree: %w", err)
	}
	return &root, nil
}

func (c *Client) RunCommand(cmd string) error {
	body, err := c.conn.raw(msgRunCommand, cmd)
	if err != nil {
		return err
	}
	var resp []struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("decode sway command reply: %w", err)
	}
	for _, r := range resp {
		if !r.Success {
			if r.Error != "" {
				return fmt.Errorf("sway command failed: %s", r.Error)
			}
			return fmt.Errorf("sway command failed")
		}
	}
	return nil
}

func (s *Service) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped {
		return nil
	}
	s.stopped = true
	close(s.stop)
	if s.cancel != nil {
		s.cancel()
	}
	if s.eventCxn != nil {
		s.eventCxn.close()
		s.eventCxn = nil
	}
	if s.client != nil && s.client.conn != nil {
		_ = s.client.conn.Close()
	}
	return nil
}

func (s *Service) QueryWindows() ([]compositor.Window, error) {
	s.mu.Lock()
	client := s.client
	s.mu.Unlock()
	if client == nil {
		return nil, fmt.Errorf("sway client not connected")
	}
	tree, err := client.Tree()
	if err != nil {
		return nil, err
	}
	return tree.windows(), nil
}

func (s *Service) FocusWorkspace(id uint64) error {
	for _, ws := range s.ServiceState.Workspaces() {
		if ws.ID == id {
			if ws.Name != "" {
				return s.command(fmt.Sprintf("workspace %s", ws.Name))
			}
			return s.command(fmt.Sprintf("workspace number %d", id))
		}
	}
	return s.command(fmt.Sprintf("workspace number %d", id))
}

func (s *Service) FocusWindow(id uint64) error {
	return s.command(fmt.Sprintf("[con_id=%d] focus", id))
}

func (s *Service) SwitchLayoutNext() error {
	return s.command("input * xkb_switch_layout next")
}

func (s *Service) command(cmd string) error {
	s.mu.Lock()
	client := s.client
	s.mu.Unlock()
	if client == nil {
		return fmt.Errorf("sway client not connected")
	}
	return client.RunCommand(cmd)
}

var _ compositor.Service = (*Service)(nil)
