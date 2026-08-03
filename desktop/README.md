# Nrynet Desktop

[中文说明](README.zh-CN.md)

Wails v3 desktop client for Nrynet. The Go module is intentionally isolated
under `desktop/` and uses:

```go
replace github.com/nrytex/nrynet => ../
```

so the GUI can reuse the real `client/agent` runtime without changing the root
module.

## Features

- Onboarding/config editor for control WebSocket, data address, token, device
  name, device ID and TLS settings.
- Control WebSocket accepts either `ws://` or `wss://`; use `wss://` when the
  deployment requires encrypted transport.
- Real connect/disconnect using `client/agent`.
- Status, tunnel table and in-memory runtime log views.
- Windows/macOS tray menu; closing the main window hides it to the tray, while
  the tray's Quit command exits the application.
- Open-at-login setting: Windows `HKCU\...\Run`, macOS LaunchAgent.
- Wails updater backed directly by public `nrytex/nrynet` GitHub Releases. It
  selects the matching desktop asset and verifies its SHA-256 digest from
  `SHA256SUMS`. It checks every six hours in the background; the Update button
  remains available on demand.

## Development

```powershell
cd desktop
go mod tidy
cd frontend
npm install
npm run build
cd ..
wails3 dev
```

## Verification

```powershell
cd desktop
go test ./...
cd frontend
npm test
npm run build
cd ..
wails3 build
```

`go test ./...` verifies one-time GitHub updater setup and desktop-only asset
matching for Windows and universal macOS packages. Windows builds use the
opaque white-background `build/appicon.png` source to regenerate `build/windows/icon.ico`,
which keeps the taskbar, tray and installer icons free of black corners. The
main window opens at 680x720 with a 560x560 minimum so it fits common laptop
screens while remaining resizable.

## Cross-Platform Builds

Windows local build:

```powershell
cd desktop
$env:APP_VERSION = "1.0.0"
wails3 build
```

macOS build on a macOS host:

```bash
cd desktop
APP_VERSION=1.0.0 wails3 build GOOS=darwin
```

`APP_VERSION` is injected into `main.appVersion` and is the authoritative
version used by update comparison and the status UI. Keep the native package
version in `build/config.yml` aligned for signed production releases.

Wails desktop builds rely on each platform's native WebView/toolchain, so
production Windows and macOS artifacts should be produced on their matching
host or a Wails-supported cross-build environment.
