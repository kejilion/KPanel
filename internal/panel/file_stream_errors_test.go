package panel

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kejilion/kejilion-panel/internal/cluster"
)

func TestFileRelayProblemClassifiesWithoutExposingCause(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{
		{cluster.ErrRateLimited, 429, "file_relay_rate_limited"},
		{cluster.ErrAuthentication, 403, "file_relay_authentication_failed"},
		{cluster.ErrPrivateOrigin, 502, "file_relay_address_rejected"},
		{cluster.ErrProtocolMismatch, 502, "file_relay_protocol_incompatible"},
		{context.DeadlineExceeded, 504, "file_relay_timeout"},
		{cluster.ErrFileRelayUnavailable, 503, "file_relay_unavailable"},
		{&cluster.FileStreamError{Code: "timeout"}, 504, "file_relay_timeout"},
		{&cluster.FileStreamError{Code: "tls_error"}, 502, "file_relay_tls_error"},
		{&cluster.FileStreamError{Code: "http_rejected", HTTPStatus: 502, Cause: errors.New("https://secret.example/private")}, 502, "file_relay_upgrade_rejected"},
		{&cluster.FileStreamError{Code: "connection_closed"}, 502, "file_relay_connection_closed"},
		{&cluster.FileStreamError{Code: "connection_failed"}, 502, "file_relay_connection_failed"},
	} {
		status, code, detail := fileRelayProblem(tc.err)
		if status != tc.status || code != tc.code || detail == "" || strings.Contains(detail, "secret") {
			t.Fatalf("error=%v status=%d code=%s detail=%s", tc.err, status, code, detail)
		}
	}
}
