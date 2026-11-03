//go:build linux

package dockerx

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type firewallRule struct {
	arguments []string
}

type firewallStackChange struct {
	binary      string
	saveCommand string
	path        string
	rules       []firewallRule
	changed     []firewallRule
	output      []byte
}

func (c *Client) updateContainerAccess(
	ctx context.Context,
	id string,
	expectedVersion string,
	allowExternal bool,
	allowedIP string,
) error {
	inspect, err := c.inspectVerifiedContainer(ctx, id, expectedVersion)
	if err != nil {
		return err
	}
	if !inspect.State.Running || inspect.State.Paused || inspect.State.Restarting {
		return ErrActionUnsupported
	}
	ipv4Addresses, ipv6Addresses, err := containerIPAddresses(inspect)
	if err != nil {
		return err
	}
	allowedIP = strings.TrimSpace(allowedIP)
	if allowedIP != "" {
		parsed := net.ParseIP(allowedIP)
		if parsed == nil || parsed.To4() == nil {
			return ErrInvalidDockerJob
		}
		allowedIP = parsed.String()
	}
	stacks := make([]firewallStackChange, 0, 2)
	if len(ipv4Addresses) > 0 {
		var rules []firewallRule
		for _, address := range ipv4Addresses {
			rules = append(rules, containerAccessRules(address, allowedIP)...)
		}
		stacks = append(stacks, firewallStackChange{binary: "iptables", saveCommand: "iptables-save", path: c.iptablesRulesPath, rules: rules})
	}
	if len(ipv6Addresses) > 0 {
		var rules []firewallRule
		for _, address := range ipv6Addresses {
			rules = append(rules, containerAccessRulesForFamily(address, "", true)...)
		}
		path := c.ip6tablesRulesPath
		if path == "" {
			path = filepath.Join(filepath.Dir(c.iptablesRulesPath), "rules.v6")
		}
		stacks = append(stacks, firewallStackChange{binary: "ip6tables", saveCommand: "ip6tables-save", path: path, rules: rules})
	}
	run := c.hostCommand
	if run == nil {
		run = runFixedDockerHostCommand
	}
	rollback := func() {
		for index := len(stacks) - 1; index >= 0; index-- {
			rollbackFirewallAccessChange(run, stacks[index].binary, allowExternal, stacks[index].changed)
		}
	}
	for index := range stacks {
		stack := &stacks[index]
		if _, err := run(ctx, stack.binary, "-w", "5", "-S", "DOCKER-USER"); err != nil {
			rollback()
			return fmt.Errorf("Docker %s DOCKER-USER firewall chain is unavailable: %w", stack.binary, err)
		}
		if allowExternal {
			stack.changed, err = removeFirewallRules(ctx, run, stack.binary, stack.rules)
		} else {
			stack.changed, err = addFirewallRules(ctx, run, stack.binary, stack.rules)
		}
		if err != nil {
			rollback()
			return err
		}
	}
	files := make([]firewallRulesFile, 0, len(stacks))
	for index := range stacks {
		stack := &stacks[index]
		stack.output, err = run(ctx, stack.saveCommand)
		if err != nil {
			rollback()
			return fmt.Errorf("save Docker %s firewall rules: %w", stack.binary, err)
		}
		files = append(files, firewallRulesFile{path: stack.path, data: stack.output})
	}
	if err := atomicWriteFirewallRulesSet(files...); err != nil {
		rollback()
		return fmt.Errorf("persist Docker firewall rules: %w", err)
	}
	return nil
}

