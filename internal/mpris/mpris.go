package mpris

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
)

const (
	mprisPrefix    = "org.mpris.MediaPlayer2."
	mprisInterface = "org.mpris.MediaPlayer2.Player"
	propsInterface = "org.freedesktop.DBus.Properties"

	// maxArtBytes caps how much album art is downloaded into the cache.
	maxArtBytes = 10 << 20
)

// PlaybackStatus represents player state.
type PlaybackStatus string

const (
	PlaybackPlaying PlaybackStatus = "Playing"
	PlaybackPaused  PlaybackStatus = "Paused"
	PlaybackStopped PlaybackStatus = "Stopped"
)

// PlayerState contains current playback information of a player.
type PlayerState struct {
	BusName       string
	Owner         string
	Identity      string
	Title         string
	Artist        string
	Album         string
	ArtURL        string
	LocalArtPath  string
	Status        PlaybackStatus
	CanGoNext     bool
	CanGoPrevious bool
	CanPlay       bool
	CanPause      bool
	CanControl    bool
}

// Controller manages discovering and interacting with MPRIS players.
type Controller struct {
	conn       *dbus.Conn
	mu         sync.RWMutex
	players    map[string]*PlayerState
	ownerToBus map[string]string
	artCache   map[string]string
	httpClient *http.Client
	cacheDir   string
	activeBus  string
	callbacks  []func(active *PlayerState)
	ctx        context.Context
	cancel     context.CancelFunc
}

// New creates and starts a new MPRIS controller.
func New() (*Controller, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, fmt.Errorf("connect session bus: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cacheDir := filepath.Join(os.TempDir(), "phalune-media-art")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		slog.Warn("mpris: failed to create art cache dir", "dir", cacheDir, "error", err)
	}
	pruneArtCache(cacheDir)

	c := &Controller{
		conn:       conn,
		players:    make(map[string]*PlayerState),
		ownerToBus: make(map[string]string),
		artCache:   make(map[string]string),
		httpClient: &http.Client{Timeout: 5 * time.Second},
		cacheDir:   cacheDir,
		callbacks:  make([]func(*PlayerState), 0),
		ctx:        ctx,
		cancel:     cancel,
	}

	c.initPlayers()
	go c.listenBusEvents()

	return c, nil
}

// Close disconnects the controller.
func (c *Controller) Close() error {
	c.cancel()
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// OnChange registers a listener called whenever active player state changes.
// The callback is invoked synchronously (outside the controller lock) with the
// current active state.
func (c *Controller) OnChange(cb func(active *PlayerState)) {
	c.mu.Lock()
	c.callbacks = append(c.callbacks, cb)
	initial := c.getActiveLocked()
	c.mu.Unlock()

	if cb != nil {
		cb(initial)
	}
}

// ActivePlayer returns a copy of current active player state or nil if none.
func (c *Controller) ActivePlayer() *PlayerState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.getActiveLocked()
}

func (c *Controller) getActiveLocked() *PlayerState {
	if c.activeBus == "" {
		return nil
	}
	st, ok := c.players[c.activeBus]
	if !ok {
		return nil
	}
	cpy := *st
	return &cpy
}

func (c *Controller) notifyChange() {
	c.mu.RLock()
	active := c.getActiveLocked()
	listeners := make([]func(*PlayerState), len(c.callbacks))
	copy(listeners, c.callbacks)
	c.mu.RUnlock()

	for _, cb := range listeners {
		if cb != nil {
			cb(active)
		}
	}
}

func (c *Controller) initPlayers() {
	var names []string
	err := c.conn.BusObject().Call("org.freedesktop.DBus.ListNames", 0).Store(&names)
	if err != nil {
		slog.Warn("mpris: failed to list dbus names", "error", err)
		return
	}

	c.mu.Lock()
	busNames := make([]string, 0, len(names))
	for _, name := range names {
		if strings.HasPrefix(name, mprisPrefix) {
			busNames = append(busNames, name)
		}
	}
	c.mu.Unlock()

	for _, name := range busNames {
		st := c.fetchPlayerState(name)
		if st != nil {
			c.mu.Lock()
			if st.Owner != "" {
				c.ownerToBus[st.Owner] = name
			}
			c.players[name] = st
			c.recalculateActiveLocked()
			c.mu.Unlock()
		}
	}

	c.mu.Lock()
	c.recalculateActiveLocked()
	c.mu.Unlock()
	c.notifyChange()
}

