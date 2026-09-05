package panel

import (
	"bytes"
	"container/heap"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kejilion/kejilion-panel/internal/appmarket"
	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/dockerx"
	"github.com/kejilion/kejilion-panel/internal/store"
	"github.com/kejilion/kejilion-panel/internal/webenv"
)

type auditJobGroup struct {
	requestID  string
	action     string
	targetKind string
	targetID   string
	intent     *store.AuditEvent
	outcome    *store.AuditEvent
}

func (s *Server) handleJobs(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/v1/jobs" || r.URL.RawPath != "" {
		s.writeProblem(w, r, http.StatusNotFound, "route_not_found", "Route not found", "")
		return
	}
	_, _, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	limit, valid := jobsLimit(r)
	if !valid {
		s.writeValidationProblem(w, r, "limit", "limit must be between 1 and 100")
		return
	}
	events, _ := s.store.ListAudit(200, "")
	jobs := jobsFromAudit(events, limit)
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	for _, source := range []struct {
		path   string
		decode func(context.Context, []byte) ([]contract.Job, error)
	}{
		{"/v1/docker/jobs", decodeOwnerJobs(jobsFromDockerJobs)},
		{"/v1/app-jobs", decodeOwnerJobs(jobsFromAppJobs)},
		{"/v1/web-environment/jobs", decodeOwnerJobs(jobsFromWebEnvironment)},
	} {
		response, err := s.hostOps.Get(ctx, source.path, "", requestID(r))
		if err != nil || response.StatusCode != http.StatusOK {
			s.writeProblem(w, r, http.StatusServiceUnavailable, "job_source_unavailable", "无法确认后台任务状态，请稍后刷新或返回业务页面查看", "")
			return
		}
		owned, err := source.decode(ctx, response.Body)
		if err != nil {
			if ctx.Err() != nil {
				s.writeProblem(w, r, http.StatusServiceUnavailable, "job_source_unavailable", "无法确认后台任务状态，请稍后刷新或返回业务页面查看", "")
				return
			}
			s.writeProblem(w, r, http.StatusBadGateway, "job_source_invalid", "后台任务状态无效，请稍后刷新", "")
			return
		}
		jobs = mergeOwnerJobs(jobs, owned)
	}
	sort.SliceStable(jobs, func(left, right int) bool {
		return jobs[left].CreatedAt.After(jobs[right].CreatedAt)
	})
	if len(jobs) > limit {
		jobs = jobs[:limit]
	}
	s.writeJSON(w, http.StatusOK, contract.PageResult[contract.Job]{Items: jobs})
}

// Owners may expose more history than this view displays. The Agent transport
// already bounds response bytes; decode one owner record at a time and retain
// only the newest 100 (the maximum public jobs limit), regardless of input order.
func decodeOwnerJobs[T any](adapt func([]T) []contract.Job) func(context.Context, []byte) ([]contract.Job, error) {
	return func(ctx context.Context, body []byte) ([]contract.Job, error) {
		decoder := json.NewDecoder(bytes.NewReader(body))
		invalid := errors.New("invalid owner job page")
		if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
			return nil, invalid
		}
		latest := make(ownerJobWindow, 0, 100)
		foundItems := false
		for decoder.More() {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			key, err := decoder.Token()
			if err != nil {
				return nil, err
			}
			if key != "items" {
				// Preserve PageResult's compatibility with optional envelope fields.
				var ignored json.RawMessage
				if err := decoder.Decode(&ignored); err != nil {
					return nil, err
				}
				continue
			}
			if foundItems {
				return nil, invalid
			}
			foundItems = true
			if token, err := decoder.Token(); err != nil || token != json.Delim('[') {
				return nil, invalid
			}
			for decoder.More() {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
				var item T
				if err := decoder.Decode(&item); err != nil {
					return nil, err
				}
				job := adapt([]T{item})[0]
				if len(latest) < cap(latest) {
					heap.Push(&latest, job)
				} else if newerOwnerJob(job, latest[0]) {
					latest[0] = job
					heap.Fix(&latest, 0)
				}
			}
			if token, err := decoder.Token(); err != nil || token != json.Delim(']') {
				return nil, invalid
			}
		}
		if token, err := decoder.Token(); err != nil || token != json.Delim('}') || !foundItems {
			return nil, invalid
		}
		if _, err := decoder.Token(); err != io.EOF {
			return nil, invalid
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		sort.Slice(latest, func(i, j int) bool { return newerOwnerJob(latest[i], latest[j]) })
		return []contract.Job(latest), nil
	}
}

type ownerJobWindow []contract.Job

