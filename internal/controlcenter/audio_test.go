package controlcenter

import (
	"testing"
)

func TestParseSinkInputs(t *testing.T) {
	raw := `
Sink Input #729
	Driver: PipeWire
	Owner Module: n/a
	Client: 71
	Sink: 59
	Corked: no
	Mute: no
	Volume: front-left: 65536 / 100% / 0.00 dB,   front-right: 65536 / 100% / 0.00 dB
	        balance 0.00
	Properties:
		client.api = "pipewire-pulse"
		application.name = "Firefox"
		application.process.binary = "firefox"
		media.name = "Transparent Proxy Mechanics"

Sink Input #738
	Driver: PipeWire
	Corked: no
	Mute: yes
	Volume: front-left: 32768 / 50% / -6.00 dB,   front-right: 32768 / 50% / -6.00 dB
	Properties:
		node.name = "Telegram"
		application.process.binary = "telegram-desktop"
		media.name = "Playback Stream"
`
	streams := ParseSinkInputs(raw)
	if len(streams) != 2 {
		t.Fatalf("expected 2 streams, got %d", len(streams))
	}

	if streams[0].ID != 729 || streams[0].AppName != "Firefox" || streams[0].Volume != 100 || streams[0].Muted {
		t.Errorf("stream 0 mismatch: %+v", streams[0])
	}
	if streams[0].MediaName != "Transparent Proxy Mechanics" {
		t.Errorf("stream 0 media name mismatch: got %q", streams[0].MediaName)
	}

	if streams[1].ID != 738 || streams[1].AppName != "Telegram" || streams[1].Volume != 50 || !streams[1].Muted {
		t.Errorf("stream 1 mismatch: %+v", streams[1])
	}
}

func TestInputVolumeIconName(t *testing.T) {
	if got := InputVolumeIconName(50, true); got != "microphone-disabled-symbolic" {
		t.Errorf("expected muted mic icon, got %s", got)
	}
	if got := InputVolumeIconName(0, false); got != "microphone-disabled-symbolic" {
		t.Errorf("expected 0%% mic icon to be disabled, got %s", got)
	}
	if got := InputVolumeIconName(50, false); got != "audio-input-microphone-symbolic" {
		t.Errorf("expected active mic icon, got %s", got)
	}
}
