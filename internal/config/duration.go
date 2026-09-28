package config

import (
	"fmt"
	"time"
)

// Duration wraps time.Duration to support TOML string values like "30s", "2s", "1m".
type Duration struct {
	time.Duration
}

func (d Duration) MarshalText() ([]byte, error) {
	return []byte(d.Duration.String()), nil
}

func (d *Duration) UnmarshalText(b []byte) error {
	parsed, err := time.ParseDuration(string(b))
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", string(b), err)
	}
	d.Duration = parsed
	return nil
}
