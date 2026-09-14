package agent

import (
	"errors"
	"net/http"

	"github.com/kejilion/kejilion-panel/internal/selfupdate"
)

func (s *Server) selfUpdateSettings(w http.ResponseWriter, r *http.Request, requestID string) {
	if r.URL.RawPath != "" || r.URL.RawQuery != "" {
		writeProblem(w, requestID, http.StatusBadRequest, "invalid_request", "自动更新请求格式无效", "")
		return
	}
	if s.selfUpdate == nil {
		writeProblem(w, requestID, http.StatusServiceUnavailable, "self_update_unavailable", "自动更新仅支持由 kejilion.sh 应用市场管理的 KPanel", "")
		return
	}
	switch r.Method {
	case http.MethodGet:
		status, err := s.selfUpdate.Status(s.version)
		if err != nil {
			writeProblem(w, requestID, http.StatusServiceUnavailable, "self_update_state_unavailable", "自动更新状态不可用", safeDetail(err))
			return
		}
		writeJSON(w, http.StatusOK, status)
	case http.MethodPut:
		var input struct {
			Enabled                 bool   `json:"enabled"`
			ExpectedResourceVersion string `json:"expectedResourceVersion"`
		}
		if err := decodeJSON(w, r, &input); err != nil {
			writeProblem(w, requestID, http.StatusBadRequest, "invalid_request", "自动更新设置格式无效", "")
			return
		}
		status, err := s.selfUpdate.SetEnabled(s.version, input.ExpectedResourceVersion, input.Enabled)
		if err != nil {
			switch {
			case errors.Is(err, selfupdate.ErrConflict):
				writeProblem(w, requestID, http.StatusConflict, "resource_version_changed", "自动更新设置已变化，请刷新后重试", "")
			case errors.Is(err, selfupdate.ErrBusy):
				writeProblem(w, requestID, http.StatusConflict, "self_update_busy", "自动更新任务正在运行", "")
			default:
				writeProblem(w, requestID, http.StatusServiceUnavailable, "self_update_state_unavailable", "自动更新设置无法保存", safeDetail(err))
			}
			return
		}
		writeJSON(w, http.StatusOK, status)
	default:
		writeProblem(w, requestID, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不允许", "")
	}
}

func (s *Server) selfUpdateCheck(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFrom(w)
	if r.URL.RawPath != "" || r.URL.RawQuery != "" || r.ContentLength != 0 || len(r.TransferEncoding) != 0 {
		writeProblem(w, requestID, http.StatusBadRequest, "invalid_request", "自动更新检查请求格式无效", "")
		return
	}
	if s.selfUpdate == nil {
		writeProblem(w, requestID, http.StatusServiceUnavailable, "self_update_unavailable", "自动更新仅支持由 kejilion.sh 应用市场管理的 KPanel", "")
		return
	}
	status, err := s.selfUpdate.Check(r.Context(), s.version)
	if err != nil {
		if errors.Is(err, selfupdate.ErrBusy) {
			writeProblem(w, requestID, http.StatusConflict, "self_update_busy", "自动更新任务正在运行", "")
			return
		}
		writeProblem(w, requestID, http.StatusBadGateway, "self_update_check_failed", "稳定版更新检查失败", safeDetail(err))
		return
	}
	writeJSON(w, http.StatusOK, status)
}
