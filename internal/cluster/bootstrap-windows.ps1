[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][ValidatePattern('^v(0|[1-9][0-9]{0,5})\.(0|[1-9][0-9]{0,5})\.(0|[1-9][0-9]{0,5})(-rc\.[1-9][0-9]{0,5})?$')][string]$Version,
    [string]$Name = '',
    [switch]$EnableDesktop
)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
if ($ExecutionContext.SessionState.LanguageMode -ne 'FullLanguage') { throw 'Constrained PowerShell is unsupported; use an offline package approved by your WDAC/AppLocker policy.' }
if (-not ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) { throw 'Run PowerShell as Administrator.' }
$token = $env:KPANEL_NODE_TOKEN
if ([String]::IsNullOrEmpty($token) -or $token -notmatch '^kp[lb]1\.[A-Za-z0-9_-]+$' -or $token.Length -gt 2048) { throw 'A valid enrollment token is required.' }
if ($Name.Length -gt 80 -or $Name -match '[\x00-\x1f\x7f]') { throw 'The node name is invalid.' }

function Assert-BootstrapDirectory([string]$Path, [bool]$Ancestor) {
    $item = Get-Item -LiteralPath $Path -Force
    if (-not $item.PSIsContainer -or ($item.Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw 'Unsafe bootstrap directory or reparse point.' }
    $acl = Get-Acl -LiteralPath $Path
    $trusted = @('S-1-5-18','S-1-5-32-544')
    if ($Ancestor) { $trusted += 'S-1-5-80-956008885-3418522649-1831038044-1853292631-2271478464' }
    if ($trusted -notcontains $acl.GetOwner([Security.Principal.SecurityIdentifier]).Value) { throw 'Unsafe bootstrap directory owner.' }
    $writeMask = 0x500d0150
    if (-not $Ancestor) { $writeMask = $writeMask -bor 0x6 }
    foreach ($rule in $acl.GetAccessRules($true,$true,[Security.Principal.SecurityIdentifier])) {
        if ($rule.PropagationFlags -band [Security.AccessControl.PropagationFlags]::InheritOnly) { continue }
        if ($rule.AccessControlType -ne [Security.AccessControl.AccessControlType]::Allow) { continue }
        if ($trusted -contains $rule.IdentityReference.Value) { continue }
        if (([int]$rule.FileSystemRights -band $writeMask) -ne 0) { throw 'Directory allows untrusted writes.' }
    }
}

function Get-BootstrapFile([string]$Url, [string]$Destination, [long]$Limit) {
    for ($hop = 0; $hop -le 5; $hop++) {
        $uri = [Uri]$Url
        if ($uri.Scheme -ne 'https' -or $uri.UserInfo -or (-not $uri.IsDefaultPort -and $uri.Port -ne 443) -or @('github.com','release-assets.githubusercontent.com','objects.githubusercontent.com') -notcontains $uri.DnsSafeHost) { throw 'Release download left the approved HTTPS hosts.' }
        $request = [Net.HttpWebRequest]::Create($uri)
        $request.AllowAutoRedirect = $false
        $request.Timeout = 120000
        $request.ReadWriteTimeout = 120000
        $response = $request.GetResponse()
        try {
            if ([int]$response.StatusCode -ge 300 -and [int]$response.StatusCode -lt 400) {
                $Url = [Uri]::new($uri, $response.Headers['Location']).AbsoluteUri
                continue
            }
            if ([int]$response.StatusCode -ne 200 -or $response.ContentLength -gt $Limit) { throw 'Invalid release response.' }
            $inputStream = $response.GetResponseStream()
            $outputStream = [IO.File]::Open($Destination, [IO.FileMode]::CreateNew, [IO.FileAccess]::Write, [IO.FileShare]::None)
            try {
                $buffer = [byte[]]::new(65536)
                [long]$total = 0
                while (($count = $inputStream.Read($buffer, 0, $buffer.Length)) -gt 0) {
                    $total += $count
                    if ($total -gt $Limit) { throw 'Release file too large.' }
                    $outputStream.Write($buffer, 0, $count)
                }
                $outputStream.Flush($true)
            } finally { $outputStream.Dispose(); $inputStream.Dispose() }
            return
        } finally { $response.Dispose() }
    }
    throw 'Too many release redirects.'
}

function Assert-ReleaseChecksum([string]$Manifest, [string]$Asset, [string]$Path, [long]$Limit) {
    $manifestItem = Get-Item -LiteralPath $Manifest -Force
    if ($manifestItem.PSIsContainer -or ($manifestItem.Attributes -band [IO.FileAttributes]::ReparsePoint) -or $manifestItem.Length -le 0 -or $manifestItem.Length -gt 65536) { throw 'Invalid release checksum manifest.' }
    $item = Get-Item -LiteralPath $Path -Force
    if ($item.PSIsContainer -or ($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -or $item.Length -le 0 -or $item.Length -gt $Limit) { throw 'Invalid release asset file.' }
    $lines = @([IO.File]::ReadAllLines($Manifest) | Where-Object { $_ -match ('^[a-f0-9]{64}\s+\*?' + [regex]::Escape($Asset) + '$') })
    if ($lines.Count -ne 1) { throw 'Release checksum missing or ambiguous.' }
    $expected = ($lines[0] -split '\s+')[0]
    if ((Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash.ToLowerInvariant() -cne $expected) { throw 'Release checksum mismatch.' }
    return $expected
}

$root = [Environment]::GetFolderPath('ProgramFiles')
$parent = [IO.DirectoryInfo]::new($root)
while ($null -ne $parent) { Assert-BootstrapDirectory $parent.FullName $true; $parent = $parent.Parent }
Assert-BootstrapDirectory ([IO.Path]::GetDirectoryName($PSCommandPath)) $false
$stage = Join-Path $root ('KPanelInstall-' + [Guid]::NewGuid().ToString('N'))
$security = [Security.AccessControl.DirectorySecurity]::new()
$security.SetSecurityDescriptorSddlForm('O:BAG:BAD:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)')
if ($PSVersionTable.PSEdition -eq 'Core') {
    $null = [IO.FileSystemAclExtensions]::Create([IO.DirectoryInfo]::new($stage), $security)
} else {
    $null = [IO.Directory]::CreateDirectory($stage, $security)
}
Assert-BootstrapDirectory $stage $false
$script = Join-Path $stage 'install-windows.ps1'
$manifest = Join-Path $stage 'SHA256SUMS'
try {
    [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
    $base = "https://github.com/kejilion/KPanel/releases/download/$Version/"
    Get-BootstrapFile ($base + 'SHA256SUMS') $manifest 65536
    Get-BootstrapFile ($base + 'install-windows.ps1') $script 1048576
    $null = Assert-ReleaseChecksum $manifest 'install-windows.ps1' $script 1048576
    Write-Warning 'Windows node is unsigned; official HTTPS and SHA-256 verify integrity, not a certificate publisher.'
    & $script -Version $Version -Name $Name -Token $token -EnableDesktop:$EnableDesktop
} finally {
    $token = $null
    Remove-Item Env:KPANEL_NODE_TOKEN -ErrorAction SilentlyContinue
    $resolved = [IO.Path]::GetFullPath($stage)
    if ($resolved.StartsWith($root + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) { Remove-Item -LiteralPath $resolved -Recurse -Force }
}
