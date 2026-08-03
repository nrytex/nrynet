[CmdletBinding()]
param(
    [string]$PublicHost = $env:COMPUTERNAME,
    [string]$Version = "latest",
    [string]$InstallDir = "$env:ProgramFiles\NAT-Link",
    [string]$DataDir = "$env:ProgramData\NAT-Link",
    [string]$AdminUser = "admin",
    [switch]$ForceConfig,
    [switch]$RenewCertificate,
    [switch]$SkipFirewall
)

$ErrorActionPreference = "Stop"
$Repository = "nrytex/nrynet"
$ServiceName = "NATLinkServer"

function Assert-Administrator {
    $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
    $principal = [Security.Principal.WindowsPrincipal]::new($identity)
    if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
        throw "Run PowerShell as Administrator, then run this installer again."
    }
}

function Find-OpenSSL {
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
    & $winget.Source install --id ShiningLight.OpenSSL.Light --exact --silent `
        --accept-package-agreements --accept-source-agreements
    if ($LASTEXITCODE -ne 0) { throw "winget could not install OpenSSL." }
    foreach ($path in $knownPaths) {
        if (Test-Path -LiteralPath $path) { return $path }
    }
    throw "OpenSSL was installed but openssl.exe could not be located."
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

Assert-Administrator
if ($PublicHost -notmatch '^[A-Za-z0-9.-]+$') {
    throw "PublicHost must be a DNS name or IPv4 address."
}
if ($AdminUser -notmatch '^[A-Za-z0-9_.-]+$') {
    throw "AdminUser may contain only letters, numbers, dot, underscore and hyphen."
}
$OpenSSL = Find-OpenSSL
$Asset = "nat-link-windows-amd64.zip"
$DownloadBase = Get-DownloadBase
$TempDir = Join-Path ([IO.Path]::GetTempPath()) ("nat-link-install-" + [guid]::NewGuid())
$Archive = Join-Path $TempDir $Asset
$ChecksumFile = Join-Path $TempDir "SHA256SUMS"

try {
    New-Item -ItemType Directory -Force -Path $TempDir | Out-Null
    Write-Host "Downloading NAT-Link Server ($Version)..."
    Invoke-WebRequest -UseBasicParsing -Uri "$DownloadBase/$Asset" -OutFile $Archive
    Invoke-WebRequest -UseBasicParsing -Uri "$DownloadBase/SHA256SUMS" -OutFile $ChecksumFile
    $checksumLine = Get-Content -LiteralPath $ChecksumFile | Where-Object { $_ -match "\s$([regex]::Escape($Asset))$" } | Select-Object -First 1
    if (-not $checksumLine) { throw "Release checksum for $Asset was not found." }
    $expected = ($checksumLine -split '\s+')[0].ToLowerInvariant()
    $actual = (Get-FileHash -LiteralPath $Archive -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actual -ne $expected) { throw "Release checksum verification failed." }

    $PackageDir = Join-Path $TempDir "package"
    Expand-Archive -LiteralPath $Archive -DestinationPath $PackageDir -Force
    $PackagedServer = Join-Path $PackageDir "nat-link-server.exe"
    if (-not (Test-Path -LiteralPath $PackagedServer)) { throw "Release archive is missing nat-link-server.exe." }

    New-Item -ItemType Directory -Force -Path $InstallDir, $DataDir | Out-Null
    $DatabaseDir = Join-Path $DataDir "data"
    $LogDir = Join-Path $DataDir "logs"
    $TlsDir = Join-Path $DataDir "tls"
    New-Item -ItemType Directory -Force -Path $DatabaseDir, $LogDir, $TlsDir | Out-Null
    $ServerExe = Join-Path $InstallDir "nat-link-server.exe"
    Copy-Item -LiteralPath $PackagedServer -Destination $ServerExe -Force

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
    $DatabaseFile = Join-Path $DatabaseDir "nat-link.db"
    $isNewDatabase = -not (Test-Path -LiteralPath $DatabaseFile)
    $initialPassword = ""
    if ($ForceConfig -or -not (Test-Path -LiteralPath $ConfigFile)) {
        $relayToken = New-RandomHex $OpenSSL 32
        if ($isNewDatabase) { $initialPassword = New-RandomHex $OpenSSL 18 }
        $yamlDatabase = ConvertTo-YamlPath $DatabaseFile
        $yamlLogs = ConvertTo-YamlPath $LogDir
        $yamlCert = ConvertTo-YamlPath $CertFile
        $yamlKey = ConvertTo-YamlPath $KeyFile
        $config = @"
server:
  listen: "0.0.0.0:7000"
  data_listen: "0.0.0.0:7001"
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
    enabled: true
    cert_file: "$yamlCert"
    key_file: "$yamlKey"
  bootstrap:
    admin_username: "$AdminUser"
    admin_password: "$initialPassword"
client:
  server_url: "wss://${PublicHost}:7000/agent/connect"
  data_address: "${PublicHost}:7001"
  transport: "websocket"
  quic_address: "${PublicHost}:7002"
  rendezvous_address: "${PublicHost}:7003"
  ca_file: "$yamlCert"
  token: ""
  name: ""
  device_id: ""
  insecure_skip_verify: false
"@
        Set-Content -LiteralPath $ConfigFile -Value $config -Encoding UTF8
    }

    $existingService = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
    $binaryPath = "`"$ServerExe`" -config `"$ConfigFile`""
    if ($existingService) {
        if ($existingService.Status -ne "Stopped") { Stop-Service -Name $ServiceName -Force }
        & sc.exe config $ServiceName "binPath= $binaryPath" "start= auto" | Out-Null
        if ($LASTEXITCODE -ne 0) { throw "Could not update the Windows service." }
    } else {
        New-Service -Name $ServiceName -BinaryPathName $binaryPath -DisplayName "NAT-Link Server" `
            -Description "NAT-Link self-hosted relay server" -StartupType Automatic | Out-Null
    }

    if (-not $SkipFirewall) {
        $rules = @(
            @{ Name = "NAT-Link TCP"; Protocol = "TCP"; Ports = "7000,7001,8080" },
            @{ Name = "NAT-Link UDP"; Protocol = "UDP"; Ports = "7002,7003" }
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
    if ((Get-Service -Name $ServiceName).Status -ne "Running") { throw "NAT-Link Server failed to start." }
    if ($initialPassword) {
        $rawConfig = Get-Content -Raw -LiteralPath $ConfigFile
        $escapedPassword = [regex]::Escape($initialPassword)
        $rawConfig = $rawConfig -replace "admin_password: `"$escapedPassword`"", 'admin_password: ""'
        Set-Content -LiteralPath $ConfigFile -Value $rawConfig -Encoding UTF8
    }

    Write-Host ""
    Write-Host "NAT-Link Server is running: https://${PublicHost}:7000"
    Write-Host "Self-signed CA certificate: $CertFile"
    if ($initialPassword) {
        Write-Host "Administrator: $AdminUser"
        Write-Host "Initial password: $initialPassword"
        Write-Host "Record this password now; it has been removed from config.yaml."
    }
    Write-Host "Copy fullchain.pem to each client and configure it as ca_file."
} finally {
    if (Test-Path -LiteralPath $TempDir) { Remove-Item -LiteralPath $TempDir -Recurse -Force }
}
