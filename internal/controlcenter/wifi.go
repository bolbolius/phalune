package controlcenter

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/godbus/dbus/v5"
)

const (
	nmDest  = "org.freedesktop.NetworkManager"
	nmPath  = "/org/freedesktop/NetworkManager"
	nmIFace = "org.freedesktop.NetworkManager"
)

type AccessPoint struct {
	SSID      string
	Strength  uint8
	Secured   bool
	Connected bool
	Saved     bool
	BSSID     string
	Path      dbus.ObjectPath
}

func WifiSignalIcon(strength uint8) string {
	switch {
	case strength >= 75:
		return "network-wireless-signal-excellent-symbolic"
	case strength >= 50:
		return "network-wireless-signal-good-symbolic"
	case strength >= 25:
		return "network-wireless-signal-ok-symbolic"
	default:
		return "network-wireless-signal-weak-symbolic"
	}
}

type WiFiController struct {
	mu             sync.Mutex
	conn           *dbus.Conn
	devicePath     dbus.ObjectPath
	deviceIface    string
	enabled        bool
	available      bool
	ssid           string
	activeStrength uint8
	networks       []AccessPoint
	savedSSIDs     map[string]bool
	savedSSIDsTime time.Time
	debounceTimer  *time.Timer
	debounceMu     sync.Mutex
	onChange       func(enabled bool, available bool, subtitle, icon string)
	onNetworks     func(networks []AccessPoint)
}

func NewWiFiController(onChange func(enabled bool, available bool, subtitle, icon string)) *WiFiController {
	return &WiFiController{
		onChange: onChange,
	}
}

func (wc *WiFiController) SetOnNetworksChanged(fn func(networks []AccessPoint)) {
	wc.mu.Lock()
	wc.onNetworks = fn
	wc.mu.Unlock()
}

func (wc *WiFiController) Start(ctx context.Context) {
	conn, err := dbus.SystemBus()
	if err != nil {
		slog.Debug("wifi: system bus unavailable", "error", err)
		if wc.onChange != nil {
			glib.IdleAdd(func() {
				wc.onChange(false, false, "Unavailable", "network-wireless-symbolic")
			})
		}
		return
	}

	wc.mu.Lock()
	wc.conn = conn
	wc.mu.Unlock()

	wc.updateDevice(conn)
	wc.Refresh()
	go func() {
		time.Sleep(300 * time.Millisecond)
		wc.Scan()
	}()

	// Zero-polling subscriptions:
	// 1. Listen for PropertiesChanged on /org/freedesktop/NetworkManager
	ruleNM := fmt.Sprintf("type='signal',interface='org.freedesktop.DBus.Properties',member='PropertiesChanged',path='%s'", nmPath)
	conn.BusObject().Call("org.freedesktop.DBus.AddMatch", 0, ruleNM)

	// 2. Listen for Device changes if wireless device found
	wc.mu.Lock()
	devPath := wc.devicePath
	wc.mu.Unlock()

	if devPath != "" {
		ruleDev := fmt.Sprintf("type='signal',interface='org.freedesktop.NetworkManager.Device.Wireless',path='%s'", devPath)
		conn.BusObject().Call("org.freedesktop.DBus.AddMatch", 0, ruleDev)
		ruleDevProps := fmt.Sprintf("type='signal',interface='org.freedesktop.DBus.Properties',member='PropertiesChanged',path='%s'", devPath)
		conn.BusObject().Call("org.freedesktop.DBus.AddMatch", 0, ruleDevProps)
	}

	// 3. Listen for Settings changes (connections added/removed/updated)
	ruleSettings := "type='signal',interface='org.freedesktop.NetworkManager.Settings',path='/org/freedesktop/NetworkManager/Settings'"
	conn.BusObject().Call("org.freedesktop.DBus.AddMatch", 0, ruleSettings)

	ch := make(chan *dbus.Signal, 64)
	conn.Signal(ch)

	scheduleRefresh := func(full bool) {
		wc.debounceMu.Lock()
		defer wc.debounceMu.Unlock()
		if wc.debounceTimer != nil {
			wc.debounceTimer.Stop()
		}
		wc.debounceTimer = time.AfterFunc(180*time.Millisecond, func() {
			if full {
				wc.Refresh()
			} else {
				wc.quickRefresh()
			}
		})
	}

	go func() {
		defer conn.RemoveSignal(ch)
		for {
			select {
			case <-ctx.Done():
				return
			case sig, ok := <-ch:
				if !ok {
					return
				}
				if strings.Contains(string(sig.Path), "Settings") {
					wc.mu.Lock()
					wc.savedSSIDs = nil
					wc.mu.Unlock()
					scheduleRefresh(true)
				} else if sig.Path == nmPath {
					wc.updateDevice(conn)
					scheduleRefresh(false)
				} else {
					wc.mu.Lock()
					curDev := wc.devicePath
					wc.mu.Unlock()
					if curDev != "" && sig.Path == curDev {
						if strings.Contains(sig.Name, "AccessPoint") {
							scheduleRefresh(true)
						} else {
							scheduleRefresh(false)
						}
					}
				}
			}
		}
	}()
}

