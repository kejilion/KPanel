package panel

import (
	"net/http"
	"strings"

	"github.com/kejilion/kejilion-panel/internal/cluster"
)

const clusterBatchTasksPath = "/api/v1/cluster/batch-tasks"

func (s *Server) handleClusterBatchTaskCatalog(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := s.requireSession(w, r); !ok {
		return
	}
	s.writeJSON(w, http.StatusOK, cluster.BatchTaskCatalogInfo())
}

func (s *Server) handleClusterBatchTasks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if _, _, ok := s.requireSession(w, r); !ok {
			return
		}
		s.writeJSON(w, http.StatusOK, s.cluster.BatchTasks())
	case http.MethodPost:
		s.handleClusterBatchTaskCreate(w, r)
	default:
		s.writeProblem(w, r, http.StatusNotFound, "route_not_found", "Route not found", "")
	}
}

func (s *Server) handleClusterBatchTaskCreate(w http.ResponseWriter, r *http.Request) {
	session, ok := s.requireClusterMutation(w, r)
	if !ok {
		return
	}
	var input cluster.CreateBatchTaskInput
	if err := s.decodeJSON(w, r, &input); err != nil {
		return
	}
	change := map[string]any{
		"action": input.Action, "targetCount": len(input.HostIDs),
		"concurrency": input.Concurrency, "timeoutSeconds": input.TimeoutSeconds,
		"confirmDisruptive": input.ConfirmDisruptive,
	}
	if err := s.audit(r, session.User.ID, "cluster.batch.create", "cluster-batch-task", string(input.Action), "intent", change); err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "audit_unavailable", "Audit storage unavailable", "")
		return
	}
	task, err := s.cluster.CreateBatchTask(r.Context(), input)
	if err != nil {
		_ = s.audit(r, session.User.ID, "cluster.batch.create", "cluster-batch-task", string(input.Action), "failure", change)
		s.writeClusterError(w, r, err)
		return
	}
	acceptedChange := map[string]any{
		"action": task.Action, "targetCount": task.Total,
		"concurrency": task.Concurrency, "timeoutSeconds": task.TimeoutSeconds,
		"confirmDisruptive": input.ConfirmDisruptive, "taskId": task.ID,
	}
	_ = s.audit(r, session.User.ID, "cluster.batch.create", "cluster-batch-task", task.ID, "accepted", acceptedChange)
	s.writeJSON(w, http.StatusAccepted, task)
}

func (s *Server) handleClusterBatchTask(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, clusterBatchTasksPath+"/")
	if rest == r.URL.Path || rest == "" {
		s.writeProblem(w, r, http.StatusNotFound, "route_not_found", "Route not found", "")
		return
	}
	parts := strings.Split(rest, "/")
	if len(parts) == 1 && r.Method == http.MethodGet {
		if _, _, ok := s.requireSession(w, r); !ok {
			return
		}
		if !validClusterBatchTaskID(parts[0]) {
			s.writeClusterBatchTaskNotFound(w, r)
			return
		}
		task, err := s.cluster.BatchTask(parts[0])
		if err != nil {
			s.writeClusterError(w, r, err)
			return
		}
		s.writeJSON(w, http.StatusOK, task)
		return
	}
	if len(parts) == 1 && r.Method == http.MethodDelete {
		s.handleClusterBatchTaskDelete(w, r, parts[0])
		return
	}
	if len(parts) != 2 || r.Method != http.MethodPost || (parts[1] != "cancel" && parts[1] != "retry") {
		s.writeProblem(w, r, http.StatusNotFound, "route_not_found", "Route not found", "")
		return
	}
	if parts[1] == "cancel" {
		s.handleClusterBatchTaskCancel(w, r, parts[0])
		return
	}
	s.handleClusterBatchTaskRetry(w, r, parts[0])
}

