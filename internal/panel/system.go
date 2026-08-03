package panel

import (
	"encoding/json"
	"net"
	"net/http"
	"reflect"
	"regexp"
	"strings"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

var panelHostnamePattern = regexp.MustCompile(
	`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)*$`,
)
var panelCronFieldPattern = regexp.MustCompile(`^[0-9*/?,\-]+$`)
var panelInterfacePattern = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,64}$`)

func (s *Server) handleSystemAction(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawPath != "" || r.URL.RawQuery != "" {
		s.writeProblem(w, r, http.StatusNotFound, "route_not_found", "Route not found", "")
		return
	}
	if !s.checkOrigin(w, r) {
		return
	}
	_, session, ok := s.requireSession(w, r)
	if !ok || !s.checkCSRF(w, r, session) {
		return
	}
	var input contract.SystemActionRequest
	if err := s.decodeJSON(w, r, &input); err != nil {
		return
	}
	if field, detail := validateSystemAction(&input); field != "" {
		s.writeValidationProblem(w, r, field, detail)
		return
	}
	body, err := json.Marshal(input)
	if err != nil {
		s.writeProblem(w, r, http.StatusInternalServerError, "request_encoding_failed", "Request encoding failed", "")
		return
	}
	action := "system." + input.Action
	change := systemActionAuditChange(input)
	if err := s.audit(r, session.User.ID, action, "system", input.Action, "intent", change); err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "audit_unavailable", "Audit storage unavailable", "")
		return
	}
	response, err := s.agent.Do(r.Context(), http.MethodPost, "/v1/system/actions", "", requestID(r), body)
	if err != nil {
		_ = s.audit(r, session.User.ID, action, "system", input.Action, "failure", change)
		s.writeProblem(w, r, http.StatusServiceUnavailable, "agent_unavailable", "Agent unavailable", "")
		return
	}
	result := "failure"
	if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
		result = "success"
	}
	_ = s.audit(r, session.User.ID, action, "system", input.Action, result, change)
	s.writeAgentResponse(w, r, response)
}

func validateSystemAction(input *contract.SystemActionRequest) (string, string) {
	if input.Action == "" {
		return "action", "action is required"
	}
	switch input.Action {
	case "hostname":
		input.Hostname = strings.ToLower(strings.TrimSpace(input.Hostname))
		if len(input.Hostname) > 253 || !panelHostnamePattern.MatchString(input.Hostname) {
			return "hostname", "hostname is invalid"
		}
	case "ssh-port":
		if input.Port == 0 {
			return "port", "port must be between 1 and 65535"
		}
	case "ssh-defense":
		if input.Enabled == nil {
			return "enabled", "enabled is required"
		}
	case "dns":
		if len(input.Servers) < 1 {
			return "servers", "one to four DNS servers are required"
		}
	case "hosts":
		if input.HostsOperation != "add" && input.HostsOperation != "delete" {
			return "hostsOperation", "hostsOperation must be add or delete"
		}
		input.HostsEntry = strings.Join(strings.Fields(input.HostsEntry), " ")
		fields := strings.Fields(input.HostsEntry)
		if len(fields) < 2 || len(fields) > 16 || net.ParseIP(fields[0]) == nil {
			return "hostsEntry", "hostsEntry must contain an IP address and one to fifteen hostnames"
		}
		for _, hostname := range fields[1:] {
			if len(hostname) > 253 || !panelHostnamePattern.MatchString(strings.ToLower(hostname)) {
				return "hostsEntry", "hostsEntry contains an invalid hostname"
			}
		}
	case "cron":
		if input.CronOperation != "add" && input.CronOperation != "delete" {
			return "cronOperation", "cronOperation must be add or delete"
		}
		input.CronEntry = strings.TrimSpace(input.CronEntry)
		if input.CronEntry == "" || len(input.CronEntry) > 4096 || strings.ContainsAny(input.CronEntry, "\x00\r\n") {
			return "cronEntry", "cronEntry is empty, too long, or contains a line break"
		}
		if input.CronOperation == "add" {
			fields := strings.Fields(input.CronEntry)
			if len(fields) < 6 {
				return "cronEntry", "cronEntry must contain five schedule fields and a command"
			}
			for _, field := range fields[:5] {
				if !panelCronFieldPattern.MatchString(field) {
					return "cronEntry", "cron schedule fields contain unsupported characters"
				}
			}
		}
	case "network-interface":
		if input.NetworkOperation != "up" && input.NetworkOperation != "down" {
			return "networkOperation", "networkOperation must be up or down"
		}
		if !panelInterfacePattern.MatchString(input.InterfaceName) {
			return "interfaceName", "interfaceName is invalid"
		}
	case "firewall":
		allowed := map[string]bool{"port-open": true, "port-close": true, "all-open": true, "all-close": true, "ip-allow": true, "ip-block": true, "ip-remove": true, "ping-allow": true, "ping-block": true, "ddos-enable": true, "ddos-disable": true, "country-block": true, "country-allow": true, "country-unblock": true}
		if !allowed[input.FirewallOperation] {
			return "firewallOperation", "firewallOperation is unsupported"
		}
		if strings.HasPrefix(input.FirewallOperation, "port-") && input.FirewallPort == 0 {
			return "firewallPort", "firewallPort is required"
		}
		if strings.HasPrefix(input.FirewallOperation, "ip-") {
			input.FirewallAddress = strings.TrimSpace(input.FirewallAddress)
			if _, _, err := net.ParseCIDR(input.FirewallAddress); err != nil && net.ParseIP(input.FirewallAddress) == nil {
				return "firewallAddress", "firewallAddress must be an IP address or CIDR"
			}
		}
		if strings.HasPrefix(input.FirewallOperation, "country-") {
			if len(input.CountryCodes) < 1 || len(input.CountryCodes) > 20 {
				return "countryCodes", "one to twenty country codes are required"
			}
			for index, code := range input.CountryCodes {
				code = strings.ToUpper(strings.TrimSpace(code))
				if len(code) != 2 || code[0] < 'A' || code[0] > 'Z' || code[1] < 'A' || code[1] > 'Z' {
					return "countryCodes", "every country code must contain two letters"
				}
				input.CountryCodes[index] = code
			}
		}
		for index, server := range input.Servers {
			server = strings.TrimSpace(server)
			if net.ParseIP(server) == nil {
				return "servers", "every DNS server must be an IP address"
			}
			input.Servers[index] = server
		}
	case "timezone":
		input.Timezone = strings.TrimSpace(input.Timezone)
		if input.Timezone == "" || len(input.Timezone) > 128 ||
			strings.Contains(input.Timezone, "..") || strings.ContainsAny(input.Timezone, "\x00\r\n") {
			return "timezone", "timezone is invalid"
		}
	case "swap":
		if input.SwapSizeMiB < 0 {
			return "swapSizeMiB", "swapSizeMiB must be zero or a positive integer"
		}
	case "mirror":
		switch input.MirrorPreset {
		case "cn-default", "cn-edu", "abroad", "smart":
		default:
			return "mirrorPreset", "mirrorPreset must be cn-default, cn-edu, abroad, or smart"
		}
	case "ip-preference":
		if input.Preference != "ipv4" && input.Preference != "system_default" {
			return "preference", "preference must be ipv4 or system_default"
		}
	case "kernel-tuning":
		switch input.Profile {
		case "high", "balanced", "web", "stream", "game", "off":
		default:
			return "profile", "profile must be high, balanced, web, stream, game, or off"
		}
	case "bbr":
		if input.Enabled == nil {
			return "enabled", "enabled is required"
		}
	case "bbrv3":
		if input.MaintenancePolicy != "install" && input.MaintenancePolicy != "update" &&
			input.MaintenancePolicy != "uninstall" {
			return "maintenancePolicy", "maintenancePolicy must be install, update, or uninstall"
		}
		allowed := contract.SystemActionRequest{
			Action:            input.Action,
			MaintenancePolicy: input.MaintenancePolicy,
		}
		if !reflect.DeepEqual(*input, allowed) {
			return "request", "only action and maintenancePolicy are allowed for bbrv3"
		}
	case "update":
		if input.MaintenancePolicy != "full" {
			return "maintenancePolicy", "maintenancePolicy must be full"
		}
	case "cleanup":
		if input.MaintenancePolicy != "cache" && input.MaintenancePolicy != "standard" {
			return "maintenancePolicy", "maintenancePolicy must be cache or standard"
		}
	case "reboot":
		if input.Hostname != "" || input.Port != 0 || len(input.Servers) != 0 ||
			input.Timezone != "" || input.SwapSizeMiB != 0 || input.MirrorPreset != "" ||
			input.Preference != "" || input.Profile != "" || input.MaintenancePolicy != "" ||
			input.Enabled != nil {
			return "request", "only action is allowed for reboot"
		}
	default:
		return "action", "unsupported system action"
	}
	return "", ""
}

func systemActionAuditChange(input contract.SystemActionRequest) map[string]any {
	change := map[string]any{"action": input.Action}
	switch input.Action {
	case "hostname":
		change["hostname"] = input.Hostname
	case "ssh-port":
		change["port"] = input.Port
	case "ssh-defense":
		change["enabled"] = input.Enabled != nil && *input.Enabled
	case "dns":
		change["servers"] = input.Servers
	case "hosts":
		change["operation"], change["entry"] = input.HostsOperation, input.HostsEntry
	case "cron":
		change["operation"], change["entry"] = input.CronOperation, input.CronEntry
	case "network-interface":
		change["operation"], change["interface"] = input.NetworkOperation, input.InterfaceName
	case "firewall":
		change["operation"] = input.FirewallOperation
		if input.FirewallPort != 0 {
			change["port"] = input.FirewallPort
		}
		if input.FirewallAddress != "" {
			change["address"] = input.FirewallAddress
		}
		if len(input.CountryCodes) > 0 {
			change["countryCodes"] = input.CountryCodes
		}
	case "timezone":
		change["timezone"] = input.Timezone
	case "swap":
		change["swapSizeMiB"] = input.SwapSizeMiB
	case "mirror":
		change["mirrorPreset"] = input.MirrorPreset
	case "ip-preference":
		change["preference"] = input.Preference
	case "kernel-tuning":
		change["profile"] = input.Profile
	case "bbr":
		change["enabled"] = input.Enabled != nil && *input.Enabled
	case "update", "cleanup", "bbrv3":
		change["maintenancePolicy"] = input.MaintenancePolicy
	case "reboot":
		change["requested"] = true
	}
	return change
}
