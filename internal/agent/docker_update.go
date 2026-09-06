package agent

import (
	"errors"
	"net/http"

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
		status, code := http.StatusBadGateway, "docker_update_unavailable"
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
