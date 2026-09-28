package tray

import (
	"fmt"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
)

const DBusMenuInterface = "com.canonical.dbusmenu"

type MenuItem struct {
	ID          int32
	Label       string
	IconName    string
	IconData    []byte
	Enabled     bool
	Visible     bool
	Type        string // "standard", "separator"
	ToggleType  string // "checkmark", "radio", ""
	ToggleState int32  // 0, 1
	Children    []*MenuItem
}

func (it *TrayItem) Activate(x, y int32) error {
	mgr, err := GetManager()
	if err != nil {
		return err
	}
	obj := mgr.conn.Object(it.BusName, it.Path)
	call := obj.Call(ItemInterface+".Activate", 0, x, y)
	return call.Err
}

func (it *TrayItem) ContextMenu(x, y int32) error {
	mgr, err := GetManager()
	if err != nil {
		return err
	}
	obj := mgr.conn.Object(it.BusName, it.Path)
	call := obj.Call(ItemInterface+".ContextMenu", 0, x, y)
	return call.Err
}

func (it *TrayItem) GetMenu() ([]*MenuItem, error) {
	if it.MenuPath == "" {
		return nil, fmt.Errorf("no menu path provided")
	}

	mgr, err := GetManager()
	if err != nil {
		return nil, err
	}

	obj := mgr.conn.Object(it.BusName, it.MenuPath)

	// Call AboutToShow(0) if supported
	_ = obj.Call(DBusMenuInterface+".AboutToShow", 0, int32(0))

	var revision uint32
	var rawLayout []interface{}

	// Call GetLayout(0, 2, [])
	call := obj.Call(DBusMenuInterface+".GetLayout", 0, int32(0), int32(2), []string{})
	if call.Err != nil {
		return nil, call.Err
	}

	if len(call.Body) < 2 {
		return nil, fmt.Errorf("invalid GetLayout response length: %d", len(call.Body))
	}

	if rev, ok := call.Body[0].(uint32); ok {
		revision = rev
		_ = revision
	}

	if raw, ok := call.Body[1].([]interface{}); ok {
		rawLayout = raw
	} else if rawStruct, ok := call.Body[1].(dbus.Variant); ok {
		if s, ok := rawStruct.Value().([]interface{}); ok {
			rawLayout = s
		}
	}

	root := parseMenuItem(rawLayout)
	if root == nil {
		return nil, fmt.Errorf("failed to parse root menu layout")
	}

	return root.Children, nil
}

func (it *TrayItem) CallMenuEvent(id int32) error {
	if it.MenuPath == "" {
		return fmt.Errorf("no menu path provided")
	}

	mgr, err := GetManager()
	if err != nil {
		return err
	}

	obj := mgr.conn.Object(it.BusName, it.MenuPath)
	call := obj.Call(DBusMenuInterface+".Event", 0, id, "clicked", dbus.MakeVariant(0), uint32(time.Now().Unix()))
	return call.Err
}

func parseMenuItem(raw []interface{}) *MenuItem {
	if len(raw) < 3 {
		return nil
	}

	item := &MenuItem{
		Enabled: true,
		Visible: true,
		Type:    "standard",
	}

	// [0] ID
	if id, ok := raw[0].(int32); ok {
		item.ID = id
	}

	// [1] Props map[string]dbus.Variant
	if props, ok := raw[1].(map[string]dbus.Variant); ok {
		for k, v := range props {
			val := v.Value()
			switch k {
			case "label":
				if s, ok := val.(string); ok {
					item.Label = cleanMenuLabel(s)
				}
			case "enabled":
				if b, ok := val.(bool); ok {
					item.Enabled = b
				}
			case "visible":
				if b, ok := val.(bool); ok {
					item.Visible = b
				}
			case "type":
				if s, ok := val.(string); ok {
					item.Type = s
				}
			case "icon-name":
				if s, ok := val.(string); ok {
					item.IconName = s
				}
			case "icon-data":
				if data, ok := val.([]byte); ok {
					item.IconData = data
				}
			case "toggle-type":
				if s, ok := val.(string); ok {
					item.ToggleType = s
				}
			case "toggle-state":
				if state, ok := val.(int32); ok {
					item.ToggleState = state
				}
			}
		}
	}

	// [2] Children []dbus.Variant or []interface{}
	if childrenRaw, ok := raw[2].([]interface{}); ok {
		for _, c := range childrenRaw {
			if childSlice, ok := c.([]interface{}); ok {
				if parsed := parseMenuItem(childSlice); parsed != nil && parsed.Visible {
					item.Children = append(item.Children, parsed)
				}
			} else if variant, ok := c.(dbus.Variant); ok {
				if childSlice, ok := variant.Value().([]interface{}); ok {
					if parsed := parseMenuItem(childSlice); parsed != nil && parsed.Visible {
						item.Children = append(item.Children, parsed)
					}
				}
			}
		}
	} else if childrenVariants, ok := raw[2].([]dbus.Variant); ok {
		for _, v := range childrenVariants {
			if childSlice, ok := v.Value().([]interface{}); ok {
				if parsed := parseMenuItem(childSlice); parsed != nil && parsed.Visible {
					item.Children = append(item.Children, parsed)
				}
			}
		}
	}

	return item
}

// cleanMenuLabel strips GTK/Qt mnemonic underscores (e.g. "_Quit" -> "Quit", "Play / _Pause" -> "Play / Pause")
func cleanMenuLabel(s string) string {
	var sb strings.Builder
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		if runes[i] == '_' {
			if i+1 < len(runes) && runes[i+1] == '_' {
				sb.WriteRune('_')
				i++
				continue
			}
			continue
		}
		sb.WriteRune(runes[i])
	}
	return sb.String()
}
