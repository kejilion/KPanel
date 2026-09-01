package contract

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	SystemHostsMaxBytes         = 256 << 10
	SystemHostsMaxLines         = 1024
	SystemCronMaxBytes          = 256 << 10
	SystemCronMaxLines          = 512
	SystemNetworkMaxEntries     = 128
	SystemFirewallMaxBytes      = 512 << 10
	SystemFirewallMaxLines      = 512
	SystemFirewallIPSetMaxBytes = 8 << 20
	SystemFirewallIPSetMaxLines = 131072
	SystemResourceMaxCommand    = 2048
)

var (
	systemResourceVersionPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
	systemHostnameLabelPattern   = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?$`)
	systemInterfacePattern       = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,14}$`)
	systemCountryCodePattern     = regexp.MustCompile(`^[A-Z]{2}$`)
)

// SystemResourceActionRequest is the mutation contract for the independent
// hosts, crontab, network-interface and firewall resource domains. It is kept
// separate from the legacy SystemActionRequest union.
type SystemResourceActionRequest struct {
	Action                  string   `json:"action"`
	Address                 string   `json:"address,omitempty"`
	Hostnames               []string `json:"hostnames,omitempty"`
	Comment                 string   `json:"comment,omitempty"`
	Line                    int      `json:"line,omitempty"`
	Expression              string   `json:"expression,omitempty"`
	Command                 string   `json:"command,omitempty"`
	InterfaceName           string   `json:"interfaceName,omitempty"`
	Enabled                 *bool    `json:"enabled,omitempty"`
	Port                    int      `json:"port,omitempty"`
	CountryCode             string   `json:"countryCode,omitempty"`
	ExpectedResourceVersion string   `json:"expectedResourceVersion"`

	providedFields map[string]struct{}
}

type systemResourceActionRequestJSON struct {
	Action                  string   `json:"action"`
	Address                 string   `json:"address,omitempty"`
	Hostnames               []string `json:"hostnames,omitempty"`
	Comment                 string   `json:"comment,omitempty"`
	Line                    int      `json:"line,omitempty"`
	Expression              string   `json:"expression,omitempty"`
	Command                 string   `json:"command,omitempty"`
	InterfaceName           string   `json:"interfaceName,omitempty"`
	Enabled                 *bool    `json:"enabled,omitempty"`
	Port                    int      `json:"port,omitempty"`
	CountryCode             string   `json:"countryCode,omitempty"`
	ExpectedResourceVersion string   `json:"expectedResourceVersion"`
}

// UnmarshalJSON keeps the exact set of provided fields so semantic validation
// can reject action-inapplicable fields even when their JSON value is zero.
func (request *SystemResourceActionRequest) UnmarshalJSON(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var value systemResourceActionRequestJSON
	if err := decoder.Decode(&value); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	*request = SystemResourceActionRequest{
		Action: value.Action, Address: value.Address, Hostnames: value.Hostnames,
		Comment: value.Comment, Line: value.Line, Expression: value.Expression,
		Command: value.Command, InterfaceName: value.InterfaceName, Enabled: value.Enabled,
		Port: value.Port, CountryCode: value.CountryCode, ExpectedResourceVersion: value.ExpectedResourceVersion,
		providedFields: make(map[string]struct{}, len(fields)),
	}
	for field := range fields {
		request.providedFields[field] = struct{}{}
	}
	return nil
}

