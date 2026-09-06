package agent

import (
	"net/http"
	"time"
)

// These fixed read-only projections reuse the full summary's authoritative
// collectors. Slow optional protocols never run on the runtime path.
func (s *Server) systemRuntime(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawPath != "" || r.URL.RawQuery != "" {
		writeProblem(w, requestIDFrom(w), http.StatusBadRequest, "invalid_query", "不支持查询参数", "")
		return
	}
	summary, err := s.system.CollectRuntime(r.Context())
	if err != nil && summary.Hostname == "" {
		writeProblem(w, requestIDFrom(w), http.StatusServiceUnavailable, "system_unavailable", "系统状态不可用", "")
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func (s *Server) systemManagementRead(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawPath != "" || r.URL.RawQuery != "" {
		writeProblem(w, requestIDFrom(w), http.StatusBadRequest, "invalid_query", "不支持查询参数", "")
		return
	}
	var state any
	switch r.URL.Path {
	case "/v1/system/management/config":
		config := s.system.CollectManagement()
		config.Maintenance = s.systemManager.MaintenanceStatus()
		state = config
	case "/v1/system/management/ssh-defense":
		state = s.systemManager.SSHDefenseStatus(r.Context())
	case "/v1/system/management/bbrv3":
		state = s.systemManager.BBRv3Status(r.Context())
	}
	writeJSON(w, http.StatusOK, struct {
		State      any       `json:"state"`
		ObservedAt time.Time `json:"observedAt"`
	}{State: state, ObservedAt: time.Now().UTC()})
}
