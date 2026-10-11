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
	bluezDest  = "org.bluez"
	bluezPath  = "/org/bluez/hci0"
	bluezIFace = "org.bluez.Adapter1"
)

type BluetoothDevice struct {
	Path      dbus.ObjectPath
	Address   string
	Name      string
	Icon      string
	Paired    bool
	Connected bool
	Trusted   bool
	RSSI      int16
}

func BluetoothDeviceIcon(icon string) string {
	switch icon {
	case "audio-headset", "audio-headphones", "audio-card":
		return "audio-headphones-symbolic"
	case "input-keyboard":
		return "input-keyboard-symbolic"
	case "input-mouse", "input-gaming":
		return "input-mouse-symbolic"
	case "phone":
		return "phone-symbolic"
	default:
		return "bluetooth-active-symbolic"
	}
}

type BluetoothController struct {
	mu            sync.Mutex
	conn          *dbus.Conn
	adapterPath   dbus.ObjectPath
	powered       bool
	available     bool
	discovering   bool
	devices       []BluetoothDevice
	debounceTimer *time.Timer
	debounceMu    sync.Mutex
	onChange      func(powered, available bool, title, icon string)
	onDevices     func(devices []BluetoothDevice)
	// onDiscovering fires on real adapter discovery state changes.
	onDiscovering func(discovering bool)
}

func (bc *BluetoothController) scheduleRefresh() {
	bc.debounceMu.Lock()
	defer bc.debounceMu.Unlock()
	if bc.debounceTimer != nil {
		bc.debounceTimer.Stop()
	}
	bc.debounceTimer = time.AfterFunc(150*time.Millisecond, func() {
		bc.Refresh()
	})
}

func (bc *BluetoothController) cancelDebounce() {
	bc.debounceMu.Lock()
	defer bc.debounceMu.Unlock()
	if bc.debounceTimer != nil {
		bc.debounceTimer.Stop()
		bc.debounceTimer = nil
	}
}

func NewBluetoothController(onChange func(powered, available bool, title, icon string)) *BluetoothController {
	return &BluetoothController{
		adapterPath: bluezPath,
		onChange:    onChange,
	}
}

func (bc *BluetoothController) SetOnDevicesChanged(fn func(devices []BluetoothDevice)) {
	bc.mu.Lock()
	bc.onDevices = fn
	bc.mu.Unlock()
}

// SetOnDiscoveringChanged reports discovery state on the GTK thread.
func (bc *BluetoothController) SetOnDiscoveringChanged(fn func(discovering bool)) {
	bc.mu.Lock()
	bc.onDiscovering = fn
	bc.mu.Unlock()
}

func (bc *BluetoothController) Start(ctx context.Context) {
	conn, err := dbus.SystemBus()
	if err != nil {
		slog.Debug("bluetooth: system bus unavailable", "error", err)
		if bc.onChange != nil {
			glib.IdleAdd(func() {
				bc.onChange(false, false, "Unavailable", "bluetooth-active-symbolic")
			})
		}
		return
	}
	bc.mu.Lock()
	bc.conn = conn
	bc.mu.Unlock()

	bc.Refresh()

	// Zero-polling subscriptions:
	// 1. Adapter property changes
	ruleAdapter := fmt.Sprintf("type='signal',interface='org.freedesktop.DBus.Properties',member='PropertiesChanged',path='%s'", bluezPath)
	conn.BusObject().Call("org.freedesktop.DBus.AddMatch", 0, ruleAdapter)

	// 2. ObjectManager interface addition/removal (discovering/pairing devices)
	ruleObjAdded := "type='signal',interface='org.freedesktop.DBus.ObjectManager',member='InterfacesAdded'"
	conn.BusObject().Call("org.freedesktop.DBus.AddMatch", 0, ruleObjAdded)

	ruleObjRemoved := "type='signal',interface='org.freedesktop.DBus.ObjectManager',member='InterfacesRemoved'"
	conn.BusObject().Call("org.freedesktop.DBus.AddMatch", 0, ruleObjRemoved)

	ch := make(chan *dbus.Signal, 15)
	conn.Signal(ch)

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
				if sig.Path == bluezPath {
					if len(sig.Body) >= 2 {
						if iface, ok := sig.Body[0].(string); ok && iface == bluezIFace {
							if changed, ok := sig.Body[1].(map[string]dbus.Variant); ok {
								bc.mu.Lock()
								if v, exists := changed["Powered"]; exists {
									if b, ok := v.Value().(bool); ok {
										bc.powered = b
										if !b {
											bc.devices = nil
										}
									}
								}
								if v, exists := changed["Discovering"]; exists {
									if d, ok := v.Value().(bool); ok {
										bc.discovering = d
									}
								}
								isOff := !bc.powered
								bc.mu.Unlock()

								if isOff {
									bc.cancelDebounce()
									bc.notify()
									continue
								}
							}
						}
					}
					bc.scheduleRefresh()
				} else if sig.Name == "org.freedesktop.DBus.ObjectManager.InterfacesAdded" ||
					sig.Name == "org.freedesktop.DBus.ObjectManager.InterfacesRemoved" ||
					strings.HasPrefix(string(sig.Path), string(bluezPath)+"/dev_") {
					if bc.IsPowered() {
						bc.scheduleRefresh()
					}
				}
			}
		}
	}()
}

