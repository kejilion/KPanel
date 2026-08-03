package systeminfo

import (
	"bufio"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

const maxConfigurationFileBytes = 1 << 20

func (c *Collector) readManagement(out *contract.SystemManagementSummary) {
	out.SSH = c.readSSHConfiguration()
	out.DNS = c.readDNSConfiguration()
	out.Hosts = c.readHostsConfiguration()
	out.Cron = c.readCronConfiguration()
	out.Timezone = c.readTimezone()
	out.Swap = c.readSwapConfiguration()
	out.PackageManager, out.PackageSources = c.readPackageSources()
	out.IPPreference = c.readIPPreference()
	out.KernelOptimization = c.readKernelOptimization()
	out.BBR = c.readBBR()
}

func (c *Collector) readCronConfiguration() []string {
	paths := []string{filepath.Join(c.VarRoot, "spool", "cron", "crontabs", "root"), filepath.Join(c.VarRoot, "spool", "cron", "root")}
	for _, path := range paths {
		data := readFileLimited(path)
		if data == "" {
			continue
		}
		var entries []string
		for _, line := range strings.Split(strings.ReplaceAll(data, "\r\n", "\n"), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") || strings.Contains(line, "\x00") {
				continue
			}
			entries = append(entries, line)
			if len(entries) == 100 {
				break
			}
		}
		return entries
	}
	return nil
}

func (c *Collector) readHostsConfiguration() []string {
	var entries []string
	for _, line := range configurationLines(readFileLimited(filepath.Join(c.EtcRoot, "hosts"))) {
		fields := strings.Fields(line)
		if len(fields) < 2 || net.ParseIP(fields[0]) == nil {
			continue
		}
		entries = append(entries, strings.Join(fields, " "))
		if len(entries) == 100 {
			break
		}
	}
	return entries
}

func (c *Collector) readSSHConfiguration() contract.SSHConfiguration {
	var files []string
	primary := filepath.Join(c.EtcRoot, "ssh", "sshd_config")
	if _, err := os.Stat(primary); err == nil {
		files = append(files, primary)
	}
	fragments, _ := filepath.Glob(filepath.Join(c.EtcRoot, "ssh", "sshd_config.d", "*.conf"))
	sort.Strings(fragments)
	files = append(files, fragments...)

	seen := make(map[uint16]bool)
	var ports []uint16
	for _, name := range files {
		for _, line := range configurationLines(readFileLimited(name)) {
			fields := strings.Fields(line)
			if len(fields) != 2 || !strings.EqualFold(fields[0], "Port") {
				continue
			}
			value, err := strconv.ParseUint(fields[1], 10, 16)
			if err != nil || value == 0 || seen[uint16(value)] {
				continue
			}
			seen[uint16(value)] = true
			ports = append(ports, uint16(value))
		}
	}
	if len(ports) == 0 {
		return contract.SSHConfiguration{Ports: []uint16{22}, Source: "default"}
	}
	return contract.SSHConfiguration{Ports: ports, Source: "configured"}
}

func (c *Collector) readDNSConfiguration() contract.DNSConfiguration {
	path := filepath.Join(c.EtcRoot, "resolv.conf")
	data := readFileLimited(path)
	manager := "unknown"
	if target, err := os.Readlink(path); err == nil {
		target = strings.ToLower(target)
		switch {
		case strings.Contains(target, "systemd/resolve"):
			manager = "systemd-resolved"
		case strings.Contains(target, "resolvconf"):
			manager = "resolvconf"
		default:
			manager = "managed"
		}
	} else if data != "" {
		manager = "static"
	}

	// KPanel writes a native systemd-resolved drop-in instead of replacing or
	// locking resolv.conf. Prefer its configured upstreams over the local
	// 127.0.0.53 stub so the UI presents the actual setting it controls.
	if manager == "systemd-resolved" {
		kpanelResolved := readFileLimited(filepath.Join(c.EtcRoot, "systemd", "resolved.conf.d", "90-kpanel.conf"))
		for _, line := range configurationLines(kpanelResolved) {
			key, value, ok := strings.Cut(line, "=")
			if !ok || !strings.EqualFold(strings.TrimSpace(key), "DNS") {
				continue
			}
			servers := strings.Fields(value)
			if len(servers) > 4 {
				servers = servers[:4]
			}
			return contract.DNSConfiguration{Servers: servers, Manager: manager}
		}
	}

	var servers []string
	seen := make(map[string]bool)
	for _, line := range configurationLines(data) {
		fields := strings.Fields(line)
		if len(fields) != 2 || !strings.EqualFold(fields[0], "nameserver") || seen[fields[1]] {
			continue
		}
		seen[fields[1]] = true
		servers = append(servers, fields[1])
		if len(servers) == 4 {
			break
		}
	}
	return contract.DNSConfiguration{Servers: servers, Manager: manager}
}

