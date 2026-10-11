#!/usr/bin/env bash
# Phalune installer for Void Linux.
# For other distributions, see the manual build instructions in docs/USAGE.md.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PREFIX="${PREFIX:-/usr/local}"

fail() { echo "error: $*" >&2; exit 1; }

# Verify Void Linux via /etc/os-release or xbps-install
if [ -f /etc/os-release ]; then
  # shellcheck disable=SC1091
  . /etc/os-release
  [ "${ID:-}" = "void" ] || fail "This script is tailored for Void Linux (detected: ${ID:-unknown}). For manual setup, see README.md or docs/USAGE.md."
fi
command -v xbps-install >/dev/null 2>&1 || fail "xbps-install not found. This script only supports Void Linux."

echo "==> Installing build dependencies via xbps..."
sudo xbps-install -S gtk4-devel gtk4-layer-shell gtk4-layer-shell-devel \
  polkit-devel gobject-introspection blueprint-compiler go pkg-config git

echo "==> Building Phalune..."
make -C "$REPO_ROOT" build

echo "==> Installing to $PREFIX (requires sudo)..."
sudo make -C "$REPO_ROOT" install PREFIX="$PREFIX"

echo "==> Setting up default configuration..."
CONFIG_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/phalune"
if [ ! -f "$CONFIG_DIR/config.toml" ]; then
  mkdir -p "$CONFIG_DIR"
  cp "$REPO_ROOT/example/config.toml" "$CONFIG_DIR/config.toml"
  echo "Created default config at $CONFIG_DIR/config.toml"
else
  echo "Existing configuration found at $CONFIG_DIR/config.toml (kept untouched)"
fi

echo
echo "Installation complete!"
echo "  Launch shell:     phalune"
echo "  Open settings:    phalune-settings"
echo "  Check status:     phalune msg ping && phalune msg status"
echo "  Niri autostart:   add 'spawn \"phalune\"' to your config.kdl"
echo "  User guide:       docs/USAGE.md"
echo
echo "Tip: Consider installing grim + slurp (screenshots) and wl-clipboard (clipboard manager)."
