package agent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kejilion/kejilion-panel/internal/systeminfo"
)

func TestSystemProcessesSharesNormalizedQueryAfterAuthentication(t *testing.T) {
	server := testServer(t)
	t.Cleanup(server.processReads.close)
	key := processReadKey{query: systeminfo.ProcessQuery{Sort: "cpu", Order: "desc", Limit: 200}}
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		_, err := server.processReads.read(context.Background(), key, func(ctx context.Context) (systeminfo.ProcessSnapshot, error) {
			select {
			case <-release:
				return systeminfo.ProcessSnapshot{Scanned: 123}, nil
			case <-ctx.Done():
				return systeminfo.ProcessSnapshot{}, ctx.Err()
			}
		})
		done <- err
	}()
	awaitProcessReaders(t, &server.processReads, 1)
	for _, item := range []struct {
		path, token string
		status      int
	}{
		{"/v1/system/processes?sort=cpu", "", http.StatusUnauthorized},
		{"/v1/system/processes?sort=cpu", "wrong", http.StatusUnauthorized},
		{"/v1/system/processes", strings.Repeat("x", 32), http.StatusTooManyRequests},
		{"/v1/system/processes?sort=memory", strings.Repeat("x", 32), http.StatusTooManyRequests},
		{"/v1/system/processes?sort=cpu&sort=cpu", strings.Repeat("x", 32), http.StatusBadRequest},
	} {
		request := httptest.NewRequest(http.MethodGet, item.path, nil)
		request.Header.Set("Authorization", "Bearer "+item.token)
		response := httptest.NewRecorder()
		server.ServeHTTP(response, request)
		if response.Code != item.status {
			t.Fatalf("%s: HTTP %d, want %d", item.path, response.Code, item.status)
		}
	}
	request := httptest.NewRequest(http.MethodGet, "/v1/system/processes?sort=%20CPU%20&order=DESC&limit=200&q=%20", nil)
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 32))
	response := httptest.NewRecorder()
	httpDone := make(chan struct{})
	go func() { server.ServeHTTP(response, request); close(httpDone) }()
	awaitProcessReaders(t, &server.processReads, 2)
	close(release)
	<-httpDone
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "\"scanned\":123") {
		t.Fatalf("normalized query did not share observation: HTTP %d %s", response.Code, response.Body.String())
	}
}
