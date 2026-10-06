### Alpha 0.1

- [x] Widgets.
  - [x] Audio widget with scrollable interaction.
  - [x] Battery widget.
  - [x] System Tray widget.
  - [x] Bluetooth widget.
  - [x] Wi-Fi widget.
  - [x] Keyboard layout indicator and switcher.
  - [x] Power widget.
- [X] Control Center.
  - [X] Audio
  - [X] Network
  - [X] Bluetooth
  - [X] Brightness
  - [X] Power Profiles
  - [X] DND
- [X] Making *almost* every shell part configurable with `toml` and support for hot-reload with `inotify`.
- [X] Multi-monitor hotplug.
- [x] Session management and lock screen.
- [x] Having logs also support showing as notifications (custom slog handler with configurable log-level).
- [X] Notification center with history and grouping.
- [x] OSD for volume, brightness, microphone mute, etc.
- [x] Brightness control.
- [x] Power menu.
- [x] Network / Wi-Fi widget (with direct subview navigation).
- [X] USB/device notifications.
- [X] Microphone/camera usage OSD (and top-bar privacy widget).
- [X] Window switcher.
- [X] Notification actions and replies.
- [X] Notification persistence across shell reloads.
- [x] Consistence animations.

### Alpha 0.2

- [X] Media / MPRIS Controls:
- [X] Launcher updates:
  - [X] Shell commands (`:reboot`, `:poweroff`, `:reload`, `:lock`).
  - [X] `.desktop` sub-actions.
- [X] Clipboard Manager:
  - [X] Background watcher.
  - [X] Searchable history overlay UI.
- [X] Polkit Agent:
  - [X] D-Bus PolicyKit1 listener.
  - [X] GTK4 auth prompt dialog.
- [X] Screenshot Tooling:
  - [X] Area / window / display grab.
  - [X] Actionable preview toast (Copy / Save / Open).

### Alpha 0.3

- [X] Settings app.
- [X] Adding support for other compositors.
- [X] `--json` flag for status queries in ipc.

### Beta 0.4

- [X] Better support for themes and color palletes and customization.
- [X] Extensibility Tier 1: Custom script bar modules (command execution, interval polling, JSON streaming, on_click actions).
- [X] Extensibility Tier 2: Real-time IPC event bus and dynamic widget feed (event subscription stream, push updates).
- [X] Pluggable UI Styles (independent presentation & layout per component):
- [ ] Wallpaper Manager:
  - [ ] Static wallpaper setting per monitor (layer-shell background).
  - [ ] Dynamic color extraction.
- [ ] Making UX better.
- [ ] Scripts for installation.
- [ ] Usage documents.
