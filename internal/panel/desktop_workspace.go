package panel

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/kejilion/kejilion-panel/internal/desktopworkspace"
)

const desktopShortcutIconPrefix = "/api/v1/desktop/shortcuts/"

func (s *Server) handleDesktopWorkspace(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPut {
		w.Header().Set("Allow", http.MethodGet+", "+http.MethodPut)
		s.writeProblem(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", "")
		return
	}
	if r.URL.RawPath != "" || r.URL.RawQuery != "" {
		s.writeProblem(w, r, http.StatusBadRequest, "desktop_workspace_request_invalid", "Desktop workspace request is invalid", "")
		return
	}
	if r.Method == http.MethodPut && !s.checkOrigin(w, r) {
		return
	}
	_, session, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	if s.desktopWorkspace == nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "desktop_workspace_unavailable", "Desktop workspace unavailable", "")
		return
	}
	if r.Method == http.MethodGet {
		s.writeJSON(w, http.StatusOK, desktopWorkspaceResponse(s.desktopWorkspace.Workspace()))
		return
	}
	if !s.checkCSRF(w, r, session) {
		return
	}

	var input desktopworkspace.ReplaceInput
	if err := decodeLimitedJSON(w, r, desktopworkspace.MaxWorkspaceBytes, &input); err != nil {
		return
	}
	if !desktopworkspace.ValidResourceVersion(input.ExpectedResourceVersion) {
		s.writeValidationProblem(w, r, "expectedResourceVersion", "a valid resourceVersion is required")
		return
	}
	change := map[string]any{
		"changedCounts": desktopWorkspaceChangedCounts(s.desktopWorkspace.Workspace(), input),
	}
	if err := s.audit(r, session.User.ID, "desktop.workspace.update", "desktop-workspace", "workspace", "intent", change); err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "audit_unavailable", "Audit storage unavailable", "")
		return
	}
	workspace, err := s.desktopWorkspace.Replace(input)
	if err != nil {
		_ = s.audit(r, session.User.ID, "desktop.workspace.update", "desktop-workspace", "workspace", "failure", change)
		s.writeDesktopWorkspaceError(w, r, err)
		return
	}
	_ = s.audit(r, session.User.ID, "desktop.workspace.update", "desktop-workspace", "workspace", "success", change)
	s.writeJSON(w, http.StatusOK, desktopWorkspaceResponse(workspace))
}

func (s *Server) handleDesktopShortcutIcon(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPut && r.Method != http.MethodDelete {
		w.Header().Set("Allow", http.MethodGet+", "+http.MethodPut+", "+http.MethodDelete)
		s.writeProblem(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", "")
		return
	}
	id, ok := desktopShortcutIconID(r.URL.Path)
	if !ok {
		s.writeProblem(w, r, http.StatusNotFound, "route_not_found", "Route not found", "")
		return
	}
	if r.URL.RawPath != "" || !validDesktopShortcutIconQuery(r) {
		s.writeProblem(w, r, http.StatusBadRequest, "desktop_shortcut_icon_request_invalid", "Desktop shortcut icon request is invalid", "")
		return
	}
	if r.Method != http.MethodGet && !s.checkOrigin(w, r) {
		return
	}
	_, session, authenticated := s.requireSession(w, r)
	if !authenticated {
		return
	}
	if s.desktopWorkspace == nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "desktop_workspace_unavailable", "Desktop workspace unavailable", "")
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.serveDesktopShortcutIcon(w, r, id)
	case http.MethodPut:
		if !s.checkCSRF(w, r, session) {
			return
		}
		s.putDesktopShortcutIcon(w, r, session.User.ID, id)
	case http.MethodDelete:
		if !s.checkCSRF(w, r, session) {
			return
		}
		s.deleteDesktopShortcutIcon(w, r, session.User.ID, id)
	}
}

func (s *Server) serveDesktopShortcutIcon(w http.ResponseWriter, r *http.Request, id string) {
	icon, err := s.desktopWorkspace.Icon(id)
	if err != nil {
		s.writeDesktopWorkspaceError(w, r, err)
		return
	}
	etag := `"` + icon.Version + `"`
	requestedVersion := r.URL.Query().Get("v")
	if requestedVersion == icon.Version {
		w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "private, no-cache")
	}
	w.Header().Set("ETag", etag)
	if requestMatchesETag(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", icon.ContentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(icon.Data)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(icon.Data)
}