func containerIPAddresses(inspect containerInspect) ([]string, []string, error) {
	var ipv4Addresses, ipv6Addresses []string
	for _, network := range inspect.NetworkSettings.Networks {
		if parsed := net.ParseIP(strings.TrimSpace(network.IPAddress)); parsed != nil && parsed.To4() != nil {
			ipv4Addresses = append(ipv4Addresses, parsed.To4().String())
		}
		if parsed := net.ParseIP(strings.TrimSpace(network.GlobalIPv6Address)); parsed != nil && parsed.To4() == nil {
			ipv6Addresses = append(ipv6Addresses, parsed.String())
		}
	}
	sort.Strings(ipv4Addresses)
	ipv4Addresses = compactStrings(ipv4Addresses)
	sort.Strings(ipv6Addresses)
	ipv6Addresses = compactStrings(ipv6Addresses)
	if len(ipv4Addresses) == 0 && len(ipv6Addresses) == 0 {
		return nil, nil, errors.New("container does not expose a Docker IP address for access control")
	}
	return ipv4Addresses, ipv6Addresses, nil
}

func compactStrings(values []string) []string {
	if len(values) < 2 {
		return values
	}
	result := values[:1]
	for _, value := range values[1:] {
		if value != result[len(result)-1] {
			result = append(result, value)
		}
	}
	return result
}

func containerAccessRules(containerIP, allowedIP string) []firewallRule {
	return containerAccessRulesForFamily(containerIP, allowedIP, false)
}

func containerAccessRulesForFamily(containerIP, allowedIP string, ipv6 bool) []firewallRule {
	loopback := "127.0.0.0/8"
	if ipv6 {
		loopback = "::1/128"
	}
	rules := []firewallRule{
		{arguments: []string{"-p", "tcp", "-d", containerIP, "-j", "DROP"}},
	}
	if allowedIP != "" {
		rules = append(rules, firewallRule{
			arguments: []string{"-p", "tcp", "-s", allowedIP, "-d", containerIP, "-j", "ACCEPT"},
		})
	}
	rules = append(rules,
		firewallRule{arguments: []string{"-p", "tcp", "-s", loopback, "-d", containerIP, "-j", "ACCEPT"}},
		firewallRule{arguments: []string{"-p", "udp", "-d", containerIP, "-j", "DROP"}},
	)
	if allowedIP != "" {
		rules = append(rules, firewallRule{
			arguments: []string{"-p", "udp", "-s", allowedIP, "-d", containerIP, "-j", "ACCEPT"},
		})
	}
	return append(rules,
		firewallRule{arguments: []string{"-p", "udp", "-s", loopback, "-d", containerIP, "-j", "ACCEPT"}},
		firewallRule{arguments: []string{"-m", "state", "--state", "ESTABLISHED,RELATED", "-d", containerIP, "-j", "ACCEPT"}},
	)
}

func addFirewallRules(
	ctx context.Context,
	run func(context.Context, string, ...string) ([]byte, error),
	binary string,
	rules []firewallRule,
) ([]firewallRule, error) {
	var added []firewallRule
	for _, rule := range rules {
		if _, err := run(ctx, binary, append([]string{"-w", "5", "-C", "DOCKER-USER"}, rule.arguments...)...); err == nil {
			continue
		}
		if _, err := run(ctx, binary, append([]string{"-w", "5", "-I", "DOCKER-USER"}, rule.arguments...)...); err != nil {
			for index := len(added) - 1; index >= 0; index-- {
				_, _ = run(
					context.Background(),
					binary,
					append([]string{"-w", "5", "-D", "DOCKER-USER"}, added[index].arguments...)...,
				)
			}
			return nil, fmt.Errorf("add Docker firewall rule: %w", err)
		}
		added = append(added, rule)
	}
	return added, nil
}

func removeFirewallRules(
	ctx context.Context,
	run func(context.Context, string, ...string) ([]byte, error),
	binary string,
	rules []firewallRule,
) ([]firewallRule, error) {
	var removed []firewallRule
	for _, rule := range rules {
		if _, err := run(ctx, binary, append([]string{"-w", "5", "-C", "DOCKER-USER"}, rule.arguments...)...); err != nil {
			continue
		}
		if _, err := run(ctx, binary, append([]string{"-w", "5", "-D", "DOCKER-USER"}, rule.arguments...)...); err != nil {
			for index := len(removed) - 1; index >= 0; index-- {
				_, _ = run(
					context.Background(),
					binary,
					append([]string{"-w", "5", "-I", "DOCKER-USER"}, removed[index].arguments...)...,
				)
			}
			return nil, fmt.Errorf("remove Docker firewall rule: %w", err)
		}
		removed = append(removed, rule)
	}
	return removed, nil
}

