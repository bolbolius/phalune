package polkit

/*
#cgo pkg-config: polkit-agent-1 polkit-gobject-1
#define POLKIT_AGENT_I_KNOW_API_IS_SUBJECT_TO_CHANGE 1
#include "listener.h"
#include <stdlib.h>
#include <string.h>
*/
import "C"

import (
	"fmt"
	"log/slog"
	"sync"
	"unsafe"

	"github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

var (
	agentMu      sync.Mutex
	currentAgent *Agent
)

type Agent struct {
	app        *gtk.Application
	listener   *C.PolkitAgentListener
	handle     C.gpointer
	dialog     *AuthDialog
	activeTask uintptr
	cookie     string
	mu         sync.Mutex
	started    bool
}

func New(app *gtk.Application) (*Agent, error) {
	dialog, err := NewAuthDialog(app)
	if err != nil {
		return nil, fmt.Errorf("failed to create polkit dialog: %w", err)
	}

	a := &Agent{
		app:    app,
		dialog: dialog,
	}

	dialog.onResponse = func(taskId uintptr, password string) {
		a.handleResponse(taskId, password)
	}
	dialog.onCancel = func(taskId uintptr) {
		a.handleCancel(taskId)
	}
	dialog.onSwitchUser = func(taskId uintptr, identityIdx int) {
		a.handleSwitchUser(taskId, identityIdx)
	}

	return a, nil
}

func (a *Agent) Start() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.started {
		return nil
	}

	listenerObj := C.phalune_agent_listener_new()
	if listenerObj == nil {
		return fmt.Errorf("failed to create polkit agent listener")
	}
	a.listener = (*C.PolkitAgentListener)(C.gpointer(listenerObj))

	var gerr *C.GError
	handle := C.phalune_register_listener(a.listener, &gerr)
	if handle == nil {
		errMsg := "unknown error"
		if gerr != nil && gerr.message != nil {
			errMsg = C.GoString(gerr.message)
			C.g_error_free(gerr)
		}
		C.g_object_unref(C.gpointer(a.listener))
		a.listener = nil
		return fmt.Errorf("polkit registration failed: %s", errMsg)
	}

	a.handle = handle
	a.started = true

	agentMu.Lock()
	currentAgent = a
	agentMu.Unlock()

	slog.Info("polkit: authentication agent registered with authority")
	return nil
}

func (a *Agent) Stop() {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.started {
		return
	}
	a.started = false

	agentMu.Lock()
	if currentAgent == a {
		currentAgent = nil
	}
	agentMu.Unlock()

	if a.activeTask != 0 {
		C.phalune_agent_session_cancel(C.uintptr_t(a.activeTask))
		C.phalune_agent_task_complete(C.uintptr_t(a.activeTask), 0, nil)
		a.activeTask = 0
	}

	if a.handle != nil {
		C.phalune_unregister_listener(a.handle)
		a.handle = nil
	}

	if a.listener != nil {
		C.g_object_unref(C.gpointer(a.listener))
		a.listener = nil
	}

	dialog := a.dialog
	a.dialog = nil
	if dialog != nil {
		glib.IdleAdd(func() {
			dialog.Destroy()
		})
	}

	slog.Info("polkit: authentication agent unregistered")
}

func (a *Agent) onInitiate(
	taskId uintptr,
	actionId, message, iconName, cookie string,
	identities []string,
) {
	if a.activeTask != 0 && a.activeTask != taskId {
		C.phalune_agent_session_cancel(C.uintptr_t(a.activeTask))
		C.phalune_agent_task_complete(C.uintptr_t(a.activeTask), 0, nil)
	}

	a.activeTask = taskId
	a.cookie = cookie

	cCookie := C.CString(cookie)
	defer C.free(unsafe.Pointer(cCookie))

	// Default to first identity
	if C.phalune_agent_session_create(C.uintptr_t(taskId), 0, cCookie) != 0 {
		C.phalune_agent_session_initiate(C.uintptr_t(taskId))
	}

	a.dialog.Open(taskId, actionId, message, iconName, cookie, identities)
}

func (a *Agent) handleResponse(taskId uintptr, password string) {
	cPass := C.CString(password)
	defer func() {
		if len(password) > 0 {
			C.memset(unsafe.Pointer(cPass), 0, C.size_t(len(password)))
		}
		C.free(unsafe.Pointer(cPass))
	}()

	C.phalune_agent_session_response(C.uintptr_t(taskId), cPass)
}