func (bc *BluetoothController) Refresh() {
	bc.mu.Lock()
	conn := bc.conn
	adapterPath := bc.adapterPath
	bc.mu.Unlock()

	if conn == nil {
		return
	}

	obj := conn.Object(bluezDest, adapterPath)
	val, err := obj.GetProperty(bluezIFace + ".Powered")
	if err != nil {
		bc.mu.Lock()
		bc.available = false
		bc.mu.Unlock()
		bc.notify()
		return
	}

	powered, _ := val.Value().(bool)

	var devices []BluetoothDevice
	if powered {
		devices = bc.queryDevices(conn)
	}

	bc.mu.Lock()
	prevDiscovering := bc.discovering
	bc.powered = powered
	bc.available = true
	bc.devices = devices
	if discVal, discErr := obj.GetProperty(bluezIFace + ".Discovering"); discErr == nil {
		if d, ok := discVal.Value().(bool); ok {
			bc.discovering = d
		}
	}
	discoveringChanged := bc.discovering != prevDiscovering
	discoveringNow := bc.discovering
	discoveringHook := bc.onDiscovering
	bc.mu.Unlock()

	if discoveringChanged && discoveringHook != nil {
		hook, discovering := discoveringHook, discoveringNow
		glib.IdleAdd(func() {
			hook(discovering)
		})
	}

	bc.notify()
}

func (bc *BluetoothController) queryDevices(conn *dbus.Conn) []BluetoothDevice {
	var objects map[dbus.ObjectPath]map[string]map[string]dbus.Variant
	err := conn.Object(bluezDest, "/").Call("org.freedesktop.DBus.ObjectManager.GetManagedObjects", 0).Store(&objects)
	if err != nil {
		return nil
	}

	var list []BluetoothDevice
	for path, ifaces := range objects {
		devProps, ok := ifaces["org.bluez.Device1"]
		if !ok {
			continue
		}

		address, _ := devProps["Address"].Value().(string)
		name, _ := devProps["Name"].Value().(string)
		if name == "" {
			name, _ = devProps["Alias"].Value().(string)
		}
		if name == "" {
			name = address
		}
		if name == "" {
			continue
		}

		icon, _ := devProps["Icon"].Value().(string)
		paired, _ := devProps["Paired"].Value().(bool)
		connected, _ := devProps["Connected"].Value().(bool)
		trusted, _ := devProps["Trusted"].Value().(bool)
		rssi, _ := devProps["RSSI"].Value().(int16)

		list = append(list, BluetoothDevice{
			Path:      path,
			Address:   address,
			Name:      name,
			Icon:      icon,
			Paired:    paired,
			Connected: connected,
			Trusted:   trusted,
			RSSI:      rssi,
		})
	}

	// Sort: Connected first, then Paired, then by Name
	sort.Slice(list, func(i, j int) bool {
		if list[i].Connected != list[j].Connected {
			return list[i].Connected
		}
		if list[i].Paired != list[j].Paired {
			return list[i].Paired
		}
		return list[i].Name < list[j].Name
	})

	return list
}

