package compositor

import (
	"errors"
	"strconv"
)

type Kind string

const (
	Niri     Kind = "niri"
	Sway     Kind = "sway"
	Hyprland Kind = "hyprland"
)

type Workspace struct {
	ID              uint64
	Idx             uint8
	Name            string
	Output          string
	IsUrgent        bool
	IsActive        bool
	IsFocused       bool
	ActiveWindowID  uint64
	HasActiveWindow bool
}

func (w Workspace) DisplayName() string {
	if w.Name != "" {
		return w.Name
	}
	return strconv.Itoa(int(w.Idx))
}

type Window struct {
	ID          uint64
	Title       string
	AppID       string
	PID         int
	WorkspaceID uint64
	IsFocused   bool
	IsFloating  bool
	IsUrgent    bool
	Pos         []float64
	Size        []float64
}

type KeyboardLayouts struct {
	Names      []string
	CurrentIdx int
}

func (k KeyboardLayouts) CurrentName() string {
	if len(k.Names) == 0 || k.CurrentIdx < 0 || k.CurrentIdx >= len(k.Names) {
		return ""
	}
	return k.Names[k.CurrentIdx]
}

type Service interface {
	Workspaces() []Workspace
	Subscribe() (<-chan []Workspace, func())

	KeyboardLayouts() KeyboardLayouts
	SubscribeKeyboard() (<-chan KeyboardLayouts, func())

	SwitchLayoutNext() error

	QueryWindows() ([]Window, error)
	FocusWorkspace(id uint64) error
	FocusWindow(id uint64) error

	Kind() Kind
	Close() error
}

var ErrNoCompositor = errors.New("no supported compositor detected (niri, sway, hyprland)")
