param(
    [string]$Version = "dev",
    [string]$Output = "bin"
)

$ErrorActionPreference = "Stop"
$targets = @(
    @{ OS = "linux"; Arch = "amd64" },
    @{ OS = "linux"; Arch = "arm64" },
    @{ OS = "windows"; Arch = "amd64" },
    @{ OS = "darwin"; Arch = "amd64" },
    @{ OS = "darwin"; Arch = "arm64" }
)

foreach ($target in $targets) {
    $env:GOOS = $target.OS
    $env:GOARCH = $target.Arch
    $env:CGO_ENABLED = "0"
    $directory = Join-Path $Output "$Version-$($target.OS)-$($target.Arch)"
    New-Item -ItemType Directory -Force -Path $directory | Out-Null
    $extension = if ($target.OS -eq "windows") { ".exe" } else { "" }
    go build -trimpath -ldflags "-s -w -X main.version=$Version" -o (Join-Path $directory "nrynet-server$extension") ./server
    if ($LASTEXITCODE -ne 0) { throw "server build failed for $($target.OS)/$($target.Arch)" }
    go build -trimpath -ldflags "-s -w -X main.version=$Version" -o (Join-Path $directory "nrynet-client$extension") ./client
    if ($LASTEXITCODE -ne 0) { throw "client build failed for $($target.OS)/$($target.Arch)" }
    go build -trimpath -ldflags "-s -w -X main.version=$Version" -o (Join-Path $directory "nrynet-relay$extension") ./relay
    if ($LASTEXITCODE -ne 0) { throw "relay build failed for $($target.OS)/$($target.Arch)" }
    Copy-Item -LiteralPath "config.example.yaml" -Destination (Join-Path $directory "config.example.yaml") -Force
    Copy-Item -LiteralPath "config.local.example.yaml" -Destination (Join-Path $directory "config.local.example.yaml") -Force
    Set-Content -LiteralPath (Join-Path $directory "VERSION") -Value $Version -Encoding ASCII
}

Remove-Item Env:GOOS, Env:GOARCH, Env:CGO_ENABLED -ErrorAction SilentlyContinue