func (wc *WiFiController) updateDevice(conn *dbus.Conn) {
	obj := conn.Object(nmDest, nmPath)
	val, err := obj.GetProperty(nmIFace + ".AllDevices")
	if err != nil {
		return
	}
	paths, ok := val.Value().([]dbus.ObjectPath)
	if !ok {
		return
	}

	var foundPath dbus.ObjectPath
	var foundIface string

	for _, p := range paths {
		devObj := conn.Object(nmDest, p)
		typeVal, err := devObj.GetProperty("org.freedesktop.NetworkManager.Device.DeviceType")
		if err != nil {
			continue
		}
		if t, ok := typeVal.Value().(uint32); ok && t == 2 { // NM_DEVICE_TYPE_WIFI = 2
			ifaceVal, err := devObj.GetProperty("org.freedesktop.NetworkManager.Device.Interface")
			if err == nil {
				foundIface, _ = ifaceVal.Value().(string)
			}
			foundPath = p
			break
		}
	}

	wc.mu.Lock()
	wc.devicePath = foundPath
	wc.deviceIface = foundIface
	wc.mu.Unlock()
}

func (wc *WiFiController) Refresh() {
	wc.mu.Lock()
	conn := wc.conn
	devPath := wc.devicePath
	wc.mu.Unlock()

	if conn == nil {
		return
	}

	obj := conn.Object(nmDest, nmPath)
	val, err := obj.GetProperty(nmIFace + ".WirelessEnabled")
	if err != nil {
		wc.mu.Lock()
		wc.available = false
		wc.mu.Unlock()
		wc.notify()
		return
	}

	enabled, _ := val.Value().(bool)

	var activeSSID string
	var activeStrength uint8
	var networks []AccessPoint

	if enabled {
		if devPath == "" {
			wc.updateDevice(conn)
			wc.mu.Lock()
			devPath = wc.devicePath
			wc.mu.Unlock()
		}
		activeSSID = wc.queryActiveWiFiSSID(conn)
		if devPath != "" {
			networks = wc.queryAccessPoints(conn, devPath, activeSSID)
			for _, ap := range networks {
				if ap.Connected {
					activeStrength = ap.Strength
					break
				}
			}
		}
	}

	wc.mu.Lock()
	wc.enabled = enabled
	wc.available = (devPath != "")
	wc.ssid = activeSSID
	wc.activeStrength = activeStrength
	wc.networks = networks
	wc.mu.Unlock()

	wc.notify()
}

func (wc *WiFiController) queryActiveWiFiSSID(conn *dbus.Conn) string {
	obj := conn.Object(nmDest, nmPath)
	val, err := obj.GetProperty(nmIFace + ".ActiveConnections")
	if err != nil {
		return ""
	}

	paths, ok := val.Value().([]dbus.ObjectPath)
	if !ok {
		return ""
	}

	for _, p := range paths {
		connObj := conn.Object(nmDest, p)
		typeVal, err := connObj.GetProperty("org.freedesktop.NetworkManager.Connection.Active.Type")
		if err != nil {
			continue
		}
		if tStr, ok := typeVal.Value().(string); ok && tStr == "802-11-wireless" {
			idVal, err := connObj.GetProperty("org.freedesktop.NetworkManager.Connection.Active.Id")
			if err == nil {
				if idStr, ok := idVal.Value().(string); ok && idStr != "" {
					return idStr
				}
			}
		}
	}
	return ""
}