func (c *Collector) readTimezone() string {
	if value := strings.TrimSpace(readFileLimited(filepath.Join(c.EtcRoot, "timezone"))); value != "" {
		return strings.Split(value, "\n")[0]
	}
	target, err := os.Readlink(filepath.Join(c.EtcRoot, "localtime"))
	if err != nil {
		return ""
	}
	target = filepath.ToSlash(target)
	if _, zone, ok := strings.Cut(target, "/zoneinfo/"); ok {
		return zone
	}
	return ""
}

func (c *Collector) readSwapConfiguration() contract.SwapConfiguration {
	out := contract.SwapConfiguration{Path: c.SwapPath}
	if info, err := os.Lstat(c.SwapPath); err == nil && info.Mode().IsRegular() && info.Size() >= 0 {
		out.FileExists = true
		out.FileSizeBytes = uint64(info.Size())
	}
	if info, err := os.Lstat(c.LegacySwapPath); err == nil && info.Mode().IsRegular() && info.Size() >= 0 {
		out.LegacyExists = true
		out.LegacySizeBytes = uint64(info.Size())
	}
	lines := strings.Split(strings.TrimSpace(c.readOptional("swaps")), "\n")
	if len(lines) > 1 {
		for _, line := range lines[1:] {
			fields := strings.Fields(line)
			if len(fields) < 5 {
				continue
			}
			sizeKiB, sizeErr := strconv.ParseUint(fields[2], 10, 64)
			usedKiB, usedErr := strconv.ParseUint(fields[3], 10, 64)
			if sizeErr != nil || usedErr != nil {
				continue
			}
			sizeBytes := sizeKiB * 1024
			usedBytes := usedKiB * 1024
			out.ActiveDevices++
			switch fields[0] {
			case c.SwapPath:
				primaryMatch := out.FileExists &&
					swapFileMatchesActiveSize(out.FileSizeBytes, sizeBytes)
				legacyMatch := out.LegacyExists &&
					swapFileMatchesActiveSize(out.LegacySizeBytes, sizeBytes)
				if legacyMatch && (!primaryMatch || out.FileActive) {
					out.LegacyActive = true
				} else {
					out.FileExists = true
					out.FileActive = true
					if out.FileSizeBytes == 0 {
						out.FileSizeBytes = sizeBytes
					}
					out.FileUsedBytes += usedBytes
				}
			case c.LegacySwapPath:
				out.LegacyExists = true
				out.LegacyActive = true
				if out.LegacySizeBytes == 0 {
					out.LegacySizeBytes = sizeBytes
				}
			default:
				out.OtherActiveDevices++
				out.OtherSwapTotalBytes += sizeBytes
				out.OtherSwapUsedBytes += usedBytes
			}
		}
	}
	return out
}

func swapFileMatchesActiveSize(fileSize, activeSize uint64) bool {
	if fileSize < activeSize {
		return false
	}
	const swapMetadataAllowance = 8 * 1024 * 1024
	return fileSize-activeSize <= swapMetadataAllowance
}

