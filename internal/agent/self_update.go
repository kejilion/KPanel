package agent

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/kejilion/kejilion-panel/internal/selfupdate"
)

const kpanelReleaseCacheTTL = 15 * time.Minute

type cachedKPanelRelease struct {
	release   selfupdate.Release
	fetchedAt time.Time
}

type kpanelReleaseInfo struct {
	Channel selfupdate.Channel `json:"channel"`
	selfupdate.Release
	Cached bool `json:"cached"`
	Stale  bool `json:"stale"`
}

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

func (s *Server) selfUpdateRelease(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFrom(w)
	if r.URL.RawPath != "" {
		writeProblem(w, requestID, http.StatusBadRequest, "invalid_request", "版本说明请求格式无效", "")
		return
	}
	values := r.URL.Query()
	channels, ok := values["channel"]
	if !ok || len(values) != 1 || len(channels) != 1 ||
		(channels[0] != string(selfupdate.ChannelStable) && channels[0] != string(selfupdate.ChannelPreview)) {
		writeProblem(w, requestID, http.StatusBadRequest, "invalid_update_channel", "更新通道只允许 stable 或 preview", "")
		return
	}
	channel := selfupdate.Channel(channels[0])

	if status, ok := s.cachedSelfUpdateRelease(channel, false); ok {
		writeJSON(w, http.StatusOK, releaseInfo(channel, status, true, false))
		return
	}
	if release, ok := s.cachedRelease(channel, false); ok {
		writeJSON(w, http.StatusOK, releaseInfo(channel, release, true, false))
		return
	}
	source := s.releaseSources[channel]
	if source == nil {
		writeProblem(w, requestID, http.StatusServiceUnavailable, "release_info_unavailable", "版本说明暂不可用", "")
		return
	}
	release, err := source.Latest(r.Context())
	if err != nil {
		if stale, ok := s.cachedRelease(channel, true); ok {
			writeJSON(w, http.StatusOK, releaseInfo(channel, stale, true, true))
			return
		}
		if stale, ok := s.cachedSelfUpdateRelease(channel, true); ok {
			writeJSON(w, http.StatusOK, releaseInfo(channel, stale, true, true))
			return
		}
		writeProblem(w, requestID, http.StatusBadGateway, "release_info_fetch_failed", "版本说明加载失败", safeDetail(err))
		return
	}
	s.releaseCacheMu.Lock()
	s.releaseCache[channel] = cachedKPanelRelease{release: release, fetchedAt: s.now().UTC()}
	s.releaseCacheMu.Unlock()
	writeJSON(w, http.StatusOK, releaseInfo(channel, release, false, false))
}

func (s *Server) cachedSelfUpdateRelease(channel selfupdate.Channel, allowStale bool) (selfupdate.Release, bool) {
	if s.selfUpdate == nil {
		return selfupdate.Release{}, false
	}
	status, err := s.selfUpdate.Status(s.version)
	if err != nil || status.Channel != channel || status.CandidateVersion == "" || status.CandidateImageDigest == "" {
		return selfupdate.Release{}, false
	}
	if status.CandidateReleaseURL == "" && status.CandidatePublishedAt == "" &&
		len(status.CandidateNotes) == 0 && len(status.CandidateUpgradeNotes) == 0 {
		return selfupdate.Release{}, false
	}
	if !allowStale && (status.State == "check_failed" || status.LastCheckedAt == nil ||
		s.now().UTC().Sub(status.LastCheckedAt.UTC()) >= kpanelReleaseCacheTTL) {
		return selfupdate.Release{}, false
	}
	return selfupdate.Release{
		Version: status.CandidateVersion, ImageDigest: status.CandidateImageDigest,
		ReleaseURL: status.CandidateReleaseURL, PublishedAt: status.CandidatePublishedAt,
		Notes:        append([]selfupdate.ReleaseNote(nil), status.CandidateNotes...),
		UpgradeNotes: append([]string(nil), status.CandidateUpgradeNotes...),
	}, true
}

func (s *Server) cachedRelease(channel selfupdate.Channel, allowStale bool) (selfupdate.Release, bool) {
	s.releaseCacheMu.Lock()
	defer s.releaseCacheMu.Unlock()
	cached, ok := s.releaseCache[channel]
	if !ok || (!allowStale && s.now().UTC().Sub(cached.fetchedAt) >= kpanelReleaseCacheTTL) {
		return selfupdate.Release{}, false
	}
	return cached.release, true
}

func releaseInfo(channel selfupdate.Channel, release selfupdate.Release, cached, stale bool) kpanelReleaseInfo {
	return kpanelReleaseInfo{Channel: channel, Release: release, Cached: cached, Stale: stale}
}
