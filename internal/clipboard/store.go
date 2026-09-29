package clipboard

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// EntryKind classifies a clipboard entry payload.
type EntryKind string

const (
	KindText  EntryKind = "text"
	KindImage EntryKind = "image"
	KindFiles EntryKind = "files"
)

// Entry is one clipboard history item.
//
// Text entries store Text directly. Image entries store the PNG payload in a
// per-entry file inside the cache dir with the path recorded in FilePath.
// File entries store the URI list in Text (one URI per line).
type Entry struct {
	ID        string    `json:"id"`
	Kind      EntryKind `json:"kind"`
	Text      string    `json:"text,omitempty"`
	FilePath  string    `json:"file_path,omitempty"` // image payload (png)
	Width     int       `json:"width,omitempty"`     // image dimensions
	Height    int       `json:"height,omitempty"`
	CreatedAt int64     `json:"created_at"`
	UseCount  int       `json:"use_count"`
	Preview   string    `json:"preview,omitempty"` // short display text
}

// Size returns the approximate payload size of the entry.
func (e *Entry) Size() int {
	switch e.Kind {
	case KindImage:
		data, err := os.ReadFile(e.FilePath)
		if err != nil {
			return 0
		}
		return len(data)
	case KindFiles:
		return len(e.Text)
	default:
		return len(e.Text)
	}
}

// Store is a bounded, deduplicating clipboard history with optional JSON
// persistence. Safe for concurrent use.
type Store struct {
	mu        sync.Mutex
	max       int
	persist   bool
	path      string
	imgDir    string
	entries   []*Entry // newest first
	lastAdded map[string]int64
}

// DefaultHistoryPath returns the history file location.
func DefaultHistoryPath() string {
	return filepath.Join(dataDir(), "history.json")
}

// DefaultImagesDir returns the directory used to persist image payloads.
func DefaultImagesDir() string {
	return filepath.Join(dataDir(), "images")
}

func dataDir() string {
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "phalune", "clipboard")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "phalune-clipboard")
	}
	return filepath.Join(home, ".local", "share", "phalune", "clipboard")
}

// NewStore creates a store keeping at most max entries. When persist is true
// the history is loaded from disk and saved on mutation.
func NewStore(max int, persist bool) *Store {
	if max <= 0 {
		max = 30
	}
	s := &Store{
		max:       max,
		persist:   persist,
		path:      DefaultHistoryPath(),
		imgDir:    DefaultImagesDir(),
		lastAdded: make(map[string]int64),
	}
	if persist {
		s.load()
	}
	return s
}

// SetPaths overrides persistence locations (used by tests).
func (s *Store) SetPaths(historyFile, imagesDir string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.path = historyFile
	s.imgDir = imagesDir
	if s.persist {
		s.load()
	}
}

// MaxEntries returns the configured capacity.
func (s *Store) MaxEntries() int {
	return s.max
}

// Resize trims the history to a new capacity.
func (s *Store) Resize(max int) {
	if max <= 0 {
		return
	}
	s.mu.Lock()
	if s.max == max {
		s.mu.Unlock()
		return
	}
	s.max = max
	if len(s.entries) > max {
		dropped := s.entries[max:]
		for _, d := range dropped {
			s.removeAssetsLocked(d)
		}
		s.entries = s.entries[:max]
		s.saveLocked()
	}
	s.mu.Unlock()
}

// Entries returns a copy of the history, newest first.
func (s *Store) Entries() []*Entry {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]*Entry, len(s.entries))
	copy(out, s.entries)
	return out
}

// Add inserts an entry at the top. A new copy of a recent duplicate moves the
// original to the top instead of creating a second entry.
func (s *Store) Add(e *Entry) {
	if e == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UnixMilli()
	e.ID = newID(now)
	if e.CreatedAt == 0 {
		e.CreatedAt = now
	}
	if e.Preview == "" {
		e.Preview = previewOf(e)
	}

	// Deduplicate: text and files compare by content; images by dimensions +
	// size hash fallback keeps duplicates out of spammy screenshot flows.
	dedupKey := e.Text
	if e.Kind == KindImage {
		dedupKey = fmt.Sprintf("img:%dx%d", e.Width, e.Height)
	}
	if last := s.lastAdded[dedupKey]; now-last < 400 {
		// Rapid re-copy of same content (common with terminals); ignore.
		return
	}
	s.lastAdded[dedupKey] = now

	for i, old := range s.entries {
		if sameContent(old, e) {
			// Move to top, refresh timestamp.
			*old = *e
			old.CreatedAt = now
			s.entries = append(s.entries[:i], s.entries[i+1:]...)
			s.entries = append([]*Entry{old}, s.entries...)
			s.saveLocked()
			return
		}
	}

	s.entries = append([]*Entry{e}, s.entries...)
	if len(s.entries) > s.max {
		dropped := s.entries[s.max:]
		for _, d := range dropped {
			s.removeAssetsLocked(d)
		}
		s.entries = s.entries[:s.max]
	}
	s.saveLocked()
}

