#!/usr/bin/env sh
set -eu

REPOSITORY="nrytex/nrynet"
VERSION="latest"
INSTALL_DIR="/opt/nat-link"
PUBLIC_HOST=""
ADMIN_USER="admin"
FORCE_CONFIG=0
RENEW_CERT=0

usage() {
  cat <<'EOF'
Install NAT-Link Server as a systemd service.

Usage: sudo ./install-server.sh [options]
  --public-host HOST   DNS name or IPv4 address advertised to clients
  --version VERSION    Release version such as 2.3.5 (default: latest)
  --install-dir PATH   Installation directory (default: /opt/nat-link)
  --admin-user NAME    Initial administrator name (default: admin)
  --force-config       Replace an existing config.yaml
  --renew-cert         Replace the generated TLS certificate
  -h, --help           Show this help
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --public-host) PUBLIC_HOST="$2"; shift 2 ;;
    --version) VERSION="$2"; shift 2 ;;
    --install-dir) INSTALL_DIR="$2"; shift 2 ;;
    --admin-user) ADMIN_USER="$2"; shift 2 ;;
    --force-config) FORCE_CONFIG=1; shift ;;
    --renew-cert) RENEW_CERT=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "Unknown option: $1" >&2; usage >&2; exit 2 ;;
  esac
done

if [ "$(id -u)" -ne 0 ]; then
  echo "Run this installer as root (for example with sudo)." >&2
  exit 1
