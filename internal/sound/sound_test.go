package sound

import (
	"testing"
)

func TestSoundInitAndPlay(t *testing.T) {
	Init(true)
	if !enabled.Load() {
		t.Fatal("expected sound to be enabled")
	}

	SetEnabled(false)
	if enabled.Load() {
		t.Fatal("expected sound to be disabled")
	}

	SetEnabled(true)
	// Testing cues render properly
	for _, cue := range []string{CueTick, CueShutter, CueToggle} {
		data, ok := cueData[cue]
		if !ok || len(data) == 0 {
			t.Errorf("missing audio buffer for cue %q", cue)
		}
	}
}
