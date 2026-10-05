package cluster

import (
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"regexp"
	"strings"
)

var windowsReleaseVersion = regexp.MustCompile(`^v?(0|[1-9][0-9]{0,5})\.(0|[1-9][0-9]{0,5})\.(0|[1-9][0-9]{0,5})(?:-rc\.[1-9][0-9]{0,5})?$`)

// The release asset is pinned into the panel binary so the short launcher can
// verify it before executing any downloaded PowerShell source.
//go:embed bootstrap-windows.ps1
var windowsBootstrapScript []byte

const windowsBootstrapLaunchGuard = `function Assert-BootstrapDirectory([string]$path,[bool]$ancestor) {
	$item=Get-Item -LiteralPath $path -Force; if (-not $item.PSIsContainer -or ($item.Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw 'Unsafe staging directory' };
	$acl=Get-Acl -LiteralPath $path; $trusted=@('S-1-5-18','S-1-5-32-544'); if ($ancestor) { $trusted+='S-1-5-80-956008885-3418522649-1831038044-1853292631-2271478464' };
	if ($trusted -notcontains $acl.GetOwner([Security.Principal.SecurityIdentifier]).Value) { throw 'Unsafe staging owner' }; $mask=0x500d0150; if (-not $ancestor) { $mask=$mask -bor 0x6 };
	foreach ($rule in $acl.GetAccessRules($true,$true,[Security.Principal.SecurityIdentifier])) { if (($rule.PropagationFlags -band [Security.AccessControl.PropagationFlags]::InheritOnly) -or $rule.AccessControlType -ne [Security.AccessControl.AccessControlType]::Allow -or $trusted -contains $rule.IdentityReference.Value) { continue }; if (([int]$rule.FileSystemRights -band $mask) -ne 0) { throw 'Unsafe staging write permissions' } }
}; function Get-BootstrapAsset([string]$url,[string]$path,[long]$limit) {
	for ($hop=0;$hop -le 5;$hop++) { $uri=[Uri]$url; if ($uri.Scheme -ne 'https' -or $uri.UserInfo -or (-not $uri.IsDefaultPort -and $uri.Port -ne 443) -or @('github.com','release-assets.githubusercontent.com','objects.githubusercontent.com') -notcontains $uri.DnsSafeHost) { throw 'Untrusted release URL' };
	$request=[Net.HttpWebRequest]::Create($uri); $request.AllowAutoRedirect=$false; $request.Timeout=120000; $request.ReadWriteTimeout=120000; $response=$request.GetResponse();
	try { if ([int]$response.StatusCode -ge 300 -and [int]$response.StatusCode -lt 400) { $url=[Uri]::new($uri,$response.Headers['Location']).AbsoluteUri; continue }; if ([int]$response.StatusCode -ne 200 -or $response.ContentLength -gt $limit) { throw 'Invalid bootstrap response' };
	$inputStream=$response.GetResponseStream(); $outputStream=[IO.File]::Open($path,[IO.FileMode]::CreateNew,[IO.FileAccess]::Write,[IO.FileShare]::None); try { $buffer=[byte[]]::new(65536); [long]$total=0; while (($count=$inputStream.Read($buffer,0,$buffer.Length)) -gt 0) { $total+=$count; if ($total -gt $limit) { throw 'Bootstrap asset too large' }; $outputStream.Write($buffer,0,$count) }; $outputStream.Flush($true) } finally { $outputStream.Dispose(); $inputStream.Dispose() }; return
	} finally { $response.Dispose() } }; throw 'Too many release redirects'
}`

func powershellSingleQuote(value string) string {
	// PowerShell also treats Unicode curly quotes as string delimiters. Keep
	// user/configuration text entirely outside its lexer, including host names.
	return "([Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('" + base64.StdEncoding.EncodeToString([]byte(value)) + "')))"
}

