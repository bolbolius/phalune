package launcher

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	halfLifeDays      = 7.0
	frecencyThreshold = 1.0
	frecencyMaxBoost  = 25.0
)

type FrecencyEntry struct {
	Count    int   `json:"count"`
	LastUsed int64 `json:"lastUsed"`
}

type FrecencyStore struct {
	mu           sync.RWMutex
	path         string
	data         map[string]FrecencyEntry
	halfLifeDays float64
	maxBoost     float64
}

func NewFrecencyStore() *FrecencyStore {
	return NewFrecencyStoreWithParams(halfLifeDays, frecencyMaxBoost)
}

func NewFrecencyStoreWithParams(halfLife, maxBoost float64) *FrecencyStore {
	if halfLife <= 0 {
		halfLife = halfLifeDays
	}
	if maxBoost <= 0 {
		maxBoost = frecencyMaxBoost
	}
	store := &FrecencyStore{
		path:         defaultFrecencyPath(),
		data:         make(map[string]FrecencyEntry),
		halfLifeDays: halfLife,
		maxBoost:     maxBoost,
	}
	store.load()
	return store
}

func (s *FrecencyStore) MaxBoost() float64 {
	if s == nil || s.maxBoost <= 0 {
		return frecencyMaxBoost
	}
	return s.maxBoost
}

func (s *FrecencyStore) SetParams(halfLife, maxBoost float64) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if halfLife > 0 {
		s.halfLifeDays = halfLife
	}
	if maxBoost > 0 {
		s.maxBoost = maxBoost
	}
}

func defaultFrecencyPath() string {
	if dataHome := os.Getenv("XDG_DATA_HOME"); dataHome != "" {
		return filepath.Join(dataHome, "phalune", "frecency.json")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "phalune-frecency.json")
	}
	return filepath.Join(home, ".local", "share", "phalune", "frecency.json")
}

func (s *FrecencyStore) load() {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path)
	if err != nil {
		return
	}

	_ = json.Unmarshal(data, &s.data)
}

func (s *FrecencyStore) save() {
	s.mu.RLock()
	data, err := json.MarshalIndent(s.data, "", "  ")
	s.mu.RUnlock()

	if err != nil {
		return
	}

	_ = os.MkdirAll(filepath.Dir(s.path), 0755)
	_ = os.WriteFile(s.path, data, 0644)
}

func (s *FrecencyStore) Score(appID string) float64 {
	s.mu.RLock()
	entry, ok := s.data[appID]
	s.mu.RUnlock()

	if !ok || entry.Count <= 0 {
		return 0
	}

	now := time.Now().UnixMilli()
	ageMs := float64(now - entry.LastUsed)
	ageDays := ageMs / (1000.0 * 60.0 * 60.0 * 24.0)
	if ageDays < 0 {
		ageDays = 0
	}

	halfLife := s.halfLifeDays
	if halfLife <= 0 {
		halfLife = halfLifeDays
	}
	lambda := 0.6931 / halfLife
	return float64(entry.Count) * math.Exp(-lambda*ageDays)
}

func (s *FrecencyStore) RecordLaunch(appID string) {
	if appID == "" {
		return
	}

	s.mu.Lock()
	entry := s.data[appID]
	entry.Count++
	entry.LastUsed = time.Now().UnixMilli()
	s.data[appID] = entry
	s.mu.Unlock()

	go s.save()
}
