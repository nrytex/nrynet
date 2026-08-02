# NAT-Link acceptance matrix

This matrix turns the PRD into testable product behavior. A checkbox is only
closed when its implementation has an automated test or a recorded manual
end-to-end verification.

## V1.0 foundation

- [ ] The server runs on Linux and Windows from YAML configuration.
- [x] First start creates a local administrator, a one-time password, and a server secret.
- [ ] An administrator can log in to the dashboard without a cloud account.
- [x] Tokens authenticate agents and can be created, disabled, and deleted.
- [ ] A Go agent runs on Windows, Linux, and macOS and maintains a heartbeat.
- [x] A TCP tunnel exposes a configured public port and relays bidirectional bytes.
- [ ] Agent/server communication supports TLS and rejects invalid credentials.
- [ ] The dashboard shows server health, online clients, tunnels, connections, bandwidth, and daily traffic.

## V1.1 management

- [ ] Administrators can inspect, rename, disable, delete, and reset a client token.
- [ ] Administrators can create, edit, start, stop, and delete tunnels.
- [ ] Tunnel changes reach an online agent without restarting it.
- [ ] Traffic is tracked for server, client, and tunnel over today and current month.
- [ ] Server logs can be searched, downloaded, and cleared.
- [ ] IP allowlists are enforced before relaying a visitor.

## V1.1 HTTP capabilities

- [x] HTTP host routing maps a domain to an agent's local service.
- [x] WebSocket upgrades pass through without protocol corruption.
- [x] HTTPS SNI routing passes encrypted TLS streams through unchanged.

## V1.2 desktop agent

- [ ] Windows and macOS users can configure, connect, disconnect, and inspect the agent in a GUI.
- [ ] The GUI reports connection, tunnel, and transfer status without exposing configuration files.
- [ ] Updates are downloaded only after version and signature verification.

## V2.0 advanced networking

- [ ] UDP tunnels relay independent visitor datagrams and replies.
- [ ] QUIC can carry authenticated agent control and data traffic over TLS 1.3.
- [ ] A rendezvous service discovers public endpoints and coordinates UDP hole punching.
- [ ] Agents fall back to the relay when a direct peer path cannot be established.
- [ ] Multiple relay nodes register, heartbeat, receive tunnel assignments, and expose health in the dashboard.

## Operational delivery

- [ ] Server and agent binaries have reproducible cross-platform build commands.
- [ ] A systemd unit and documented directory layout support `/opt/nat-link`.
- [x] SQLite persists configuration and metrics across server restarts.
- [ ] Automated tests cover authentication, storage, protocol framing, and TCP/HTTP/UDP end-to-end paths.
