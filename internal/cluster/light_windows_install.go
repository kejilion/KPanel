package cluster

import (
	"encoding/base64"
	"regexp"
	"strings"
)

var windowsReleaseVersion = regexp.MustCompile(`^v?(0|[1-9][0-9]{0,5})\.(0|[1-9][0-9]{0,5})\.(0|[1-9][0-9]{0,5})(?:-rc\.[1-9][0-9]{0,5})?$`)

// Before trusting downloaded code, protect its directory and fetch only bounded
// files through official HTTPS release hosts. This bootstrap executes no text.
const windowsBootstrapGuard = `function Assert-BootstrapDirectory([string]$path,[bool]$ancestor) {
 $item=Get-Item -LiteralPath $path -Force; if (-not $item.PSIsContainer -or ($item.Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw 'Unsafe staging directory' };
 $acl=Get-Acl -LiteralPath $path; $trusted=@('S-1-5-18','S-1-5-32-544'); if ($ancestor) { $trusted+='S-1-5-80-956008885-3418522649-1831038044-1853292631-2271478464' };
 if ($trusted -notcontains $acl.GetOwner([Security.Principal.SecurityIdentifier]).Value) { throw 'Unsafe staging owner' }; $mask=0x500d0150; if (-not $ancestor) { $mask=$mask -bor 0x6 };
 foreach ($rule in $acl.GetAccessRules($true,$true,[Security.Principal.SecurityIdentifier])) { if (($rule.PropagationFlags -band [Security.AccessControl.PropagationFlags]::InheritOnly) -or $rule.AccessControlType -ne [Security.AccessControl.AccessControlType]::Allow -or $trusted -contains $rule.IdentityReference.Value) { continue }; if (([int]$rule.FileSystemRights -band $mask) -ne 0) { throw 'Unsafe staging write permissions' } }
}; function Get-BootstrapFile([string]$url,[string]$path,[long]$limit) {
 for ($hop=0;$hop -le 5;$hop++) { $uri=[Uri]$url; if ($uri.Scheme -ne 'https' -or $uri.UserInfo -or (-not $uri.IsDefaultPort -and $uri.Port -ne 443) -or @('github.com','release-assets.githubusercontent.com','objects.githubusercontent.com') -notcontains $uri.DnsSafeHost) { throw 'Untrusted release URL' };
 $request=[Net.HttpWebRequest]::Create($uri); $request.AllowAutoRedirect=$false; $request.Timeout=120000; $request.ReadWriteTimeout=120000; $response=$request.GetResponse();
 try { if ([int]$response.StatusCode -ge 300 -and [int]$response.StatusCode -lt 400) { $url=[Uri]::new($uri,$response.Headers['Location']).AbsoluteUri; continue }; if ([int]$response.StatusCode -ne 200 -or $response.ContentLength -gt $limit) { throw 'Invalid release response' };
 $inputStream=$response.GetResponseStream(); $outputStream=[IO.File]::Open($path,[IO.FileMode]::CreateNew,[IO.FileAccess]::Write,[IO.FileShare]::None); try { $buffer=[byte[]]::new(65536); [long]$total=0; while (($count=$inputStream.Read($buffer,0,$buffer.Length)) -gt 0) { $total+=$count; if ($total -gt $limit) { throw 'Release file too large' }; $outputStream.Write($buffer,0,$count) }; $outputStream.Flush($true) } finally { $outputStream.Dispose(); $inputStream.Dispose() }; return
 } finally { $response.Dispose() } }; throw 'Too many release redirects'
}`

func powershellSingleQuote(value string) string {
	// PowerShell also treats Unicode curly quotes as string delimiters. Keep
	// user/configuration text entirely outside its lexer, including host names.
	return "([Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('" + base64.StdEncoding.EncodeToString([]byte(value)) + "')))"
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
		strings.ReplaceAll(windowsBootstrapGuard, "\n", " "),
		"$root=[Environment]::GetFolderPath('ProgramFiles')",
		"$parent=[IO.DirectoryInfo]::new($root); while ($null -ne $parent) { Assert-BootstrapDirectory $parent.FullName $true; $parent=$parent.Parent }",
		"$stage=Join-Path $root ('KPanelBootstrap-'+[guid]::NewGuid().ToString('N'))",
		"$security=[Security.AccessControl.DirectorySecurity]::new(); $security.SetSecurityDescriptorSddlForm('O:BAG:BAD:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)')",
		"if ($PSVersionTable.PSEdition -eq 'Core') { $null=[IO.FileSystemAclExtensions]::Create([IO.DirectoryInfo]::new($stage),$security) } else { $null=[IO.Directory]::CreateDirectory($stage,$security) }",
		"Assert-BootstrapDirectory $stage $false",
		"$script=Join-Path $stage 'install-windows.ps1'",
		"$manifest=Join-Path $stage 'SHA256SUMS'",
		"try { [Net.ServicePointManager]::SecurityProtocol=[Net.SecurityProtocolType]::Tls12",
		"Get-BootstrapFile " + powershellSingleQuote(base+"SHA256SUMS") + " $manifest 65536",
		"Get-BootstrapFile " + powershellSingleQuote(base+"install-windows.ps1") + " $script 1048576",
		"$lines=@([IO.File]::ReadAllLines($manifest) | Where-Object { $_ -match '^[a-f0-9]{64}\\s+\\*?install-windows\\.ps1$' })",
		"if ($lines.Count -ne 1) { throw 'Installer checksum missing or ambiguous' }",
		"$expected=($lines[0] -split '\\s+')[0]; if ((Get-FileHash -LiteralPath $script -Algorithm SHA256).Hash.ToLowerInvariant() -cne $expected) { throw 'Installer checksum mismatch' }",
		"Write-Warning 'Windows node is unsigned; official HTTPS and SHA-256 verify integrity, not a certificate publisher'",
	}
	command := "& $script -Token " + powershellSingleQuote(token) + " -Version " + powershellSingleQuote("v"+strings.TrimPrefix(s.panelVersion, "v"))
	if name != "" {
		command += " -Name " + powershellSingleQuote(name)
	}
	if desktop {
		command += " -EnableDesktop"
	}
	parts = append(parts, command, "} finally { foreach ($file in @($script,$manifest)) { if (Test-Path -LiteralPath $file) { Remove-Item -LiteralPath $file -Force } }; Remove-Item -LiteralPath $stage -Force }")
	return strings.Join(parts, "; ") + " }", nil
}
