package hyprland

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"phalune/internal/compositor"
)

const (
	socketCommand = ".socket.sock"
	socketEvent   = ".socket2.sock"
)

type Service struct {
	*compositor.ServiceState

	his string

	mu        sync.Mutex
	eventConn net.Conn

	cancel  context.CancelFunc
	stopped chan struct{}
	once    sync.Once
}

func New() *Service {
	return &Service{
		ServiceState: compositor.NewServiceState(),
		his:          os.Getenv("HYPRLAND_INSTANCE_SIGNATURE"),
	}
}

func (s *Service) Kind() compositor.Kind { return compositor.Hyprland }

func (s *Service) runtimeDir() (string, error) {
	base := os.Getenv("XDG_RUNTIME_DIR")
	if base == "" {
		return "", fmt.Errorf("XDG_RUNTIME_DIR not set")
	}
	return filepath.Join(base, "hypr", s.his), nil
}

func (s *Service) dialCommand() (net.Conn, error) {
	dir, err := s.runtimeDir()
	if err != nil {
		return nil, err
	}
	c, err := net.Dial("unix", filepath.Join(dir, socketCommand))
	if err != nil {
		return nil, fmt.Errorf("connect hyprland ipc: %w", err)
	}
	return c, nil
}

func (s *Service) dialEvent() (net.Conn, error) {
	dir, err := s.runtimeDir()
	if err != nil {
		return nil, err
	}
	c, err := net.Dial("unix", filepath.Join(dir, socketEvent))
	if err != nil {
		return nil, fmt.Errorf("connect hyprland event socket: %w", err)
	}
	return c, nil
}

func (s *Service) Ping() error {
	c, err := s.dialCommand()
	if err != nil {
		return err
	}
	defer c.Close()
	return nil
}

func (s *Service) request(cmd string) ([]byte, error) {
	c, err := s.dialCommand()
	if err != nil {
		return nil, err
	}
	defer c.Close()

	if _, err := c.Write([]byte(cmd + "\n")); err != nil {
		return nil, fmt.Errorf("hyprland write %q: %w", cmd, err)
	}
	data, err := io.ReadAll(c)
	if err != nil {
		return nil, fmt.Errorf("hyprland read %q: %w", cmd, err)
	}
	return data, nil
}

func (s *Service) Start() error {
	if ws, err := s.QueryWorkspaces(); err == nil {
		s.ServiceState.SetWorkspaces(ws)
	} else {
		slog.Warn("hyprland: initial workspace query failed", "error", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.stopped = make(chan struct{})

	go s.eventLoop(ctx)
	return nil
}

func (s *Service) eventLoop(ctx context.Context) {
	defer s.once.Do(func() { close(s.stopped) })
	defer slog.Debug("hyprland: event loop stopped")

	conn, err := s.dialEvent()
	if err != nil {
		slog.Warn("hyprland: event socket unavailable", "error", err)
		return
	}

	s.mu.Lock()
	s.eventConn = conn
	s.mu.Unlock()

	defer func() {
		_ = conn.Close()
		s.mu.Lock()
		s.eventConn = nil
		s.mu.Unlock()
	}()

	scan := bufio.NewScanner(conn)
	scan.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		if !scan.Scan() {
			return
		}
		line := strings.TrimSpace(scan.Text())
		if line == "" {
			continue
		}
		s.handleEventLine(line)
	}
}

func (s *Service) handleEventLine(line string) {
	name, payload, _ := strings.Cut(line, ">>")
	switch name {
	case "workspace", "workspacev2", "focusedmon", "focusedmonv2",
		"createworkspace", "createworkspacev2", "destroyworkspace", "destroyworkspacev2",
		"renameworkspace", "moveworkspace", "moveworkspacev2", "activespecial", "activespecialv2",
		"openwindow", "closewindow", "kill", "movewindow", "movewindowv2",
		"urgent", "fullscreen", "changefloatingmode", "minimized",
		"configreloaded":
		if ws, err := s.QueryWorkspaces(); err == nil {
			s.ServiceState.SetWorkspaces(ws)
		} else {
			slog.Debug("hyprland: workspace refresh failed", "error", err)
		}
	case "activelayout":
		layout := payload
		if _, l, ok := strings.Cut(payload, ","); ok {
			layout = l
		}
		if layout != "" {
			kb := s.ServiceState.KeyboardLayouts()
			if kb.Names == nil {
				kb.Names = []string{layout}
				kb.CurrentIdx = 0
			} else if idx := indexOf(kb.Names, layout); idx >= 0 {
				kb.CurrentIdx = idx
			} else {
				kb.Names = append(kb.Names, layout)
				kb.CurrentIdx = len(kb.Names) - 1
			}
			s.ServiceState.SetKeyboardLayouts(kb)
		}
	}
}

func indexOf(names []string, v string) int {
	for i, n := range names {
		if n == v {
			return i
		}
	}
	return -1
}

func (s *Service) Close() error {
	if s == nil {
		return nil
	}
	if s.cancel != nil {
		s.cancel()
	}
	s.mu.Lock()
	if s.eventConn != nil {
		_ = s.eventConn.Close()
	}
	s.mu.Unlock()

	if s.stopped != nil {
		<-s.stopped
	}
	return nil
}

var _ compositor.Service = (*Service)(nil)
