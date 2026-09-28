package notify

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
)

const (
	DBusPath      = "/org/freedesktop/Notifications"
	DBusInterface = "org.freedesktop.Notifications"
	DBusName      = "org.freedesktop.Notifications"
)

const notificationIntrospectionXML = `
<node>
  <interface name="org.freedesktop.Notifications">
    <method name="GetCapabilities">
      <arg name="capabilities" type="as" direction="out"/>
    </method>
    <method name="Notify">
      <arg name="app_name" type="s" direction="in"/>
      <arg name="replaces_id" type="u" direction="in"/>
      <arg name="app_icon" type="s" direction="in"/>
      <arg name="summary" type="s" direction="in"/>
      <arg name="body" type="s" direction="in"/>
      <arg name="actions" type="as" direction="in"/>
      <arg name="hints" type="a{sv}" direction="in"/>
      <arg name="expire_timeout" type="i" direction="in"/>
      <arg name="id" type="u" direction="out"/>
    </method>
    <method name="CloseNotification">
      <arg name="id" type="u" direction="in"/>
    </method>
    <method name="GetServerInformation">
      <arg name="name" type="s" direction="out"/>
      <arg name="vendor" type="s" direction="out"/>
      <arg name="version" type="s" direction="out"/>
      <arg name="spec_version" type="s" direction="out"/>
    </method>
    <signal name="NotificationClosed">
      <arg name="id" type="u"/>
      <arg name="reason" type="u"/>
    </signal>
    <signal name="ActionInvoked">
      <arg name="id" type="u"/>
      <arg name="action_key" type="s"/>
    </signal>
    <signal name="NotificationReplied">
      <arg name="id" type="u"/>
      <arg name="text" type="s"/>
    </signal>
  </interface>
  <interface name="org.freedesktop.DBus.Introspectable">
    <method name="Introspect">
      <arg name="data" type="s" direction="out"/>
    </method>
  </interface>
</node>
`

var ErrNameAlreadyTaken = errors.New("org.freedesktop.Notifications is already owned by another service")

// IsServiceOwned checks whether org.freedesktop.Notifications is currently owned on the session bus.
func IsServiceOwned() bool {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return false
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	var hasOwner bool
	err = conn.BusObject().CallWithContext(ctx, "org.freedesktop.DBus.NameHasOwner", 0, DBusName).Store(&hasOwner)
	if err != nil {
		return false
	}
	return hasOwner
}

type DBusServer struct {
	conn    *dbus.Conn
	manager *Manager
	mu      sync.Mutex
	closed  bool
}

type dbusHandler struct {
	server *DBusServer
}

func StartDBusServer(mgr *Manager) (*DBusServer, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to session bus: %w", err)
	}

	checkCtx, checkCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer checkCancel()

	// Check if already owned by another service first without blocking
	var hasOwner bool
	err = conn.BusObject().CallWithContext(checkCtx, "org.freedesktop.DBus.NameHasOwner", 0, DBusName).Store(&hasOwner)
	if err == nil && hasOwner {
		_ = conn.Close()
		return nil, ErrNameAlreadyTaken
	}

	server := &DBusServer{
		conn:    conn,
		manager: mgr,
	}

	handler := &dbusHandler{server: server}

	if err := conn.Export(handler, dbus.ObjectPath(DBusPath), DBusInterface); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("failed to export %s: %w", DBusInterface, err)
	}

	if err := conn.Export(introspect.Introspectable(notificationIntrospectionXML), dbus.ObjectPath(DBusPath), "org.freedesktop.DBus.Introspectable"); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("failed to export introspection: %w", err)
	}

	reqCtx, reqCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer reqCancel()

	var r uint32
	err = conn.BusObject().CallWithContext(reqCtx, "org.freedesktop.DBus.RequestName", 0, DBusName, dbus.NameFlagDoNotQueue).Store(&r)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("failed to request name %s: %w", DBusName, err)
	}
	reply := dbus.RequestNameReply(r)

	if reply != dbus.RequestNameReplyPrimaryOwner && reply != dbus.RequestNameReplyAlreadyOwner {
		_ = conn.Close()
		return nil, fmt.Errorf("%w (reply code: %v)", ErrNameAlreadyTaken, reply)
	}

	if mgr != nil {
		mgr.SetOnClose(func(id uint32, reason uint32) {
			server.emitClosed(id, reason)
		})
		mgr.SetOnAction(func(id uint32, actionKey string) {
			server.emitAction(id, actionKey)
		})
		mgr.SetOnReply(func(id uint32, text string) {
			server.emitReply(id, text)
		})
	}

	return server, nil
}

