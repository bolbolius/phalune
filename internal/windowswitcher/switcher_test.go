package windowswitcher

import (
	"testing"
)

func TestResolveAppName(t *testing.T) {
	tests := []struct {
		appID    string
		title    string
		expected string
	}{
		{"org.telegram.desktop", "Chat", "Telegram"},
		{"Firefox", "Mozilla Firefox", "Firefox"},
		{"kitty", "terminal", "Kitty"},
		{"", "Special Window", "Special Window"},
		{"", "", "Window"},
	}

	for _, tt := range tests {
		got := ResolveAppName(tt.appID, tt.title)
		if got != tt.expected {
			t.Errorf("ResolveAppName(%q, %q) = %q, want %q", tt.appID, tt.title, got, tt.expected)
		}
	}
}

func TestResolveAppIcon(t *testing.T) {
	icon := ResolveAppIcon("")
	if icon != "application-x-executable-symbolic" {
		t.Errorf("expected fallback icon, got %q", icon)
	}
}