// fetchPlayerState queries a player over D-Bus without holding the controller
// lock; callers must publish the result under the lock.
func (c *Controller) fetchPlayerState(busName string) *PlayerState {
	obj := c.conn.Object(busName, "/org/mpris/MediaPlayer2")

	var identity string
	propIdent, err := obj.GetProperty("org.mpris.MediaPlayer2.Identity")
	if err == nil {
		if s, ok := propIdent.Value().(string); ok {
			identity = s
		}
	}
	if identity == "" {
		identity = strings.TrimPrefix(busName, mprisPrefix)
	}
	if idx := strings.Index(identity, ".instance_"); idx != -1 {
		identity = identity[:idx]
	}
	if len(identity) > 0 {
		identity = strings.ToUpper(identity[:1]) + identity[1:]
	}

	var owner string
	_ = c.conn.BusObject().Call("org.freedesktop.DBus.GetNameOwner", 0, busName).Store(&owner)

	st := &PlayerState{
		BusName:  busName,
		Owner:    owner,
		Identity: identity,
		Status:   PlaybackStopped,
	}

	// Read PlaybackStatus
	statusProp, err := obj.GetProperty(mprisInterface + ".PlaybackStatus")
	if err == nil {
		if s, ok := statusProp.Value().(string); ok {
			st.Status = PlaybackStatus(s)
		}
	}

	// Read Capabilities
	if canGoNext, err := obj.GetProperty(mprisInterface + ".CanGoNext"); err == nil {
		if b, ok := canGoNext.Value().(bool); ok {
			st.CanGoNext = b
		}
	}
	if canGoPrev, err := obj.GetProperty(mprisInterface + ".CanGoPrevious"); err == nil {
		if b, ok := canGoPrev.Value().(bool); ok {
			st.CanGoPrevious = b
		}
	}
	if canPlay, err := obj.GetProperty(mprisInterface + ".CanPlay"); err == nil {
		if b, ok := canPlay.Value().(bool); ok {
			st.CanPlay = b
		}
	}
	if canPause, err := obj.GetProperty(mprisInterface + ".CanPause"); err == nil {
		if b, ok := canPause.Value().(bool); ok {
			st.CanPause = b
		}
	}
	if canControl, err := obj.GetProperty(mprisInterface + ".CanControl"); err == nil {
		if b, ok := canControl.Value().(bool); ok {
			st.CanControl = b
		}
	} else {
		st.CanControl = true
	}

	// Read Metadata
	metaProp, err := obj.GetProperty(mprisInterface + ".Metadata")
	if err == nil {
		if metaMap, ok := metaProp.Value().(map[string]dbus.Variant); ok {
			c.parseMetadata(metaMap, st)
		}
	}

	return st
}

func (c *Controller) parseMetadata(meta map[string]dbus.Variant, st *PlayerState) {
	if val, ok := meta["xesam:title"]; ok {
		if s, ok := val.Value().(string); ok {
			st.Title = s
		}
	}
	if val, ok := meta["xesam:album"]; ok {
		if s, ok := val.Value().(string); ok {
			st.Album = s
		}
	}
	if val, ok := meta["xesam:artist"]; ok {
		switch v := val.Value().(type) {
		case []string:
			st.Artist = strings.Join(v, ", ")
		case string:
			st.Artist = v
		}
	}
	if val, ok := meta["mpris:artUrl"]; ok {
		if s, ok := val.Value().(string); ok {
			st.ArtURL = s
			c.resolveArtURL(st)
		}
	}
}

