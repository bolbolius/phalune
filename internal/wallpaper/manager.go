package wallpaper

import (
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"phalune/internal/config"

	"github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

var supportedExts = map[string]bool{
	".png":  true,
	".jpg":  true,
	".jpeg": true,
	".webp": true,
	".svg":  true,
	".bmp":  true,
	".gif":  true,
	".avif": true,
	".tiff": true,
	".tif":  true,
}

type monitorSurface struct {
	connector string
	monitor   *gdk.Monitor
	window    *gtk.Window
	picture   *gtk.Picture
}

// Manager coordinates wallpaper rendering across monitors and handles slideshow rotation.
type Manager struct {
	mu                 sync.Mutex
	app                *gtk.Application
	cfg                config.WallpaperConfig
	surfaces           map[string]*monitorSurface
	tickerStop         chan struct{}
	fileIndex          int
	currentWallpaper   string
	onWallpaperChanged func(path string)
}

func New(app *gtk.Application, cfg config.WallpaperConfig) *Manager {
	return &Manager{
		app:      app,
		cfg:      cfg,
		surfaces: make(map[string]*monitorSurface),
	}
}

// SetOnWallpaperChanged sets a callback fired whenever a new wallpaper is loaded onto the primary/active surface.
func (m *Manager) SetOnWallpaperChanged(fn func(path string)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onWallpaperChanged = fn
}

// CurrentWallpaper returns the active wallpaper image path.
func (m *Manager) CurrentWallpaper() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.currentWallpaper
}

// Start begins wallpaper presentation and slideshow tickers if enabled.
func (m *Manager) Start() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.cfg.Enabled {
		return
	}

	m.syncMonitorsLocked()
	m.startSlideshowLocked()
}

// Stop tears down all wallpaper windows and halts slideshow goroutines.
func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.stopSlideshowLocked()

	for conn, s := range m.surfaces {
		if s.window != nil {
			s.window.Destroy()
		}
		delete(m.surfaces, conn)
	}
}

// UpdateConfig updates wallpaper settings live during hot-reload.
func (m *Manager) UpdateConfig(newCfg config.WallpaperConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()

	wasEnabled := m.cfg.Enabled
	m.cfg = newCfg

	if !newCfg.Enabled {
		if wasEnabled {
			m.stopSlideshowLocked()
			for conn, s := range m.surfaces {
				if s.window != nil {
					s.window.Destroy()
				}
				delete(m.surfaces, conn)
			}
		}
		return
	}

	m.stopSlideshowLocked()
	m.syncMonitorsLocked()
	m.refreshAllLocked()
	m.startSlideshowLocked()
}

// SyncMonitors reconciles wallpaper surfaces when monitors are added or removed.
// It schedules surface creation and teardown onto the GTK main loop.
func (m *Manager) SyncMonitors(currentMonitors map[string]*gdk.Monitor) {
	monitorsCopy := make(map[string]*gdk.Monitor, len(currentMonitors))
	for k, v := range currentMonitors {
		monitorsCopy[k] = v
	}

	glib.IdleAdd(func() {
		m.mu.Lock()
		defer m.mu.Unlock()

		if !m.cfg.Enabled {
			return
		}

		// Remove disappeared monitors
		for conn, s := range m.surfaces {
			if _, exists := monitorsCopy[conn]; !exists {
				if s.window != nil {
					s.window.Destroy()
				}
				delete(m.surfaces, conn)
			}
		}

		// Add new monitors
		for conn, mon := range monitorsCopy {
			if _, exists := m.surfaces[conn]; !exists {
				surf := m.createSurface(conn, mon)
				if surf != nil {
					m.surfaces[conn] = surf
					m.loadImageOnSurface(surf)
					surf.window.Present()
				}
			}
		}
	})
}

func (m *Manager) syncMonitorsLocked() {
	display := gdk.DisplayGetDefault()
	if display == nil {
		return
	}
	monitorsList := display.Monitors()
	if monitorsList == nil {
		return
	}
	n := monitorsList.NItems()

	current := make(map[string]*gdk.Monitor)
	for i := uint(0); i < n; i++ {
		item := monitorsList.Item(i)
		if item == nil {
			continue
		}
		mon, ok := item.Cast().(*gdk.Monitor)
		if ok {
			current[mon.Connector()] = mon
		}
	}

	// Remove unmanaged
	for conn, s := range m.surfaces {
		if _, exists := current[conn]; !exists {
			if s.window != nil {
				s.window.Destroy()
			}
			delete(m.surfaces, conn)
		}
	}

	// Add missing
	for conn, mon := range current {
		if _, exists := m.surfaces[conn]; !exists {
			surf := m.createSurface(conn, mon)
			if surf != nil {
				m.surfaces[conn] = surf
				m.loadImageOnSurface(surf)
				surf.window.Present()
			}
		}
	}
}

