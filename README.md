<p align="center">
  <img src="installer/linux/spout-remote-play.png" alt="Spout Remote Play" width="128">
</p>

<h1 align="center">SpoutRemotePlay Host</h1>

<p align="center">
  <b>Pick up your handheld, press play on a game from your PC, and go.</b><br>
  Complements Steam's native Remote Play: wakes your PC and keeps the stream on a virtual monitor, with your other displays off.
</p>

<p align="center">
  <a href="https://github.com/justjoseorg/SpoutRemotePlayHost/releases/latest"><img alt="Latest release" src="https://img.shields.io/github/v/release/justjoseorg/SpoutRemotePlayHost?label=latest&color=2ea44f"></a>
  <a href="https://github.com/justjoseorg/SpoutRemotePlayHost/releases"><img alt="Downloads" src="https://img.shields.io/github/downloads/justjoseorg/SpoutRemotePlayHost/total"></a>
  <img alt="Platforms" src="https://img.shields.io/badge/platform-Windows%20%7C%20Linux-blue">
  <img alt="Architecture" src="https://img.shields.io/badge/arch-x86__64-lightgrey">
  <a href="https://ko-fi.com/justjose"><img alt="Ko-fi" src="https://img.shields.io/badge/Ko--fi-support-FF5E5B?logo=ko-fi&logoColor=white"></a>
</p>

<p align="center">
  <a href="https://github.com/justjoseorg/SpoutRemotePlayHost/releases/latest"><b>⬇️ Download the latest release</b></a>
  &nbsp;·&nbsp;
  <a href="https://github.com/justjoseorg/SpoutRemotePlay">🎮 Decky plugin</a>
  &nbsp;·&nbsp;
  <a href="CHANGELOG.md">📝 Changelog</a>
</p>

---

## Contents

