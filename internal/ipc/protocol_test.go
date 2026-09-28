package ipc

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIPCServerClient(t *testing.T) {
	sockDir := t.TempDir()
	sockPath := filepath.Join(sockDir, "test.sock")

	handler := func(req Request) Response {
		switch req.Action {
		case ActionPing:
			return Response{OK: true, Message: "pong"}
		case ActionToggleLauncher:
			return Response{OK: true, Message: "toggled"}
		case ActionShowOSD:
			return Response{OK: true, Message: req.Args["type"] + ":" + req.Args["value"]}
		case ActionNotify:
			return Response{OK: true, Message: req.Args["summary"]}
		case ActionLock:
			return Response{OK: true, Message: "locked"}
		case ActionTogglePowerMenu:
			return Response{OK: true, Message: "power menu toggled"}
		default:
			return Response{OK: false, Error: "unknown action"}
		}
	}

	server, err := NewServer(sockPath, handler)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	defer server.Close()

	if _, err := os.Stat(sockPath); err != nil {
		t.Fatalf("socket file does not exist: %v", err)
	}

	resp, err := SendCommand(sockPath, ActionPing, nil)
	if err != nil {
		t.Fatalf("SendCommand ping failed: %v", err)
	}
	if !resp.OK || resp.Message != "pong" {
		t.Errorf("unexpected ping response: %+v", resp)
	}

	resp, err = SendCommand(sockPath, ActionToggleLauncher, nil)
	if err != nil {
		t.Fatalf("SendCommand toggle-launcher failed: %v", err)
	}
	if !resp.OK || resp.Message != "toggled" {
		t.Errorf("unexpected toggle response: %+v", resp)
	}

	resp, err = SendCommand(sockPath, ActionShowOSD, map[string]string{"type": "volume", "value": "50"})
	if err != nil {
		t.Fatalf("SendCommand osd failed: %v", err)
	}
	if !resp.OK || resp.Message != "volume:50" {
		t.Errorf("unexpected osd response: %+v", resp)
	}

	resp, err = SendCommand(sockPath, ActionNotify, map[string]string{"summary": "hello"})
	if err != nil {
		t.Fatalf("SendCommand notify failed: %v", err)
	}
	if !resp.OK || resp.Message != "hello" {
		t.Errorf("unexpected notify response: %+v", resp)
	}

	resp, err = SendCommand(sockPath, ActionLock, nil)
	if err != nil {
		t.Fatalf("SendCommand lock failed: %v", err)
	}

	resp, err = SendCommand(sockPath, ActionTogglePowerMenu, nil)
	if err != nil {
		t.Fatalf("SendCommand toggle-power-menu failed: %v", err)
	}

	resp, err = SendCommand(sockPath, "unknown", nil)
	if err != nil {
		t.Fatalf("SendCommand unknown failed: %v", err)
	}
	if resp.OK || resp.Error != "unknown action" {
		t.Errorf("expected error for unknown action, got %+v", resp)
	}
}
