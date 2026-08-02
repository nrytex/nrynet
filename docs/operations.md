# NAT-Link operations

## Network ports

The example configuration exposes these server listeners:

| Port | Transport | Purpose |
| --- | --- | --- |
| 7000 | HTTP or HTTPS | Dashboard, administration API, and agent WebSocket control |
| 7001 | TCP or TLS | Per-connection TCP relay data channel |
| 7002 | UDP/QUIC | Authenticated QUIC control and data streams |
| 7003 | UDP | P2P endpoint rendezvous and hole punching |
| 8080 | TCP | Shared HTTP Host and HTTPS SNI gateway |
| 7100 | HTTP or HTTPS | Distributed relay control API (relay node only) |
| tunnel-defined | TCP or UDP | Public visitor ports |

## Distributed relay nodes

A relay node owns the public TCP listener for every tunnel assigned to it. It
forwards each accepted visitor stream to the central broker, which authenticates
the node and then pairs the stream with the assigned agent. The relay never
dials the agent's local service directly.

Set `server.public_data_address` to an address reachable from each relay and
set the same long random `server.relay_api_token` on the server and every node.
It must be distinct from administrator and agent credentials; leaving it empty
disables relay registration. Run a node with
an advertised public address, a separately advertised control address reachable
from the control server, and a local bind host for public visitor ports:

```sh
./nat-link-relay \
  -server http://control.example.com:7000 \
  -id edge-singapore-1 \
  -address 203.0.113.10 \
  -control-listen 0.0.0.0:7100 \
  -control-address http://10.0.0.10:7100 \
  -bind-host 0.0.0.0 \
  -broker control.example.com:7001 \
  -token "$RELAY_API_TOKEN"
```

The Dashboard Relays page shows node heartbeat health, active connection
counts, and current tunnel assignments. A missed heartbeat marks a node
unhealthy; the control server moves its tunnels to another healthy relay or
falls back to its own TCP listener when no relay remains.

For TLS broker listeners, add `-broker-tls -broker-server-name control.example.com`.
Private certificate authorities can be supplied with `-broker-ca-file /path/to/ca.pem`;
hostname verification remains enabled.

Use an `https://` value for `-control-address` together with `-control-tls`,
`-control-cert-file`, and `-control-key-file` to encrypt the server-to-relay
control plane. Plain HTTP control listeners are intended only for local or
otherwise protected development networks.

Expose only the administration address ranges that need Dashboard access.
Tunnel IP allowlists are enforced independently for visitors.

## TLS

Agent control and TCP data listeners use the same certificate when TLS is enabled:

```yaml
server:
  tls:
    enabled: true
    cert_file: "/opt/nat-link/tls/fullchain.pem"
    key_file: "/opt/nat-link/tls/privkey.pem"
client:
  server_url: "wss://relay.example.com:7000/agent/connect"
  data_address: "relay.example.com:7001"
  insecure_skip_verify: false
```

Use a certificate whose DNS names include the host in `client.data_address`.
`insecure_skip_verify` exists only for controlled development with self-signed
certificates. HTTPS tunnels are passed through without terminating visitor TLS.

## QUIC and P2P

QUIC always uses TLS 1.3. For production, enable server TLS with a certificate
whose DNS names include the host in `client.quic_address`, then select the
transport on the CLI or desktop agent:

```yaml
client:
  transport: "quic"
  quic_address: "relay.example.com:7002"
```

When server TLS is disabled, NAT-Link creates an ephemeral development
certificate for QUIC; only controlled local testing should pair that with
`insecure_skip_verify: true`. UDP tunnels attempt the direct punched path over
the rendezvous listener and automatically use the authenticated control relay
when punching or the direct round trip fails.

## Linux installation

Create a system user and the PRD directory layout:

```sh
sudo useradd --system --home /opt/nat-link --shell /usr/sbin/nologin nat-link
sudo install -d -o nat-link -g nat-link /opt/nat-link/{data,logs,tls}
sudo install -o nat-link -g nat-link nat-link-server /opt/nat-link/
sudo install -o nat-link -g nat-link -m 0600 config.yaml /opt/nat-link/
sudo install -m 0644 deploy/nat-link-server.service /etc/systemd/system/nat-link.service
sudo systemctl daemon-reload
sudo systemctl enable --now nat-link
sudo journalctl -u nat-link -f
```

Install an agent with `deploy/nat-link-client.service` under
`/opt/nat-link-client` in the same way.

## Docker

Create `config.yaml` before running Compose. Set public addresses to the Docker
host's reachable DNS name, not `127.0.0.1`.

```sh
cp config.example.yaml config.yaml
docker compose up -d --build
docker compose logs -f nat-link-server
```

SQLite and logs are stored in named volumes. Mount certificate files read-only
and use their container paths in YAML when TLS is enabled.

## Cross-platform builds

On PowerShell:

```powershell
.\scripts\build.ps1 -Version 1.0.0
```

On Linux or macOS:

```sh
VERSION=1.0.0 ./scripts/build.sh
```

Both scripts produce Linux amd64/arm64, Windows amd64, and macOS amd64/arm64
server, CLI agent, and relay node binaries under `bin/`. Desktop artifacts use the build
instructions in `desktop/README.md`.

## Backup and recovery

Stop the server or use SQLite's online backup tooling before copying
`data/nat-link.db`. Keep the YAML configuration and TLS private key in a secret
backup. Restoring those files preserves administrator identity, tokens,
clients, tunnels, and traffic history.

## Security checklist

- Enable TLS with a trusted certificate.
- Replace the generated administrator password and protect the one-time output.
- Bind the Dashboard behind a firewall or private management network.
- Use a separate agent token per device so revocation is isolated.
- Add tunnel IP allowlists for sensitive services such as SSH and databases.
- Keep `data/`, configuration, and private keys readable only by the service user.
- Review and export logs before clearing them from the Dashboard.
