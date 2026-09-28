package removable

import (
	"testing"

	"github.com/godbus/dbus/v5"
)

func TestRemovableMonitorHandlers(t *testing.T) {
	m := NewMonitor(nil)

	// Simulate InterfacesAdded signal
	sig := &dbus.Signal{
		Name: "org.freedesktop.DBus.ObjectManager.InterfacesAdded",
		Body: []any{
			dbus.ObjectPath("/org/freedesktop/UDisks2/drives/SanDisk_Ultra"),
			map[string]map[string]dbus.Variant{
				"org.freedesktop.UDisks2.Drive": {
					"Removable":     dbus.MakeVariant(true),
					"ConnectionBus": dbus.MakeVariant("usb"),
					"Vendor":        dbus.MakeVariant("SanDisk"),
					"Model":         dbus.MakeVariant("Ultra"),
				},
			},
		},
	}

	m.handleAdded(sig)

	m.mu.Lock()
	name, exists := m.known[dbus.ObjectPath("/org/freedesktop/UDisks2/drives/SanDisk_Ultra")]
	m.mu.Unlock()

	if !exists {
		t.Fatal("expected drive to be stored in known map")
	}
	if name != "SanDisk Ultra" {
		t.Fatalf("expected SanDisk Ultra, got %q", name)
	}

	// Simulate InterfacesRemoved signal
	sigRem := &dbus.Signal{
		Name: "org.freedesktop.DBus.ObjectManager.InterfacesRemoved",
		Body: []any{
			dbus.ObjectPath("/org/freedesktop/UDisks2/drives/SanDisk_Ultra"),
			[]string{"org.freedesktop.UDisks2.Drive"},
		},
	}

	m.handleRemoved(sigRem)

	m.mu.Lock()
	_, existsAfter := m.known[dbus.ObjectPath("/org/freedesktop/UDisks2/drives/SanDisk_Ultra")]
	m.mu.Unlock()

	if existsAfter {
		t.Fatal("expected drive to be removed from known map")
	}
}
