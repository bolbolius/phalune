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
- [ ] Polkit Agent:
  - [ ] D-Bus PolicyKit1 listener.
  - [ ] GTK4 auth prompt dialog.
- [ ] Screenshot Tooling:
  - [ ] Area / window / display grab.
  - [ ] Actionable preview toast (Copy / Save / Open).

### Alpha 0.3

- [ ] Settings app.
- [ ] Adding support for other WMs.
- [ ] `--json` flag for status queries in ipc.
