# Build outputs are private intermediates. Only Verify produces a release manifest.
[CmdletBinding()]
param(
    [ValidateSet('Check', 'Build', 'Verify')][string]$Mode = 'Check',
    [string]$OutputDirectory,
    [string]$Version = $env:GITHUB_REF_NAME,
    [string]$Publisher = $env:KPANEL_WINDOWS_NODE_PUBLISHER,
    [string]$ProfileOID = $env:KPANEL_WINDOWS_NODE_PROFILE_OID
)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$script:WindowsNodeAssets = @('kejilion-node-windows-amd64.exe', 'kejilion-node-windows-arm64.exe', 'install-windows.ps1')

function Assert-NodeReleaseVersion([string]$Value) {
    if ($Value -cnotmatch '^v(0|[1-9][0-9]{0,5})\.(0|[1-9][0-9]{0,5})\.(0|[1-9][0-9]{0,5})(-rc\.[1-9][0-9]{0,5})?$') { throw 'Expected a canonical vX.Y.Z or vX.Y.Z-rc.N release tag.' }
    if ($Value.Substring(1) -cne (Get-Content -LiteralPath (Join-Path $PSScriptRoot '../VERSION') -Raw).Trim()) { throw 'Release tag does not match VERSION.' }
}

function Assert-NodePublisher([string]$Subject, [string]$OID) {
    if ([string]::IsNullOrWhiteSpace($Subject) -or $Subject.Length -gt 1024 -or $Subject -match '[\r\n\x00]') { throw 'An exact trusted publisher Subject is required.' }
    if ($OID -and ($OID.Length -gt 256 -or $OID -cnotmatch '^[0-2](\.[0-9]+)+$')) { throw 'Invalid Artifact Signing profile OID.' }
}

function Assert-NodeSigningConfiguration {
    foreach ($name in @('KPANEL_AZURE_TENANT_ID', 'KPANEL_AZURE_CLIENT_ID', 'KPANEL_AZURE_CLIENT_SECRET', 'KPANEL_SIGNING_ACCOUNT', 'KPANEL_SIGNING_PROFILE')) {
        if ([string]::IsNullOrWhiteSpace([Environment]::GetEnvironmentVariable($name))) { throw "Missing signing configuration: $name" }
    }
    $endpoint = [Environment]::GetEnvironmentVariable('KPANEL_SIGNING_ENDPOINT')
    if ($endpoint -cnotmatch '^https://[a-z0-9-]+\.(code|artifact)signing\.azure\.net/?$') { throw 'Expected a regional HTTPS Azure Artifact Signing endpoint.' }
}

function Assert-NodeSignature([string]$Path, [string]$Subject, [string]$OID) {
    Assert-NodePublisher $Subject $OID
    $signature = Get-AuthenticodeSignature -LiteralPath $Path
    if ($signature.Status -ne 'Valid' -or $null -eq $signature.SignerCertificate -or $signature.SignerCertificate.Subject -cne $Subject) { throw "Authenticode chain or publisher verification failed: $Path" }
    if ($null -eq $signature.TimeStamperCertificate) { throw "A trusted RFC3161 timestamp is required: $Path" }
    $eku = @($signature.SignerCertificate.Extensions | Where-Object { $_.Oid.Value -eq '2.5.29.37' } | ForEach-Object { $_.EnhancedKeyUsages } | ForEach-Object { $_.Value })
    if ($eku -cnotcontains '1.3.6.1.5.5.7.3.3' -or ($OID -and $eku -cnotcontains $OID)) { throw "Code signing or profile EKU mismatch: $Path" }
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
        # Establish final PowerShell encoding before signing; never normalize signed bytes.
        [IO.File]::WriteAllText((Join-Path $Directory 'install-windows.ps1'), $installer, [Text.UTF8Encoding]::new($true))
    } finally {
        Pop-Location
        foreach ($name in $previous.Keys) { [Environment]::SetEnvironmentVariable($name, $previous[$name]) }
    }
}

function Verify-NodeRelease([string]$Directory, [string]$Subject, [string]$OID) {
    Assert-NodePublisher $Subject $OID
    $Directory = Get-NodeReleaseDirectory $Directory
    $manifest = Join-Path $Directory 'SHA256SUMS.windows'
    if (Test-Path -LiteralPath $manifest) { Remove-Item -LiteralPath $manifest -Force }
    $items = @(Get-ChildItem -LiteralPath $Directory -Force)
    if ($items.Count -ne $script:WindowsNodeAssets.Count) { throw 'Expected exactly the two node executables and installer.' }
    $lines = foreach ($name in $script:WindowsNodeAssets) {
        $path = Join-Path $Directory $name
        $item = Get-Item -LiteralPath $path -Force
        if ($item.PSIsContainer -or ($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -or $item.Length -le 0 -or $item.Length -gt 256MB) { throw "Invalid release asset: $name" }
        Assert-NodeSignature $path $Subject $OID
        "$( (Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLowerInvariant() )  $name"
    }
    # The checksum covers the complete signed file, including its timestamp.
    [IO.File]::WriteAllText($manifest, (($lines -join "`n") + "`n"), [Text.UTF8Encoding]::new($false))
}

# Dot sourcing exposes functions to local tests; release jobs always use an explicit mode.
if ($MyInvocation.InvocationName -eq '.') { return }
if ([Environment]::OSVersion.Platform -ne [PlatformID]::Win32NT) { throw 'A native Windows signing runner is required.' }
Assert-NodeReleaseVersion $Version
switch ($Mode) {
    'Check' { Assert-NodePublisher $Publisher $ProfileOID; Assert-NodeSigningConfiguration }
    'Build' { Build-NodeRelease $OutputDirectory $Version }
    'Verify' { Verify-NodeRelease $OutputDirectory $Publisher $ProfileOID }
}
