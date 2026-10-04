package panel

import (
	"net/http"

	"github.com/kejilion/kejilion-panel/internal/auth"
	"github.com/kejilion/kejilion-panel/internal/notification"
)

// notifyPanelLogin hands a successful sign-in to the notification center. It
// never blocks or fails the login; whether a notice is recorded is decided by
// the notification rules.
func (s *Server) notifyPanelLogin(r *http.Request, user auth.PublicUser, passkey bool) {
	if s.notifications == nil {
		return
	}
	method := notification.PanelLoginPassword
	switch {
	case passkey && user.TOTPEnabled:
		method = notification.PanelLoginPasskeyTOTP
	case passkey:
		method = notification.PanelLoginPasskey
	case user.TOTPEnabled:
		// Login refuses a TOTP-enabled account without a verified second factor.
		method = notification.PanelLoginPasswordTOTP
	}
	s.notifications.RecordPanelLogin(notification.PanelLogin{
		Username: user.Username, RemoteAddress: s.remoteIP(r), Method: method,
	})
}
