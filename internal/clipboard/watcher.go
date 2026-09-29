package clipboard

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"phalune/internal/config"

	"github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gdkpixbuf/v2"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	glibv2 "github.com/diamondburned/gotk4/pkg/glib/v2"
)

const (
	mimeImagePrefix = "image/"
	mimeText        = "text/plain;charset=utf-8"
	mimeTextPlain   = "text/plain"
	mimeFilesNautil = "x-special/nautilus-clipboard"
	mimeFilesGnome  = "text/x-special/gnome-copied-files"
	mimeURIList     = "text/uri-list"
	mimeImagePNG    = "image/png"
)

// Watcher records clipboard changes into a Store and can restore entries back
// to the clipboard. It must be created and used on the GTK main loop.
type Watcher struct {
	store    *Store
	ctx      context.Context
	cancel   context.CancelFunc
	mu       sync.Mutex
	cfg      config.ClipboardConfig
	onChange []func()
	paused   bool
}

// NewWatcher starts watching the default display clipboard.
func NewWatcher(cfg config.ClipboardConfig) (*Watcher, error) {
	display := gdk.DisplayGetDefault()
	if display == nil {
		return nil, fmt.Errorf("no gdk display")
	}

	store := NewStore(cfg.MaxEntries, cfg.Persist)

	ctx, cancel := context.WithCancel(context.Background())

	w := &Watcher{
		store:  store,
		ctx:    ctx,
		cancel: cancel,
		cfg:    cfg,
	}

	clip := display.Clipboard()
	if clip == nil {
		cancel()
		return nil, fmt.Errorf("no clipboard on default display")
	}

	clip.ConnectChanged(func() {
		// Capture synchronously on the main loop; reads are async though.
		w.capture(clip)
	})

	go func() {
		<-ctx.Done()
	}()

	return w, nil
}

// Stop stops the watcher.
func (w *Watcher) Stop() {
	if w != nil && w.cancel != nil {
		w.cancel()
	}
}

// OnChange registers a callback fired (on the main loop) after the history
// changed.
func (w *Watcher) OnChange(cb func()) {
	w.mu.Lock()
	w.onChange = append(w.onChange, cb)
	w.mu.Unlock()
}

// SetPaused disables capture (used while pasting back to avoid loops).
func (w *Watcher) SetPaused(paused bool) {
	w.paused = paused
}

// Store exposes the backing history store.
func (w *Watcher) Store() *Store {
	return w.store
}

// SetConfig applies new clipboard settings; capacity changes trim the
// history. Must be called from the GTK main loop.
func (w *Watcher) SetConfig(cfg config.ClipboardConfig) {
	w.mu.Lock()
	w.cfg = cfg
	w.mu.Unlock()

	if w.store.MaxEntries() != cfg.MaxEntries && cfg.MaxEntries > 0 {
		w.store.Resize(cfg.MaxEntries)
	}
}

func (w *Watcher) isBlacklisted(clip *gdk.Clipboard, text string) bool {
	if clip != nil {
		formats := clip.Formats()
		if formats != nil {
			if formats.ContainMIMEType("x-kde-passwordManagerHint") ||
				formats.ContainMIMEType("application/x-password-manager-hint") ||
				formats.ContainMIMEType("application/x-keepassxc-metadata") {
				return true
			}
		}
	}

	w.mu.Lock()
	blacklist := append([]string{}, w.cfg.Blacklist...)
	blacklist = append(blacklist, w.cfg.IgnoredApps...)
	w.mu.Unlock()

	for _, item := range blacklist {
		item = strings.TrimSpace(strings.ToLower(item))
		if item == "" {
			continue
		}
		if clip != nil {
			formats := clip.Formats()
			if formats != nil && formats.ContainMIMEType(item) {
				return true
			}
		}
		if text != "" && strings.Contains(strings.ToLower(text), item) {
			return true
		}
	}
	return false
}

func (w *Watcher) capture(clip *gdk.Clipboard) {
	if w.paused || w.isBlacklisted(clip, "") {
		return
	}
	formats := clip.Formats()
	if formats == nil {
		return
	}

	if formats.ContainMIMEType(mimeFilesGnome) || formats.ContainMIMEType(mimeFilesNautil) {
		w.readTextForFiles(clip)
		return
	}
	if formats.ContainMIMEType(mimeURIList) {
		w.readTextForFiles(clip)
		return
	}

	if formats.ContainMIMEType(mimeImagePrefix) {
		w.readImage(clip)
		return
	}

	if formats.ContainMIMEType(mimeText) || formats.ContainMIMEType(mimeTextPlain) {
		w.readText(clip)
	}
}

func (w *Watcher) readText(clip *gdk.Clipboard) {
	clip.ReadTextAsync(w.ctx, func(res gio.AsyncResulter) {
		if w.ctx.Err() != nil {
			return
		}
		text, err := clip.ReadTextFinish(res)
		if err != nil {
			return
		}
		w.recordText(text)
	})
}

