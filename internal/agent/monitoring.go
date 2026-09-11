package agent

import (
	"github.com/kejilion/kejilion-panel/internal/monitoring"
	"net/http"
)

// NewMonitoringHandler exposes only the existing history reader to an
// authenticated node relay. It has no system, file or terminal action routes.
func NewMonitoringHandler(provider monitoringHistoryProvider) http.Handler {
	s := &Server{monitoring: provider}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/monitoring/history" || r.URL.RawPath != "" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if _, err := monitoring.ParseQuery(r.URL.RawQuery); err != nil || r.ContentLength != 0 || len(r.TransferEncoding) != 0 {
			writeProblem(w, requestIDFrom(w), http.StatusUnprocessableEntity, "invalid_monitoring_query", "监控查询参数无效", "")
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		s.monitoringHistory(w, r)
	})
}
