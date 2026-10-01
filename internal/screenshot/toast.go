package screenshot

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"phalune/internal/config"
	"phalune/ui"

	"github.com/diamondburned/gotk4-layer-shell/pkg/gtk4layershell"
	"github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gdkpixbuf/v2"
	glibv2 "github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

const defaultToastTimeout = 6 * time.Second

type Toast struct {
	window  *gtk.Window
	picture *gtk.Picture
	info    *gtk.Label
	copyBtn *gtk.Button
	saveBtn *gtk.Button
	openBtn *gtk.Button

	timeout time.Duration
	saveDir string

	mu       sync.Mutex
	timer    *time.Timer
	lastPNG  []byte
	lastPath string
}

func NewToast(app *gtk.Application, cfg config.ScreenshotConfig) (*Toast, error) {
	if !gtk4layershell.IsSupported() {
		return nil, fmt.Errorf("Wayland compositor does not support wlr-layer-shell protocol")
	}

	win := gtk.NewWindow()
	win.SetApplication(app)
	win.SetTitle("phalune-screenshot")
	win.SetDecorated(false)
	win.AddCSSClass("screenshot-window")

	gtk4layershell.InitForWindow(win)
	gtk4layershell.SetNamespace(win, "phalune-screenshot")
	gtk4layershell.SetLayer(win, gtk4layershell.LayerShellLayerOverlay)

	gtk4layershell.SetAnchor(win, gtk4layershell.LayerShellEdgeBottom, true)
	gtk4layershell.SetExclusiveZone(win, -1)
	gtk4layershell.SetKeyboardMode(win, gtk4layershell.LayerShellKeyboardModeOnDemand)

	builder := gtk.NewBuilderFromString(ui.ScreenshotToast)
	card := builder.GetObject("toast_card").Cast().(*gtk.Box)
	picture := builder.GetObject("toast_picture").Cast().(*gtk.Picture)
	info := builder.GetObject("toast_info").Cast().(*gtk.Label)
	copyBtn := builder.GetObject("copy_button").Cast().(*gtk.Button)
	saveBtn := builder.GetObject("save_button").Cast().(*gtk.Button)
	openBtn := builder.GetObject("open_button").Cast().(*gtk.Button)
	closeBtn := builder.GetObject("close_button").Cast().(*gtk.Button)

	win.SetChild(card)

	timeout := cfg.ToastTimeout.Duration
	if timeout <= 0 {
		timeout = defaultToastTimeout
	}

	t := &Toast{
		window:  win,
		picture: picture,
		info:    info,
		copyBtn: copyBtn,
		saveBtn: saveBtn,
		openBtn: openBtn,
		timeout: timeout,
		saveDir: cfg.SaveDir,
	}

	copyBtn.ConnectClicked(func() { t.onCopy() })
	saveBtn.ConnectClicked(func() { t.onSave() })
	openBtn.ConnectClicked(func() { t.onOpen() })
	closeBtn.ConnectClicked(func() { t.Hide() })

	keyCtrl := gtk.NewEventControllerKey()
	keyCtrl.ConnectKeyPressed(func(keyval, keycode uint, state gdk.ModifierType) bool {
		if keyval == gdk.KEY_Escape {
			t.Hide()
			return true
		}
		return false
	})
	win.AddController(keyCtrl)

	return t, nil
}

func (t *Toast) Show(png []byte, shotPath string) {
	if t == nil {
		return
	}
	if shotPath == "" && len(png) > 0 {
		shotPath = writeTempPNG(png)
	}

	t.mu.Lock()
	t.lastPNG = png
	t.lastPath = shotPath
	timeout := t.timeout
	t.mu.Unlock()

	glib.IdleAdd(func() {
		if t.window == nil {
			return
		}
		t.mu.Lock()
		path := t.lastPath
		png := t.lastPNG
		t.mu.Unlock()

		loaded := false
		if path != "" {
			if pb, err := gdkpixbuf.NewPixbufFromFileAtScale(path, 440, 248, true); err == nil && pb != nil {
				t.picture.SetPixbuf(pb)
				loaded = true
			}
			t.info.SetText(path)
			t.info.SetTooltipText(path)
		} else {
			t.info.SetText("Screenshot preview")
		}
		if !loaded {
			t.picture.SetPaintable(nil)
		}

		t.copyBtn.SetSensitive(len(png) > 0)
		t.saveBtn.SetSensitive(len(png) > 0)
		t.openBtn.SetSensitive(path != "")

		t.window.SetVisible(true)
		t.window.Present()
	})

	t.armTimer(timeout)
}