func (w *Watcher) readTextForFiles(clip *gdk.Clipboard) {
	clip.ReadTextAsync(w.ctx, func(res gio.AsyncResulter) {
		if w.ctx.Err() != nil {
			return
		}
		text, err := clip.ReadTextFinish(res)
		if err != nil {
			return
		}
		uris := splitURIs(text)
		if len(uris) == 0 {
			return
		}
		w.recordFiles(fileCopyPayload(uris))
	})
}

func (w *Watcher) readImage(clip *gdk.Clipboard) {
	clip.ReadTextureAsync(w.ctx, func(res gio.AsyncResulter) {
		if w.ctx.Err() != nil {
			return
		}
		tex, err := clip.ReadTextureFinish(res)
		if err != nil || tex == nil {
			return
		}
		w.recordTexture(tex)
	})
}

func (w *Watcher) recordTexture(tex gdk.Texturer) {
	texture := tex.(*gdk.Texture)
	width, height := texture.Width(), texture.Height()
	if width <= 0 || height <= 0 {
		return
	}

	pb := gdk.PixbufGetFromTexture(texture)
	if pb == nil {
		return
	}
	png, err := pb.SaveToBufferv("png", nil, nil)
	if err != nil {
		slog.Debug("clipboard: image encode failed", "error", err)
		return
	}
	if w.cfg.MaxImageBytes > 0 && len(png) > w.cfg.MaxImageBytes {
		slog.Debug("clipboard: image too large, skipped", "bytes", len(png))
		return
	}
	w.storeImage(png, width, height)
}

func (w *Watcher) storeImage(png []byte, width, height int) {
	path, err := w.store.CreateImageFile(png)
	if err != nil {
		slog.Debug("clipboard: image save failed", "error", err)
		return
	}

	w.add(&Entry{
		Kind:   KindImage,
		Width:  width,
		Height: height,
	}, path)
}

func (w *Watcher) recordText(text string) {
	if trimText(text) == "" || w.isBlacklisted(nil, text) {
		return
	}
	if w.cfg.MaxTextBytes > 0 && len(text) > w.cfg.MaxTextBytes {
		slog.Debug("clipboard: text too large, skipped", "bytes", len(text))
		return
	}
	w.add(&Entry{Kind: KindText, Text: text}, "")
}

// recordFiles stores a copied-files entry.
func (w *Watcher) recordFiles(payload string) {
	if len(splitURIs(payload)) == 0 {
		return
	}
	w.add(&Entry{Kind: KindFiles, Text: payload}, "")
}

func (w *Watcher) add(e *Entry, imgPath string) {
	if imgPath != "" {
		e.FilePath = imgPath
	}
	w.store.Add(e)
	w.notifyChange()
}

func (w *Watcher) notifyChange() {
	w.mu.Lock()
	cbs := make([]func(), len(w.onChange))
	copy(cbs, w.onChange)
	w.mu.Unlock()
	for _, cb := range cbs {
		cb()
	}
}

// Paste writes an entry back to the clipboard.
func (w *Watcher) Paste(entry *Entry) {
	if entry == nil {
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

	w.store.Touch(entry.ID)

	switch entry.Kind {
	case KindImage:
		pb, err := gdkpixbuf.NewPixbufFromFile(entry.FilePath)
		if err != nil || pb == nil {
			slog.Warn("clipboard: cannot decode image payload", "path", entry.FilePath, "error", err)
			return
		}
		tex := gdk.NewTextureForPixbuf(pb)
		w.SetPaused(true)
		clip.SetTexture(tex)
		glibTimeout(300, func() { w.SetPaused(false) })

	case KindFiles:
		// Provide the uri-list bytes; GTK maps text/uri-list to file drops and
		// most apps accept the plain text fallback.
		w.SetPaused(true)
		clip.SetContent(gdk.NewContentProviderForBytes(mimeURIList, glibv2.NewBytes([]byte(entry.Text))))
		glibTimeout(300, func() { w.SetPaused(false) })

	default:
		w.SetPaused(true)
		clip.SetText(entry.Text)
		glibTimeout(300, func() { w.SetPaused(false) })
	}
}

// glibTimeout runs fn once after delayMs milliseconds on the main loop.
func glibTimeout(delayMs uint, fn func()) {
	var handle glib.SourceHandle
	handle = glib.TimeoutAdd(delayMs, func() bool {
		fn()
		glib.SourceRemove(handle)
		return false
	})
	_ = handle
}

func trimText(s string) string {
	for len(s) > 0 && (s[0] == '\n' || s[0] == '\r' || s[0] == ' ' || s[0] == '\t') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r' || s[len(s)-1] == ' ' || s[len(s)-1] == '\t') {
		s = s[:len(s)-1]
	}
	return s
}
