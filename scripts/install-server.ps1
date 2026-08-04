<#
.SYNOPSIS
Installs the Nrynet Server Windows service.

.PARAMETER Proxy
HTTP(S) or SOCKS5h proxy URL used for package installation and GitHub downloads. Both
`-Proxy URL` and `--proxy URL` are accepted.
#>
[CmdletBinding(PositionalBinding = $false)]
param(
    [string]$PublicHost = $env:COMPUTERNAME,
    [string]$Version = "latest",
    [string]$InstallDir = "$env:ProgramFiles\Nrynet",
    [string]$DataDir = "$env:ProgramData\Nrynet",
    [string]$AdminUser = "admin",
    [switch]$ForceConfig,
    [switch]$RenewCertificate,
    [switch]$AllowDowngrade,
    [switch]$SkipFirewall,
    [switch]$EnableWS,
    [string]$Proxy,
    [Parameter(ValueFromRemainingArguments = $true)]
    [string[]]$ExtraArgs
)

$ErrorActionPreference = "Stop"
$Repository = "nrytex/nrynet"
$ServiceName = "NrynetServer"
$LegacyServiceName = "NATLinkServer"
$DefaultInstallDir = "$env:ProgramFiles\Nrynet"
$DefaultDataDir = "$env:ProgramData\Nrynet"
$LegacyInstallDir = "$env:ProgramFiles\NAT-Link"
$LegacyDataDir = "$env:ProgramData\NAT-Link"

while ($ExtraArgs) {
    if ($ExtraArgs[0] -eq "--proxy" -and $ExtraArgs.Count -ge 2 -and -not $Proxy) {
        $Proxy = $ExtraArgs[1]
        $ExtraArgs = @($ExtraArgs | Select-Object -Skip 2)
    } elseif ($ExtraArgs[0] -eq "--enable-ws") {
        $EnableWS = $true
        $ExtraArgs = @($ExtraArgs | Select-Object -Skip 1)
    } else {
        throw "Unknown arguments. Use -Proxy URL, --proxy URL, -EnableWS, or --enable-ws."
    }
}

function Assert-Administrator {
    $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
    $principal = [Security.Principal.WindowsPrincipal]::new($identity)
    if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
        throw "Run PowerShell as Administrator, then run this installer again."
    }
}

function Find-OpenSSL([string]$ProxyUri) {
    $command = Get-Command openssl.exe -ErrorAction SilentlyContinue
    if ($command) { return $command.Source }
    $knownPaths = @(
        "$env:ProgramFiles\OpenSSL-Win64\bin\openssl.exe",
        "$env:ProgramFiles\OpenSSL-Win32\bin\openssl.exe"
    )
    foreach ($path in $knownPaths) {
        if (Test-Path -LiteralPath $path) { return $path }
    }
    $winget = Get-Command winget.exe -ErrorAction SilentlyContinue
    if (-not $winget) {
        throw "OpenSSL is required. Install OpenSSL and ensure openssl.exe is in PATH."
    }
    Write-Host "Installing OpenSSL with winget..."
    $arguments = @(
        "install", "--id", "ShiningLight.OpenSSL.Light", "--exact", "--silent",
        "--accept-package-agreements", "--accept-source-agreements"
    )
    if ($ProxyUri) { $arguments += @("--proxy", $ProxyUri) }
    & $winget.Source @arguments
    if ($LASTEXITCODE -ne 0) { throw "winget could not install OpenSSL." }
    foreach ($path in $knownPaths) {
        if (Test-Path -LiteralPath $path) { return $path }
    }
    throw "OpenSSL was installed but openssl.exe could not be located."
}

function Resolve-Proxy([string]$Value) {
    if ([string]::IsNullOrWhiteSpace($Value)) { return $null }
    $candidate = $Value.Trim()
    if ($candidate -notmatch '^[a-zA-Z][a-zA-Z0-9+.-]*://') {
        $candidate = "http://$candidate"
    }
    $uri = $null
    if (-not [uri]::TryCreate($candidate, [UriKind]::Absolute, [ref]$uri) -or
        $uri.Scheme -notin @("http", "https", "socks5h") -or -not $uri.Host) {
        throw "Proxy must be a valid HTTP(S) or SOCKS5h proxy URL."
    }
    return $uri.AbsoluteUri.TrimEnd('/')
}