func (m *Manager) createSurface(connector string, mon *gdk.Monitor) *monitorSurface {
	win := gtk.NewWindow()
	if m.app != nil {
		win.SetApplication(m.app)
	}
	win.SetDecorated(false)
	win.AddCSSClass("wallpaper-window")

	if err := ConfigureSurface(win, mon); err != nil {
		slog.Warn("wallpaper: failed to configure layer surface", "connector", connector, "error", err)
	}

	pic := gtk.NewPicture()
	pic.SetHExpand(true)
	pic.SetVExpand(true)
	pic.SetCanShrink(true)
	applyContentFit(pic, m.cfg.Mode)

	win.SetChild(pic)

	return &monitorSurface{
		connector: connector,
		monitor:   mon,
		window:    win,
		picture:   pic,
	}
}

func applyContentFit(pic *gtk.Picture, mode string) {
	switch strings.ToLower(mode) {
	case "fit", "contain":
		pic.SetContentFit(gtk.ContentFitContain)
	case "stretch":
		pic.SetContentFit(gtk.ContentFitFill)
	case "center":
		pic.SetContentFit(gtk.ContentFitScaleDown)
	case "fill", "cover", "crop":
		fallthrough
	default:
		pic.SetContentFit(gtk.ContentFitCover)
	}
}

func (m *Manager) refreshAllLocked() {
	for _, surf := range m.surfaces {
		applyContentFit(surf.picture, m.cfg.Mode)
		m.loadImageOnSurface(surf)
	}
}

func (m *Manager) resolvePath(connector string) string {
	if override, ok := m.cfg.Outputs[connector]; ok && override != "" {
		return expandUserPath(override)
	}
	return expandUserPath(m.cfg.Path)
}

func expandUserPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

func (m *Manager) loadImageOnSurface(surf *monitorSurface) {
	target := m.resolvePath(surf.connector)
	if target == "" {
		surf.picture.SetPaintable(nil)
		return
	}

	fi, err := os.Stat(target)
	if err != nil {
		slog.Debug("wallpaper: target path not accessible", "path", target, "error", err)
		surf.picture.SetPaintable(nil)
		return
	}

	filePath := target
	if fi.IsDir() {
		files := listImageFiles(target)
		if len(files) == 0 {
			slog.Debug("wallpaper: directory has no supported images", "dir", target)
			surf.picture.SetPaintable(nil)
			return
		}
		idx := m.fileIndex % len(files)
		filePath = files[idx]
	}

	surf.picture.SetFilename(filePath)

	if m.currentWallpaper != filePath {
		m.currentWallpaper = filePath
		if m.onWallpaperChanged != nil {
			go m.onWallpaperChanged(filePath)
		}
	}
}

func (m *Manager) advanceSlideshow() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.fileIndex++
	for _, surf := range m.surfaces {
		target := m.resolvePath(surf.connector)
		if fi, err := os.Stat(target); err == nil && fi.IsDir() {
			m.loadImageOnSurface(surf)
		}
	}
}

func (m *Manager) startSlideshowLocked() {
	dur := m.cfg.Interval.Duration
	if dur <= 0 {
		return
	}
	if dur < time.Second {
		dur = time.Second
	}

	ticker := time.NewTicker(dur)
	stop := make(chan struct{})
	m.tickerStop = stop

	go func() {
		for {
			select {
			case <-stop:
				ticker.Stop()
				return
			case <-ticker.C:
				glib.IdleAdd(func() {
					m.advanceSlideshow()
				})
			}
		}
	}()
}

func (m *Manager) stopSlideshowLocked() {
	if m.tickerStop != nil {
		close(m.tickerStop)
		m.tickerStop = nil
	}
}

func listImageFiles(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if supportedExts[ext] {
			out = append(out, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(out)
	return out
}
