# SpoutRemotePlay Host

Host-side app for the [SpoutRemotePlay](https://github.com/justjoseorg/SpoutRemotePlay) Decky plugin. Targets Windows and Linux (x86_64).

Supported architecture: x86_64 only (Windows and Linux); no ARM builds.

## The idea

**Pick up your handheld, press play on a game from your PC, and go.**

This is an integrated solution with a narrow purpose: **the virtual monitor, integrated with Steam's native Remote Play.** It does not stream anything itself and does not replace Steam. Steam keeps doing the streaming (capture, encode, input, Steam Link on the handheld); this app only watches for a Remote Play session and gives it a dedicated virtual monitor matching the connecting device (its resolution and refresh rate), then removes it when the session ends. Your physical displays are left alone.

Pair each device once, give it its own monitor settings, and from then on it just works. Together with the [Decky plugin](https://github.com/justjoseorg/SpoutRemotePlay) (Wake-on-LAN, pairing, settings), the flow is: wake the PC, press play in Steam, stream.

## How it knows a stream started

Two independent signals, so it doesn't depend on one fragile hook:

- **From the handheld:** the plugin reports Steam's Remote Play start/stop to every paired PC (`POST /api/session`, authenticated with the device's own token). Verified with an AYN Odin 2 Portal streaming Celeste.
- **From the PC:** the host tails Steam's `streaming_log.txt` ("Streaming started to <device>…"). The session end marker is only known on Linux; on Windows rely on the plugin's stop signal.

The monitor is created from that device's config (or the defaults), is kept across a quick stream restart (5 s grace), and is only removed by the device that owns it.

## Status

Nothing here has been run on a Windows machine yet.

- Web UI and API (`127.0.0.1:47995`): dark UI with a Devices tab (each paired device has its own resolution and refresh) and a Monitor defaults tab (default 1920x1080@60). Works and has tests. To let the Decky plugin connect, start with `-listen 0.0.0.0:47995`; non-loopback requests must send a bearer token: the API token (printed at startup, stored in `token` next to `config.json`) or a per-device token from pairing. Cross-origin browser writes are rejected.
- Session detection: verified end to end on Linux (plugin signal and log watcher both reached the host). The resulting monitor creation on Linux is not yet verified, because the driver install is still being tested. Not run on Windows.
- There is no codec setting: Steam Remote Play negotiates the codec itself, and PyroWave is not available with Steam streaming.
- Windows backend: talks to the [SudoVDA](https://github.com/SudoMaker/SudoVDA) virtual display driver (same driver ArtLight uses) over its IOCTL protocol, including the watchdog ping. Built and signed in CI; untested on a machine.
- Linux backend: uses the `vibeshine_drm` kernel module (the driver ArtLight uses, built via DKMS) and `kscreen-doctor`, so it needs KDE Plasma on Wayland and Linux 6.16+. Unit-tested; not yet run on hardware.
- Hotkey: Ctrl+Alt+Shift+Q (Moonlight's quit-stream shortcut) removes the virtual monitor. Windows only (`RegisterHotKey`); compiles, untested. Restoring physical monitors is not implemented yet.
- Tray icon (Windows and Linux): click it, or choose "Open Spout Remote Play Host", to open the web UI. Run with `-no-tray` to disable. Linux needs a StatusNotifier-capable panel (KDE has one; GNOME needs the AppIndicator extension). Verified to register on KDE only; the Windows tray is untested.

## Install

- **Windows:** run `SpoutRemotePlayHost-Setup-vX.Y.Z.exe` from Releases. The optional "SudoVDA virtual display driver" component is built from [SudoMaker/SudoVDA](https://github.com/SudoMaker/SudoVDA) (MIT) in CI and signed with a self-signed certificate; installing it adds that certificate to the Windows Trusted Root and Trusted Publishers stores (removed on uninstall). Untested.
- **Linux:** extract `spout-host-vX.Y.Z-linux-amd64.tar.gz` and run `./install.sh` (user systemd service; `--uninstall` removes it). It also sets up the driver with DKMS: it installs `dkms` and kernel headers for you (Fedora/Arch/Debian/Ubuntu/openSUSE), and on Fedora it fetches headers for the exact running kernel from Koji so no reboot is needed (only as a last resort does it build for the newest installed kernel and ask for a reboot). It needs sudo and a sudoers rule limited to `/usr/local/libexec/spout-vdisplay`; pass `--no-driver` to skip.

## Build

```bash
go build ./cmd/spout-host
GOOS=windows GOARCH=amd64 go build ./cmd/spout-host
```

## Releases

Windows: download `SpoutRemotePlayHost-Setup-v*.exe` from the latest release. It installs the host, the SudoVDA virtual display driver and a tray icon. Linux: `spout-host-v*-linux-amd64.tar.gz` (run `install.sh`).

Merging a PR into `main` publishes a release automatically. `MAJOR.MINOR` is set by hand in the `VERSION` file; the patch number is auto-incremented per merge (e.g. `0.1.0`, `0.1.1`, ...). Edit `VERSION` in a PR to start a new minor/major. Add the `no-release` label to a PR to skip releasing.

## Apps

The **Apps** tab of the web UI adds programs to this PC's Steam library (as non-Steam shortcuts), so they can be streamed like any other game. The Decky plugin can list them from the host.

- Steam is controlled through its local debug port. Click **Enable Steam integration** once, then restart Steam.
- Adding, removing and deleting apps works only from this PC (loopback), because it runs programs on the host. Paired devices can only read the list.
- Optional: save a free [SteamGridDB](https://www.steamgriddb.com/profile/preferences/api) API key in the Apps tab and new shortcuts get a matching cover, hero, logo and icon. Setting artwork on Steam is verified on Linux; the SteamGridDB lookup itself is only unit tested against a fake server.
- Verified on Linux Steam: add and remove show up in the library. Whether the Steam Link client lists them is not yet verified.

## Pairing

Same flow as ArtLight/ArtMoon: the client shows a 4-digit PIN, the host raises a notification (Windows toast or Linux `notify-send`; clicking it opens the UI), and you type the PIN into the host UI. Only a salted hash of the PIN leaves the client, requests expire after 2 minutes and are cancelled after 5 wrong PINs, and confirming/revoking is only possible from this PC. Each device gets its own revocable token (`paired.json` stores hashes only). The notification code is untested on Windows.

## Credits

This project is based on the ideas and work of others, and I'm grateful to them:

- [Moonlight](https://moonlight-stream.org/) – the open-source game streaming client that started it all.
- [StreamLight](https://github.com/FoggyBytes/StreamLight) – a Moonlight fork with deeper host integration.
- [ArtMoon](https://github.com/onaiaku/ArtMoon) and [ArtLight](https://github.com/onaiaku/ArtLight) – the gamepad-first client and its one-installer host. The virtual-monitor design, the pairing flow and the Linux driver choice follow ArtLight.
- [SudoVDA](https://github.com/SudoMaker/SudoVDA) (MIT) – the Windows virtual display driver.
- [libvirtualdisplay](https://github.com/Nonary/libvirtualdisplay) (MIT, GPL-2.0 module) – the Linux `vibeshine_drm` virtual display module, itself derived from the Linux kernel's VKMS.

SpoutRemotePlayHost is an independent project and is not affiliated with any of them or with Valve.
