package lockscreen

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
)

type Authenticator interface {
	Authenticate(username, password string) (bool, error)
}

type UnixAuthenticator struct {
	CustomCommand string
	chkpwdPath    string
}

func NewUnixAuthenticator(customCommand string) *UnixAuthenticator {
	path, _ := exec.LookPath("unix_chkpwd")
	if path == "" {
		if _, err := os.Stat("/usr/sbin/unix_chkpwd"); err == nil {
			path = "/usr/sbin/unix_chkpwd"
		}
	}
	return &UnixAuthenticator{
		CustomCommand: customCommand,
		chkpwdPath:    path,
	}
}

func (a *UnixAuthenticator) Authenticate(username, password string) (bool, error) {
	if a.CustomCommand != "" {
		cmd := exec.Command("sh", "-c", a.CustomCommand)
		cmd.Env = append(os.Environ(), "USER="+username)
		cmd.Stdin = bytes.NewBufferString(password + "\n")
		err := cmd.Run()
		return err == nil, nil
	}

	if a.chkpwdPath != "" {
		cmd := exec.Command(a.chkpwdPath, username, "nullok")
		// unix_chkpwd reads password terminated by a null byte or newline from stdin
		cmd.Stdin = bytes.NewBuffer(append([]byte(password), 0))
		err := cmd.Run()
		if err == nil {
			return true, nil
		}
		// If command failed with an exit error, password was wrong
		if _, ok := err.(*exec.ExitError); ok {
			return false, nil
		}
		return false, fmt.Errorf("auth helper failed: %w", err)
	}

	return false, fmt.Errorf("no authentication helper found (unix_chkpwd missing)")
}

type MockAuthenticator struct {
	ValidPassword string
	AlwaysAllow   bool
}

func (m *MockAuthenticator) Authenticate(username, password string) (bool, error) {
	if m.AlwaysAllow {
		return true, nil
	}
	if password == m.ValidPassword {
		return true, nil
	}
	return false, nil
}
