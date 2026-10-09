#!/usr/bin/env bash
# Installs spout-host for the current user with a systemd user service.
# Usage: ./install.sh [path/to/spout-host]   |   ./install.sh --uninstall
# Also installs the vibeshine_drm virtual display kernel module (DKMS, built from
# Nonary/libvirtualdisplay, MIT/GPL-2.0) and a root helper that only the installing
# user may run via sudo. Needs Linux 6.16+, kernel headers, dkms, and KDE Plasma (kscreen-doctor).
# Use --no-driver to skip that part.
set -euo pipefail

bin_dir="${HOME}/.local/bin"
unit_dir="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user"
here="$(cd "$(dirname "$0")" && pwd)"

helper=/usr/local/libexec/spout-vdisplay
sudoers=/etc/sudoers.d/spout-remote-play
drm_src=3083d210b55fba4b886e5c634e6abee0942201d4 # Nonary/libvirtualdisplay
drm_ver=1.0.0
drm_dir=/usr/src/vibeshine-drm-$drm_ver

install_driver() {
  command -v dkms >/dev/null || { echo "dkms is required (e.g. sudo dnf install dkms kernel-devel / sudo pacman -S dkms linux-headers)" >&2; return 1; }
  command -v kscreen-doctor >/dev/null || echo "warning: kscreen-doctor not found; the virtual display needs KDE Plasma" >&2
  local tmp; tmp="$(mktemp -d)"; trap 'rm -rf "$tmp"' RETURN
  curl -fsSL "https://github.com/Nonary/libvirtualdisplay/archive/${drm_src}.tar.gz" | tar xz -C "$tmp" --strip-components=1
  sudo rm -rf "$drm_dir"
  sudo cp -r "$tmp/linux/vibeshine-drm" "$drm_dir"
  sudo sed "s/@PROJECT_VERSION_NUMERIC@/$drm_ver/" "$drm_dir/dkms.conf.in" | sudo tee "$drm_dir/dkms.conf" >/dev/null
  sudo dkms remove "vibeshine-drm/$drm_ver" --all >/dev/null 2>&1 || true
  sudo dkms install "vibeshine-drm/$drm_ver" --force
  sudo install -D -m 0755 -o root -g root "$here/spout-vdisplay" "$helper"
  printf '%s ALL=(root) NOPASSWD: %s\n' "$(id -un)" "$helper" | sudo tee "$sudoers" >/dev/null
  sudo chmod 0440 "$sudoers"
  sudo visudo -cf "$sudoers" >/dev/null
  sudo modprobe vibeshine_drm create_default_dev=0
}

if [[ "${1:-}" == "--uninstall" ]]; then
  systemctl --user disable --now spout-host.service 2>/dev/null || true
  rm -f "$unit_dir/spout-host.service" "$bin_dir/spout-host"
  if [[ -e "$helper" ]]; then
    sudo "$helper" destroy || true
    sudo rm -f "$helper" "$sudoers"
    sudo dkms remove "vibeshine-drm/$drm_ver" --all 2>/dev/null || true
    sudo rm -rf "$drm_dir"
  fi
  systemctl --user daemon-reload
  echo "Removed."
  exit 0
fi

no_driver=0
if [[ "${1:-}" == "--no-driver" ]]; then no_driver=1; shift; fi
src="${1:-$here/spout-host}"
[[ -x "$src" ]] || { echo "spout-host binary not found at $src" >&2; exit 1; }

mkdir -p "$bin_dir" "$unit_dir"
install -m 0755 "$src" "$bin_dir/spout-host"
install -m 0644 "$here/spout-host.service" "$unit_dir/spout-host.service"
[[ $no_driver == 1 ]] || install_driver
systemctl --user daemon-reload
systemctl --user enable --now spout-host.service
echo "Installed. UI: http://127.0.0.1:47995"
