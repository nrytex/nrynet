# Nrynet operations

[中文安装部署指南](deployment.zh-CN.md)

## Network ports

The example configuration exposes these server listeners:

| Port | Transport | Purpose |
| --- | --- | --- |
| 7000 | HTTPS | Dashboard, administration API, and agent WebSocket control |
| 7001 | TLS | Per-connection TCP relay data channel |
| 7002 | UDP/QUIC | Authenticated QUIC control and data streams |
| 7003 | UDP | P2P endpoint rendezvous and hole punching |
| 7004 | HTTP/WS | Optional plaintext Dashboard, API, and agent control |
| 7005 | TCP | Optional plaintext relay data channel for WS agents |
| 8080 | TCP | Shared HTTP Host and HTTPS SNI gateway |
| 7100 | HTTPS | Distributed relay control API (relay node only) |
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
./nrynet-relay \
  -server https://control.example.com:7000 \
  -id edge-singapore-1 \
  -address 203.0.113.10 \
  -control-listen 0.0.0.0:7100 \
  -control-address https://relay.example.com:7100 \
  -bind-host 0.0.0.0 \
  -broker control.example.com:7001 \
  -broker-tls \
  -broker-server-name control.example.com \
  -control-tls \
  -control-cert-file /opt/nrynet-relay/tls/fullchain.pem \
  -control-key-file /opt/nrynet-relay/tls/privkey.pem \
  -token "$RELAY_API_TOKEN"
```

The Dashboard Relays page shows node heartbeat health, active connection
counts, and current tunnel assignments. A missed heartbeat marks a node
unhealthy; the control server moves its tunnels to another healthy relay or
falls back to its own TCP listener when no relay remains.

Private certificate authorities can be supplied with `-broker-ca-file /path/to/ca.pem`;
hostname verification remains enabled.

Use an `https://` value for `-control-address` together with `-control-tls`,
`-control-cert-file`, and `-control-key-file` to encrypt the server-to-relay
control plane. Plain HTTP control and broker connections are rejected unless
every endpoint is loopback. This keeps local development simple without
permitting remote credentials or tunneled bytes on a plaintext control plane.

Expose only the administration address ranges that need Dashboard access.
Tunnel IP allowlists are enforced independently for visitors.

## WS, WSS, And TLS

`server.listen` and `server.data_listen` remain the primary pair. With
`server.tls.enabled: true` they serve HTTPS/WSS control and TLS data. With TLS
disabled they serve HTTP/WS control and plaintext data. To run both transports
at the same time, keep TLS enabled on the primary pair, keep the plaintext
addresses configured, and enable `server.plain_enabled`. The Dashboard settings
page can turn plaintext WS access on or off; saved changes are persisted to
the server database as configuration overrides and take effect after restarting
the server service:

```yaml
server:
  listen: "0.0.0.0:7000"
  data_listen: "0.0.0.0:7001"
  plain_enabled: true
  plain_listen: "0.0.0.0:7004"
  plain_data_listen: "0.0.0.0:7005"
  tls:
    enabled: true
    cert_file: "/opt/nrynet/tls/fullchain.pem"
    key_file: "/opt/nrynet/tls/privkey.pem"
client:
  server_url: "wss://relay.example.com:7000/agent/connect"
  data_address: "relay.example.com:7001"
  ca_file: ""
  insecure_skip_verify: false
```

WS agents must use `server_url: "ws://host:7004/agent/connect"` together with
`data_address: "host:7005"`. WSS agents must use
`server_url: "wss://host:7000/agent/connect"` together with
`data_address: "host:7001"`. Mixing WS control with the TLS data port, or WSS
control with the plaintext data port, will fail because the agent deliberately
chooses data-channel TLS from the WebSocket scheme.

Publicly trusted certificates must include the control and data hosts in their
DNS or IP names. New Agent Tokens carry the installer-generated self-signed
certificate SPKI pin, so pinned agents can safely use an IP or alternate host
without `client.ca_file` or a matching certificate SAN.
Legacy tokens may still use `client.ca_file`. Regenerate Agent Tokens whenever
the server certificate key changes. `insecure_skip_verify` is accepted only for
loopback development; remote clients must validate the certificate or token pin.
HTTPS tunnels are passed through without terminating visitor TLS.

For a domain certificate on Linux, use certbot through the installer:

```sh
sudo ./install-server.sh \
  --certbot-domain nat.example.com \
  --certbot-email admin@example.com \
  --enable-ws
```

This command presets `server.plain_enabled: true`, giving domain users
`wss://nat.example.com:7000/agent/connect` and also enabling IP users to connect
with `ws://<server-ip>:7004/agent/connect`. Omit `--enable-ws` when plaintext
WS/API should stay disabled initially; administrators can later change the same
setting from the Dashboard and restart `nrynet-server`.

Certbot uses the standalone HTTP-01 challenge, so `nat.example.com` must resolve
to this server and inbound TCP/80 must reach it while the installer runs. The
installer runs certbot with `--reuse-key`, copies the renewed `fullchain.pem`
and `privkey.pem` into `/opt/nrynet/tls`, and registers a deploy hook that
restarts `nrynet-server` only for the target certificate lineage. Reusing the
key preserves Agent Token SPKI pins across normal renewals. Switching from an
existing self-signed certificate to certbot usually changes the SPKI; the
installer backs up the old certificate and warns that Agent Tokens must be
regenerated when it detects that change. If certbot fails, check DNS and TCP/80
before retrying.

## QUIC and P2P

QUIC always uses TLS 1.3. For production, enable server TLS with a publicly
trusted certificate matching `client.quic_address` or use a pinned Agent Token,
then select the
transport on the CLI or desktop agent:

```yaml
client:
  transport: "quic"
  quic_address: "relay.example.com:7002"
```

When server TLS is disabled, Nrynet creates an ephemeral development
certificate for QUIC; only controlled local testing should pair that with
`insecure_skip_verify: true`. UDP tunnels attempt the direct punched path over
the rendezvous listener and automatically use the authenticated control relay
when punching or the direct round trip fails.

## Linux installation

Create a system user and the PRD directory layout:

```sh
sudo useradd --system --home /opt/nrynet --shell /usr/sbin/nologin nrynet
sudo install -d -o nrynet -g nrynet /opt/nrynet/{data,logs,tls}
sudo install -o nrynet -g nrynet nrynet-server /opt/nrynet/
sudo install -o nrynet -g nrynet -m 0600 config.yaml /opt/nrynet/
sudo install -m 0644 deploy/nrynet-server.service /etc/systemd/system/nrynet-server.service
sudo systemctl daemon-reload
sudo systemctl enable --now nrynet-server
sudo journalctl -u nrynet-server -f
```

Install an agent with `deploy/nrynet-client.service` under
`/opt/nrynet-client` in the same way.

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
`data/nrynet.db`. Keep the YAML configuration and TLS private key in a secret
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
