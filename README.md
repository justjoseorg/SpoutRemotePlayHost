# SpoutRemotePlay Host

Host-side app for the [SpoutRemotePlay](https://github.com/justjoseorg/SpoutRemotePlay) Decky plugin. Targets Windows and Linux (x86_64).

Supported architecture: x86_64 only (Windows and Linux); no ARM builds.

## Goal

When a Steam Remote Play session starts, create/activate a dedicated virtual monitor for the stream so the physical displays are left alone; tear it down when the session ends.

## Status

Nothing here has been run on a Windows machine yet.

- Web UI and API (`127.0.0.1:47995`): edit width/height/refresh/auto-create/codec preference, create/destroy the monitor. Works and has tests. To let the Decky plugin connect, start with `-listen 0.0.0.0:47995`; non-loopback requests must send a bearer token: the API token (printed at startup, stored in `token` next to `config.json`) or a per-device token from pairing. Cross-origin browser writes are rejected.
- Codec preference is only a stored hint: Steam Remote Play negotiates the real codec itself, and PyroWave is not available with Steam streaming.
- Windows backend: talks to the [SudoVDA](https://github.com/SudoMaker/SudoVDA) virtual display driver (same driver ArtLight uses) over its IOCTL protocol, including the watchdog ping. Compiles; untested. SudoVDA must be installed separately.
- Hotkey: Ctrl+Alt+Shift+Q (Moonlight's quit-stream shortcut) removes the virtual monitor. Windows only (`RegisterHotKey`); compiles, untested. Restoring physical monitors is not implemented yet.
- Linux: UI runs, but there is no virtual display backend yet.
- Not started: detecting a Steam Remote Play session (for `autoCreate`).

## Install

- **Windows:** run `SpoutRemotePlayHost-Setup-vX.Y.Z.exe` from Releases. The optional "SudoVDA virtual display driver" component is built from [SudoMaker/SudoVDA](https://github.com/SudoMaker/SudoVDA) (MIT) in CI and signed with a self-signed certificate; installing it adds that certificate to the Windows Trusted Root and Trusted Publishers stores (removed on uninstall). Untested.
- **Linux:** extract `spout-host-vX.Y.Z-linux-amd64.tar.gz` and run `./install.sh` (user systemd service; `--uninstall` removes it). No virtual display driver is installed because there is no Linux backend yet.

## Build

```bash
go build ./cmd/spout-host
GOOS=windows GOARCH=amd64 go build ./cmd/spout-host
```

## Releases

Merging a PR into `main` publishes a release automatically. `MAJOR.MINOR` is set by hand in the `VERSION` file; the patch number is auto-incremented per merge (e.g. `0.1.0`, `0.1.1`, ...). Edit `VERSION` in a PR to start a new minor/major. Add the `no-release` label to a PR to skip releasing.

## Pairing

Same flow as ArtLight/ArtMoon: the client shows a 4-digit PIN, the host raises a notification (Windows toast or Linux `notify-send`; clicking it opens the UI), and you type the PIN into the host UI. Only a salted hash of the PIN leaves the client, requests expire after 2 minutes and are cancelled after 5 wrong PINs, and confirming/revoking is only possible from this PC. Each device gets its own revocable token (`paired.json` stores hashes only). The notification code is untested on Windows.
