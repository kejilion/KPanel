package panel

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"github.com/kejilion/kejilion-panel/internal/cluster"
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
	result, err := s.cluster.History(ctx, query.Get("hostId"), query.Get("range"), start, end)
	if err != nil {
		s.writeHistoryError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	s.writeJSON(w, http.StatusOK, result)
}
