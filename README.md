# NAT-Link

NAT-Link is a self-hosted NAT traversal system made of a Go server, a Go agent,
an embedded React dashboard, and a desktop agent. The implementation follows
`NAT-Link PRD.pdf` and targets Linux, Windows, and macOS.

## Components

- `server`: Gin control API, embedded React dashboard, SQLite storage, client hub, and relay runtimes.
- `client`: cross-platform Go agent with heartbeat, automatic reconnect, and local service relays.
- `web/dashboard`: React, TypeScript, Vite, and Ant Design administration console.
- `desktop`: Windows and macOS desktop agent built on the reusable Go agent core.

## Quick start

1. Copy `config.example.yaml` to `config.yaml` and set
   `server.public_data_address` to an address reachable by agents.
2. Start the server with `go run ./server -config config.yaml`.
3. Record the one-time administrator password printed on first start, then open
   `http://127.0.0.1:7000`.
4. Create an agent token in the Tokens page.
5. Put the token in the client section of `config.yaml`, then run
   `go run ./client -config config.yaml`.
6. Create and start a TCP, HTTP, HTTPS, or UDP tunnel in the dashboard.

TLS should be enabled for any deployment outside a trusted development network.
See `docs/operations.md` for certificates, systemd, Docker, ports, and builds.
See `docs/requirements.md` for the versioned acceptance matrix.

## Test

```sh
go test ./...
cd web/dashboard && npm ci && npm test && npm run build
```

The integration suite starts real local agents and services and verifies TCP,
HTTP Host, WebSocket, HTTPS SNI, and advanced transport paths end to end.
