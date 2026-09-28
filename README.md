# Shared Hub

A shared clipboard, files and notes across your computers. One app per computer,
nothing else to install. Sync is peer to peer and end-to-end encrypted, powered by an
embedded [Syncthing](https://syncthing.net) engine.

> **Status: early development.** Sync, the dashboard and the clipboard work. Device
> pairing from the dashboard, installers and a tray icon are next (see the roadmap).

## What it does

- **Clipboard:** paste anywhere (⌘V / Ctrl+V) and it appears on your other devices in about a second.
  Text, links, screenshots and files. Private clips hide until hovered and delete themselves.
- **Files:** browse, preview, upload, rename and delete in the shared folder.
- **Notes:** notes that sync everywhere and save as you type.
- **Phones:** scan a QR code to use the dashboard from a phone on the same Wi-Fi.
- **Mac, Windows and Linux.**

## How it works

```
┌──────────────────── one Shared Hub process ────────────────────┐
│  dashboard (localhost:8765)  ←→  hub API  ←→  shared folder     │
│                                      │                           │
│                    embedded Syncthing engine (lib/syncthing)     │
└──────────────────────────────┬──────────────────────────────────┘
                               │  encrypted, peer to peer (LAN or relays)
                        your other devices
```

- Syncthing runs **inside** the app, imported as a Go library at a pinned release. It gets its
  own identity, settings and ports, so it never clashes with a Syncthing you already run.
- Shared data lives in the folder under `.hub/`. Each device writes only its own file
  (`.hub/clips/<device ID>.json`) and merges the others', so Syncthing never sees
  conflicting edits.
- After every change the hub asks the engine to rescan exactly what changed, so edits go
  out immediately.

## Build

Requires Go 1.26 or newer.

```bash
make            # this machine
make release    # Mac (Apple Silicon + Intel), Windows and Linux (x64 + ARM) into dist/
```

Run it:

```bash
./bin/sharedhub                       # uses ~/Shared Hub and opens the dashboard
./bin/sharedhub -folder ~/Sync -port 8765 -open=false
```

Notes on the build:

- `-tags noassets` leaves out Syncthing's own web UI; Shared Hub is the interface.
- macOS builds use cgo so Syncthing can watch the folder with FSEvents. Windows and Linux
  build without cgo (pure-Go SQLite), so they cross-compile from any machine.

## Project layout

```
cmd/sharedhub/      entry point
internal/engine/    embedded Syncthing: identity, config, folder, lifecycle
internal/hub/       dashboard server: clipboard, notes, files, sync status, per-OS bits
web/                the dashboard UI and icons, compiled into the binary
```

## Roadmap

1. **Pairing:** "Add a device" with a QR code or code, and an Accept prompt on the other side.
2. **Tray / menu bar icon:** open dashboard, pause, quit; start at login.
3. **Installers:** signed `.dmg`, Windows installer, AppImage / `.deb`; Homebrew, winget, Flathub.
4. **Updates:** automatic, including new Syncthing releases.

## Licence

Shared Hub is MIT licensed (see `LICENSE`). It includes Syncthing, licensed under the
Mozilla Public License 2.0, and Microsoft Fluent Emoji icons (MIT). See `NOTICE`.
"Syncthing" is a trademark of the Syncthing Foundation; Shared Hub is not affiliated with it.