func (wc *WiFiController) querySavedSSIDs(conn *dbus.Conn) map[string]bool {
	wc.mu.Lock()
	if wc.savedSSIDs != nil && time.Since(wc.savedSSIDsTime) < 30*time.Second {
		res := make(map[string]bool, len(wc.savedSSIDs))
		for k, v := range wc.savedSSIDs {
			res[k] = v
		}
		wc.mu.Unlock()
		return res
	}
	wc.mu.Unlock()

	saved := make(map[string]bool)
	if conn != nil {
		settingsObj := conn.Object(nmDest, "/org/freedesktop/NetworkManager/Settings")
		var conPaths []dbus.ObjectPath
		err := settingsObj.Call("org.freedesktop.NetworkManager.Settings.ListConnections", 0).Store(&conPaths)
		if err == nil {
			for _, cp := range conPaths {
				conObj := conn.Object(nmDest, cp)
				var settings map[string]map[string]dbus.Variant
				if err := conObj.Call("org.freedesktop.NetworkManager.Settings.Connection.GetSettings", 0).Store(&settings); err == nil {
					conSection := settings["connection"]
					conType, _ := conSection["type"].Value().(string)
					if conType == "802-11-wireless" {
						if wifiSection, ok := settings["802-11-wireless"]; ok {
							if ssidBytes, ok := wifiSection["ssid"].Value().([]byte); ok && len(ssidBytes) > 0 {
								saved[string(ssidBytes)] = true
								continue
							}
						}
						if idStr, ok := conSection["id"].Value().(string); ok && idStr != "" {
							saved[idStr] = true
						}
					}
				}
			}
		}
	}

	if len(saved) == 0 {
		out, err := exec.Command("nmcli", "-t", "-f", "NAME,TYPE", "connection", "show").Output()
		if err == nil {
			lines := strings.Split(string(out), "\n")
			for _, line := range lines {
				parts := strings.Split(line, ":")
				if len(parts) >= 2 && parts[1] == "802-11-wireless" {
					name := strings.TrimSpace(parts[0])
					if name != "" {
						saved[name] = true
					}
				}
			}
		}
	}

	wc.mu.Lock()
	wc.savedSSIDs = saved
	wc.savedSSIDsTime = time.Now()
	res := make(map[string]bool, len(saved))
	for k, v := range saved {
		res[k] = v
	}
	wc.mu.Unlock()

	return res
}

func SortAccessPoints(list []AccessPoint) {
	sort.Slice(list, func(i, j int) bool {
		if list[i].Connected != list[j].Connected {
			return list[i].Connected
		}
		if list[i].Saved != list[j].Saved {
			return list[i].Saved
		}
		if list[i].Strength != list[j].Strength {
			return list[i].Strength > list[j].Strength
		}
		return list[i].SSID < list[j].SSID
	})
}

func (wc *WiFiController) quickRefresh() {
	wc.mu.Lock()
	conn := wc.conn
	devPath := wc.devicePath
	wc.mu.Unlock()

	if conn == nil {
		return
	}

	obj := conn.Object(nmDest, nmPath)
	val, err := obj.GetProperty(nmIFace + ".WirelessEnabled")
	if err != nil {
		return
	}

	enabled, _ := val.Value().(bool)

	var activeSSID string
	var activeStrength uint8

	if enabled {
		activeSSID = wc.queryActiveWiFiSSID(conn)
	}

	wc.mu.Lock()
	wc.enabled = enabled
	wc.available = (devPath != "")
	wc.ssid = activeSSID

	var stateChanged bool
	for i := range wc.networks {
		isConn := (activeSSID != "" && wc.networks[i].SSID == activeSSID)
		if wc.networks[i].Connected != isConn {
			wc.networks[i].Connected = isConn
			stateChanged = true
		}
		if isConn {
			activeStrength = wc.networks[i].Strength
		}
	}
	wc.activeStrength = activeStrength
	if stateChanged {
		SortAccessPoints(wc.networks)
	}
	wc.mu.Unlock()

	wc.notify()
}

func (wc *WiFiController) queryAccessPoints(conn *dbus.Conn, devPath dbus.ObjectPath, activeSSID string) []AccessPoint {
	if devPath == "" {
		return nil
	}
	obj := conn.Object(nmDest, devPath)
	var apPaths []dbus.ObjectPath
	err := obj.Call("org.freedesktop.NetworkManager.Device.Wireless.GetAllAccessPoints", 0).Store(&apPaths)
	if err != nil {
		return nil
	}

	savedSSIDs := wc.querySavedSSIDs(conn)

	apMap := make(map[string]AccessPoint)
	for _, apPath := range apPaths {
		apObj := conn.Object(nmDest, apPath)
		var props map[string]dbus.Variant
		err := apObj.Call("org.freedesktop.DBus.Properties.GetAll", 0, "org.freedesktop.NetworkManager.AccessPoint").Store(&props)
		if err != nil || len(props) == 0 {
			continue
		}

		ssidBytes, _ := props["Ssid"].Value().([]byte)
		ssid := string(ssidBytes)
		if strings.TrimSpace(ssid) == "" {
			continue
		}

		strength, _ := props["Strength"].Value().(byte)
		rsn, _ := props["RsnFlags"].Value().(uint32)
		wpa, _ := props["WpaFlags"].Value().(uint32)
		hwAddr, _ := props["HwAddress"].Value().(string)
		isSecured := rsn != 0 || wpa != 0

		existing, found := apMap[ssid]
		if !found || strength > existing.Strength {
			apMap[ssid] = AccessPoint{
				SSID:      ssid,
				Strength:  uint8(strength),
				Secured:   isSecured,
				Connected: (activeSSID != "" && ssid == activeSSID),
				Saved:     savedSSIDs[ssid],
				BSSID:     hwAddr,
				Path:      apPath,
			}
		}
	}

	var list []AccessPoint
	for _, ap := range apMap {
		list = append(list, ap)
	}

	SortAccessPoints(list)

	return list
}

