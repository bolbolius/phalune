package launcher

import (
	"fmt"
	"sort"
	"strings"
)

const (
	CommandReboot    = "reboot"
	CommandPowerOff  = "poweroff"
	CommandSuspend   = "suspend"
	CommandHibernate = "hibernate"
	CommandLogout    = "logout"
	CommandLock      = "lock"
	CommandReload    = "reload"
	CommandPowerMenu = "power-menu"
	CommandClipboard = "clipboard"
)

type ShellCommands struct {
	Reboot    func() error
	PowerOff  func() error
	Suspend   func() error
	Hibernate func() error
	Logout    func() error
	Lock       func()
	Reload     func()
	PowerMenu  func()
	Clipboard  func()
	Screenshot func()
}

type commandEntry struct {
	Name        string
	Description string
	Icon        string
	Aliases     []string
	Run         func() error
}

func commandIcon(name string) string {
	switch name {
	case "reboot":
		return "system-reboot-symbolic"
	case "poweroff", "shutdown":
		return "system-shutdown-symbolic"
	case "suspend":
		return "weather-clear-night-symbolic"
	case "hibernate":
		return "media-playback-pause-symbolic"
	case "logout":
		return "system-log-out-symbolic"
	case "lock":
		return "system-lock-screen-symbolic"
	case "reload":
		return "view-refresh-symbolic"
	case "power-menu":
		return "system-shutdown-symbolic"
	case "clipboard":
		return "edit-paste-symbolic"
	default:
		return "application-x-executable-symbolic"
	}
}

func commandList(svc *ShellCommands) []commandEntry {
	if svc == nil {
		return nil
	}

	entries := []commandEntry{
		{CommandReboot, "Restart computer", "", []string{"restart"}, wrapErr(svc.Reboot)},
		{CommandPowerOff, "Shut down computer", "", []string{"shutdown", "halt"}, wrapErr(svc.PowerOff)},
		{CommandSuspend, "Suspend to RAM", "", []string{"sleep"}, wrapErr(svc.Suspend)},
		{CommandHibernate, "Suspend to disk", "", []string{}, wrapErr(svc.Hibernate)},
		{CommandLogout, "End session", "", []string{}, wrapErr(svc.Logout)},
		{CommandLock, "Lock screen", "", []string{}, wrapVoid(svc.Lock)},
		{CommandReload, "Reload shell configuration", "", []string{}, wrapVoid(svc.Reload)},
		{CommandPowerMenu, "Open power menu", "", []string{"powermenu"}, wrapVoid(svc.PowerMenu)},
		{CommandClipboard, "Clipboard history", "", []string{"paste", "clips"}, wrapVoid(svc.Clipboard)},
	}

	for i := range entries {
		if entries[i].Icon == "" {
			entries[i].Icon = commandIcon(entries[i].Name)
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})
	return entries
}

func wrapErr(fn func() error) func() error {
	if fn == nil {
		return func() error { return fmt.Errorf("not available") }
	}
	return fn
}

func wrapVoid(fn func()) func() error {
	if fn == nil {
		return func() error { return fmt.Errorf("not available") }
	}
	return func() error {
		fn()
		return nil
	}
}

func matchCommand(entry commandEntry, query string) int {
	q := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(query, ":")))
	if q == "" {
		return 1
	}

	best := 0
	check := func(candidate string, weight int) {
		if s := commandMatchScore(q, candidate); s > 0 {
			weighted := weight + s
			if weighted > best {
				best = weighted
			}
		}
	}

	check(entry.Name, 40)
	if strings.HasPrefix(entry.Name, q) {
		if best < 100 {
			best = 100
		}
	}
	for _, alias := range entry.Aliases {
		check(alias, 25)
	}
	if descScore := commandMatchScore(q, strings.ToLower(entry.Description)); descScore > 0 && best < 20+descScore {
		best = 20 + descScore
	}
	return best
}

func commandMatchScore(query, candidate string) int {
	if candidate == "" {
		return 0
	}
	if query == candidate {
		return 50
	}
	if strings.HasPrefix(candidate, query) {
		return 30
	}
	if strings.Contains(candidate, query) {
		return 10
	}
	if FuzzyScore(query, candidate) > 0 {
		return 5
	}
	return 0
}
