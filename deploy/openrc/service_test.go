package openrc

import (
	"os"
	"strings"
	"testing"
)

func TestAgentServiceKeepsFixedRootBoundaryAndSupervision(t *testing.T) {
	content, err := os.ReadFile("kejilion-agent")
	if err != nil {
		t.Fatal(err)
	}
	service := string(content)
	for _, required := range []string{
		"#!/sbin/openrc-run",
		`command="/usr/local/libexec/kejilion-agent"`,
		`command_user="root:kejilion-panel"`,
		`pidfile="/run/kejilion-panel/agent.pid"`,
		`supervisor="supervise-daemon"`,
		"stopgroup=true",
		"no_new_privs=true",
		`output_logger="logger -t kejilion-agent"`,
		`error_logger="logger -t kejilion-agent"`,
		"need localmount docker",
		"checkpath --directory --mode 0750 --owner root:kejilion-panel /run/kejilion-panel",
	} {
		if !strings.Contains(service, required) {
			t.Fatalf("OpenRC service is missing %q", required)
		}
	}
	for _, forbidden := range []string{"eval ", "sh -c", "command_args=\"$", "output_log=", "error_log="} {
		if strings.Contains(service, forbidden) {
			t.Fatalf("OpenRC service contains unsafe construct %q", forbidden)
		}
	}
}