// ValidateSystemResourceAction normalizes safe scalar inputs and returns a
// stable field/detail pair suitable for either a Panel or Agent 422 response.
func ValidateSystemResourceAction(request *SystemResourceActionRequest) (string, string) {
	if request == nil {
		return "request", "request is required"
	}
	request.Action = strings.TrimSpace(request.Action)
	if request.Action == "" {
		return "action", "action is required"
	}
	allowed := map[string]bool{"action": true, "expectedResourceVersion": true}
	require := func(fields ...string) {
		for _, field := range fields {
			allowed[field] = true
		}
	}
	switch request.Action {
	case "hosts-add":
		require("address", "hostnames", "comment")
	case "hosts-delete", "cron-delete":
		require("line")
	case "cron-add":
		require("expression", "command")
	case "cron-update":
		require("line", "expression", "command")
	case "network-interface-state":
		require("interfaceName", "enabled")
	case "firewall-open-port", "firewall-close-port":
		require("port")
	case "firewall-allow-ip", "firewall-block-ip", "firewall-remove-ip":
		require("address")
	case "firewall-allow-country", "firewall-block-country", "firewall-remove-country":
		require("countryCode")
	case "firewall-open-all", "firewall-close-all", "firewall-enable-ping",
		"firewall-disable-ping", "firewall-enable-ddos", "firewall-disable-ddos":
	default:
		return "action", "unsupported system resource action"
	}
	for field := range request.actualProvidedFields() {
		if !allowed[field] {
			return field, "field is not allowed for this action"
		}
	}
	if !systemResourceVersionPattern.MatchString(request.ExpectedResourceVersion) {
		return "expectedResourceVersion", "expectedResourceVersion must be 64 lowercase hexadecimal characters"
	}

	switch request.Action {
	case "hosts-add":
		request.Address = strings.TrimSpace(request.Address)
		ip := net.ParseIP(request.Address)
		if ip == nil {
			return "address", "address must be an IPv4 or IPv6 address"
		}
		request.Address = ip.String()
		if len(request.Hostnames) == 0 || len(request.Hostnames) > 16 {
			return "hostnames", "one to 16 hostnames are required"
		}
		seen := make(map[string]bool, len(request.Hostnames))
		for index, hostname := range request.Hostnames {
			hostname = strings.TrimSpace(hostname)
			if !validSystemHostname(hostname) {
				return "hostnames", "every hostname must be a valid single-line hostname"
			}
			key := strings.ToLower(hostname)
			if seen[key] {
				return "hostnames", "hostnames must not contain duplicates"
			}
			seen[key] = true
			request.Hostnames[index] = hostname
		}
		if len(strings.Join(request.Hostnames, ",")) > 1024 {
			return "hostnames", "hostnames exceed the 1024-byte script protocol limit"
		}
		request.Comment = strings.TrimSpace(request.Comment)
		if len(request.Comment) > 256 || strings.ContainsAny(request.Comment, "\x00\r\n") {
			return "comment", "comment must be a single line no longer than 256 bytes"
		}
	case "hosts-delete":
		if request.Line < 1 || request.Line > SystemHostsMaxLines {
			return "line", "line must identify one of the bounded hosts lines"
		}
	case "cron-add", "cron-update":
		if request.Action == "cron-update" && (request.Line < 1 || request.Line > SystemCronMaxLines) {
			return "line", "line must identify one of the bounded crontab lines"
		}
		request.Expression = strings.TrimSpace(request.Expression)
		if field, detail := validateCronExpression(request.Expression); field != "" {
			return field, detail
		}
		if strings.TrimSpace(request.Command) == "" || len(request.Command) > SystemResourceMaxCommand ||
			strings.ContainsAny(request.Command, "\x00\r\n") {
			return "command", "command must be a non-empty single line no longer than 2048 bytes"
		}
	case "cron-delete":
		if request.Line < 1 || request.Line > SystemCronMaxLines {
			return "line", "line must identify one of the bounded crontab lines"
		}
	case "network-interface-state":
		request.InterfaceName = strings.TrimSpace(request.InterfaceName)
		if !systemInterfacePattern.MatchString(request.InterfaceName) {
			return "interfaceName", "interfaceName must be a valid Linux interface name"
		}
		if request.Enabled == nil {
			return "enabled", "enabled is required"
		}
	case "firewall-open-port", "firewall-close-port":
		if request.Port < 1 || request.Port > 65535 {
			return "port", "port must be between 1 and 65535"
		}
	case "firewall-allow-ip", "firewall-block-ip", "firewall-remove-ip":
		address, ok := normalizeIPv4OrCIDR(request.Address)
		if !ok {
			return "address", "address must be an IPv4 address or IPv4 CIDR"
		}
		request.Address = address
	case "firewall-allow-country", "firewall-block-country", "firewall-remove-country":
		request.CountryCode = strings.ToUpper(strings.TrimSpace(request.CountryCode))
		if !systemCountryCodePattern.MatchString(request.CountryCode) {
			return "countryCode", "countryCode must be two uppercase letters such as US"
		}
	}
	return "", ""
}