func (wc *WiFiController) notify() {
	wc.mu.Lock()
	enabled := wc.enabled
	avail := wc.available
	ssid := wc.ssid
	activeStrength := wc.activeStrength
	nets := make([]AccessPoint, len(wc.networks))
	copy(nets, wc.networks)
	onChange := wc.onChange
	onNetworks := wc.onNetworks
	wc.mu.Unlock()

	glib.IdleAdd(func() {
		if onChange != nil {
			if !avail {
				onChange(false, false, "Unavailable", "network-wireless-symbolic")
			} else if !enabled {
				onChange(false, true, "Off", "network-wireless-symbolic")
			} else if ssid != "" {
				icon := WifiSignalIcon(activeStrength)
				onChange(true, true, ssid, icon)
			} else {
				onChange(true, true, "Disconnected", "network-wireless-symbolic")
			}
		}

		if onNetworks != nil {
			onNetworks(nets)
		}
	})
}

func (wc *WiFiController) Toggle() {
	wc.mu.Lock()
	conn := wc.conn
	avail := wc.available
	if !avail || conn == nil {
		wc.mu.Unlock()
		return
	}
	target := !wc.enabled
	wc.enabled = target
	wc.mu.Unlock()

	obj := conn.Object(nmDest, nmPath)
	_ = obj.SetProperty(nmIFace+".WirelessEnabled", dbus.MakeVariant(target))
}

func (wc *WiFiController) SetWirelessEnabled(enabled bool) {
	wc.mu.Lock()
	conn := wc.conn
	avail := wc.available
	current := wc.enabled
	if !avail || conn == nil || current == enabled {
		wc.mu.Unlock()
		return
	}
	wc.enabled = enabled
	wc.mu.Unlock()

	obj := conn.Object(nmDest, nmPath)
	_ = obj.SetProperty(nmIFace+".WirelessEnabled", dbus.MakeVariant(enabled))
}

func (wc *WiFiController) Scan() {
	wc.mu.Lock()
	conn := wc.conn
	devPath := wc.devicePath
	wc.mu.Unlock()

	// 1. Immediately refresh with existing known APs
	go wc.Refresh()

	// 2. Request fresh radio scan
	if conn != nil && devPath != "" {
		obj := conn.Object(nmDest, devPath)
		_ = obj.Call("org.freedesktop.NetworkManager.Device.Wireless.RequestScan", 0, map[string]dbus.Variant{})
	}

	// 3. Re-refresh after scan completes
	go func() {
		time.Sleep(1200 * time.Millisecond)
		wc.Refresh()
	}()
}

func (wc *WiFiController) Connect(ssid, password string) error {
	var cmd *exec.Cmd
	if password != "" {
		cmd = exec.Command("nmcli", "dev", "wifi", "connect", ssid, "password", password)
	} else {
		cmd = exec.Command("nmcli", "con", "up", "id", ssid)
		if _, err := cmd.CombinedOutput(); err == nil {
			wc.Refresh()
			return nil
		}
		cmd = exec.Command("nmcli", "dev", "wifi", "connect", ssid)
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s (%w)", strings.TrimSpace(string(out)), err)
	}

	wc.Refresh()
	return nil
}

func (wc *WiFiController) Disconnect() error {
	wc.mu.Lock()
	ssid := wc.ssid
	dev := wc.deviceIface
	wc.mu.Unlock()

	var cmd *exec.Cmd
	if ssid != "" {
		cmd = exec.Command("nmcli", "con", "down", "id", ssid)
	} else if dev != "" {
		cmd = exec.Command("nmcli", "dev", "disconnect", dev)
	} else {
		return fmt.Errorf("no active connection")
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s (%w)", strings.TrimSpace(string(out)), err)
	}

	wc.Refresh()
	return nil
}

func (wc *WiFiController) IsEnabled() bool {
	wc.mu.Lock()
	defer wc.mu.Unlock()
	return wc.enabled
}

func (wc *WiFiController) IsAvailable() bool {
	wc.mu.Lock()
	defer wc.mu.Unlock()
	return wc.available
}

func (wc *WiFiController) ActiveSSID() string {
	wc.mu.Lock()
	defer wc.mu.Unlock()
	return wc.ssid
}

func (wc *WiFiController) Networks() []AccessPoint {
	wc.mu.Lock()
	defer wc.mu.Unlock()
	nets := make([]AccessPoint, len(wc.networks))
	copy(nets, wc.networks)
	return nets
}
