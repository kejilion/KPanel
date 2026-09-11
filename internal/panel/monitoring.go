package panel

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"time"

	"github.com/kejilion/kejilion-panel/internal/cluster"
	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/monitoring"
)

func (s *Server) handleClusterHistory(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := s.requireSession(w, r); !ok {
		return
	}
	if r.Method != http.MethodGet {
		s.writeProblem(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", "")
		return
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	valid := err == nil && r.URL.RawPath == "" && len(r.URL.RawQuery) <= 512 && r.ContentLength == 0 && len(r.TransferEncoding) == 0
	for key, values := range query {
		if len(values) != 1 || (key != "hostId" && key != "range" && key != "start" && key != "end") {
			valid = false
		}
	}
	var start, end time.Time
	if query.Has("start") || query.Has("end") {
		var startErr, endErr error
		start, startErr = time.Parse(time.RFC3339Nano, query.Get("start"))
		end, endErr = time.Parse(time.RFC3339Nano, query.Get("end"))
		valid = valid && startErr == nil && endErr == nil
	}
	if !valid || len(query.Get("hostId")) != 32 {
		s.writeProblem(w, r, http.StatusBadRequest, "invalid_monitoring_query", "Invalid monitoring query", "")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), cluster.HistoryTimeout)
	defer cancel()
	started := false
	err = s.cluster.WithHistory(ctx, query.Get("hostId"), query.Get("range"), start, end, func(result contract.MonitoringHistory) error {
		content, err := json.Marshal(result)
		if err != nil || int64(len(content)) > monitoring.MaxHistoryResponseBytes {
			return cluster.ErrHistoryUnavailable
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		// Bound slow clients as well as remote reads. Cancellation interrupts a
		// blocked network write; the query slot stays held until it has unwound.
		controller := http.NewResponseController(w)
		deadline := time.Now().Add(30 * time.Second)
		if parent, ok := ctx.Deadline(); ok && parent.Before(deadline) {
			deadline = parent
		}
		_ = controller.SetWriteDeadline(deadline)
		interrupted := make(chan struct{})
		stop := context.AfterFunc(ctx, func() {
			_ = controller.SetWriteDeadline(time.Now())
			close(interrupted)
		})
		defer func() {
			if !stop() {
				<-interrupted
			}
		}()
		compressed := len(content) >= 1024 && r.Header.Get("Range") == "" && acceptsGzip(r.Header.Get("Accept-Encoding"))
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		addVary(w.Header(), "Accept-Encoding")
		if compressed {
			w.Header().Set("Content-Encoding", "gzip")
		}
		started = true
		w.WriteHeader(http.StatusOK)
		// Compress directly to the browser, without a second response buffer.
		return monitoring.CopyHistoryPayload(w, bytes.NewReader(content), compressed)
	})
	if err != nil && !started {
		s.writeHistoryError(w, r, err)
	}
}
