# SpigotRemotePlay Host

Host-side app for the [SpigotRemotePlay](https://github.com/justjoseorg/SpigotRemotePlay) Decky plugin. Targets Windows and Linux (x86_64).

## Goal

When a Steam Remote Play session starts, create/activate a dedicated virtual monitor for the stream so the physical displays are left alone; tear it down when the session ends.

## Status

Nothing here has been run on a Windows machine yet.

- Web UI and API (`127.0.0.1:47995`, override with `-listen`): edit width/height/refresh/auto-create, create/destroy the monitor. Works and has tests; no authentication, so it binds to loopback by default.
- Windows backend: talks to the [SudoVDA](https://github.com/SudoMaker/SudoVDA) virtual display driver (same driver ArtLight uses) over its IOCTL protocol, including the watchdog ping. Compiles; untested. SudoVDA must be installed separately.
- Hotkey: Ctrl+Alt+Shift+Q (Moonlight's quit-stream shortcut) removes the virtual monitor. Windows only (`RegisterHotKey`); compiles, untested. Restoring physical monitors is not implemented yet.
- Linux: UI runs, but there is no virtual display backend yet.
- Not started: detecting a Steam Remote Play session (for `autoCreate`).

## Install

- **Windows:** run `SpigotRemotePlayHost-Setup-vX.Y.Z.exe` from Releases. The optional "SudoVDA virtual display driver" component is built from [SudoMaker/SudoVDA](https://github.com/SudoMaker/SudoVDA) (MIT) in CI and signed with a self-signed certificate; installing it adds that certificate to the Windows Trusted Root and Trusted Publishers stores (removed on uninstall). Untested.
- **Linux:** extract `spigot-host-vX.Y.Z-linux-amd64.tar.gz` and run `./install.sh` (user systemd service; `--uninstall` removes it). No virtual display driver is installed because there is no Linux backend yet.

## Build

```bash
go build ./cmd/spigot-host
GOOS=windows GOARCH=amd64 go build ./cmd/spigot-host
```

## Releases

Merging a PR into `main` publishes a release automatically. `MAJOR.MINOR` is set by hand in the `VERSION` file; the patch number is auto-incremented per merge (e.g. `0.1.0`, `0.1.1`, ...). Edit `VERSION` in a PR to start a new minor/major. Add the `no-release` label to a PR to skip releasing.
