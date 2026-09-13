package panel

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

type federatedFileAgentStub struct {
	getPath  string
	getQuery string
}

func (a *federatedFileAgentStub) Get(_ context.Context, path, query, _ string) (AgentResponse, error) {
	a.getPath = path
	a.getQuery = query
	return AgentResponse{
		StatusCode: http.StatusOK, ContentType: "application/json", Body: []byte(`{"items":[]}`),
	}, nil
}

func (a *federatedFileAgentStub) Do(_ context.Context, _ string, _ string, _ string, _ string, _ []byte) (AgentResponse, error) {
	return AgentResponse{StatusCode: http.StatusOK, ContentType: "application/json", Body: []byte(`{}`)}, nil
}

func TestFederatedFileHandlerOnlyExposesAllowlistedFileRoutes(t *testing.T) {
	agent := &federatedFileAgentStub{}
	server := &Server{agent: agent}
	handler := server.federatedFileHandler()

	request := httptest.NewRequest(http.MethodGet, "/v1/files?path=%2F&limit=100", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || agent.getPath != "/v1/files" || agent.getQuery != "path=%2F&limit=100" {
		t.Fatalf("allowlisted file request = status %d path %q query %q", recorder.Code, agent.getPath, agent.getQuery)
	}

	unknown := httptest.NewRequest(http.MethodGet, "/v1/system/info", nil)
	unknownRecorder := httptest.NewRecorder()
	handler.ServeHTTP(unknownRecorder, unknown)
	if unknownRecorder.Code != http.StatusNotFound {
		t.Fatalf("unknown federated route status = %d, want 404", unknownRecorder.Code)
	}

	invalidQuery := httptest.NewRequest(http.MethodGet, "/v1/files?path=%2F&hostId=local", nil)
	invalidRecorder := httptest.NewRecorder()
	handler.ServeHTTP(invalidRecorder, invalidQuery)
	if invalidRecorder.Code != http.StatusBadRequest {
		t.Fatalf("unknown file query status = %d, want 400", invalidRecorder.Code)
	}
}