function Invoke-Download([string]$Uri, [string]$OutFile, [string]$ProxyUri) {
    if ($ProxyUri -and ([uri]$ProxyUri).Scheme -eq "socks5h") {
        $curl = Get-Command curl.exe -ErrorAction SilentlyContinue
        if (-not $curl) {
            throw "curl.exe is required when using a SOCKS5h proxy."
        }
        & $curl.Source --fail --location --retry 3 --proxy $ProxyUri --output $OutFile $Uri
        if ($LASTEXITCODE -ne 0) { throw "Download through the SOCKS5h proxy failed." }
        return
    }
    if ($ProxyUri) {
        Invoke-WebRequest -UseBasicParsing -Uri $Uri -OutFile $OutFile -Proxy $ProxyUri
        return
    }
    Invoke-WebRequest -UseBasicParsing -Uri $Uri -OutFile $OutFile
}

function Get-DownloadBase {
    if ($Version -eq "latest") {
        return "https://github.com/$Repository/releases/latest/download"
    }
    $tag = if ($Version.StartsWith("v")) { $Version } else { "v$Version" }
    return "https://github.com/$Repository/releases/download/$tag"
}

function ConvertTo-YamlPath([string]$Path) {
    return $Path.Replace("\", "/")
}

function New-RandomHex([string]$OpenSSL, [int]$Bytes) {
    $value = (& $OpenSSL rand -hex $Bytes | Out-String).Trim()
    if ($LASTEXITCODE -ne 0 -or -not $value) { throw "OpenSSL failed to generate a secret." }
    return $value
}

function Test-PlainEnabledPresent([string]$ConfigPath) {
    if (-not (Test-Path -LiteralPath $ConfigPath)) { return $false }
    $raw = Get-Content -Raw -LiteralPath $ConfigPath
    return $raw -match '(?m)^\s+plain_enabled:\s*(true|false)\s*$'
}

function Test-PlainEnabledTrue([string]$ConfigPath) {
    if (-not (Test-Path -LiteralPath $ConfigPath)) { return $false }
    $raw = Get-Content -Raw -LiteralPath $ConfigPath
    return $raw -match '(?m)^\s+plain_enabled:\s*true\s*$'
}

function Test-PlainPairConfigured([string]$ConfigPath) {
    if (-not (Test-Path -LiteralPath $ConfigPath)) { return $false }
    $raw = Get-Content -Raw -LiteralPath $ConfigPath
    return $raw -match '(?m)^\s+plain_listen:\s*"[^"]+"\s*$' -and
        $raw -match '(?m)^\s+plain_data_listen:\s*"[^"]+"\s*$'
}

function Sync-PlainWSConfig([string]$ConfigPath, [bool]$EnablePreset) {
    if (-not (Test-Path -LiteralPath $ConfigPath)) { return }
    $desired = $EnablePreset -or (Test-PlainEnabledTrue $ConfigPath) -or
        ((-not (Test-PlainEnabledPresent $ConfigPath)) -and (Test-PlainPairConfigured $ConfigPath))
    $lines = @(Get-Content -LiteralPath $ConfigPath)
    $output = [System.Collections.Generic.List[string]]::new()
    $inServer = $false
    $seenPlainEnabled = $false
    $seenPlainListen = $false
    $seenPlainData = $false
    $enabledText = if ($desired) { "true" } else { "false" }

    function Add-MissingPlainFields {
        if ($inServer) {
            if (-not $seenPlainEnabled) { $output.Add("  plain_enabled: $enabledText") }
            if (-not $seenPlainListen) { $output.Add('  plain_listen: "0.0.0.0:7004"') }
            if (-not $seenPlainData) { $output.Add('  plain_data_listen: "0.0.0.0:7005"') }
        }
    }

    foreach ($line in $lines) {
        if ($line -match '^server:\s*$') {
            $output.Add($line)
            $inServer = $true
            $seenPlainEnabled = $false
            $seenPlainListen = $false
            $seenPlainData = $false
            continue
        }
        if ($inServer -and $line -match '^\S[^:]*:') {
            Add-MissingPlainFields
            $inServer = $false
        }
        if ($inServer -and $line -match '^\s+plain_enabled:') {
            $output.Add("  plain_enabled: $enabledText")
            $seenPlainEnabled = $true
            continue
        }
        if ($inServer -and $line -match '^\s+plain_listen:') {
            if ($line -match '^\s+plain_listen:\s*""\s*$') {
                $output.Add('  plain_listen: "0.0.0.0:7004"')
            } else {
                $output.Add($line)
            }
            $seenPlainListen = $true
            continue
        }
        if ($inServer -and $line -match '^\s+plain_data_listen:') {
            if ($line -match '^\s+plain_data_listen:\s*""\s*$') {
                $output.Add('  plain_data_listen: "0.0.0.0:7005"')
            } else {
                $output.Add($line)
            }
            $seenPlainData = $true
            continue
        }
        $output.Add($line)
    }
    Add-MissingPlainFields

    $tempPath = "$ConfigPath.tmp.$PID"
    Set-Content -LiteralPath $tempPath -Value $output -Encoding UTF8
    Move-Item -LiteralPath $tempPath -Destination $ConfigPath -Force
}

function Test-WSConfigEnabled([string]$ConfigPath) {
    return (Test-PlainEnabledTrue $ConfigPath) -and (Test-PlainPairConfigured $ConfigPath)
}

function Test-TLSConfigEnabled([string]$ConfigPath) {
    if (-not (Test-Path -LiteralPath $ConfigPath)) { return $false }
    $inTLS = $false
    foreach ($line in Get-Content -LiteralPath $ConfigPath) {
        if ($line -match '^\s{2}tls:\s*$') { $inTLS = $true; continue }
        if ($inTLS -and $line -match '^\s{2}\S') { $inTLS = $false }
        if ($inTLS -and $line -match '^\s{4}enabled:\s*true\s*$') { return $true }
    }
    return $false
}

function Move-LegacyInstallation {
    if ($InstallDir -ne $DefaultInstallDir -or $DataDir -ne $DefaultDataDir) { return $false }
    $legacyService = Get-Service -Name $LegacyServiceName -ErrorAction SilentlyContinue
    if ($legacyService -and $legacyService.Status -ne "Stopped") {
        Stop-Service -Name $LegacyServiceName -Force
        $legacyService.WaitForStatus("Stopped", [TimeSpan]::FromSeconds(20))
    }
    $migrated = $false
    if ((Test-Path -LiteralPath $LegacyInstallDir) -and -not (Test-Path -LiteralPath $InstallDir)) {
        Move-Item -LiteralPath $LegacyInstallDir -Destination $InstallDir
        $migrated = $true
    }
    if ((Test-Path -LiteralPath $LegacyDataDir) -and -not (Test-Path -LiteralPath $DataDir)) {
        Move-Item -LiteralPath $LegacyDataDir -Destination $DataDir
        $migrated = $true
    }
    $legacyBinary = Join-Path $InstallDir "nat-link-server.exe"
    $serverBinary = Join-Path $InstallDir "nrynet-server.exe"
    if ((Test-Path -LiteralPath $legacyBinary) -and -not (Test-Path -LiteralPath $serverBinary)) {
        Move-Item -LiteralPath $legacyBinary -Destination $serverBinary
        $migrated = $true
    }
    $legacyDatabase = Join-Path $DataDir "data\nat-link.db"
    $database = Join-Path $DataDir "data\nrynet.db"
    if ((Test-Path -LiteralPath $legacyDatabase) -and -not (Test-Path -LiteralPath $database)) {
        Move-Item -LiteralPath $legacyDatabase -Destination $database
        $migrated = $true
    }
    $config = Join-Path $DataDir "config.yaml"
    if ($migrated -and (Test-Path -LiteralPath $config)) {
        $content = Get-Content -Raw -LiteralPath $config
        $content = $content.Replace((ConvertTo-YamlPath $LegacyDataDir), (ConvertTo-YamlPath $DataDir))
        $content = $content.Replace("nat-link.db", "nrynet.db")
        Set-Content -LiteralPath $config -Value $content -Encoding UTF8
    }
    if ($legacyService) {
        & sc.exe delete $LegacyServiceName | Out-Null
        if ($LASTEXITCODE -ne 0) { throw "Could not remove the legacy Windows service." }
        $migrated = $true
    }
    Remove-NetFirewallRule -DisplayName "NAT-Link TCP" -ErrorAction SilentlyContinue
    Remove-NetFirewallRule -DisplayName "NAT-Link UDP" -ErrorAction SilentlyContinue
    return $migrated
}

function Assert-LegacyMigrationIsSafe {
    if ($InstallDir -ne $DefaultInstallDir -or $DataDir -ne $DefaultDataDir) { return }
    if ((Test-Path -LiteralPath $LegacyInstallDir) -and (Test-Path -LiteralPath $InstallDir)) {
        throw "Both $LegacyInstallDir and $InstallDir exist; resolve the legacy installation manually before continuing."
    }
    if ((Test-Path -LiteralPath $LegacyDataDir) -and (Test-Path -LiteralPath $DataDir)) {
        throw "Both $LegacyDataDir and $DataDir exist; resolve the legacy data directory manually before continuing."
    }
    foreach ($dataPath in @($LegacyDataDir, $DataDir)) {
        $legacyDatabase = Join-Path $dataPath "data\nat-link.db"
        $database = Join-Path $dataPath "data\nrynet.db"
        if ((Test-Path -LiteralPath $legacyDatabase) -and (Test-Path -LiteralPath $database)) {
            throw "Both nat-link.db and nrynet.db exist in $dataPath; resolve the database conflict manually before continuing."
        }
    }
}

function Assert-NoVersionDowngrade([version]$TargetVersion) {
    $serverExe = Join-Path $InstallDir "nrynet-server.exe"
    if (-not (Test-Path -LiteralPath $serverExe)) {
        $serverExe = Join-Path $InstallDir "nat-link-server.exe"
    }
    if ($InstallDir -eq $DefaultInstallDir -and -not (Test-Path -LiteralPath $serverExe)) {
        $serverExe = Join-Path $LegacyInstallDir "nat-link-server.exe"
    }
    if (-not (Test-Path -LiteralPath $serverExe)) { return }
    $installedVersionText = (& $serverExe -version 2>$null | Out-String).Trim().TrimStart("v")
    if (-not $installedVersionText) { return }
    try { $installedVersion = [version]$installedVersionText } catch { return }
    if ($installedVersion -gt $TargetVersion -and -not $AllowDowngrade) {
        throw "Installed version $installedVersion is newer than requested $TargetVersion. Use -AllowDowngrade only for an intentional rollback."
    }
}

Assert-Administrator
if ($PublicHost -notmatch '^[A-Za-z0-9.-]+$') {
    throw "PublicHost must be a DNS name or IPv4 address."
}
if ($AdminUser -notmatch '^[A-Za-z0-9_.-]+$') {
    throw "AdminUser may contain only letters, numbers, dot, underscore and hyphen."
}
$ResolvedProxy = Resolve-Proxy $Proxy
if ($ResolvedProxy) {
    $env:HTTP_PROXY = $ResolvedProxy
    $env:HTTPS_PROXY = $ResolvedProxy
    $env:ALL_PROXY = $ResolvedProxy
    $env:http_proxy = $ResolvedProxy
    $env:https_proxy = $ResolvedProxy
    $env:all_proxy = $ResolvedProxy
}
$OpenSSL = Find-OpenSSL $ResolvedProxy
$Asset = "nrynet-windows-amd64.zip"
$DownloadBase = Get-DownloadBase
$TempDir = Join-Path ([IO.Path]::GetTempPath()) ("nrynet-install-" + [guid]::NewGuid())
$Archive = Join-Path $TempDir $Asset
$ChecksumFile = Join-Path $TempDir "SHA256SUMS"

try {
    New-Item -ItemType Directory -Force -Path $TempDir | Out-Null
    Write-Host "Downloading Nrynet Server ($Version)..."
    Invoke-Download "$DownloadBase/$Asset" $Archive $ResolvedProxy
    Invoke-Download "$DownloadBase/SHA256SUMS" $ChecksumFile $ResolvedProxy
    $checksumLine = Get-Content -LiteralPath $ChecksumFile | Where-Object { $_ -match "\s$([regex]::Escape($Asset))$" } | Select-Object -First 1
    if (-not $checksumLine) { throw "Release checksum for $Asset was not found." }
    $expected = ($checksumLine -split '\s+')[0].ToLowerInvariant()
    $actual = (Get-FileHash -LiteralPath $Archive -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actual -ne $expected) { throw "Release checksum verification failed." }

    $PackageDir = Join-Path $TempDir "package"
    Expand-Archive -LiteralPath $Archive -DestinationPath $PackageDir -Force
    $PackagedServer = Join-Path $PackageDir "nrynet-server.exe"
    if (-not (Test-Path -LiteralPath $PackagedServer)) { throw "Release archive is missing nrynet-server.exe." }
    $PackageVersionFile = Join-Path $PackageDir "VERSION"
    if (-not (Test-Path -LiteralPath $PackageVersionFile)) { throw "Release archive is missing VERSION." }
    $targetVersionText = (Get-Content -Raw -LiteralPath $PackageVersionFile).Trim().TrimStart("v")
    try { $targetVersion = [version]$targetVersionText } catch { throw "Release version '$targetVersionText' is invalid." }

    Assert-LegacyMigrationIsSafe
    Assert-NoVersionDowngrade $targetVersion
    $migratedLegacy = Move-LegacyInstallation

    New-Item -ItemType Directory -Force -Path $InstallDir, $DataDir | Out-Null
    $DatabaseDir = Join-Path $DataDir "data"
    $LogDir = Join-Path $DataDir "logs"
    $TlsDir = Join-Path $DataDir "tls"
    New-Item -ItemType Directory -Force -Path $DatabaseDir, $LogDir, $TlsDir | Out-Null
    $ServerExe = Join-Path $InstallDir "nrynet-server.exe"
    $installedVersionText = ""
    if (Test-Path -LiteralPath $ServerExe) {
        $installedVersionText = (& $ServerExe -version 2>$null | Out-String).Trim().TrimStart("v")
    }
    $replaceBinary = $true
    if ($installedVersionText) {
        try { $installedVersion = [version]$installedVersionText } catch { $installedVersion = [version]"0.0.0" }
        if ($installedVersion -eq $targetVersion -and -not $migratedLegacy) {
            $replaceBinary = $false
            Write-Host "Nrynet Server $targetVersionText is already installed."
        } elseif ($installedVersion -gt $targetVersion -and -not $AllowDowngrade) {
            throw "Installed version $installedVersion is newer than requested $targetVersion. Use -AllowDowngrade only for an intentional rollback."
        } else {
            Write-Host "Upgrading Nrynet Server from $installedVersion to $targetVersion..."
        }
    } else {
        Write-Host "Installing Nrynet Server $targetVersion..."
    }
    $existingService = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
    if ($replaceBinary) {
        if ($existingService -and $existingService.Status -ne "Stopped") {
            Stop-Service -Name $ServiceName -Force
        }
        Copy-Item -LiteralPath $PackagedServer -Destination $ServerExe -Force
    }

    $CertFile = Join-Path $TlsDir "fullchain.pem"
    $KeyFile = Join-Path $TlsDir "privkey.pem"
    if ($RenewCertificate -or -not (Test-Path $CertFile) -or -not (Test-Path $KeyFile)) {
        $primarySan = if ($PublicHost -match '^\d+(\.\d+){3}$') { "IP:$PublicHost" } else { "DNS:$PublicHost" }
        Write-Host "Generating a self-signed TLS certificate with OpenSSL..."
        & $OpenSSL req -x509 -newkey rsa:3072 -sha256 -nodes -days 825 `
            -keyout $KeyFile -out $CertFile -subj "/CN=$PublicHost" `
            -addext "subjectAltName=$primarySan,DNS:localhost,IP:127.0.0.1"
        if ($LASTEXITCODE -ne 0) { throw "OpenSSL certificate generation failed." }
    }

    $ConfigFile = Join-Path $DataDir "config.yaml"
    $DatabaseFile = Join-Path $DatabaseDir "nrynet.db"
    $isNewDatabase = -not (Test-Path -LiteralPath $DatabaseFile)
    $initialPassword = ""
    if ($ForceConfig -or -not (Test-Path -LiteralPath $ConfigFile)) {
        $relayToken = New-RandomHex $OpenSSL 32
        if ($isNewDatabase) { $initialPassword = New-RandomHex $OpenSSL 18 }
        $yamlDatabase = ConvertTo-YamlPath $DatabaseFile
        $yamlLogs = ConvertTo-YamlPath $LogDir
        $yamlCert = ConvertTo-YamlPath $CertFile
        $yamlKey = ConvertTo-YamlPath $KeyFile
        $plainConfig = '  plain_enabled: false
  plain_listen: "0.0.0.0:7004"
  plain_data_listen: "0.0.0.0:7005"'
        if ($EnableWS) {
            $plainConfig = '  plain_enabled: true
  plain_listen: "0.0.0.0:7004"
  plain_data_listen: "0.0.0.0:7005"'
        }
        $config = @"
server:
  listen: "0.0.0.0:7000"
  data_listen: "0.0.0.0:7001"
$plainConfig
  public_data_address: "${PublicHost}:7001"
  quic_listen: "0.0.0.0:7002"
  public_quic_address: "${PublicHost}:7002"
  rendezvous_listen: "0.0.0.0:7003"
  public_rendezvous_address: "${PublicHost}:7003"
  relay_api_token: "$relayToken"
  http_listen: "0.0.0.0:8080"
  database: "$yamlDatabase"
  log_directory: "$yamlLogs"
  jwt_ttl: "12h"
  heartbeat_timeout: "45s"
  tls:
    enabled: false
    cert_file: "$yamlCert"
    key_file: "$yamlKey"
  bootstrap:
    admin_username: "$AdminUser"
    admin_password: "$initialPassword"
client:
  server_url: "ws://${PublicHost}:7000/agent/connect"
  data_address: "${PublicHost}:7001"
  transport: "websocket"
  quic_address: "${PublicHost}:7002"
  rendezvous_address: "${PublicHost}:7003"
  ca_file: ""
  token: ""
  name: ""
  device_id: ""
  insecure_skip_verify: false
"@
        Set-Content -LiteralPath $ConfigFile -Value $config -Encoding UTF8
    }
    Sync-PlainWSConfig $ConfigFile $EnableWS
    $wsConfigEnabled = Test-WSConfigEnabled $ConfigFile
    $tlsConfigEnabled = Test-TLSConfigEnabled $ConfigFile

    $existingService = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
    $binaryPath = "`"$ServerExe`" -config `"$ConfigFile`""
    if ($existingService) {
        if ($existingService.Status -ne "Stopped") { Stop-Service -Name $ServiceName -Force }
        & sc.exe config $ServiceName "binPath= $binaryPath" "start= auto" | Out-Null
        if ($LASTEXITCODE -ne 0) { throw "Could not update the Windows service." }
    } else {
        New-Service -Name $ServiceName -BinaryPathName $binaryPath -DisplayName "Nrynet Server" `
            -Description "Nrynet self-hosted relay server" -StartupType Automatic | Out-Null
    }

    if (-not $SkipFirewall) {
        # Reserve the optional WS ports so a later Dashboard toggle works after
        # restarting the service; plain_enabled still controls whether anything listens.
        $tcpPorts = "7000,7001,7004,7005,8080"
        $rules = @(
            @{ Name = "Nrynet TCP"; Protocol = "TCP"; Ports = $tcpPorts },
            @{ Name = "Nrynet UDP"; Protocol = "UDP"; Ports = "7002,7003" }
        )
        foreach ($rule in $rules) {
            Remove-NetFirewallRule -DisplayName $rule.Name -ErrorAction SilentlyContinue
            New-NetFirewallRule -DisplayName $rule.Name -Direction Inbound -Action Allow `
                -Protocol $rule.Protocol -LocalPort $rule.Ports | Out-Null
        }
    }

    Start-Service -Name $ServiceName
    (Get-Service -Name $ServiceName).WaitForStatus("Running", [TimeSpan]::FromSeconds(20))
    Start-Sleep -Seconds 2
    if ((Get-Service -Name $ServiceName).Status -ne "Running") { throw "Nrynet Server failed to start." }
    if ($initialPassword) {
        $rawConfig = Get-Content -Raw -LiteralPath $ConfigFile
        $escapedPassword = [regex]::Escape($initialPassword)
        $rawConfig = $rawConfig -replace "admin_password: `"$escapedPassword`"", 'admin_password: ""'
        Set-Content -LiteralPath $ConfigFile -Value $rawConfig -Encoding UTF8
    }

    Write-Host ""
    Write-Host "Nrynet Server HTTP console is running: http://${PublicHost}:7000"
    if ($tlsConfigEnabled) { Write-Host "Nrynet Server HTTPS console is running: https://${PublicHost}:7000" }
    if ($wsConfigEnabled) { Write-Host "Plaintext console/control is enabled: http://${PublicHost}:7004" }
    Write-Host "Installed version: $targetVersionText"
    Write-Host "Available self-signed certificate: $CertFile"
    if ($initialPassword) {
        Write-Host "Administrator: $AdminUser"
        Write-Host "Initial password: $initialPassword"
        Write-Host "Record this password now; it has been removed from config.yaml."
    }
    if ($tlsConfigEnabled) {
        Write-Host "Self-signed TLS certificates are pinned into new Agent Tokens; trusted CA certificates need no ca_file."
    } else {
        Write-Host "TLS is disabled by default. It can be enabled later with a configured certificate."
    }
} finally {
    if (Test-Path -LiteralPath $TempDir) { Remove-Item -LiteralPath $TempDir -Recurse -Force }
}
