package niri

import (
	"encoding/json"
	"testing"
)

func TestUnmarshalWorkspacesResponse(t *testing.T) {
	data := `{"Ok":{"Workspaces":[{"id":1,"idx":1,"name":null,"output":"eDP-1","is_urgent":false,"is_active":true,"is_focused":true,"active_window_id":120},{"id":2,"idx":2,"name":"term","output":"eDP-1","is_urgent":false,"is_active":false,"is_focused":false,"active_window_id":null}]}}`

	var resp Response
	if err := json.Unmarshal([]byte(data), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	var wsResp WorkspacesResponse
	if err := json.Unmarshal(resp.Ok, &wsResp); err != nil {
		t.Fatalf("failed to unmarshal workspaces: %v", err)
	}

	if len(wsResp.Workspaces) != 2 {
		t.Fatalf("expected 2 workspaces, got %d", len(wsResp.Workspaces))
	}

	ws1 := wsResp.Workspaces[0]
	if ws1.ID != 1 || ws1.Idx != 1 || !ws1.IsFocused {
		t.Errorf("unexpected ws1 properties: %+v", ws1)
	}
	if ws1.DisplayName() != "1" {
		t.Errorf("expected DisplayName '1', got %q", ws1.DisplayName())
	}

	ws2 := wsResp.Workspaces[1]
	if ws2.DisplayName() != "term" {
		t.Errorf("expected DisplayName 'term', got %q", ws2.DisplayName())
	}
}

func TestUnmarshalWorkspacesChangedEvent(t *testing.T) {
	data := `{"WorkspacesChanged":{"workspaces":[{"id":1,"idx":1,"name":null,"output":"eDP-1","is_urgent":false,"is_active":true,"is_focused":true,"active_window_id":null}]}}`

	var ev Event
	if err := json.Unmarshal([]byte(data), &ev); err != nil {
		t.Fatalf("failed to unmarshal event: %v", err)
	}

	if ev.WorkspacesChanged == nil {
		t.Fatal("expected WorkspacesChanged event, got nil")
	}

	if len(ev.WorkspacesChanged.Workspaces) != 1 {
		t.Fatalf("expected 1 workspace, got %d", len(ev.WorkspacesChanged.Workspaces))
	}
}

func TestUnmarshalWorkspaceActivatedEvent(t *testing.T) {
	data := `{"WorkspaceActivated":{"id":2,"focused":true}}`

	var ev Event
	if err := json.Unmarshal([]byte(data), &ev); err != nil {
		t.Fatalf("failed to unmarshal event: %v", err)
	}

	if ev.WorkspaceActivated == nil {
		t.Fatal("expected WorkspaceActivated event, got nil")
	}

	if ev.WorkspaceActivated.ID != 2 || !ev.WorkspaceActivated.Focused {
		t.Errorf("unexpected activated event data: %+v", ev.WorkspaceActivated)
	}
}

func TestMarshalFocusWorkspaceAction(t *testing.T) {
	id := uint64(42)
	req := ActionRequest{
		Action: ActionPayload{
			FocusWorkspace: &FocusWorkspacePayload{
				Reference: WorkspaceReference{
					ID: &id,
				},
			},
		},
	}

	bytes, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal request: %v", err)
	}

	expected := `{"Action":{"FocusWorkspace":{"reference":{"Id":42}}}}`
	if string(bytes) != expected {
		t.Errorf("expected %s, got %s", expected, string(bytes))
	}
}

func TestUnmarshalKeyboardLayouts(t *testing.T) {
	data := `{"Ok":{"names":["English (US)","Persian"],"current_idx":1}}`

	var resp Response
	if err := json.Unmarshal([]byte(data), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	var kbResp KeyboardLayoutsResponse
	if err := json.Unmarshal(resp.Ok, &kbResp); err != nil {
		t.Fatalf("failed to unmarshal keyboard layouts: %v", err)
	}

	if len(kbResp.Names) != 2 || kbResp.CurrentIdx != 1 {
		t.Fatalf("unexpected kbResp: %+v", kbResp)
	}
	if kbResp.CurrentName() != "Persian" {
		t.Errorf("expected 'Persian', got %q", kbResp.CurrentName())
	}
}

func TestKeyboardLayoutEvents(t *testing.T) {
	ev1Data := `{"KeyboardLayoutsChanged":{"keyboard_layouts":{"names":["English (US)","German"],"current_idx":0}}}`
	var ev1 Event
	if err := json.Unmarshal([]byte(ev1Data), &ev1); err != nil {
		t.Fatalf("failed to unmarshal KeyboardLayoutsChanged: %v", err)
	}
	if ev1.KeyboardLayoutsChanged == nil {
		t.Fatal("expected KeyboardLayoutsChanged, got nil")
	}
	if len(ev1.KeyboardLayoutsChanged.KeyboardLayouts.Names) != 2 {
		t.Errorf("expected 2 names, got %d", len(ev1.KeyboardLayoutsChanged.KeyboardLayouts.Names))
	}

	ev2Data := `{"KeyboardLayoutSwitched":{"idx":1}}`
	var ev2 Event
	if err := json.Unmarshal([]byte(ev2Data), &ev2); err != nil {
		t.Fatalf("failed to unmarshal KeyboardLayoutSwitched: %v", err)
	}
	if ev2.KeyboardLayoutSwitched == nil {
		t.Fatal("expected KeyboardLayoutSwitched, got nil")
	}
	if ev2.KeyboardLayoutSwitched.Idx != 1 {
		t.Errorf("expected idx 1, got %d", ev2.KeyboardLayoutSwitched.Idx)
	}
}
