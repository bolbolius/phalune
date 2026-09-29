package polkit

import (
	"os"
	"testing"

	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

func TestFormatIdentity(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"unix-user:root", "Administrator (root)"},
		{"unix-user:meytili", "meytili (User)"},
		{"unix-group:wheel", "unix-group:wheel"},
		{"custom-identity", "custom-identity"},
	}

	for _, tt := range tests {
		got := formatIdentity(tt.in)
		if got != tt.want {
			t.Errorf("formatIdentity(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestNewAuthDialog(t *testing.T) {
	if os.Getenv("WAYLAND_DISPLAY") == "" && os.Getenv("DISPLAY") == "" {
		t.Skip("skipping UI test without display server")
	}

	app := gtk.NewApplication("org.phalune.test.polkit", gio.ApplicationFlagsNone)
	app.ConnectActivate(func() {
		dlg, err := NewAuthDialog(app)
		if err != nil {
			t.Fatalf("failed to create auth dialog: %v", err)
		}

		if dlg.window == nil {
			t.Errorf("dialog window is nil")
		}
		if dlg.passwordEntry == nil {
			t.Errorf("password entry is nil")
		}
		if dlg.authBtn == nil || dlg.cancelBtn == nil {
			t.Errorf("buttons not initialized")
		}

		dlg.Destroy()
		app.Quit()
	})

	app.Run([]string{"test"})
}
