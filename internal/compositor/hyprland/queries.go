package hyprland

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"phalune/internal/compositor"
)

func (s *Service) QueryWorkspaces() ([]compositor.Workspace, error) {
	data, err := s.request("j/workspaces")
	if err != nil {
		return nil, err
	}

	var raws []hyprWorkspace
	if err := json.Unmarshal(data, &raws); err != nil {
		return nil, fmt.Errorf("decode hyprland workspaces: %w", err)
	}

	activeWorkspaceID := map[string]int{}
	activeWorkspaceName := map[string]string{}
	focusedMonitor := map[string]bool{}

	monData, monErr := s.request("j/monitors")
	if monErr == nil {
		var monitors []hyprMonitor
		if jErr := json.Unmarshal(monData, &monitors); jErr == nil {
			for _, m := range monitors {
				if m.ActiveWorkspace.ID != 0 {
					activeWorkspaceID[m.Name] = m.ActiveWorkspace.ID
				}
				if m.ActiveWorkspace.Name != "" {
					activeWorkspaceName[m.Name] = m.ActiveWorkspace.Name
				}
				if m.Focused {
					focusedMonitor[m.Name] = true
				}
			}
		}
	}

	out := make([]compositor.Workspace, 0, len(raws))
	for _, r := range raws {
		cw := r.toCompositor()
		isActive := false
		if r.ID != nil && *r.ID != 0 && activeWorkspaceID[cw.Output] == *r.ID {
			isActive = true
		} else if r.Name != "" && activeWorkspaceName[cw.Output] == r.Name {
			isActive = true
		}
		if isActive {
			cw.IsActive = true
			cw.IsFocused = focusedMonitor[cw.Output]
		}
		out = append(out, cw)
	}
	return out, nil
}

func (s *Service) QueryWindows() ([]compositor.Window, error) {
	data, err := s.request("j/clients")
	if err != nil {
		return nil, err
	}

	var raws []hyprClient
	if err := json.Unmarshal(data, &raws); err != nil {
		return nil, fmt.Errorf("decode hyprland clients: %w", err)
	}

	out := make([]compositor.Window, 0, len(raws))
	for _, r := range raws {
		w := r.toCompositor()
		if r.Focused {
			w.IsFocused = true
		}
		out = append(out, w)
	}
	return out, nil
}

func (s *Service) FocusWorkspace(id uint64) error {
	for _, ws := range s.ServiceState.Workspaces() {
		if ws.ID == id {
			if ws.Name != "" {
				_, err := s.request(fmt.Sprintf("dispatch workspace name:%s", ws.Name))
				return err
			}
			break
		}
	}
	_, err := s.request(fmt.Sprintf("dispatch workspace %d", id))
	return err
}

func (s *Service) FocusWindow(id uint64) error {
	_, err := s.request(fmt.Sprintf("dispatch focuswindow address:0x%x", id))
	return err
}

func (s *Service) SwitchLayoutNext() error {
	_, err := s.request("dispatch switchxkblayout current next")
	return err
}

type hyprWorkspace struct {
	ID            *int   `json:"id"`
	Type          string `json:"type"`
	Name          string `json:"name"`
	Monitor       string `json:"monitor"`
	MonitorID     *int   `json:"monitorID"`
	Windows       int    `json:"windows"`
	HasFullscreen bool   `json:"hasfullscreen"`
	LastWindow    string `json:"lastwindow"`
}

func (w hyprWorkspace) toCompositor() compositor.Workspace {
	cw := compositor.Workspace{
		Name:     w.Name,
		Output:   w.Monitor,
		IsUrgent: false,
	}
	if w.ID != nil && *w.ID >= 0 {
		cw.ID = uint64(*w.ID)
		cw.Idx = uint8(*w.ID)
	} else {
		cw.ID = pseudoID(w.Name)
	}
	if w.LastWindow != "" {
		if id, ok := parseAddress(w.LastWindow); ok {
			cw.ActiveWindowID = id
			cw.HasActiveWindow = true
		}
	}
	return cw
}

type hyprMonitor struct {
	Name            string `json:"name"`
	Focused         bool   `json:"focused"`
	ActiveWorkspace struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"activeWorkspace"`
}

type hyprClient struct {
	Address   string `json:"address"`
	Mapped    bool   `json:"mapped"`
	Hidden    bool   `json:"hidden"`
	At        [2]int `json:"at"`
	Size      [2]int `json:"size"`
	Workspace struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"workspace"`
	Floating     bool   `json:"floating"`
	Pseudo       bool   `json:"pseudo"`
	Monitor      int    `json:"monitor"`
	Class        string `json:"class"`
	Title        string `json:"title"`
	InitialClass string `json:"initialClass"`
	InitialTitle string `json:"initialTitle"`
	PID          int    `json:"pid"`
	XWayland     bool   `json:"xwayland"`
	Focused      bool   `json:"focused"`
	Fullscreen   bool   `json:"fullscreen"`
}

func (c hyprClient) toCompositor() compositor.Window {
	w := compositor.Window{
		Title:       c.Title,
		AppID:       firstNonEmpty(c.InitialClass, c.Class),
		PID:         c.PID,
		WorkspaceID: uint64(c.Workspace.ID),
		IsFloating:  c.Floating,
		Pos:         []float64{float64(c.At[0]), float64(c.At[1])},
		Size:        []float64{float64(c.Size[0]), float64(c.Size[1])},
	}
	if c.Workspace.ID >= 0 {
		w.WorkspaceID = uint64(c.Workspace.ID)
	} else {
		w.WorkspaceID = pseudoID(c.Workspace.Name)
	}
	if id, ok := parseAddress(c.Address); ok {
		w.ID = id
	}
	return w
}

func parseAddress(addr string) (uint64, bool) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return 0, false
	}
	v, err := strconv.ParseUint(strings.TrimPrefix(addr, "0x"), 16, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func pseudoID(name string) uint64 {
	var h uint64 = 14695981039346656037
	for i := 0; i < len(name); i++ {
		h ^= uint64(name[i])
		h *= 1099511628211
	}
	return (h & 0xFFFFFFFFFFFFF000) | 1<<40
}