func newerOwnerJob(a, b contract.Job) bool {
	if a.CreatedAt.Equal(b.CreatedAt) {
		return a.ID < b.ID
	}
	return a.CreatedAt.After(b.CreatedAt)
}
func (w ownerJobWindow) Len() int           { return len(w) }
func (w ownerJobWindow) Less(i, j int) bool { return newerOwnerJob(w[j], w[i]) }
func (w ownerJobWindow) Swap(i, j int)      { w[i], w[j] = w[j], w[i] }
func (w *ownerJobWindow) Push(value any)    { *w = append(*w, value.(contract.Job)) }
func (w *ownerJobWindow) Pop() any {
	last := len(*w) - 1
	job := (*w)[last]
	(*w)[last] = contract.Job{}
	*w = (*w)[:last]
	return job
}

func mergeOwnerJobs(jobs, owned []contract.Job) []contract.Job {
	index := make(map[string]int, len(jobs))
	for i, job := range jobs {
		index[job.ID] = i
	}
	for _, job := range owned {
		if i, exists := index[job.ID]; exists {
			jobs[i] = job
		} else {
			index[job.ID] = len(jobs)
			jobs = append(jobs, job)
		}
	}
	return jobs
}

var ownerJobIDPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)

// Audit records describe submission, never the eventual result of a 202 task.
// Store only the owner's identity, not arbitrary response fields or inputs.
func acceptedJobAudit(response AgentResponse, source string, change map[string]any) (string, map[string]any) {
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "failure", change
	}
	if response.StatusCode != http.StatusAccepted {
		return "success", change
	}
	if change == nil {
		change = make(map[string]any)
	}
	var job struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(response.Body, &job) == nil && ownerJobIDPattern.MatchString(job.ID) {
		change["jobSource"] = source
		change["jobId"] = job.ID
	}
	return "accepted", change
}

func jobsFromDockerJobs(items []dockerx.MaintenanceJob) []contract.Job {
	jobs := make([]contract.Job, 0, len(items))
	for _, item := range items {
		job := contract.Job{ID: "docker:" + item.ID, Action: "docker." + item.Action, Origin: contract.OriginWeb,
			State: ownerJobState(item.Status), Stage: item.Stage, Progress: item.Progress,
			TargetKind: "docker", TargetID: item.Target, TargetLabel: item.Target,
			CreatedAt: item.CreatedAt, StartedAt: item.StartedAt, FinishedAt: item.FinishedAt}
		if job.State == contract.JobFailedNeedsAttention {
			job.Error = &contract.Problem{Title: "Docker 任务需要检查", Code: "docker_job_attention", Detail: item.Message}
		}
		jobs = append(jobs, job)
	}
	return jobs
}

func ownerJobState(status string) contract.JobState {
	switch status {
	case "queued":
		return contract.JobQueued
	case "running":
		return contract.JobRunning
	case "succeeded":
		return contract.JobSucceeded
	case "cancelled":
		return contract.JobCancelled
	case "interrupted":
		return contract.JobInterrupted
	default:
		return contract.JobFailedNeedsAttention
	}
}

func jobsFromAppJobs(items []appmarket.AppJob) []contract.Job {
	jobs := make([]contract.Job, 0, len(items))
	for _, item := range items {
		state := ownerJobState(item.Status)
		job := contract.Job{
			ID: "app:" + item.ID, Action: "app." + item.Action, Origin: contract.OriginWeb,
			State: state, Stage: item.Stage, Progress: item.Progress,
			TargetKind: "application", TargetID: item.AppID, TargetLabel: item.AppName,
			CreatedAt: item.CreatedAt, StartedAt: item.StartedAt, FinishedAt: item.FinishedAt,
		}
		if state == contract.JobFailedNeedsAttention {
			job.Error = &contract.Problem{
				Title: "应用任务需要检查", Code: "app_job_attention",
				Detail: item.Message, Retryable: item.Status == "failed",
			}
		}
		jobs = append(jobs, job)
	}
	return jobs
}

func jobsFromWebEnvironment(items []webenv.Job) []contract.Job {
	jobs := make([]contract.Job, 0, len(items))
	for _, item := range items {
		state := ownerJobState(item.Status)
		job := contract.Job{
			ID: "webenv:" + item.ID, Action: "web.environment." + item.Action, Origin: contract.OriginWeb,
			State: state, Stage: item.Stage, Progress: item.Progress,
			TargetKind: "web_environment", TargetID: item.Target, TargetLabel: item.Target,
			CreatedAt: item.CreatedAt, StartedAt: item.StartedAt, FinishedAt: item.FinishedAt,
		}
		if state == contract.JobFailedNeedsAttention {
			job.Error = &contract.Problem{
				Title: "LDNMP 环境任务未完成", Code: "web_environment_job_failed",
				Detail: item.Message, Retryable: item.Status == "failed",
			}
		}
		jobs = append(jobs, job)
	}
	return jobs
}

