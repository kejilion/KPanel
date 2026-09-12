package contract

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

// ParseSiteAddress accepts the existing ASCII domain syntax. An explicit port
// selects HTTP and additionally permits IPv4 addresses. The host stays the
// filesystem identity; a port must never become part of a site directory.
func ParseSiteAddress(raw string) (host string, port int, err error) {
	invalid := func() (string, int, error) { return "", 0, fmt.Errorf("invalid site address") }
	if raw == "" || strings.TrimSpace(raw) != raw || len(raw) > 259 {
		return invalid()
	}
	host = strings.ToLower(raw)
	if strings.Contains(host, ":") {
		var text string
		host, text, err = net.SplitHostPort(host)
		if err != nil || len(text) == 0 || len(text) > 5 {
			return invalid()
		}
		for _, c := range text {
			if c < '0' || c > '9' {
				return invalid()
			}
		}
		port, err = strconv.Atoi(text)
		if err != nil || port < 1 || port > 65535 {
			return invalid()
		}
	}
	if ip := net.ParseIP(host); ip != nil {
		if port == 0 || ip.To4() == nil || strings.Contains(host, ":") {
			return invalid()
		}
		return ip.String(), port, nil
	}
	if port != 0 && strings.Trim(host, "0123456789.") == "" {
		return invalid()
	}
	if len(host) > 253 || !strings.Contains(host, ".") || strings.HasSuffix(host, ".") {
		return invalid()
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return invalid()
		}
		for _, c := range label {
			if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' {
				return invalid()
			}
		}
	}
	return host, port, nil
}
