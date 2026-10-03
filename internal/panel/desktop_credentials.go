package panel

import (
	"errors"
	"net/http"
	"strings"

	"github.com/kejilion/kejilion-panel/internal/backup"
	"github.com/kejilion/kejilion-panel/internal/desktopcredentials"
)

func (s *Server) desktopCredentialBinding(userID, hostID string) (desktopcredentials.Binding, error) {
	user, err := s.store.UserByID(userID)
	if err != nil {
		return desktopcredentials.Binding{}, err
	}
	identity, err := s.cluster.DesktopCredentialIdentity(hostID)
	if err != nil {
		return desktopcredentials.Binding{}, err
	}
	return desktopcredentials.Binding{UserID: userID, HostID: hostID, UserVersion: user.CredentialVersion, HostIdentity: identity}, nil
}

func (s *Server) savedDesktopCredentials(userID, hostID string) (desktopcredentials.Credentials, error) {
	if s.desktopCredentials == nil {
		return desktopcredentials.Credentials{}, desktopcredentials.ErrInvalid
	}
	binding, err := s.desktopCredentialBinding(userID, hostID)
	if err != nil {
		return desktopcredentials.Credentials{}, desktopcredentials.ErrMissing
	}
	return s.desktopCredentials.Get(binding)
}

// All three operations use the same authenticated POST/Origin/CSRF boundary as
// opening a desktop. Status never returns a password, including after saving.
func (s *Server) handleDesktopCredentials(w http.ResponseWriter, r *http.Request, userID string) {
	w.Header().Set("Cache-Control", "no-store")
	operation := strings.TrimPrefix(r.URL.Path, desktopSessionsPath+"/credentials/")
	if r.Method != http.MethodPost || (operation != "status" && operation != "save" && operation != "clear") {
		s.writeProblem(w, r, 404, "route_not_found", "Route not found", "")
		return
	}
	var input struct {
		HostID   string `json:"hostId"`
		Username string `json:"username"`
		Domain   string `json:"domain"`
		Password string `json:"password"`
	}
	if s.decodeJSON(w, r, &input) != nil {
		return
	}
	if !backup.ValidID(input.HostID) {
		s.writeProblem(w, r, 400, "invalid_desktop_request", "Invalid desktop host", "")
		return
	}
	if s.desktopCredentials == nil {
		s.writeProblem(w, r, 503, "desktop_credentials_unavailable", "Saved desktop credentials unavailable", "")
		return
	}
	if operation == "clear" {
		if s.audit(r, userID, "desktop.credentials.clear", "cluster_host", input.HostID, "intent", nil) != nil {
			s.writeProblem(w, r, 503, "audit_unavailable", "Audit storage unavailable", "")
			return
		}
		// Clearing remains possible when the host is offline or was removed.
		if s.desktopCredentials.Delete(userID, input.HostID) != nil {
			s.writeProblem(w, r, 503, "desktop_credentials_unavailable", "Saved desktop credentials unavailable", "")
			return
		}
		_ = s.audit(r, userID, "desktop.credentials.clear", "cluster_host", input.HostID, "success", nil)
		s.writeJSON(w, 200, map[string]bool{"saved": false})
		return
	}
	binding, err := s.desktopCredentialBinding(userID, input.HostID)
	if err != nil {
		if operation == "status" {
			s.writeJSON(w, 200, map[string]bool{"saved": false})
			return
		}
		s.writeProblem(w, r, 409, "desktop_unavailable", "Remote desktop is unavailable", "")
		return
	}
	var credentials desktopcredentials.Credentials
	if operation == "save" {
		host, hostErr := s.cluster.Host(r.Context(), input.HostID)
		if hostErr != nil || !host.DesktopAvailable {
			s.writeProblem(w, r, 409, "desktop_unavailable", "Remote desktop is unavailable", "")
			return
		}
		credentials = desktopcredentials.Credentials{Username: input.Username, Domain: input.Domain, Password: input.Password}
		if credentials.Validate() != nil {
			s.writeProblem(w, r, 400, "invalid_desktop_credentials", "Invalid Windows account", "")
			return
		}
		if s.audit(r, userID, "desktop.credentials.save", "cluster_host", input.HostID, "intent", nil) != nil {
			s.writeProblem(w, r, 503, "audit_unavailable", "Audit storage unavailable", "")
			return
		}
		if s.desktopCredentials.Put(binding, credentials) != nil {
			s.writeProblem(w, r, 503, "desktop_credentials_unavailable", "Saved desktop credentials unavailable", "")
			return
		}
		_ = s.audit(r, userID, "desktop.credentials.save", "cluster_host", input.HostID, "success", nil)
	} else {
		credentials, err = s.desktopCredentials.Get(binding)
		if errors.Is(err, desktopcredentials.ErrMissing) {
			s.writeJSON(w, 200, map[string]bool{"saved": false})
			return
		}
		if err != nil {
			s.writeProblem(w, r, 503, "desktop_credentials_unavailable", "Saved desktop credentials unavailable", "")
			return
		}
	}
	s.writeJSON(w, 200, map[string]any{"saved": true, "username": credentials.Username, "domain": credentials.Domain})
}
