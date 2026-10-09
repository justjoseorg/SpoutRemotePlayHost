# SpoutRemotePlay Host

Host-side app for the [SpoutRemotePlay](https://github.com/justjoseorg/SpoutRemotePlay) Decky plugin. Targets Windows and Linux (x86_64).

Supported architecture: x86_64 only (Windows and Linux); no ARM builds.

## The idea

**Pick up your handheld, press play on a game from your PC, and go.**

This is an integrated solution with a narrow purpose: **the virtual monitor, integrated with Steam's native Remote Play.** It does not stream anything itself and does not replace Steam. Steam keeps doing the streaming (capture, encode, input, Steam Link on the handheld); this app only watches for a Remote Play session and gives it a dedicated virtual monitor matching the connecting device (its resolution and refresh rate), then removes it when the session ends. While the session runs the virtual monitor is the primary display, so Steam streams it. On Windows every other display is turned off during the session, except the ones ticked under Monitor config → "Displays during a session"; the previous layout is restored when it ends. Linux doesn't turn displays off yet.

Pair each device once, give it its own monitor settings, and from then on it just works. Together with the [Decky plugin](https://github.com/justjoseorg/SpoutRemotePlay) (Wake-on-LAN, pairing, settings), the flow is: wake the PC, press play in Steam, stream.

## How it knows a stream started

Two independent signals, so it doesn't depend on one fragile hook:

- **From the handheld:** the plugin reports Steam's Remote Play start/stop to every paired PC (`POST /api/session`, authenticated with the device's own token). Verified with an AYN Odin 2 Portal streaming Celeste.
- **From the PC:** the host tails Steam's `streaming_log.txt` ("Streaming started to <device>…"; the end is "PipeWire: Deinitializing streaming" on Linux and "Encoding complete" on Windows). On Windows it was seen ending the monitor after a real session.

Only **paired** devices get a virtual monitor, on every platform: a session from an unpaired client (or a plugin call with the API token instead of a device token) is logged and ignored. The monitor is created from that device's config, is kept across a quick stream restart (5 s grace), and is only removed by the device that owns it.

## Install

- **Windows:** run `SpoutRemotePlayHost-Setup-vX.Y.Z.exe` from Releases (not the bare `spout-host-*-windows-amd64.exe`, which is a portable build without driver or autostart). The installer has these options:
  - **SudoVDA virtual display driver** component: if a SudoVDA is already installed (e.g. by [ArtLight](https://github.com/onaiaku/ArtLight) or Apollo) it is reused and left untouched, so both apps work side by side, and uninstalling SpoutRemotePlayHost never removes it. Otherwise it installs a SudoVDA built from [SudoMaker/SudoVDA](https://github.com/SudoMaker/SudoVDA) (MIT) in CI and signed with a self-signed certificate, which is added to the Windows Trusted Root and Trusted Publishers stores; that driver and certificate are removed on uninstall unless another app has replaced the driver since. Untick it to install the host only.
  - **Start automatically when I sign in**: a logon task runs the host (`-background`), also on battery and without a time limit.
  - **Allow Decky to scan this PC on the local network**: the host listens on `0.0.0.0:47995` and a firewall rule allows it on all network profiles, since home networks are often classified as Public (requests from other machines still need a token).
  Launching the host while it already runs just opens its web UI; launched by hand it opens the UI at startup. Since the exe has no console, errors are shown in a message box and logged to `%APPDATA%\SpoutRemotePlayHost\spout-host.log`. If another app already registered Ctrl+Alt+Shift+Q, the hotkey is disabled (logged) and the host runs normally. The installer is untested.
- **Linux:** extract `spout-host-vX.Y.Z-linux-amd64.tar.gz` and run `./install.sh` (user systemd service; `--uninstall` removes it). It also sets up the driver with DKMS: it installs `dkms` and kernel headers for you (Fedora/Arch/Debian/Ubuntu/openSUSE), and on Fedora it fetches headers for the exact running kernel from Koji so no reboot is needed (only as a last resort does it build for the newest installed kernel and ask for a reboot). It needs sudo and a sudoers rule limited to `/usr/local/libexec/spout-vdisplay`; pass `--no-driver` to skip.

## Build

```bash
go build ./cmd/spout-host
GOOS=windows GOARCH=amd64 go build ./cmd/spout-host
```

## Releases

Windows: download `SpoutRemotePlayHost-Setup-v*.exe` from the latest release. It installs the host and a tray icon, and the SudoVDA virtual display driver unless one is already installed (see Install). Linux: `spout-host-v*-linux-amd64.tar.gz` (run `install.sh`).

## Android and other Steam Link devices

No Android app is needed: Steam Link already streams. The Devices tab has an **Other Steam devices** section listing the devices Steam knows (read from the running Steam client), where you set a virtual monitor for each (resolution, refresh, auto-create). When a stream starts, the host matches the client name in Steam's log to that device. The device list is verified against the real Steam on Linux; an actual Android stream has not been tried yet. Pair the device in Steam first. The list also shows other PCs Steam can see, so ignore those.

## Apps

The **Apps** tab of the web UI adds programs to this PC's Steam library (as non-Steam shortcuts), so they can be streamed like any other game. The Decky plugin can list them from the host.

- Steam is controlled through its local debug port. Click **Enable Steam integration** once, then restart Steam.
- Adding, removing and deleting apps works only from this PC (loopback), because it runs programs on the host. Paired devices can only read the list.
- Optional: save a free [SteamGridDB](https://www.steamgriddb.com/profile/preferences/api) API key in the Apps tab and new shortcuts get a matching cover, hero, logo and icon. Setting artwork on Steam is verified on Linux; the SteamGridDB lookup itself is only unit tested against a fake server.
- Verified on Linux Steam: add and remove show up in the library. Whether the Steam Link client lists them is not yet verified.

## Pairing

The client shows a 4-digit PIN, the host raises a notification (Windos & Linux), and you type the PIN into the host UI. Requests expire after 2 minutes and are cancelled after 5 wrong PINs, and confirming/revoking is only possible from the host PC.
## Credits

This project is based on the ideas and work of others, and I'm grateful to them:

- [Moonlight](https://moonlight-stream.org/) – the open-source game streaming client that started it all.
- [StreamLight](https://github.com/FoggyBytes/StreamLight) – a Moonlight fork with deeper host integration.
- [ArtMoon](https://github.com/onaiaku/ArtMoon) and [ArtLight](https://github.com/onaiaku/ArtLight) – the gamepad-first client and its one-installer host. The virtual-monitor design, the pairing flow and the Linux driver choice follow ArtLight.
- [SudoVDA](https://github.com/SudoMaker/SudoVDA) (MIT) – the Windows virtual display driver.
- [libvirtualdisplay](https://github.com/Nonary/libvirtualdisplay) (MIT, GPL-2.0 module) – the Linux `vibeshine_drm` virtual display module, itself derived from the Linux kernel's VKMS.

SpoutRemotePlayHost is an independent project and is not affiliated with any of them or with Valve.
