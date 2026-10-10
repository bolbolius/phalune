package sound

import (
	"bytes"
	"encoding/binary"
	"math"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"
)

// Sound cue types
const (
	CueTick    = "tick"
	CueShutter = "shutter"
	CueToggle  = "toggle"
)

var (
	enabled   atomic.Bool
	initOnce  sync.Once
	cueData   map[string][]byte
	lastPlay  map[string]time.Time
	playMu    sync.Mutex
	pwPlayBin string
	paplayBin string
)

// Init initializes the sound feedback system with the master toggle.
func Init(isEnabled bool) {
	enabled.Store(isEnabled)
	initOnce.Do(func() {
		pwPlayBin, _ = exec.LookPath("pw-play")
		paplayBin, _ = exec.LookPath("paplay")

		cueData = make(map[string][]byte)
		lastPlay = make(map[string]time.Time)

		// Pre-render minimal PCM WAV buffers
		cueData[CueTick] = generateWav(0.025, 950, 0.18, true)
		cueData[CueShutter] = generateShutterWav()
		cueData[CueToggle] = generateWav(0.035, 750, 0.22, true)
	})
}

// SetEnabled updates the master enable state at runtime.
func SetEnabled(isEnabled bool) {
	enabled.Store(isEnabled)
}

// Play triggers an asynchronous audio cue if sound is enabled.
func Play(cue string) {
	if !enabled.Load() {
		return
	}
	if pwPlayBin == "" && paplayBin == "" {
		return
	}

	playMu.Lock()
	now := time.Now()
	if last, ok := lastPlay[cue]; ok {
		minInterval := 40 * time.Millisecond
		if cue == CueTick {
			minInterval = 50 * time.Millisecond
		}
		if now.Sub(last) < minInterval {
			playMu.Unlock()
			return
		}
	}
	lastPlay[cue] = now
	wavBytes, ok := cueData[cue]
	playMu.Unlock()

	if !ok || len(wavBytes) == 0 {
		return
	}

	go func(data []byte) {
		bin := pwPlayBin
		args := []string{"-"}
		if bin == "" {
			bin = paplayBin
			args = []string{"/dev/stdin"}
		}
		cmd := exec.Command(bin, args...)
		cmd.Stdin = bytes.NewReader(data)
		_ = cmd.Run()
	}(wavBytes)
}

func generateWav(durationSec float64, freq float64, volume float64, decay bool) []byte {
	sampleRate := 44100
	nSamples := int(float64(sampleRate) * durationSec)
	pcm := make([]byte, nSamples*2)

	for i := 0; i < nSamples; i++ {
		t := float64(i) / float64(sampleRate)
		env := 1.0
		if decay {
			env = math.Exp(-t / (durationSec * 0.35))
		}
		sampleVal := math.Sin(2.0*math.Pi*freq*t) * env * volume
		valInt := int16(sampleVal * 32767.0)
		binary.LittleEndian.PutUint16(pcm[i*2:], uint16(valInt))
	}

	return createWavContainer(sampleRate, pcm)
}

func generateShutterWav() []byte {
	sampleRate := 44100
	duration := 0.08
	nSamples := int(float64(sampleRate) * duration)
	pcm := make([]byte, nSamples*2)

	for i := 0; i < nSamples; i++ {
		t := float64(i) / float64(sampleRate)
		env := math.Exp(-t / (duration * 0.3))
		f := 1200.0 - (t/duration)*700.0
		sampleVal := math.Sin(2.0*math.Pi*f*t) * env * 0.25
		valInt := int16(sampleVal * 32767.0)
		binary.LittleEndian.PutUint16(pcm[i*2:], uint16(valInt))
	}

	return createWavContainer(sampleRate, pcm)
}

func createWavContainer(sampleRate int, pcm []byte) []byte {
	buf := new(bytes.Buffer)
	buf.WriteString("RIFF")
	binary.Write(buf, binary.LittleEndian, uint32(36+len(pcm)))
	buf.WriteString("WAVEfmt ")
	binary.Write(buf, binary.LittleEndian, uint32(16))     // Subchunk1Size (16 for PCM)
	binary.Write(buf, binary.LittleEndian, uint16(1))      // AudioFormat (1 = PCM)
	binary.Write(buf, binary.LittleEndian, uint16(1))      // NumChannels (1 = Mono)
	binary.Write(buf, binary.LittleEndian, uint32(sampleRate))
	binary.Write(buf, binary.LittleEndian, uint32(sampleRate*2)) // ByteRate
	binary.Write(buf, binary.LittleEndian, uint16(2))      // BlockAlign
	binary.Write(buf, binary.LittleEndian, uint16(16))     // BitsPerSample
	buf.WriteString("data")
	binary.Write(buf, binary.LittleEndian, uint32(len(pcm)))
	buf.Write(pcm)
	return buf.Bytes()
}
