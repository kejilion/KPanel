package panel

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

type federatedFileAgentStub struct {
	getPath  string
	getQuery string
}

type federatedFileStreamAgentStub struct {
	federatedFileAgentStub
	response *http.Response
}

func (a *federatedFileStreamAgentStub) OpenStream(context.Context, string, string, string, string, io.Reader, http.Header, int64) (*http.Response, error) {
	return a.response, nil
}

type interruptedFileBody struct{ sent bool }

func (b *interruptedFileBody) Read(p []byte) (int, error) {
	if !b.sent {
		b.sent = true
		return copy(p, "partial"), nil
	}
	return 0, io.ErrUnexpectedEOF
}
func (*interruptedFileBody) Close() error { return nil }

func TestFederatedFileExportPropagatesSourceFailureAndFinalTrailer(t *testing.T) {
	for _, mode := range []string{"ok", "error-trailer", "missing-trailer", "truncated"} {
		t.Run(mode, func(t *testing.T) {
			var body io.ReadCloser = io.NopCloser(bytes.NewBufferString("complete-data"))
			trailer := make(http.Header)
			if mode == "ok" {
				trailer.Set("X-KPanel-Transfer-Result", "ok")
			}
			if mode == "error-trailer" {
				trailer.Set("X-KPanel-Transfer-Result", "error")
			}
			if mode == "truncated" {
				body = &interruptedFileBody{}
			}
			stub := &federatedFileStreamAgentStub{response: &http.Response{StatusCode: 200, Header: make(http.Header), Body: body, Trailer: trailer, ContentLength: -1}}
			server := &Server{agent: stub}
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest("GET", "/v1/files/transfer/export?path=%2Fa&resourceVersion=sha256:test", nil)
			var caught any
			func() {
				defer func() { caught = recover() }()
				server.federatedFileHandler().ServeHTTP(recorder, request)
			}()
			if mode == "ok" {
				if caught != nil || recorder.Header().Get("X-KPanel-Transfer-Result") != "ok" {
					t.Fatalf("valid export: panic=%v headers=%v", caught, recorder.Header())
				}
			} else if caught != http.ErrAbortHandler {
				t.Fatalf("source failure returned successful response: panic=%v", caught)
			}
		})
	}
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
