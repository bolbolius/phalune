# Architecture

The setup is similar to Qt/QML (inspired by `quickshell`):
- Go handles logic, state, IPC, and system integration.
- Blueprint (`.blp`) defines the widget tree, layouts, sizing, and CSS classes.

Go code shouldn't construct GTK layouts manually. We load `.blp` templates via `gtk.NewBuilderFromString(ui.<Widget>)` and wire up signals or dynamic children from there.

## Structure

- `cmd/phalune/` — App entry point. Runs the daemon or sends CLI messages (`phalune msg ...`).
- `cmd/phalune-settings/` — Standalone settings app (`org.phalune.settings`). Edits config.toml with line-preserving writes, then asks the shell to reload over the same IPC socket.
- `internal/`
  - `compositor/` — Compositor-agnostic `Service` interface, shared state cache, and backends:
    - `niri/` — Niri JSON-RPC socket; workspace/window/keyboard events and focus actions (stable).
    - `sway/` — Sway i3-IPC socket on `SWAYSOCK` (experimental).
    - `hyprland/` — Hyprland command/event sockets in `$XDG_RUNTIME_DIR/hypr/` (experimental).
    - `detect/` — Environment-based backend auto-detection (`NIRI_SOCKET`, `SWAYSOCK`, `HYPRLAND_INSTANCE_SIGNATURE`).
  - `config/` — Parses `config.toml` and validates widget names.
  - `ipc/` — Unix socket server and client at `$XDG_RUNTIME_DIR/phalune.sock`.
    Beyond one-shot commands it hosts the Tier 2 extensibility surface:
    - `EventBus` — in-process pub-sub; shell state changes publish to topics
      (`workspaces`, `volume`, `brightness`, `microphone`, `mpris`,
      `notifications`, `battery`) and `subscribe` streams them to clients as NDJSON.
    - `WidgetHub` — state store for `ipc:<id>` bar widgets pushed by external
      daemons over `widget watch` connections (exclusive ownership, state
      persists across daemon restarts).
  - `shell/` — Glues the bar, launcher, OSD, notifications, and CSS together (style pipeline: `default.css` component rules + `tokens.css` fallbacks + generated theme block + user `style.css`, in cascade order); starts the event emitters that mirror internal state onto the bus.
    - `bar/` — Top bar layer surface created for each connected monitor.
  - `theme/` — Color palette engine: TOML theme files (`[colors]`, `[opacity]`) parsed into semantic tokens; three built-ins embedded (`phalune`, `tokyo-night`, `dracula`); user themes in `$XDG_CONFIG_HOME/phalune/themes/`. Renders the `:root` CSS variable block that recolors `shell/default.css`.
  - `widget/` — Bar widget interface and registry (`clock`, `workspaces`, `audio`, `battery`, `tray`, `bluetooth`, `wifi`, `keyboard`, `power`, `notifications`, `privacy`), two extension factories:
    - `custom/` — Tier 1 user script widgets (`custom.<name>`: exec/interval/tail/JSON).
    - `ipcx/` — Tier 2 dynamic widgets (`ipc:<id>`): render state pushed by external daemons through the WidgetHub, with declarative popovers and click/action callbacks streamed back over the socket.
  - `launcher/` — App launcher overlay with fuzzy search, frecency ranking, and `.desktop` parsing.
  - `controlcenter/` — Quick settings overlay (Wi-Fi, Bluetooth, DND, power profiles, volume and brightness sliders, media stream routing).
  - `notificationcenter/` — Dropdown panel for notification history and quick actions.
  - `windowswitcher/` — Horizontal Alt-Tab window switcher with application tiles and instant workspace navigation.
  - `osd/` — Volume, microphone, caps lock, and brightness overlay with auto-dismiss and native listeners.
  - `notify/` — Notification toasts and standard `org.freedesktop.Notifications` D-Bus service.
  - `powermenu/` — System power management dialog (Lock, Suspend, Hibernate, Reboot, Power Off).
  - `lockscreen/` — Full-screen overlay lock screen with PAM password authentication.
  - `session/` — Session management listening to `systemd-logind` / `ConsoleKit2` signals.
  - `privacy/` — Hardware usage indicators (microphone and camera active detection).
  - `removable/` — Automatic notifications when USB drives and storage media are plugged in.
  - `screenshot/` — Screenshot tooling (grim/slurp capture for area, window, and display) with an actionable Copy / Save / Open preview toast.
  - `settings/` — Settings app backend: schema (pages/rows), line-preserving TOML editor, shell IPC client.
- `ui/` — Blueprint files (`.blp`), plus `ui.go` which embeds the compiled `.ui` XML for Go.

## How it works

Running `phalune` reads `config.toml`, detects and connects to the running compositor (see `internal/compositor/detect`), and registers the bar widgets. Once the GTK application activates, `shell.New()` creates a top bar on each connected monitor.

The launcher, control center, OSD, and notification popups also create layer-shell surfaces on the overlay layer, but stay hidden until triggered.

While running, the shell listens on a Unix socket. Commands like `phalune msg toggle-launcher` connect to this socket, dispatch the action to GTK's main loop via `glib.IdleAdd`, and return a status string.

The same socket carries the Tier 2 streams: `phalune msg subscribe` receives every internal state change as NDJSON, and `phalune msg widget watch --id=x` gives an external daemon exclusive ownership of an `ipc:x` bar widget — it pushes state lines and receives click/`popover_action` events on the same connection. See `docs/IPC.md` for the wire protocol.

## Notes

- Single binary: Both the daemon and CLI client live in the same `phalune` binary.
- Wayland only: Startup forces `GDK_BACKEND=wayland` because `gtk4-layer-shell` requires Wayland.
- GTK and goroutines: GTK is not thread-safe. Anything touching widgets from a background goroutine (compositor event stream, IPC, D-Bus, tickers) must go through `glib.IdleAdd`.
- No polling: We avoid periodic polling loops across the shell because it wastes CPU and battery. We prefer push-based event streams and kernel subscriptions (e.g. Niri JSON-RPC socket stream, sway/Hyprland IPC, pactl subscribe for audio/mic, and Linux Netlink uevent socket for backlight and hardware keys).
- Generated UI files: `make` compiles `.blp` into `.ui` files in `ui/`. We have them git-ignored and keep on disk so `gopls` doesn't complain about missing embed files.

