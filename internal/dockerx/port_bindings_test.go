package dockerx

import (
	"encoding/json"
	"testing"
)

func TestStoppedContainerRetainsConfiguredLoopbackPorts(t *testing.T) {
	for _, state := range []string{"exited", "created"} {
		t.Run(state, func(t *testing.T) {
			var raw containerInspect
			if err := json.Unmarshal([]byte(`{
				"Id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				"Name":"/speedtest", "Config":{"Image":"ghcr.io/librespeed/speedtest"},
				"HostConfig":{"PortBindings":{"8080/tcp":[{"HostIp":"127.0.0.1","HostPort":"8028"}]}},
			"NetworkSettings":{"Ports":{"8080/tcp":null}}
			}`), &raw); err != nil {
				t.Fatal(err)
			}
			raw.State.Status = state
			got := (&Client{}).summaryFromInspect(raw)
			if len(got.Ports) != 1 || got.Ports[0].IP != "127.0.0.1" || got.Ports[0].PublicPort != 8028 {
				t.Fatalf("stopped configured binding was lost: %#v", got.Ports)
			}
			raw.State.Running = true
			if got := (&Client{}).summaryFromInspect(raw); len(got.Ports) != 1 || got.Ports[0].PublicPort != 0 {
				t.Fatalf("running container fabricated an active binding from config: %#v", got.Ports)
			}
		})
	}
}
