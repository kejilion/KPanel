package panel

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

type gatedTOTPBody struct {
	reader  io.Reader
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (b *gatedTOTPBody) Read(p []byte) (int, error) {
	b.once.Do(func() { close(b.started) })
	<-b.release
	return b.reader.Read(p)
}

func TestTOTPEnrollmentCannotFinishAfterPasswordRevokesPreauthenticatedRequest(t *testing.T) {
	s, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, s, tokenPath)
	session, err := s.auth.Authenticate(sessionCookie.Value)
	if err != nil {
		t.Fatal(err)
	}
	enrollment, err := s.auth.StartTOTPEnrollment(session.User.ID, "a-strong-password-1")
	if err != nil {
		t.Fatal(err)
	}
	code := panelTestTOTP(t, enrollment.Secret, time.Now())
	payload, _ := json.Marshal(map[string]string{"enrollmentId": enrollment.ID, "code": code})
	body := &gatedTOTPBody{reader: bytes.NewReader(payload), started: make(chan struct{}), release: make(chan struct{})}
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(body.release) }) }
	request := httptest.NewRequest(http.MethodPut, "http://panel.test/api/v1/settings/totp/enrollment", body)
	request.AddCookie(sessionCookie)
	request.AddCookie(csrfCookie)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://panel.test")
	request.Header.Set("X-CSRF-Token", csrfCookie.Value)
	response := httptest.NewRecorder()
	done := make(chan struct{})
	go func() { defer close(done); s.ServeHTTP(response, request) }()
	defer func() {
		release()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("request did not stop")
		}
	}()
	select {
	case <-body.started:
	case <-time.After(5 * time.Second):
		t.Fatal("request did not reach body after session/CSRF validation")
	}
	if err := s.auth.ChangePassword(session.User.ID, "a-strong-password-1", "replacement-strong-password-2"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.auth.Authenticate(sessionCookie.Value); err == nil {
		t.Fatal("password change retained old session")
	}
	release()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("request did not finish")
	}
	if response.Code != http.StatusGone {
		t.Fatalf("stale TOTP enrollment accepted after credential/session revocation: HTTP %d", response.Code)
	}
	status, err := s.auth.TOTPStatus(session.User.ID)
	if err != nil || status.Enabled {
		t.Fatalf("stale enrollment changed TOTP state: enabled=%v err=%v", status.Enabled, err)
	}
}
