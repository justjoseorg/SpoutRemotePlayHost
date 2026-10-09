#!/usr/bin/env bash
# Installs spout-host for the current user with a systemd user service.
# Usage: ./install.sh [path/to/spout-host]   |   ./install.sh --uninstall
# Note: there is no Linux virtual display backend yet, so no display driver is installed.
set -euo pipefail

bin_dir="${HOME}/.local/bin"
unit_dir="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user"
here="$(cd "$(dirname "$0")" && pwd)"

if [[ "${1:-}" == "--uninstall" ]]; then
  systemctl --user disable --now spout-host.service 2>/dev/null || true
  rm -f "$unit_dir/spout-host.service" "$bin_dir/spout-host"
  systemctl --user daemon-reload
  echo "Removed."
  exit 0
fi

src="${1:-$here/spout-host}"
[[ -x "$src" ]] || { echo "spout-host binary not found at $src" >&2; exit 1; }

mkdir -p "$bin_dir" "$unit_dir"
install -m 0755 "$src" "$bin_dir/spout-host"
install -m 0644 "$here/spout-host.service" "$unit_dir/spout-host.service"
systemctl --user daemon-reload
systemctl --user enable --now spout-host.service
echo "Installed. UI: http://127.0.0.1:47995"
