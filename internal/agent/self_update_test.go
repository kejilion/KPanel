package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/selfupdate"
)

type agentReleaseSource struct{ version string }

func (source agentReleaseSource) Latest(context.Context) (selfupdate.Release, error) {
	return selfupdate.Release{
		Version: source.version, ImageDigest: "sha256:" + strings.Repeat("a", 64),
	}, nil
}

func authenticatedSelfUpdateRequest(server *Server, method, target, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 32))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	return response
}

func TestSelfUpdateEndpointsExposeTypedPolicyAndCheck(t *testing.T) {
	server := testServer(t)
	server.version = "1.0.0"
	service, err := selfupdate.New(selfupdate.Config{
		StateDir: t.TempDir(), Source: agentReleaseSource{version: "1.1.0"},
		Now: func() time.Time { return time.Date(2026, 9, 14, 1, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}
	server.selfUpdate = service

	read := authenticatedSelfUpdateRequest(server, http.MethodGet, "/v1/self-update", "")
	if read.Code != http.StatusOK {
		t.Fatalf("read status=%d body=%s", read.Code, read.Body.String())
	}
	var initial selfupdate.Status
	if err := json.Unmarshal(read.Body.Bytes(), &initial); err != nil {
		t.Fatal(err)
	}
	if initial.Enabled || initial.State != "disabled" || initial.CurrentVersion != "1.0.0" {
		t.Fatalf("initial status=%#v", initial)
	}

	updateBody := `{"enabled":true,"expectedResourceVersion":"` + initial.ResourceVersion + `"}`
	updated := authenticatedSelfUpdateRequest(server, http.MethodPut, "/v1/self-update", updateBody)
	if updated.Code != http.StatusOK || !strings.Contains(updated.Body.String(), `"enabled":true`) {
		t.Fatalf("update status=%d body=%s", updated.Code, updated.Body.String())
	}

	checked := authenticatedSelfUpdateRequest(server, http.MethodPost, "/v1/self-update/check", "")
	if checked.Code != http.StatusOK || !strings.Contains(checked.Body.String(), `"candidateVersion":"1.1.0"`) || !strings.Contains(checked.Body.String(), `"state":"waiting"`) {
		t.Fatalf("check status=%d body=%s", checked.Code, checked.Body.String())
	}
}

func TestSelfUpdateEndpointsRejectUnavailableAndUntypedRequests(t *testing.T) {
	server := testServer(t)
	unavailable := authenticatedSelfUpdateRequest(server, http.MethodGet, "/v1/self-update", "")
	if unavailable.Code != http.StatusServiceUnavailable || !strings.Contains(unavailable.Body.String(), "self_update_unavailable") {
		t.Fatalf("unavailable status=%d body=%s", unavailable.Code, unavailable.Body.String())
	}

	service, err := selfupdate.New(selfupdate.Config{StateDir: t.TempDir(), Source: agentReleaseSource{version: "1.1.0"}})
	if err != nil {
		t.Fatal(err)
	}
	server.selfUpdate = service
	queried := authenticatedSelfUpdateRequest(server, http.MethodGet, "/v1/self-update?channel=beta", "")
	if queried.Code != http.StatusBadRequest {
		t.Fatalf("query status=%d body=%s", queried.Code, queried.Body.String())
	}
	unknown := authenticatedSelfUpdateRequest(server, http.MethodPut, "/v1/self-update", `{"enabled":true,"expectedResourceVersion":"x","channel":"beta"}`)
	if unknown.Code != http.StatusBadRequest {
		t.Fatalf("unknown field status=%d body=%s", unknown.Code, unknown.Body.String())
	}
	checkBody := authenticatedSelfUpdateRequest(server, http.MethodPost, "/v1/self-update/check", `{}`)
	if checkBody.Code != http.StatusBadRequest {
		t.Fatalf("check body status=%d body=%s", checkBody.Code, checkBody.Body.String())
	}
}