fi
case "$INSTALL_DIR" in
  /*) ;;
  *) echo "--install-dir must be an absolute path." >&2; exit 2 ;;
esac
case "$INSTALL_DIR" in
  *" "*|*"'"*|*'"'*) echo "--install-dir cannot contain spaces or quotes." >&2; exit 2 ;;
esac

install_dependencies() {
  missing=""
  for command in curl openssl tar sha256sum; do
    command -v "$command" >/dev/null 2>&1 || missing="$missing $command"
  done
  [ -z "$missing" ] && return
  echo "Installing required tools:$missing"
  if command -v apt-get >/dev/null 2>&1; then
    apt-get update
    DEBIAN_FRONTEND=noninteractive apt-get install -y curl ca-certificates openssl tar coreutils
  elif command -v dnf >/dev/null 2>&1; then
    dnf install -y curl ca-certificates openssl tar coreutils
  elif command -v yum >/dev/null 2>&1; then
    yum install -y curl ca-certificates openssl tar coreutils
  elif command -v apk >/dev/null 2>&1; then
    apk add --no-cache curl ca-certificates openssl tar coreutils
  else
    echo "Install curl, openssl, tar and sha256sum, then run this script again." >&2
    exit 1
  fi
}

detect_arch() {
  case "$(uname -m)" in
    x86_64|amd64) echo "amd64" ;;
    aarch64|arm64) echo "arm64" ;;
    *) echo "Unsupported CPU architecture: $(uname -m)" >&2; exit 1 ;;
  esac
}

detect_public_host() {
  if [ -n "$PUBLIC_HOST" ]; then
    printf '%s' "$PUBLIC_HOST"
    return
  fi
  host="$(hostname -f 2>/dev/null || hostname)"
  [ -n "$host" ] || host="127.0.0.1"
  printf '%s' "$host"
}

validate_host() {
  if ! printf '%s' "$1" | grep -Eq '^[A-Za-z0-9.-]+$'; then
    echo "Public host must be a DNS name or IPv4 address." >&2
    exit 2
  fi
}

if ! printf '%s' "$ADMIN_USER" | grep -Eq '^[A-Za-z0-9_.-]+$'; then
  echo "Administrator name may contain only letters, numbers, dot, underscore and hyphen." >&2
  exit 2
fi

install_dependencies
command -v systemctl >/dev/null 2>&1 || { echo "This installer requires a systemd-based Linux distribution." >&2; exit 1; }
ARCH="$(detect_arch)"
PUBLIC_HOST="$(detect_public_host)"
validate_host "$PUBLIC_HOST"
ASSET="nat-link-linux-$ARCH.tar.gz"
if [ "$VERSION" = "latest" ]; then
  DOWNLOAD_BASE="https://github.com/$REPOSITORY/releases/latest/download"
else
  case "$VERSION" in v*) TAG="$VERSION" ;; *) TAG="v$VERSION" ;; esac
  DOWNLOAD_BASE="https://github.com/$REPOSITORY/releases/download/$TAG"
fi

TEMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TEMP_DIR"' EXIT INT TERM
echo "Downloading NAT-Link Server ($ARCH, $VERSION)..."
curl -fL --retry 3 -o "$TEMP_DIR/$ASSET" "$DOWNLOAD_BASE/$ASSET"
curl -fL --retry 3 -o "$TEMP_DIR/SHA256SUMS" "$DOWNLOAD_BASE/SHA256SUMS"
EXPECTED="$(awk -v name="$ASSET" '$2 == name {print $1}' "$TEMP_DIR/SHA256SUMS")"
[ -n "$EXPECTED" ] || { echo "Release checksum for $ASSET was not found." >&2; exit 1; }
printf '%s  %s\n' "$EXPECTED" "$TEMP_DIR/$ASSET" | sha256sum -c -
mkdir -p "$TEMP_DIR/package"
tar -xzf "$TEMP_DIR/$ASSET" -C "$TEMP_DIR/package"
[ -f "$TEMP_DIR/package/nat-link-server" ] || { echo "Release archive is missing nat-link-server." >&2; exit 1; }

if ! getent group nat-link >/dev/null 2>&1; then
  groupadd --system nat-link
fi
if ! id -u nat-link >/dev/null 2>&1; then
  useradd --system --gid nat-link --home-dir "$INSTALL_DIR" --shell /usr/sbin/nologin nat-link
fi
install -d -m 0750 -o nat-link -g nat-link "$INSTALL_DIR" "$INSTALL_DIR/data" "$INSTALL_DIR/logs"
install -d -m 0750 -o root -g nat-link "$INSTALL_DIR/tls"
install -m 0755 "$TEMP_DIR/package/nat-link-server" "$INSTALL_DIR/nat-link-server"

CERT_FILE="$INSTALL_DIR/tls/fullchain.pem"
KEY_FILE="$INSTALL_DIR/tls/privkey.pem"
if [ ! -s "$CERT_FILE" ] || [ ! -s "$KEY_FILE" ] || [ "$RENEW_CERT" -eq 1 ]; then
  case "$PUBLIC_HOST" in
    *[!0-9.]*) PRIMARY_SAN="DNS:$PUBLIC_HOST" ;;
    *) PRIMARY_SAN="IP:$PUBLIC_HOST" ;;
  esac
  echo "Generating a self-signed TLS certificate with OpenSSL..."
  openssl req -x509 -newkey rsa:3072 -sha256 -nodes -days 825 \
    -keyout "$KEY_FILE" -out "$CERT_FILE" -subj "/CN=$PUBLIC_HOST" \
    -addext "subjectAltName=$PRIMARY_SAN,DNS:localhost,IP:127.0.0.1"
  chown root:nat-link "$CERT_FILE" "$KEY_FILE"
  chmod 0644 "$CERT_FILE"
  chmod 0640 "$KEY_FILE"
fi

CONFIG_FILE="$INSTALL_DIR/config.yaml"
NEW_DATABASE=0
[ -f "$INSTALL_DIR/data/nat-link.db" ] || NEW_DATABASE=1
INITIAL_PASSWORD=""
if [ ! -f "$CONFIG_FILE" ] || [ "$FORCE_CONFIG" -eq 1 ]; then
  RELAY_TOKEN="$(openssl rand -hex 32)"
  [ "$NEW_DATABASE" -eq 0 ] || INITIAL_PASSWORD="$(openssl rand -hex 18)"
  cat >"$TEMP_DIR/config.yaml" <<EOF
server:
  listen: "0.0.0.0:7000"
  data_listen: "0.0.0.0:7001"
  public_data_address: "$PUBLIC_HOST:7001"
  quic_listen: "0.0.0.0:7002"
  public_quic_address: "$PUBLIC_HOST:7002"
  rendezvous_listen: "0.0.0.0:7003"
  public_rendezvous_address: "$PUBLIC_HOST:7003"
  relay_api_token: "$RELAY_TOKEN"
  http_listen: "0.0.0.0:8080"
  database: "$INSTALL_DIR/data/nat-link.db"
  log_directory: "$INSTALL_DIR/logs"
  jwt_ttl: "12h"
  heartbeat_timeout: "45s"
  tls:
    enabled: true
    cert_file: "$CERT_FILE"
    key_file: "$KEY_FILE"
  bootstrap:
    admin_username: "$ADMIN_USER"
    admin_password: "$INITIAL_PASSWORD"
client:
  server_url: "wss://$PUBLIC_HOST:7000/agent/connect"
  data_address: "$PUBLIC_HOST:7001"
  transport: "websocket"
  quic_address: "$PUBLIC_HOST:7002"
  rendezvous_address: "$PUBLIC_HOST:7003"
  ca_file: "$CERT_FILE"
  token: ""
  name: ""
  device_id: ""
  insecure_skip_verify: false
EOF
  install -m 0640 -o root -g nat-link "$TEMP_DIR/config.yaml" "$CONFIG_FILE"
fi

cat >"$TEMP_DIR/nat-link-server.service" <<EOF
[Unit]
Description=NAT-Link self-hosted relay server
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=nat-link
Group=nat-link
WorkingDirectory=$INSTALL_DIR
ExecStart=$INSTALL_DIR/nat-link-server -config $CONFIG_FILE
Restart=on-failure
RestartSec=3
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=$INSTALL_DIR/data $INSTALL_DIR/logs
LimitNOFILE=1048576

[Install]
WantedBy=multi-user.target
EOF
install -m 0644 "$TEMP_DIR/nat-link-server.service" /etc/systemd/system/nat-link-server.service
systemctl daemon-reload
systemctl enable --now nat-link-server
sleep 2
if ! systemctl is-active --quiet nat-link-server; then
  journalctl -u nat-link-server -n 40 --no-pager >&2 || true
  echo "NAT-Link Server failed to start." >&2
  exit 1
fi
if [ -n "$INITIAL_PASSWORD" ]; then
  sed -i 's/^    admin_password: ".*"/    admin_password: ""/' "$CONFIG_FILE"
fi

echo
echo "NAT-Link Server is running: https://$PUBLIC_HOST:7000"
echo "Self-signed CA certificate: $CERT_FILE"
if [ -n "$INITIAL_PASSWORD" ]; then
  echo "Administrator: $ADMIN_USER"
  echo "Initial password: $INITIAL_PASSWORD"
  echo "Record this password now; it has been removed from config.yaml."
fi
echo "Copy fullchain.pem to each client and configure it as ca_file."
