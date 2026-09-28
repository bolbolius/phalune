package privacy

import (
	"testing"
)

func TestPrivacyMonitorState(t *testing.T) {
	osdCalled := false
	m := NewMonitor(func(icon, label string, val float64) {
		osdCalled = true
	})

	mic, cam := m.State()
	if mic || cam {
		t.Fatalf("expected initial state false, false, got %v, %v", mic, cam)
	}

	var observedMic, observedCam bool
	unsub := m.Subscribe(func(mic, cam bool) {
		observedMic = mic
		observedCam = cam
	})
	defer unsub()

	m.setMicActive(true)
	if !observedMic {
		t.Fatalf("expected observedMic to be true")
	}
	if !osdCalled {
		t.Fatalf("expected OSD to be called on mic activation")
	}

	m.setCamActive(true)
	if !observedCam {
		t.Fatalf("expected observedCam to be true")
	}

	m.setMicActive(false)
	if observedMic {
		t.Fatalf("expected observedMic to be false")
	}
}
