package niri

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
)

type Client struct {
	socketPath string
}

func NewClient(socketPath string) (*Client, error) {
	path := socketPath
	if path == "" {
		path = os.Getenv("NIRI_SOCKET")
		if path == "" {
			return nil, fmt.Errorf("NIRI_SOCKET not set")
		}
	}
	return &Client{socketPath: path}, nil
}

func (c *Client) connect() (net.Conn, error) {
	conn, err := net.Dial("unix", c.socketPath)
	if err != nil {
		return nil, fmt.Errorf("connect %q: %w", c.socketPath, err)
	}
	return conn, nil
}

func (c *Client) QueryWorkspaces() ([]Workspace, error) {
	conn, err := c.connect()
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	if _, err := conn.Write([]byte("\"Workspaces\"\n")); err != nil {
		return nil, fmt.Errorf("write workspaces request: %w", err)
	}

	reader := bufio.NewReader(conn)
	line, err := reader.ReadBytes('\n')
	if err != nil {
		return nil, fmt.Errorf("read workspaces response: %w", err)
	}

	var resp Response
	if err := json.Unmarshal(line, &resp); err != nil {
		return nil, fmt.Errorf("decode niri response: %w", err)
	}

	if resp.Err != "" {
		return nil, fmt.Errorf("niri error: %s", resp.Err)
	}

	var wsResp WorkspacesResponse
	if err := json.Unmarshal(resp.Ok, &wsResp); err != nil {
		return nil, fmt.Errorf("decode workspaces: %w", err)
	}

	return wsResp.Workspaces, nil
}

func (c *Client) FocusWorkspace(id uint64) error {
	conn, err := c.connect()
	if err != nil {
		return err
	}
	defer conn.Close()

	req := ActionRequest{
		Action: ActionPayload{
			FocusWorkspace: &FocusWorkspacePayload{
				Reference: WorkspaceReference{ID: &id},
			},
		},
	}

	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("encode focus request: %w", err)
	}

	if _, err := conn.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write focus request: %w", err)
	}

	reader := bufio.NewReader(conn)
	line, err := reader.ReadBytes('\n')
	if err != nil {
		return fmt.Errorf("read focus response: %w", err)
	}

	var resp Response
	if err := json.Unmarshal(line, &resp); err != nil {
		return fmt.Errorf("decode focus response: %w", err)
	}

	if resp.Err != "" {
		return fmt.Errorf("focus workspace %d: %s", id, resp.Err)
	}

	return nil
}

func (c *Client) OpenEventStream() (net.Conn, *bufio.Scanner, error) {
	conn, err := c.connect()
	if err != nil {
		return nil, nil, err
	}

	if _, err := conn.Write([]byte("\"EventStream\"\n")); err != nil {
		conn.Close()
		return nil, nil, fmt.Errorf("initiate event stream: %w", err)
	}

	return conn, bufio.NewScanner(conn), nil
}

func (c *Client) QueryKeyboardLayouts() (*KeyboardLayouts, error) {
	conn, err := c.connect()
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	if _, err := conn.Write([]byte("\"KeyboardLayouts\"\n")); err != nil {
		return nil, fmt.Errorf("write keyboard layouts request: %w", err)
	}

	reader := bufio.NewReader(conn)
	line, err := reader.ReadBytes('\n')
	if err != nil {
		return nil, fmt.Errorf("read keyboard layouts response: %w", err)
	}

	var resp Response
	if err := json.Unmarshal(line, &resp); err != nil {
		return nil, fmt.Errorf("decode niri response: %w", err)
	}

	if resp.Err != "" {
		return nil, fmt.Errorf("niri error: %s", resp.Err)
	}

	var kbResp KeyboardLayoutsResponse
	if err := json.Unmarshal(resp.Ok, &kbResp); err != nil {
		return nil, fmt.Errorf("decode keyboard layouts: %w", err)
	}

	return &kbResp, nil
}

func (c *Client) SwitchLayoutNext() error {
	conn, err := c.connect()
	if err == nil {
		defer conn.Close()
		req := map[string]any{
			"Action": map[string]any{
				"SwitchLayout": map[string]any{
					"layout": "Next",
				},
			},
		}
		data, mErr := json.Marshal(req)
		if mErr == nil {
			if _, wErr := conn.Write(append(data, '\n')); wErr == nil {
				reader := bufio.NewReader(conn)
				line, rErr := reader.ReadBytes('\n')
				if rErr == nil {
					var resp Response
					if json.Unmarshal(line, &resp) == nil && resp.Err == "" {
						return nil
					}
				}
			}
		}
	}

	// Fallback to niri CLI
	cmd := exec.Command("niri", "msg", "action", "switch-layout", "next")
	return cmd.Run()
}

func (c *Client) QueryWindows() ([]Window, error) {
	conn, err := c.connect()
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	if _, err := conn.Write([]byte("\"Windows\"\n")); err != nil {
		return nil, fmt.Errorf("write windows request: %w", err)
	}

	reader := bufio.NewReader(conn)
	line, err := reader.ReadBytes('\n')
	if err != nil {
		return nil, fmt.Errorf("read windows response: %w", err)
	}

	var resp Response
	if err := json.Unmarshal(line, &resp); err != nil {
		return nil, fmt.Errorf("decode niri response: %w", err)
	}

	if resp.Err != "" {
		return nil, fmt.Errorf("niri error: %s", resp.Err)
	}

	var winResp WindowsResponse
	if err := json.Unmarshal(resp.Ok, &winResp); err != nil {
		return nil, fmt.Errorf("decode windows: %w", err)
	}

	return winResp.Windows, nil
}

func (c *Client) FocusWindow(id uint64) error {
	conn, err := c.connect()
	if err != nil {
		return err
	}
	defer conn.Close()

	req := ActionRequest{
		Action: ActionPayload{
			FocusWindow: &FocusWindowPayload{
				ID: id,
			},
		},
	}

	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("encode focus window request: %w", err)
	}

	if _, err := conn.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write focus window request: %w", err)
	}

	reader := bufio.NewReader(conn)
	line, err := reader.ReadBytes('\n')
	if err != nil {
		return fmt.Errorf("read focus window response: %w", err)
	}

	var resp Response
	if err := json.Unmarshal(line, &resp); err != nil {
		return fmt.Errorf("decode focus window response: %w", err)
	}

	if resp.Err != "" {
		return fmt.Errorf("focus window %d: %s", id, resp.Err)
	}

	return nil
}
