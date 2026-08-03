#!/usr/bin/env sh
set -eu

REPOSITORY="nrytex/nrynet"
VERSION="latest"
INSTALL_DIR="/opt/nrynet"
INSTALL_DIR_SET=0
LEGACY_INSTALL_DIR="/opt/nat-link"
LEGACY_SERVICE="nat-link-server"
PUBLIC_HOST=""
ADMIN_USER="admin"
FORCE_CONFIG=0
RENEW_CERT=0
ALLOW_DOWNGRADE=0
PROXY=""
CERTBOT_DOMAIN=""
CERTBOT_EMAIL=""
CERTBOT_STAGING=0
ENABLE_WS=0

usage() {
  cat <<'EOF'
Install Nrynet Server as a systemd service.

Usage: sudo ./install-server.sh [options]
  --public-host HOST   DNS name or IPv4 address advertised to clients
  --version VERSION    Release version such as 1.0.0 (default: latest)
  --install-dir PATH   Installation directory (default: /opt/nrynet)
  --admin-user NAME    Initial administrator name (default: admin)
  --force-config       Replace an existing config.yaml
  --renew-cert         Replace the generated TLS certificate
  --certbot-domain DNS Obtain/renew a Let's Encrypt certificate for this DNS name
  --certbot-email EMAIL
                       Email address for Let's Encrypt registration
  --certbot-staging    Use the Let's Encrypt staging directory
  --enable-ws          Also expose plaintext HTTP/WS on 7004 and data on 7005
  --allow-downgrade    Permit replacing a newer installed version
  --proxy URL          HTTP(S) or SOCKS5h proxy for dependencies and downloads
  -h, --help           Show this help
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --public-host) PUBLIC_HOST="$2"; shift 2 ;;
    --version) VERSION="$2"; shift 2 ;;
    --install-dir) INSTALL_DIR="$2"; INSTALL_DIR_SET=1; shift 2 ;;
    --admin-user) ADMIN_USER="$2"; shift 2 ;;
    --force-config) FORCE_CONFIG=1; shift ;;
    --renew-cert) RENEW_CERT=1; shift ;;
    --certbot-domain) CERTBOT_DOMAIN="$2"; shift 2 ;;
    --certbot-email) CERTBOT_EMAIL="$2"; shift 2 ;;
    --certbot-staging) CERTBOT_STAGING=1; shift ;;
    --enable-ws) ENABLE_WS=1; shift ;;
    --allow-downgrade) ALLOW_DOWNGRADE=1; shift ;;
    --proxy)
      [ "$#" -ge 2 ] || { echo "--proxy requires a URL." >&2; exit 2; }
      PROXY="$2"
      shift 2
      ;;
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

if [ -n "$PROXY" ]; then
  case "$PROXY" in
    http://*|https://*|socks5h://*) ;;
    *) echo "--proxy must be an HTTP(S) or SOCKS5h proxy URL." >&2; exit 2 ;;
  esac
  # Package managers and curl honor these standard proxy environment variables.
  export HTTP_PROXY="$PROXY" HTTPS_PROXY="$PROXY" ALL_PROXY="$PROXY"
  export http_proxy="$PROXY" https_proxy="$PROXY" all_proxy="$PROXY"
fi
case "$INSTALL_DIR" in
  *" "*|*"'"*|*'"'*) echo "--install-dir cannot contain spaces or quotes." >&2; exit 2 ;;
esac

