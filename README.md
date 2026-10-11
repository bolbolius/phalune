# phalune

Modular Linux Wayland desktop shell written in Go, `gtk4`, `gtk4-layer-shell`, and GTK Blueprint. Primary target is [Niri](https://github.com/YaLTeR/niri); [Sway](https://github.com/swaywm/sway) and [Hyprland](https://github.com/hyprwm/Hyprland) are supported experimentally.

---

## Compositor Support (Experimental)

Phalune talks to the running compositor through a compositor-agnostic interface (`internal/compositor`). Backends auto-detect from the environment at startup:

| Compositor | Detection | Status |
|------------|-----------|--------|
| Niri | `NIRI_SOCKET` | Stable |
| Sway | `SWAYSOCK` | Experimental |
| Hyprland | `HYPRLAND_INSTANCE_SIGNATURE` | Experimental |

**Note on experimental status:** Niri is my daily driver and fully tested. Sway and Hyprland are supported but I don't daily drive them to test thoroughly, so there may be rough edges. Feedback and bug reports are welcome.

---

## Requirements

- Linux with Wayland compositor
- `gtk4` & `gtk4-devel`
- `gtk4-layer-shell` & `gtk4-layer-shell-devel`
- `polkit-devel`
- `gobject-introspection`
- `blueprint-compiler`
- Go 1.22+

Install dependencies on Void Linux:
```bash
sudo xbps-install -S gtk4-devel gtk4-layer-shell gtk4-layer-shell-devel polkit-devel gobject-introspection blueprint-compiler
```

---

## Build & Run

The build process uses the `Makefile` to automatically compile all Blueprint files in `ui/` into `.ui` definitions and link the `phalune` binary:

```bash
# Automated install on Void Linux (dependencies + build + install + default config)
./scripts/install.sh

# Or build manually (.blp -> .ui and binary compilation)
make build

# Install system files (PREFIX=/usr/local by default)
sudo make install

# Run
./phalune

# Run settings app
./phalune-settings

# Run with custom config
./phalune --config /path/to/config.toml
```

Other targets:
```bash
# Compile UI definitions only
make ui

# Build only the shell or only the settings app
make shell
make settings

# Run test suite
make test

# Clean binary and generated UI files
make clean
```

---

## Project & UI Architecture

All declarative UI definitions are structured as reusable Blueprint templates inside `ui/` and compiled to GTK XML (`.ui`), embedded via `phalune/ui`. Go handles event subscriptions, system bus interactions, and window lifecycle without hardcoded UI layout hierarchies.

---

## Configuration

Loaded from `$XDG_CONFIG_HOME/phalune/config.toml` or `~/.config/phalune/config.toml` (uses defaults if absent). See [`example/config.toml`](example/config.toml) for the full documented reference.

```toml
[bar]
height = 36

[bar.left]
widgets = ["workspaces"]

[bar.center]
widgets = ["clock"]

[bar.right]
widgets = ["privacy", "tray", "wifi", "bluetooth", "audio", "battery", "keyboard", "clipboard", "notifications", "power"]

[bar.clock]
format = "15:04"

[window_switcher]
all_workspaces = true
```

---

## IPC & CLI Commands

Phalune runs a Unix domain socket server at `$XDG_RUNTIME_DIR/phalune.sock`. You can control every part of the shell, bind shortcuts in your compositor, or write scripts using `phalune msg`:

```bash
# App Launcher
phalune msg toggle-launcher
phalune msg open-launcher
phalune msg close-launcher

# Control Center (Quick Settings)
phalune msg toggle-control-center
phalune msg open-control-center
phalune msg close-control-center

# Notification Center
phalune msg toggle-notification-center

# Window Switcher (Alt-Tab)
phalune msg toggle-window-switcher
phalune msg window-switcher next
phalune msg window-switcher prev
phalune msg window-switcher close

# Power Menu
phalune msg toggle-power-menu
phalune msg open-power-menu
phalune msg close-power-menu

# Clipboard History
phalune msg toggle-clipboard
phalune msg open-clipboard
phalune msg close-clipboard

# Session & Screen Locking
phalune msg lock
phalune msg unlock
phalune msg is-locked
phalune msg suspend
phalune msg hibernate
phalune msg reboot
phalune msg poweroff
phalune msg logout

# On-Screen Display (OSD)
phalune msg osd volume 75
phalune msg osd brightness 50
phalune msg test-osd

# Screenshots (modes: area, window, display; needs grim and slurp)
phalune msg screenshot
phalune msg screenshot area
phalune msg screenshot window
phalune msg screenshot display

# Desktop Notifications
phalune msg notify "Download Finished" "archlinux-2026.iso has finished downloading."
phalune msg test-notify

# Reload configuration and stylesheets without restarting
phalune msg reload-config
phalune msg reload-style

# Diagnostics & Status (supports --json / -j for scripting)
phalune msg status
phalune msg status --json
phalune msg is-locked --json
phalune msg ping

# Dynamic bar widgets (custom scripts pushing over IPC; see docs/IPC.md)
phalune msg widget push --id=pomodoro --text="🍅 18:42" --class=running --percentage=65
phalune msg widget clear --id=pomodoro
phalune msg widget watch --id=pomodoro

# Stream desktop events in real time as NDJSON
phalune msg subscribe
phalune msg subscribe --events=workspaces,volume,mpris
```

---

## Extensibility

You can extend the status bar through two different approaches:

- **Custom Script Widgets (`custom.<name>`)**: Run any shell command at a set interval, follow an ongoing stdout stream, or output structured JSON with tooltips and CSS classes. Also supports click and scroll actions.
- **Dynamic IPC Widgets (`ipc:<id>`)**: Let external scripts, daemons, or background jobs reserve a slot in the bar and push updates directly over the IPC socket. External tools can also subscribe to live desktop events like workspace switching, volume changes, MPRIS playback, and battery levels.

For the full socket protocol specification and example clients, see [**docs/IPC.md**](docs/IPC.md).

---

## Features

- Window Switcher (Alt-Tab)
- Application Launcher
- Clipboard History (search, paste, preview, persist)
- Control Center (Quick Settings)
- Multi-Monitor Bar
- On-Screen Display (OSD)
- Notifications & Notification Center
- Screenshots (area/window/display with actionable Copy / Save / Open preview toast)
- Settings App (`phalune-settings`, edits config.toml with line-preserving writes + hot-reload)
- Tier 1 script widgets + Tier 2 IPC event bus and dynamic widget push

---

## Styling

Shell colors come from **themes**. Built-ins: `phalune` (default, luna moth), `tokyo-night`, `dracula`.

```toml
# config.toml
[theme]
name = "dracula"          # built-in or ~/.config/phalune/themes/<name>.toml

# optional: override individual tokens on top of the theme
[theme.values]
accent = "#ff79c6"
surface = [40, 42, 54]    # "r g b" triplet; the opacity ladder is applied by phalune
```

Theme files accept a `[colors]` table of tokens (`surface`, `text`, `accent`,
`error`, `success`, ...), an `[opacity]` table (`bar`, `overlay`, `solid`,
`scrim`, `shadow`), and `name` / `description` / `dark` metadata. See the
embedded copies in [`internal/theme/themes/`](internal/theme/themes/) for
working examples. Settings → Appearance lists every available theme.

Custom CSS still works and overrides everything (append-only):
`~/.config/phalune/style.css` — the default stylesheet references theme
tokens through CSS variables, so hardcoding colors there is no longer
necessary.

---

## Documentation

- [`docs/USAGE.md`](docs/USAGE.md) — Setup, configuration reference, and everyday usage.
- [`docs/IPC.md`](docs/IPC.md) — Socket IPC wire protocol and scripting reference.
- [`ARCHITECTURE.md`](ARCHITECTURE.md) — Internal architecture and project structure.
- [`CONTRIBUTING.md`](CONTRIBUTING.md) — Development workflow and contribution guidelines.

---

## License

Phalune is licensed under the GNU General Public License v3.0 or later.
