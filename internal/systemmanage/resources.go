package systemmanage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

const (
	resourceAddressOutputLimit = 256 << 10
	resourceNetworkScanLimit   = 4096
	resourceReceiptOutputLimit = 16 << 10
	resourceScriptMaxBytes     = 4 << 20
	resourceWriterLockTimeout  = 2 * time.Second
	resourceActionTimeout      = 45 * time.Second
)

var (
	errResourceOutputTooLarge = errors.New("system resource output exceeds its configured limit")
	resourceVersionPattern    = regexp.MustCompile(`^[a-f0-9]{64}$`)
	resourceProtocolV4Pattern = regexp.MustCompile(`(?m)^KPANEL_SYSTEM_RESOURCE_PROTOCOL_VERSION="4"\r?$`)
	resourceRecoveryPattern   = regexp.MustCompile(`^/var/lib/kejilion-panel/system/recovery/system-resource/[0-9]{8}T[0-9]{6}Z-(?:hosts|cron|firewall)\.[A-Za-z0-9]{6}$`)
	firewallCounterPattern    = regexp.MustCompile(`^(.* )\[[0-9]+:[0-9]+\]$`)
	firewallDDoSTCPLimit      = regexp.MustCompile(`^-A INPUT -p tcp(?: -m tcp)? (?:--syn|--tcp-flags (?:FIN,SYN,RST,ACK SYN|SYN,RST,ACK,FIN SYN|0x17 0x02|0x17/0x02)) -m limit --limit [0-9]+/(?:s|sec|second) --limit-burst 100 -j ACCEPT$`)
	firewallDDoSTCPDrop       = regexp.MustCompile(`^-A INPUT -p tcp(?: -m tcp)? (?:--syn|--tcp-flags (?:FIN,SYN,RST,ACK SYN|SYN,RST,ACK,FIN SYN|0x17 0x02|0x17/0x02)) -j DROP$`)
	firewallDDoSUDPLimit      = regexp.MustCompile(`^-A INPUT -p udp(?: -m udp)? -m limit --limit [0-9]+/(?:s|sec|second)(?: --limit-burst 5)? -j ACCEPT$`)
	firewallDDoSUDPDrop       = regexp.MustCompile(`^-A INPUT -p udp(?: -m udp)? -j DROP$`)
	firewallManagedPingRule   = regexp.MustCompile(`^-A INPUT -p icmp(?: -m icmp)? --icmp-type (?:8|echo-request) -j (?:ACCEPT|DROP)$`)
	firewallCountrySetPattern = regexp.MustCompile(`^([a-z]{2})_block$`)
	cronEnvironmentPattern    = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*\s*=`)
	cronStandardLinePattern   = regexp.MustCompile(`^(\S+\s+\S+\s+\S+\s+\S+\s+\S+)\s+(.+)$`)
	cronMacroLinePattern      = regexp.MustCompile(`^(@\S+)\s+(.+)$`)
)

type resourceCommandRunner interface {
	RunResource(context.Context, int, []byte, string, ...string) ([]byte, []byte, error)
}

type boundedResourceBuffer struct {
	buffer   bytes.Buffer
	limit    int
	overflow bool
}

func (buffer *boundedResourceBuffer) Write(value []byte) (int, error) {
	written := len(value)
	remaining := buffer.limit - buffer.buffer.Len()
	if remaining > 0 {
		if remaining > len(value) {
			remaining = len(value)
		}
		_, _ = buffer.buffer.Write(value[:remaining])
	}
	if remaining < len(value) {
		buffer.overflow = true
	}
	return written, nil
}

func (runner commandRunner) RunResource(
	ctx context.Context,
	limit int,
	input []byte,
	name string,
	arguments ...string,
) ([]byte, []byte, error) {
	command := exec.CommandContext(ctx, name, arguments...)
	command.Env = append(
		os.Environ(),
		"LC_ALL=C",
		"LANG=C",
		"DEBIAN_FRONTEND=noninteractive",
		"NEEDRESTART_MODE=a",
		"APT_LISTCHANGES_FRONTEND=none",
	)
	if input != nil {
		command.Stdin = bytes.NewReader(input)
	}
	stdout := &boundedResourceBuffer{limit: limit}
	stderr := &boundedResourceBuffer{limit: 4096}
	command.Stdout = stdout
	command.Stderr = stderr
	err := command.Run()
	if stdout.overflow || stderr.overflow {
		return stdout.buffer.Bytes(), stderr.buffer.Bytes(), errResourceOutputTooLarge
	}
	if err != nil {
		detail := strings.TrimSpace(stderr.buffer.String())
		if detail != "" {
			return stdout.buffer.Bytes(), stderr.buffer.Bytes(), fmt.Errorf("%s: %w", detail, err)
		}
	}
	return stdout.buffer.Bytes(), stderr.buffer.Bytes(), err
}

func (m *Manager) runResourceCommand(
	ctx context.Context,
	limit int,
	name string,
	arguments ...string,
) ([]byte, []byte, error) {
	return m.runResourceCommandInput(ctx, limit, nil, name, arguments...)
}

func (m *Manager) runResourceCommandInput(
	ctx context.Context,
	limit int,
	input []byte,
	name string,
	arguments ...string,
) ([]byte, []byte, error) {
	if runner, ok := m.runner.(resourceCommandRunner); ok {
		return runner.RunResource(ctx, limit, input, name, arguments...)
	}
	if input != nil {
		return nil, nil, fmt.Errorf("%w: command runner does not support bounded stdin", ErrUnsupported)
	}
	output, err := m.runner.Run(ctx, name, arguments...)
	if len(output) > limit {
		return output[:limit], nil, errResourceOutputTooLarge
	}
	return output, nil, err
}

// SystemResourceCapabilities reports read support independently from the
// trusted-script write adapter for each typed resource domain.
func (m *Manager) SystemResourceCapabilities() []contract.Capability {
	readErrors := map[string]error{
		"hosts":              m.resourceReadAvailability("hosts"),
		"cron":               m.resourceReadAvailability("cron"),
		"network-interfaces": m.resourceReadAvailability("network-interfaces"),
		"firewall":           m.resourceReadAvailability("firewall"),
	}
	commonWriteErr := m.resourceCommonWriteAvailability()
	read := func(id, resource string) contract.Capability {
		err := readErrors[resource]
		if err != nil {
			return contract.Capability{ID: id, Enabled: false, Reason: resourceCapabilityReason(err)}
		}
		return contract.Capability{ID: id, Enabled: true, Methods: []string{"GET"}}
	}
	write := func(id, resource string) contract.Capability {
		err := commonWriteErr
		if err == nil {
			err = m.resourceSpecificWriteAvailability(resource, readErrors[resource])
		}
		if err != nil {
			return contract.Capability{ID: id, Enabled: false, Reason: resourceCapabilityReason(err)}
		}
		return contract.Capability{ID: id, Enabled: true, Methods: []string{"POST"}}
	}
	return []contract.Capability{
		read("system.hosts.read", "hosts"),
		write("system.hosts.write", "hosts"),
		read("system.cron.read", "cron"),
		write("system.cron.write", "cron"),
		read("system.network-interfaces.read", "network-interfaces"),
		write("system.network-interfaces.write", "network-interfaces"),
		read("system.firewall.read", "firewall"),
		write("system.firewall.write", "firewall"),
	}
}

func resourceCapabilityReason(err error) string {
	if err == nil {
		return ""
	}
	value := strings.TrimSpace(err.Error())
	for _, prefix := range []error{ErrDisabled, ErrUnsupported} {
		value = strings.TrimPrefix(value, prefix.Error()+": ")
	}
	return value
}

func (m *Manager) resourceReadAvailability(resource string) error {
	switch resource {
	case "hosts":
		info, err := os.Lstat(filepath.Join(m.etcRoot, "hosts"))
		if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 ||
			info.Size() > contract.SystemHostsMaxBytes {
			return fmt.Errorf("%w: /etc/hosts is not a readable regular file", ErrUnsupported)
		}
	case "cron":
		if err := m.cronReadPrivilegeAvailability(); err != nil {
			return err
		}
		if _, err := m.runner.LookPath("crontab"); err != nil {
			return fmt.Errorf("%w: crontab is unavailable", ErrUnsupported)
		}
	case "network-interfaces":
		if runtime.GOOS != "linux" {
			return fmt.Errorf("%w: network interfaces are only available on Linux", ErrUnsupported)
		}
		if info, err := os.Stat(filepath.Join(m.sysRoot, "class", "net")); err != nil || !info.IsDir() {
			return fmt.Errorf("%w: /sys/class/net is unavailable", ErrUnsupported)
		}
		if _, err := m.runner.LookPath("ip"); err != nil {
			return fmt.Errorf("%w: ip is unavailable", ErrUnsupported)
		}
	case "firewall":
		if runtime.GOOS != "linux" {
			return fmt.Errorf("%w: firewall state is only available on Linux", ErrUnsupported)
		}
		if err := m.firewallReadPrivilegeAvailability(); err != nil {
			return err
		}
		if _, err := m.runner.LookPath("iptables-save"); err != nil {
			return fmt.Errorf("%w: iptables-save is unavailable", ErrUnsupported)
		}
	default:
		return fmt.Errorf("%w: unknown system resource", ErrUnsupported)
	}
	return nil
}

func (m *Manager) cronReadPrivilegeAvailability() error {
	if m.effectiveUID() != 0 {
		return fmt.Errorf("%w: root crontab requires root privileges", ErrUnsupported)
	}
	return nil
}

func (m *Manager) firewallReadPrivilegeAvailability() error {
	if !m.hasEffectiveCapability(12) {
		return fmt.Errorf("%w: CAP_NET_ADMIN is unavailable", ErrUnsupported)
	}
	return nil
}

func (m *Manager) resourceWriteAvailability(resource string) error {
	if err := m.resourceCommonWriteAvailability(); err != nil {
		return err
	}
	readResource := resource
	if resource == "network-interface" {
		readResource = "network-interfaces"
	}
	return m.resourceSpecificWriteAvailability(resource, m.resourceReadAvailability(readResource))
}

func (m *Manager) resourceCommonWriteAvailability() error {
	if !m.enabled {
		return fmt.Errorf("%w: 宿主机系统写入开关未启用", ErrDisabled)
	}
	if runtime.GOOS != "linux" {
		return fmt.Errorf("%w: 系统资源写入仅支持 Linux", ErrUnsupported)
	}
	if m.effectiveUID() != 0 {
		return fmt.Errorf("%w: Agent 必须以受限 root 服务运行", ErrUnsupported)
	}
	commands := []string{"env", "bash", "sha256sum", "mktemp", "flock"}
	for _, command := range commands {
		if _, err := m.runner.LookPath(command); err != nil {
			return fmt.Errorf("%w: %s is unavailable", ErrUnsupported, command)
		}
	}
	if _, err := m.systemResourceScriptPath(); err != nil {
		return err
	}
	return nil
}

func (m *Manager) resourceSpecificWriteAvailability(resource string, readErr error) error {
	if readErr != nil {
		return readErr
	}
	if (resource == "network-interfaces" || resource == "network-interface" || resource == "firewall") &&
		!m.hasEffectiveCapability(12) {
		return fmt.Errorf("%w: CAP_NET_ADMIN is unavailable", ErrUnsupported)
	}
	commands := []string{}
	switch resource {
	case "hosts":
	case "cron":
		commands = append(commands, "crontab")
	case "network-interfaces", "network-interface":
		commands = append(commands, "ip")
	case "firewall":
		commands = append(commands, "iptables", "iptables-save", "iptables-restore", "crontab")
	default:
		return fmt.Errorf("%w: unknown system resource", ErrUnsupported)
	}
	for _, command := range commands {
		if _, err := m.runner.LookPath(command); err != nil {
			return fmt.Errorf("%w: %s is unavailable", ErrUnsupported, command)
		}
	}
	return nil
}

func (m *Manager) resourceActionDependencyAvailability(action string) error {
	commands := []string{}
	switch action {
	case "firewall-block-country", "firewall-allow-country":
		commands = []string{"ipset", "wget"}
	case "firewall-remove-country":
		commands = []string{"ipset"}
	default:
		return nil
	}
	for _, command := range commands {
		if _, err := m.runner.LookPath(command); err != nil {
			return fmt.Errorf("%w: %s is unavailable for country firewall rules", ErrUnsupported, command)
		}
	}
	return nil
}

func (m *Manager) hasEffectiveCapability(bit uint) bool {
	file, err := os.Open(filepath.Join(m.procRoot, "self", "status"))
	if err != nil {
		return false
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, 64<<10))
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(content), "\n") {
		if !strings.HasPrefix(line, "CapEff:") {
			continue
		}
		value, err := strconv.ParseUint(strings.TrimSpace(strings.TrimPrefix(line, "CapEff:")), 16, 64)
		return err == nil && value&(uint64(1)<<bit) != 0
	}
	return false
}

// Hosts reads the real hosts file without creating a Panel-owned shadow copy.
func (m *Manager) Hosts(ctx context.Context) (contract.SystemHostsSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return contract.SystemHostsSnapshot{}, err
	}
	raw, err := readResourceFile(filepath.Join(m.etcRoot, "hosts"), contract.SystemHostsMaxBytes)
	if err != nil {
		return contract.SystemHostsSnapshot{}, fmt.Errorf("%w: read hosts: %v", ErrUnsupported, err)
	}
	lines := resourceLines(raw)
	entries := make([]contract.SystemHostEntry, 0, min(len(lines), contract.SystemHostsMaxLines))
	total := 0
	for index, rawLine := range lines {
		entry, ok := parseHostLine(index+1, rawLine)
		if !ok {
			continue
		}
		total++
		if index < contract.SystemHostsMaxLines {
			entries = append(entries, entry)
		}
	}
	return contract.SystemHostsSnapshot{
		ResourceVersion: resourceHash(raw), Entries: entries, Total: total,
		Truncated: len(lines) > contract.SystemHostsMaxLines || total > len(entries),
	}, nil
}

func parseHostLine(line int, raw string) (contract.SystemHostEntry, bool) {
	content := raw
	comment := ""
	if index := strings.Index(content, "#"); index >= 0 {
		comment = strings.TrimSpace(content[index+1:])
		content = content[:index]
	}
	fields := strings.Fields(content)
	if len(fields) < 2 || net.ParseIP(fields[0]) == nil {
		return contract.SystemHostEntry{}, false
	}
	return contract.SystemHostEntry{
		Line: line, Address: fields[0], Hostnames: append([]string(nil), fields[1:]...),
		Comment: comment, Raw: raw,
	}, true
}

// Cron reads root's active crontab. crontab's conventional exit status for a
// missing table is represented as an empty byte stream and empty snapshot.
func (m *Manager) Cron(ctx context.Context) (contract.SystemCronSnapshot, error) {
	if err := m.cronReadPrivilegeAvailability(); err != nil {
		return contract.SystemCronSnapshot{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	raw, stderr, err := m.runResourceCommand(ctx, contract.SystemCronMaxBytes, "crontab", "-l")
	if err != nil {
		if noCrontab(stderr, err) {
			raw = []byte{}
		} else {
			return contract.SystemCronSnapshot{}, fmt.Errorf("%w: read crontab: %v", ErrUnsupported, err)
		}
	}
	lines := resourceLines(raw)
	limit := min(len(lines), contract.SystemCronMaxLines)
	entries := make([]contract.SystemCronEntry, 0, limit)
	for index := 0; index < limit; index++ {
		entries = append(entries, parseCronLine(index+1, lines[index]))
	}
	return contract.SystemCronSnapshot{
		ResourceVersion: resourceHash(raw), Entries: entries, Total: len(lines),
		Truncated: len(lines) > contract.SystemCronMaxLines,
	}, nil
}

func noCrontab(stderr []byte, err error) bool {
	value := strings.ToLower(string(stderr) + " " + err.Error())
	return strings.Contains(value, "no crontab for")
}

func parseCronLine(line int, raw string) contract.SystemCronEntry {
	entry := contract.SystemCronEntry{Line: line, Kind: "unknown", Raw: raw}
	trimmed := strings.TrimSpace(raw)
	switch {
	case trimmed == "":
		entry.Kind = "blank"
	case strings.HasPrefix(trimmed, "#"):
		entry.Kind = "comment"
	case cronEnvironmentPattern.MatchString(trimmed):
		entry.Kind = "environment"
	case cronMacroLinePattern.MatchString(trimmed):
		matches := cronMacroLinePattern.FindStringSubmatch(trimmed)
		entry.Kind, entry.Expression, entry.Command = "job", matches[1], matches[2]
	case cronStandardLinePattern.MatchString(trimmed):
		matches := cronStandardLinePattern.FindStringSubmatch(trimmed)
		entry.Kind = "job"
		entry.Expression = strings.Join(strings.Fields(matches[1]), " ")
		entry.Command = matches[2]
	}
	return entry
}

type ipAddressRecord struct {
	Name      string `json:"ifname"`
	Addresses []struct {
		Family string `json:"family"`
		Local  string `json:"local"`
		Prefix int    `json:"prefixlen"`
	} `json:"addr_info"`
}

// NetworkInterfaces reads bounded sysfs identities and one bounded ip JSON
// snapshot. State is the administrative IFF_UP bit rather than carrier-backed
// operstate, so an enabled interface without link is still represented as up.
// No interface is hidden based on loopback or routing importance.
func (m *Manager) NetworkInterfaces(ctx context.Context) (contract.SystemNetworkSnapshot, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	root := filepath.Join(m.sysRoot, "class", "net")
	directory, err := os.Open(root)
	if err != nil {
		return contract.SystemNetworkSnapshot{}, fmt.Errorf("%w: read network interfaces: %v", ErrUnsupported, err)
	}
	defer directory.Close()
	directoryEntries, err := directory.ReadDir(resourceNetworkScanLimit + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return contract.SystemNetworkSnapshot{}, fmt.Errorf("%w: read network interfaces: %v", ErrUnsupported, err)
	}
	if len(directoryEntries) > resourceNetworkScanLimit {
		return contract.SystemNetworkSnapshot{}, fmt.Errorf(
			"%w: network interface count exceeds %d",
			ErrUnsupported,
			resourceNetworkScanLimit,
		)
	}
	names := make([]string, 0, len(directoryEntries))
	for _, entry := range directoryEntries {
		if entry.Name() != "" {
			names = append(names, entry.Name())
		}
	}
	slices.Sort(names)
	rawAddresses, _, err := m.runResourceCommand(ctx, resourceAddressOutputLimit, "ip", "-j", "address", "show")
	if err != nil {
		return contract.SystemNetworkSnapshot{}, fmt.Errorf("%w: read interface addresses: %v", ErrUnsupported, err)
	}
	var addressRecords []ipAddressRecord
	if err := json.Unmarshal(rawAddresses, &addressRecords); err != nil {
		return contract.SystemNetworkSnapshot{}, fmt.Errorf("%w: parse interface addresses: %v", ErrUnsupported, err)
	}
	addresses := make(map[string][]string, len(addressRecords))
	for _, record := range addressRecords {
		name := record.Name
		if index := strings.IndexByte(name, '@'); index >= 0 {
			name = name[:index]
		}
		for _, address := range record.Addresses {
			if address.Local == "" || (address.Family != "inet" && address.Family != "inet6") {
				continue
			}
			addresses[name] = append(addresses[name], address.Local+"/"+strconv.Itoa(address.Prefix))
		}
		slices.Sort(addresses[name])
	}
	total := len(names)
	limit := min(total, contract.SystemNetworkMaxEntries)
	entries := make([]contract.SystemNetworkInterface, 0, limit)
	records := make([]string, 0, limit)
	for _, name := range names[:limit] {
		flags, err := readResourceValue(filepath.Join(root, name, "flags"), 64)
		if err != nil {
			return contract.SystemNetworkSnapshot{}, fmt.Errorf("%w: read %s state: %v", ErrUnsupported, name, err)
		}
		flagValue, err := strconv.ParseUint(strings.TrimPrefix(strings.ToLower(flags), "0x"), 16, 64)
		if err != nil {
			return contract.SystemNetworkSnapshot{}, fmt.Errorf("%w: parse %s flags: %v", ErrUnsupported, name, err)
		}
		state := "down"
		if flagValue&1 != 0 {
			state = "up"
		}
		mac, err := readResourceValue(filepath.Join(root, name, "address"), 64)
		if err != nil {
			return contract.SystemNetworkSnapshot{}, fmt.Errorf("%w: read %s address: %v", ErrUnsupported, name, err)
		}
		record := name + "|" + state + "|" + mac
		records = append(records, record)
		entries = append(entries, contract.SystemNetworkInterface{
			Name: name, State: state, MACAddress: mac,
			Addresses: append([]string{}, addresses[name]...), Loopback: name == "lo",
			ResourceVersion: resourceHash([]byte(record)),
		})
	}
	return contract.SystemNetworkSnapshot{
		ResourceVersion: resourceHash([]byte(strings.Join(records, "\n"))),
		Entries:         entries, Total: total, Truncated: total > limit,
	}, nil
}

// Firewall parses the exact iptables-save stdout but removes generated
// timestamps and live counters before optimistic-version hashing.
func (m *Manager) Firewall(ctx context.Context) (contract.SystemFirewallSnapshot, error) {
	if err := m.firewallReadPrivilegeAvailability(); err != nil {
		return contract.SystemFirewallSnapshot{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	raw, _, err := m.runResourceCommand(ctx, contract.SystemFirewallMaxBytes, "iptables-save")
	if err != nil {
		return contract.SystemFirewallSnapshot{}, fmt.Errorf("%w: read firewall: %v", ErrUnsupported, err)
	}
	ipsetAvailable := false
	var ipsetRaw []byte
	if _, lookErr := m.runner.LookPath("ipset"); lookErr == nil {
		ipsetAvailable = true
		ipsetRaw, _, err = m.runResourceCommand(ctx, contract.SystemFirewallIPSetMaxBytes, "ipset", "save")
		if err != nil {
			return contract.SystemFirewallSnapshot{}, fmt.Errorf("%w: read firewall country sets: %v", ErrUnsupported, err)
		}
	}
	lines := resourceLines(raw)
	backend := "iptables"
	inputPolicy := ""
	rules := make([]contract.SystemFirewallRule, 0, min(len(lines), contract.SystemFirewallMaxLines))
	total := 0
	pingAllowed := false
	pingDecision := false
	var ddosRules uint8
	inFilterTable := false
	for index, rawLine := range lines {
		lower := strings.ToLower(rawLine)
		if strings.Contains(lower, "(nf_tables)") {
			backend = "iptables-nft"
		} else if strings.Contains(lower, "(legacy)") {
			backend = "iptables-legacy"
		}
		switch {
		case rawLine == "*filter":
			inFilterTable = true
			continue
		case strings.HasPrefix(rawLine, "*"):
			inFilterTable = false
			continue
		case rawLine == "COMMIT":
			inFilterTable = false
			continue
		case !inFilterTable:
			continue
		}
		if strings.HasPrefix(rawLine, ":INPUT ") {
			fields := strings.Fields(rawLine)
			if len(fields) >= 2 {
				inputPolicy = fields[1]
			}
		}
		rule, ok := parseFirewallRule(index+1, rawLine)
		if !ok {
			continue
		}
		total++
		if len(rules) < contract.SystemFirewallMaxLines {
			rules = append(rules, rule)
		}
		if !pingDecision && rule.Chain == "INPUT" && rule.Protocol == "icmp" &&
			firewallManagedPingRule.MatchString(rawLine) {
			switch rule.Target {
			case "ACCEPT":
				pingAllowed, pingDecision = true, true
			case "DROP", "REJECT":
				pingAllowed, pingDecision = false, true
			}
		}
		ddosRules |= firewallDDoSRuleSignature(rawLine)
	}
	if !pingDecision {
		pingAllowed = inputPolicy == "ACCEPT"
	}
	resourceVersion := firewallResourceVersion(raw)
	if ipsetAvailable {
		resourceVersion = firewallResourceVersion(raw, ipsetRaw)
	}
	return contract.SystemFirewallSnapshot{
		ResourceVersion: resourceVersion, Backend: backend, InputPolicy: inputPolicy,
		Rules: rules, Total: total, Truncated: total > len(rules),
		CountryRules: parseFirewallCountryRules(rules, ipsetRaw),
		PingAllowed:  pingAllowed, DDoSEnabled: ddosRules == 0b1111,
	}, nil
}

func firewallDDoSRuleSignature(raw string) uint8 {
	switch {
	case firewallDDoSTCPLimit.MatchString(raw):
		return 1 << 0
	case firewallDDoSTCPDrop.MatchString(raw):
		return 1 << 1
	case firewallDDoSUDPLimit.MatchString(raw):
		return 1 << 2
	case firewallDDoSUDPDrop.MatchString(raw):
		return 1 << 3
	default:
		return 0
	}
}

func firewallResourceVersion(raw []byte, ipsetRaw ...[]byte) string {
	canonical := canonicalFirewallRules(raw)
	if len(ipsetRaw) == 0 {
		return resourceHash(canonical)
	}
	value := make([]byte, 0, len(canonical)+len(ipsetRaw[0])+16)
	value = append(value, "iptables\n"...)
	value = append(value, canonical...)
	value = append(value, "ipsets\n"...)
	value = append(value, canonicalFirewallIPSets(ipsetRaw[0])...)
	return resourceHash(value)
}

func canonicalFirewallIPSets(raw []byte) []byte {
	lines := resourceLines(raw)
	if len(lines) == 0 {
		return nil
	}
	return []byte(strings.Join(lines, "\n") + "\n")
}

func parseFirewallCountryRules(rules []contract.SystemFirewallRule, ipsetRaw []byte) []contract.SystemFirewallCountryRule {
	networks := make(map[string]map[string]struct{})
	for _, line := range resourceLines(ipsetRaw) {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "create" {
			if match := firewallCountrySetPattern.FindStringSubmatch(fields[1]); match != nil {
				if networks[match[1]] == nil {
					networks[match[1]] = make(map[string]struct{})
				}
			}
			continue
		}
		if len(fields) < 3 || fields[0] != "add" {
			continue
		}
		match := firewallCountrySetPattern.FindStringSubmatch(fields[1])
		if match == nil {
			continue
		}
		if networks[match[1]] == nil {
			networks[match[1]] = make(map[string]struct{})
		}
		networks[match[1]][fields[2]] = struct{}{}
	}

	result := make([]contract.SystemFirewallCountryRule, 0)
	seen := make(map[string]bool)
	for _, rule := range rules {
		if rule.Chain != "INPUT" {
			continue
		}
		setName, ok := firewallCountrySetFromRaw(rule.Raw)
		if !ok {
			continue
		}
		decision := ""
		switch strings.ToUpper(rule.Target) {
		case "ACCEPT":
			decision = "allow"
		case "DROP", "REJECT":
			decision = "block"
		default:
			continue
		}
		match := firewallCountrySetPattern.FindStringSubmatch(setName)
		if match == nil {
			continue
		}
		key := match[1] + "|" + decision
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, contract.SystemFirewallCountryRule{
			Code: strings.ToUpper(match[1]), Decision: decision, Zone: "inbound",
			NetworkCount: len(networks[match[1]]),
		})
	}
	slices.SortFunc(result, func(left, right contract.SystemFirewallCountryRule) int {
		if left.Code != right.Code {
			return strings.Compare(left.Code, right.Code)
		}
		if left.Decision == right.Decision {
			return 0
		}
		if left.Decision == "block" {
			return -1
		}
		return 1
	})
	return result
}

func firewallCountrySetFromRaw(raw string) (string, bool) {
	fields := strings.Fields(raw)
	for index := 0; index+2 < len(fields); index++ {
		if fields[index] == "--match-set" && fields[index+2] == "src" &&
			firewallCountrySetPattern.MatchString(fields[index+1]) {
			return fields[index+1], true
		}
	}
	return "", false
}

func canonicalFirewallRules(raw []byte) []byte {
	lines := resourceLines(raw)
	canonical := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.HasPrefix(line, "# Generated by iptables-save ") ||
			strings.HasPrefix(line, "# Completed on ") {
			continue
		}
		if strings.HasPrefix(line, ":") {
			line = firewallCounterPattern.ReplaceAllString(line, `${1}[0:0]`)
		}
		canonical = append(canonical, line)
	}
	if len(canonical) == 0 {
		return nil
	}
	return []byte(strings.Join(canonical, "\n") + "\n")
}

func parseFirewallRule(line int, raw string) (contract.SystemFirewallRule, bool) {
	fields := strings.Fields(raw)
	if len(fields) < 4 || fields[0] != "-A" {
		return contract.SystemFirewallRule{}, false
	}
	rule := contract.SystemFirewallRule{
		Line: line, Chain: fields[1], Protocol: "all", Source: "0.0.0.0/0",
		Destination: "0.0.0.0/0", Options: []string{}, Raw: raw,
	}
	for index := 2; index < len(fields); index++ {
		field := fields[index]
		if index+1 < len(fields) {
			switch field {
			case "-p":
				rule.Protocol = fields[index+1]
				index++
				continue
			case "-s":
				rule.Source = fields[index+1]
				index++
				continue
			case "-d":
				rule.Destination = fields[index+1]
				index++
				continue
			case "-j":
				rule.Target = fields[index+1]
				index++
				continue
			}
		}
		rule.Options = append(rule.Options, field)
	}
	return rule, true
}

// ExecuteSystemResourceAction delegates all mutations to the trusted
// kejilion.sh machine protocol. Go performs an optimistic preflight and exact
// post-write readback but does not implement a competing write path.
func (m *Manager) ExecuteSystemResourceAction(
	ctx context.Context,
	request contract.SystemResourceActionRequest,
) (contract.SystemResourceActionResult, error) {
	if field, detail := contract.ValidateSystemResourceAction(&request); field != "" {
		return contract.SystemResourceActionResult{}, fmt.Errorf("%w: %s: %s", ErrInvalidInput, field, detail)
	}
	resource, action, arguments, input := systemResourceInvocation(request)
	if err := m.resourceWriteAvailability(resource); err != nil {
		return contract.SystemResourceActionResult{}, err
	}
	if err := m.resourceActionDependencyAvailability(request.Action); err != nil {
		return contract.SystemResourceActionResult{}, err
	}
	lockContext, cancelLock := context.WithTimeout(ctx, resourceWriterLockTimeout)
	if !lockSystemResource(lockContext, &m.mu) {
		cancelLock()
		return contract.SystemResourceActionResult{}, fmt.Errorf("%w: timed out waiting for the system resource writer", ErrConflict)
	}
	cancelLock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return contract.SystemResourceActionResult{}, err
	}
	// Once a privileged transaction owns the writer lock, let it finish even if
	// the browser disconnects. The independent deadline still bounds the worker.
	transactionContext, cancelTransaction := context.WithTimeout(context.WithoutCancel(ctx), resourceActionTimeout)
	defer cancelTransaction()
	ctx = transactionContext

	currentVersion, err := m.systemResourceVersion(ctx, resource, request)
	if err != nil {
		return contract.SystemResourceActionResult{}, err
	}
	if currentVersion != request.ExpectedResourceVersion {
		return contract.SystemResourceActionResult{}, fmt.Errorf("%w: expected resource version is stale", ErrConflict)
	}
	if err := m.validateResourceTarget(ctx, resource, request); err != nil {
		return contract.SystemResourceActionResult{}, err
	}
	script, err := m.systemResourceScriptPath()
	if err != nil {
		return contract.SystemResourceActionResult{}, err
	}
	commandArguments := []string{
		"KJ_SYSTEM_RESOURCE_NONINTERACTIVE=1", "bash", script,
		"kpanel", "system-resource", resource, action,
		request.ExpectedResourceVersion,
	}
	commandArguments = append(commandArguments, arguments...)
	output, _, runErr := m.runResourceCommandInput(
		ctx, resourceReceiptOutputLimit, input, "env", commandArguments...,
	)
	receipt, receiptErr := parseSystemResourceReceipt(output)
	if receiptErr != nil {
		return contract.SystemResourceActionResult{}, fmt.Errorf(
			"%w: system resource receipt is invalid: %v", ErrNeedsAttention, receiptErr,
		)
	}
	switch receipt.Status {
	case "conflict":
		return contract.SystemResourceActionResult{}, fmt.Errorf("%w: script detected a locked resource conflict", ErrConflict)
	case "failed":
		return contract.SystemResourceActionResult{}, fmt.Errorf("%w: kejilion.sh reported a completed rollback", ErrRolledBack)
	case "rollback-failed":
		detail := "kejilion.sh reported rollback failure"
		if receipt.Backup != "" {
			detail += "; recovery backup: " + receipt.Backup
		}
		return contract.SystemResourceActionResult{}, fmt.Errorf("%w: %s", ErrRollbackFailed, detail)
	case "needs-attention":
		return contract.SystemResourceActionResult{}, fmt.Errorf("%w: kejilion.sh requested manual inspection", ErrNeedsAttention)
	case "applied", "unchanged":
		if runErr != nil {
			return contract.SystemResourceActionResult{}, fmt.Errorf("%w: script exited after a success receipt: %v", ErrNeedsAttention, runErr)
		}
	default:
		return contract.SystemResourceActionResult{}, fmt.Errorf("%w: unsupported script receipt status", ErrNeedsAttention)
	}
	actualVersion, err := m.systemResourceVersion(ctx, resource, request)
	if err != nil {
		return contract.SystemResourceActionResult{}, fmt.Errorf("%w: post-write readback failed: %v", ErrNeedsAttention, err)
	}
	if actualVersion != receipt.Version {
		return contract.SystemResourceActionResult{}, fmt.Errorf("%w: post-write resource version does not match the script receipt", ErrNeedsAttention)
	}
	status := "succeeded"
	changed := receipt.Status == "applied"
	message := "system resource action applied by kejilion.sh"
	if !changed {
		status = "unchanged"
		message = "system resource already matched the requested state"
	}
	return contract.SystemResourceActionResult{
		Action: request.Action, Status: status, Changed: changed, Message: message,
		ResourceVersion: actualVersion, BackupPath: receipt.Backup,
		AppliedAt: m.now().UTC(),
	}, nil
}

func lockSystemResource(ctx context.Context, mutex interface {
	TryLock() bool
}) bool {
	if mutex.TryLock() {
		return true
	}
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return false
		case <-ticker.C:
			if mutex.TryLock() {
				return true
			}
		}
	}
}

func systemResourceInvocation(request contract.SystemResourceActionRequest) (string, string, []string, []byte) {
	switch request.Action {
	case "hosts-add":
		return "hosts", "add", []string{request.Address, strings.Join(request.Hostnames, ","), request.Comment}, nil
	case "hosts-delete":
		return "hosts", "delete", []string{strconv.Itoa(request.Line)}, nil
	case "cron-add":
		return "cron", "add", []string{request.Expression, "--command-stdin"}, []byte(request.Command + "\n")
	case "cron-update":
		return "cron", "update", []string{strconv.Itoa(request.Line), request.Expression, "--command-stdin"}, []byte(request.Command + "\n")
	case "cron-delete":
		return "cron", "delete", []string{strconv.Itoa(request.Line)}, nil
	case "network-interface-state":
		state := "down"
		if *request.Enabled {
			state = "up"
		}
		return "network-interface", "state", []string{request.InterfaceName, state}, nil
	case "firewall-open-port":
		return "firewall", "open-port", []string{strconv.Itoa(request.Port)}, nil
	case "firewall-close-port":
		return "firewall", "close-port", []string{strconv.Itoa(request.Port)}, nil
	case "firewall-allow-ip":
		return "firewall", "allow-ip", []string{request.Address}, nil
	case "firewall-block-ip":
		return "firewall", "block-ip", []string{request.Address}, nil
	case "firewall-remove-ip":
		return "firewall", "remove-ip", []string{request.Address}, nil
	case "firewall-allow-country":
		return "firewall", "allow-country", []string{request.CountryCode}, nil
	case "firewall-block-country":
		return "firewall", "block-country", []string{request.CountryCode}, nil
	case "firewall-remove-country":
		return "firewall", "remove-country", []string{request.CountryCode}, nil
	case "firewall-open-all":
		return "firewall", "open-all", nil, nil
	case "firewall-close-all":
		return "firewall", "close-all", nil, nil
	case "firewall-enable-ping":
		return "firewall", "enable-ping", nil, nil
	case "firewall-disable-ping":
		return "firewall", "disable-ping", nil, nil
	case "firewall-enable-ddos":
		return "firewall", "enable-ddos", nil, nil
	case "firewall-disable-ddos":
		return "firewall", "disable-ddos", nil, nil
	default:
		return "", "", nil, nil
	}
}

func (m *Manager) systemResourceVersion(
	ctx context.Context,
	resource string,
	request contract.SystemResourceActionRequest,
) (string, error) {
	switch resource {
	case "hosts":
		snapshot, err := m.Hosts(ctx)
		if err == nil && snapshot.Truncated {
			err = fmt.Errorf("%w: hosts resource exceeds the managed 1024-line bound", ErrUnsupported)
		}
		return snapshot.ResourceVersion, err
	case "cron":
		snapshot, err := m.Cron(ctx)
		if err == nil && snapshot.Truncated {
			err = fmt.Errorf("%w: crontab resource exceeds the managed 512-line bound", ErrUnsupported)
		}
		return snapshot.ResourceVersion, err
	case "network-interface":
		snapshot, err := m.NetworkInterfaces(ctx)
		if err != nil {
			return "", err
		}
		for _, entry := range snapshot.Entries {
			if entry.Name == request.InterfaceName {
				return entry.ResourceVersion, nil
			}
		}
		return "", fmt.Errorf("%w: network interface does not exist in the bounded snapshot", ErrInvalidInput)
	case "firewall":
		snapshot, err := m.Firewall(ctx)
		return snapshot.ResourceVersion, err
	default:
		return "", fmt.Errorf("%w: unknown system resource", ErrInvalidInput)
	}
}

func (m *Manager) validateResourceTarget(
	ctx context.Context,
	resource string,
	request contract.SystemResourceActionRequest,
) error {
	switch request.Action {
	case "hosts-delete":
		snapshot, err := m.Hosts(ctx)
		if err != nil {
			return err
		}
		for _, entry := range snapshot.Entries {
			if entry.Line == request.Line {
				return nil
			}
		}
		return fmt.Errorf("%w: hosts line does not identify a visible entry", ErrInvalidInput)
	case "cron-update", "cron-delete":
		snapshot, err := m.Cron(ctx)
		if err != nil {
			return err
		}
		if request.Line > len(snapshot.Entries) {
			return fmt.Errorf("%w: crontab line does not exist", ErrInvalidInput)
		}
	case "network-interface-state":
		_, err := m.systemResourceVersion(ctx, resource, request)
		return err
	}
	return nil
}

type systemResourceReceipt struct {
	Status  string
	Version string
	Backup  string
}

func parseSystemResourceReceipt(output []byte) (systemResourceReceipt, error) {
	receipt := systemResourceReceipt{}
	seen := make(map[string]bool)
	for _, line := range resourceLines(output) {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		matched := false
		for key, target := range map[string]*string{
			"KPANEL_SYSTEM_RESOURCE_STATUS":  &receipt.Status,
			"KPANEL_SYSTEM_RESOURCE_VERSION": &receipt.Version,
			"KPANEL_SYSTEM_RESOURCE_BACKUP":  &receipt.Backup,
		} {
			value, ok := receiptValue(line, key)
			if !ok {
				continue
			}
			matched = true
			if seen[key] {
				return systemResourceReceipt{}, fmt.Errorf("duplicate %s", key)
			}
			seen[key] = true
			*target = value
		}
		if !matched {
			return systemResourceReceipt{}, errors.New("unexpected non-empty output")
		}
	}
	if receipt.Status == "" {
		return systemResourceReceipt{}, errors.New("status is missing")
	}
	if !resourceVersionPattern.MatchString(receipt.Version) {
		return systemResourceReceipt{}, errors.New("version is missing or malformed")
	}
	if receipt.Backup != "" {
		if len(receipt.Backup) > 4096 || !filepath.IsAbs(receipt.Backup) ||
			filepath.Clean(receipt.Backup) != receipt.Backup || strings.ContainsAny(receipt.Backup, "\x00\r\n") ||
			receipt.Status != "rollback-failed" || !resourceRecoveryPattern.MatchString(receipt.Backup) {
			return systemResourceReceipt{}, errors.New("backup path is malformed")
		}
	}
	return receipt, nil
}

func receiptValue(line, key string) (string, bool) {
	for _, separator := range []string{" ", "="} {
		prefix := key + separator
		if strings.HasPrefix(line, prefix) {
			value := strings.TrimSpace(strings.TrimPrefix(line, prefix))
			return value, value != ""
		}
	}
	return "", false
}

func (m *Manager) systemResourceScriptPath() (string, error) {
	path, err := m.resourceScript()
	if err != nil {
		return "", fmt.Errorf("%w: trusted kejilion.sh system-resource protocol was not found", ErrUnsupported)
	}
	path = filepath.Clean(path)
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("%w: system-resource script path must be absolute", ErrUnsupported)
	}
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() ||
		info.Size() < 1024 || info.Size() > resourceScriptMaxBytes ||
		info.Mode().Perm()&0o022 != 0 || !dnsScriptOwnerTrusted(info) {
		return "", fmt.Errorf("%w: system-resource script is not a trusted root-owned regular file", ErrUnsupported)
	}
	content, err := readResourceFile(path, resourceScriptMaxBytes)
	if err != nil || !trustedKejilionSystemResourceContent(content) {
		return "", fmt.Errorf("%w: kejilion.sh system-resource protocol marker is missing", ErrUnsupported)
	}
	return path, nil
}

func findKejilionSystemResourceScript() (string, error) {
	candidates := []string{
		"/home/docker/kpanel/bin/kejilion.sh",
		"/usr/local/bin/k",
		"/usr/bin/k",
		"/root/kejilion.sh",
	}
	if path, err := exec.LookPath("k"); err == nil {
		candidates = append(candidates, path)
	}
	seen := make(map[string]bool)
	for _, candidate := range candidates {
		candidate = filepath.Clean(candidate)
		if seen[candidate] {
			continue
		}
		seen[candidate] = true
		info, err := os.Lstat(candidate)
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() ||
			info.Size() < 1024 || info.Size() > resourceScriptMaxBytes ||
			info.Mode().Perm()&0o022 != 0 || !dnsScriptOwnerTrusted(info) {
			continue
		}
		content, err := readResourceFile(candidate, resourceScriptMaxBytes)
		if err == nil && trustedKejilionSystemResourceContent(content) {
			return candidate, nil
		}
	}
	return "", errors.New("a trusted kejilion.sh system-resource command was not found")
}

func trustedKejilionSystemResourceContent(content []byte) bool {
	value := string(content)
	return dnsScriptLicense.Match(content) &&
		resourceProtocolV4Pattern.Match(content) &&
		strings.Contains(value, "KJ_SYSTEM_RESOURCE_NONINTERACTIVE") &&
		strings.Contains(value, "kpanel_system_resource_dispatch") &&
		strings.Contains(value, "KPANEL_SYSTEM_RESOURCE_STATUS") &&
		strings.Contains(value, "KPANEL_SYSTEM_RESOURCE_VERSION")
}

func readResourceFile(path string, limit int) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, errors.New("resource is not a regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, int64(limit)+1))
	if err != nil {
		return nil, err
	}
	if len(content) > limit {
		return nil, errResourceOutputTooLarge
	}
	return content, nil
}

func readResourceValue(path string, limit int) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, int64(limit)+1))
	if err != nil {
		return "", err
	}
	if len(content) > limit {
		return "", errResourceOutputTooLarge
	}
	value := strings.TrimSpace(string(content))
	if value == "" || strings.ContainsAny(value, "\x00\r\n") {
		return "", errors.New("resource value is empty or malformed")
	}
	return value, nil
}

func resourceLines(raw []byte) []string {
	if len(raw) == 0 {
		return []string{}
	}
	lines := strings.Split(string(raw), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	for index := range lines {
		lines[index] = strings.TrimSuffix(lines[index], "\r")
	}
	return lines
}

func resourceHash(raw []byte) string {
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}