install_dependencies() {
  missing=""
  for command in curl openssl tar sha256sum; do
    command -v "$command" >/dev/null 2>&1 || missing="$missing $command"
  done
  if [ -n "$CERTBOT_DOMAIN" ] && ! command -v certbot >/dev/null 2>&1; then
    missing="$missing certbot"
  fi
  [ -z "$missing" ] && return 0
  echo "Installing required tools:$missing"
  if command -v apt-get >/dev/null 2>&1; then
    apt-get update
    DEBIAN_FRONTEND=noninteractive apt-get install -y curl ca-certificates openssl tar coreutils ${CERTBOT_DOMAIN:+certbot}
  elif command -v dnf >/dev/null 2>&1; then
    dnf install -y curl ca-certificates openssl tar coreutils ${CERTBOT_DOMAIN:+certbot}
  elif command -v yum >/dev/null 2>&1; then
    yum install -y curl ca-certificates openssl tar coreutils ${CERTBOT_DOMAIN:+certbot}
  elif command -v apk >/dev/null 2>&1; then
    apk add --no-cache curl ca-certificates openssl tar coreutils ${CERTBOT_DOMAIN:+certbot}
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
    return 0
  fi
  if [ -n "$CERTBOT_DOMAIN" ]; then
    printf '%s' "$CERTBOT_DOMAIN"
    return 0
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

validate_certbot_options() {
  [ -n "$CERTBOT_DOMAIN" ] || return 0
  if [ -z "$CERTBOT_EMAIL" ]; then
    echo "--certbot-email is required with --certbot-domain." >&2
    exit 2
  fi
  if printf '%s' "$CERTBOT_DOMAIN" | grep -Eq '^[0-9.]+$'; then
    echo "--certbot-domain must be a DNS name; Let's Encrypt cannot issue for an IP address." >&2
    exit 2
  fi
}

certificate_spki() {
  [ -s "$1" ] || return 1
  openssl x509 -in "$1" -pubkey -noout |
    openssl pkey -pubin -outform DER |
    openssl dgst -sha256 -binary |
    openssl base64 -A
}

backup_existing_certificate() {
  [ -s "$CERT_FILE" ] || return 0
  [ -s "$KEY_FILE" ] || return 0
  backup_dir="$INSTALL_DIR/tls/backup-$(date -u +%Y%m%d%H%M%S)"
  install -d -m 0750 -o root -g nrynet "$backup_dir"
  install -m 0644 -o root -g nrynet "$CERT_FILE" "$backup_dir/fullchain.pem"
  install -m 0640 -o root -g nrynet "$KEY_FILE" "$backup_dir/privkey.pem"
  echo "Backed up the previous TLS certificate to $backup_dir"
}

enable_ws_in_config() {
  [ -f "$CONFIG_FILE" ] || return 0
  tmp_config="$CONFIG_FILE.tmp.$$"
  awk '
    function emit_missing() {
      if (in_server) {
        if (!seen_plain_listen) print "  plain_listen: \"0.0.0.0:7004\""
        if (!seen_plain_data) print "  plain_data_listen: \"0.0.0.0:7005\""
      }
    }
    /^server:[[:space:]]*$/ { print; in_server=1; seen_plain_listen=0; seen_plain_data=0; next }
    in_server && /^[^[:space:]#][^:]*:/ { emit_missing(); in_server=0 }
    in_server && /^[[:space:]]+plain_listen:/ {
      print "  plain_listen: \"0.0.0.0:7004\""; seen_plain_listen=1; next
    }
    in_server && /^[[:space:]]+plain_data_listen:/ {
      print "  plain_data_listen: \"0.0.0.0:7005\""; seen_plain_data=1; next
    }
    { print }
    END { emit_missing() }
  ' "$CONFIG_FILE" > "$tmp_config"
  chown root:nrynet "$tmp_config"
  chmod 0640 "$tmp_config"
  mv -f "$tmp_config" "$CONFIG_FILE"
}

config_ws_enabled() {
  [ -f "$CONFIG_FILE" ] || return 1
  grep -Eq '^[[:space:]]+plain_listen:[[:space:]]+"0\.0\.0\.0:7004"[[:space:]]*$' "$CONFIG_FILE" &&
    grep -Eq '^[[:space:]]+plain_data_listen:[[:space:]]+"0\.0\.0\.0:7005"[[:space:]]*$' "$CONFIG_FILE"
}

install_certbot_certificate() {
  [ -n "$CERTBOT_DOMAIN" ] || return 1
  staging=""
  [ "$CERTBOT_STAGING" -eq 0 ] || staging="--staging"
  hook="/etc/letsencrypt/renewal-hooks/deploy/nrynet-server.sh"
  old_spki="$(certificate_spki "$CERT_FILE" 2>/dev/null || true)"
  if [ -n "$old_spki" ]; then
    backup_existing_certificate
  fi
  install -d -m 0755 /etc/letsencrypt/renewal-hooks/deploy
  cat >"$TEMP_DIR/nrynet-certbot-deploy-hook.sh" <<EOF
#!/usr/bin/env sh
set -eu
target_lineage="/etc/letsencrypt/live/$CERTBOT_DOMAIN"
lineage="\${RENEWED_LINEAGE:-\$target_lineage}"
resolved_lineage="\$(readlink -f "\$lineage")"
resolved_target="\$(readlink -f "\$target_lineage")"
[ "\$resolved_lineage" = "\$resolved_target" ] || exit 0
install -m 0644 -o root -g nrynet "\$resolved_lineage/fullchain.pem" "$CERT_FILE"
install -m 0640 -o root -g nrynet "\$resolved_lineage/privkey.pem" "$KEY_FILE"
if systemctl list-unit-files nrynet-server.service >/dev/null 2>&1; then
  systemctl restart nrynet-server.service
fi
EOF
  install -m 0755 "$TEMP_DIR/nrynet-certbot-deploy-hook.sh" "$hook"
  echo "Requesting a Let's Encrypt certificate for $CERTBOT_DOMAIN with certbot..."
  certbot certonly --standalone --non-interactive --agree-tos --reuse-key \
    -m "$CERTBOT_EMAIL" -d "$CERTBOT_DOMAIN" $staging || {
      echo "Certbot failed. Verify $CERTBOT_DOMAIN resolves to this server and inbound TCP/80 reaches this host for the standalone HTTP-01 challenge." >&2
      exit 1
    }
  install -m 0644 -o root -g nrynet "/etc/letsencrypt/live/$CERTBOT_DOMAIN/fullchain.pem" "$CERT_FILE"
  install -m 0640 -o root -g nrynet "/etc/letsencrypt/live/$CERTBOT_DOMAIN/privkey.pem" "$KEY_FILE"
  new_spki="$(certificate_spki "$CERT_FILE" 2>/dev/null || true)"
  if [ -n "$old_spki" ] && [ -n "$new_spki" ] && [ "$old_spki" != "$new_spki" ]; then
    echo "WARNING: The TLS certificate public key changed while switching to certbot."
    echo "Existing pinned Agent Tokens will reject the new certificate; regenerate Agent Tokens in the Dashboard and update agents."
  fi
}

migrate_legacy_install() {
  [ "$INSTALL_DIR_SET" -eq 0 ] || return 0
  [ "$INSTALL_DIR" = "/opt/nrynet" ] || return 0
  legacy_found=0
  if [ -d "$LEGACY_INSTALL_DIR" ] && [ ! -e "$INSTALL_DIR" ]; then
    systemctl stop "$LEGACY_SERVICE" 2>/dev/null || true
    mv "$LEGACY_INSTALL_DIR" "$INSTALL_DIR"
    [ ! -f "$INSTALL_DIR/data/nat-link.db" ] || \
      mv "$INSTALL_DIR/data/nat-link.db" "$INSTALL_DIR/data/nrynet.db"
    [ ! -f "$INSTALL_DIR/nat-link-server" ] || \
      mv "$INSTALL_DIR/nat-link-server" "$INSTALL_DIR/nrynet-server"
    if [ -f "$INSTALL_DIR/config.yaml" ]; then
      sed -i 's#/opt/nat-link#/opt/nrynet#g; s#nat-link\.db#nrynet.db#g' "$INSTALL_DIR/config.yaml"
    fi
    legacy_found=1
  fi
  if systemctl cat "$LEGACY_SERVICE" >/dev/null 2>&1; then
    systemctl disable --now "$LEGACY_SERVICE" 2>/dev/null || true
    rm -f "/etc/systemd/system/$LEGACY_SERVICE.service"
    systemctl daemon-reload
    legacy_found=1
  fi
  MIGRATED_LEGACY="$legacy_found"
  return 0
}

preflight_legacy_install() {
  [ "$INSTALL_DIR_SET" -eq 0 ] || return 0
  [ "$INSTALL_DIR" = "/opt/nrynet" ] || return 0
  if [ -e "$LEGACY_INSTALL_DIR" ] && [ -e "$INSTALL_DIR" ]; then
    echo "Both $LEGACY_INSTALL_DIR and $INSTALL_DIR exist; resolve the legacy installation manually before continuing." >&2
    exit 1
  fi
  for database_dir in "$LEGACY_INSTALL_DIR/data" "$INSTALL_DIR/data"; do
    if [ -f "$database_dir/nat-link.db" ] && [ -f "$database_dir/nrynet.db" ]; then
      echo "Both nat-link.db and nrynet.db exist in $database_dir; resolve the database conflict manually before continuing." >&2
      exit 1
    fi
  done
  return 0
}

preflight_installed_version() {
  version_binary="$INSTALL_DIR/nrynet-server"
  if [ ! -x "$version_binary" ] && [ -x "$INSTALL_DIR/nat-link-server" ]; then
    version_binary="$INSTALL_DIR/nat-link-server"
  fi
  if [ "$INSTALL_DIR_SET" -eq 0 ] && [ "$INSTALL_DIR" = "/opt/nrynet" ] && \
    [ ! -x "$version_binary" ] && [ -x "$LEGACY_INSTALL_DIR/nat-link-server" ]; then
    version_binary="$LEGACY_INSTALL_DIR/nat-link-server"
  fi
  [ -x "$version_binary" ] || return 0
  installed_version="$("$version_binary" -version 2>/dev/null || true)"
  installed_version="$(printf '%s' "$installed_version" | tr -d '\r\n ' | sed 's/^v//')"
  [ -n "$installed_version" ] || return 0
  lowest="$(printf '%s\n%s\n' "$installed_version" "$TARGET_VERSION" | sort -V | head -n 1)"
  if [ "$lowest" = "$TARGET_VERSION" ] && [ "$ALLOW_DOWNGRADE" -ne 1 ] && [ "$installed_version" != "$TARGET_VERSION" ]; then
    echo "Installed version $installed_version is newer than requested $TARGET_VERSION." >&2
    echo "Use --allow-downgrade only when an intentional rollback is required." >&2
    exit 1
  fi
  return 0
}

if ! printf '%s' "$ADMIN_USER" | grep -Eq '^[A-Za-z0-9_.-]+$'; then
  echo "Administrator name may contain only letters, numbers, dot, underscore and hyphen." >&2
  exit 2
fi
validate_certbot_options

install_dependencies
command -v systemctl >/dev/null 2>&1 || { echo "This installer requires a systemd-based Linux distribution." >&2; exit 1; }
ARCH="$(detect_arch)"
PUBLIC_HOST="$(detect_public_host)"
validate_host "$PUBLIC_HOST"
ASSET="nrynet-linux-$ARCH.tar.gz"
if [ "$VERSION" = "latest" ]; then
  DOWNLOAD_BASE="https://github.com/$REPOSITORY/releases/latest/download"
else
  case "$VERSION" in v*) TAG="$VERSION" ;; *) TAG="v$VERSION" ;; esac
  DOWNLOAD_BASE="https://github.com/$REPOSITORY/releases/download/$TAG"
