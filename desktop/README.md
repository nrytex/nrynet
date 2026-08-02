# NAT-Link Desktop

Wails v3 desktop client for NAT-Link. The Go module is intentionally isolated
under `desktop/` and uses:

```go
replace github.com/nat-link/nat-link => ../
```

so the GUI can reuse the real `client/agent` runtime without changing the root
module.

## Features

- Onboarding/config editor for control WebSocket, data address, token, device
  name, device ID, TLS skip flag, updater manifest, updater public key and
  channel.
- Real connect/disconnect using `client/agent`.
- Status, tunnel table and in-memory runtime log views.
- Windows/macOS tray menu plus hide/show behavior.
- Open-at-login setting: Windows `HKCU\...\Run`, macOS LaunchAgent.
- Wails updater backed by a self-hosted Wails endpoint manifest. It compares
  versions, downloads to the Wails staging area, verifies SHA-256 digest and
  Ed25519 signatures using the configured public key, then uses the Wails
  updater helper for replacement/restart.

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

`go test ./...` includes a signed update verification test that serves a Wails
manifest and artifact from `httptest`, signs the SHA-256 digest with Ed25519,
and asserts Wails stages the verified artifact.

## Cross-Platform Builds

Windows local build:

```powershell
cd desktop
wails3 build
```

macOS build on a macOS host:

```bash
cd desktop
wails3 build GOOS=darwin
```

Wails desktop builds rely on each platform's native WebView/toolchain, so
production Windows and macOS artifacts should be produced on their matching
host or a Wails-supported cross-build environment.
