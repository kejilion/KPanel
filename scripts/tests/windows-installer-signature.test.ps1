$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
# Extract only the signature helper: the installer itself must never execute in tests.
$tokens=$null; $parseErrors=$null
$ast=[Management.Automation.Language.Parser]::ParseFile((Join-Path $PSScriptRoot '../../deploy/windows/install.ps1'),[ref]$tokens,[ref]$parseErrors)
if($parseErrors.Count){throw 'Installer syntax errors'}
$definition=$ast.Find({param($node) $node -is [Management.Automation.Language.FunctionDefinitionAst] -and $node.Name -eq 'Assert-Signature'},$true)
. ([scriptblock]::Create($definition.Extent.Text))
$candidate=Join-Path $PSHOME 'pwsh.exe'
if(-not (Test-Path -LiteralPath $candidate)){$candidate=Join-Path $PSHOME 'powershell.exe'}
$signed=Get-AuthenticodeSignature -LiteralPath $candidate
if($signed.Status -ne 'Valid'){throw 'A trusted PowerShell executable is required for this regression'}
$Publisher=$signed.SignerCertificate.Subject
$ProfileOID='1.3.6.1.5.5.7.3.3'
Assert-Signature $candidate
$ProfileOID='1.3.6.1.4.1.99999.12345'
$rejected=$false
try{Assert-Signature $candidate}catch{if($_.Exception.Message -notmatch 'profile does not match'){throw};$rejected=$true}
if(-not $rejected){throw 'Wrong profile accepted'}
Write-Output 'Windows installer signature/profile checks passed without running installer.'
