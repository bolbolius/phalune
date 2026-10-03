package style

import "testing"

func TestResolve(t *testing.T) {
	tests := []struct {
		component, in, wantStyle, wantClass string
	}{
		{Bar, "bubble", "bubble", "bar-style-bubble"},
		{Bar, " BUBBLE ", "bubble", "bar-style-bubble"},
		{Bar, "", "", "bar-style-default"},
		{Bar, "nope", "", "bar-style-default"},
		{OSD, "pill", "pill", "osd-style-pill"},
		{ControlCenter, "compact", "compact", "control-center-style-compact"},
		{Launcher, "fullscreen", "fullscreen", "launcher-style-fullscreen"},
		{Notifications, "bubbles", "bubbles", "notifications-style-bubbles"},
		{"unknown", "anything", "", "unknown-style-default"},
	}

	for _, tc := range tests {
		s, c := Resolve(tc.component, tc.in)
		if s != tc.wantStyle || c != tc.wantClass {
			t.Errorf("Resolve(%q, %q) = (%q, %q), want (%q, %q)", tc.component, tc.in, s, c, tc.wantStyle, tc.wantClass)
		}
	}
}

func TestHasTemplate(t *testing.T) {
	if !HasTemplate(OSD, "pill") || !HasTemplate(OSD, "bar") || !HasTemplate(OSD, "minimal") {
		t.Error("expected OSD templates for pill, bar, minimal")
	}
	if HasTemplate(OSD, "") || HasTemplate(Bar, "bubble") {
		t.Error("unexpected template")
	}
}
