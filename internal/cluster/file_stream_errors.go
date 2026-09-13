package cluster

import (
	"context"
	"errors"
	"net"
	"net/http"

	"github.com/coder/websocket"
)

// FileStreamError exposes a bounded diagnostic without leaking a URL, address,
// credential or an untrusted proxy response into the browser or audit log.
type FileStreamError struct {
	Stage      string
	Code       string
	HTTPStatus int
	Cause      error
}

func (e *FileStreamError) Error() string { return "file stream " + e.Stage + ": " + e.Code }
func (e *FileStreamError) Unwrap() error { return e.Cause }

func fileStreamTransportError(stage string, err error) error {
	if err == nil {
		return nil
	}
	switch websocket.CloseStatus(err) {
	case websocket.StatusTryAgainLater:
		return ErrRateLimited
	case websocket.StatusPolicyViolation:
		return ErrAuthentication
	}
	code := "connection_closed"
	if stage == "upgrade" {
		code = "connection_failed"
	}
	var timed net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &timed) && timed.Timeout()) {
		code = "timeout"
	} else if classified := classifyRemoteTransportError(err); classified != nil {
		var remote *RemoteError
		if errors.As(classified, &remote) && remote.Code == "tls_error" {
			code = "tls_error"
		}
		if errors.Is(classified, ErrPrivateOrigin) {
			return ErrPrivateOrigin
		}
	}
	return &FileStreamError{Stage: stage, Code: code, Cause: err}
}

func fileStreamHTTPError(status int, err error) error {
	if status == http.StatusTooManyRequests {
		return ErrRateLimited
	}
	return &FileStreamError{Stage: "upgrade", Code: "http_rejected", HTTPStatus: status, Cause: err}
}
