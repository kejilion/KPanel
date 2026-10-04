$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
# Parse the installer and extract only its checksum helper. Never run installation.
$tokens=$null; $parseErrors=$null
$ast=[Management.Automation.Language.Parser]::ParseFile((Join-Path $PSScriptRoot '../../deploy/windows/install.ps1'),[ref]$tokens,[ref]$parseErrors)
if($parseErrors.Count){throw ($parseErrors.Message -join '; ')}
$definition=$ast.Find({param($node) $node -is [Management.Automation.Language.FunctionDefinitionAst] -and $node.Name -eq 'Assert-ReleaseChecksum'},$true)
if(-not $definition){throw 'Checksum helper missing'}
. ([scriptblock]::Create($definition.Extent.Text))
function Assert-Failure([scriptblock]$Action,[string]$Pattern){
 $caught=$false
 try{& $Action}catch{if($_.Exception.Message -notmatch $Pattern){throw};$caught=$true}
 if(-not $caught){throw "Expected failure: $Pattern"}
}
$root=Join-Path ([IO.Path]::GetTempPath()) ('KPanelIntegrityTests-'+[Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $root | Out-Null
try{
 $asset=Join-Path $root 'installer.ps1';$manifest=Join-Path $root 'SHA256SUMS'
 [IO.File]::WriteAllText($asset,'test fixture only')
 $hash=(Get-FileHash -LiteralPath $asset -Algorithm SHA256).Hash.ToLowerInvariant()
 $line="$hash  install-windows.ps1"
 [IO.File]::WriteAllText($manifest,$line)
 if((Assert-ReleaseChecksum $manifest 'install-windows.ps1' $asset 1048576) -cne $hash){throw 'Wrong hash'}
 [IO.File]::WriteAllText($manifest,($line+[Environment]::NewLine+$line))
 Assert-Failure {Assert-ReleaseChecksum $manifest 'install-windows.ps1' $asset 1048576} 'ambiguous'
 [IO.File]::WriteAllText($manifest,"$hash  other.exe")
 Assert-Failure {Assert-ReleaseChecksum $manifest 'install-windows.ps1' $asset 1048576} 'missing'
 [IO.File]::WriteAllText($manifest,$line)
 [IO.File]::AppendAllText($asset,'tampered')
 Assert-Failure {Assert-ReleaseChecksum $manifest 'install-windows.ps1' $asset 1048576} 'mismatch'
 Assert-Failure {Assert-ReleaseChecksum $manifest 'install-windows.ps1' $asset 1} 'asset file'
 Assert-Failure {Assert-ReleaseChecksum $manifest 'install-windows.ps1' $root 1048576} 'asset file'
 [IO.File]::WriteAllText($manifest,('x'*65537))
 Assert-Failure {Assert-ReleaseChecksum $manifest 'install-windows.ps1' $asset 1048576} 'manifest'
 Write-Output 'Installer integrity tests passed without executing the installer.'
}finally{
 $resolved=[IO.Path]::GetFullPath($root)
 $parent=[IO.Path]::GetFullPath([IO.Path]::GetTempPath()).TrimEnd('\')
 if([IO.Path]::GetDirectoryName($resolved) -eq $parent -and [IO.Path]::GetFileName($resolved) -match '^KPanelIntegrityTests-[0-9a-f]{32}$'){Remove-Item -LiteralPath $resolved -Recurse -Force}
}
