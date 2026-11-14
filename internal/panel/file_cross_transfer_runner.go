package panel

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"time"

	"github.com/kejilion/kejilion-panel/internal/cluster"
	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/filemanager"
	"github.com/kejilion/kejilion-panel/internal/remotedownload"
)

func (s *Server) startCrossFileTransferJob(w http.ResponseWriter, r *http.Request, input contract.FileTransferRequest, hostID string, kind cluster.HostKind, actorID string) {
	var accessLifetime context.Context
	var hostAccessLifetimes []context.Context
	sourceHostID := input.SourceNodeID
	if input.SourceNodeID != s.cluster.NodeID() {
		var err error
		sourceHostID, err = s.cluster.FileSourceHostID(input.SourceNodeID)
		if err != nil {
			s.writeProblem(w, r, 400, "source_unavailable", "来源主机无效。", "")
			return
		}
	}
	if input.SourceNodeID != s.cluster.NodeID() || hostID != "" && hostID != cluster.LocalHostID {
		accessLifetime = s.cluster.RemoteOperationLifetime("files")
	}
	if input.SourceNodeID != s.cluster.NodeID() {
		hostAccessLifetimes = append(hostAccessLifetimes, s.cluster.RemoteOperationLifetime("files", sourceHostID))
	}
	if hostID != "" && hostID != cluster.LocalHostID {
		hostAccessLifetimes = append(hostAccessLifetimes, s.cluster.RemoteOperationLifetime("files", hostID))
	}
	fail := func(status int, code, detail string) {
		_ = s.audit(r, actorID, "file.transfer.copy", "directory", input.TargetDirectory, "failure", map[string]any{
			"sourceNodeId": input.SourceNodeID, "sourcePath": input.Path, "targetHostId": hostID, "errorCode": code,
		})
		s.writeProblem(w, r, status, code, detail, "")
	}
	if accessLifetime != nil && accessLifetime.Err() != nil {
		fail(http.StatusForbidden, "remote_files_disabled", "当前中心已禁用远程文件管理。")
		return
	}
	for _, lifetime := range hostAccessLifetimes {
		if lifetime.Err() != nil {
			fail(403, "remote_files_disabled", "当前中心已禁用该主机的远程文件管理。")
			return
		}
	}
	if s.remoteDownloadJobs == nil || !s.remoteDownloadJobs.Available() {
		fail(http.StatusServiceUnavailable, "remote_download_jobs_unavailable", "文件传输任务暂不可用")
		return
	}
	now := time.Now().UTC()
	if input.ConflictPolicies != nil {
		if inherited := s.remoteDownloadJobs.BatchPolicies(input.BatchID, hostID, input.TargetDirectory); inherited != nil {
			input.ConflictPolicies = inherited
		}
	}
	job := contract.FileRemoteDownloadJob{ConflictPolicies: input.ConflictPolicies, BatchID: input.BatchID, ID: newRequestID(), State: "queued", Source: "kpanel://" + input.SourceNodeID,
		SourceKind: "cross-host", TargetHostID: hostID, TargetDirectory: input.TargetDirectory, Name: path.Base(input.Path), CreatedAt: now, UpdatedAt: now}
	ctx, cancel, ok := s.reserveRemoteDownloadJob(job.ID)
	if !ok {
		fail(http.StatusTooManyRequests, "remote_download_queue_full", "文件传输队列已满，请稍后重试")
		return
	}
	if err := s.remoteDownloadJobs.Create(job); err != nil {
		s.releaseRemoteDownloadReservation(job.ID, cancel)
		s.remoteDownloadWG.Done()
		fail(http.StatusServiceUnavailable, "remote_download_jobs_unavailable", "文件传输任务暂不可用")
		return
	}
	requestID := requestID(r)
	task := fileRemoteDownloadTask{job: job, actorID: actorID, sourceIP: s.remoteIP(r), requestID: requestID,
		execute: func(ctx context.Context, emit func(contract.FileTransferEvent) bool) contract.FileTransferEvent {
			return s.executeCrossFileTransfer(ctx, input, hostID, kind, requestID, emit)
		}}
	task.accessLifetime = accessLifetime
	task.hostAccessLifetimes = hostAccessLifetimes
	go s.runFileRemoteDownloadJob(ctx, cancel, task)
	s.writeJSON(w, http.StatusAccepted, job)
}

