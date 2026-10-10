package ipc

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// usageText is the phalune msg help output.
const usageText = `Usage: phalune msg [flags] <command> [args...]

Flags:
  -j, --json                  Format output as JSON

Shell control:
  status                      Show shell status summary
  toggle-launcher             Toggle application launcher
  open-launcher               Open application launcher
  close-launcher              Close application launcher
  toggle-control-center       Toggle control center (quick settings)
  open-control-center         Open control center
  close-control-center        Close control center
  toggle-notifications        Toggle notification center
  open-notifications          Open notification center
  close-notifications         Close notification center
  toggle-power-menu           Toggle power menu overlay
  open-power-menu             Open power menu overlay
  close-power-menu            Close power menu overlay
  toggle-clipboard            Toggle clipboard history overlay
  open-clipboard              Open clipboard history overlay
  close-clipboard             Close clipboard history overlay
  window-switcher [next|prev|close]  Alt-Tab window switcher (opens/steps)
  toggle-window-switcher      Toggle window switcher overlay
  next-window                 Switch to next window
  prev-window                 Switch to previous window
  lock                        Lock session and show lock screen
  unlock                      Unlock session
  is-locked                   Check if screen is locked
  suspend                     Suspend computer
  hibernate                   Hibernate computer
  reboot                      Restart computer
  poweroff                    Power off computer
  logout                      Log out of current session

Audio and display:
  volume-up [step]            Increase speaker volume (default 5%)
  volume-down [step]          Decrease speaker volume (default 5%)
  volume-mute                 Toggle speaker mute
  volume-set <val>            Set speaker volume (0-100 or 0-150)
  brightness-up [step]        Increase screen brightness (default 5%)
  brightness-down [step]      Decrease screen brightness (default 5%)
  brightness-set <val>        Set screen brightness (0-100)

Overlays and toasts:
  osd <type> <value>          Show OSD (e.g. 'osd volume 75' or 'osd brightness 50')
  screenshot [mode]           Take screenshot (area, window, display)
  test-osd                    Show sample volume OSD
  notify <summary> [body]     Show notification toast
  test-notify                 Show sample notification toast

IPC widgets (Tier 2):
  widget push --id=<id> [--text=...] [--class=...] [--tooltip=...] [--icon=...] [--percentage=N]
                              Push bar widget state (text supports Pango markup)
  widget clear --id=<id>      Hide a pushed bar widget
  widget watch --id=<id>      Claim a widget for this process; push JSON states
                              on stdin (or "clear"), receive click events on stdout
  subscribe [--events=t1,t2]  Stream shell events (NDJSON); default: all topics

Reloading and logs:
  reload-config               Reload configuration and apply changes
  reload-style                Reload CSS stylesheet
  set-log-level <level> [notify_level]  Set console and optional notify log level
  toggle-notify-logs [on|off] Toggle forwarding logs to desktop notifications
  ping                        Ping running phalune daemon
`

// RunClient is the entry point of "phalune msg".
func RunClient(args []string) int {
	jsonOutput := false
	filtered := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == "--json" || arg == "-j" {
			jsonOutput = true
		} else {
			filtered = append(filtered, arg)
		}
	}
	args = filtered

	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		fmt.Print(usageText)
		return 0
	}

	if args[0] == "widget" && len(args) > 1 {
		return runWidgetCommand(jsonOutput, args[1:])
	}
	if args[0] == ActionSubscribeEvents {
		return runSubscribe(jsonOutput, args[1:])
	}

	action, reqArgs := parseOneShot(args)
	resp, err := SendCommand("", action, reqArgs)
	return printResult(jsonOutput, resp, err)
}

// runWidgetCommand handles "phalune msg widget push|clear|watch".
func runWidgetCommand(jsonOutput bool, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: phalune msg widget push|clear|watch ...")
		return 2
	}

	flags := parseKeyValues(args[1:])
	sub := args[0]

	switch sub {
	case "push":
		if id := flags["id"]; id == "" {
			fmt.Fprintln(os.Stderr, "error: widget push requires --id")
			return 2
		}
		st := WidgetState{
			ID:      flags["id"],
			Text:    flags["text"],
			Tooltip: flags["tooltip"],
			Class:   flags["class"],
			Icon:    flags["icon"],
		}
		if raw := flags["percentage"]; raw != "" {
			pct, ok := parseFloatArg(raw)
			if !ok {
				fmt.Fprintf(os.Stderr, "error: invalid percentage %q\n", raw)
				return 2
			}
			st.Percent = &pct
		}
		resp, err := PushWidget("", st)
		return printResult(jsonOutput, resp, err)

	case "clear":
		if id := flags["id"]; id == "" {
			fmt.Fprintln(os.Stderr, "error: widget clear requires --id")
			return 2
		}
		resp, err := ClearWidget("", flags["id"])
		return printResult(jsonOutput, resp, err)

	case ActionWidgetWatch:
		if id := flags["id"]; id == "" {
			fmt.Fprintln(os.Stderr, "error: widget watch requires --id")
			return 2
		}
		if err := WatchWidget("", flags["id"]); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
		return 0

	default:
		fmt.Fprintf(os.Stderr, "unknown widget command %q (push, clear, watch)\n", sub)
		return 2
	}
}

