$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
. (Join-Path $PSScriptRoot '../windows-node-release.ps1')
function Assert-Failure([scriptblock]$Action,[string]$Pattern){
 $caught=$false
 try{& $Action}catch{if($_.Exception.Message -notmatch $Pattern){throw};$caught=$true}
 if(-not $caught){throw "Expected failure: $Pattern"}
}
$root=Join-Path ([IO.Path]::GetTempPath()) ('KPanelReleaseTests-'+[Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $root | Out-Null
try{
 $version='v'+(Get-Content (Join-Path $PSScriptRoot '../../VERSION') -Raw).Trim()
 Assert-NodeReleaseVersion $version
 foreach($bad in @('v01.2.3','v1.2.3-rc.0','v1.2.3-dev','v1000000.2.3','v1.2.3;whoami')){Assert-Failure {Assert-NodeReleaseVersion $bad} 'canonical'}
 Assert-Failure {Assert-NodeReleaseVersion 'v999999.999999.999999'} 'does not match'
 foreach($name in $script:WindowsNodeAssets){[IO.File]::WriteAllText((Join-Path $root $name),"unsigned fixture $name")}
 Verify-NodeRelease $root
 $manifest=Join-Path $root 'SHA256SUMS.windows'
 $lines=@(Get-Content -LiteralPath $manifest)
 if($lines.Count -ne 3){throw 'Expected all three assets'}
 foreach($name in $script:WindowsNodeAssets){
  $hash=(Get-FileHash -LiteralPath (Join-Path $root $name) -Algorithm SHA256).Hash.ToLowerInvariant()
  if($lines -cnotcontains "$hash  $name"){throw 'Manifest must hash final bytes'}
 }
 $installer=Join-Path $root 'install-windows.ps1'
 [IO.File]::WriteAllText($installer,('x'*1048577))
 Assert-Failure {Verify-NodeRelease $root} 'Invalid release asset'
 if(Test-Path -LiteralPath $manifest){throw 'Failed verification retained stale manifest'}
 [IO.File]::WriteAllText($installer,'restored fixture')
 Verify-NodeRelease $root
 [IO.File]::WriteAllText((Join-Path $root 'unexpected.exe'),'extra')
 Assert-Failure {Verify-NodeRelease $root} 'exactly'
 Assert-Failure {Build-NodeRelease $root $version} 'must be empty'
 Assert-Failure {Get-NodeReleaseDirectory 'relative-path'} 'absolute'
 Write-Output 'Windows release boundary tests passed: three unsigned assets and final-byte checksums.'
}finally{
 $resolved=[IO.Path]::GetFullPath($root)
 $parent=[IO.Path]::GetFullPath([IO.Path]::GetTempPath()).TrimEnd('\')
 if([IO.Path]::GetDirectoryName($resolved) -eq $parent -and [IO.Path]::GetFileName($resolved) -match '^KPanelReleaseTests-[0-9a-f]{32}$'){Remove-Item -LiteralPath $resolved -Recurse -Force}
}
