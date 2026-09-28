package controlcenter

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/godbus/dbus/v5"
)

const (
	powerProfilesDest  = "net.hadess.PowerProfiles"
	powerProfilesPath  = "/net/hadess/PowerProfiles"
	powerProfilesIFace = "net.hadess.PowerProfiles"

	ProfilePowerSaver  = "power-saver"
	ProfileBalanced    = "balanced"
	ProfilePerformance = "performance"
)

type PowerController struct {
	mu       sync.Mutex
	conn     *dbus.Conn
	profile  string
	onChange func(profile, title, icon string, active bool)
}

func NextPowerProfile(current string) string {
	switch strings.ToLower(strings.TrimSpace(current)) {
	case ProfilePowerSaver:
		return ProfileBalanced
	case ProfileBalanced:
		return ProfilePerformance
	case ProfilePerformance:
		return ProfilePowerSaver
	default:
		return ProfileBalanced
	}
}

func PowerProfileInfo(profile string) (title string, icon string, active bool) {
	switch strings.ToLower(strings.TrimSpace(profile)) {
	case ProfilePowerSaver:
		return "Power Saver", "power-profile-power-saver-symbolic", true
	case ProfilePerformance:
		return "Performance", "power-profile-performance-symbolic", true
	case ProfileBalanced:
		return "Balanced", "power-profile-balanced-symbolic", true
	default:
		if profile == "" {
			return "Unavailable", "power-profile-balanced-symbolic", false
		}
		return strings.Title(profile), "power-profile-balanced-symbolic", true
	}
}

func NewPowerController(onChange func(profile, title, icon string, active bool)) *PowerController {
	pc := &PowerController{
		onChange: onChange,
		profile:  ProfileBalanced,
	}
	return pc
}

func (pc *PowerController) Start(ctx context.Context) {
	conn, err := dbus.SystemBus()
	if err != nil {
		// System bus not available (e.g. mock / test env)
		if pc.onChange != nil {
			glib.IdleAdd(func() {
				title, icon, active := PowerProfileInfo("")
				pc.onChange("", title, icon, active)
			})
		}
		return
	}
	pc.mu.Lock()
	pc.conn = conn
	pc.mu.Unlock()

	// Query initial profile
	obj := conn.Object(powerProfilesDest, powerProfilesPath)
	val, err := obj.GetProperty(powerProfilesIFace + ".ActiveProfile")
	if err == nil {
		if s, ok := val.Value().(string); ok && s != "" {
			pc.mu.Lock()
			pc.profile = s
			pc.mu.Unlock()
			if pc.onChange != nil {
				title, icon, active := PowerProfileInfo(s)
				glib.IdleAdd(func() {
					pc.onChange(s, title, icon, active)
				})
			}
		}
	} else {
		// power-profiles-daemon might not be installed
		if pc.onChange != nil {
			title, icon, active := PowerProfileInfo("")
			glib.IdleAdd(func() {
				pc.onChange("", title, icon, active)
			})
		}
	}

	// Zero-polling subscription: listen for PropertiesChanged on /net/hadess/PowerProfiles
	rule := fmt.Sprintf("type='signal',interface='org.freedesktop.DBus.Properties',member='PropertiesChanged',path='%s'", powerProfilesPath)
	conn.BusObject().Call("org.freedesktop.DBus.AddMatch", 0, rule)

	ch := make(chan *dbus.Signal, 10)
	conn.Signal(ch)

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case sig, ok := <-ch:
				if !ok {
					return
				}
				if sig.Path == powerProfilesPath && len(sig.Body) >= 2 {
					if iface, ok := sig.Body[0].(string); ok && iface == powerProfilesIFace {
						if changed, ok := sig.Body[1].(map[string]dbus.Variant); ok {
							if v, exists := changed["ActiveProfile"]; exists {
								if s, ok := v.Value().(string); ok {
									pc.mu.Lock()
									pc.profile = s
									pc.mu.Unlock()
									if pc.onChange != nil {
										title, icon, active := PowerProfileInfo(s)
										glib.IdleAdd(func() {
											pc.onChange(s, title, icon, active)
										})
									}
								}
							}
						}
					}
				}
			}
		}
	}()
}

func (pc *PowerController) CurrentProfile() string {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	return pc.profile
}

func (pc *PowerController) SetProfile(profile string) {
	profile = strings.ToLower(strings.TrimSpace(profile))
	if profile != ProfilePowerSaver && profile != ProfileBalanced && profile != ProfilePerformance {
		return
	}

	pc.mu.Lock()
	conn := pc.conn
	pc.mu.Unlock()

	if conn != nil {
		obj := conn.Object(powerProfilesDest, powerProfilesPath)
		_ = obj.SetProperty(powerProfilesIFace+".ActiveProfile", dbus.MakeVariant(profile))
	} else {
		pc.mu.Lock()
		pc.profile = profile
		pc.mu.Unlock()
		if pc.onChange != nil {
			title, icon, active := PowerProfileInfo(profile)
			glib.IdleAdd(func() {
				pc.onChange(profile, title, icon, active)
			})
		}
	}
}

func (pc *PowerController) Toggle() {
	pc.mu.Lock()
	current := pc.profile
	pc.mu.Unlock()

	next := NextPowerProfile(current)
	pc.SetProfile(next)
}

