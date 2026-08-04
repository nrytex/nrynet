# Nrynet

Nrynet is a self-hosted NAT traversal system made of a Go server, a Go agent,
an embedded React dashboard, and a desktop agent targeting Linux, Windows,
and macOS.

## Components

- `server`: Gin control API, embedded React dashboard, SQLite storage, client hub, and relay runtimes.
- `client`: cross-platform Go agent with heartbeat, automatic reconnect, and local service relays.
- `web/dashboard`: React, TypeScript, Vite, and Ant Design administration console.
- `desktop`: Windows and macOS desktop agent built on the reusable Go agent core.

## Quick start

1. Copy `config.local.example.yaml` to `config.yaml` for a loopback-only
   evaluation. `config.example.yaml` listens on `0.0.0.0` with HTTP/WS and
   plaintext data enabled on the primary ports; TLS starts disabled.
2. Start the server with `go run ./server -config config.yaml`.
3. Record the one-time administrator password printed on first start, then open
   `http://127.0.0.1:7000`.
4. Create an agent token in the Tokens page.
5. Put the token in the client section of `config.yaml`, then run
   `go run ./client -config config.yaml`.
6. Create and start a TCP, HTTP, HTTPS, or UDP tunnel in the dashboard.

WS agents use `ws://host:7000/agent/connect` with data port `7001`. After a
domain certificate is enabled, the same ports also accept WSS and TLS data;
HTTP/WS continues to work. The Dashboard can request Let's Encrypt certificates,
toggle TLS, and hot-load renewed certificates without restarting Nrynet. See
`docs/operations.md` for certificate, systemd, port, and build details.
中文安装与生产部署步骤见 `docs/deployment.zh-CN.md`。
See `docs/requirements.md` for the versioned acceptance matrix.

## One-command server install

Linux systemd hosts can install the latest release in one command. New installs
start with HTTP/WS, register the `nrynet-server` service, and install a restricted
Certbot helper used by the Dashboard:

```sh
curl -fLO https://github.com/nrytex/nrynet/releases/latest/download/install-server.sh
chmod +x install-server.sh
sudo ./install-server.sh --public-host nat.example.com
```

Open `http://host:7000`, then bind a domain from **Settings > Access and
Certificates**. DNS must point to the server and inbound TCP/80 must be open.
The older non-interactive installer flow remains available:

```sh
sudo ./install-server.sh --certbot-domain nat.example.com --certbot-email admin@example.com
```

Add `--proxy http://127.0.0.1:7890` or `--proxy socks5h://127.0.0.1:1080`
when dependencies and release downloads must use a proxy. The Windows installer
accepts both `--proxy URL` and its native PowerShell spelling, `-Proxy URL`.
`--enable-ws` and `-EnableWS` only enable the optional legacy `7004/7005`
compatibility pair. The primary `7000/7001` pair already supports HTTP/WS, and
all Dashboard TLS and compatibility switches hot-update without a restart.

Windows Server users can download `install-server.ps1` from the same release
and run it from an elevated PowerShell session. Full instructions, certificate
trust steps, and manual installation options are in `docs/deployment.zh-CN.md`.

## Test

```sh
go test ./...
cd web/dashboard && npm ci && npm test && npm run build
```

The integration suite starts real local agents and services and verifies TCP,
HTTP Host, WebSocket, HTTPS SNI, and advanced transport paths end to end.
