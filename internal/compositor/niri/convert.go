package niri

import "phalune/internal/compositor"

func toCompositorWorkspace(w Workspace) compositor.Workspace {
	cw := compositor.Workspace{
		ID:        w.ID,
		Idx:       w.Idx,
		IsActive:  w.IsActive,
		IsFocused: w.IsFocused,
		IsUrgent:  w.IsUrgent,
	}
	if w.Name != nil {
		cw.Name = *w.Name
	}
	if w.Output != nil {
		cw.Output = *w.Output
	}
	if w.ActiveWindowID != nil {
		cw.ActiveWindowID = *w.ActiveWindowID
		cw.HasActiveWindow = true
	}
	return cw
}

func toCompositorWindow(w Window) compositor.Window {
	cw := compositor.Window{
		ID:          w.ID,
		Title:       w.Title,
		AppID:       w.AppID,
		PID:         w.PID,
		WorkspaceID: w.WorkspaceID,
		IsFocused:   w.IsFocused,
		IsFloating:  w.IsFloating,
		IsUrgent:    w.IsUrgent,
	}
	if w.Layout != nil {
		if len(w.Layout.TileSize) >= 4 {
			cw.Pos = []float64{w.Layout.TileSize[0], w.Layout.TileSize[1]}
			cw.Size = []float64{w.Layout.TileSize[2], w.Layout.TileSize[3]}
		} else if len(w.Layout.TileSize) >= 2 {
			cw.Pos = []float64{0, 0}
			cw.Size = []float64{w.Layout.TileSize[0], w.Layout.TileSize[1]}
		} else if len(w.Layout.WindowSize) >= 2 {
			cw.Pos = []float64{0, 0}
			cw.Size = []float64{float64(w.Layout.WindowSize[0]), float64(w.Layout.WindowSize[1])}
		}
	}
	return cw
}

func toCompositorKeyboardLayouts(k KeyboardLayouts) compositor.KeyboardLayouts {
	return compositor.KeyboardLayouts{
		Names:      k.Names,
		CurrentIdx: k.CurrentIdx,
	}
}
