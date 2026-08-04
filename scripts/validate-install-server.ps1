param(
    [string]$ScriptPath = (Join-Path $PSScriptRoot "install-server.sh"),
    [string]$PowerShellScriptPath = (Join-Path $PSScriptRoot "install-server.ps1")
)

$ErrorActionPreference = "Stop"
$linuxScript = Get-Content -Raw -LiteralPath $ScriptPath
$powerShellScript = Get-Content -Raw -LiteralPath $PowerShellScriptPath

function Assert-Contains([string]$Text, [string]$Needle, [string]$Message) {
    if (-not $Text.Contains($Needle)) { throw $Message }
}

Assert-Contains $linuxScript '--enable-ws)' "install-server.sh must accept --enable-ws as an initial plaintext WS preset."
Assert-Contains $linuxScript 'TLS_ENABLED=false' "Linux installer must default new installations to HTTP/WS."
Assert-Contains $linuxScript 'server_url: "ws://$PUBLIC_HOST:7000/agent/connect"' "Linux installer must default agents to WS."
Assert-Contains $linuxScript 'coreutils certbot' "Linux installer must install Certbot for later Dashboard domain binding."
Assert-Contains $linuxScript 'systemd_certbot_available()' "Linux installer must require a native Certbot executable usable by the restricted helper."
Assert-Contains $linuxScript 'Snap-only Certbot installations cannot run inside the restricted Nrynet helper.' "Linux installer must explain why a snap-only Certbot is unsupported."
Assert-Contains $linuxScript 'install -d -m 0750 -o root -g nrynet "$INSTALL_DIR"' "Install directory must not be writable by the unprivileged service that triggers the root helper."
Assert-Contains $linuxScript 'plain_enabled: false' "Default generated config must keep plaintext WS disabled."
Assert-Contains $linuxScript 'plain_listen: "0.0.0.0:7004"' "Default generated config must store the plaintext control address for dashboard toggles."
Assert-Contains $linuxScript 'plain_enabled: true' "Generated config must enable plaintext WS only when requested."
Assert-Contains $linuxScript 'sync_plain_config()' "Linux installer must update existing configs with the plaintext WS boolean."
Assert-Contains $linuxScript 'config_plain_enabled_present' "Linux installer must preserve old enabled plaintext pairs during upgrade."
Assert-Contains $linuxScript 'mv -f "$tmp_config" "$CONFIG_FILE"' "Linux existing-config update must use a same-directory temporary file and atomic replacement."
Assert-Contains $linuxScript 'WS_CONFIG_ENABLED=0' "Linux installer must derive output from the actual config state."
Assert-Contains $linuxScript 'if [ "$WS_CONFIG_ENABLED" -eq 1 ]; then' "Linux installer must only announce WS when the config is actually enabled."
Assert-Contains $powerShellScript 'function Sync-PlainWSConfig' "PowerShell installer must update existing configs with the plaintext WS boolean."
Assert-Contains $powerShellScript 'function Test-PlainEnabledPresent' "PowerShell installer must preserve old enabled plaintext pairs during upgrade."
Assert-Contains $powerShellScript 'Move-Item -LiteralPath $tempPath -Destination $ConfigPath -Force' "PowerShell existing-config update must replace via a temporary file."
Assert-Contains $powerShellScript '$wsConfigEnabled = Test-WSConfigEnabled $ConfigFile' "PowerShell installer must derive its output from the actual config state."
Assert-Contains $powerShellScript '$tcpPorts = "7000,7001,7004,7005,8080"' "PowerShell installer must reserve optional WS ports for later Dashboard toggles."
Assert-Contains $powerShellScript 'if ($wsConfigEnabled) { Write-Host "Plaintext console/control is enabled' "PowerShell installer must only announce WS when the config is actually enabled."
Assert-Contains $linuxScript 'target_lineage="/etc/letsencrypt/live/$CERTBOT_DOMAIN"' "Certbot hook must pin the expected lineage."
Assert-Contains $linuxScript '[ "\$resolved_lineage" = "\$resolved_target" ] || exit 0' "Certbot hook must ignore other renewed lineages."
Assert-Contains $linuxScript 'certbot certonly --standalone --non-interactive --agree-tos --reuse-key' "Certbot must use standalone mode with --reuse-key."
Assert-Contains $linuxScript '--cert-name "$CERTBOT_DOMAIN"' "Direct Certbot issuance must use a deterministic certificate lineage."
Assert-Contains $linuxScript 'PathChanged=$INSTALL_DIR/data/certbot/inbox/request.json' "Linux installer must register the isolated Dashboard Certbot request inbox."
Assert-Contains $linuxScript 'install -d -m 0750 -o root -g nrynet /var/lib/nrynet/certbot' "Linux installer must create the root-owned Certbot state directory."
Assert-Contains $linuxScript 'install -d -m 0700 -o root -g root /var/lib/nrynet/certbot/work' "Linux installer must keep Certbot work state root-only."
Assert-Contains $linuxScript '--certbot-helper --certbot-helper-install-dir $INSTALL_DIR' "Linux installer must register the privileged Certbot helper."
Assert-Contains $linuxScript 'Environment=PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin' "Certbot helpers must receive a deterministic native executable path."
Assert-Contains $linuxScript '--certbot-renew --certbot-helper-install-dir $INSTALL_DIR' "Linux installer must register automatic managed certificate renewal."
Assert-Contains $linuxScript 'nrynet-certbot.path nrynet-certbot-renew.timer' "Linux installer must enable Certbot request and renewal units."
Assert-Contains $linuxScript 'mv -f "$managed_tmp" /var/lib/nrynet/certbot/managed.json' "Direct Certbot installation must initialize the root-only renewal target."
Assert-Contains $linuxScript 'mv -f "$CERT_FILE.new" "$CERT_FILE"' "Certbot renewal hook must atomically replace the certificate."
if ($linuxScript.Contains('systemctl restart nrynet-server.service')) {
    throw "Certbot renewal must hot-load certificates instead of restarting Nrynet."
}
Assert-Contains $powerShellScript 'enabled: false' "Windows installer must default new installations to HTTP/WS."
Assert-Contains $powerShellScript 'server_url: "ws://${PublicHost}:7000/agent/connect"' "Windows installer must default agents to WS."
Assert-Contains $linuxScript 'Existing pinned Agent Tokens will reject the new certificate' "Certbot SPKI change warning is required."

"ok"
