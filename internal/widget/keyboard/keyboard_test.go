package keyboard

import (
	"testing"
)

func TestFormatLayoutName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"English (US)", "US"},
		{"English (US, intl-altgr-dead-keys)", "US"},
		{"Persian (IR)", "FA"},
		{"German (DE)", "DE"},
		{"French (AZERTY)", "FR"},
		{"Russian", "RU"},
		{"Persian", "FA"},
		{"Farsi", "FA"},
		{"English", "US"},
		{"us", "US"},
		{"ir", "FA"},
		{"ru", "RU"},
		{"de", "DE"},
		{"Spanish", "ES"},
		{"Arabic", "AR"},
		{"Turkish", "TR"},
		{"Chinese", "ZH"},
		{"Japanese", "JA"},
		{"Korean", "KO"},
		{"Dvorak", "DV"},
		{"", ""},
	}

	for _, tc := range tests {
		got := FormatLayoutName(tc.input)
		if got != tc.expected {
			t.Errorf("FormatLayoutName(%q) = %q, expected %q", tc.input, got, tc.expected)
		}
	}
}