func (request *SystemResourceActionRequest) actualProvidedFields() map[string]struct{} {
	if request.providedFields != nil {
		return request.providedFields
	}
	fields := map[string]struct{}{"action": {}, "expectedResourceVersion": {}}
	if request.Address != "" {
		fields["address"] = struct{}{}
	}
	if request.Hostnames != nil {
		fields["hostnames"] = struct{}{}
	}
	if request.Comment != "" {
		fields["comment"] = struct{}{}
	}
	if request.Line != 0 {
		fields["line"] = struct{}{}
	}
	if request.Expression != "" {
		fields["expression"] = struct{}{}
	}
	if request.Command != "" {
		fields["command"] = struct{}{}
	}
	if request.InterfaceName != "" {
		fields["interfaceName"] = struct{}{}
	}
	if request.Enabled != nil {
		fields["enabled"] = struct{}{}
	}
	if request.Port != 0 {
		fields["port"] = struct{}{}
	}
	if request.CountryCode != "" {
		fields["countryCode"] = struct{}{}
	}
	return fields
}

func validSystemHostname(hostname string) bool {
	if len(hostname) == 0 || len(hostname) > 253 || strings.ContainsAny(hostname, "\x00\r\n") {
		return false
	}
	hostname = strings.TrimSuffix(hostname, ".")
	if hostname == "" {
		return false
	}
	for _, label := range strings.Split(hostname, ".") {
		if len(label) == 0 || len(label) > 63 || !systemHostnameLabelPattern.MatchString(label) {
			return false
		}
	}
	return true
}

type cronFieldSpec struct {
	name  string
	min   int
	max   int
	names map[string]int
}

func validateCronExpression(expression string) (string, string) {
	switch expression {
	case "@reboot", "@hourly", "@daily", "@weekly", "@monthly", "@yearly", "@annually", "@midnight":
		return "", ""
	}
	if len(expression) == 0 || len(expression) > 128 {
		return "expression", "expression must be a supported five-field or @ cron expression"
	}
	fields := strings.Fields(expression)
	if len(fields) != 5 || strings.Join(fields, " ") != expression {
		return "expression", "expression must contain exactly five single-spaced fields"
	}
	monthNames := map[string]int{
		"JAN": 1, "FEB": 2, "MAR": 3, "APR": 4, "MAY": 5, "JUN": 6,
		"JUL": 7, "AUG": 8, "SEP": 9, "OCT": 10, "NOV": 11, "DEC": 12,
	}
	dayNames := map[string]int{
		"SUN": 0, "MON": 1, "TUE": 2, "WED": 3, "THU": 4, "FRI": 5, "SAT": 6,
	}
	specs := []cronFieldSpec{
		{name: "minute", min: 0, max: 59},
		{name: "hour", min: 0, max: 23},
		{name: "dayOfMonth", min: 1, max: 31},
		{name: "month", min: 1, max: 12, names: monthNames},
		{name: "dayOfWeek", min: 0, max: 7, names: dayNames},
	}
	for index, field := range fields {
		if len(field) > 64 || !validCronField(field, specs[index]) {
			return "expression." + specs[index].name,
				fmt.Sprintf("%s contains an out-of-range value or invalid list/range/step", specs[index].name)
		}
	}
	return "", ""
}

func validCronField(field string, spec cronFieldSpec) bool {
	if field == "" {
		return false
	}
	for _, item := range strings.Split(field, ",") {
		if item == "" {
			return false
		}
		stepParts := strings.Split(item, "/")
		if len(stepParts) > 2 || stepParts[0] == "" {
			return false
		}
		if len(stepParts) == 2 {
			step, err := strconv.Atoi(stepParts[1])
			if err != nil || !decimalCronToken(stepParts[1]) || step < 1 || step > spec.max {
				return false
			}
		}
		base := stepParts[0]
		if base == "*" {
			continue
		}
		rangeParts := strings.Split(base, "-")
		if len(rangeParts) > 2 {
			return false
		}
		start, ok := cronFieldValue(rangeParts[0], spec)
		if !ok {
			return false
		}
		if len(rangeParts) == 2 {
			end, ok := cronFieldValue(rangeParts[1], spec)
			if !ok || start > end {
				return false
			}
		}
	}
	return true
}

