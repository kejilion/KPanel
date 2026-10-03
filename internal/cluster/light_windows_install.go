package cluster

import (
	"encoding/base64"
	"regexp"
	"strings"
)

var windowsReleaseVersion = regexp.MustCompile(`^v?[0-9]+\.[0-9]+\.[0-9]+(?:-[A-Za-z0-9]+(?:[.-][A-Za-z0-9]+)*)?$`)
var windowsProfileOID = regexp.MustCompile(`^[0-2](?:\.[0-9]+)+$`)

func powershellSingleQuote(value string) string {
	// PowerShell also treats Unicode curly quotes as string delimiters. Keep
	// user/configuration text entirely outside its lexer, including host names.
	return "([Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('" + base64.StdEncoding.EncodeToString([]byte(value)) + "')))"
}

func (s *Service) lightEnrollmentCommand(platform, token, name string) (string, error) {
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
	publisher, profile := s.windowsNodePublisher, s.windowsNodeProfileOID
	if strings.TrimSpace(publisher) == "" || len(publisher) > 1024 || strings.ContainsAny(publisher, "\r\n\x00") ||
		!windowsReleaseVersion.MatchString(s.panelVersion) || (profile != "" && !windowsProfileOID.MatchString(profile)) {
		return "", ErrWindowsInstallerUnavailable
	}
	asset := "https://github.com/kejilion/KPanel/releases/download/v" + strings.TrimPrefix(s.panelVersion, "v") + "/install-windows.ps1"
	// Program Files is not writable by standard users. A random staging directory
	// there prevents replacement between signature verification and script execution.
	// The bootstrap does not run downloaded text via Invoke-Expression.
	parts := []string{
		"& { $ErrorActionPreference='Stop'",
		"if (-not ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) { throw 'Run in administrator PowerShell' }",
		"$root=[Environment]::GetFolderPath('ProgramFiles')",
		"if ((Get-Item -LiteralPath $root -Force).Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'Unsafe staging directory' }",
		"$stage=Join-Path $root ('KPanelBootstrap-'+[guid]::NewGuid().ToString('N'))",
		"$null=New-Item -ItemType Directory -Path $stage",
		"$script=Join-Path $stage 'install-windows.ps1'",
		"try { [Net.ServicePointManager]::SecurityProtocol=[Net.SecurityProtocolType]::Tls12",
		"Invoke-WebRequest -UseBasicParsing -Uri " + powershellSingleQuote(asset) + " -OutFile $script",
		"$signature=Get-AuthenticodeSignature -LiteralPath $script",
		"if ($signature.Status -ne 'Valid' -or $signature.SignerCertificate.Subject -cne " + powershellSingleQuote(publisher) + ") { throw 'Installer signature verification failed' }",
	}
	if profile != "" {
		parts = append(parts, "$oids=@($signature.SignerCertificate.Extensions | Where-Object { $_.Oid.Value -eq '2.5.29.37' } | ForEach-Object { $_.EnhancedKeyUsages } | ForEach-Object { $_.Value })",
			"if ("+powershellSingleQuote(profile)+" -notin $oids) { throw 'Installer signing profile mismatch' }")
	}
	command := "& $script -Token " + powershellSingleQuote(token) + " -Publisher " + powershellSingleQuote(publisher) + " -Version " + powershellSingleQuote("v"+strings.TrimPrefix(s.panelVersion, "v"))
	if name != "" {
		command += " -Name " + powershellSingleQuote(name)
	}
	if profile != "" {
		command += " -ProfileOID " + powershellSingleQuote(profile)
	}
	parts = append(parts, command, "} finally { if (Test-Path -LiteralPath $script) { Remove-Item -LiteralPath $script -Force }; Remove-Item -LiteralPath $stage -Force }")
	return strings.Join(parts, "; ") + " }", nil
}
