$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
. (Join-Path $PSScriptRoot '../windows-node-release.ps1')

function Assert-Failure([scriptblock]$Action, [string]$Pattern) {
    $caught = $false
    try { & $Action } catch {
        if ($_.Exception.Message -notmatch $Pattern) { throw }
        $caught = $true
    }
    if (-not $caught) { throw "Expected failure: $Pattern" }
}

$testDirectory = Join-Path ([IO.Path]::GetTempPath()) ('KPanelReleaseTests-' + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $testDirectory | Out-Null
try {
    $version = 'v' + (Get-Content (Join-Path $PSScriptRoot '../../VERSION') -Raw).Trim()
    Assert-NodeReleaseVersion $version
    foreach ($badVersion in @('v01.2.3', 'v1.2.3-rc.0', 'v1.2.3-dev', 'v1000000.2.3', 'v1.2.3;whoami')) {
        Assert-Failure { Assert-NodeReleaseVersion $badVersion } 'canonical'
    }
    Assert-Failure { Assert-NodeReleaseVersion 'v999999.999999.999999' } 'does not match'
    Assert-Failure { Assert-NodePublisher '' '' } 'publisher'
    Assert-Failure { Assert-NodePublisher "CN=bad`nSubject" '' } 'publisher'
    Assert-Failure { Assert-NodePublisher 'CN=Test Publisher' 'profile-name' } 'OID'
    Assert-NodePublisher 'CN=Test Publisher' '1.3.6.1.4.1.311.97.1.1.1234'

    $configuration = @{
        KPANEL_AZURE_TENANT_ID = 'test-tenant'; KPANEL_AZURE_CLIENT_ID = 'test-client'
        KPANEL_AZURE_CLIENT_SECRET = 'test-only-not-a-credential'; KPANEL_SIGNING_ACCOUNT = 'test-account'
        KPANEL_SIGNING_PROFILE = 'test-profile'; KPANEL_SIGNING_ENDPOINT = 'https://eus.codesigning.azure.net/'
    }
    $saved = @{}
    try {
        foreach ($name in $configuration.Keys) {
            $saved[$name] = [Environment]::GetEnvironmentVariable($name)
            [Environment]::SetEnvironmentVariable($name, $configuration[$name])
        }
        Assert-NodeSigningConfiguration
        foreach ($name in $configuration.Keys | Where-Object { $_ -ne 'KPANEL_SIGNING_ENDPOINT' }) {
            [Environment]::SetEnvironmentVariable($name, '')
            Assert-Failure { Assert-NodeSigningConfiguration } 'Missing signing configuration'
            [Environment]::SetEnvironmentVariable($name, $configuration[$name])
        }
        foreach ($endpoint in @('http://eus.codesigning.azure.net', 'https://evil.example', 'https://eus.codesigning.azure.net.evil.example')) {
            $env:KPANEL_SIGNING_ENDPOINT = $endpoint
            Assert-Failure { Assert-NodeSigningConfiguration } 'regional HTTPS'
        }
        $env:KPANEL_SIGNING_ENDPOINT = 'https://wus.artifactsigning.azure.net'
        Assert-NodeSigningConfiguration
    } finally {
        foreach ($name in $saved.Keys) { [Environment]::SetEnvironmentVariable($name, $saved[$name]) }
    }

    $unsigned = Join-Path $testDirectory 'unsigned.ps1'
    [IO.File]::WriteAllText($unsigned, '# not signed')
    Assert-Failure { Assert-NodeSignature $unsigned 'CN=Test Publisher' '' } 'verification failed'
    Remove-Item -LiteralPath $unsigned

    # Mock only the OS signature result, preserving the production validator and
    # manifest path. There is no unsigned/test switch in the production script.
    $script:signature = [pscustomobject]@{
        Status = 'Valid'
        SignerCertificate = [pscustomobject]@{
            Subject = 'CN=Test Publisher'
            Thumbprint = 'short-lived-leaf-one'
            EnhancedKeyUsageList = @([pscustomobject]@{ObjectId = [pscustomobject]@{Value = '1.3.6.1.5.5.7.3.3'}}, [pscustomobject]@{ObjectId = [pscustomobject]@{Value = '1.2.3.4.5'}})
        }
        TimeStamperCertificate = [pscustomobject]@{Subject = 'CN=Timestamp'}
    }
    function Get-AuthenticodeSignature { param([string]$LiteralPath) return $script:signature }
    foreach ($name in $script:WindowsNodeAssets) { [IO.File]::WriteAllText((Join-Path $testDirectory $name), "signed fixture $name") }
    Verify-NodeRelease $testDirectory 'CN=Test Publisher' '1.2.3.4.5'
    $manifest = Join-Path $testDirectory 'SHA256SUMS.windows'
    $lines = @(Get-Content -LiteralPath $manifest)
    if ($lines.Count -ne 3) { throw 'Expected all three assets in manifest.' }
    foreach ($name in $script:WindowsNodeAssets) {
        $hash = (Get-FileHash -LiteralPath (Join-Path $testDirectory $name) -Algorithm SHA256).Hash.ToLowerInvariant()
        if ($lines -cnotcontains "$hash  $name") { throw 'Manifest did not hash final signed bytes.' }
    }
    $script:signature.SignerCertificate.Thumbprint = 'rotated-leaf-two'
    Verify-NodeRelease $testDirectory 'CN=Test Publisher' '1.2.3.4.5'
    Assert-Failure { Verify-NodeRelease $testDirectory 'CN=Other Publisher' '1.2.3.4.5' } 'publisher verification'
    if (Test-Path -LiteralPath $manifest) { throw 'Failed verification retained a stale release manifest.' }
    Assert-Failure { Verify-NodeRelease $testDirectory 'CN=Test Publisher' '1.2.3.4.6' } 'EKU mismatch'
    $script:signature.TimeStamperCertificate = $null
    Assert-Failure { Verify-NodeRelease $testDirectory 'CN=Test Publisher' '' } 'timestamp'
    $script:signature.TimeStamperCertificate = [pscustomobject]@{Subject = 'CN=Timestamp'}
    $script:signature.Status = 'HashMismatch'
    Assert-Failure { Verify-NodeRelease $testDirectory 'CN=Test Publisher' '' } 'verification failed'
    $script:signature.Status = 'Valid'
    [IO.File]::WriteAllText((Join-Path $testDirectory 'unexpected.exe'), 'extra')
    Assert-Failure { Verify-NodeRelease $testDirectory 'CN=Test Publisher' '' } 'exactly'
    Assert-Failure { Build-NodeRelease $testDirectory $version } 'must be empty'
    Write-Host 'Windows release verification boundary tests passed.'
} finally {
    # Only the freshly created, fixed-prefix temp directory is eligible for cleanup.
    $resolved = [IO.Path]::GetFullPath($testDirectory)
    $expectedParent = [IO.Path]::GetFullPath([IO.Path]::GetTempPath()).TrimEnd('\')
    if ([IO.Path]::GetDirectoryName($resolved) -eq $expectedParent -and [IO.Path]::GetFileName($resolved) -match '^KPanelReleaseTests-[0-9a-f]{32}$') {
        Remove-Item -LiteralPath $resolved -Recurse -Force
    }
}
