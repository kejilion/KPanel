# KPanel Windows node installer. Callers must verify this file against SHA256SUMS
# from the same fixed official HTTPS release before executing it.
[CmdletBinding()]
param(
    [string]$Token = $env:KPANEL_NODE_TOKEN,
    [string]$Name = '',
    [string]$Capabilities = '',
    [switch]$EnableDesktop,
    [Parameter(Mandatory = $true)][ValidatePattern('^v(0|[1-9][0-9]{0,5})\.(0|[1-9][0-9]{0,5})\.(0|[1-9][0-9]{0,5})(-rc\.[1-9][0-9]{0,5})?$')][string]$Version
)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
if ($ExecutionContext.SessionState.LanguageMode -ne 'FullLanguage') { throw 'Constrained PowerShell is unsupported; use an offline package approved by your WDAC/AppLocker policy.' }
$identity = [Security.Principal.WindowsIdentity]::GetCurrent()
$principal = [Security.Principal.WindowsPrincipal]::new($identity)
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) { throw 'Run PowerShell as Administrator.' }
if (-not [Environment]::Is64BitProcess) { throw 'Use 64-bit PowerShell to install a native Windows node.' }
if ([Environment]::OSVersion.Version.Major -lt 10 -or -not [Environment]::Is64BitOperatingSystem) { throw 'Windows 10 / Server 2016 or later, 64-bit, is required.' }
if ($Token -notmatch '^kp[lb]1\.[A-Za-z0-9_-]+$' -or $Token.Length -gt 2048) { throw 'A valid enrollment token is required.' }
if ($Name.Length -gt 80 -or $Name -match '[\x00-\x1f\x7f]') { throw 'The node name is invalid.' }
if ($EnableDesktop) {
    Write-Host 'Administrator desktop selected: supported non-domain Windows will enable RDP/NLA and create a managed local administrator. UAC remains enabled. Existing firewall rules may also allow other authorized accounts to connect over the network. Closing the managed desktop logs it off; save your work first.'
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
function Assert-Directory([string]$Path, [bool]$Ancestor) {
    $item = Get-Item -LiteralPath $Path -Force
    if (-not $item.PSIsContainer -or ($item.Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw "Untrusted directory or reparse point: $Path" }
    $acl = Get-Acl -LiteralPath $Path
    $owner = $acl.GetOwner([Security.Principal.SecurityIdentifier]).Value
    $trusted = @('S-1-5-18','S-1-5-32-544')
    if ($Ancestor) { $trusted += 'S-1-5-80-956008885-3418522649-1831038044-1853292631-2271478464' }
    if ($trusted -notcontains $owner) { throw "Untrusted directory owner: $Path" }
    $writeMask = 0x500d0150 # EA/attributes, delete-child, DELETE, WRITE_DAC/OWNER, generic write/all
    if (-not $Ancestor) { $writeMask = $writeMask -bor 0x6 }
    foreach ($rule in $acl.GetAccessRules($true,$true,[Security.Principal.SecurityIdentifier])) {
        if ($rule.PropagationFlags -band [Security.AccessControl.PropagationFlags]::InheritOnly) { continue }
        if ($rule.AccessControlType -ne [Security.AccessControl.AccessControlType]::Allow) { continue }
        if ($trusted -contains $rule.IdentityReference.Value) { continue }
        $rights = [int]$rule.FileSystemRights
        $commonDataRoot = [IO.Path]::GetFullPath([Environment]::GetFolderPath('CommonApplicationData')).TrimEnd('\')
        $isStandardUsersMetadataAce = $Ancestor -and
            [String]::Equals([IO.Path]::GetFullPath($Path).TrimEnd('\'),$commonDataRoot,[StringComparison]::OrdinalIgnoreCase) -and
            $rule.IdentityReference.Value -eq 'S-1-5-32-545'
        if ($isStandardUsersMetadataAce) { $rights = $rights -band (-bnot 0x110) } # ProgramData grants Users WRITE_EA/WRITE_ATTRIBUTES by default.
        if (($rights -band $writeMask) -ne 0) { throw "Directory allows untrusted writes: $Path" }
    }
}
function New-ProtectedDirectory([string]$Path, [bool]$PublicRead) {
    $parent = [IO.Directory]::GetParent($Path)
    $parents = [Collections.Generic.List[string]]::new()
    while ($null -ne $parent) { $parents.Add($parent.FullName); $parent = $parent.Parent }
    for ($i = $parents.Count - 1; $i -ge 0; $i--) { Assert-Directory $parents[$i] $true }
    if (Test-Path -LiteralPath $Path) { Assert-Directory $Path $false; return }
    $sddl = 'O:BAG:BAD:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)'
    if ($PublicRead) { $sddl += '(A;OICI;FRFX;;;BU)' }
    $security = [Security.AccessControl.DirectorySecurity]::new()
    $security.SetSecurityDescriptorSddlForm($sddl)
    if ($PSVersionTable.PSEdition -eq 'Core') {
        [IO.FileSystemAclExtensions]::Create([IO.DirectoryInfo]::new($Path),$security)
    } else {
        [IO.Directory]::CreateDirectory($Path,$security) | Out-Null
    }
    Assert-Directory $Path $false
}
function Get-ReleaseFile([string]$Url, [string]$Destination, [long]$Limit) {
    for ($hop=0; $hop -le 5; $hop++) {
        $uri = [Uri]$Url
        if ($uri.Scheme -ne 'https' -or $uri.UserInfo -or (-not $uri.IsDefaultPort -and $uri.Port -ne 443) -or @('github.com','release-assets.githubusercontent.com','objects.githubusercontent.com') -notcontains $uri.DnsSafeHost) { throw 'Release download left the approved HTTPS hosts.' }
        $request = [Net.HttpWebRequest]::Create($uri)
        $request.AllowAutoRedirect = $false
        $request.Timeout = 120000
        $request.ReadWriteTimeout = 120000
        $response = $request.GetResponse()
        try {
            if ([int]$response.StatusCode -ge 300 -and [int]$response.StatusCode -lt 400) {
                $Url = [Uri]::new($uri,$response.Headers['Location']).AbsoluteUri
                continue
            }
            if ([int]$response.StatusCode -ne 200 -or $response.ContentLength -gt $Limit) { throw 'Invalid release response.' }
            $inputStream = $response.GetResponseStream()
            $outputStream = [IO.File]::Open($Destination,[IO.FileMode]::CreateNew,[IO.FileAccess]::Write,[IO.FileShare]::None)
            try {
                $buffer = [byte[]]::new(65536)
                [long]$total = 0
                while (($count = $inputStream.Read($buffer,0,$buffer.Length)) -gt 0) {
                    $total += $count
                    if ($total -gt $Limit) { throw 'Release asset exceeds its size limit.' }
                    $outputStream.Write($buffer,0,$count)
                }
                $outputStream.Flush($true)
            } finally { $outputStream.Dispose(); $inputStream.Dispose() }
            return
        } finally { $response.Dispose() }
    }
    throw 'Too many release redirects.'
}

$installRoot = Join-Path ([Environment]::GetFolderPath('ProgramFiles')) 'KejilionNode'
$dataRoot = Join-Path ([Environment]::GetFolderPath('CommonApplicationData')) 'KejilionNode'
New-ProtectedDirectory $installRoot $true
New-ProtectedDirectory $dataRoot $false
$staging = Join-Path $installRoot ('install-' + [Guid]::NewGuid().ToString('N'))
New-ProtectedDirectory $staging $false
$binary = Join-Path $installRoot 'kejilion-node.exe'
$bootstrap = Join-Path $installRoot 'kejilion-node-bootstrap.exe'
try {
    if (Get-Service -Name 'KejilionNode' -ErrorAction SilentlyContinue) { throw 'A node service is already installed. Use status/update, or uninstall before changing enrollment.' }
    # PROCESSOR_ARCHITEW6432 reports native architecture for a 32-bit process.
    $nativeArch = $env:PROCESSOR_ARCHITEW6432
    if (-not $nativeArch) { $nativeArch = $env:PROCESSOR_ARCHITECTURE }
    $arch = switch ($nativeArch.ToUpperInvariant()) { 'AMD64' {'amd64'}; 'ARM64' {'arm64'}; default {throw 'Unsupported native Windows architecture.'} }
    $asset = "kejilion-node-windows-$arch.exe"
    $base = "https://github.com/kejilion/KPanel/releases/download/$Version/"
    $manifest = Join-Path $staging 'SHA256SUMS'
    $download = Join-Path $staging $asset
    Get-ReleaseFile ($base + 'SHA256SUMS') $manifest 65536
    Assert-Directory ([IO.Path]::GetDirectoryName($PSCommandPath)) $false
    $null = Assert-ReleaseChecksum $manifest 'install-windows.ps1' $PSCommandPath 1048576
    Get-ReleaseFile ($base + $asset) $download 67108864
    $expected = Assert-ReleaseChecksum $manifest $asset $download 67108864
    foreach ($destination in @($binary,$bootstrap)) {
        if (Test-Path -LiteralPath $destination) {
            $null = Assert-ReleaseChecksum $manifest $asset $destination 67108864
        } else {
            [IO.File]::Copy($download,$destination,$false)
        }
    }
    $fileSecurity = [Security.AccessControl.FileSecurity]::new()
    $fileSecurity.SetSecurityDescriptorSddlForm('O:BAG:BAD:P(A;;FA;;;SY)(A;;FA;;;BA)(A;;FRFX;;;BU)')
    Set-Acl -LiteralPath $binary -AclObject $fileSecurity
    Set-Acl -LiteralPath $bootstrap -AclObject $fileSecurity
    $request = @{token=$Token;name=$Name;capabilities=$Capabilities;enableDesktop=[bool]$EnableDesktop;sha256=$expected} | ConvertTo-Json -Compress
    $startInfo = [Diagnostics.ProcessStartInfo]::new($binary,'install --stdin')
    $startInfo.UseShellExecute = $false
    $startInfo.CreateNoWindow = $true
    $startInfo.RedirectStandardInput = $true
    $process = [Diagnostics.Process]::Start($startInfo)
    try {
        $requestBytes = [Text.UTF8Encoding]::new($false).GetBytes($request)
        $process.StandardInput.BaseStream.Write($requestBytes,0,$requestBytes.Length)
        $process.StandardInput.BaseStream.Flush()
        [Array]::Clear($requestBytes,0,$requestBytes.Length)
        $process.StandardInput.Close()
        if (-not $process.WaitForExit(150000)) { throw 'Bootstrap is still running; check service status before retrying.' }
        $installExit = $process.ExitCode
    } finally { $process.Dispose() }
    if ($installExit -ne 0) { throw 'Node bootstrap failed. Preserve the protected batch attempt state before retrying enrollment.' }
    if ($EnableDesktop) {
        Write-Host 'RDP was opted in. On supported non-domain Windows, KPanel configures RDP/NLA and manages a dedicated local administrator. Existing firewall rules may also allow other authorized accounts to connect over the network; no firewall rule was added. Domain machines keep existing RDP/account settings.'
    } else {
        Write-Host 'Windows node installed. RDP and firewall settings were not changed.'
    }
    Write-Host 'Review PSReadLine history for enrollment tokens; revoke unused batch tokens in the center.'
} finally {
    $Token = $null; $request = $null
    Remove-Item Env:KPANEL_NODE_TOKEN -ErrorAction SilentlyContinue
    # Only this invocation owns this random, protected staging directory.
    $resolved = [IO.Path]::GetFullPath($staging)
    if ($resolved.StartsWith($installRoot + [IO.Path]::DirectorySeparatorChar,[StringComparison]::OrdinalIgnoreCase)) { Remove-Item -LiteralPath $resolved -Recurse -Force }
}
