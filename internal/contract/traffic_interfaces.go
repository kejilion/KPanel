package contract

import (
	"errors"
	"fmt"
	"strings"
)

// MaxTrafficInterfaceSelection bounds each list of a traffic interface
// selection. Hosts rarely have more than a handful of uplinks.
const MaxTrafficInterfaceSelection = 16

// Reasons an interface is or is not counted toward host traffic.
const (
	TrafficInterfaceDefaultRoute    = "default-route"
	TrafficInterfaceNotDefaultRoute = "not-default-route"
	TrafficInterfaceAutomatic       = "automatic"
	TrafficInterfaceVirtual         = "virtual"
	TrafficInterfaceLoopback        = "loopback"
	TrafficInterfaceSelected        = "selected"
	TrafficInterfaceNotSelected     = "not-selected"
	TrafficInterfaceExcluded        = "excluded"
	TrafficInterfaceMissing         = "missing"
)

// TrafficInterfaceSelection chooses which interfaces count toward host
// traffic. A non-empty Include replaces the automatic choice (interfaces that
// carry a default route); Exclude removes names from whichever choice applies.
type TrafficInterfaceSelection struct {
	Include []string `json:"include"`
	Exclude []string `json:"exclude"`
}

type TrafficInterfaceStatus struct {
	Name          string `json:"name"`
	ReceivedBytes uint64 `json:"receivedBytes"`
	SentBytes     uint64 `json:"sentBytes"`
	Counted       bool   `json:"counted"`
	Reason        string `json:"reason"`
}

type TrafficInterfacesSnapshot struct {
	Selection TrafficInterfaceSelection `json:"selection"`
	// Interfaces lists every interface in /proc/net/dev plus selected names
	// that are currently missing.
	Interfaces []TrafficInterfaceStatus `json:"interfaces"`
	// SelectionError is set when the stored selection cannot be read; traffic
	// then falls back to the automatic choice.
	SelectionError  string `json:"selectionError,omitempty"`
	ResourceVersion string `json:"resourceVersion"`
}

type UpdateTrafficInterfacesInput struct {
	Include                 []string `json:"include"`
	Exclude                 []string `json:"exclude"`
	ExpectedResourceVersion string   `json:"expectedResourceVersion"`
}

// ValidNetworkInterfaceName follows the Linux interface name rules (at most
// 15 bytes, no slash, whitespace, colon or control character, not "." or
// "..") and rejects the loopback device, which never carries host traffic.
func ValidNetworkInterfaceName(name string) bool {
	if name == "" || len(name) > 15 || name == "." || name == ".." || name == "lo" {
		return false
	}
	for _, character := range name {
		if character <= ' ' || character == 0x7f || character == '/' || character == ':' || character > 0x7e {
			return false
		}
	}
	return true
}

// Normalize trims names, drops duplicates keeping the first occurrence, and
// validates the selection.
func (s TrafficInterfaceSelection) Normalize() (TrafficInterfaceSelection, error) {
	include, err := normalizeInterfaceNames("include", s.Include)
	if err != nil {
		return TrafficInterfaceSelection{}, err
	}
	exclude, err := normalizeInterfaceNames("exclude", s.Exclude)
	if err != nil {
		return TrafficInterfaceSelection{}, err
	}
	for _, name := range include {
		for _, excluded := range exclude {
			if name == excluded {
				return TrafficInterfaceSelection{}, fmt.Errorf("network interface %q is both included and excluded", name)
			}
		}
	}
	return TrafficInterfaceSelection{Include: include, Exclude: exclude}, nil
}

func normalizeInterfaceNames(field string, names []string) ([]string, error) {
	result := []string{}
	seen := map[string]bool{}
	for _, raw := range names {
		name := strings.TrimSpace(raw)
		if !ValidNetworkInterfaceName(name) {
			return nil, fmt.Errorf("%s contains an invalid network interface name", field)
		}
		if !seen[name] {
			seen[name] = true
			result = append(result, name)
		}
	}
	if len(result) > MaxTrafficInterfaceSelection {
		return nil, errors.New(field + " lists too many network interfaces")
	}
	return result, nil
}
