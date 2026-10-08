package panel

import (
	"fmt"
	"net/http"
	"testing"
)

// The common runner emits terminal events; the authenticated caller must close
// its original audit intent even when connecting to the source fails.
func TestFileTransferErrorEventsCloseTheAuditChain(t *testing.T) {
	server, tokenPath := newTestServer(t)
	cookie, csrf := bootstrapCookies(t, server, tokenPath)
	server.agent = &fileRemoteDownloadAgent{}
	response := authenticatedRequest(server, http.MethodPost, "/api/v1/files/transfers", []byte(fmt.Sprintf(
		`{"sourceNodeId":%q,"path":"/source.bin","resourceVersion":"sha256:test","targetDirectory":"/home"}`, server.cluster.NodeID())),
		cookie, csrf, map[string]string{"Content-Type": "application/json", "Origin": "http://panel.test", "X-CSRF-Token": csrf.Value})
	events := decodeFileRemoteDownloadEvents(t, response.Body.String())
	if response.Code != http.StatusOK || len(events) == 0 || events[len(events)-1].State != "error" {
		t.Fatalf("failure stream=%d %v", response.Code, events)
	}
	audits, _, err := server.store.ListAudit(100, "")
	if err != nil {
		t.Fatal(err)
	}
	intent, failure := false, false
	for _, event := range audits {
		if event.Action != "file.transfer.copy" {
			continue
		}
		intent = intent || event.Result == "intent"
		failure = failure || event.Result == "failure"
	}
	if !intent || !failure {
		t.Fatalf("terminal transfer error left an open audit chain: %#v", audits)
	}
}