func (s *DBusServer) emitClosed(id uint32, reason uint32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.conn == nil {
		return
	}
	_ = s.conn.Emit(dbus.ObjectPath(DBusPath), DBusInterface+".NotificationClosed", id, reason)
}

func (s *DBusServer) emitAction(id uint32, actionKey string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.conn == nil {
		return
	}
	_ = s.conn.Emit(dbus.ObjectPath(DBusPath), DBusInterface+".ActionInvoked", id, actionKey)
}

func (s *DBusServer) emitReply(id uint32, text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.conn == nil {
		return
	}
	_ = s.conn.Emit(dbus.ObjectPath(DBusPath), DBusInterface+".NotificationReplied", id, text)
	// Also emit ActionInvoked with inline-reply for clients expecting action signal
	_ = s.conn.Emit(dbus.ObjectPath(DBusPath), DBusInterface+".ActionInvoked", id, "inline-reply")
}

func (s *DBusServer) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true

	if s.conn != nil {
		_, _ = s.conn.ReleaseName(DBusName)
		return s.conn.Close()
	}
	return nil
}

// GetCapabilities returns supported notification server capabilities.
func (h *dbusHandler) GetCapabilities() ([]string, *dbus.Error) {
	return []string{
		"action-icons",
		"actions",
		"body",
		"body-markup",
		"icon-static",
		"persistence",
	}, nil
}

// Notify processes an incoming org.freedesktop.Notifications.Notify call.
func (h *dbusHandler) Notify(
	appName string,
	replacesID uint32,
	appIcon string,
	summary string,
	body string,
	actions []string,
	hints map[string]dbus.Variant,
	expireTimeout int32,
) (uint32, *dbus.Error) {
	parsedActions := ParseActions(actions)
	urgency, parsedHints := ParseHints(hints)

	var timeout time.Duration
	if expireTimeout > 0 {
		timeout = time.Duration(expireTimeout) * time.Millisecond
	} else if expireTimeout == 0 {
		timeout = 0
	} else {
		// Server default
		switch urgency {
		case UrgencyLow:
			timeout = 3 * time.Second
		case UrgencyCritical:
			timeout = 0
		default:
			timeout = 5 * time.Second
		}
	}

	notif := Notification{
		ID:            replacesID,
		AppName:       appName,
		Summary:       summary,
		Body:          body,
		Icon:          appIcon,
		Actions:       parsedActions,
		Hints:         parsedHints,
		Urgency:       urgency,
		ExpireTimeout: timeout,
	}

	id := h.server.manager.Show(notif)
	return id, nil
}

// CloseNotification closes an active notification.
func (h *dbusHandler) CloseNotification(id uint32) *dbus.Error {
	h.server.manager.CloseNotification(id)
	return nil
}

// GetServerInformation returns server metadata.
func (h *dbusHandler) GetServerInformation() (string, string, string, string, *dbus.Error) {
	return "phalune", "phalune", "0.1.0", "1.2", nil
}

// ParseActions transforms string slice pairs into Action structs.
func ParseActions(raw []string) []Action {
	var acts []Action
	for i := 0; i+1 < len(raw); i += 2 {
		acts = append(acts, Action{
			Key:   raw[i],
			Label: raw[i+1],
		})
	}
	return acts
}

// ParseHints extracts urgency and maps variant values to interface{}.
func ParseHints(hints map[string]dbus.Variant) (Urgency, map[string]any) {
	urgency := UrgencyNormal
	parsed := make(map[string]any, len(hints))

	for k, v := range hints {
		val := v.Value()
		parsed[k] = val
		if k == "urgency" {
			urgency = ParseUrgency(val)
		}
		if (k == "image-path" || k == "image_path") && parsed["image-path"] == nil {
			if path, ok := val.(string); ok {
				parsed["image-path"] = path
			}
		}
	}

	return urgency, parsed
}

// ParseUrgency converts diverse numeric representations into Urgency.
func ParseUrgency(v any) Urgency {
	switch val := v.(type) {
	case byte:
		if val <= 2 {
			return Urgency(val)
		}
	case int:
		if val >= 0 && val <= 2 {
			return Urgency(val)
		}
	case int8:
		if val >= 0 && val <= 2 {
			return Urgency(val)
		}
	case int16:
		if val >= 0 && val <= 2 {
			return Urgency(val)
		}
	case int32:
		if val >= 0 && val <= 2 {
			return Urgency(val)
		}
	case int64:
		if val >= 0 && val <= 2 {
			return Urgency(val)
		}
	case uint16:
		if val <= 2 {
			return Urgency(val)
		}
	case uint32:
		if val <= 2 {
			return Urgency(val)
		}
	case uint64:
		if val <= 2 {
			return Urgency(val)
		}
	}
	return UrgencyNormal
}
