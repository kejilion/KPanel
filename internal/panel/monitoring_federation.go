package panel

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/kejilion/kejilion-panel/internal/cluster"
	"github.com/kejilion/kejilion-panel/internal/httpstream"
	"github.com/kejilion/kejilion-panel/internal/monitoring"
)

func (s *Server) openLocalHistory(r *http.Request, query monitoring.Query) (*http.Response, error) {
	streamer, ok := s.agent.(agentStreamAPI)
	if !ok {
		return nil, cluster.ErrHistoryUnavailable
	}
	return streamer.OpenStream(r.Context(), http.MethodGet, cluster.HistoryPath, query.Encode(), requestID(r), http.NoBody, nil, 0)
}

func (s *Server) handleFederationHistoryV2(w http.ResponseWriter, r *http.Request, envelope cluster.FederationEnvelopeV2) {
	query, authorization, err := s.cluster.AuthorizeHistoryV2(s.remoteIP(r), envelope)
	if err != nil {
		s.writeHistoryError(w, r, err)
		return
	}
	defer authorization.Close()
	ctx, cancel := context.WithTimeout(r.Context(), cluster.HistoryTimeout)
	defer cancel()
	r = r.WithContext(ctx)
	response, err := s.openLocalHistory(r, query)
	if err != nil {
		s.writeHistoryError(w, r, err)
		return
	}
	defer response.Body.Close()
	stop := context.AfterFunc(ctx, func() { _ = response.Body.Close() })
	defer stop()
	sealed, cipher, err := authorization.SealStatus(response.StatusCode)
	if err != nil {
		s.writeHistoryError(w, r, err)
		return
	}
	writer := httpstream.NewIdleResponseWriter(ctx, w, 30*time.Second)
	writer.Header().Set("Content-Type", "application/x-kpanel-noise-stream")
	writer.Header().Set("Cache-Control", "no-store")
	writer.WriteHeader(http.StatusOK)
	if err := cluster.WriteFederationFileHeader(writer, sealed); err != nil {
		return
	}
	stream := cluster.NewFederationFileWriter(writer, cipher)
	reader := &io.LimitedReader{R: response.Body, N: monitoring.MaxHistoryResponseBytes + 1}
	_, copyErr := io.CopyBuffer(stream, reader, make([]byte, 60<<10))
	if reader.N <= 0 {
		copyErr = errors.New("history response exceeds limit")
	}
	_ = stream.Finish(copyErr)
}

func (s *Server) handleFederationHistoryV1(w http.ResponseWriter, r *http.Request) {
	query, release, err := s.cluster.AuthorizeHistoryV1(s.remoteIP(r), r)
	if err != nil {
		s.writeHistoryError(w, r, err)
		return
	}
	defer release()
	ctx, cancel := context.WithTimeout(r.Context(), cluster.HistoryTimeout)
	defer cancel()
	r = r.WithContext(ctx)
	response, err := s.openLocalHistory(r, query)
	if err != nil {
		s.writeHistoryError(w, r, err)
		return
	}
	defer response.Body.Close()
	stop := context.AfterFunc(ctx, func() { _ = response.Body.Close() })
	defer stop()
	writer := httpstream.NewIdleResponseWriter(ctx, w, 30*time.Second)
	writer.Header().Set("Content-Type", "application/json")
	writer.Header().Set("Cache-Control", "no-store")
	writer.WriteHeader(response.StatusCode)
	reader := &io.LimitedReader{R: response.Body, N: monitoring.MaxHistoryResponseBytes + 1}
	_, err = io.CopyBuffer(writer, reader, make([]byte, 60<<10))
	if err != nil || reader.N <= 0 {
		panic(http.ErrAbortHandler)
	}
}

func (s *Server) writeHistoryError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, monitoring.ErrBusy), errors.Is(err, cluster.ErrRateLimited):
		w.Header().Set("Retry-After", "2")
		s.writeProblem(w, r, http.StatusTooManyRequests, "monitoring_busy", "历史查询繁忙，请稍后重试。", "")
	case errors.Is(err, monitoring.ErrInvalidRange), errors.Is(err, monitoring.ErrInvalidWindow):
		s.writeProblem(w, r, http.StatusBadRequest, "invalid_monitoring_query", "监控时间范围或查询参数无效。", "")
	case errors.Is(err, cluster.ErrHistoryUnsupported):
		s.writeProblem(w, r, http.StatusUpgradeRequired, "monitoring_upgrade_required", "当前节点版本尚不支持远程历史，请升级节点后重试。", "")
	case errors.Is(err, cluster.ErrAuthentication), errors.Is(err, cluster.ErrReplay):
		s.writeProblem(w, r, http.StatusForbidden, "monitoring_access_denied", "节点历史查询授权无效，请检查配对关系。", "")
	case errors.Is(err, cluster.ErrNotFound):
		s.writeProblem(w, r, http.StatusNotFound, "cluster_host_not_found", "所选主机已移除或不存在，请重新选择主机。", "")
	default:
		s.writeProblem(w, r, http.StatusServiceUnavailable, "cluster_history_unavailable", "节点历史监控暂不可用，请确认节点在线、已升级且监控服务正常运行。", "")
	}
}
