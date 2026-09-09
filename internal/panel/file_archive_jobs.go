package panel

import "github.com/kejilion/kejilion-panel/internal/contract"

func jobsFromFileArchives(items []contract.FileArchiveJob) []contract.Job {
	jobs := make([]contract.Job, 0, len(items))
	for _, item := range items {
		state := ownerJobState(item.State)
		if item.State == "complete" {
			state = contract.JobSucceeded
		}
		if item.State == "cancelling" {
			state = contract.JobRunning
		}
		job := contract.Job{ID: "file-archive:" + item.ID, Action: "file." + item.Action, Origin: contract.OriginWeb, State: state, Stage: item.State, TargetKind: "file", TargetID: item.Target, TargetLabel: item.Name, CreatedAt: item.CreatedAt}
		if item.State != "queued" {
			started := item.CreatedAt
			job.StartedAt = &started
		}
		if item.State != "queued" && item.State != "running" && item.State != "cancelling" {
			finished := item.UpdatedAt
			job.FinishedAt = &finished
		}
		if state == contract.JobSucceeded {
			job.Progress = 100
		}
		if state == contract.JobFailedNeedsAttention || state == contract.JobInterrupted {
			job.Error = &contract.Problem{Title: "归档任务需要检查", Code: "archive_job_attention", Detail: item.Detail}
			if job.Error.Detail == "" && len(item.Result.Failed) > 0 {
				job.Error.Detail = item.Result.Failed[0].Detail
			}
		}
		jobs = append(jobs, job)
	}
	return jobs
}
