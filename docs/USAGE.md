# Phalune Usage Guide

A practical guide to installing, configuring, and using Phalune as your daily desktop shell. For lower-level socket details and command wire formats, see [IPC.md](IPC.md).

---

## Installation

### Void Linux (Automated)

An automated installer is provided for Void Linux:

```bash
./scripts/install.sh
```

This script installs necessary build dependencies via `xbps`, compiles Phalune, installs the binaries and desktop files to `/usr/local` (or your custom `PREFIX`), and creates a starter configuration at `~/.config/phalune/config.toml` if one doesn't exist yet.

### Manual Installation (Any Distribution)

To build and install manually:

```bash
# Required build dependencies:
# gtk4-devel, gtk4-layer-shell(-devel), polkit-devel,
# gobject-introspection, blueprint-compiler, Go 1.22+, pkg-config

make build
sudo make install            # Installs to PREFIX=/usr/local by default
sudo make uninstall          # Removes installed files
```

### Recommended Runtime Tools

While Phalune works standalone, these utilities enhance the experience:
- **Screenshots:** `grim` and `slurp` (required for interactive area captures)
- **Clipboard history:** `wl-clipboard`
- **Preferred terminal emulator:** `alacritty`, `kitty`, `foot`, `wezterm`, `gnome-terminal`, etc.
- **Audio control GUI:** `pavucontrol`

---

## Running Phalune

Start the shell daemon:

```bash
phalune                                  # Reads ~/.config/phalune/config.toml
phalune --config /path/to/config.toml    # Custom config path
phalune-settings                         # Graphical settings app (hot-reloads the shell)
```

Quick checks via CLI:

```bash
phalune msg ping                         # Health check (returns "pong")
phalune msg status                       # Summary of monitors, bars, compositor, locks, DND
phalune msg status --json                # Scriptable JSON format including ipc_version
```

### Autostarting in Niri

Add this line near the top of your `~/.config/niri/config.kdl`:

```kdl
spawn-at-startup "phalune"
```

> **Note:** Phalune runs exclusively under Wayland (`GDK_BACKEND=wayland` is enforced for `gtk4-layer-shell`). Compositor detection works natively on Niri (`NIRI_SOCKET`); Sway (`SWAYSOCK`) and Hyprland support are currently experimental.

---

## Configuration

Phalune looks for its configuration file at:
`$XDG_CONFIG_HOME/phalune/config.toml` (defaulting to `~/.config/phalune/config.toml`).

A fully documented template is available at [`example/config.toml`](../example/config.toml).

- **Live hot-reload:** Changes to `config.toml` and stylesheet files are detected via inotify and applied instantly. You can also trigger reloads manually:
  ```bash
  phalune msg reload-config
  phalune msg reload-style
  ```
- **Safe fallback:** Invalid values fall back gracefully to defaults with a log warning; unrecognized keys are safely ignored.
- **Settings GUI:** Changes saved through `phalune-settings` preserve existing comments and formatting in your TOML file.

Key configuration tables include `[theme]`, `[bar]`, `[lockscreen]`, `[session]`, `[power_menu]`, `[notifications]`, `[osd]`, `[control_center]`, `[launcher]`, `[window_switcher]`, `[screenshot]`, `[clipboard]`, `[wallpaper]`, and `[logging]`.

---

## Status Bar Widgets

Built-in widgets include:
`workspaces`, `clock`, `audio`, `battery`, `tray`, `bluetooth`, `wifi` (or `network`), `keyboard`, `power`, `clipboard`, `notifications`, and `privacy`.

Arrange widgets across the left, center, and right bar sections:

```toml
[bar.right]
widgets = ["tray", "audio", "battery", "notifications", "power"]
```

Helpful tips:
- Scroll on the audio pill to adjust volume smoothly (step size configured under `[bar.audio]`).
- The privacy pill automatically highlights active microphone and camera usage.

### Custom Script Widgets (`custom.<name>`)

Run external scripts on an interval, once on startup, or follow continuous stdout streams. You can return raw text or a JSON object with `text`, `tooltip`, and `class` fields:

```toml
[bar.right]
widgets = ["custom.weather"]

[bar.custom.weather]
exec = "~/.config/phalune/weather.sh"
interval = "10m"
on_click = "xdg-open https://open-meteo.com"
hide_empty = true
```

### Dynamic IPC Widgets (`ipc:<id>`)

External scripts and background daemons can push custom state directly to the bar over the IPC socket:

