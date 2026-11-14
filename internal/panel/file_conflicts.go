package panel

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/kejilion/kejilion-panel/internal/cluster"
	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/filemanager"
)

func (s *Server) transferConflictEntry(ctx context.Context, hostID string, kind cluster.HostKind, virtual, requestID string) (*contract.FileEntry, error) {
	query := url.Values{"path": {virtual}}.Encode()
	var payload []byte
	var status int
	if hostID == "" {
		response, err := s.agent.Get(ctx, "/v1/files/entry", query, requestID)
		if err != nil {
			return nil, err
		}
		status, payload = response.StatusCode, response.Body
	} else {
		response, err := s.openFileHostRequest(ctx, hostID, kind, cluster.LightFileRequest{Method: http.MethodGet, Path: "/v1/files/entry", RawQuery: query, Body: http.NoBody})
		if err != nil {
			return nil, err
		}
		defer response.Body.Close()
		stop := context.AfterFunc(ctx, func() { _ = response.Body.Close() })
		defer stop()
		status = response.StatusCode
		payload, err = io.ReadAll(io.LimitReader(response.Body, (64<<10)+1))
		if err != nil {
			return nil, err
		}
	}
	if status == 404 {
		return nil, nil
	}
	if status != 200 {
		return nil, &fileReceiveHTTPError{status: status}
	}
	var entry contract.FileEntry
	if len(payload) > 64<<10 || json.Unmarshal(payload, &entry) != nil || entry.Path != virtual || entry.ResourceVersion == "" {
		return nil, errors.New("invalid conflict metadata")
	}
	return &entry, nil
}

func (s *Server) resolveTransferConflict(ctx context.Context, source contract.FileEntry, directory, hostID string, kind cluster.HostKind, requestID string, changed bool, closeSource func()) (contract.FileConflictResolution, bool, error) {
	session := filemanager.FileConflicts(ctx)
	if session == nil {
		return contract.FileConflictResolution{}, false, filemanager.ErrAction
	}
	copy := *session
	closed := false
	if copy.Prompt != nil {
		original := copy.Prompt
		copy.Prompt = func(ctx context.Context, c contract.FileConflict) (contract.FileConflictDecision, error) {
			if closeSource != nil {
				closeSource()
				closed = true
			}
			return original(ctx, c)
		}
	}
	choice, err := filemanager.ResolveFileConflict(ctx, source, directory, &copy, changed, func(ctx context.Context, path string) (*contract.FileEntry, error) {
		return s.transferConflictEntry(ctx, hostID, kind, path, requestID)
	})
	session.Policies = copy.Policies
	return choice, closed, err
}

func (s *Server) awaitTransferConflict(ctx context.Context, job *contract.FileRemoteDownloadJob, conflict contract.FileConflict, update func(contract.FileTransferEvent) bool, releaseWork func(), acquireWork func() error) (contract.FileConflictDecision, error) {
	if !update(contract.FileTransferEvent{State: "waiting_input", Name: job.Name, LoadedBytes: job.LoadedBytes, TotalBytes: job.TotalBytes, Conflict: &conflict}) {
		return contract.FileConflictDecision{}, errors.New("conflict persistence failed")
	}
	releaseWork()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	timer := time.NewTimer(24 * time.Hour)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return contract.FileConflictDecision{}, ctx.Err()
		case <-timer.C:
			return contract.FileConflictDecision{}, context.DeadlineExceeded
		case <-ticker.C:
			current, err := s.remoteDownloadJobs.Get(job.ID)
			if err != nil {
				return contract.FileConflictDecision{}, err
			}
			if current.Decision == nil {
				continue
			}
			decision := *current.Decision
			*job = current
			if err := acquireWork(); err != nil {
				return contract.FileConflictDecision{}, err
			}
			if !update(contract.FileTransferEvent{State: "connecting", Name: job.Name, LoadedBytes: job.LoadedBytes, TotalBytes: job.TotalBytes}) {
				return contract.FileConflictDecision{}, errors.New("conflict acknowledgement persistence failed")
			}
			return decision, nil
		}
	}
}