fi

TEMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TEMP_DIR"' EXIT INT TERM
echo "Downloading Nrynet Server ($ARCH, $VERSION)..."
download_file() {
  output="$1"
  url="$2"
  if [ -n "$PROXY" ]; then
    curl -fL --retry 3 --proxy "$PROXY" -o "$output" "$url"
  else
    curl -fL --retry 3 -o "$output" "$url"
  fi
}
download_file "$TEMP_DIR/$ASSET" "$DOWNLOAD_BASE/$ASSET"
download_file "$TEMP_DIR/SHA256SUMS" "$DOWNLOAD_BASE/SHA256SUMS"
EXPECTED="$(awk -v name="$ASSET" '$2 == name {print $1}' "$TEMP_DIR/SHA256SUMS")"
[ -n "$EXPECTED" ] || { echo "Release checksum for $ASSET was not found." >&2; exit 1; }
printf '%s  %s\n' "$EXPECTED" "$TEMP_DIR/$ASSET" | sha256sum -c -
mkdir -p "$TEMP_DIR/package"
tar -xzf "$TEMP_DIR/$ASSET" -C "$TEMP_DIR/package"
[ -f "$TEMP_DIR/package/nrynet-server" ] || { echo "Release archive is missing nrynet-server." >&2; exit 1; }
[ -f "$TEMP_DIR/package/VERSION" ] || { echo "Release archive is missing VERSION." >&2; exit 1; }
TARGET_VERSION="$(tr -d '\r\n ' < "$TEMP_DIR/package/VERSION")"
TARGET_VERSION="${TARGET_VERSION#v}"
[ -n "$TARGET_VERSION" ] || { echo "Release version is empty." >&2; exit 1; }