func (s *Server) executeCrossFileTransfer(ctx context.Context, input contract.FileTransferRequest, targetHostID string, targetKind cluster.HostKind, requestID string, emit func(contract.FileTransferEvent) bool) contract.FileTransferEvent {
	// Bind the whole job, including local receive publication, to the policy.
	if input.SourceNodeID != s.cluster.NodeID() || targetHostID != "" && targetHostID != cluster.LocalHostID {
		operation, cancel, err := s.cluster.BeginRemoteOperation(ctx, "files")
		if err != nil {
			event := contract.FileTransferEvent{State: "error", Code: "remote_files_disabled", Detail: "当前中心已禁用远程文件管理。"}
			emit(event)
			return event
		}
		defer cancel()
		ctx = operation
	}
	sourceHostID := input.SourceNodeID
	if sourceHostID != s.cluster.NodeID() {
		var err error
		sourceHostID, err = s.cluster.FileSourceHostID(input.SourceNodeID)
		if err != nil {
			event := contract.FileTransferEvent{State: "error", Code: "source_unavailable", Detail: "来源主机无效。"}
			emit(event)
			return event
		}
	}
	for _, id := range []string{sourceHostID, targetHostID} {
		if id == "" || id == cluster.LocalHostID || id == s.cluster.NodeID() {
			continue
		}
		operation, cancel, err := s.cluster.BeginRemoteOperation(ctx, "files", id)
		if err != nil {
			event := contract.FileTransferEvent{State: "error", Code: "remote_files_disabled", Detail: "当前中心已禁用该主机的远程文件管理。"}
			emit(event)
			return event
		}
		defer cancel()
		ctx = operation
	}
	fail := func(code, detail string, loaded int64) contract.FileTransferEvent {
		if ctx.Err() != nil {
			code, detail = "remote_download_cancelled", "文件传输已取消。"
		}
		event := contract.FileTransferEvent{State: "error", Code: code, Detail: detail, LoadedBytes: loaded}
		emit(event)
		return event
	}
	if !emit(contract.FileTransferEvent{State: "connecting"}) {
		return fail("file_transfer_cancelled", "文件传输已取消。", 0)
	}
	var err error
	if targetHostID == "" {
		err = s.ensureFileTransferDirectory(ctx, input.TargetDirectory, requestID)
	} else {
		err = s.ensureFileHostTransferDirectory(ctx, targetHostID, targetKind, input.TargetDirectory)
	}
	if err != nil {
		return fail("target_unavailable", "目标目录不存在或不可写。", 0)
	}
	options := cluster.FederationFileOpenRequest{Path: input.Path, ResourceVersion: input.ResourceVersion, TransferVersion: 1}
	body, metadata, err := s.openCrossTransferSource(ctx, input, options, requestID)
	if err != nil {
		return fail("source_unavailable", "无法连接来源主机，或配对未授权文件复制。", 0)
	}
	defer body.Close()
	if metadata.Name != path.Base(input.Path) || metadata.ResourceVersion != input.ResourceVersion || metadata.Offset != 0 {
		return fail("source_changed", "来源文件在拖拽后已发生变化。", 0)
	}
	name := metadata.Name
	choice := contract.FileConflictResolution{Name: name}
	if input.ConflictPolicies != nil && metadata.Kind == "file" {
		var closed bool
		choice, closed, err = s.resolveTransferConflict(ctx, contract.FileEntry{Name: metadata.Name, Kind: metadata.Kind, SizeBytes: metadata.SizeBytes}, input.TargetDirectory, targetHostID, targetKind, requestID, false, func() { _ = body.Close() })
		if err == nil && choice.Skipped {
			event := contract.FileTransferEvent{State: "skipped", Name: name}
			emit(event)
			return event
		}
		if err == nil && closed {
			var next contract.FileTransferMetadata
			body, next, err = s.openCrossTransferSource(ctx, input, options, requestID)
			if err == nil && (next.ResourceVersion != metadata.ResourceVersion || next.Name != metadata.Name || next.Kind != metadata.Kind || next.SizeBytes != metadata.SizeBytes) {
				body.Close()
				err = remotedownload.ErrSourceChanged
			}
		}
		name = choice.Name
	} else if input.ConflictPolicies == nil {
		if targetHostID == "" {
			name, err = s.uniqueFileTransferName(ctx, input.TargetDirectory, metadata.Name, requestID)
		} else {
			name, err = s.uniqueFileHostTransferName(ctx, targetHostID, targetKind, input.TargetDirectory, metadata.Name, requestID)
		}
	}
	if err != nil {
		return fail("target_name_unavailable", "无法确定目标文件名。", 0)
	}
	size := metadata.SizeBytes
	if metadata.Kind == "directory" {
		size = -1
	}
	receive := contract.FileReceiveInput{RequestedName: metadata.Name, Directory: input.TargetDirectory, Name: name, Kind: metadata.Kind, SizeBytes: size,
		Overwrite: choice.Overwrite, ExpectedTargetResourceVersion: choice.Version, SourceKey: transferSourceKey(input.SourceNodeID + "\n" + input.Path + "\n" + input.ResourceVersion + "\n" + name + "\n" + choice.Version), Mode: metadata.Mode, ModifiedAt: metadata.ModifiedAt}
	if input.ConflictPolicies != nil && metadata.Kind == "directory" {
		p := filemanager.FileConflicts(ctx).Policies
		receive.ConflictPolicies = &p
	}
	reopen := func(ctx context.Context, offset int64, prefix string) (io.ReadCloser, error) {
		request := options
		if metadata.Kind == "file" {
			request.Offset = offset
		}
		next, nextMetadata, err := s.openCrossTransferSource(ctx, input, request, requestID)
		if err != nil {
			return nil, err
		}
		ok := false
		defer func() {
			if !ok {
				_ = next.Close()
			}
		}()
		if nextMetadata.Name != metadata.Name || nextMetadata.Kind != metadata.Kind || nextMetadata.ResourceVersion != metadata.ResourceVersion || nextMetadata.SizeBytes != metadata.SizeBytes {
			return nil, remotedownload.ErrSourceChanged
		}
		if metadata.Kind == "file" && nextMetadata.TransferVersion == 1 {
			if nextMetadata.Offset != offset {
				return nil, remotedownload.ErrSourceChanged
			}
		} else {
			if nextMetadata.Offset != 0 {
				return nil, remotedownload.ErrSourceChanged
			}
			if err := discardTransferPrefix(ctx, next, offset, prefix); err != nil {
				return nil, err
			}
		}
		ok = true
		return next, nil
	}
	entry, loaded, err := s.receiveFileTransfer(ctx, receive, body, reopen, s.fileReceiver(targetHostID, targetKind, requestID), emit, func(body io.ReadCloser) (contract.FileEntry, error) {
		if input.ConflictPolicies != nil {
			return contract.FileEntry{}, errReceiveUnsupported
		}
		return s.importLegacyCrossTransfer(ctx, input.TargetDirectory, name, metadata, body, targetHostID, targetKind, requestID, emit)
	})
	if errors.Is(err, filemanager.ErrFileSkipped) {
		event := contract.FileTransferEvent{State: "skipped", Name: name}
		emit(event)
		return event
	}
	var partial *partialFileTransfer
	if errors.As(err, &partial) {
		event := contract.FileTransferEvent{State: "partial", Name: partial.entry.Name, Entry: &partial.entry, LoadedBytes: loaded, TotalBytes: max(size, 0), Result: partial.result}
		emit(event)
		return event
	}
	if err != nil {
		if errors.Is(err, remotedownload.ErrSourceChanged) {
			return fail("source_changed", "来源文件在续传期间发生变化，已停止拼接。", loaded)
		}
		return fail("file_transfer_failed", "文件传输中断或校验失败，未发布半成品。", loaded)
	}
	event := contract.FileTransferEvent{State: "complete", Entry: &entry, Name: entry.Name, LoadedBytes: loaded, TotalBytes: max(size, 0)}
	emit(event)
	return event
}

