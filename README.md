# SpigotRemotePlay Companion

Host-side companion for the [SpigotRemotePlay](https://github.com/justjoseorg/SpigotRemotePlay) Decky plugin. Targets Windows and Linux (x86_64).

## Goal

When a Steam Remote Play session starts, create/activate a dedicated virtual monitor for the stream so the physical displays are left alone; tear it down when the session ends.

## Status

Scaffold only. Nothing here creates a virtual display yet.

## Planned design

- `internal/display`: `Manager` interface (`Create`, `Destroy`) with per-OS implementations.
  - Windows: virtual display driver (IddCx-based) plus CCD API for mode/topology.
  - Linux: to be decided (KMS/EDID override or compositor-specific).
- Session detection: watch for the Steam Remote Play streaming process/connection.
- Optional control channel for the plugin (status, resolution matching the client).

## Build

```bash
go build ./cmd/spigot-companion
GOOS=windows GOARCH=amd64 go build ./cmd/spigot-companion
```