```toml
[bar.right]
widgets = ["ipc:pomodoro"]

[bar.ipc.pomodoro]
tooltip = "Focus timer"
```

Push or clear states from any shell script or cron job:

```bash
phalune msg widget push --id=pomodoro --text="25:00" --class=running
phalune msg widget clear --id=pomodoro

# Try the built-in interactive demo:
python3 scripts/ipc_showcase.py --id demo
```

---

## Daily Usage & Shortcuts

Bind these commands to key combinations in your compositor config:

### App Launcher
```bash
phalune msg toggle-launcher
```
- Features fuzzy search with frecency-based ranking.
- Supports `.desktop` desktop actions and quick commands like `:reboot`, `:poweroff`, `:reload`, and `:lock`.

### Control Center & Quick Settings
```bash
phalune msg toggle-control-center
phalune msg open-wifi
phalune msg open-bluetooth
```
Provides volume/brightness sliders, audio output routing, Do Not Disturb toggles, and power profile switching.

### Notifications
```bash
phalune msg toggle-notifications
phalune msg notify "Title" "Message body goes here"
```

### Clipboard History
```bash
phalune msg toggle-clipboard
```
Displays searchable clipboard history. Enable persistent cache via `[clipboard] persist = true`. Sensitive inputs from password managers are automatically ignored.

### Screenshots
```bash
phalune msg screenshot area
phalune msg screenshot window
phalune msg screenshot display
```
Interactive toast preview lets you immediately copy, save, or open the image. Requires `grim` and `slurp`.

### Window Switcher
```bash
phalune msg toggle-window-switcher
phalune msg window-switcher next
phalune msg window-switcher prev
phalune msg window-switcher close
```

### Session & Power Management
```bash
phalune msg toggle-power-menu
phalune msg lock
phalune msg unlock
phalune msg is-locked
phalune msg suspend
phalune msg hibernate
phalune msg reboot
phalune msg poweroff
phalune msg logout
```

### Volume & Brightness Controls
```bash
phalune msg volume-up 5
phalune msg volume-down 5
phalune msg volume-mute
phalune msg volume-set 50

phalune msg brightness-up 5
phalune msg brightness-down 5
phalune msg brightness-set 70

phalune msg osd volume 75
```

---

## Wallpapers and Styling

Configure wallpapers and themes in `config.toml`:

```toml
[wallpaper]
enabled = true
path = "~/Pictures/Wallpapers"   # Points to a single file or a slideshow folder
mode = "fill"                    # Options: fill, fit, center, stretch
interval = "15m"                 # Rotation frequency ("0s" to disable slideshow)

[theme]
name = "dracula"                 # Options: phalune, tokyo-night, dracula, from-wallpaper, or custom
```

- **Custom color palettes:** Drop new files into `~/.config/phalune/themes/<name>.toml` defining `[colors]` and `[opacity]`. Individual token overrides can also be set directly under `[theme.values]`.
- **Custom CSS:** Append your own tweaks in `~/.config/phalune/style.css`. It overrides internal rules and hot-reloads instantly.

---

## Scripting & IPC Streaming

Subscribe to live desktop events via newline-delimited JSON streams:

```bash
# Listen to real-time events (workspaces, volume, MPRIS player updates)
phalune msg subscribe --events=workspaces,volume,mpris

# Adjust logging levels on the fly
phalune msg set-log-level debug
phalune msg toggle-notify-logs on
```

During the beta phase, changes to the IPC protocol remain additive: new actions and properties may be introduced without breaking existing integrations. A change to `ipc_version` indicates a breaking change.

---

## Troubleshooting

- **Shell is unresponsive:** Check if the socket exists at `$XDG_RUNTIME_DIR/phalune.sock` (or `/tmp/phalune-<UID>.sock`, permissions `0700`), then test with `phalune msg ping`.
- **Bar does not appear:** Confirm that your compositor environment variable is set (`NIRI_SOCKET`, `SWAYSOCK`, or Hyprland signatures). Check terminal output for `compositor: no backend detected`.
- **Config changes not taking effect:** Run `phalune msg reload-config` and inspect terminal stderr output for validation warnings.
- **Custom CSS styling looks wrong:** Run `phalune msg reload-style`; syntax errors are logged without crashing the default appearance.
- **Desktop notification debugging:** Enable `[logging] notify = true` with `notify_level = "warn"` to view warnings as desktop notifications.
- **Screenshot commands fail:** Make sure `grim` and `slurp` are installed and available in your `$PATH`.