func jobsLimit(r *http.Request) (int, bool) {
	values := r.URL.Query()
	for key := range values {
		if key != "limit" {
			return 0, false
		}
	}
	rawValues, present := values["limit"]
	if !present {
		return 50, true
	}
	if len(rawValues) != 1 || rawValues[0] == "" {
		return 0, false
	}
	value, err := strconv.Atoi(rawValues[0])
	if err != nil || value < 1 || value > 100 {
		return 0, false
	}
	return value, true
}

func jobsFromAudit(events []store.AuditEvent, limit int) []contract.Job {
	if limit < 1 || limit > 100 {
		limit = 50
	}
	groups := make(map[string]*auditJobGroup)
	for index := range events {
		event := &events[index]
		if !managementAuditAction(event.Action) || !jobAuditResult(event.Result) {
			continue
		}
		groupRequestID := event.RequestID
		if groupRequestID == "" {
			groupRequestID = event.ID
		}
		key := strings.Join(
			[]string{groupRequestID, event.Action, event.TargetKind, event.TargetID},
			"\x00",
		)
		group := groups[key]
		if group == nil {
			group = &auditJobGroup{
				requestID: groupRequestID, action: event.Action,
				targetKind: event.TargetKind, targetID: event.TargetID,
			}
			groups[key] = group
		}
		switch event.Result {
		case "intent":
			if group.intent == nil || event.OccurredAt.After(group.intent.OccurredAt) {
				group.intent = event
			}
		case "success", "accepted", "failure", "denied":
			if group.outcome == nil || event.OccurredAt.After(group.outcome.OccurredAt) {
				group.outcome = event
			}
		}
	}

	jobs := make([]contract.Job, 0, len(groups))
	for _, group := range groups {
		// Old successful async submissions have no reliable job identity. They
		// remain in the audit history; only owner records can supply their result.
		async := strings.HasPrefix(group.action, "app.") || strings.HasPrefix(group.action, "web.environment.") ||
			dockerx.IsMaintenanceAction(strings.TrimPrefix(group.action, "docker."))
		if async && group.outcome != nil && group.outcome.Result == "success" {
			continue
		}
		createdAt := time.Time{}
		if group.intent != nil {
			createdAt = group.intent.OccurredAt
		} else if group.outcome != nil {
			createdAt = group.outcome.OccurredAt
		}
		job := contract.Job{
			ID: group.requestID, Action: group.action, Origin: contract.OriginWeb,
			State: contract.JobRunning, Stage: "running",
			TargetKind: group.targetKind, TargetID: group.targetID,
			TargetLabel: group.targetID, CreatedAt: createdAt,
		}
		if group.intent != nil {
			startedAt := group.intent.OccurredAt
			job.StartedAt = &startedAt
		}
		if group.outcome != nil {
			finishedAt := group.outcome.OccurredAt
			job.FinishedAt = &finishedAt
			job.Progress = 100
			switch group.outcome.Result {
			case "accepted":
				job.State = contract.JobFailedNeedsAttention
				job.Stage = "status_unavailable"
				job.Progress = 0
				job.StartedAt = nil
				job.FinishedAt = nil
				job.Error = &contract.Problem{Title: "任务已接受，执行结果尚未确认", Code: "job_status_unavailable", Detail: "任务不在当前可查询记录中，请返回业务页面核对资源状态"}
				source, _ := group.outcome.Change["jobSource"].(string)
				id, _ := group.outcome.Change["jobId"].(string)
				if (source == "docker" || source == "app" || source == "webenv") && ownerJobIDPattern.MatchString(id) {
					job.ID = source + ":" + id
				}
			case "success":
				job.State = contract.JobSucceeded
				job.Stage = "completed"
			default:
				job.State = contract.JobFailedNeedsAttention
				job.Stage = "attention_required"
			}
		}
		if group.outcome == nil {
			job.State = contract.JobFailedNeedsAttention
			job.Stage = "outcome_unknown"
			job.StartedAt = nil
			job.Error = &contract.Problem{Title: "仅有操作提交记录，执行结果未确认", Code: "job_outcome_unknown"}
		}
		jobs = append(jobs, job)
	}
	sort.SliceStable(jobs, func(left, right int) bool {
		return jobs[left].CreatedAt.After(jobs[right].CreatedAt)
	})
	if len(jobs) > limit {
		jobs = jobs[:limit]
	}
	return jobs
}

func managementAuditAction(action string) bool {
	return strings.HasPrefix(action, "app.") || strings.HasPrefix(action, "docker.") ||
		strings.HasPrefix(action, "site.") ||
		strings.HasPrefix(action, "system.") ||
		strings.HasPrefix(action, "web.environment.")
}

func jobAuditResult(result string) bool {
	switch result {
	case "intent", "accepted", "success", "failure", "denied":
		return true
	default:
		return false
	}
}
