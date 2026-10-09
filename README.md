# SpoutRemotePlay Host

Host-side app for the [SpoutRemotePlay](https://github.com/justjoseorg/SpoutRemotePlay) Decky plugin. Targets Windows and Linux (x86_64).

Supported architecture: x86_64 only (Windows and Linux); no ARM builds.

## The idea

**Pick up your handheld, press play on a game from your PC, and go.**

This repo complements Steam Remote Play and makes it better. It does not stream anything itself and does not replace Steam: Steam keeps doing the streaming (capture, encode, input, Steam Link on the handheld). This app adds the pieces around it:

- **Windows:** Steam creates its own virtual display (SudoVDA) for the session. This app watches for the session and switches off every other monitor so only the virtual display is active, then restores your monitors when the session ends. It never creates the virtual display itself, and it leaves your monitors alone if Steam's display does not show up.
- **Linux:** Steam does not create a virtual display there, so this app still creates one (resolution and refresh of the connecting device) and removes it when the session ends.
- Wake-on-LAN, pairing, per-device settings and an Apps list, together with the Decky plugin.

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
- Windows backend: waits (up to 20 s) for Steam's SudoVDA virtual display to appear, saves the monitor layout, disables the other monitors with `SetDisplayConfig`, and restores the saved layout when the session ends (falling back to "extend" if the saved layout no longer applies). The resolution and refresh set in the UI are ignored on Windows because Steam sizes its display itself. It compiles and its topology logic is unit tested, but it has not been run on a Windows machine.
- Linux backend: uses the `vibeshine_drm` kernel module (the driver ArtLight uses, built via DKMS) and `kscreen-doctor`, so it needs KDE Plasma on Wayland and Linux 6.16+. Unit-tested; not yet run on hardware.
- Hotkey: Ctrl+Alt+Shift+Q (Moonlight's quit-stream shortcut) ends the session's monitor handling: on Windows it restores your physical monitors, on Linux it removes the virtual monitor. Windows only (`RegisterHotKey`); compiles, untested.
- Tray icon (Windows and Linux): click it, or choose "Open Spout Remote Play Host", to open the web UI. Run with `-no-tray` to disable. Linux needs a StatusNotifier-capable panel (KDE has one; GNOME needs the AppIndicator extension). Verified to register on KDE only; the Windows tray is untested.

## Install

- **Windows:** run `SpoutRemotePlayHost-Setup-vX.Y.Z.exe` from Releases (set up Steam as described below). The optional "SudoVDA virtual display driver" component is built from [SudoMaker/SudoVDA](https://github.com/SudoMaker/SudoVDA) (MIT) in CI and signed with a self-signed certificate; installing it adds that certificate to the Windows Trusted Root and Trusted Publishers stores (removed on uninstall). Untested.
- **Linux:** extract `spout-host-vX.Y.Z-linux-amd64.tar.gz` and run `./install.sh` (user systemd service; `--uninstall` removes it). It also sets up the driver with DKMS: it installs `dkms` and kernel headers for you (Fedora/Arch/Debian/Ubuntu/openSUSE), and on Fedora it fetches headers for the exact running kernel from Koji so no reboot is needed (only as a last resort does it build for the newest installed kernel and ask for a reboot). It needs sudo and a sudoers rule limited to `/usr/local/libexec/spout-vdisplay`; pass `--no-driver` to skip.

### Steam setup (Windows)

Steam's own virtual display support uses the SudoVDA driver this installer provides. These steps are from public descriptions of Steam's beta and have not been verified on our own machine; Steam's menu wording may differ.

1. Install the host with the **SudoVDA virtual display driver** component ticked.
2. In Steam, opt into the client beta: **Steam > Settings > Interface > Client Beta Participation**, then restart Steam.
3. In **Steam > Settings > Remote Play > Advanced Host Options**, choose the SudoVDA virtual display if Steam offers it.
4. Pair your handheld in Steam and start a stream. The host then switches off your other monitors for the session and restores them afterwards.

## Build

```bash
go build ./cmd/spout-host
GOOS=windows GOARCH=amd64 go build ./cmd/spout-host
```

## Releases

Windows: download `SpoutRemotePlayHost-Setup-v*.exe` from the latest release. It installs the host, the SudoVDA virtual display driver (which Steam uses) and a tray icon. Linux: `spout-host-v*-linux-amd64.tar.gz` (run `install.sh`).

Merging a PR into `main` publishes a release automatically. `MAJOR.MINOR` is set by hand in the `VERSION` file; the patch number is auto-incremented per merge (e.g. `0.1.0`, `0.1.1`, ...). Edit `VERSION` in a PR to start a new minor/major. Add the `no-release` label to a PR to skip releasing.

## Android and other Steam Link devices

No Android app is needed: Steam Link already streams. The Devices tab has an **Other Steam devices** section listing the devices Steam knows (read from the running Steam client), where you set a virtual monitor for each (resolution, refresh, auto-create). When a stream starts, the host matches the client name in Steam's log to that device. The device list is verified against the real Steam on Linux; an actual Android stream has not been tried yet. Pair the device in Steam first. The list also shows other PCs Steam can see, so ignore those.

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
