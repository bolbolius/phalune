package sway

import (
	"phalune/internal/compositor"
)

type swayWorkspace struct {
	Num     int    `json:"num"`
	Name    string `json:"name"`
	Focused bool   `json:"focused"`
	Visible bool   `json:"visible"`
	Urgent  bool   `json:"urgent"`
	Output  string `json:"output"`
}

func (w swayWorkspace) toCompositor() compositor.Workspace {
	cw := compositor.Workspace{
		Name:      w.Name,
		Output:    w.Output,
		IsUrgent:  w.Urgent,
		IsActive:  w.Visible,
		IsFocused: w.Focused,
	}
	if w.Num > 0 {
		cw.ID = uint64(w.Num)
		cw.Idx = uint8(w.Num)
	} else {
		cw.ID = pseudoID(w.Name)
	}
	return cw
}

func pseudoID(name string) uint64 {
	var h uint64 = 14695981039346656037
	for i := 0; i < len(name); i++ {
		h ^= uint64(name[i])
		h *= 1099511628211
	}
	return (h & 0xFFFFFFFFFFFFF000) | 1<<40
}

type swayNode struct {
	ID          uint64  `json:"id"`
	Name        string  `json:"name"`
	Type        string  `json:"type"`
	AppID       *string `json:"app_id"`
	WindowCl    *string `json:"window_properties_class"`
	WindowTitle *string `json:"window_properties_title"`
	WindowProps *struct {
		Class *string `json:"class"`
		Title *string `json:"title"`
	} `json:"window_properties"`
	PID        int        `json:"pid"`
	Urgent     bool       `json:"urgent"`
	Focused    bool       `json:"focused"`
	Nodes      []swayNode `json:"nodes"`
	Float      []swayNode `json:"floating_nodes"`
	Fullscreen bool       `json:"fullscreen_mode"`
	Rect       rect       `json:"rect"`
	WindowRect rect       `json:"window_rect"`
	Num        int        `json:"num"`
}

type rect struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

func (n *swayNode) windows() []compositor.Window {
	var out []compositor.Window
	var walk func(node *swayNode, wsID uint64, floating bool)
	walk = func(node *swayNode, wsID uint64, floating bool) {
		if node == nil {
			return
		}
		if node.Type == "workspace" {
			wsID = node.workspaceID()
			floating = false
		}
		if node.Type == "con" && node.isWindow() {
			w := compositor.Window{
				ID:          node.ID,
				Title:       node.title(),
				AppID:       node.appID(),
				PID:         node.PID,
				WorkspaceID: wsID,
				IsFocused:   node.Focused,
				IsFloating:  floating,
				IsUrgent:    node.Urgent,
			}
			if node.WindowRect.Width > 0 {
				w.Pos = []float64{float64(node.Rect.X + node.WindowRect.X), float64(node.Rect.Y + node.WindowRect.Y)}
				w.Size = []float64{float64(node.WindowRect.Width), float64(node.WindowRect.Height)}
			} else if node.Rect.Width > 0 {
				w.Pos = []float64{float64(node.Rect.X), float64(node.Rect.Y)}
				w.Size = []float64{float64(node.Rect.Width), float64(node.Rect.Height)}
			}
			out = append(out, w)
		}
		for i := range node.Nodes {
			walk(&node.Nodes[i], wsID, floating)
		}
		for i := range node.Float {
			walk(&node.Float[i], wsID, true)
		}
	}
	walk(n, 0, false)
	return out
}

func (n *swayNode) isWindow() bool {
	cls := n.appID()
	return cls != "" || (n.PID > 0 && len(n.Nodes) == 0)
}

func (n *swayNode) workspaceID() uint64 {
	if n.Num > 0 {
		return uint64(n.Num)
	}
	return pseudoID(n.Name)
}

func (n *swayNode) appID() string {
	if n.AppID != nil && *n.AppID != "" {
		return *n.AppID
	}
	if n.WindowProps != nil && n.WindowProps.Class != nil {
		return *n.WindowProps.Class
	}
	if n.WindowCl != nil {
		return *n.WindowCl
	}
	return ""
}

func (n *swayNode) title() string {
	if n.WindowProps != nil && n.WindowProps.Title != nil {
		return *n.WindowProps.Title
	}
	if n.WindowTitle != nil {
		return *n.WindowTitle
	}
	return n.Name
}