func rollbackFirewallAccessChange(
	run func(context.Context, string, ...string) ([]byte, error),
	binary string,
	allowedExternal bool,
	rules []firewallRule,
) {
	for index := len(rules) - 1; index >= 0; index-- {
		action := "-D"
		if allowedExternal {
			action = "-I"
		}
		_, _ = run(
			context.Background(),
			binary,
			append([]string{"-w", "5", action, "DOCKER-USER"}, rules[index].arguments...)...,
		)
	}
}

func runFixedDockerHostCommand(
	ctx context.Context,
	name string,
	arguments ...string,
) ([]byte, error) {
	candidates := map[string][]string{
		"iptables":       {"/usr/sbin/iptables", "/sbin/iptables", "/usr/bin/iptables", "/bin/iptables"},
		"iptables-save":  {"/usr/sbin/iptables-save", "/sbin/iptables-save", "/usr/bin/iptables-save", "/bin/iptables-save"},
		"ip6tables":      {"/usr/sbin/ip6tables", "/sbin/ip6tables", "/usr/bin/ip6tables", "/bin/ip6tables"},
		"ip6tables-save": {"/usr/sbin/ip6tables-save", "/sbin/ip6tables-save", "/usr/bin/ip6tables-save", "/bin/ip6tables-save"},
		"scp":            {"/usr/bin/scp", "/bin/scp"},
	}
	paths, ok := candidates[name]
	if !ok {
		return nil, errors.New("unsupported fixed Docker host command")
	}
	for _, path := range paths {
		resolved, err := trustedDockerHostExecutable(path)
		if err != nil {
			continue
		}
		command := exec.CommandContext(ctx, resolved, arguments...)
		output, runErr := command.CombinedOutput()
		if len(output) > 1<<20 {
			output = output[:1<<20]
		}
		if runErr != nil {
			return output, fmt.Errorf("%s failed: %w", name, runErr)
		}
		return output, nil
	}
	return nil, fmt.Errorf("%s executable is unavailable", name)
}

