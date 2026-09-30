package settings

import (
	"phalune/internal/ipc"
)

type ipcResult struct {
	errMsg    string
	unreached bool
}

func ipcSend(action string) (ipcResult, error) {
	resp, err := ipc.SendCommand("", action, nil)
	if err != nil {
		return ipcResult{unreached: true}, err
	}
	if !resp.OK {
		return ipcResult{errMsg: resp.Error}, nil
	}
	return ipcResult{}, nil
}
