package dockerx

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStandaloneUpgradePayloadPreservesMountsAndRequestedNetworkSettings(t *testing.T) {
	const source = `{
		"Config":{"Image":"example/nginx:alpine","Env":["PROFILE=test"],"Cmd":["sleep","60"]},
		"HostConfig":{"Binds":["/bind:/mnt:ro","/anonymous"],"Mounts":[{"Type":"volume","Target":"/structured","VolumeOptions":{"NoCopy":true}}],"Memory":1048576},
		"NetworkSettings":{"Networks":{
			"dynamic":{"IPAddress":"172.24.0.4","Aliases":["app"],"MacAddress":"02:42:ac:18:00:04"},
			"static":{"IPAddress":"172.25.0.7","IPAMConfig":{"IPv4Address":"172.25.0.7"},"GwPriority":2}
		}}
	}`
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		_, _ = response.Write([]byte(source))
	}))
	defer server.Close()
	client := testHTTPClient(server)
	current := containerInspect{ID: strings.Repeat("a", 64), Mounts: []dockerMount{
		{Type: "volume", Name: "anonymous-old", Destination: "/anonymous", RW: true},
		{Type: "volume", Name: "structured-old", Destination: "/structured", RW: true},
		{Type: "volume", Name: "image-volume-old", Destination: "/image-volume", RW: false},
	}}
	payload, err := client.standaloneUpgradePayload(context.Background(), current, "example/nginx:stable")
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Image      string
		HostConfig struct {
			Binds  []string
			Mounts []struct {
				Source, Target string
				ReadOnly       bool
				VolumeOptions  struct{ NoCopy bool }
			}
			Memory int64
		}
		NetworkingConfig struct {
			EndpointsConfig map[string]map[string]json.RawMessage
		}
	}
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatal(err)
	}
	host := result.HostConfig
	if result.Image != "example/nginx:stable" || host.Memory != 1048576 || strings.Join(host.Binds, ",") != "/bind:/mnt:ro,anonymous-old:/anonymous" ||
		len(host.Mounts) != 2 || host.Mounts[0].Source != "structured-old" || !host.Mounts[0].VolumeOptions.NoCopy ||
		host.Mounts[1].Source != "image-volume-old" || host.Mounts[1].Target != "/image-volume" || !host.Mounts[1].ReadOnly {
		t.Fatalf("host config not preserved: %#v", host)
	}
	endpoints := result.NetworkingConfig.EndpointsConfig
	if endpoints["dynamic"]["IPAddress"] != nil || endpoints["dynamic"]["MacAddress"] != nil ||
		string(endpoints["dynamic"]["Aliases"]) != `["app"]` ||
		string(endpoints["static"]["IPAMConfig"]) != `{"IPv4Address":"172.25.0.7"}` ||
		string(endpoints["static"]["GwPriority"]) != "2" {
		t.Fatalf("network configuration not preserved: %#v", endpoints)
	}
}
