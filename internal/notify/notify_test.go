package notify

import (
	"errors"
	"reflect"
	"testing"

	"github.com/godbus/dbus/v5"
)

func TestParseActions(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []Action
	}{
		{
			name:     "empty",
			input:    nil,
			expected: nil,
		},
		{
			name:  "standard pairs",
			input: []string{"default", "Open", "dismiss", "Dismiss"},
			expected: []Action{
				{Key: "default", Label: "Open"},
				{Key: "dismiss", Label: "Dismiss"},
			},
		},
		{
			name:  "odd number of items",
			input: []string{"key1", "label1", "trailing_key"},
			expected: []Action{
				{Key: "key1", Label: "label1"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseActions(tt.input)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("ParseActions(%v) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestParseUrgency(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected Urgency
	}{
		{"byte low", byte(0), UrgencyLow},
		{"byte normal", byte(1), UrgencyNormal},
		{"byte critical", byte(2), UrgencyCritical},
		{"int critical", int(2), UrgencyCritical},
		{"uint32 normal", uint32(1), UrgencyNormal},
		{"out of range", int(5), UrgencyNormal},
		{"string invalid", "high", UrgencyNormal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseUrgency(tt.input)
			if got != tt.expected {
				t.Errorf("ParseUrgency(%v) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestParseHints(t *testing.T) {
	hints := map[string]dbus.Variant{
		"urgency":       dbus.MakeVariant(byte(2)),
		"image-path":    dbus.MakeVariant("/usr/share/icons/alert.png"),
		"desktop-entry": dbus.MakeVariant("org.example.App"),
	}

	urgency, parsed := ParseHints(hints)
	if urgency != UrgencyCritical {
		t.Errorf("expected UrgencyCritical, got %v", urgency)
	}
	if parsed["image-path"] != "/usr/share/icons/alert.png" {
		t.Errorf("expected image-path to be preserved, got %v", parsed["image-path"])
	}
	if parsed["desktop-entry"] != "org.example.App" {
		t.Errorf("expected desktop-entry to be preserved, got %v", parsed["desktop-entry"])
	}
}

func TestDBusCapabilitiesAndInfo(t *testing.T) {
	h := &dbusHandler{}
	caps, err := h.GetCapabilities()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expectedCaps := []string{"action-icons", "actions", "body", "body-markup", "icon-static", "persistence"}
	if !reflect.DeepEqual(caps, expectedCaps) {
		t.Errorf("GetCapabilities() = %v, want %v", caps, expectedCaps)
	}

	name, vendor, version, specVersion, err := h.GetServerInformation()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "phalune" || vendor != "phalune" || version != "0.1.0" || specVersion != "1.2" {
		t.Errorf("unexpected server info: name=%s vendor=%s version=%s specVersion=%s", name, vendor, version, specVersion)
	}
}

func TestSendDBus(t *testing.T) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		t.Skip("session bus not available, skipping TestSendDBus")
	}
	defer conn.Close()

	id, err := SendDBus(Notification{
		AppName: "phalune-test",
		Summary: "Test Unit Notification",
		Body:    "Testing SendDBus client method",
		Urgency: UrgencyLow,
	})
	if err != nil {
		t.Fatalf("SendDBus failed: %v", err)
	}
	if id == 0 {
		t.Errorf("expected non-zero notification ID, got %d", id)
	}
}

func TestStartDBusServer_AlreadyOwned(t *testing.T) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		t.Skip("session bus not available, skipping test")
	}
	defer conn.Close()

	var owner string
	err = conn.BusObject().Call("org.freedesktop.DBus.GetNameOwner", 0, DBusName).Store(&owner)
	if err != nil || owner == "" {
		t.Skip("org.freedesktop.Notifications is not currently owned on this session bus, skipping test")
	}

	server, err := StartDBusServer(&Manager{items: make(map[uint32]*notificationItem)})
	if err == nil {
		if server != nil {
			_ = server.Close()
		}
		t.Fatalf("expected error when name is already owned, got nil")
	}

	if !errors.Is(err, ErrNameAlreadyTaken) {
		t.Errorf("expected error wrapping ErrNameAlreadyTaken, got: %v", err)
	}
}

func TestStartDBusServer_SecondInstanceFails(t *testing.T) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		t.Skip("session bus not available, skipping test")
	}
	_ = conn.Close()

	s1, err := StartDBusServer(nil)
	if errors.Is(err, ErrNameAlreadyTaken) {
		// External daemon already owns it, so ErrNameAlreadyTaken is verified immediately
		return
	}
	if err != nil {
		t.Skipf("cannot start first server: %v", err)
	}
	defer s1.Close()

	s2, err := StartDBusServer(nil)
	if s2 != nil {
		_ = s2.Close()
		t.Fatalf("expected second server to fail, got non-nil server")
	}
	if !errors.Is(err, ErrNameAlreadyTaken) {
		t.Fatalf("expected ErrNameAlreadyTaken, got %v", err)
	}
}

func TestDND(t *testing.T) {
	mgr := &Manager{
		items:  make(map[uint32]*notificationItem),
		nextID: 1,
	}

	if mgr.IsDND() {
		t.Errorf("expected DND to be false by default")
	}

	mgr.SetDND(true)
	if !mgr.IsDND() {
		t.Errorf("expected DND to be true after SetDND(true)")
	}

	// Normal notification should be assigned an ID but not stored in active UI items
	id := mgr.Show(Notification{
		Summary: "Test normal during DND",
		Urgency: UrgencyNormal,
	})
	if id == 0 {
		t.Errorf("expected valid non-zero ID during DND, got 0")
	}

	mgr.mu.Lock()
	if _, exists := mgr.items[id]; exists {
		mgr.mu.Unlock()
		t.Errorf("expected normal notification to not be added to active items during DND")
	} else {
		mgr.mu.Unlock()
	}

	mgr.SetDND(false)
	if mgr.IsDND() {
		t.Errorf("expected DND to be false after SetDND(false)")
	}
}
