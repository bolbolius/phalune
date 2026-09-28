package config

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

type Watcher struct {
	path          string
	onReload      func(newCfg *Config)
	file          *os.File
	fd            int
	wdDir         int
	wdFile        int
	mu            sync.Mutex
	debounceTimer *time.Timer
	debounceDelay time.Duration
	closed        bool
	done          chan struct{}
}

// StartWatcher initializes an inotify watcher on the given config file path.
// It watches the file's parent directory and the file itself to capture direct
// writes as well as atomic temp-file renames (used by Vim, VS Code, Nano, etc.).
func StartWatcher(configPath string, onReload func(newCfg *Config)) (*Watcher, error) {
	if configPath == "" {
		defaultPath, err := DefaultConfigPath()
		if err != nil {
			return nil, fmt.Errorf("cannot resolve default config path: %w", err)
		}
		configPath = defaultPath
	}

	absPath, err := filepath.Abs(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path for config: %w", err)
	}

	dir := filepath.Dir(absPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to ensure config directory exists: %w", err)
	}

	fd, err := unix.InotifyInit1(unix.IN_NONBLOCK | unix.IN_CLOEXEC)
	if err != nil {
		return nil, fmt.Errorf("inotify_init1 failed: %w", err)
	}

	f := os.NewFile(uintptr(fd), "inotify")

	// Watch the parent directory for close_write, moved_to, create
	wdDir, err := unix.InotifyAddWatch(fd, dir, unix.IN_CLOSE_WRITE|unix.IN_MOVED_TO|unix.IN_CREATE)
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("failed to add inotify watch on directory %s: %w", dir, err)
	}

	// Also watch the file directly if it exists
	wdFile := -1
	if _, statErr := os.Stat(absPath); statErr == nil {
		if wf, wfErr := unix.InotifyAddWatch(fd, absPath, unix.IN_CLOSE_WRITE|unix.IN_MODIFY|unix.IN_DELETE_SELF|unix.IN_MOVE_SELF); wfErr == nil {
			wdFile = wf
		}
	}

	w := &Watcher{
		path:          absPath,
		onReload:      onReload,
		file:          f,
		fd:            fd,
		wdDir:         wdDir,
		wdFile:        wdFile,
		debounceDelay: 100 * time.Millisecond,
		done:          make(chan struct{}),
	}

	go w.readLoop()

	return w, nil
}

func (w *Watcher) SetDebounceDelay(d time.Duration) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.debounceDelay = d
}

func (w *Watcher) Stop() {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return
	}
	w.closed = true
	if w.debounceTimer != nil {
		w.debounceTimer.Stop()
		w.debounceTimer = nil
	}
	w.mu.Unlock()

	_ = w.file.Close()
	<-w.done
}

func (w *Watcher) readLoop() {
	defer close(w.done)

	targetName := filepath.Base(w.path)
	buf := make([]byte, 4096)

	for {
		n, err := w.file.Read(buf)
		if err != nil {
			w.mu.Lock()
			closed := w.closed
			w.mu.Unlock()
			if closed || errors.Is(err, os.ErrClosed) || errors.Is(err, io.EOF) {
				return
			}
			slog.Warn("config watcher read error", "error", err)
			return
		}

		offset := 0
		relevant := false

		for offset <= n-unix.SizeofInotifyEvent {
			wd := int32(binary.LittleEndian.Uint32(buf[offset : offset+4]))
			mask := binary.LittleEndian.Uint32(buf[offset+4 : offset+8])
			nameLen := binary.LittleEndian.Uint32(buf[offset+12 : offset+16])

			var name string
			if nameLen > 0 && offset+unix.SizeofInotifyEvent+int(nameLen) <= n {
				rawName := buf[offset+unix.SizeofInotifyEvent : offset+unix.SizeofInotifyEvent+int(nameLen)]
				name = strings.TrimRight(string(rawName), "\x00")
			}

			offset += unix.SizeofInotifyEvent + int(nameLen)

			w.mu.Lock()
			wdFile := w.wdFile
			w.mu.Unlock()

			if int(wd) == wdFile {
				if mask&(unix.IN_CLOSE_WRITE|unix.IN_MODIFY) != 0 {
					relevant = true
				}
				if mask&(unix.IN_DELETE_SELF|unix.IN_MOVE_SELF|unix.IN_IGNORED) != 0 {
					// Inode replaced (atomic save). Re-attach file watch if possible.
					w.mu.Lock()
					w.wdFile = -1
					if wf, err := unix.InotifyAddWatch(w.fd, w.path, unix.IN_CLOSE_WRITE|unix.IN_MODIFY|unix.IN_DELETE_SELF|unix.IN_MOVE_SELF); err == nil {
						w.wdFile = wf
					}
					w.mu.Unlock()
					relevant = true
				}
			} else if int(wd) == w.wdDir {
				if name == targetName && (mask&(unix.IN_CLOSE_WRITE|unix.IN_MOVED_TO|unix.IN_CREATE)) != 0 {
					relevant = true
					// Re-attach watch to newly created or moved-to file
					w.mu.Lock()
					if wf, err := unix.InotifyAddWatch(w.fd, w.path, unix.IN_CLOSE_WRITE|unix.IN_MODIFY|unix.IN_DELETE_SELF|unix.IN_MOVE_SELF); err == nil {
						w.wdFile = wf
					}
					w.mu.Unlock()
				}
			}
		}

		if relevant {
			w.scheduleReload()
		}
	}
}

func (w *Watcher) scheduleReload() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return
	}

	if w.debounceTimer != nil {
		w.debounceTimer.Stop()
	}

	w.debounceTimer = time.AfterFunc(w.debounceDelay, func() {
		w.triggerReload()
	})
}

func (w *Watcher) triggerReload() {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return
	}
	w.mu.Unlock()

	newCfg, err := Load(w.path)
	if err != nil {
		slog.Warn("config: failed to reload config file, keeping active config", "path", w.path, "error", err)
		return
	}

	slog.Info("config: file changed, triggering hot reload", "path", w.path)
	if w.onReload != nil {
		w.onReload(newCfg)
	}
}
