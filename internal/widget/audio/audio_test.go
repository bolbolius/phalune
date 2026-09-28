package audio

import (
	"testing"
)

func TestVolumeIconName(t *testing.T) {
	tests := []struct {
		pct      int
		muted    bool
		expected string
	}{
		{50, true, "audio-volume-muted-symbolic"},
		{0, false, "audio-volume-muted-symbolic"},
		{15, false, "audio-volume-low-symbolic"},
		{32, false, "audio-volume-low-symbolic"},
		{33, false, "audio-volume-medium-symbolic"},
		{65, false, "audio-volume-medium-symbolic"},
		{66, false, "audio-volume-high-symbolic"},
		{100, false, "audio-volume-high-symbolic"},
	}

	for _, tt := range tests {
		got := VolumeIconName(tt.pct, tt.muted)
		if got != tt.expected {
			t.Errorf("VolumeIconName(%d, %v) = %q, want %q", tt.pct, tt.muted, got, tt.expected)
		}
	}
}

func TestParseWpctlVolume(t *testing.T) {
	tests := []struct {
		input     string
		wantVol   float64
		wantMuted bool
		wantErr   bool
	}{
		{"Volume: 0.55\n", 55.0, false, false},
		{"Volume: 0.00 [MUTED]\n", 0.0, true, false},
		{"Volume: 1.00\n", 100.0, false, false},
		{"Invalid\n", 0.0, false, true},
	}

	for _, tt := range tests {
		vol, muted, err := parseWpctlVolume(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("parseWpctlVolume(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			continue
		}
		if !tt.wantErr {
			if vol != tt.wantVol || muted != tt.wantMuted {
				t.Errorf("parseWpctlVolume(%q) = (%v, %v), want (%v, %v)", tt.input, vol, muted, tt.wantVol, tt.wantMuted)
			}
		}
	}
}

func TestParsePactlVolume(t *testing.T) {
	tests := []struct {
		input   string
		wantVol float64
		wantErr bool
	}{
		{"Volume: front-left: 32768 /  50% / -18.06 dB", 50.0, false},
		{"Volume: front-left: 65536 / 100% / 0.00 dB", 100.0, false},
		{"No percent here", 0.0, true},
	}

	for _, tt := range tests {
		vol, err := parsePactlVolume(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("parsePactlVolume(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			continue
		}
		if !tt.wantErr && vol != tt.wantVol {
			t.Errorf("parsePactlVolume(%q) = %v, want %v", tt.input, vol, tt.wantVol)
		}
	}
}