func (c *Controller) resolveArtURL(st *PlayerState) {
	raw := strings.TrimSpace(st.ArtURL)
	if raw == "" {
		st.LocalArtPath = ""
		return
	}

	if strings.HasPrefix(raw, "file://") {
		st.LocalArtPath = strings.TrimPrefix(raw, "file://")
		return
	}
	if strings.HasPrefix(raw, "/") {
		st.LocalArtPath = raw
		return
	}

	if strings.HasPrefix(raw, "data:") {
		// data:image/jpeg;base64,...
		commaIdx := strings.Index(raw, ",")
		if commaIdx != -1 {
			data := raw[commaIdx+1:]
			decoded, err := base64.StdEncoding.DecodeString(data)
			if err == nil && len(decoded) > 0 {
				h := sha256.Sum256([]byte(raw))
				dest := filepath.Join(c.cacheDir, hex.EncodeToString(h[:16])+".img")
				if _, err := os.Stat(dest); err != nil {
					_ = os.WriteFile(dest, decoded, 0644)
				}
				st.LocalArtPath = dest
				return
			}
		}
	}

	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		c.mu.RLock()
		cached, ok := c.artCache[raw]
		c.mu.RUnlock()
		if ok && cached != "" {
			st.LocalArtPath = cached
			return
		}

		// Download async
		go func(url, busName string) {
			h := sha256.Sum256([]byte(url))
			ext := filepath.Ext(url)
			if ext == "" || len(ext) > 5 {
				ext = ".img"
			}
			dest := filepath.Join(c.cacheDir, hex.EncodeToString(h[:16])+ext)

			// Check if file exists on disk
			if _, err := os.Stat(dest); err == nil {
				c.mu.Lock()
				c.artCache[url] = dest
				if p := c.players[busName]; p != nil && p.ArtURL == url {
					p.LocalArtPath = dest
				}
				c.mu.Unlock()
				c.notifyChange()
				return
			}

			resp, err := c.httpClient.Get(url)
			if err != nil {
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				return
			}

			tmpFile := dest + ".tmp"
			f, err := os.Create(tmpFile)
			if err != nil {
				return
			}
			_, err = io.Copy(f, io.LimitReader(resp.Body, maxArtBytes))
			f.Close()
			if err != nil {
				_ = os.Remove(tmpFile)
				return
			}

			if err := os.Rename(tmpFile, dest); err == nil {
				c.mu.Lock()
				c.artCache[url] = dest
				if p := c.players[busName]; p != nil && p.ArtURL == url {
					p.LocalArtPath = dest
				}
				c.mu.Unlock()
				c.notifyChange()
			}
		}(raw, st.BusName)
	}
}

func (c *Controller) recalculateActiveLocked() {
	// Prioritize Playing players, then Paused, then others. Bus names are
	// walked in sorted order so the result is deterministic.
	busNames := make([]string, 0, len(c.players))
	for name := range c.players {
		busNames = append(busNames, name)
	}
	sort.Strings(busNames)

	var candidatePlaying, candidatePaused, candidateAny string
	for _, name := range busNames {
		st := c.players[name]
		if candidatePlaying == "" && st.Status == PlaybackPlaying {
			candidatePlaying = name
		}
		if candidatePaused == "" && st.Status == PlaybackPaused {
			candidatePaused = name
		}
		if candidateAny == "" {
			candidateAny = name
		}
	}

	switch {
	case candidatePlaying != "":
		c.activeBus = candidatePlaying
	case candidatePaused != "":
		c.activeBus = candidatePaused
	case candidateAny != "":
		c.activeBus = candidateAny
	default:
		c.activeBus = ""
	}
}

func (c *Controller) listenBusEvents() {
	rules := []string{
		"type='signal',interface='org.freedesktop.DBus',member='NameOwnerChanged'",
		"type='signal',interface='org.freedesktop.DBus.Properties',member='PropertiesChanged',path='/org/mpris/MediaPlayer2'",
	}

	for _, rule := range rules {
		call := c.conn.BusObject().Call("org.freedesktop.DBus.AddMatch", 0, rule)
		if call.Err != nil {
			slog.Warn("mpris: AddMatch error", "rule", rule, "error", call.Err)
		}
	}

	ch := make(chan *dbus.Signal, 32)
	c.conn.Signal(ch)

	for {
		select {
		case <-c.ctx.Done():
			return
		case sig, ok := <-ch:
			if !ok {
				return
			}
			c.handleSignal(sig)
		}
	}
}

