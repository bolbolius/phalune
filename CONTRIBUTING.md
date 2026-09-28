# Contributing

Notes on building, hacking on, and changing `phalune`.

## Setup & build

You need Go 1.22+, GTK4 development headers, `gtk4-layer-shell`, and `blueprint-compiler`.

On Void Linux:
```bash
sudo xbps-install -S gtk4-devel gtk4-layer-shell-devel blueprint-compiler go
```

### Useful commands

```bash
make build    # compiles .blp -> .ui, then builds ./phalune
make ui       # recompile blueprint files only
make test     # run all tests
make tidy     # run go mod tidy
make clean    # removes binary and generated .ui files
```

Run the shell:
```bash
./phalune
```

Send IPC messages to the running instance:
```bash
./phalune msg toggle-launcher
./phalune msg osd volume 75
./phalune msg notify "Hello" "Test notification"
./phalune msg reload-style
./phalune msg ping
```

## Where code goes

- Adding a bar widget:
  1. Put the layout in `ui/widget/<name>.blp`.
  2. Embed it in `ui/ui.go`.
  3. Implement the `widget.Widget` interface in `internal/widget/<name>/`.
  4. Register it in `cmd/phalune/main.go` via `reg.Register(...)`.
  5. Add default CSS in `internal/shell/default.css`.
- Adding an IPC command:
  1. Add the action name to `internal/ipc/ipc.go`.
  2. Handle it in `cmd/phalune/main.go` inside `ipcHandler`.
  3. Call the relevant method on `shell.Shell`.
- Changing UI or layouts:
  1. Edit the `.blp` file in `ui/`. Never build static layouts in Go.
  2. Run `make ui` to update the `.ui` files.

## Notes

- GTK and goroutines: GTK is single-threaded. Always wrap widget updates from goroutines (IPC, Niri listener, tickers, D-Bus) in `glib.IdleAdd`.
- No polling: Never introduce periodic polling loops for system state. Polling burns CPU and battery. Use subscriptions, streaming sockets, or OS events instead.
- Styles: Keep widget structure, layout, and CSS class names in Blueprint. Put visual styling in `internal/shell/default.css` (or override in `~/.config/phalune/style.css`).
- Errors: Return errors with context (`fmt.Errorf("failed to ...: %w", err)`) instead of continuing with nil pointers.

## Development Guidelines

- Read [`ARCHITECTURE.md`](ARCHITECTURE.md) before making structural changes.
- Follow existing patterns before introducing new abstractions.
- Keep GTK layouts in Blueprint rather than procedural Go.
- Run tests and linters before submitting changes (`make test`, `go vet ./...`).