// runSubscribe streams events until interrupt.
func runSubscribe(jsonOutput bool, args []string) int {
	flags := parseKeyValues(args)
	var patterns []string
	if raw := flags["events"]; raw != "" {
		patterns = ParseEventTopics(raw)
	}

	if err := StreamEvents("", patterns); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	return 0
}

// parseKeyValues converts --key=value (or --key value) args into a map.
func parseKeyValues(args []string) map[string]string {
	flags := make(map[string]string)
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "--") {
			continue
		}
		kv := strings.TrimPrefix(arg, "--")
		if k, v, ok := strings.Cut(kv, "="); ok {
			flags[k] = v
			continue
		}
		if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
			flags[kv] = args[i+1]
			i++
		} else {
			flags[kv] = "true"
		}
	}
	return flags
}

// parseOneShot maps CLI argv onto a socket action + args for the legacy
// request surface.
func parseOneShot(args []string) (string, map[string]string) {
	action := args[0]
	args2 := make(map[string]string)

	aliases := map[string]string{
		"alt-tab":       ActionWindowSwitcher,
		"switch-window": ActionWindowSwitcher,
		"sleep":         ActionSuspend,
		"restart":       ActionReboot,
		"shutdown":      ActionPowerOff,
		"exit-session":  ActionLogout,
		"lock-screen":   ActionLock,
		"shell-status":  ActionStatus,
		"osd-test":      ActionTestOSD,
		"notify-test":   ActionTestNotify,
	}
	if canonical, ok := aliases[action]; ok {
		action = canonical
	}

	switch action {
	case ActionWindowSwitcher:
		if len(args) > 1 {
			args2["action"] = args[1]
		}
	case ActionSetLogLevel:
		if len(args) > 1 {
			args2["level"] = args[1]
		}
		if len(args) > 2 {
			args2["notify_level"] = args[2]
		}
	case ActionToggleNotifyLogs:
		if len(args) > 1 {
			args2["state"] = args[1]
		}
	case ActionScreenshot:
		if len(args) > 1 {
			args2["mode"] = args[1]
		}
	case ActionShowOSD:
		if len(args) > 1 {
			args2["type"] = args[1]
		}
		if len(args) > 2 {
			args2["value"] = args[2]
		}
	case ActionNotify:
		if len(args) > 1 {
			args2["summary"] = args[1]
		}
		if len(args) > 2 {
			args2["body"] = args[2]
		}
		if len(args) > 3 {
			args2["icon"] = args[3]
		}
	case ActionVolumeUp, ActionVolumeDown:
		if len(args) > 1 {
			args2["step"] = args[1]
		}
	case ActionVolumeSet:
		if len(args) > 1 {
			args2["value"] = args[1]
		}
	case ActionBrightnessUp, ActionBrightnessDown:
		if len(args) > 1 {
			args2["step"] = args[1]
		}
	case ActionBrightnessSet:
		if len(args) > 1 {
			args2["value"] = args[1]
		}
	}
	return action, args2
}

// printResult renders a Response for the CLI.
func printResult(jsonOutput bool, resp *Response, err error) int {
	if err != nil {
		if jsonOutput {
			fmt.Println(mustJSONIndent(map[string]any{"ok": false, "error": err.Error()}))
		} else {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
		}
		return 1
	}
	if !resp.OK {
		errStr := resp.Error
		if errStr == "" {
			errStr = "command failed"
		}
		if jsonOutput {
			fmt.Println(mustJSONIndent(map[string]any{"ok": false, "error": errStr}))
		} else {
			fmt.Fprintf(os.Stderr, "error: %s\n", errStr)
		}
		return 1
	}

	if jsonOutput {
		if len(resp.Data) > 0 {
			var obj map[string]any
			merged := map[string]any{"ok": true}
			if err := json.Unmarshal(resp.Data, &obj); err == nil {
				for k, v := range obj {
					merged[k] = v
				}
				fmt.Println(mustJSONIndent(merged))
				return 0
			}
			merged["data"] = json.RawMessage(resp.Data)
			fmt.Println(mustJSONIndent(merged))
			return 0
		}
		fmt.Println(mustJSONIndent(map[string]any{"ok": true, "message": resp.Message}))
		return 0
	}

	if resp.Message != "" {
		fmt.Println(resp.Message)
	}
	return 0
}

func mustJSONIndent(v any) string {
	out, _ := json.MarshalIndent(v, "", "  ")
	return string(out)
}
