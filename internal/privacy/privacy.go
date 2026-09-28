package privacy

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

type Monitor struct {
	mu          sync.RWMutex
	micActive   bool
	camActive   bool
	subscribers map[int]func(mic, cam bool)
	nextSubID   int
	showOSD     func(icon, label string, value float64)

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func NewMonitor(showOSD func(icon, label string, value float64)) *Monitor {
	return &Monitor{
		subscribers: make(map[int]func(mic, cam bool)),
		showOSD:     showOSD,
	}
}

func (m *Monitor) State() (mic, cam bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.micActive, m.camActive
}

func (m *Monitor) Subscribe(fn func(mic, cam bool)) func() {
	m.mu.Lock()
	id := m.nextSubID
	m.nextSubID++
	m.subscribers[id] = fn
	mic, cam := m.micActive, m.camActive
	m.mu.Unlock()

	fn(mic, cam)

	return func() {
		m.mu.Lock()
		delete(m.subscribers, id)
		m.mu.Unlock()
	}
}

func (m *Monitor) notify() {
	m.mu.RLock()
	mic, cam := m.micActive, m.camActive
	subs := make([]func(mic, cam bool), 0, len(m.subscribers))
	for _, fn := range m.subscribers {
		subs = append(subs, fn)
	}
	m.mu.RUnlock()

	for _, fn := range subs {
		fn(mic, cam)
	}
}

func (m *Monitor) setMicActive(active bool) {
	m.mu.Lock()
	if m.micActive == active {
		m.mu.Unlock()
		return
	}
	m.micActive = active
	show := m.showOSD
	m.mu.Unlock()

	if active && show != nil {
		show("audio-input-microphone-symbolic", "Microphone Active", 1.0)
	}
	m.notify()
}

func (m *Monitor) setCamActive(active bool) {
	m.mu.Lock()
	if m.camActive == active {
		m.mu.Unlock()
		return
	}
	m.camActive = active
	show := m.showOSD
	m.mu.Unlock()

	if active && show != nil {
		show("camera-web-symbolic", "Camera Active", 1.0)
	}
	m.notify()
}

func (m *Monitor) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel

	m.wg.Add(2)
	go m.runMicMonitor(ctx)
	go m.runCamMonitor(ctx)
}

func (m *Monitor) Stop() {
	if m.cancel != nil {
		m.cancel()
	}
	m.wg.Wait()
}

func (m *Monitor) runMicMonitor(ctx context.Context) {
	defer m.wg.Done()

	pactlPath, err := exec.LookPath("pactl")
	if err != nil {
		return
	}

	checkMic := func() {
		out, err := exec.CommandContext(ctx, pactlPath, "list", "short", "source-outputs").Output()
		if err == nil {
			active := len(strings.TrimSpace(string(out))) > 0
			m.setMicActive(active)
		}
	}

	checkMic()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		cmd := exec.CommandContext(ctx, pactlPath, "subscribe")
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}

		if err := cmd.Start(); err != nil {
			time.Sleep(2 * time.Second)
			continue
		}

		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.Contains(line, "source-output") {
				checkMic()
			}
		}

		_ = cmd.Wait()
		select {
		case <-ctx.Done():
			return
		case <-time.After(1 * time.Second):
			checkMic()
		}
	}
}

func (m *Monitor) runCamMonitor(ctx context.Context) {
	defer m.wg.Done()

	checkCam := func() {
		active := isCameraInUse()
		m.setCamActive(active)
	}

	checkCam()

	// Setup inotify on /dev/video*
	fd, err := syscall.InotifyInit1(syscall.IN_NONBLOCK | syscall.IN_CLOEXEC)
	if err == nil {
		defer syscall.Close(fd)

		matches, _ := filepath.Glob("/dev/video*")
		for _, dev := range matches {
			_, _ = syscall.InotifyAddWatch(fd, dev, syscall.IN_OPEN|syscall.IN_CLOSE_WRITE|syscall.IN_CLOSE_NOWRITE)
		}

		buf := make([]byte, 1024)
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			n, err := syscall.Read(fd, buf)
			if err == nil && n > 0 {
				checkCam()
			} else {
				time.Sleep(250 * time.Millisecond)
				// Re-check periodically in case devices were plugged
				checkCam()
			}
		}
	} else {
		// Fallback polling if inotify fails
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				checkCam()
			}
		}
	}
}

// isCameraInUse inspects /proc for open file descriptors pointing to /dev/video.
func isCameraInUse() bool {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return false
	}

	selfPid := os.Getpid()
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if len(name) == 0 || name[0] < '0' || name[0] > '9' {
			continue
		}

		// Avoid matching our own process
		var pid int
		for i := 0; i < len(name); i++ {
			pid = pid*10 + int(name[i]-'0')
		}
		if pid == selfPid {
			continue
		}

		fdDir := "/proc/" + name + "/fd"
		fds, err := os.ReadDir(fdDir)
		if err != nil {
			continue
		}

		for _, fd := range fds {
			linkPath := fdDir + "/" + fd.Name()
			buf := make([]byte, 64)
			n, err := syscall.Readlink(linkPath, buf)
			if err == nil && n > 0 {
				target := *(*string)(unsafe.Pointer(&buf))
				if strings.HasPrefix(target[:n], "/dev/video") {
					return true
				}
			}
		}
	}
	return false
}