func (c *Controller) handleSignal(sig *dbus.Signal) {
	switch sig.Name {
	case "org.freedesktop.DBus.NameOwnerChanged":
		if len(sig.Body) < 3 {
			return
		}
		name, _ := sig.Body[0].(string)
		oldOwner, _ := sig.Body[1].(string)
		newOwner, _ := sig.Body[2].(string)

		if !strings.HasPrefix(name, mprisPrefix) {
			return
		}

		c.mu.Lock()
		if newOwner == "" {
			// Player closed
			if st, ok := c.players[name]; ok {
				if st.Owner != "" {
					delete(c.ownerToBus, st.Owner)
				}
				delete(c.players, name)
			}
			if c.activeBus == name {
				c.recalculateActiveLocked()
			}
		} else if oldOwner == "" || newOwner != "" {
			// Player registered or switched owner
			if oldOwner != "" {
				delete(c.ownerToBus, oldOwner)
			}
		}
		c.mu.Unlock()

		if newOwner != "" {
			// Query the player outside the lock; D-Bus calls may block.
			st := c.fetchPlayerState(name)
			if st != nil {
				c.mu.Lock()
				if st.Owner != "" {
					c.ownerToBus[st.Owner] = name
				}
				c.players[name] = st
				c.recalculateActiveLocked()
				c.mu.Unlock()
			}
		}
		c.notifyChange()

	case "org.freedesktop.DBus.Properties.PropertiesChanged":
		if len(sig.Body) < 2 {
			return
		}
		iface, _ := sig.Body[0].(string)
		if iface != mprisInterface {
			return
		}
		changed, _ := sig.Body[1].(map[string]dbus.Variant)

		c.mu.Lock()
		targetBus := c.ownerToBus[sig.Sender]
		if targetBus == "" && strings.HasPrefix(sig.Sender, mprisPrefix) {
			targetBus = sig.Sender
		}
		if targetBus == "" {
			// Fallback: try checking if only 1 player or iterate matching owner
			for bus, p := range c.players {
				if p.Owner == sig.Sender || bus == sig.Sender {
					targetBus = bus
					break
				}
			}
		}

		st := c.players[targetBus]
		if st != nil {
			if val, ok := changed["PlaybackStatus"]; ok {
				if s, ok := val.Value().(string); ok {
					st.Status = PlaybackStatus(s)
				}
			}
			if val, ok := changed["Metadata"]; ok {
				if meta, ok := val.Value().(map[string]dbus.Variant); ok {
					c.parseMetadata(meta, st)
				}
			}
			if val, ok := changed["CanGoNext"]; ok {
				if b, ok := val.Value().(bool); ok {
					st.CanGoNext = b
				}
			}
			if val, ok := changed["CanGoPrevious"]; ok {
				if b, ok := val.Value().(bool); ok {
					st.CanGoPrevious = b
				}
			}
			if val, ok := changed["CanPlay"]; ok {
				if b, ok := val.Value().(bool); ok {
					st.CanPlay = b
				}
			}
			if val, ok := changed["CanPause"]; ok {
				if b, ok := val.Value().(bool); ok {
					st.CanPause = b
				}
			}
			c.recalculateActiveLocked()
		}
		c.mu.Unlock()
		c.notifyChange()
	}
}

// PlayPause toggles playback on active player.
func (c *Controller) PlayPause() error {
	return c.callActive("PlayPause")
}

// Next skips to next track.
func (c *Controller) Next() error {
	return c.callActive("Next")
}

// Previous skips to previous track.
func (c *Controller) Previous() error {
	return c.callActive("Previous")
}

// Stop stops playback.
func (c *Controller) Stop() error {
	return c.callActive("Stop")
}

func (c *Controller) callActive(method string) error {
	c.mu.RLock()
	busName := c.activeBus
	c.mu.RUnlock()

	if busName == "" {
		return fmt.Errorf("no active mpris player")
	}

	obj := c.conn.Object(busName, "/org/mpris/MediaPlayer2")
	call := obj.Call(mprisInterface+"."+method, 0)
	return call.Err
}

// pruneArtCache removes cached art files older than a week to keep the cache
// directory bounded across sessions.
func pruneArtCache(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-7 * 24 * time.Hour)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}