preflight_legacy_install
preflight_installed_version
MIGRATED_LEGACY=0
migrate_legacy_install

INSTALLED_VERSION=""
if [ -x "$INSTALL_DIR/nrynet-server" ]; then
  INSTALLED_VERSION="$("$INSTALL_DIR/nrynet-server" -version 2>/dev/null || true)"
  INSTALLED_VERSION="$(printf '%s' "$INSTALLED_VERSION" | tr -d '\r\n ' | sed 's/^v//')"
fi
REPLACE_BINARY=1
if [ "$INSTALLED_VERSION" = "$TARGET_VERSION" ]; then
  if [ "$MIGRATED_LEGACY" -eq 1 ]; then
    echo "Migrating Nrynet Server $TARGET_VERSION to the corrected product name..."
  else
    REPLACE_BINARY=0
    echo "Nrynet Server $TARGET_VERSION is already installed."
  fi
elif [ -n "$INSTALLED_VERSION" ]; then
  LOWEST="$(printf '%s\n%s\n' "$INSTALLED_VERSION" "$TARGET_VERSION" | sort -V | head -n 1)"
  if [ "$LOWEST" = "$TARGET_VERSION" ] && [ "$ALLOW_DOWNGRADE" -ne 1 ]; then
    echo "Installed version $INSTALLED_VERSION is newer than requested $TARGET_VERSION." >&2
    echo "Use --allow-downgrade only when an intentional rollback is required." >&2
    exit 1
  fi
  echo "Upgrading Nrynet Server from $INSTALLED_VERSION to $TARGET_VERSION..."
