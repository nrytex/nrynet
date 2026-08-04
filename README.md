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
   evaluation. `config.example.yaml` listens on `0.0.0.0` with WSS/TLS ports;
   WS/plaintext ports stay disabled until explicitly configured.
2. Start the server with `go run ./server -config config.yaml`.
3. Record the one-time administrator password printed on first start, then open
   `http://127.0.0.1:7000`.
4. Create an agent token in the Tokens page.
5. Put the token in the client section of `config.yaml`, then run
   `go run ./client -config config.yaml`.
6. Create and start a TCP, HTTP, HTTPS, or UDP tunnel in the dashboard.

For WSS agents, use `wss://host:7000/agent/connect` with data port `7001`.
For WS agents, use `ws://host:7004/agent/connect` with plaintext data port
`7005` after `server.plain_enabled` is enabled. The dashboard can change this
setting and it takes effect after restarting the server. See `docs/operations.md`
for certificates, certbot, systemd, ports, and builds.
中文安装与生产部署步骤见 `docs/deployment.zh-CN.md`。
See `docs/requirements.md` for the versioned acceptance matrix.

## One-command server install

Linux systemd hosts can install the latest release and generate a self-signed
TLS certificate with OpenSSL in one command. The installer registers,
enables, and starts the `nrynet-server` systemd service:

```sh
curl -fLO https://github.com/nrytex/nrynet/releases/latest/download/install-server.sh
chmod +x install-server.sh
sudo ./install-server.sh --public-host nat.example.com
```

For a Let's Encrypt certificate, run:

```sh
sudo ./install-server.sh --certbot-domain nat.example.com --certbot-email admin@example.com
```

To preset domain WSS and explicit IP WS at the same time, run:

```sh
sudo ./install-server.sh --certbot-domain nat.example.com --certbot-email admin@example.com --enable-ws
```

Add `--proxy http://127.0.0.1:7890` or `--proxy socks5h://127.0.0.1:1080`
when dependencies and release downloads must use a proxy. The Windows installer
accepts both `--proxy URL` and its native PowerShell spelling, `-Proxy URL`.
`--enable-ws` and `-EnableWS` are only installation presets; administrators can
also enable or disable plaintext WS in the dashboard and restart the service.

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