func cronFieldValue(value string, spec cronFieldSpec) (int, bool) {
	if value == "" {
		return 0, false
	}
	if named, ok := spec.names[strings.ToUpper(value)]; ok {
		return named, true
	}
	number, err := strconv.Atoi(value)
	if err != nil || !decimalCronToken(value) || number < spec.min || number > spec.max {
		return 0, false
	}
	return number, true
}

func decimalCronToken(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func normalizeIPv4OrCIDR(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if ip := net.ParseIP(value); ip != nil && ip.To4() != nil {
		return ip.To4().String(), true
	}
	ip, network, err := net.ParseCIDR(value)
	if err != nil || ip.To4() == nil {
		return "", false
	}
	return network.String(), true
}

type SystemHostsSnapshot struct {
	ResourceVersion string            `json:"resourceVersion"`
	Entries         []SystemHostEntry `json:"entries"`
	Total           int               `json:"total"`
	Truncated       bool              `json:"truncated"`
}

type SystemHostEntry struct {
	Line      int      `json:"line"`
	Address   string   `json:"address"`
	Hostnames []string `json:"hostnames"`
	Comment   string   `json:"comment"`
	Raw       string   `json:"raw"`
}

type SystemCronSnapshot struct {
	ResourceVersion string            `json:"resourceVersion"`
	Entries         []SystemCronEntry `json:"entries"`
	Total           int               `json:"total"`
	Truncated       bool              `json:"truncated"`
}

type SystemCronEntry struct {
	Line       int    `json:"line"`
	Kind       string `json:"kind"`
	Expression string `json:"expression"`
	Command    string `json:"command"`
	Raw        string `json:"raw"`
}

type SystemNetworkSnapshot struct {
	ResourceVersion string                   `json:"resourceVersion"`
	Entries         []SystemNetworkInterface `json:"entries"`
	Total           int                      `json:"total"`
	Truncated       bool                     `json:"truncated"`
}

type SystemNetworkInterface struct {
	Name            string   `json:"name"`
	State           string   `json:"state"`
	MACAddress      string   `json:"macAddress"`
	Addresses       []string `json:"addresses"`
	Loopback        bool     `json:"loopback"`
	ResourceVersion string   `json:"resourceVersion"`
}

type SystemFirewallSnapshot struct {
	ResourceVersion string                      `json:"resourceVersion"`
	Backend         string                      `json:"backend"`
	InputPolicy     string                      `json:"inputPolicy"`
	Rules           []SystemFirewallRule        `json:"rules"`
	CountryRules    []SystemFirewallCountryRule `json:"countryRules"`
	Total           int                         `json:"total"`
	Truncated       bool                        `json:"truncated"`
	PingAllowed     bool                        `json:"pingAllowed"`
	DDoSEnabled     bool                        `json:"ddosEnabled"`
}

type SystemFirewallCountryRule struct {
	Code         string `json:"code"`
	Decision     string `json:"decision"`
	Zone         string `json:"zone"`
	NetworkCount int    `json:"networkCount"`
}

type SystemFirewallRule struct {
	Line        int      `json:"line"`
	Chain       string   `json:"chain"`
	Target      string   `json:"target"`
	Protocol    string   `json:"protocol"`
	Source      string   `json:"source"`
	Destination string   `json:"destination"`
	Options     []string `json:"options"`
	Raw         string   `json:"raw"`
}

type SystemResourceActionResult struct {
	Action          string    `json:"action"`
	Status          string    `json:"status"`
	Changed         bool      `json:"changed"`
	Message         string    `json:"message"`
	ResourceVersion string    `json:"resourceVersion"`
	BackupPath      string    `json:"backupPath,omitempty"`
	AppliedAt       time.Time `json:"appliedAt"`
}