func (s *Server) handleClusterBatchTaskCancel(w http.ResponseWriter, r *http.Request, id string) {
	session, ok := s.requireClusterMutation(w, r)
	if !ok {
		return
	}
	if !validClusterBatchTaskID(id) {
		s.writeClusterBatchTaskNotFound(w, r)
		return
	}
	if !s.emptyClusterBatchBody(w, r) {
		return
	}
	if err := s.audit(r, session.User.ID, "cluster.batch.cancel", "cluster-batch-task", id, "intent", nil); err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "audit_unavailable", "Audit storage unavailable", "")
		return
	}
	task, err := s.cluster.CancelBatchTask(id)
	if err != nil {
		_ = s.audit(r, session.User.ID, "cluster.batch.cancel", "cluster-batch-task", id, "failure", nil)
		s.writeClusterError(w, r, err)
		return
	}
	_ = s.audit(r, session.User.ID, "cluster.batch.cancel", "cluster-batch-task", id, "success", nil)
	s.writeJSON(w, http.StatusAccepted, task)
}

func (s *Server) handleClusterBatchTaskRetry(w http.ResponseWriter, r *http.Request, id string) {
	session, ok := s.requireClusterMutation(w, r)
	if !ok {
		return
	}
	if !validClusterBatchTaskID(id) {
		s.writeClusterBatchTaskNotFound(w, r)
		return
	}
	var input cluster.RetryBatchTaskInput
	if err := s.decodeJSON(w, r, &input); err != nil {
		return
	}
	change := map[string]any{"confirmDisruptive": input.ConfirmDisruptive}
	if err := s.audit(r, session.User.ID, "cluster.batch.retry", "cluster-batch-task", id, "intent", change); err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "audit_unavailable", "Audit storage unavailable", "")
		return
	}
	task, err := s.cluster.RetryBatchTask(r.Context(), id, input)
	if err != nil {
		_ = s.audit(r, session.User.ID, "cluster.batch.retry", "cluster-batch-task", id, "failure", change)
		s.writeClusterError(w, r, err)
		return
	}
	_ = s.audit(r, session.User.ID, "cluster.batch.retry", "cluster-batch-task", task.ID, "accepted", map[string]any{"parentTaskId": id})
	s.writeJSON(w, http.StatusAccepted, task)
}

func (s *Server) handleClusterBatchTaskDelete(w http.ResponseWriter, r *http.Request, id string) {
	session, ok := s.requireClusterMutation(w, r)
	if !ok {
		return
	}
	if !validClusterBatchTaskID(id) {
		s.writeClusterBatchTaskNotFound(w, r)
		return
	}
	if !s.emptyClusterBatchBody(w, r) {
		return
	}
	if err := s.audit(r, session.User.ID, "cluster.batch.delete", "cluster-batch-task", id, "intent", nil); err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "audit_unavailable", "Audit storage unavailable", "")
		return
	}
	if err := s.cluster.DeleteBatchTask(id); err != nil {
		_ = s.audit(r, session.User.ID, "cluster.batch.delete", "cluster-batch-task", id, "failure", nil)
		s.writeClusterError(w, r, err)
		return
	}
	_ = s.audit(r, session.User.ID, "cluster.batch.delete", "cluster-batch-task", id, "success", nil)
	s.writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

func (s *Server) emptyClusterBatchBody(w http.ResponseWriter, r *http.Request) bool {
	if r.ContentLength != 0 || len(r.TransferEncoding) != 0 {
		s.writeProblem(w, r, http.StatusBadRequest, "request_body_not_allowed", "Request body not allowed", "")
		return false
	}
	return true
}

func validClusterBatchTaskID(id string) bool {
	return ownerJobIDPattern.MatchString(id)
}

func (s *Server) writeClusterBatchTaskNotFound(w http.ResponseWriter, r *http.Request) {
	s.writeProblem(w, r, http.StatusNotFound, "cluster_batch_task_not_found", "Cluster batch task not found", "")
}