func (a *Agent) handleCancel(taskId uintptr) {
	C.phalune_agent_session_cancel(C.uintptr_t(taskId))
	cErr := C.CString("Cancelled by user")
	defer C.free(unsafe.Pointer(cErr))

	C.phalune_agent_task_complete(C.uintptr_t(taskId), 0, cErr)
	if a.activeTask == taskId {
		a.activeTask = 0
	}
}

func (a *Agent) handleSwitchUser(taskId uintptr, identityIdx int) {
	cCookie := C.CString(a.cookie)
	defer C.free(unsafe.Pointer(cCookie))

	if C.phalune_agent_session_create(C.uintptr_t(taskId), C.int(identityIdx), cCookie) != 0 {
		C.phalune_agent_session_initiate(C.uintptr_t(taskId))
	}
}

//export goInitiateAuthentication
func goInitiateAuthentication(
	taskId C.uintptr_t,
	actionId *C.char,
	message *C.char,
	iconName *C.char,
	cookie *C.char,
	identities **C.char,
	numIdentities C.int,
) {
	n := int(numIdentities)
	slice := unsafe.Slice(identities, n)
	goIdents := make([]string, n)
	for i := 0; i < n; i++ {
		goIdents[i] = C.GoString(slice[i])
	}

	act := C.GoString(actionId)
	msg := C.GoString(message)
	ico := C.GoString(iconName)
	cook := C.GoString(cookie)

	glib.IdleAdd(func() {
		agentMu.Lock()
		ag := currentAgent
		agentMu.Unlock()

		if ag != nil {
			ag.onInitiate(uintptr(taskId), act, msg, ico, cook, goIdents)
		} else {
			cErr := C.CString("Agent inactive")
			defer C.free(unsafe.Pointer(cErr))
			C.phalune_agent_task_complete(taskId, 0, cErr)
		}
	})
}

//export goSessionRequest
func goSessionRequest(taskId C.uintptr_t, request *C.char, echoOn C.int) {
	req := C.GoString(request)
	glib.IdleAdd(func() {
		agentMu.Lock()
		ag := currentAgent
		agentMu.Unlock()

		if ag != nil && ag.activeTask == uintptr(taskId) && ag.dialog != nil {
			ag.dialog.SetPrompt(req)
		}
	})
}

//export goSessionShowError
func goSessionShowError(taskId C.uintptr_t, text *C.char) {
	errText := C.GoString(text)
	glib.IdleAdd(func() {
		agentMu.Lock()
		ag := currentAgent
		agentMu.Unlock()

		if ag != nil && ag.activeTask == uintptr(taskId) && ag.dialog != nil {
			ag.dialog.ShowError(errText)
		}
	})
}

//export goSessionShowInfo
func goSessionShowInfo(taskId C.uintptr_t, text *C.char) {
	infoText := C.GoString(text)
	glib.IdleAdd(func() {
		agentMu.Lock()
		ag := currentAgent
		agentMu.Unlock()

		if ag != nil && ag.activeTask == uintptr(taskId) && ag.dialog != nil {
			ag.dialog.ShowInfo(infoText)
		}
	})
}

//export goSessionCompleted
func goSessionCompleted(taskId C.uintptr_t, gainedAuthorization C.int) {
	glib.IdleAdd(func() {
		agentMu.Lock()
		ag := currentAgent
		agentMu.Unlock()

		if ag != nil && ag.activeTask == uintptr(taskId) {
			if gainedAuthorization != 0 {
				if ag.dialog != nil {
					ag.dialog.Close()
				}
				C.phalune_agent_task_complete(taskId, 1, nil)
				ag.activeTask = 0
			} else {
				identIdx := 0
				if ag.dialog != nil {
					identIdx = ag.dialog.SelectedIdentityIndex()
					ag.dialog.ShowError("Authentication failed. Please try again.")
				}
				ag.handleSwitchUser(uintptr(taskId), identIdx)
			}
		}
	})
}

//export goTaskCancelled
func goTaskCancelled(taskId C.uintptr_t) {
	glib.IdleAdd(func() {
		agentMu.Lock()
		ag := currentAgent
		agentMu.Unlock()

		if ag != nil && ag.activeTask == uintptr(taskId) {
			if ag.dialog != nil {
				ag.dialog.Close()
			}
			C.phalune_agent_session_cancel(taskId)
			C.phalune_agent_task_complete(taskId, 0, nil)
			ag.activeTask = 0
		}
	})
}
