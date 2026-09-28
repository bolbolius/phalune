package lockscreen

import (
	"testing"

	"phalune/internal/config"
)

type dummySessionActions struct {
	suspended  bool
	rebooted   bool
	poweredOff bool
	locked     bool
}

func (d *dummySessionActions) Suspend() error {
	d.suspended = true
	return nil
}
func (d *dummySessionActions) Reboot() error {
	d.rebooted = true
	return nil
}
func (d *dummySessionActions) PowerOff() error {
	d.poweredOff = true
	return nil
}
func (d *dummySessionActions) SetLocked(l bool) {
	d.locked = l
}

func TestLockScreenManagerInit(t *testing.T) {
	sess := &dummySessionActions{}
	cfg := config.LockScreenConfig{
		TimeFormat: "15:04",
		DateFormat: "2006-01-02",
	}

	mgr := New(nil, cfg, sess)
	mgr.SetAuthenticator(&MockAuthenticator{ValidPassword: "secret"})

	if mgr.IsLocked() {
		t.Errorf("expected initial state not locked")
	}

	mgr.UpdateConfig(config.LockScreenConfig{
		TimeFormat: "03:04 PM",
	})
	if mgr.cfg.TimeFormat != "03:04 PM" {
		t.Errorf("expected updated time format '03:04 PM', got %q", mgr.cfg.TimeFormat)
	}
}
