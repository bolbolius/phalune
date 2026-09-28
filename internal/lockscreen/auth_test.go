package lockscreen

import (
	"testing"
)

func TestMockAuthenticator(t *testing.T) {
	auth := &MockAuthenticator{ValidPassword: "correcthorse"}

	ok, err := auth.Authenticate("user", "correcthorse")
	if err != nil || !ok {
		t.Errorf("expected success for correct password, got ok=%v, err=%v", ok, err)
	}

	ok, err = auth.Authenticate("user", "wrongpassword")
	if err != nil || ok {
		t.Errorf("expected failure for wrong password, got ok=%v, err=%v", ok, err)
	}

	allowAll := &MockAuthenticator{AlwaysAllow: true}
	ok, err = allowAll.Authenticate("user", "anything")
	if err != nil || !ok {
		t.Errorf("expected allowAll to succeed, got ok=%v, err=%v", ok, err)
	}
}

func TestUnixAuthenticatorCustomCommand(t *testing.T) {
	// A custom command that succeeds if input equals "mypass"
	auth := &UnixAuthenticator{
		CustomCommand: `read -r line; [ "$line" = "mypass" ]`,
	}

	ok, err := auth.Authenticate("testuser", "mypass")
	if err != nil || !ok {
		t.Errorf("expected custom command auth success, got ok=%v, err=%v", ok, err)
	}

	ok, err = auth.Authenticate("testuser", "wrongpass")
	if err != nil || ok {
		t.Errorf("expected custom command auth failure, got ok=%v, err=%v", ok, err)
	}
}