else
  echo "Installing Nrynet Server $TARGET_VERSION..."
fi

if ! getent group nrynet >/dev/null 2>&1; then
  groupadd --system nrynet
fi
if ! id -u nrynet >/dev/null 2>&1; then
  useradd --system --gid nrynet --home-dir "$INSTALL_DIR" --shell /usr/sbin/nologin nrynet
fi
install -d -m 0750 -o nrynet -g nrynet "$INSTALL_DIR" "$INSTALL_DIR/data" "$INSTALL_DIR/logs"
install -d -m 0750 -o root -g nrynet "$INSTALL_DIR/tls"
chown -R nrynet:nrynet "$INSTALL_DIR/data" "$INSTALL_DIR/logs"
if [ "$REPLACE_BINARY" -eq 1 ]; then
  if systemctl is-active --quiet nrynet-server 2>/dev/null; then
    systemctl stop nrynet-server
  fi
  install -m 0755 "$TEMP_DIR/package/nrynet-server" "$INSTALL_DIR/nrynet-server"
fi

CERT_FILE="$INSTALL_DIR/tls/fullchain.pem"
KEY_FILE="$INSTALL_DIR/tls/privkey.pem"
if [ -n "$CERTBOT_DOMAIN" ]; then
  install_certbot_certificate
elif [ ! -s "$CERT_FILE" ] || [ ! -s "$KEY_FILE" ] || [ "$RENEW_CERT" -eq 1 ]; then
    case "$PUBLIC_HOST" in
      *[!0-9.]*) PRIMARY_SAN="DNS:$PUBLIC_HOST" ;;
      *) PRIMARY_SAN="IP:$PUBLIC_HOST" ;;
    esac
    echo "Generating a self-signed TLS certificate with OpenSSL..."
    openssl req -x509 -newkey rsa:3072 -sha256 -nodes -days 825 \
      -keyout "$KEY_FILE" -out "$CERT_FILE" -subj "/CN=$PUBLIC_HOST" \
      -addext "subjectAltName=$PRIMARY_SAN,DNS:localhost,IP:127.0.0.1"
    chown root:nrynet "$CERT_FILE" "$KEY_FILE"
    chmod 0644 "$CERT_FILE"
    chmod 0640 "$KEY_FILE"
fi
chown root:nrynet "$CERT_FILE" "$KEY_FILE"
chmod 0644 "$CERT_FILE"
chmod 0640 "$KEY_FILE"

