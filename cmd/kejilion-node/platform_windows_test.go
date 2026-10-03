//go:build windows

package main

import (
	"context"
	"encoding/base64"
	"github.com/kejilion/kejilion-panel/internal/windowsnode"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
)

func TestWindowsManagementRequiresFreshCenterCapability(t *testing.T) {
	supports := true
	requests := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != "POST" || r.URL.Path != "/api/v3/federation/light/capabilities" || r.Header.Get("X-KPanel-Signature") == "" {
			t.Error("unsigned/wrong capability probe")
		}
		data, _ := io.ReadAll(r.Body)
		if string(data) != "{}" {
			t.Error("capability probe changed state body")
		}
		if !supports {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"capabilities":["windows-node-v1"]}`)
	}))
	defer server.Close()
	prior := nodeHTTPClient
	nodeHTTPClient = server.Client()
	defer func() { nodeHTTPClient = prior }()
	config := nodeConfig{Origin: server.URL, NodeID: "0123456789abcdef0123456789abcdef", ReportingKey: base64.RawURLEncoding.EncodeToString(make([]byte, 32)), Capabilities: []string{"monitoring", "terminal"}, WindowsNode: true}
	if err := requireWindowsCapability(context.Background(), config, "terminal"); err != nil {
		t.Fatal(err)
	}
	supports = false
	if err := requireWindowsCapability(context.Background(), config, "terminal"); err == nil {
		t.Fatal("cached WindowsNode allowed a rolled-back center")
	}
	if requests != 2 {
		t.Fatal("capability must be checked again")
	}
	if err := requireWindowsCapability(context.Background(), config, "files"); err == nil {
		t.Fatal("uninstalled file capability granted")
	}
	if requests != 2 {
		t.Fatal("uninstalled capability should fail before network access")
	}
}
func TestWindowsServiceHealthMapping(t *testing.T) {
	state := mapWindowsServiceHealth(svc.Stopped, 5, mgr.StartAutomatic)
	if state.ActiveState != "failed" || state.SubState != "failed" || state.UnitFileState != "enabled" {
		t.Fatalf("%+v", state)
	}
	state = mapWindowsServiceHealth(svc.Running, 0, mgr.StartManual)
	if state.ActiveState != "active" || state.UnitFileState != "static" {
		t.Fatalf("%+v", state)
	}
}
func TestWindowsCapabilitiesAndPrivilegeBoundary(t *testing.T) {
	selected, err := enrollmentCapabilities("monitoring,files,monitoring")
	if err != nil || !slices.Equal(selected, []string{"monitoring", "files"}) {
		t.Fatalf("%v %v", selected, err)
	}
	if _, err := enrollmentCapabilities("monitoring,unknown"); err == nil {
		t.Fatal("unknown capability accepted")
	}
	if !windowsnode.IsSystem() {
		if requireBrokerIdentity() == nil || requireEnrollmentIdentity() == nil {
			t.Fatal("admin process gained SYSTEM broker/enrollment identity")
		}
	}
}
