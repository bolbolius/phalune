package osd

import (
	"testing"
)

func TestParseWpctlVolume(t *testing.T) {
	tests := []struct {
		input       string
		expectedVol float64
		expectedMut bool
		expectErr   bool
	}{
		{"Volume: 0.30\n", 30.0, false, false},
		{"Volume: 0.30 [MUTED]\n", 30.0, true, false},
		{"Volume: 1.00\n", 100.0, false, false},
		{"Volume: 0.00\n", 0.0, false, false},
		{"Volume: 0.05 [MUTED]\n", 5.0, true, false},
		{"Invalid output\n", 0, false, true},
	}

	for _, tt := range tests {
		vol, mut, err := parseWpctlVolume(tt.input)
		if tt.expectErr {
			if err == nil {
				t.Errorf("expected error for input %q, got nil", tt.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("unexpected error for input %q: %v", tt.input, err)
			continue
		}
		if vol != tt.expectedVol {
			t.Errorf("for %q, expected volume %.1f, got %.1f", tt.input, tt.expectedVol, vol)
		}
		if mut != tt.expectedMut {
			t.Errorf("for %q, expected muted %v, got %v", tt.input, tt.expectedMut, mut)
		}
	}
}

func TestParsePactlVolume(t *testing.T) {
	tests := []struct {
		input       string
		expectedVol float64
		expectErr   bool
	}{
		{"Volume: front-left: 19660 /  30% / -31.37 dB,   front-right: 19660 /  30% / -31.37 dB\n", 30.0, false},
		{"Volume: front-left: 65536 / 100% / 0.00 dB,   front-right: 65536 / 100% / 0.00 dB\n", 100.0, false},
		{"Volume: front-left: 0 / 0% / -inf dB\n", 0.0, false},
		{"No percent here\n", 0, true},
	}

	for _, tt := range tests {
		vol, err := parsePactlVolume(tt.input)
		if tt.expectErr {
			if err == nil {
				t.Errorf("expected error for %q, got nil", tt.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("unexpected error for %q: %v", tt.input, err)
			continue
		}
		if vol != tt.expectedVol {
			t.Errorf("for %q, expected %.1f, got %.1f", tt.input, tt.expectedVol, vol)
		}
	}
}

func TestParsePactlMute(t *testing.T) {
	if !parsePactlMute("Mute: yes\n") {
		t.Errorf("expected true for 'Mute: yes'")
	}
	if parsePactlMute("Mute: no\n") {
		t.Errorf("expected false for 'Mute: no'")
	}
	if !parsePactlMute("mute: YES\n") {
		t.Errorf("expected true for 'mute: YES'")
	}
}

func TestParsePactlSubscribeFacility(t *testing.T) {
	tests := []struct {
		line     string
		expected string
	}{
		{"Event 'change' on sink #59", "sink"},
		{"Event 'change' on source #60", "source"},
		{"Event 'change' on server #1", "server"},
		{"Event 'change' on sink-input #12", "sink-input"},
		{"Event 'new' on client #1918", "client"},
		{"Event 'remove' on client #1918", "client"},
		{"Random garbage text", ""},
		{"on", ""},
	}

	for _, tt := range tests {
		got := parsePactlSubscribeFacility(tt.line)
		if got != tt.expected {
			t.Errorf("for %q, expected facility %q, got %q", tt.line, tt.expected, got)
		}
	}
}