func trustedDockerHostExecutable(path string) (string, error) {
	linkInfo, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	linkUID, _, err := fileNumericOwnership(linkInfo)
	if err != nil || linkUID != 0 {
		return "", errors.New("executable link is not owned by root")
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	resolved = filepath.Clean(resolved)
	trusted := false
	for _, root := range []string{"/usr/sbin", "/usr/bin", "/sbin", "/bin"} {
		if pathWithin(resolved, root) && resolved != root {
			trusted = true
			break
		}
	}
	if !trusted {
		return "", errors.New("executable resolved outside trusted system directories")
	}
	info, err := os.Lstat(resolved)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o022 != 0 {
		return "", errors.New("executable is unavailable or writable by an untrusted user")
	}
	uid, _, err := fileNumericOwnership(info)
	if err != nil || uid != 0 {
		return "", errors.New("executable is not owned by root")
	}
	parentInfo, err := os.Stat(filepath.Dir(resolved))
	if err != nil || !parentInfo.IsDir() || parentInfo.Mode().Perm()&0o022 != 0 {
		return "", errors.New("executable directory is not trusted")
	}
	parentUID, _, err := fileNumericOwnership(parentInfo)
	if err != nil || parentUID != 0 {
		return "", errors.New("executable directory is not owned by root")
	}
	// Execute through the validated fixed entrypoint so multi-call binaries
	// such as Debian's xtables-nft-multi still receive "iptables" as argv[0].
	return path, nil
}

type firewallRulesFile struct {
	path string
	data []byte
}

type firewallRulesSnapshot struct {
	file   firewallRulesFile
	data   []byte
	exists bool
}

func atomicWriteFirewallRules(path string, data []byte) error {
	return atomicWriteFirewallRulesSet(firewallRulesFile{path: path, data: data})
}

func atomicWriteFirewallRulesSet(files ...firewallRulesFile) error {
	if len(files) == 0 {
		return nil
	}
	snapshots := make([]firewallRulesSnapshot, 0, len(files))
	tempPaths := make([]string, 0, len(files))
	committed := 0
	defer func() {
		for _, path := range tempPaths {
			_ = os.Remove(path)
		}
	}()
	seen := make(map[string]struct{}, len(files))
	for _, file := range files {
		path := filepath.Clean(file.path)
		parent := filepath.Dir(path)
		if !filepath.IsAbs(path) || path == string(filepath.Separator) ||
			!filepath.IsAbs(parent) || parent == string(filepath.Separator) {
			return errors.New("iptables persistence path is unsafe")
		}
		if _, exists := seen[path]; exists {
			return errors.New("iptables persistence paths are duplicated")
		}
		seen[path] = struct{}{}
		if err := os.MkdirAll(parent, 0o700); err != nil {
			return err
		}
		snapshot := firewallRulesSnapshot{file: firewallRulesFile{path: path, data: file.data}}
		info, err := os.Lstat(path)
		if err == nil {
			if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > 1<<20 {
				return errors.New("iptables persistence target is not a bounded regular file")
			}
			snapshot.data, err = os.ReadFile(path)
			if err != nil {
				return err
			}
			snapshot.exists = true
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		snapshots = append(snapshots, snapshot)
		temp, err := os.CreateTemp(parent, "."+filepath.Base(path)+".kpanel-*.tmp")
		if err != nil {
			return err
		}
		tempPath := temp.Name()
		tempPaths = append(tempPaths, tempPath)
		if err := temp.Chmod(0o600); err != nil {
			_ = temp.Close()
			return err
		}
		if _, err := temp.Write(file.data); err != nil {
			_ = temp.Close()
			return err
		}
		if err := temp.Sync(); err != nil {
			_ = temp.Close()
			return err
		}
		if err := temp.Close(); err != nil {
			return err
		}
	}
	for index := range files {
		if err := os.Rename(tempPaths[index], snapshots[index].file.path); err != nil {
			return errors.Join(err, rollbackFirewallRulesFiles(snapshots[:committed]))
		}
		committed++
	}
	directories := make(map[string]struct{}, len(files))
	for _, file := range files {
		directories[filepath.Dir(filepath.Clean(file.path))] = struct{}{}
	}
	for directory := range directories {
		if err := syncDirectoryPath(directory); err != nil {
			return errors.Join(err, rollbackFirewallRulesFiles(snapshots[:committed]))
		}
	}
	return nil
}

func rollbackFirewallRulesFiles(snapshots []firewallRulesSnapshot) error {
	var rollbackErr error
	for index := len(snapshots) - 1; index >= 0; index-- {
		snapshot := snapshots[index]
		if snapshot.exists {
			rollbackErr = errors.Join(rollbackErr, writeFirewallRulesFileAtomic(snapshot.file.path, snapshot.data))
			continue
		}
		if err := os.Remove(snapshot.file.path); err != nil && !errors.Is(err, os.ErrNotExist) {
			rollbackErr = errors.Join(rollbackErr, err)
		} else {
			rollbackErr = errors.Join(rollbackErr, syncDirectoryPath(filepath.Dir(snapshot.file.path)))
		}
	}
	return rollbackErr
}

func writeFirewallRulesFileAtomic(path string, data []byte) error {
	path = filepath.Clean(path)
	parent := filepath.Dir(path)
	if !filepath.IsAbs(path) || path == string(filepath.Separator) ||
		!filepath.IsAbs(parent) || parent == string(filepath.Separator) {
		return errors.New("iptables persistence path is unsafe")
	}
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return err
	}
	temp, err := os.CreateTemp(parent, "."+filepath.Base(path)+".kpanel-*.tmp")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		return err
	}
	return syncDirectoryPath(parent)
}
