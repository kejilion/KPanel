package agent

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/systeminfo"
)

func trafficInterfacesServer(t *testing.T) *Server {
	t.Helper()
	server := testServer(t)
	procRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(procRoot, "net"), 0o755); err != nil {
		t.Fatal(err)
	}
	dev := "Inter-| Receive | Transmit\n" +
		" lo: 1 0 0 0 0 0 0 0 1 0 0 0 0 0 0 0\n" +
		" eth0: 1000 0 0 0 0 0 0 0 2000 0 0 0 0 0 0 0\n" +
		" eth1: 300 0 0 0 0 0 0 0 400 0 0 0 0 0 0 0\n"
	route := "Iface Destination Gateway Flags RefCnt Use Metric Mask MTU Window IRTT\n" +
		"eth0 00000000 0100000A 0003 0 0 100 00000000 0 0 0\n"
	for name, content := range map[string]string{"dev": dev, "route": route} {
		if err := os.WriteFile(filepath.Join(procRoot, "net", name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	server.system = &systeminfo.Collector{
		ProcRoot: procRoot, TrafficSelectionPath: filepath.Join(t.TempDir(), "traffic-interfaces.json"),
	}
	return server
}

func trafficInterfacesRequest(t *testing.T, server *Server, method, body string) *httptest.ResponseRecorder {
	t.Helper()
	var request *http.Request
	if body == "" {
		request = httptest.NewRequest(method, "/v1/system/traffic-interfaces", nil)
	} else {
		request = httptest.NewRequest(method, "/v1/system/traffic-interfaces", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
	}
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 32))
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	return response
}

func TestTrafficInterfacesReadAndReplace(t *testing.T) {
	server := trafficInterfacesServer(t)
	read := trafficInterfacesRequest(t, server, http.MethodGet, "")
	if read.Code != http.StatusOK {
		t.Fatalf("GET status = %d body=%s", read.Code, read.Body.String())
	}
	var snapshot contract.TrafficInterfacesSnapshot
	if err := json.Unmarshal(read.Body.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Interfaces) != 3 || !snapshot.Interfaces[0].Counted || snapshot.Interfaces[0].Name != "eth0" ||
		snapshot.Interfaces[0].Reason != contract.TrafficInterfaceDefaultRoute {
		t.Fatalf("snapshot = %#v", snapshot)
	}

	update := trafficInterfacesRequest(t, server, http.MethodPut,
		`{"include":["eth0","eth1"],"exclude":[],"expectedResourceVersion":"`+snapshot.ResourceVersion+`"}`)
	if update.Code != http.StatusOK {
		t.Fatalf("PUT status = %d body=%s", update.Code, update.Body.String())
	}
	var updated contract.TrafficInterfacesSnapshot
	if err := json.Unmarshal(update.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if strings.Join(updated.Selection.Include, ",") != "eth0,eth1" || !updated.Interfaces[1].Counted {
		t.Fatalf("updated = %#v", updated)
	}

	stale := trafficInterfacesRequest(t, server, http.MethodPut,
		`{"include":[],"exclude":[],"expectedResourceVersion":"`+snapshot.ResourceVersion+`"}`)
	if stale.Code != http.StatusConflict || !strings.Contains(stale.Body.String(), "traffic_interfaces_changed") {
		t.Fatalf("stale PUT status = %d body=%s", stale.Code, stale.Body.String())
	}
	invalid := trafficInterfacesRequest(t, server, http.MethodPut,
		`{"include":["eth0/x"],"exclude":[],"expectedResourceVersion":"`+updated.ResourceVersion+`"}`)
	if invalid.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid PUT status = %d body=%s", invalid.Code, invalid.Body.String())
	}
}

func TestTrafficInterfacesRejectsMalformedRequests(t *testing.T) {
	server := trafficInterfacesServer(t)
	for _, test := range []struct {
		method string
		body   string
		status int
	}{
		{method: http.MethodPost, status: http.StatusMethodNotAllowed},
		{method: http.MethodPut, body: `{"include":[],"unknown":1}`, status: http.StatusBadRequest},
		{method: http.MethodPut, body: `{"include":["` + strings.Repeat("e", 5000) + `"]}`, status: http.StatusBadRequest},
	} {
		if response := trafficInterfacesRequest(t, server, test.method, test.body); response.Code != test.status {
			t.Fatalf("%s %.40q status = %d, want %d", test.method, test.body, response.Code, test.status)
		}
	}
	server.system = &systeminfo.Collector{ProcRoot: t.TempDir()}
	if response := trafficInterfacesRequest(t, server, http.MethodGet, ""); response.Code != http.StatusServiceUnavailable {
		t.Fatalf("unconfigured GET status = %d", response.Code)
	}
}