func (s *Server) importLegacyCrossTransfer(ctx context.Context, directory, name string, metadata contract.FileTransferMetadata, body io.ReadCloser, targetHostID string, kind cluster.HostKind, requestID string, emit func(contract.FileTransferEvent) bool) (contract.FileEntry, error) {
	query := url.Values{"path": {directory}, "name": {name}, "kind": {metadata.Kind}, "size": {strconv.FormatInt(metadata.SizeBytes, 10)}}
	var loaded int64
	tracked := &fileTransferProgressReader{source: body, report: func(count int64) {
		loaded += count
		emit(contract.FileTransferEvent{State: "transferring", Name: name, LoadedBytes: loaded, TotalBytes: metadata.SizeBytes})
	}}
	var response *http.Response
	var err error
	if targetHostID != "" {
		response, err = s.openFileHostRequest(ctx, targetHostID, kind, cluster.LightFileRequest{Method: http.MethodPost, Path: "/v1/files/transfer/import", RawQuery: query.Encode(), Headers: map[string]string{"Content-Type": "application/octet-stream"}, Body: tracked, BodyLength: -1})
	} else {
		streamer, ok := s.agent.(agentStreamAPI)
		if !ok {
			return contract.FileEntry{}, errors.New("Agent stream unavailable")
		}
		response, err = streamer.OpenStream(ctx, http.MethodPost, "/v1/files/transfer/import", query.Encode(), requestID, tracked, http.Header{"Content-Type": {"application/octet-stream"}}, -1)
	}
	if err != nil {
		return contract.FileEntry{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return contract.FileEntry{}, &fileReceiveHTTPError{status: response.StatusCode}
	}
	var entry contract.FileEntry
	decoder := json.NewDecoder(io.LimitReader(response.Body, 1<<20))
	decoder.DisallowUnknownFields()
	var extra any
	if decoder.Decode(&entry) != nil || decoder.Decode(&extra) != io.EOF || entry.Path != path.Join(directory, name) || entry.Kind != metadata.Kind || metadata.Kind == "file" && entry.SizeBytes != metadata.SizeBytes {
		return contract.FileEntry{}, errors.New("legacy receiver result invalid")
	}
	return entry, nil
}
