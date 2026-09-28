package tray

import (
	"testing"

	"github.com/godbus/dbus/v5"
)

func TestNormalizeItemService(t *testing.T) {
	tests := []struct {
		service  string
		sender   string
		expected string
	}{
		{"/StatusNotifierItem", ":1.42", ":1.42/StatusNotifierItem"},
		{"org.kde.StatusNotifierItem-1234", ":1.42", "org.kde.StatusNotifierItem-1234/StatusNotifierItem"},
		{":1.50/CustomItem", ":1.50", ":1.50/CustomItem"},
	}

	for _, tt := range tests {
		got := NormalizeItemService(tt.service, tt.sender)
		if got != tt.expected {
			t.Errorf("NormalizeItemService(%q, %q) = %q, want %q", tt.service, tt.sender, got, tt.expected)
		}
	}
}

func TestCleanMenuLabel(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"_Quit", "Quit"},
		{"_Preferences...", "Preferences..."},
		{"Save __ As", "Save _ As"},
		{"Open File", "Open File"},
	}

	for _, tt := range tests {
		got := cleanMenuLabel(tt.input)
		if got != tt.expected {
			t.Errorf("cleanMenuLabel(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestParseMenuItem(t *testing.T) {
	rawChild := []interface{}{
		int32(101),
		map[string]dbus.Variant{
			"label":   dbus.MakeVariant("_Exit"),
			"enabled": dbus.MakeVariant(true),
			"visible": dbus.MakeVariant(true),
			"type":    dbus.MakeVariant("standard"),
		},
		[]interface{}{},
	}

	rawRoot := []interface{}{
		int32(0),
		map[string]dbus.Variant{},
		[]interface{}{rawChild},
	}

	root := parseMenuItem(rawRoot)
	if root == nil {
		t.Fatalf("expected non-nil root")
	}
	if len(root.Children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(root.Children))
	}

	child := root.Children[0]
	if child.ID != 101 {
		t.Errorf("expected child ID 101, got %d", child.ID)
	}
	if child.Label != "Exit" {
		t.Errorf("expected child label 'Exit', got %q", child.Label)
	}
}