- [✨ What it does](#-what-it-does)
- [📥 Download and install](#-download-and-install)
- [🔐 Pairing](#-pairing)
- [📺 Monitor config](#-monitor-config)
- [📡 How it knows a stream started](#-how-it-knows-a-stream-started)
- [📱 Android and other Steam Link devices](#-android-and-other-steam-link-devices)
- [🧩 Apps](#-apps)
- [🔧 Build](#-build)
- [🙏 Credits](#-credits)

## ✨ What it does

This app complements Steam's native Remote Play and makes it better. It does not stream anything itself and does not replace Steam: Steam keeps doing the streaming (capture, encode, input, Steam Link on the handheld). This app watches for a Remote Play session and makes sure the stream lands on a virtual monitor, with your other displays off, then puts everything back when the session ends.

- **Windows:** Steam creates the virtual display itself (with the SudoVDA driver) and sizes it to the device. The host waits for that display, turns the other displays off and restores them afterwards. See [Steam setup](#-steam-setup-windows).
- **Linux:** Steam doesn't create a virtual display there, so the host creates one sized to the device and removes it afterwards.

| | |
|---|---|
| 🖥️ **Virtual monitor per device** | On Linux, resolution and refresh rate come from that device's config. On Windows, Steam sizes its own virtual display. |
| ⭐ **Primary while streaming** | The virtual monitor is the primary display during the session, so Steam streams it. |
| 🌑 **Other displays off** (Windows) | Every other display is turned off during the session, except the ones you keep on. The previous layout is restored when it ends. Linux doesn't turn displays off yet. |
| 🔐 **Paired devices only** | Unpaired clients never get a monitor. |
| 🧩 **Apps** | Add programs to Steam from the web UI, with optional SteamGridDB artwork. |

Pair each device once, give it its own monitor settings, and from then on it just works. Together with the [Decky plugin](https://github.com/justjoseorg/SpoutRemotePlay) (Wake-on-LAN, pairing, settings), the flow is: **wake the PC → press play in Steam → stream.**

> **Why "Spout"?** It's a play on **Valve**: a valve controls the flow of steam, and a spout is where the steam comes out. Valve's Steam does the streaming; Spout just gives the stream somewhere to pour, a virtual monitor shaped for your handheld.

## 📥 Download and install

Get the files from the **[latest release](https://github.com/justjoseorg/SpoutRemotePlayHost/releases/latest)**:

| Platform | File | |
|---|---|---|
| 🪟 Windows | `SpoutRemotePlayHost-Setup-vX.Y.Z.exe` | Installer (recommended) |
| 🪟 Windows | `spout-host-vX.Y.Z-windows-amd64.exe` | Portable, without driver or autostart |
| 🐧 Linux | `spout-host-vX.Y.Z-linux-amd64.tar.gz` | Run `./install.sh` |

### 🪟 Windows

Run the Setup (not the bare `spout-host-*-windows-amd64.exe`). It installs the host, a tray icon and a Start Menu shortcut, and has these options:

- **SudoVDA virtual display driver.** If a SudoVDA is already installed (e.g. by [ArtLight](https://github.com/onaiaku/ArtLight) or Apollo) it is reused and left untouched, so both apps work side by side, and uninstalling SpoutRemotePlayHost never removes it. Otherwise it installs a SudoVDA built from [SudoMaker/SudoVDA](https://github.com/SudoMaker/SudoVDA) (MIT) in CI and signed with a self-signed certificate, which is added to the Windows Trusted Root and Trusted Publishers stores; that driver and certificate are removed on uninstall unless another app has replaced the driver since. Untick it to install the host only.
- **Start automatically when I sign in.** A logon task runs the host (`-background`), also on battery and without a time limit.
- **Allow Decky to scan this PC on the local network.** The host listens on `0.0.0.0:47995` and a firewall rule allows it on all network profiles, since home networks are often classified as Public (requests from other machines still need a token).

Good to know:

- Launching the host while it already runs just opens its web UI; launched by hand it opens the UI at startup.
- The exe has no console: errors are shown in a message box and logged to `%APPDATA%\SpoutRemotePlayHost\spout-host.log`.
- Ctrl+Alt+Shift+Q (Moonlight's quit-stream shortcut) ends the session's display handling and brings your displays back. If another app already registered it, the hotkey is disabled (logged) and the host runs normally.

### 🎮 Steam setup (Windows)

Steam creates its own virtual display for Remote Play using the SudoVDA driver the installer provides. The menu paths below follow public descriptions of the Steam client beta, so Steam's wording may differ:

1. Install the host with the **SudoVDA virtual display driver** option ticked (or keep an existing SudoVDA).
2. In Steam, join the client beta: **Steam > Settings > Interface > Client Beta Participation**, then restart Steam.
3. In **Steam > Settings > Remote Play > Advanced Host Options**, pick SudoVDA as the virtual display device if Steam offers it (Steam saves this as `CustomDisplayDevice "SUDOVDA"` in its config).
4. Stream from your handheld. The host waits up to 20 seconds for Steam's display (Steam names it after the device; the host recognises it by SudoVDA's monitor ID, `SMK`), turns the other displays off and restores them when the session ends. If Steam's display never appears, your displays are left on.

Tested on Windows 11 with an Odin 2 Portal on the Steam client beta: Steam's display was recognised, the other displays turned off during the stream, and the layout was restored afterwards.

### 🐧 Linux

Extract `spout-host-vX.Y.Z-linux-amd64.tar.gz` and run `./install.sh` (user systemd service; `--uninstall` removes it). It also sets up the driver with DKMS:

- It installs `dkms` and kernel headers for you (Fedora/Arch/Debian/Ubuntu/openSUSE).
- On Fedora it fetches headers for the exact running kernel from Koji, so no reboot is needed (only as a last resort does it build for the newest installed kernel and ask for a reboot).
- It needs sudo and a sudoers rule limited to `/usr/local/libexec/spout-vdisplay`; pass `--no-driver` to skip.

The Linux backend uses the `vibeshine_drm` kernel module (the driver ArtLight uses) and `kscreen-doctor`, so it needs **KDE Plasma on Wayland** and **Linux 6.16+**.

## 🔐 Pairing

1. In the Decky plugin, pick this PC.
2. The client shows a 4-digit PIN and the host raises a notification (Windows and Linux).
3. Type the PIN into the host's web UI.

Requests expire after 2 minutes and are cancelled after 5 wrong PINs. Confirming and revoking is only possible from the host PC.

## 📺 Monitor config

The **Monitor config** tab of the web UI has:

- **Defaults:** the virtual monitor used by devices without their own settings (Linux only; on Windows Steam sizes its own display). Each device can override it from the Devices tab or from the plugin.
- **Displays during a session** (Windows): tick the displays to keep on while streaming. Unticked ones are turned off and come back when the session ends. If Windows turns a display back on mid-session (e.g. a monitor that was off goes to sleep and reconnects), the host turns it off again.

There is no codec setting: Steam Remote Play negotiates the codec itself, and PyroWave is not available with Steam streaming.

## 📡 How it knows a stream started

Two independent signals, so it doesn't depend on one fragile hook:

- **From the handheld:** the plugin reports Steam's Remote Play start/stop to every paired PC (`POST /api/session`, authenticated with the device's own token).
- **From the PC:** the host tails Steam's `streaming_log.txt` ("Streaming started to <device>…"; the end is "PipeWire: Deinitializing streaming" on Linux and "Encoding complete" on Windows).

Only **paired** devices get a virtual monitor, on every platform: a session from an unpaired client (or a plugin call with the API token instead of a device token) is logged and ignored. The monitor is created from that device's config, is kept across a quick stream restart (5 s grace), and is only removed by the device that owns it.

## 📱 Android and other Steam Link devices

No Android app is needed: Steam Link already streams. The Devices tab has an **Other Steam devices** section listing the devices Steam knows (read from the running Steam client), where you set a virtual monitor for each (resolution, refresh, auto-create). When a stream starts, the host matches the client name in Steam's log to that device.

- Pair the device in Steam first.
- The list also shows other PCs Steam can see; ignore those.
- The device list is verified against the real Steam on Linux; an actual Android stream has not been tried yet.

## 🧩 Apps

The **Apps** tab adds programs to this PC's Steam library (as non-Steam shortcuts), so they can be streamed like any other game. The Decky plugin can list them from the host.

- **Steam integration:** Steam is controlled through its local debug port. Click **Enable Steam integration** once, then restart Steam.
- **Local only:** adding, removing and deleting apps works only from this PC (loopback), because it runs programs on the host. Paired devices can only read the list.
- **Artwork (optional):** save a free [SteamGridDB](https://www.steamgriddb.com/profile/preferences/api) API key and new shortcuts get a matching cover, hero, logo and icon. Setting artwork on Steam is verified on Linux; the SteamGridDB lookup itself is only unit tested against a fake server.
- Verified on Linux Steam: add and remove show up in the library. Whether the Steam Link client lists them is not yet verified.

## 🔧 Build

```bash
go build ./cmd/spout-host
GOOS=windows GOARCH=amd64 go build ./cmd/spout-host
```

Releases are built by CI when a pull request is merged into `main`.

## 🙏 Credits

This project is based on the ideas and work of others, and I'm grateful to them:

- [Moonlight](https://moonlight-stream.org/) – the open-source game streaming client that started it all.
- [StreamLight](https://github.com/FoggyBytes/StreamLight) – a Moonlight fork with deeper host integration.
- [ArtMoon](https://github.com/onaiaku/ArtMoon) and [ArtLight](https://github.com/onaiaku/ArtLight) – the gamepad-first client and its one-installer host. The virtual-monitor design, the pairing flow and the Linux driver choice follow ArtLight.
- [SudoVDA](https://github.com/SudoMaker/SudoVDA) (MIT) – the Windows virtual display driver.
- [libvirtualdisplay](https://github.com/Nonary/libvirtualdisplay) (MIT, GPL-2.0 module) – the Linux `vibeshine_drm` virtual display module, itself derived from the Linux kernel's VKMS.

SpoutRemotePlayHost is an independent project and is not affiliated with any of them or with Valve.

---

<p align="center">
  If this is useful to you, you can <a href="https://ko-fi.com/justjose">☕ buy me a coffee on Ko-fi</a>.
</p>
