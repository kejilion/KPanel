package agent

import "net/http"

// This read-only classification uses the same native Compose discovery as
// the lifecycle operations. An unavailable classification is never an allow.
func (s *Server) fileWritePolicy(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	if len(query) != 1 || len(query["path"]) != 1 || r.URL.RawPath != "" {
		writeProblem(w, requestIDFrom(w), http.StatusBadRequest, "invalid_query", "文件路径无效", "")
		return
	}
	composeSource, err := s.docker.IsComposeSource(r.Context(), query.Get("path"))
	if err != nil {
		writeProblem(w, requestIDFrom(w), http.StatusServiceUnavailable, "file_write_policy_unavailable", "文件审批分类暂不可用", "")
		return
	}
	writeJSON(w, http.StatusOK, struct {
		ComposeSource bool `json:"composeSource"`
	}{composeSource})
}
