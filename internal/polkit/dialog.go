package polkit

import (
	"fmt"
	"log/slog"
	"strings"

	"phalune/ui"

	"github.com/diamondburned/gotk4-layer-shell/pkg/gtk4layershell"
	"github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

type AuthDialog struct {
	window           *gtk.Window
	overlayBox       *gtk.Overlay
	card             *gtk.Box
	icon             *gtk.Image
	titleLabel       *gtk.Label
	actionLabel      *gtk.Label
	messageLabel     *gtk.Label
	identityBox      *gtk.Box
	identityDropdown *gtk.DropDown
	passwordEntry    *gtk.PasswordEntry
	errorLabel       *gtk.Label
	cancelBtn        *gtk.Button
	authBtn          *gtk.Button

	taskId       uintptr
	cookie       string
	identities   []string
	selectedIdx  int
	onResponse   func(taskId uintptr, password string)
	onCancel     func(taskId uintptr)
	onSwitchUser func(taskId uintptr, identityIdx int)
	closed       bool
}

func NewAuthDialog(app *gtk.Application) (*AuthDialog, error) {
	win := gtk.NewWindow()
	if app != nil {
		win.SetApplication(app)
	}
	win.SetTitle("Authentication Required")
	win.SetDecorated(false)
	win.AddCSSClass("polkit-window")

	if gtk4layershell.IsSupported() {
		gtk4layershell.InitForWindow(win)
		gtk4layershell.SetNamespace(win, "phalune-polkit")
		gtk4layershell.SetLayer(win, gtk4layershell.LayerShellLayerOverlay)
		gtk4layershell.SetAnchor(win, gtk4layershell.LayerShellEdgeTop, true)
		gtk4layershell.SetAnchor(win, gtk4layershell.LayerShellEdgeBottom, true)
		gtk4layershell.SetAnchor(win, gtk4layershell.LayerShellEdgeLeft, true)
		gtk4layershell.SetAnchor(win, gtk4layershell.LayerShellEdgeRight, true)
		gtk4layershell.SetExclusiveZone(win, -1)
		gtk4layershell.SetKeyboardMode(win, gtk4layershell.LayerShellKeyboardModeExclusive)
	}

	builder := gtk.NewBuilderFromString(ui.PolkitDialog)
	overlayBox := builder.GetObject("overlay_box").Cast().(*gtk.Overlay)
	card := builder.GetObject("card").Cast().(*gtk.Box)
	icon := builder.GetObject("icon").Cast().(*gtk.Image)
	titleLabel := builder.GetObject("title_label").Cast().(*gtk.Label)
	actionLabel := builder.GetObject("action_label").Cast().(*gtk.Label)
	messageLabel := builder.GetObject("message_label").Cast().(*gtk.Label)
	identityBox := builder.GetObject("identity_box").Cast().(*gtk.Box)
	identityDropdown := builder.GetObject("identity_dropdown").Cast().(*gtk.DropDown)
	passwordEntry := builder.GetObject("password_entry").Cast().(*gtk.PasswordEntry)
	errorLabel := builder.GetObject("error_label").Cast().(*gtk.Label)
	cancelBtn := builder.GetObject("cancel_button").Cast().(*gtk.Button)
	authBtn := builder.GetObject("auth_button").Cast().(*gtk.Button)

	win.SetChild(overlayBox)

	d := &AuthDialog{
		window:           win,
		overlayBox:       overlayBox,
		card:             card,
		icon:             icon,
		titleLabel:       titleLabel,
		actionLabel:      actionLabel,
		messageLabel:     messageLabel,
		identityBox:      identityBox,
		identityDropdown: identityDropdown,
		passwordEntry:    passwordEntry,
		errorLabel:       errorLabel,
		cancelBtn:        cancelBtn,
		authBtn:          authBtn,
	}

	d.setupEvents()
	return d, nil
}

func (d *AuthDialog) setupEvents() {
	d.cancelBtn.ConnectClicked(func() {
		d.Cancel()
	})

	d.authBtn.ConnectClicked(func() {
		d.submit()
	})

	d.passwordEntry.ConnectActivate(func() {
		d.submit()
	})

	d.identityDropdown.NotifyProperty("selected", func() {
		newIdx := int(d.identityDropdown.Selected())
		if newIdx >= 0 && newIdx < len(d.identities) && newIdx != d.selectedIdx {
			d.selectedIdx = newIdx
			d.passwordEntry.SetText("")
			d.errorLabel.SetVisible(false)
			if d.onSwitchUser != nil {
				d.onSwitchUser(d.taskId, newIdx)
			}
		}
	})

	// Dismiss on Escape key
	keyCtrl := gtk.NewEventControllerKey()
	keyCtrl.ConnectKeyPressed(func(keyval, keycode uint, state gdk.ModifierType) bool {
		if keyval == gdk.KEY_Escape {
			d.Cancel()
			return true
		}
		return false
	})
	d.window.AddController(keyCtrl)
}

func (d *AuthDialog) SelectedIdentityIndex() int {
	return d.selectedIdx
}

func (d *AuthDialog) submit() {
	if d.closed {
		return
	}
	pass := d.passwordEntry.Text()
	d.authBtn.SetSensitive(false)
	d.passwordEntry.SetSensitive(false)
	if d.onResponse != nil {
		d.onResponse(d.taskId, pass)
	}
}

func (d *AuthDialog) Open(
	taskId uintptr,
	actionId string,
	message string,
	iconName string,
	cookie string,
	identities []string,
) {
	d.taskId = taskId
	d.cookie = cookie
	d.identities = identities
	d.selectedIdx = 0
	d.closed = false

	if iconName != "" {
		d.icon.SetFromIconName(iconName)
	} else {
		d.icon.SetFromIconName("dialog-password-symbolic")
	}

	if actionId != "" {
		d.actionLabel.SetText(actionId)
		d.actionLabel.SetVisible(true)
	} else {
		d.actionLabel.SetVisible(false)
	}

	if message != "" {
		d.messageLabel.SetText(message)
	} else {
		d.messageLabel.SetText("Authentication is required to perform this action.")
	}

	if len(identities) > 1 {
		items := make([]string, len(identities))
		for i, id := range identities {
			items[i] = formatIdentity(id)
		}
		strList := gtk.NewStringList(items)
		d.identityDropdown.SetModel(strList)
		d.identityDropdown.SetSelected(0)
		d.identityBox.SetVisible(true)
	} else {
		d.identityBox.SetVisible(false)
	}

	d.passwordEntry.SetText("")
	d.passwordEntry.SetSensitive(true)
	d.errorLabel.SetText("")
	d.errorLabel.SetVisible(false)
	d.authBtn.SetSensitive(true)

	d.window.Present()

	// Focus password input after window is mapped
	glib.IdleAdd(func() {
		d.passwordEntry.GrabFocus()
	})
}

func (d *AuthDialog) ShowError(msg string) {
	if d.closed {
		return
	}
	d.errorLabel.SetText(msg)
	d.errorLabel.SetVisible(true)
	d.passwordEntry.SetSensitive(true)
	d.passwordEntry.SetText("")
	d.authBtn.SetSensitive(true)
	d.passwordEntry.GrabFocus()
}

func (d *AuthDialog) ShowInfo(msg string) {
	if d.closed {
		return
	}
	slog.Info("polkit info", "message", msg)
}

func (d *AuthDialog) SetPrompt(prompt string) {
	if d.closed {
		return
	}
	clean := strings.TrimRight(prompt, ": \t\n")
	if clean != "" {
		d.messageLabel.SetText(clean)
	}
}

func (d *AuthDialog) Close() {
	if d.closed {
		return
	}
	d.closed = true
	d.window.SetVisible(false)
}

func (d *AuthDialog) Cancel() {
	if d.closed {
		return
	}
	d.Close()
	if d.onCancel != nil {
		d.onCancel(d.taskId)
	}
}

func (d *AuthDialog) Destroy() {
	d.Close()
	if d.window != nil {
		if d.window.Realized() {
			d.window.Destroy()
		}
		d.window = nil
	}
}

func formatIdentity(id string) string {
	if strings.HasPrefix(id, "unix-user:") {
		u := strings.TrimPrefix(id, "unix-user:")
		if u == "root" {
			return "Administrator (root)"
		}
		return fmt.Sprintf("%s (User)", u)
	}
	return id
}
