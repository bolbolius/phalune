package niri

import "encoding/json"

type Workspace struct {
	ID             uint64  `json:"id"`
	Idx            uint8   `json:"idx"`
	Name           *string `json:"name"`
	Output         *string `json:"output"`
	IsUrgent       bool    `json:"is_urgent"`
	IsActive       bool    `json:"is_active"`
	IsFocused      bool    `json:"is_focused"`
	ActiveWindowID *uint64 `json:"active_window_id"`
}

func (w *Workspace) DisplayName() string {
	if w.Name != nil && *w.Name != "" {
		return *w.Name
	}
	return string(rune('0' + w.Idx))
}

type Response struct {
	Ok  json.RawMessage `json:"Ok,omitempty"`
	Err string          `json:"Err,omitempty"`
}

type WorkspacesResponse struct {
	Workspaces []Workspace `json:"Workspaces"`
}

type KeyboardLayouts struct {
	Names      []string `json:"names"`
	CurrentIdx int      `json:"current_idx"`
}

func (k KeyboardLayouts) CurrentName() string {
	if len(k.Names) == 0 || k.CurrentIdx < 0 || k.CurrentIdx >= len(k.Names) {
		return ""
	}
	return k.Names[k.CurrentIdx]
}

type KeyboardLayoutsResponse = KeyboardLayouts

type Event struct {
	WorkspacesChanged      *WorkspacesChangedEvent      `json:"WorkspacesChanged,omitempty"`
	WorkspaceActivated     *WorkspaceActivatedEvent     `json:"WorkspaceActivated,omitempty"`
	KeyboardLayoutsChanged *KeyboardLayoutsChangedEvent `json:"KeyboardLayoutsChanged,omitempty"`
	KeyboardLayoutSwitched *KeyboardLayoutSwitchedEvent `json:"KeyboardLayoutSwitched,omitempty"`
}

type WorkspacesChangedEvent struct {
	Workspaces []Workspace `json:"workspaces"`
}

type WorkspaceActivatedEvent struct {
	ID      uint64 `json:"id"`
	Focused bool   `json:"focused"`
}

type KeyboardLayoutsChangedEvent struct {
	KeyboardLayouts KeyboardLayouts `json:"keyboard_layouts"`
}

type KeyboardLayoutSwitchedEvent struct {
	Idx int `json:"idx"`
}

type Window struct {
	ID             uint64        `json:"id"`
	Title          string        `json:"title"`
	AppID          string        `json:"app_id"`
	PID            int           `json:"pid"`
	WorkspaceID    uint64        `json:"workspace_id"`
	IsFocused      bool          `json:"is_focused"`
	IsFloating     bool          `json:"is_floating"`
	IsUrgent       bool          `json:"is_urgent"`
	Layout         *WindowLayout `json:"layout,omitempty"`
	FocusTimestamp *Timestamp    `json:"focus_timestamp,omitempty"`
}

type WindowLayout struct {
	PosInScrollingLayout []int     `json:"pos_in_scrolling_layout,omitempty"`
	TileSize             []float64 `json:"tile_size,omitempty"`
	WindowSize           []int     `json:"window_size,omitempty"`
}

type Timestamp struct {
	Secs  uint64 `json:"secs"`
	Nanos uint32 `json:"nanos"`
}

type WindowsResponse struct {
	Windows []Window `json:"Windows"`
}

type ActionRequest struct {
	Action ActionPayload `json:"Action"`
}

type ActionPayload struct {
	FocusWorkspace *FocusWorkspacePayload `json:"FocusWorkspace,omitempty"`
	FocusWindow    *FocusWindowPayload    `json:"FocusWindow,omitempty"`
}

type FocusWindowPayload struct {
	ID uint64 `json:"id"`
}

type FocusWorkspacePayload struct {
	Reference WorkspaceReference `json:"reference"`
}

type WorkspaceReference struct {
	ID    *uint64 `json:"Id,omitempty"`
	Index *uint8  `json:"Index,omitempty"`
	Name  *string `json:"Name,omitempty"`
}
