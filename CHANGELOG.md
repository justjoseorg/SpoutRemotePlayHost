# Changelog

Minor releases (`X.Y.0`) get a written entry here, which becomes their release notes. Patch releases (`X.Y.Z`) are listed on the [Releases](https://github.com/justjoseorg/SpoutRemotePlayHost/releases) page with the change that produced them. Each entry covers everything since the previous minor release.

## 0.2

Released 2026-10-09.

### Windows

- **Works alongside ArtLight and Apollo.** The installer reuses an existing SudoVDA driver and never removes one it didn't install.
- **The virtual monitor becomes the primary display** during a session, so Steam streams it.
- **Other displays turn off during a session.** Pick the ones to keep on under Monitor config → "Displays during a session". The previous layout comes back when the session ends, and stays enforced if Windows turns a display back on mid-session (e.g. a monitor that was off sleeps and reconnects).
- **Session end detected from Steam's log** ("Encoding complete"), as well as from the plugin.
- **Installer:** options to start automatically at sign-in and to allow Decky to scan this PC on the local network (firewall rule); asks for admin; closes the running host and replaces the previous install's files on upgrade.
- **Visible startup and errors:** launching the host again opens its web UI; errors are shown in a message box and logged to `%APPDATA%\SpoutRemotePlayHost\spout-host.log`.

### All platforms

- **Only paired devices get a virtual monitor.** Sessions from unpaired clients are logged and ignored.
- **Steam Link devices (e.g. Android)** can have their own virtual monitor config, from the Devices tab's "Other Steam devices".
- **Apps tab:** add programs to this PC's Steam library as non-Steam shortcuts, with optional SteamGridDB artwork. Paired devices can read the list.
- **No codec setting any more:** Steam negotiates the codec itself.
- **Monitor config tab** (formerly Monitor defaults), with the defaults and the display allowlist.
- **App icons** in the exe, the installers, the Start Menu shortcut and the Linux launcher.

### Linux

- **Virtual display** via the `vibeshine_drm` module (DKMS), and **session detection** from Steam's log.
- **install.sh:** no reboot needed on Fedora (headers for the running kernel come from Koji), MOK enrollment under Secure Boot, picks the connected virtual connector, and gives `kscreen-doctor` a Wayland environment when the service starts before the desktop session.

## 0.1

Released 2026-10-09. First release.

- Web UI and API on port 47995, with a tray icon that opens it.
- PIN pairing with per-device tokens and host notifications.
- Per-device virtual monitor config (resolution, refresh rate), re-applied when it changes during a session.
- Windows: SudoVDA virtual display backend, installer with the driver, and Ctrl+Alt+Shift+Q to remove the virtual monitor.
- Linux: install script with a user systemd service.