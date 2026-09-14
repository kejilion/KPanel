package agent

import (
	"context"
	"errors"
	"net/http"
	"time"

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
			Enabled                 bool               `json:"enabled"`
			Channel                 selfupdate.Channel `json:"channel"`
			ExpectedResourceVersion string             `json:"expectedResourceVersion"`
		}
		if err := decodeJSON(w, r, &input); err != nil {
			writeProblem(w, requestID, http.StatusBadRequest, "invalid_request", "自动更新设置格式无效", "")
			return
		}
		status, err := s.selfUpdate.SetPolicy(
			s.version,
			input.ExpectedResourceVersion,
			input.Enabled,
			input.Channel,
		)
		if err != nil {
			switch {
			case errors.Is(err, selfupdate.ErrInvalidChannel):
				writeProblem(w, requestID, http.StatusBadRequest, "invalid_update_channel", "更新通道只允许 stable 或 preview", "")
			case errors.Is(err, selfupdate.ErrConflict):
				writeProblem(w, requestID, http.StatusConflict, "resource_version_changed", "自动更新设置已变化，请刷新后重试", "")
			case errors.Is(err, selfupdate.ErrBusy):
				writeProblem(w, requestID, http.StatusConflict, "self_update_busy", "自动更新任务正在运行", "")
			case errors.Is(err, selfupdate.ErrChannelUnavailable):
				writeProblem(w, requestID, http.StatusServiceUnavailable, "update_channel_unavailable", "所选更新通道当前不可用", "")
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
		writeProblem(w, requestID, http.StatusBadGateway, "self_update_check_failed", "更新通道检查失败", safeDetail(err))
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) selfUpdateInstall(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFrom(w)
	if r.URL.RawPath != "" || r.URL.RawQuery != "" {
		writeProblem(w, requestID, http.StatusBadRequest, "invalid_request", "立即安装请求格式无效", "")
		return
	}
	if s.selfUpdate == nil {
		writeProblem(w, requestID, http.StatusServiceUnavailable, "self_update_unavailable", "自动更新仅支持由 kejilion.sh 应用市场管理的 KPanel", "")
		return
	}
	var input struct {
		ExpectedResourceVersion string `json:"expectedResourceVersion"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, requestID, http.StatusBadRequest, "invalid_request", "立即安装请求格式无效", "")
		return
	}
	status, err := s.selfUpdate.QueueInstall(s.version, input.ExpectedResourceVersion)
	if err != nil {
		switch {
		case errors.Is(err, selfupdate.ErrConflict):
			writeProblem(w, requestID, http.StatusConflict, "resource_version_changed", "更新状态已变化，请刷新后重试", "")
		case errors.Is(err, selfupdate.ErrBusy):
			writeProblem(w, requestID, http.StatusConflict, "self_update_busy", "更新任务正在运行", "")
		case errors.Is(err, selfupdate.ErrNoUpdate):
			writeProblem(w, requestID, http.StatusConflict, "self_update_not_available", "当前没有可立即安装的更新", "")
		default:
			writeProblem(w, requestID, http.StatusServiceUnavailable, "self_update_state_unavailable", "立即安装请求无法保存", safeDetail(err))
		}
		return
	}
	if s.selfUpdateStarter == nil {
		_, _ = s.selfUpdate.CancelQueuedInstall(
			status.CandidateVersion,
			status.CandidateImageDigest,
			errors.New("automatic update service starter is unavailable"),
		)
		writeProblem(w, requestID, http.StatusServiceUnavailable, "self_update_start_unavailable", "宿主机更新服务不可用", "")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err := s.selfUpdateStarter.Start(ctx); err != nil {
		_, _ = s.selfUpdate.CancelQueuedInstall(status.CandidateVersion, status.CandidateImageDigest, err)
		writeProblem(w, requestID, http.StatusServiceUnavailable, "self_update_start_failed", "宿主机更新服务启动失败", safeDetail(err))
		return
	}
	writeJSON(w, http.StatusAccepted, status)
}
