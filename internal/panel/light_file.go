package panel

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/kejilion/kejilion-panel/internal/cluster"
	"github.com/kejilion/kejilion-panel/internal/httpstream"
)

func isLightFileRelayRequest(r *http.Request) bool {
	if r == nil || !cluster.FileRelayRequestPath(strings.TrimPrefix(r.URL.Path, "/api")) {
		return false
	}
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return false
	}
	ids, ok := values["hostId"]
	return ok && len(ids) == 1 && strings.TrimSpace(ids[0]) != ""
}

func (s *Server) handleLightFileRelay(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawPath != "" {
		s.writeProblem(w, r, http.StatusBadRequest, "file_query_invalid", "文件路径无效", "")
		return
	}
	publicPath := r.URL.Path
	agentPath := strings.TrimPrefix(publicPath, "/api")
	if !cluster.FileRelayRequestPath(agentPath) {
		s.writeProblem(w, r, http.StatusNotFound, "route_not_found", "Route not found", "")
		return
	}
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		s.writeProblem(w, r, http.StatusBadRequest, "file_query_invalid", "文件查询参数无效", "")
		return
	}
	hostIDs := values["hostId"]
	if len(hostIDs) != 1 || strings.TrimSpace(hostIDs[0]) == "" {
		s.writeProblem(w, r, http.StatusBadRequest, "file_host_invalid", "文件主机无效", "")
		return
	}
	hostID := strings.TrimSpace(hostIDs[0])
	values.Del("hostId")
	if !s.checkLightFileMethod(w, r) {
		return
	}
	_, session, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead && !s.checkCSRF(w, r, session) {
		return
	}

	transferContext, cancel := context.WithTimeout(r.Context(), panelFileTransferMaxDuration)
	defer cancel()
	response, err := s.cluster.OpenLightFile(transferContext, hostID, cluster.LightFileRequest{
		Method: r.Method, Path: agentPath, RawQuery: values.Encode(),
		Headers: lightFileRelayHeaders(r), Body: r.Body, BodyLength: r.ContentLength,
	})
	if err != nil {
		status, code, detail := http.StatusServiceUnavailable, "file_relay_unavailable", "轻量节点文件代理未连接"
		if errors.Is(err, cluster.ErrRateLimited) {
			status, code, detail = http.StatusTooManyRequests, "file_relay_rate_limited", "轻量节点文件操作过于频繁"
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			_ = s.audit(r, session.User.ID, "file.light.relay", "light-node", hostID, "failure", nil)
		}
		s.writeProblem(w, r, status, code, detail, "")
		return
	}
	defer response.Body.Close()
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		result := "failure"
		if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
			result = "success"
		}
		_ = s.audit(r, session.User.ID, "file.light.relay", "light-node", hostID, result, nil)
	}
	copyFileHeaders(w.Header(), response.Header)
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Pragma", "no-cache")
	writer := httpstream.NewIdleResponseWriter(transferContext, w, panelFileTransferIdleTimeout)
	writer.WriteHeader(response.StatusCode)
	if r.Method == http.MethodHead {
		return
	}
	_, _ = io.CopyBuffer(writer, response.Body, make([]byte, 64<<10))
}

func (s *Server) checkLightFileMethod(w http.ResponseWriter, r *http.Request) bool {
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut:
		return true
	default:
		w.Header().Set("Allow", "GET, HEAD, POST, PUT")
		s.writeProblem(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", "")
		return false
	}
}

func lightFileRelayHeaders(r *http.Request) map[string]string {
	result := make(map[string]string)
	for _, name := range []string{
		"Accept", "Content-Type", "If-Modified-Since", "If-None-Match", "If-Range", "Range",
	} {
		if value := r.Header.Get(name); value != "" {
			result[name] = value
		}
	}
	return result
}
