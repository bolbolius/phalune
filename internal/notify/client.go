package notify

import (
	"context"
	"fmt"
	"time"

	"github.com/godbus/dbus/v5"
)

// SendDBus sends a notification via the standard org.freedesktop.Notifications D-Bus service.
func SendDBus(n Notification) (uint32, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return 0, fmt.Errorf("failed to connect to session bus: %w", err)
	}
	defer conn.Close()

	var actions []string
	for _, act := range n.Actions {
		actions = append(actions, act.Key, act.Label)
	}

	hints := make(map[string]dbus.Variant)
	for k, v := range n.Hints {
		hints[k] = dbus.MakeVariant(v)
	}
	hints["urgency"] = dbus.MakeVariant(byte(n.Urgency))

	var timeoutMs int32 = -1
	if n.ExpireTimeout > 0 {
		timeoutMs = int32(n.ExpireTimeout / time.Millisecond)
	} else if n.ExpireTimeout == 0 && n.Urgency == UrgencyCritical {
		timeoutMs = 0
	}

	appName := n.AppName
	if appName == "" {
		appName = "phalune"
	}

	obj := conn.Object(DBusName, dbus.ObjectPath(DBusPath))
	var id uint32
	call := obj.CallWithContext(ctx, DBusInterface+".Notify", 0,
		appName,
		n.ID,
		n.Icon,
		n.Summary,
		n.Body,
		actions,
		hints,
		timeoutMs,
	)
	if call.Err != nil {
		return 0, fmt.Errorf("D-Bus Notify failed: %w", call.Err)
	}

	if err := call.Store(&id); err != nil {
		return 0, fmt.Errorf("failed to decode notification ID: %w", err)
	}

	return id, nil
}