CONFIG_FILE="$INSTALL_DIR/config.yaml"
NEW_DATABASE=0
[ -f "$INSTALL_DIR/data/nrynet.db" ] || NEW_DATABASE=1
INITIAL_PASSWORD=""
if [ ! -f "$CONFIG_FILE" ] || [ "$FORCE_CONFIG" -eq 1 ]; then
  RELAY_TOKEN="$(openssl rand -hex 32)"
  [ "$NEW_DATABASE" -eq 0 ] || INITIAL_PASSWORD="$(openssl rand -hex 18)"
  PLAIN_CONFIG='  plain_listen: ""
  plain_data_listen: ""'
  if [ "$ENABLE_WS" -eq 1 ]; then
    PLAIN_CONFIG='  plain_listen: "0.0.0.0:7004"
  plain_data_listen: "0.0.0.0:7005"'
  fi
  cat >"$TEMP_DIR/config.yaml" <<EOF
server:
  listen: "0.0.0.0:7000"
  data_listen: "0.0.0.0:7001"
$PLAIN_CONFIG
  public_data_address: "$PUBLIC_HOST:7001"
  quic_listen: "0.0.0.0:7002"
  public_quic_address: "$PUBLIC_HOST:7002"
  rendezvous_listen: "0.0.0.0:7003"
  public_rendezvous_address: "$PUBLIC_HOST:7003"
  relay_api_token: "$RELAY_TOKEN"
  http_listen: "0.0.0.0:8080"
  database: "$INSTALL_DIR/data/nrynet.db"
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
  ca_file: ""
  token: ""
  name: ""
  device_id: ""
  insecure_skip_verify: false
EOF
  install -m 0640 -o root -g nrynet "$TEMP_DIR/config.yaml" "$CONFIG_FILE"
fi
if [ "$ENABLE_WS" -eq 1 ]; then
  enable_ws_in_config
fi
chown root:nrynet "$CONFIG_FILE"
chmod 0640 "$CONFIG_FILE"
WS_CONFIG_ENABLED=0
if config_ws_enabled; then
  WS_CONFIG_ENABLED=1
fi

cat >"$TEMP_DIR/nrynet-server.service" <<EOF
[Unit]
Description=Nrynet self-hosted relay server
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=nrynet
Group=nrynet
WorkingDirectory=$INSTALL_DIR
ExecStart=$INSTALL_DIR/nrynet-server -config $CONFIG_FILE
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
install -m 0644 "$TEMP_DIR/nrynet-server.service" /etc/systemd/system/nrynet-server.service
systemctl daemon-reload
systemctl enable nrynet-server
if systemctl is-active --quiet nrynet-server 2>/dev/null; then
  systemctl restart nrynet-server
else
  systemctl start nrynet-server
fi
sleep 2
if ! systemctl is-active --quiet nrynet-server; then
  journalctl -u nrynet-server -n 40 --no-pager >&2 || true
  echo "Nrynet Server failed to start." >&2
  exit 1
fi
if [ -n "$INITIAL_PASSWORD" ]; then
  sed -i 's/^    admin_password: ".*"/    admin_password: ""/' "$CONFIG_FILE"
fi

echo
echo "Nrynet Server is running: https://$PUBLIC_HOST:7000"
if [ "$WS_CONFIG_ENABLED" -eq 1 ]; then
  echo "Plaintext console/control is enabled: http://$PUBLIC_HOST:7004"
fi
echo "Installed version: $TARGET_VERSION"
if [ -n "$CERTBOT_DOMAIN" ]; then
  echo "Let's Encrypt certificate: $CERT_FILE"
  echo "Certbot renewals reuse the same private key and restart nrynet-server after copying renewed files."
else
  echo "Self-signed CA certificate: $CERT_FILE"
fi
if [ -n "$INITIAL_PASSWORD" ]; then
  echo "Administrator: $ADMIN_USER"
  echo "Initial password: $INITIAL_PASSWORD"
  echo "Record this password now; it has been removed from config.yaml."
fi
echo "New Agent Tokens automatically include this server certificate pin; clients do not need ca_file."
echo "If the TLS private key changes, regenerate Agent Tokens in the Dashboard."
