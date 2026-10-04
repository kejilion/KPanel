# Windows release assets are unsigned; SHA256SUMS covers their final bytes.
[CmdletBinding()]
param(
    [ValidateSet('Check', 'Build', 'Verify')][string]$Mode = 'Check',
    [string]$OutputDirectory,
    [string]$Version = $env:GITHUB_REF_NAME
)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$script:WindowsNodeAssets = @('kejilion-node-windows-amd64.exe', 'kejilion-node-windows-arm64.exe', 'install-windows.ps1')

function Assert-NodeReleaseVersion([string]$Value) {
    if ($Value -cnotmatch '^v(0|[1-9][0-9]{0,5})\.(0|[1-9][0-9]{0,5})\.(0|[1-9][0-9]{0,5})(-rc\.[1-9][0-9]{0,5})?$') { throw 'Expected a canonical vX.Y.Z or vX.Y.Z-rc.N release tag.' }
    if ($Value.Substring(1) -cne (Get-Content -LiteralPath (Join-Path $PSScriptRoot '../VERSION') -Raw).Trim()) { throw 'Release tag does not match VERSION.' }
}

function Get-NodeReleaseDirectory([string]$Path) {
    if (-not [IO.Path]::IsPathRooted($Path)) { throw 'OutputDirectory must be absolute.' }
    $item = Get-Item -LiteralPath $Path -Force
    if (-not $item.PSIsContainer -or ($item.Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw 'Expected a regular output directory.' }
    return $item.FullName
}

function Build-NodeRelease([string]$Directory, [string]$Tag) {
    Assert-NodeReleaseVersion $Tag
    $Directory = Get-NodeReleaseDirectory $Directory
    if (@(Get-ChildItem -LiteralPath $Directory -Force).Count -ne 0) { throw 'Build output directory must be empty; stale outputs are not reusable.' }
    $previous = @{}
    foreach ($name in @('GOOS', 'GOARCH', 'CGO_ENABLED')) { $previous[$name] = [Environment]::GetEnvironmentVariable($name) }
    Push-Location (Join-Path $PSScriptRoot '..')
    try {
        $env:GOOS = 'windows'; $env:CGO_ENABLED = '0'
        foreach ($arch in @('amd64', 'arm64')) {
            $env:GOARCH = $arch
            & go build -trimpath -ldflags "-s -w -X github.com/kejilion/kejilion-panel/internal/version.Version=$($Tag.Substring(1))" -o (Join-Path $Directory "kejilion-node-windows-$arch.exe") ./cmd/kejilion-node
            if ($LASTEXITCODE -ne 0) { throw "Windows $arch node build failed." }
        }
        $installer = [IO.File]::ReadAllText((Join-Path $PSScriptRoot '../deploy/windows/install.ps1')) -replace '\r?\n', "`r`n"
        # Establish final PowerShell encoding before computing the final release checksum.
        [IO.File]::WriteAllText((Join-Path $Directory 'install-windows.ps1'), $installer, [Text.UTF8Encoding]::new($true))
    } finally {
        Pop-Location
        foreach ($name in $previous.Keys) { [Environment]::SetEnvironmentVariable($name, $previous[$name]) }
    }
}

function Verify-NodeRelease([string]$Directory) {
    $Directory = Get-NodeReleaseDirectory $Directory
    $manifest = Join-Path $Directory 'SHA256SUMS.windows'
    if (Test-Path -LiteralPath $manifest) { Remove-Item -LiteralPath $manifest -Force }
    $items = @(Get-ChildItem -LiteralPath $Directory -Force)
    if ($items.Count -ne $script:WindowsNodeAssets.Count) { throw 'Expected exactly the two node executables and installer.' }
    $lines = foreach ($name in $script:WindowsNodeAssets) {
        $path = Join-Path $Directory $name
        $item = Get-Item -LiteralPath $path -Force
        $limit = if ($name -eq 'install-windows.ps1') { 1MB } else { 64MB }
        if ($item.PSIsContainer -or ($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -or $item.Length -le 0 -or $item.Length -gt $limit) { throw "Invalid release asset: $name" }
        "$( (Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLowerInvariant() )  $name"
    }
    [IO.File]::WriteAllText($manifest, (($lines -join "`n") + "`n"), [Text.UTF8Encoding]::new($false))
}

if ($MyInvocation.InvocationName -eq '.') { return }
Assert-NodeReleaseVersion $Version
switch ($Mode) {
    'Check' { }
    'Build' { Build-NodeRelease $OutputDirectory $Version }
    'Verify' { Verify-NodeRelease $OutputDirectory }
}
