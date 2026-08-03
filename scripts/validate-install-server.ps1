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

Assert-Contains $linuxScript '--enable-ws)' "install-server.sh must require --enable-ws before exposing plaintext WS ports."
Assert-Contains $linuxScript 'plain_listen: ""' "Default generated config must keep plain_listen empty."
Assert-Contains $linuxScript '[ "$ENABLE_WS" -eq 1 ]' "Generated config must enable plaintext ports only when requested."
Assert-Contains $linuxScript 'enable_ws_in_config()' "Linux installer must be able to update an existing config for --enable-ws."
Assert-Contains $linuxScript 'mv -f "$tmp_config" "$CONFIG_FILE"' "Linux existing-config update must use a same-directory temporary file and atomic replacement."
Assert-Contains $linuxScript 'WS_CONFIG_ENABLED=0' "Linux installer must derive output from the actual config state."
Assert-Contains $linuxScript 'if [ "$WS_CONFIG_ENABLED" -eq 1 ]; then' "Linux installer must only announce WS when the config is actually enabled."
Assert-Contains $powerShellScript 'function Enable-WSConfigPair' "PowerShell installer must update an existing config for -EnableWS."
Assert-Contains $powerShellScript 'Move-Item -LiteralPath $tempPath -Destination $ConfigPath -Force' "PowerShell existing-config update must replace via a temporary file."
Assert-Contains $powerShellScript '$wsConfigEnabled = Test-WSConfigEnabled $ConfigFile' "PowerShell installer must derive firewall/output from actual config state."
Assert-Contains $powerShellScript '$tcpPorts = if ($wsConfigEnabled)' "PowerShell firewall ports must follow actual config state."
Assert-Contains $powerShellScript 'if ($wsConfigEnabled) { Write-Host "Plaintext console/control is enabled' "PowerShell installer must only announce WS when the config is actually enabled."
Assert-Contains $linuxScript 'target_lineage="/etc/letsencrypt/live/$CERTBOT_DOMAIN"' "Certbot hook must pin the expected lineage."
Assert-Contains $linuxScript '[ "\$resolved_lineage" = "\$resolved_target" ] || exit 0' "Certbot hook must ignore other renewed lineages."
Assert-Contains $linuxScript 'certbot certonly --standalone --non-interactive --agree-tos --reuse-key' "Certbot must use standalone mode with --reuse-key."
Assert-Contains $linuxScript 'Existing pinned Agent Tokens will reject the new certificate' "Certbot SPKI change warning is required."

"ok"