func (s *Server) putDesktopShortcutIcon(w http.ResponseWriter, r *http.Request, actorID, id string) {
	contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || (contentType != "image/png" && contentType != "image/jpeg" && contentType != "image/webp") {
		s.writeProblem(w, r, http.StatusUnsupportedMediaType, "desktop_shortcut_icon_type_invalid", "PNG, JPEG, or WebP icon required", "")
		return
	}
	if r.ContentLength > desktopworkspace.MaxIconBytes {
		s.writeProblem(w, r, http.StatusRequestEntityTooLarge, "desktop_shortcut_icon_too_large", "Desktop shortcut icon is too large", "")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, desktopworkspace.MaxIconBytes)
	data, err := io.ReadAll(r.Body)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			s.writeProblem(w, r, http.StatusRequestEntityTooLarge, "desktop_shortcut_icon_too_large", "Desktop shortcut icon is too large", "")
			return
		}
		s.writeProblem(w, r, http.StatusBadRequest, "desktop_shortcut_icon_read_failed", "Unable to read desktop shortcut icon", "")
		return
	}
	change := map[string]any{"bytes": len(data), "contentType": contentType}
	if err := s.audit(r, actorID, "desktop.shortcut.icon.update", "desktop-shortcut", id, "intent", change); err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "audit_unavailable", "Audit storage unavailable", "")
		return
	}
	icon, err := s.desktopWorkspace.PutIcon(id, contentType, data)
	if err != nil {
		_ = s.audit(r, actorID, "desktop.shortcut.icon.update", "desktop-shortcut", id, "failure", change)
		s.writeDesktopWorkspaceError(w, r, err)
		return
	}
	_ = s.audit(r, actorID, "desktop.shortcut.icon.update", "desktop-shortcut", id, "success", change)
	s.writeJSON(w, http.StatusOK, map[string]string{
		"iconVersion": icon.Version,
		"iconURL":     desktopShortcutIconURL(id, icon.Version),
	})
}

func (s *Server) deleteDesktopShortcutIcon(w http.ResponseWriter, r *http.Request, actorID, id string) {
	if err := s.audit(r, actorID, "desktop.shortcut.icon.delete", "desktop-shortcut", id, "intent", nil); err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "audit_unavailable", "Audit storage unavailable", "")
		return
	}
	if err := s.desktopWorkspace.DeleteIcon(id); err != nil {
		_ = s.audit(r, actorID, "desktop.shortcut.icon.delete", "desktop-shortcut", id, "failure", nil)
		s.writeDesktopWorkspaceError(w, r, err)
		return
	}
	_ = s.audit(r, actorID, "desktop.shortcut.icon.delete", "desktop-shortcut", id, "success", nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) writeDesktopWorkspaceError(w http.ResponseWriter, r *http.Request, err error) {
	var validation *desktopworkspace.ValidationError
	switch {
	case errors.As(err, &validation):
		s.writeValidationProblem(w, r, validation.Field, validation.Detail)
	case errors.Is(err, desktopworkspace.ErrConflict):
		s.writeProblem(w, r, http.StatusConflict, "desktop_workspace_changed", "Desktop workspace changed", "")
	case errors.Is(err, desktopworkspace.ErrUnavailable):
		s.writeProblem(w, r, http.StatusServiceUnavailable, "desktop_workspace_unavailable", "Desktop workspace unavailable", "")
	case errors.Is(err, desktopworkspace.ErrNotFound):
		s.writeProblem(w, r, http.StatusNotFound, "desktop_shortcut_icon_not_found", "Desktop shortcut icon not found", "")
	case errors.Is(err, desktopworkspace.ErrIconInvalid):
		s.writeValidationProblem(w, r, "icon", "icon must be a valid PNG, JPEG, or WebP image within the configured limits")
	case errors.Is(err, desktopworkspace.ErrIconQuota):
		s.writeProblem(w, r, http.StatusInsufficientStorage, "desktop_shortcut_icon_quota_exceeded", "Desktop shortcut icon storage limit reached", "")
	default:
		s.writeProblem(w, r, http.StatusInternalServerError, "desktop_workspace_operation_failed", "Desktop workspace operation failed", "")
	}
}