func writeTempPNG(png []byte) string {
	dir := os.TempDir()
	if runtimeDir := os.Getenv("XDG_RUNTIME_DIR"); runtimeDir != "" {
		dir = filepath.Join(runtimeDir, "phalune-shots")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ""
	}
	f, err := os.CreateTemp(dir, "shot-*.png")
	if err != nil {
		return ""
	}
	defer f.Close()
	if _, err := f.Write(png); err != nil {
		f.Close()
		os.Remove(f.Name())
		return ""
	}
	return f.Name()
}

func (t *Toast) armTimer(timeout time.Duration) {
	t.mu.Lock()
	if t.timer != nil {
		t.timer.Stop()
	}
	timer := time.AfterFunc(timeout, func() {
		t.Hide()
	})
	t.timer = timer
	t.mu.Unlock()
}

func (t *Toast) Hide() {
	if t == nil {
		return
	}
	glib.IdleAdd(func() {
		if t.window == nil {
			return
		}
		t.mu.Lock()
		if t.timer != nil {
			t.timer.Stop()
			t.timer = nil
		}
		t.mu.Unlock()
		t.window.SetVisible(false)
	})
}

func (t *Toast) Destroy() {
	if t == nil || t.window == nil {
		return
	}
	t.mu.Lock()
	if t.timer != nil {
		t.timer.Stop()
		t.timer = nil
	}
	win := t.window
	t.window = nil
	t.mu.Unlock()

	if win.Realized() {
		win.Destroy()
	}
}

func (t *Toast) UpdateConfig(cfg config.ScreenshotConfig) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if cfg.ToastTimeout.Duration > 0 {
		t.timeout = cfg.ToastTimeout.Duration
	}
	if cfg.SaveDir != "" {
		t.saveDir = cfg.SaveDir
	}
}

func (t *Toast) CopyToClipboard() {
	if t == nil {
		return
	}
	t.mu.Lock()
	png := t.lastPNG
	path := t.lastPath
	t.mu.Unlock()
	if len(png) == 0 {
		return
	}

	var tex *gdk.Texture
	if path != "" {
		if loaded, err := gdk.NewTextureFromFilename(path); err == nil {
			tex = loaded
		}
	}
	if tex == nil && len(png) > 0 {
		if loaded, err := gdk.NewTextureFromBytes(glibv2.NewBytes(png)); err == nil {
			tex = loaded
		}
	}
	if tex == nil {
		return
	}

	display := gdk.DisplayGetDefault()
	if display == nil {
		return
	}
	clip := display.Clipboard()
	if clip == nil {
		return
	}
	clip.SetTexture(tex)
}

func (t *Toast) onCopy() {
	t.CopyToClipboard()
	t.Hide()
}

func (t *Toast) onSave() {
	t.mu.Lock()
	png := t.lastPNG
	dir := t.saveDir
	t.mu.Unlock()
	if len(png) == 0 {
		return
	}

	path, err := Save(dir, png, time.Now())
	if err != nil {
		slog.Warn("screenshot: save failed", "error", err)
		return
	}

	t.mu.Lock()
	t.lastPath = path
	t.mu.Unlock()
	t.Hide()
}

func (t *Toast) onOpen() {
	t.mu.Lock()
	path := t.lastPath
	t.mu.Unlock()
	if path == "" {
		return
	}
	if err := exec.Command("xdg-open", path).Start(); err != nil {
		slog.Warn("screenshot: open failed", "error", err)
		return
	}
	t.Hide()
}
