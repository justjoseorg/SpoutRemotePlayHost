# Changelog

Minor releases (`X.Y.0`) get a written entry here, which becomes their release notes. Patch releases (`X.Y.Z`) are listed on the [Releases](https://github.com/justjoseorg/SpoutRemotePlayHost/releases) page with the change that produced them. Each entry covers everything since the previous minor release.

## 0.5

### Windows

- **Spout Sign-In** (not yet tested on a device): an optional system service, chosen in Setup, that lets a paired device type your Windows PIN at the sign-in screen. Together with the Decky plugin's numpad, a PC woken with Wake-on-LAN can be signed in remotely so Steam can start. It only types while the sign-in screen shows and never stores the PIN. See [Sign in after Wake-on-LAN](https://github.com/justjoseorg/SpoutRemotePlayHost#-sign-in-after-wake-on-lan-windows).
- **Spout Sign-In reports a locked PC**, so the Decky plugin offers the numpad after Win+L too, not only after a fresh boot. *(0.5.1)*
- **Spout Sign-In fixes from the first device test:** it presses Space instead of Ctrl to lift the lock screen picture, and it detects a locked PC by the lock screen itself, so a signed-in PC no longer shows "not signed in". *(0.5.2)*
- **Spout Sign-In presses Enter** to lift the lock screen picture (Space didn't), after clearing the PIN box so a half-typed PIN is never submitted. **The host starts Steam** if it isn't running 30 seconds after sign-in, so a PC signed in from the plugin can stream. *(0.5.3)*
- **Spout Sign-In presses Enter again** if the sign-in screen still has the keyboard 2 seconds after typing, for when the lock screen picture took the first Enter. **The host starts Steam 5 seconds after sign-in** instead of 30, since Steam's own autostart can be slow. *(0.5.4)*
- **Setup skips the lock screen picture when Spout Sign-In is ticked** (Windows' "Do not display the lock screen" policy). A test showed the picture stays on top of a typed PIN and takes the Enter, so the PIN was never submitted. Removing Sign-In turns the policy back off. *(0.5.5)*

## 0.4

Released 2026-10-09.

### Windows

- **Handles stream reconnects** (not yet tested on a device). If the stream drops and Steam reconnects, Steam removes its virtual display and adds a new one. The host now follows it to the new display and turns the other displays off again. Before, it kept watching the old display, so the other displays were not turned off again. While no virtual display exists, Windows keeps a physical display on and the host leaves it alone.
- **Longer wait before restoring the layout (90 s).** A dropped stream can take Steam about a minute to reconnect, so the session is kept that long. The physical displays still come back on as soon as Steam removes its display; only the exact layout restore waits.

### Linux

- **Listens on the LAN** (`0.0.0.0:47995`) in the installed service, so the Decky plugin can reach it. Requests from other machines still need the token.

## 0.3

Released 2026-10-09.

### Windows

- **Steam creates the virtual display now.** Steam's Remote Play (client beta, with SudoVDA as its display device) adds a virtual display sized to the device. The host no longer creates its own; it waits up to 20 seconds for Steam's display, makes it the only active one (except the displays you keep on), and restores your layout when the session ends. If Steam's display never shows up, your displays are left on. See [Steam setup](https://github.com/justjoseorg/SpoutRemotePlayHost#-steam-setup-windows).
- **Recognises Steam's display by SudoVDA's monitor ID** (`SMK`), since Steam names it after the client device.
- The per-device resolution and refresh settings now only apply to Linux hosts; `/api/discover` reports the host OS so the plugin can hide them for Windows.

### Project

- Written changelog for minor releases, used as their release notes; patch releases keep the change title.
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