func (bc *BluetoothController) notify() {
	bc.mu.Lock()
	powered := bc.powered
	avail := bc.available
	devs := make([]BluetoothDevice, len(bc.devices))
	copy(devs, bc.devices)
	onChange := bc.onChange
	onDevices := bc.onDevices
	bc.mu.Unlock()

	glib.IdleAdd(func() {
		if onChange != nil {
			if !avail {
				onChange(false, false, "Unavailable", "bluetooth-active-symbolic")
			} else if powered {
				connectedName := ""
				for _, d := range devs {
					if d.Connected {
						connectedName = d.Name
						break
					}
				}
				if connectedName != "" {
					onChange(true, true, connectedName, "bluetooth-active-symbolic")
				} else {
					onChange(true, true, "On", "bluetooth-active-symbolic")
				}
			} else {
				onChange(false, true, "Off", "bluetooth-active-symbolic")
			}
		}

		if onDevices != nil {
			onDevices(devs)
		}
	})
}

func (bc *BluetoothController) Toggle() {
	bc.mu.Lock()
	conn := bc.conn
	avail := bc.available
	if !avail || conn == nil {
		bc.mu.Unlock()
		return
	}
	target := !bc.powered
	bc.powered = target
	if !target {
		bc.devices = nil
	}
	bc.mu.Unlock()

	if !target {
		bc.cancelDebounce()
	}
	bc.notify()

	obj := conn.Object(bluezDest, bluezPath)
	_ = obj.SetProperty(bluezIFace+".Powered", dbus.MakeVariant(target))
}

func (bc *BluetoothController) SetPowered(powered bool) {
	bc.mu.Lock()
	conn := bc.conn
	avail := bc.available
	current := bc.powered
	if !avail || conn == nil || current == powered {
		bc.mu.Unlock()
		return
	}
	bc.powered = powered
	if !powered {
		bc.devices = nil
	}
	bc.mu.Unlock()

	if !powered {
		bc.cancelDebounce()
	}
	bc.notify()

	obj := conn.Object(bluezDest, bluezPath)
	_ = obj.SetProperty(bluezIFace+".Powered", dbus.MakeVariant(powered))
}

func (bc *BluetoothController) Scan() {
	bc.mu.Lock()
	conn := bc.conn
	avail := bc.available
	powered := bc.powered
	bc.mu.Unlock()

	if !avail || !powered || conn == nil {
		return
	}

	obj := conn.Object(bluezDest, bluezPath)
	_ = obj.Call(bluezIFace+".StartDiscovery", 0)

	go func() {
		// Discover for 12 seconds, then stop discovery to conserve energy
		time.Sleep(12 * time.Second)
		bc.mu.Lock()
		c := bc.conn
		bc.mu.Unlock()
		if c != nil {
			o := c.Object(bluezDest, bluezPath)
			_ = o.Call(bluezIFace+".StopDiscovery", 0)
		}
		bc.Refresh()
	}()
}

func (bc *BluetoothController) Connect(address string) error {
	cmd := exec.Command("bluetoothctl", "connect", address)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s (%w)", strings.TrimSpace(string(out)), err)
	}
	bc.Refresh()
	return nil
}

func (bc *BluetoothController) Disconnect(address string) error {
	cmd := exec.Command("bluetoothctl", "disconnect", address)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s (%w)", strings.TrimSpace(string(out)), err)
	}
	bc.Refresh()
	return nil
}

func (bc *BluetoothController) Forget(address string) error {
	cmd := exec.Command("bluetoothctl", "remove", address)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s (%w)", strings.TrimSpace(string(out)), err)
	}
	bc.Refresh()
	return nil
}

func (bc *BluetoothController) IsPowered() bool {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	return bc.powered
}

func (bc *BluetoothController) IsAvailable() bool {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	return bc.available
}

func (bc *BluetoothController) Devices() []BluetoothDevice {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	devs := make([]BluetoothDevice, len(bc.devices))
	copy(devs, bc.devices)
	return devs
}
