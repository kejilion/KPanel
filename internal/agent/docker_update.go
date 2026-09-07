package agent

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/kejilion/kejilion-panel/internal/dockerx"
)

func (s *Server) checkDockerImageUpdate(w http.ResponseWriter, r *http.Request, requestID, id string) {
	if r.Method != http.MethodPost || r.URL.RawPath != "" || r.URL.RawQuery != "" {
		w.Header().Set("Allow", http.MethodPost)
		writeProblem(w, requestID, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不允许", "")
		return
	}
	var input struct {
		ResourceVersion string `json:"resourceVersion"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, requestID, http.StatusBadRequest, "invalid_request", "请求格式无效", "")
		return
	}
	result, err := s.docker.CheckContainerImageUpdate(r.Context(), id, input.ResourceVersion)
	if err != nil && !errors.Is(err, dockerx.ErrImageUpdateFixed) {
		status, code := http.StatusBadGateway, dockerImageUpdateErrorCode(err)
		switch {
		case errors.Is(err, dockerx.ErrResourceConflict):
			status, code = http.StatusConflict, "resource_conflict"
		case errors.Is(err, dockerx.ErrVersionRequired):
			status, code = http.StatusBadRequest, "resource_version_required"
		case errors.Is(err, dockerx.ErrImageUpdateBusy):
			status, code = http.StatusTooManyRequests, "docker_update_busy"
		case errors.Is(err, dockerx.ErrActionUnsupported):
			status = http.StatusUnprocessableEntity
		}
		writeProblem(w, requestID, status, code, "无法确认镜像更新状态", safeDetail(err))
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// Return only stable categories to the UI; raw registry messages can contain
// URLs or credentials and must never become notification text.
func dockerImageUpdateErrorCode(err error) string {
	switch {
	case errors.Is(err, dockerx.ErrImageUpdateDigestMissing):
		return "docker_update_digest_missing"
	case errors.Is(err, dockerx.ErrImageUpdateIncomparable):
		return "docker_update_incomparable"
	case errors.Is(err, context.DeadlineExceeded):
		return "docker_update_timeout"
	}
	var apiError *dockerx.APIError
	if errors.As(err, &apiError) {
		message := strings.ToLower(apiError.Message)
		switch {
		case apiError.Status == 429 || strings.Contains(message, "toomanyrequests") || strings.Contains(message, "too many requests"):
			return "docker_update_rate_limited"
		case apiError.Status == 401 || apiError.Status == 403 || strings.Contains(message, "unauthorized") || strings.Contains(message, "denied"):
			return "docker_update_registry_auth"
		case apiError.Status == 404 || strings.Contains(message, "manifest unknown") || strings.Contains(message, "manifest_unknown"):
			return "docker_update_registry_missing"
		case apiError.Status == 504 || strings.Contains(message, "timeout") || strings.Contains(message, "deadline exceeded"):
			return "docker_update_timeout"
		}
	}
	return "docker_update_unavailable"
}