func (c *Collector) readPackageSources() (string, []string) {
	type sourceFile struct {
		manager string
		path    string
	}
	files := []sourceFile{
		{manager: "apt", path: filepath.Join(c.EtcRoot, "apt", "sources.list")},
		{manager: "apk", path: filepath.Join(c.EtcRoot, "apk", "repositories")},
		{manager: "pacman", path: filepath.Join(c.EtcRoot, "pacman.d", "mirrorlist")},
	}
	for _, pattern := range []struct {
		manager string
		pattern string
	}{
		{"apt", filepath.Join(c.EtcRoot, "apt", "sources.list.d", "*.list")},
		{"apt", filepath.Join(c.EtcRoot, "apt", "sources.list.d", "*.sources")},
		{"rpm", filepath.Join(c.EtcRoot, "yum.repos.d", "*.repo")},
		{"zypper", filepath.Join(c.EtcRoot, "zypp", "repos.d", "*.repo")},
	} {
		matches, _ := filepath.Glob(pattern.pattern)
		sort.Strings(matches)
		for _, match := range matches {
			files = append(files, sourceFile{manager: pattern.manager, path: match})
		}
	}

	manager := ""
	seen := make(map[string]bool)
	var sources []string
	for _, file := range files {
		data := readFileLimited(file.path)
		if data == "" {
			continue
		}
		if manager == "" {
			manager = file.manager
		}
		for _, line := range configurationLines(data) {
			for _, field := range strings.Fields(line) {
				field = strings.TrimSpace(strings.TrimPrefix(field, "URIs:"))
				field = strings.TrimSpace(strings.TrimPrefix(field, "baseurl="))
				parsed, err := url.Parse(field)
				if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" {
					continue
				}
				host := strings.ToLower(parsed.Hostname())
				if !seen[host] {
					seen[host] = true
					sources = append(sources, host)
				}
				if len(sources) == 4 {
					return manager, sources
				}
			}
		}
	}
	return manager, sources
}

func (c *Collector) readIPPreference() string {
	for _, line := range configurationLines(readFileLimited(filepath.Join(c.EtcRoot, "gai.conf"))) {
		fields := strings.Fields(line)
		if len(fields) == 3 && fields[0] == "precedence" && fields[1] == "::ffff:0:0/96" && fields[2] == "100" {
			return "ipv4"
		}
	}
	return "system_default"
}

func (c *Collector) readKernelOptimization() contract.KernelOptimizationSummary {
	path := filepath.Join(c.EtcRoot, "sysctl.d", "99-kejilion-optimize.conf")
	data := readFileLimited(path)
	if data != "" {
		for _, line := range strings.Split(data, "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "# 模式:") {
				profile := strings.TrimSpace(strings.SplitN(strings.TrimPrefix(trimmed, "# 模式:"), "|", 2)[0])
				return contract.KernelOptimizationSummary{Enabled: true, Profile: profile, Source: "kejilion"}
			}
		}
		return contract.KernelOptimizationSummary{Enabled: true, Profile: "自定义", Source: "kejilion"}
	}
	if readFileLimited(filepath.Join(c.EtcRoot, "sysctl.d", "99-network-optimize.conf")) != "" {
		return contract.KernelOptimizationSummary{Enabled: true, Profile: "自动调优模式", Source: "kejilion"}
	}
	return contract.KernelOptimizationSummary{}
}

func (c *Collector) readBBR() contract.BBRSummary {
	current := strings.TrimSpace(c.readOptional("sys/net/ipv4/tcp_congestion_control"))
	qdisc := strings.TrimSpace(c.readOptional("sys/net/core/default_qdisc"))
	available := strings.Fields(c.readOptional("sys/net/ipv4/tcp_available_congestion_control"))
	supported := false
	for _, name := range available {
		if name == "bbr" {
			supported = true
			break
		}
	}
	return contract.BBRSummary{
		Supported: supported, Enabled: current == "bbr",
		CongestionControl: current, DefaultQDisc: qdisc, Available: available,
	}
}

func configurationLines(data string) []string {
	var lines []string
	scanner := bufio.NewScanner(strings.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if before, _, ok := strings.Cut(line, "#"); ok {
			line = strings.TrimSpace(before)
		}
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func readFileLimited(path string) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()
	data, _ := io.ReadAll(io.LimitReader(file, maxConfigurationFileBytes))
	return string(data)
}
