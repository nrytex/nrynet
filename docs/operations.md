# Nrynet operations

[中文安装部署指南](deployment.zh-CN.md)

## Network ports

The example configuration exposes these server listeners:

| Port | Transport | Purpose |
| --- | --- | --- |
| 7000 | HTTP/HTTPS, WS/WSS | Dashboard, administration API, and agent control |
| 7001 | TCP/TLS | Per-connection relay data channel |
| 7002 | UDP/QUIC | Authenticated QUIC control and data streams |
| 7003 | UDP | P2P endpoint rendezvous and hole punching |
| 7004 | HTTP/WS | Optional plaintext Dashboard, API, and agent control |
| 7005 | TCP | Optional plaintext relay data channel for WS agents |
| 8080 | TCP | Shared HTTP Host and HTTPS SNI gateway |
| 7100 | HTTPS | Distributed relay control API (relay node only) |
| tunnel-defined | TCP, P2P, or UDP | Public visitor ports |

`visitor_webrtc` does not allocate a public TCP port. Its visitor URL is served
on the control/Dashboard listener. The browser uses that page for signaling and
then loads the complete local web application over a multiplexed, streaming
WebRTC DataChannel to the Agent. HTML, CSS, JavaScript, images, and API calls
use the direct channel; only signaling crosses the Server after the peer
connection is established.

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

`server.listen` and `server.data_listen` are the primary pair. New installations
start with HTTP/WS control and plaintext data. Enabling TLS adds HTTPS/WSS and
TLS data on the same ports by classifying TLS handshakes; it does not remove the
existing HTTP/WS path. Certificate and TLS changes are hot-loaded. The optional
`plain_*` pair is retained only for legacy clients that require separate ports:

```yaml
server:
  listen: "0.0.0.0:7000"
  data_listen: "0.0.0.0:7001"
  plain_enabled: false
  plain_listen: "0.0.0.0:7004"
  plain_data_listen: "0.0.0.0:7005"
  tls:
    enabled: false
    cert_file: "/opt/nrynet/tls/fullchain.pem"
    key_file: "/opt/nrynet/tls/privkey.pem"
client:
  server_url: "ws://relay.example.com:7000/agent/connect"
  data_address: "relay.example.com:7001"
  ca_file: ""
  insecure_skip_verify: false
```

WS agents use `ws://host:7000/agent/connect`; WSS agents use
`wss://domain:7000/agent/connect`. Both use data port `7001`, which performs the
same plaintext/TLS handshake classification. The optional `7004/7005` pair can
still be hot-enabled from the Dashboard for older deployment layouts.

Publicly trusted certificates must include the control and data hosts in their
DNS or IP names. New Agent Tokens carry the installer-generated self-signed
certificate SPKI pin, so pinned agents can safely use an IP or alternate host
without `client.ca_file` or a matching certificate SAN.
Legacy tokens may still use `client.ca_file`. Regenerate Agent Tokens whenever
the server certificate key changes. `insecure_skip_verify` is accepted only for
loopback development; remote clients must validate the certificate or token pin.
HTTPS tunnels are passed through without terminating visitor TLS.

On an installer-managed Linux server, open the Dashboard **Settings > Access
and Certificates**, enter the domain and email address, and request a Let's
Encrypt certificate. The request is handled by a restricted root systemd helper;
the main Nrynet process remains the unprivileged `nrynet` user. The CLI flow is
also available:

```sh
sudo ./install-server.sh \
  --certbot-domain nat.example.com \
  --certbot-email admin@example.com
```

After issuance, `https://nat.example.com:7000` and
`wss://nat.example.com:7000/agent/connect` become available immediately while
the original HTTP/WS endpoints remain available.

Certbot uses the standalone HTTP-01 challenge, so `nat.example.com` must resolve
to this server and inbound TCP/80 must reach it while the installer runs. The
helper runs certbot with `--reuse-key`, atomically installs `fullchain.pem` and
`privkey.pem` into `/opt/nrynet/tls`, and renews daily. Nrynet detects the new
files and swaps the certificate used by new handshakes without restarting the
service. The unprivileged server can only write the request inbox; the approved
renewal target, lock, and Certbot work state stay root-owned under
`/var/lib/nrynet/certbot`. If issuance fails, check DNS, conflicting port-80
services, and host or cloud firewall rules before retrying.

When manual Certbot succeeds but the Dashboard helper fails, rerun the latest
installer to refresh the helper units and ensure a native Certbot exists at
`/usr/bin/certbot` or `/usr/local/bin/certbot`. Snap-only Certbot executables are
not used by the restricted helper. The Dashboard includes the bounded Certbot
diagnostic returned by the helper. Full diagnostics remain available with
`sudo cat /var/lib/nrynet/certbot/status.json` and
`sudo journalctl -u nrynet-certbot.service -n 100 --no-pager`.

## Automatic tunnel subdomains

In **Settings > Access and Certificates**, set a tunnel root such as
`tunnels.example.com` and enable automatic subdomain assignment. Configure this
wildcard DNS record once:

```text
*.tunnels.example.com  A/AAAA  <Nrynet server address>
```

New HTTP/HTTPS tunnels whose domain field is empty receive a unique hostname
derived from the tunnel name, for example `dashboard.tunnels.example.com`.
Duplicate names receive a numeric suffix. A domain entered explicitly always
wins. Disabling the feature only stops new allocations; existing tunnel domains
continue to route normally.

The shared gateway listens on TCP `8080` and routes HTTP by `Host` and HTTPS by
SNI. Expose or forward the required public port to `8080`. HTTPS remains SNI
pass-through, so the destination service must present a certificate valid for
the assigned hostname. The Dashboard's single-host HTTP-01 certificate is not a
wildcard tunnel certificate.

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
when punching or the direct round trip fails. With `server.p2p_enabled: true`,
TCP tunnel connections also try a QUIC stream over a UDP hole-punched
server-to-Agent path and fall back to the normal broker on failure.

This is a direct path between the public Nrynet server and the Agent. A
visitor still connects to the server's public tunnel port, so it does not
remove the server's public ingress/egress bandwidth. Full visitor-to-Agent
P2P requires the visitor to run a compatible client or join an overlay
network such as WireGuard or Tailscale.

For a public deployment, set `server.public_rendezvous_address` to the
server's public `host:port`, expose UDP port `7003`, and allow the dynamic
UDP sockets used by punched sessions. Agents need outbound UDP access. A
strict NAT or firewall may reject hole punching; the normal relay path remains
the fallback.

Each TCP visitor gets its own punched QUIC session. Direct P2P no longer has a
fixed 128-session application cap; capacity is governed by available CPU,
memory, socket/file-descriptor limits, and the QUIC flow-control windows. If a
network cannot punch successfully, that individual visitor falls back to the
authenticated broker relay.

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
