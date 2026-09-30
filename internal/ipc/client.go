package ipc

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
)

func SendCommand(socketPath string, action string, args map[string]string) (*Response, error) {
	if socketPath == "" {
		socketPath = DefaultSocketPath()
	}

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("could not connect to phalune socket at %s: %w", socketPath, err)
	}
	defer conn.Close()

	req := Request{
		Action: action,
		Args:   args,
	}

	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to encode request: %w", err)
	}

	data = append(data, '\n')
	if _, err := conn.Write(data); err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return nil, fmt.Errorf("failed to read response: %w", err)
		}
		return nil, fmt.Errorf("empty response from server")
	}

	var resp Response
	if err := json.Unmarshal(scanner.Bytes(), &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &resp, nil
}

func RunClient(args []string) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		fmt.Println("Usage: phalune msg <command> [args...]")
		fmt.Println()
		fmt.Println("Commands:")
		fmt.Println("  toggle-launcher             Toggle application launcher")
		fmt.Println("  open-launcher               Open application launcher")
		fmt.Println("  close-launcher              Close application launcher")
		fmt.Println("  toggle-control-center       Toggle control center (quick settings)")
		fmt.Println("  open-control-center         Open control center")
		fmt.Println("  close-control-center        Close control center")
		fmt.Println("  toggle-notification-center  Toggle notification center")
		fmt.Println("  open-notification-center    Open notification center")
		fmt.Println("  close-notification-center   Close notification center")
		fmt.Println("  toggle-power-menu           Toggle power menu overlay")
		fmt.Println("  open-power-menu             Open power menu overlay")
		fmt.Println("  toggle-clipboard            Toggle clipboard history overlay")
		fmt.Println("  open-clipboard              Open clipboard history overlay")
		fmt.Println("  close-clipboard             Close clipboard history overlay")
		fmt.Println("  window-switcher [next|prev|close]  Alt-Tab window switcher (opens/steps)")
		fmt.Println("  toggle-window-switcher      Toggle window switcher overlay")
		fmt.Println("  open-window-switcher        Open window switcher overlay")
		fmt.Println("  close-window-switcher       Close window switcher overlay")
		fmt.Println("  next-window                 Switch to next window")
		fmt.Println("  prev-window                 Switch to previous window")
		fmt.Println("  lock                        Lock session and show lock screen")
		fmt.Println("  unlock                      Unlock session")
		fmt.Println("  is-locked                   Check if screen is locked")
		fmt.Println("  suspend                     Suspend computer")
		fmt.Println("  hibernate                   Hibernate computer")
		fmt.Println("  reboot                      Restart computer")
		fmt.Println("  poweroff                    Power off computer")
		fmt.Println("  logout                      Log out of current session")
		fmt.Println("  osd <type> <value>          Show OSD (e.g. 'osd volume 75' or 'osd brightness 50')")
		fmt.Println("  screenshot [mode]           Take screenshot (area, window, display)")
		fmt.Println("  test-osd                    Show sample volume OSD")
		fmt.Println("  notify <summary> [body]     Show notification toast")
		fmt.Println("  test-notify                 Show sample notification toast")
		fmt.Println("  reload-config               Reload configuration and apply changes")
		fmt.Println("  reload-style                Reload CSS stylesheet")
		fmt.Println("  set-log-level <level> [notify_level]  Set console and optional notify log level")
		fmt.Println("  toggle-notify-logs [on|off] Toggle forwarding logs to desktop notifications")
		fmt.Println("  ping                        Ping running phalune daemon")
		return 0
	}

	action := args[0]
	var reqArgs map[string]string

	switch action {
	case ActionWindowSwitcher, "alt-tab", "switch-window":
		reqArgs = make(map[string]string)
		if len(args) > 1 {
			reqArgs["action"] = args[1]
		}
	case ActionSetLogLevel:
		reqArgs = make(map[string]string)
		if len(args) > 1 {
			reqArgs["level"] = args[1]
		}
		if len(args) > 2 {
			reqArgs["notify_level"] = args[2]
		}
	case ActionToggleNotifyLogs:
		reqArgs = make(map[string]string)
		if len(args) > 1 {
			reqArgs["state"] = args[1]
		}
	case ActionScreenshot:
		reqArgs = make(map[string]string)
		if len(args) > 1 {
			reqArgs["mode"] = args[1]
		}
	case ActionShowOSD:
		reqArgs = make(map[string]string)
		if len(args) > 1 {
			reqArgs["type"] = args[1]
		}
		if len(args) > 2 {
			reqArgs["value"] = args[2]
		}
	case ActionNotify:
		reqArgs = make(map[string]string)
		if len(args) > 1 {
			reqArgs["summary"] = args[1]
		}
		if len(args) > 2 {
			reqArgs["body"] = args[2]
		}
		if len(args) > 3 {
			reqArgs["icon"] = args[3]
		}
	}

	resp, err := SendCommand("", action, reqArgs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	if !resp.OK {
		if resp.Error != "" {
			fmt.Fprintf(os.Stderr, "error: %s\n", resp.Error)
		} else {
			fmt.Fprintf(os.Stderr, "command failed\n")
		}
		return 1
	}

	if resp.Message != "" {
		fmt.Println(resp.Message)
	}
	return 0
}