func desktopWorkspaceResponse(workspace desktopworkspace.Workspace) desktopworkspace.Workspace {
	for index := range workspace.Shortcuts {
		if workspace.Shortcuts[index].IconVersion != "" {
			workspace.Shortcuts[index].IconURL = desktopShortcutIconURL(
				workspace.Shortcuts[index].ID,
				workspace.Shortcuts[index].IconVersion,
			)
		}
	}
	return workspace
}

func desktopShortcutIconURL(id, version string) string {
	return desktopShortcutIconPrefix + id + "/icon?v=" + version
}

func desktopShortcutIconID(path string) (string, bool) {
	if !strings.HasPrefix(path, desktopShortcutIconPrefix) || !strings.HasSuffix(path, "/icon") {
		return "", false
	}
	id := strings.TrimSuffix(strings.TrimPrefix(path, desktopShortcutIconPrefix), "/icon")
	if strings.Contains(id, "/") || !desktopworkspace.ValidShortcutID(id) {
		return "", false
	}
	return id, true
}

func validDesktopShortcutIconQuery(r *http.Request) bool {
	if r.Method != http.MethodGet {
		return r.URL.RawQuery == ""
	}
	if r.URL.RawQuery == "" {
		return true
	}
	values := r.URL.Query()
	versions, ok := values["v"]
	return ok && len(values) == 1 && len(versions) == 1 &&
		len(versions[0]) == 64 && strings.IndexFunc(versions[0], func(character rune) bool {
		return (character < '0' || character > '9') && (character < 'a' || character > 'f')
	}) == -1
}

func desktopWorkspaceChangedCounts(current desktopworkspace.Workspace, input desktopworkspace.ReplaceInput) map[string]int {
	return map[string]int{
		"hiddenEntries": changedStringSetCount(current.HiddenEntryKeys, input.HiddenEntryKeys),
		"positions":     changedPositionCount(current.Positions, input.Positions),
		"labels":        changedStringMapCount(current.Labels, input.Labels),
		"shortcuts":     changedShortcutCount(current.Shortcuts, input.Shortcuts),
	}
}

func changedStringSetCount(left, right []string) int {
	leftSet := make(map[string]bool, len(left))
	rightSet := make(map[string]bool, len(right))
	for _, value := range left {
		leftSet[value] = true
	}
	for _, value := range right {
		rightSet[value] = true
	}
	changed := 0
	for value := range leftSet {
		if !rightSet[value] {
			changed++
		}
	}
	for value := range rightSet {
		if !leftSet[value] {
			changed++
		}
	}
	return changed
}

func changedPositionCount(left, right map[string]desktopworkspace.Position) int {
	changed := 0
	for key, leftValue := range left {
		if rightValue, ok := right[key]; !ok || rightValue != leftValue {
			changed++
		}
	}
	for key := range right {
		if _, ok := left[key]; !ok {
			changed++
		}
	}
	return changed
}

func changedStringMapCount(left, right map[string]string) int {
	changed := 0
	for key, leftValue := range left {
		if rightValue, ok := right[key]; !ok || rightValue != leftValue {
			changed++
		}
	}
	for key := range right {
		if _, ok := left[key]; !ok {
			changed++
		}
	}
	return changed
}

func changedShortcutCount(left []desktopworkspace.Shortcut, right []desktopworkspace.ShortcutInput) int {
	leftByID := make(map[string]desktopworkspace.Shortcut, len(left))
	rightByID := make(map[string]desktopworkspace.ShortcutInput, len(right))
	for _, shortcut := range left {
		leftByID[shortcut.ID] = shortcut
	}
	for _, shortcut := range right {
		rightByID[shortcut.ID] = shortcut
	}
	changed := 0
	for id, leftValue := range leftByID {
		rightValue, ok := rightByID[id]
		if !ok || leftValue.Name != rightValue.Name || leftValue.Description != rightValue.Description ||
			leftValue.TargetType != rightValue.TargetType || leftValue.URL != rightValue.URL || leftValue.Path != rightValue.Path {
			changed++
		}
	}
	for id := range rightByID {
		if _, ok := leftByID[id]; !ok {
			changed++
		}
	}
	return changed
}