// Touch bumps the use count of an entry by ID.
func (s *Store) Touch(id string) {
	s.mu.Lock()
	for _, e := range s.entries {
		if e.ID == id {
			e.UseCount++
			break
		}
	}
	s.mu.Unlock()
}

// Remove deletes an entry by ID.
func (s *Store) Remove(id string) {
	s.mu.Lock()
	for i, e := range s.entries {
		if e.ID == id {
			s.removeAssetsLocked(e)
			s.entries = append(s.entries[:i], s.entries[i+1:]...)
			break
		}
	}
	s.mu.Unlock()
	s.saveLocked()
}

// Clear drops all entries, including persisted image payloads.
func (s *Store) Clear() {
	s.mu.Lock()
	for _, e := range s.entries {
		s.removeAssetsLocked(e)
	}
	s.entries = nil
	s.mu.Unlock()
	s.saveLocked()
}

func sameContent(a, b *Entry) bool {
	if a.Kind != b.Kind {
		return false
	}
	switch a.Kind {
	case KindImage:
		return a.Width == b.Width && a.Height == b.Height && filepath.Base(a.FilePath) == filepath.Base(b.FilePath)
	default:
		return a.Text == b.Text
	}
}

func previewOf(e *Entry) string {
	switch e.Kind {
	case KindFiles:
		uris := splitURIs(e.Text)
		if len(uris) == 0 {
			return ""
		}
		names := make([]string, 0, 2)
		for i, u := range uris {
			if i == 2 {
				break
			}
			names = append(names, filepath.Base(fileURIToPath(u)))
		}
		if len(uris) > 2 {
			names = append(names, fmt.Sprintf("+%d more", len(uris)-2))
		}
		return joinNonEmpty(names, ", ")
	default:
		return firstLine(e.Text, 80)
	}
}

// saveLocked persists the history. Callers must hold s.mu.
func (s *Store) saveLocked() {
	if !s.persist {
		return
	}
	data, err := json.MarshalIndent(s.entries, "", "  ")
	if err != nil {
		slog.Debug("clipboard: marshal history failed", "error", err)
		return
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		slog.Debug("clipboard: history dir create failed", "error", err)
		return
	}
	if err := os.WriteFile(s.path, data, 0600); err != nil {
		slog.Debug("clipboard: history write failed", "error", err)
	}
}

// load reads the persisted history, dropping entries whose image payload is
// missing on disk.
func (s *Store) load() {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return
	}
	var entries []*Entry
	if err := json.Unmarshal(data, &entries); err != nil {
		slog.Warn("clipboard: failed to parse history file", "path", s.path, "error", err)
		return
	}

	kept := entries[:0]
	for _, e := range entries {
		if e == nil {
			continue
		}
		if e.Kind == KindImage && e.FilePath != "" {
			if _, err := os.Stat(e.FilePath); err != nil {
				continue
			}
		}
		kept = append(kept, e)
	}

	sort.SliceStable(kept, func(i, j int) bool {
		return kept[i].CreatedAt > kept[j].CreatedAt
	})
	if len(kept) > s.max {
		dropped := kept[s.max:]
		for _, d := range dropped {
			s.removeAssetsLocked(d)
		}
		kept = kept[:s.max]
	}

	s.entries = kept
}

// removeAssetsLocked deletes the image payload of an entry if present.
// Callers must hold s.mu.
func (s *Store) removeAssetsLocked(e *Entry) {
	if e.Kind == KindImage && e.FilePath != "" {
		_ = os.Remove(e.FilePath)
	}
}

// WriteImage writes a PNG payload into the images dir and returns its path.
// Callers must hold s.mu when invoked during Add; the standalone variant is
// CreateImageFile.
func (s *Store) CreateImageFile(data []byte) (string, error) {
	if err := os.MkdirAll(s.imgDir, 0700); err != nil {
		return "", fmt.Errorf("images dir: %w", err)
	}
	name := fmt.Sprintf("img-%d.png", time.Now().UnixNano())
	dest := filepath.Join(s.imgDir, name)
	if err := os.WriteFile(dest, data, 0600); err != nil {
		return "", fmt.Errorf("write image: %w", err)
	}
	return dest, nil
}

func firstLine(s string, limit int) string {
	for i, r := range s {
		if r == '\n' {
			s = s[:i]
			break
		}
		_ = r
	}
	if len(s) > limit {
		return s[:limit] + "…"
	}
	return s
}

func joinNonEmpty(parts []string, sep string) string {
	out := ""
	for _, p := range parts {
		if p == "" {
			continue
		}
		if out != "" {
			out += sep
		}
		out += p
	}
	return out
}