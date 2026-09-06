package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/cluster"
	"github.com/kejilion/kejilion-panel/internal/contract"
)

func TestUpdateHealthParsing(t *testing.T) {
	now := time.Unix(2000000000, 0)
	for _, tc := range []struct{ name, input, want string }{
		{"current", `{"state":"current","checkedAt":1999999000,"finishedAt":1999999050}`, "current"},
		{"running", `{"state":"running","checkedAt":1999999990}`, "running"},
		{"abandoned", `{"state":"running","checkedAt":1999998000}`, "interrupted"},
		{"secret-field", `{"state":"missing","token":"secret"}`, "invalid"},
		{"error-text", `{"state":"failed","checkedAt":1999999990,"finishedAt":2000000000,"errorCode":"Bearer secret"}`, "invalid"},
		{"future", `{"state":"running","checkedAt":2000000900}`, "invalid"},
		{"time-order", `{"state":"updated","checkedAt":1999999900,"finishedAt":1999999000}`, "invalid"},
		{"success-unfinished", `{"state":"updated","checkedAt":1999999900}`, "invalid"},
		{"trailing", `{"state":"missing"}{}`, "invalid"},
		{"oversize", strings.Repeat(" ", 1025), "invalid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseUpdateHealth([]byte(tc.input), now); got.State != tc.want {
				t.Fatalf("got %#v", got)
			}
		})
	}
}

func TestFixedServiceHealthAndBounds(t *testing.T) {
	content := []byte("Id=kejilion-node-update.timer\nLoadState=loaded\nActiveState=active\nSubState=waiting\nUnitFileState=disabled\n\nId=kejilion-node-file.service\nLoadState=not-found\nActiveState=inactive\nSubState=dead\n\nId=ssh.service\nLoadState=loaded\nActiveState=active\n")
	got := parseServiceHealth(content)
	if got[0].UnitFileState != "disabled" || got[3].LoadState != "not-found" || got[1].LoadState != "unknown" {
		t.Fatalf("got %#v", got)
	}
	if got := parseServiceHealth(bytes.Repeat([]byte("x"), 4097)); got[0].LoadState != "unknown" {
		t.Fatal("oversize accepted")
	}
	var out healthOutput
	if _, err := out.Write(make([]byte, 4097)); err == nil || out.Len() != 0 {
		t.Fatal("output was unbounded")
	}
	// Match os/exec's pipe copying path; embedding bytes.Buffer would expose
	// ReaderFrom and bypass Write even though the direct call above passed.
	if _, err := io.Copy(&out, io.LimitReader(strings.NewReader(strings.Repeat("x", 8192)), 8192)); err == nil || out.Len() > 4096 {
		t.Fatal("io.Copy bypassed output bound")
	}
}

func TestHealthCapabilityIsEphemeralAndOldCenterFallbackResigns(t *testing.T) {
	secret := bytes.Repeat([]byte{1}, 32)
	config := nodeConfig{Health: true, SSHLogin: true, NodeID: strings.Repeat("b", 32)}
	encoded, _ := json.Marshal(config)
	if bytes.Contains(encoded, []byte("health")) || bytes.Contains(encoded, []byte("Health")) {
		t.Fatal("health capability persisted")
	}
	var ids []string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		ids = append(ids, r.Header.Get("X-KPanel-Request-ID"))
		expected := cluster.LightRequestSignature(secret, "POST", lightReportPath, config.NodeID, r.Header.Get("X-KPanel-Timestamp"), ids[len(ids)-1], body)
		if r.Header.Get("X-KPanel-Signature") != expected {
			t.Error("unsigned retry")
		}
		var legacy struct {
			Telemetry contract.HostTelemetry `json:"telemetry"`
		}
		decoder := json.NewDecoder(bytes.NewReader(body))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&legacy); err != nil {
			http.Error(w, "unknown field", 400)
			return
		}
		fmt.Fprintf(w, `{"acceptedAt":%q,"nextReportSeconds":30}`, time.Now().UTC().Format(time.RFC3339))
	}))
	defer server.Close()
	previous := nodeHTTPClient
	nodeHTTPClient = server.Client()
	defer func() { nodeHTTPClient = previous }()
	config.Origin = server.URL
	updated, _, err := sendLightReport(context.Background(), config, secret, reportRequest{Health: &contract.LightNodeHealth{}}, nil)
	if err != nil || len(ids) != 2 || ids[0] == ids[1] || updated.Health || updated.SSHLogin {
		t.Fatalf("fallback: %v %#v ids=%v", err, updated, ids)
	}
	ids = nil
	config.Health = false
	config.SSHLogin = false
	if _, _, err := sendLightReport(context.Background(), config, secret, reportRequest{Health: &contract.LightNodeHealth{}}, nil); err != nil || len(ids) != 1 {
		t.Fatal("initial old-center report expanded")
	}
	headers := http.Header{}
	headers.Set(cluster.LightResponseCapabilitiesHeader, cluster.LightHealthCapability)
	if !enableSSHLoginCapability(config, headers).Health {
		t.Fatal("new capability not learned")
	}
}