func windowsBootstrapScriptSHA256() string {
	sum := sha256.Sum256(windowsBootstrapScript)
	return hex.EncodeToString(sum[:])
}

func (s *Service) lightEnrollmentCommand(platform, token, name string) (string, error) {
	return s.lightEnrollmentCommandWithDesktop(platform, token, name, false)
}

func (s *Service) lightEnrollmentCommandWithDesktop(platform, token, name string, desktop bool) (string, error) {
	if desktop && platform != "windows" {
		return "", ErrProtocolMismatch
	}
	if platform == "" || platform == "linux" {
		command := "bash <(curl -fsSL https://kejilion.sh) kpanel node join '" + token + "'"
		if name != "" {
			command += " --name " + shellSingleQuote(name)
		}
		return command, nil
	}
	if platform != "windows" {
		return "", ErrProtocolMismatch
	}
	if !windowsReleaseVersion.MatchString(s.panelVersion) {
		return "", ErrWindowsInstallerUnavailable
	}
	base := "https://github.com/kejilion/KPanel/releases/download/v" + strings.TrimPrefix(s.panelVersion, "v") + "/"
	parts := []string{
		"& { $ErrorActionPreference='Stop'",
		"if (-not ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) { throw 'Run in administrator PowerShell' }",
		strings.ReplaceAll(windowsBootstrapLaunchGuard, "\n", " "),
		"$root=[Environment]::GetFolderPath('ProgramFiles')",
		"$parent=[IO.DirectoryInfo]::new($root); while ($null -ne $parent) { Assert-BootstrapDirectory $parent.FullName $true; $parent=$parent.Parent }",
		"$stage=Join-Path $root ('KPanelBootstrap-'+[guid]::NewGuid().ToString('N'))",
		"$security=[Security.AccessControl.DirectorySecurity]::new(); $security.SetSecurityDescriptorSddlForm('O:BAG:BAD:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)')",
		"if ($PSVersionTable.PSEdition -eq 'Core') { $null=[IO.FileSystemAclExtensions]::Create([IO.DirectoryInfo]::new($stage),$security) } else { $null=[IO.Directory]::CreateDirectory($stage,$security) }",
		"Assert-BootstrapDirectory $stage $false",
		"$bootstrap=Join-Path $stage 'bootstrap-windows.ps1'",
		"try { [Net.ServicePointManager]::SecurityProtocol=[Net.SecurityProtocolType]::Tls12",
		"Get-BootstrapAsset " + powershellSingleQuote(base+"bootstrap-windows.ps1") + " $bootstrap 262144",
		"$expected='" + windowsBootstrapScriptSHA256() + "'; if ((Get-FileHash -LiteralPath $bootstrap -Algorithm SHA256).Hash.ToLowerInvariant() -cne $expected) { throw 'Bootstrap checksum mismatch' }",
		"Write-Warning 'Windows node is unsigned; the pinned SHA-256 verifies the bootstrap, not a certificate publisher'",
	}
	command := "$previousToken=$env:KPANEL_NODE_TOKEN; try { $env:KPANEL_NODE_TOKEN=" + powershellSingleQuote(token) + "; & $bootstrap -Version " + powershellSingleQuote("v"+strings.TrimPrefix(s.panelVersion, "v"))
	if name != "" {
		command += " -Name " + powershellSingleQuote(name)
	}
	if desktop {
		command += " -EnableDesktop"
	}
	command += " } finally { if ($null -eq $previousToken) { Remove-Item Env:KPANEL_NODE_TOKEN -ErrorAction SilentlyContinue } else { $env:KPANEL_NODE_TOKEN=$previousToken }; $previousToken=$null }"
	parts = append(parts, command, "} finally { foreach ($file in @($bootstrap)) { if (Test-Path -LiteralPath $file) { Remove-Item -LiteralPath $file -Force } }; Remove-Item -LiteralPath $stage -Force }")
	return strings.Join(parts, "; ") + " }", nil
}
