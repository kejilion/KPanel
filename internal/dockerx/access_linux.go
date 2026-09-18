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
	containerIPs, err := containerIPv4s(inspect)
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
	var rules []firewallRule
	for _, containerIP := range containerIPs {
		rules = append(rules, containerAccessRules(containerIP, allowedIP)...)
	}
	run := c.hostCommand
	if run == nil {
		run = runFixedDockerHostCommand
	}
	if _, err := run(ctx, "iptables", "-w", "5", "-S", "DOCKER-USER"); err != nil {
		return fmt.Errorf("Docker DOCKER-USER firewall chain is unavailable: %w", err)
	}
	var changed []firewallRule
	if allowExternal {
		changed, err = removeFirewallRules(ctx, run, rules)
	} else {
		changed, err = addFirewallRules(ctx, run, rules)
	}
	if err != nil {
		return err
	}
	output, err := run(ctx, "iptables-save")
	if err != nil {
		rollbackFirewallAccessChange(run, allowExternal, changed)
		return fmt.Errorf("save Docker firewall rules: %w", err)
	}
	if err := atomicWriteFirewallRules(c.iptablesRulesPath, output); err != nil {
		rollbackFirewallAccessChange(run, allowExternal, changed)
		return fmt.Errorf("persist Docker firewall rules: %w", err)
	}
	return nil
}

func containerIPv4s(inspect containerInspect) ([]string, error) {
	var addresses []string
	for _, network := range inspect.NetworkSettings.Networks {
		value := strings.TrimSpace(network.IPAddress)
		parsed := net.ParseIP(value)
		if parsed == nil || parsed.To4() == nil {
			continue
		}
		addresses = append(addresses, parsed.String())
	}
	sort.Strings(addresses)
	addresses = compactStrings(addresses)
	if len(addresses) == 0 {
		return nil, errors.New("container does not expose a Docker IPv4 address for access control")
	}
	return addresses, nil
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
	rules := []firewallRule{
		{arguments: []string{"-p", "tcp", "-d", containerIP, "-j", "DROP"}},
	}
	if allowedIP != "" {
		rules = append(rules, firewallRule{
			arguments: []string{"-p", "tcp", "-s", allowedIP, "-d", containerIP, "-j", "ACCEPT"},
		})
	}
	rules = append(rules,
		firewallRule{arguments: []string{"-p", "tcp", "-s", "127.0.0.0/8", "-d", containerIP, "-j", "ACCEPT"}},
		firewallRule{arguments: []string{"-p", "udp", "-d", containerIP, "-j", "DROP"}},
	)
	if allowedIP != "" {
		rules = append(rules, firewallRule{
			arguments: []string{"-p", "udp", "-s", allowedIP, "-d", containerIP, "-j", "ACCEPT"},
		})
	}
	return append(rules,
		firewallRule{arguments: []string{"-p", "udp", "-s", "127.0.0.0/8", "-d", containerIP, "-j", "ACCEPT"}},
		firewallRule{arguments: []string{"-m", "state", "--state", "ESTABLISHED,RELATED", "-d", containerIP, "-j", "ACCEPT"}},
	)
}

func addFirewallRules(
	ctx context.Context,
	run func(context.Context, string, ...string) ([]byte, error),
	rules []firewallRule,
) ([]firewallRule, error) {
	var added []firewallRule
	for _, rule := range rules {
		if _, err := run(ctx, "iptables", append([]string{"-w", "5", "-C", "DOCKER-USER"}, rule.arguments...)...); err == nil {
			continue
		}
		if _, err := run(ctx, "iptables", append([]string{"-w", "5", "-I", "DOCKER-USER"}, rule.arguments...)...); err != nil {
			for index := len(added) - 1; index >= 0; index-- {
				_, _ = run(
					context.Background(),
					"iptables",
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
	rules []firewallRule,
) ([]firewallRule, error) {
	var removed []firewallRule
	for _, rule := range rules {
		if _, err := run(ctx, "iptables", append([]string{"-w", "5", "-C", "DOCKER-USER"}, rule.arguments...)...); err != nil {
			continue
		}
		if _, err := run(ctx, "iptables", append([]string{"-w", "5", "-D", "DOCKER-USER"}, rule.arguments...)...); err != nil {
			for index := len(removed) - 1; index >= 0; index-- {
				_, _ = run(
					context.Background(),
					"iptables",
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
			"iptables",
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
		"iptables":      {"/usr/sbin/iptables", "/sbin/iptables", "/usr/bin/iptables", "/bin/iptables"},
		"iptables-save": {"/usr/sbin/iptables-save", "/sbin/iptables-save", "/usr/bin/iptables-save", "/bin/iptables-save"},
		"scp":           {"/usr/bin/scp", "/bin/scp"},
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

func atomicWriteFirewallRules(path string, data []byte) error {
	path = filepath.Clean(path)
	parent := filepath.Dir(path)
	if !filepath.IsAbs(path) || path == string(filepath.Separator) ||
		!filepath.IsAbs(parent) || parent == string(filepath.Separator) {
		return errors.New("iptables persistence path is unsafe")
	}
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return err
	}
	temp, err := os.CreateTemp(parent, ".rules.v4.kpanel-*.tmp")
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
