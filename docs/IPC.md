# Phalune IPC Architecture & Protocol Reference

Phalune exposes a high-performance Unix domain socket allowing scripts, window managers, keybinds, and external daemons to control the shell, stream real-time desktop telemetry, and build dynamic, interactive bar widgets.

---

## 1. Overview & Connection

### Socket Location
The shell creates and listens on a Unix domain stream socket:
```
$XDG_RUNTIME_DIR/phalune.sock
```
*(Falls back to `/tmp/phalune-<UID>.sock` if `$XDG_RUNTIME_DIR` is unset).*

### Wire Protocol
* **Format:** Newline-delimited JSON ([NDJSON](https://ndjson.org/)).
* **Framing:** Each message is exactly one valid JSON object terminated by `\n` (`0x0A`).
* **Encoding:** UTF-8, no byte-order mark (BOM).
* **Message limit:** 1 MiB per line.
* **Security:** Socket file permissions are set to `0700` (accessible only by the running user).

### Communication Modes

| Mode | Direction | Protocol Pattern | Typical Use Case |
| :--- | :--- | :--- | :--- |
| **Control Commands** | In $\to$ Out | Synchronous request $\to$ single response | Keybinds, hotkeys, window management (`phalune msg ...`) |
| **Event Bus (Pub/Sub)** | Out (Streaming) | Client subscribes $\to$ server pushes stream | Desktop listeners, logging, external docks/scripts |
| **Widget State (Push)** | In $\to$ Out | One-shot state push $\to$ acknowledgment | Periodic cron scripts, shell script status monitors |
| **Widget Daemon (Watch)** | Bi-directional | Persistent connection with events both ways | Interactive extensions (Pomodoro, media controllers, system monitors) |

---

## 2. CLI Control Reference (`phalune msg`)

Phalune includes a built-in CLI client in the same binary. Run `phalune msg <command>` directly or pass `--json` (`-j`) for raw JSON output suitable for scripting.

### Request & Response Format
```json
// Request
{"action": "toggle-launcher", "args": {}}

// Success Response
{"ok": true, "message": "launcher toggled"}

// Error Response
{"ok": false, "error": "unknown command \"xyz\""}
```

### Full Command Reference

#### Application Launcher
| Command | JSON Action | Description |
| :--- | :--- | :--- |
| `phalune msg toggle-launcher` | `toggle-launcher` | Toggle application launcher overlay |
| `phalune msg open-launcher` | `open-launcher` | Open launcher and focus search input |
| `phalune msg close-launcher` | `close-launcher` | Close launcher overlay |

#### Quick Settings & Control Center
| Command | JSON Action | Description |
| :--- | :--- | :--- |
| `phalune msg toggle-control-center` | `toggle-control-center` | Toggle quick settings panel |
| `phalune msg open-control-center` | `open-control-center` | Open quick settings panel |
| `phalune msg close-control-center` | `close-control-center` | Close quick settings panel |
| `phalune msg open-wifi` | `open-wifi` | Open Control Center directly to Wi-Fi network page |
| `phalune msg open-bluetooth` | `open-bluetooth` | Open Control Center directly to Bluetooth page |

#### Notifications
| Command | JSON Action | Description |
| :--- | :--- | :--- |
| `phalune msg toggle-notifications` | `toggle-notifications` | Toggle notification center dropdown |
| `phalune msg open-notifications` | `open-notifications` | Open notification center |
| `phalune msg close-notifications` | `close-notifications` | Close notification center |
| `phalune msg notify <summary> [body]` | `notify` | Send an in-shell desktop notification toast |
| `phalune msg test-notify` | `test-notify` | Display a sample notification toast |

#### Window Switcher (Alt-Tab)
| Command | JSON Action | Description |
| :--- | :--- | :--- |
| `phalune msg window-switcher [next\|prev\|close]` | `window-switcher` | Open or step through Alt-Tab window switcher |
| `phalune msg toggle-window-switcher` | `toggle-window-switcher` | Toggle window switcher visibility |
| `phalune msg next-window` | `next-window` | Select next application window |
| `phalune msg prev-window` | `prev-window` | Select previous application window |

#### Power & Session Management
| Command | JSON Action | Description |
| :--- | :--- | :--- |
| `phalune msg toggle-power-menu` | `toggle-power-menu` | Toggle system power menu dialog |
| `phalune msg open-power-menu` | `open-power-menu` | Open power menu |
| `phalune msg close-power-menu` | `close-power-menu` | Close power menu |
| `phalune msg lock` | `lock` | Lock screen immediately |
| `phalune msg unlock` | `unlock` | Dismiss lock screen (if authenticated) |
| `phalune msg is-locked` | `is-locked` | Check lock status (`{"ok": true, "data": {"locked": true}}`) |
| `phalune msg suspend` | `suspend` | Trigger system suspend/sleep |
| `phalune msg hibernate` | `hibernate` | Trigger system hibernation |
| `phalune msg reboot` | `reboot` | Restart system |
| `phalune msg poweroff` | `poweroff` | Shut down system |
| `phalune msg logout` | `logout` | Terminate compositor session |

#### Clipboard Manager
| Command | JSON Action | Description |
| :--- | :--- | :--- |
| `phalune msg toggle-clipboard` | `toggle-clipboard` | Toggle clipboard history overlay |
| `phalune msg open-clipboard` | `open-clipboard` | Open clipboard history |
| `phalune msg close-clipboard` | `close-clipboard` | Close clipboard history |

#### On-Screen Display (OSD) & Utilities
| Command | JSON Action | Description |
| :--- | :--- | :--- |
| `phalune msg osd <volume\|brightness\|mic> <val>` | `osd` | Display transient feedback OSD slider (0–100) |
| `phalune msg test-osd` | `test-osd` | Show sample volume OSD |
| `phalune msg screenshot [area\|window\|display]` | `screenshot` | Capture interactive screenshot with preview toast |

#### Daemon Lifecycle & Configuration
| Command | JSON Action | Description |
| :--- | :--- | :--- |
| `phalune msg ping` | `ping` | Ping daemon (`{"ok": true, "message": "pong"}`) |
| `phalune msg status` | `status` | Query active monitors, bars, and compositor backend |
| `phalune msg reload-config` | `reload-config` | Hot-reload `config.toml` |
| `phalune msg reload-style` | `reload-style` | Recompile and hot-reload CSS stylesheet |
| `phalune msg set-log-level <level>` | `set-log-level` | Change console log level (`debug`, `info`, `warn`, `error`) |

---

## 3. Dynamic Bar Widgets (`ipc:<id>`)

Phalune allows external processes to control dedicated pill widgets on the top bar. You define a slot in your layout, and any script or daemon can push icons, Pango markup text, progress gauges, dynamic CSS classes, and interactive GTK popovers.

### Step 1: Declare the Widget Slot

Add `ipc:<id>` to any bar section in `~/.config/phalune/config.toml`:

```toml
[bar.right]
widgets = [
  "privacy",
  "tray",
  "wifi",
  "battery",
  "ipc:pomodoro",   # <-- Widget slot for id "pomodoro"
  "clock",
  "power"
]
```

### Step 2: (Optional) Slot Settings in `config.toml`

Widgets are completely driven by IPC. You can tune slot behavior under `[bar.ipc.<id>]`:

```toml
[bar.ipc.pomodoro]
tooltip = "Pomodoro Focus Timer"
hide_when_clear = true    # Keep pill hidden until state is pushed (default: true)
```

> **Note:** Widget icons, text, and styling are controlled dynamically by the pushing script/daemon, keeping `config.toml` clean and focused on layout.

---

### One-Shot State Push (`widget push`)

Useful for cron jobs, backup scripts, package updates, or music alerts:

```bash
# Push an active state
phalune msg widget push --id=pomodoro \
  --icon="alarm-symbolic" \
  --text="<b>24:15</b>" \
  --tooltip="Focus Sprint (Task: Refactor IPC)" \
  --class="running" \
  --percentage=85

# Clear and hide the widget
phalune msg widget clear --id=pomodoro
```

---

### Long-Running Interactive Daemons (`widget watch`)

For rich interactive widgets (timers, monitors, chat status, media controllers), open a persistent bi-directional connection.

#### Starting the Watch Stream
Send a `widget-watch` request to claim exclusive ownership of the widget slot:

```json
{
  "action": "widget-watch",
  "args": {
    "id": "pomodoro",
    "clear_on_disconnect": "true"
  }
}
```

* **`clear_on_disconnect`**: When set to `"true"`, Phalune automatically hides the widget from the bar if the daemon exits or crashes, preventing stale indicators.
* **Handshake Reply:**
  ```json
  {"ok": true, "message": "watching", "data": {"id": "pomodoro", "state": {...}}}
  ```

---

### Widget State Schema (`WidgetState`)

Once connected, stream JSON state lines over the socket:

```json
{
  "id": "pomodoro",
  "text": "<b>18:42</b>",
  "icon": "alarm-symbolic",
  "tooltip": "Pomodoro: 18m left (Work)",
  "class": "running",
  "percentage": 74.8,
  "popover": { ... }
}
```

| Field | Type | Description |
| :--- | :--- | :--- |
| `id` | `string` | **Required.** Must match the slot id (1–64 characters: `a-z0-9._-`). |
| `text` | `string` | Label text. Full **Pango markup** is supported (`<b>`, `<span>`, `<i>`, `<small>`, colors). |
| `icon` | `string` | Themed GTK symbolic icon name (e.g. `alarm-symbolic`, `utilities-system-monitor-symbolic`). |
| `tooltip` | `string` | Text displayed when hovering over the widget. |
| `class` | `string` | Space-separated CSS class names added to the pill (e.g. `running`, `warning`, `urgent`). |
| `percentage`| `number` | Floating point value `0.0`–`100.0`. Displays a smoothed horizontal LevelBar when set. |
| `popover` | `object` | Declarative interactive flyout menu spec (see below). Set `null` to remove. |

* **Clearing a widget:** Send a state line with empty text and icon:
  ```json
  {"id": "pomodoro", "text": "", "icon": ""}
  ```

---

### Declarative Popover Specification (`PopoverSpec`)

When a `popover` object is attached to the pushed state, clicking the widget pill displays a native GTK popover flyout.

```json
{
  "title": "🍅 Pomodoro Timer (Focus Session)",
  "rows": [
    {
      "type": "progress",
      "label": "Focus Session: 18m remaining",
      "value": 0.748
    },
    {
      "type": "button_row",
      "buttons": [
        {"id": "btn_toggle", "label": "⏸ Pause", "class": "suggested-action"},
        {"id": "btn_skip", "label": "⏭ Skip"},
        {"id": "btn_reset", "label": "🔄 Reset"},
        {"id": "btn_stop", "label": "⏹ Stop", "class": "destructive"}
      ]
    },
    {
      "type": "list",
      "items": [
        "Completed sessions: 3",
        "Target today: 8 sessions",
        "Left-click opens controls"
      ]
    },
    {
      "type": "label",
      "label": "<small><i>Phalune IPC Extension</i></small>"
    }
  ]
}
```

#### Supported Row Types
1. **`label`**: Informational text row with Pango markup support.
2. **`progress`**: GTK progress bar row (`value` between `0.0` and `1.0`, optional `label`).
3. **`button_row`**: Horizontal button bar. Each button has `id`, `label`, and optional CSS `class` (e.g. `suggested-action`, `destructive`).
4. **`list`**: Bulleted list of status strings.

---

### User Interaction Events (Phalune $\to$ Daemon)

User clicks on the bar or inside the popover are streamed directly to your daemon over the socket:

#### 1. Direct Widget Click (`widget_clicked`)
Emitted when the user clicks the pill on the bar (when no popover is attached):
```json
{"event": "widget_clicked", "id": "pomodoro", "button": 1}
```
* **`button`**: `1` = Primary (Left), `2` = Middle, `3` = Secondary (Right).

#### 2. Popover Action Click (`popover_action`)
Emitted when the user clicks any action button declared in a popover `button_row`:
```json
{"event": "popover_action", "id": "pomodoro", "action": "btn_toggle"}
```

---

## 4. Shell Event Bus (`subscribe`)

Phalune includes an in-process pub-sub event bus. External programs can subscribe to telemetry updates across the desktop shell.

```bash
# Subscribe to all topics:
phalune msg subscribe

# Subscribe to selected topics:
phalune msg subscribe --events=workspaces,volume,mpris
```

### Raw Socket Request
```json
{"action": "subscribe", "args": {"events": "workspaces,volume,mpris"}}
```
Reply confirmation:
```json
{"ok": true, "message": "subscribed", "data": {"topics": ["mpris", "volume", "workspaces"]}}
```

### Emitted Event Topics & Schemas

#### 1. `workspaces`
Emitted immediately whenever workspaces are focused, switched, or window state changes:
```json
{
  "event": "workspaces",
  "data": {
    "active": {
      "id": 1,
      "name": "1",
      "output": "eDP-1",
      "active": true,
      "focused": true,
      "urgent": false
    },
    "workspaces": [
      {"id": 1, "name": "1", "output": "eDP-1", "active": true, "focused": true, "urgent": false},
      {"id": 2, "name": "2", "output": "eDP-1", "active": false, "focused": false, "urgent": false}
    ]
  }
}
```

#### 2. `volume` & `microphone`
Emitted on audio volume or mute state adjustments:
```json
{
  "event": "volume",
  "data": {
    "volume": 65,
    "muted": false
  }
}
```
```json
{
  "event": "microphone",
  "data": {
    "volume": 80,
    "muted": true
  }
}
```

#### 3. `brightness`
Emitted when display backlight changes:
```json
{
  "event": "brightness",
  "data": {
    "brightness": 70
  }
}
```

#### 4. `mpris`
Emitted on media track change, playback pause/play, or player lifecycle:
```json
{
  "event": "mpris",
  "data": {
    "player": "spotify",
    "title": "Starboy",
    "artist": "The Weeknd",
    "album": "Starboy",
    "status": "Playing"
  }
}
```
*(If all players close, emits `{"status": "Stopped"}`).*

#### 5. `battery`
Emitted via kernel Netlink uevents when battery capacity changes:
```json
{
  "event": "battery",
  "data": {
    "percentage": 92
  }
}
```

#### 6. `notifications`
Emitted when notification count changes:
```json
{
  "event": "notifications",
  "data": {
    "count": 1,
    "items": [
      {
        "id": 104,
        "app_name": "Slack",
        "summary": "New message from Alex",
        "urgency": 1
      }
    ]
  }
}
```

---

## 5. Implementation Examples

### Python (Non-blocking Socket Client)

The complete reference implementation is available in [`scripts/ipc_showcase.py`](../scripts/ipc_showcase.py). Below is a minimal, robust client demonstrating `select`-based non-blocking reads and signal cleanup:

```python
#!/usr/bin/env python3
import json, os, select, signal, socket, sys, time

def default_socket_path():
    runtime_dir = os.environ.get("XDG_RUNTIME_DIR")
    if runtime_dir:
        return os.path.join(runtime_dir, "phalune.sock")
    return f"/tmp/phalune-{os.getuid()}.sock"

sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
sock.connect(default_socket_path())

# 1. Claim widget with auto-clear on disconnect
handshake_req = {
    "action": "widget-watch",
    "args": {"id": "demo", "clear_on_disconnect": "true"}
}
sock.sendall(json.dumps(handshake_req).encode() + b"\n")

# Read handshake confirmation
recv_buf = ""
while "\n" not in recv_buf:
    recv_buf += sock.recv(1024).decode()
handshake_line, recv_buf = recv_buf.split("\n", 1)
print("Handshake:", handshake_line)

def send_state(text, icon="alarm-symbolic", percent=None):
    payload = {
        "id": "demo",
        "text": text,
        "icon": icon,
        "percentage": percent,
        "popover": {
            "title": "Demo Widget",
            "rows": [
                {"type": "label", "label": "Interactive Python Widget"},
                {"type": "button_row", "buttons": [{"id": "btn_ping", "label": "Ping"}]}
            ]
        }
    }
    sock.sendall(json.dumps(payload).encode() + b"\n")

def cleanup(*_):
    print("\nExiting and clearing widget...")
    try:
        sock.sendall(b'{"id":"demo","text":"","icon":""}\n')
        sock.close()
    except OSError:
        pass
    sys.exit(0)

signal.signal(signal.SIGINT, cleanup)
signal.signal(signal.SIGTERM, cleanup)

# Main loop
send_state("<b>Running</b>", percent=50.0)
print("Widget active! Press Ctrl+C to quit.")

while True:
    r, _, _ = select.select([sock], [], [], 1.0)
    if r:
        chunk = sock.recv(4096)
        if not chunk:
            break
        recv_buf += chunk.decode()
        while "\n" in recv_buf:
            line, recv_buf = recv_buf.split("\n", 1)
            event = json.loads(line)
            print("Received event:", event)
            if event.get("event") == "popover_action":
                send_state("<b>Clicked!</b>", percent=100.0)
```

---

### Shell / Bash (`socat` / `jq`)

Stream volume events and trigger custom actions:

```bash
#!/usr/bin/env bash
SOCKET="${XDG_RUNTIME_DIR:-/tmp/phalune-$UID}/phalune.sock"

# Listen to volume events continuously
socat - UNIX-CONNECT:"$SOCKET" <<EOF | while IFS= read -r line; do
{"action": "subscribe", "args": {"events": "volume"}}
EOF
  volume=$(echo "$line" | jq -r '.data.volume // empty')
  if [ -n "$volume" ]; then
    echo "Current Volume: $volume%"
  fi
done
```

---

### Node.js

```javascript
const net = require('net');
const path = process.env.XDG_RUNTIME_DIR
  ? `${process.env.XDG_RUNTIME_DIR}/phalune.sock`
  : `/tmp/phalune-${process.getuid()}.sock`;

const client = net.createConnection(path, () => {
  // Subscribe to MPRIS player updates
  client.write(JSON.stringify({ action: 'subscribe', args: { events: 'mpris' } }) + '\n');
});

client.on('data', (data) => {
  const lines = data.toString().trim().split('\n');
  for (const line of lines) {
    const msg = JSON.parse(line);
    if (msg.event === 'mpris') {
      console.log(`Now playing: ${msg.data.title} by ${msg.data.artist}`);
    }
  }
});
```

---

## 6. Troubleshooting & Best Practices

1. **Deadlock Prevention in Multithreaded Clients:**
   Avoid blocking readers like Python's `sock.makefile().readline()` across multiple threads on Unix domain sockets; use non-blocking raw socket reads with `select.select()` or `poll()` so the process can exit cleanly on `SIGINT`.
2. **Backpressure:**
   Phalune operates on a broadcast model with dropped messages if a subscriber stalls. Never block event-reading loops.
3. **Shell Hot-Reloads:**
   When Phalune's configuration (`config.toml`) is reloaded, all bar widgets automatically re-attach to their respective states in the `WidgetHub`. Connected daemons do not need to restart or re-send state.
4. **Interactive Showcase:**
   For a complete, interactive reference implementation featuring a full Pomodoro timer and real-time System Resource Monitor, see [`scripts/ipc_showcase.py`](../scripts/ipc_showcase.py).