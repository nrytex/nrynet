# Nrynet acceptance matrix

This matrix turns the PRD into testable product behavior. A checkbox is only
closed when its implementation has an automated test or a recorded manual
end-to-end verification.

## V1.0 foundation

- [ ] The server runs on Linux and Windows from YAML configuration.
- [x] First start creates a local administrator, a one-time password, and a server secret.
- [x] An administrator can log in to the dashboard without a cloud account.
- [x] Tokens authenticate agents and can be created, disabled, and deleted.
- [ ] A Go agent runs on Windows, Linux, and macOS and maintains a heartbeat.
- [x] A TCP tunnel exposes a configured public port and relays bidirectional bytes.
- [x] Agents can choose WS/plaintext or WSS/TLS control and data pairs, and invalid credentials are rejected.
- [x] P2P datagrams are encrypted with an authenticated, replay-protected session cipher.
- [x] The dashboard shows server health, online clients, tunnels, connections, bandwidth, and daily traffic.

## V1.1 management

- [x] Administrators can inspect, rename, disable, delete, and reset a client token.
- [x] Administrators can create, edit, start, stop, and delete tunnels.
- [x] Tunnel changes reach an online agent without restarting it.
- [x] Agents can use only administrator-assigned tunnels, and stopped tunnels refuse new visitors.
- [x] Traffic is tracked for server, client, and tunnel over today and current month.
- [x] Server logs can be searched, downloaded, and cleared.
- [x] IP allowlists are enforced before relaying a visitor.

## V1.1 HTTP capabilities

- [x] HTTP host routing maps a domain to an agent's local service.
- [x] WebSocket upgrades pass through without protocol corruption.
- [x] HTTPS SNI routing passes encrypted TLS streams through unchanged.

## V1.2 desktop agent

- [ ] Windows and macOS users can configure, connect, disconnect, and inspect the agent in a GUI.
- [x] The GUI reports connection, tunnel, and transfer status without exposing configuration files.
- [x] Updates are downloaded only after version and signature verification.

## V2.0 advanced networking

- [x] UDP tunnels relay independent visitor datagrams and replies.
- [x] QUIC can carry authenticated agent control and data traffic over TLS 1.3.
- [x] A rendezvous service discovers public endpoints and coordinates UDP hole punching.
- [x] Agents fall back to the relay when a direct peer path cannot be established.
- [x] Multiple relay nodes register, heartbeat, receive tunnel assignments, and expose health in the dashboard.

## Operational delivery

- [x] Server and agent binaries have reproducible cross-platform build commands.
- [x] A systemd unit and documented directory layout support `/opt/nrynet`.
- [x] SQLite persists configuration and metrics across server restarts.
- [x] Linux installer can request certbot certificates with key reuse and renewal restart hooks.
- [x] Automated tests cover authentication, storage, protocol framing, and TCP/HTTP/UDP end-to-end paths.

Platform-specific runtime rows remain open until the binaries and GUI are exercised on every named operating system; cross-build and CI configuration alone are not recorded as runtime proof.

Local Windows acceptance on 2026-08-02 exercised the server, CLI agent, and
Wails desktop executable. The desktop UI saved configuration without losing
in-progress edits, authenticated, received a live tunnel snapshot, and relayed
traffic through that tunnel. Linux and macOS remain cross-build/CI evidence
only because native runners are unavailable on this Windows host.
