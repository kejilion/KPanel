//go:build windows

package main

import (
	"strings"
	"testing"
)

func TestWindowsTrafficInterfacesExplainsMissingLinuxCounters(t *testing.T) {
	for _, arguments := range [][]string{nil, {"include", "Ethernet"}} {
		if err := runTrafficInterfaces(arguments); err == nil || !strings.Contains(err.Error(), "Linux /proc") {
			t.Fatalf("interfaces %v: expected unsupported platform, got %v", arguments, err)
		}
	}
}
