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
	"github.com/kejilion/kejilion-panel/internal/remotedownload"
)

func (s *Server) startCrossFileTransferJob(w http.ResponseWriter, r *http.Request, input contract.FileTransferRequest, hostID string, kind cluster.HostKind, actorID string) {
	fail := func(status int, code, detail string) {
		_ = s.audit(r, actorID, "file.transfer.copy", "directory", input.TargetDirectory, "failure", map[string]any{
			"sourceNodeId": input.SourceNodeID, "sourcePath": input.Path, "targetHostId": hostID, "errorCode": code,
		})
		s.writeProblem(w, r, status, code, detail, "")
	}
	if s.remoteDownloadJobs == nil || !s.remoteDownloadJobs.Available() {
		fail(http.StatusServiceUnavailable, "remote_download_jobs_unavailable", "文件传输任务暂不可用")
		return
	}
	now := time.Now().UTC()
	job := contract.FileRemoteDownloadJob{ID: newRequestID(), State: "queued", Source: "kpanel://" + input.SourceNodeID,
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
	go s.runFileRemoteDownloadJob(ctx, cancel, task)
	s.writeJSON(w, http.StatusAccepted, job)
}

func (s *Server) executeCrossFileTransfer(ctx context.Context, input contract.FileTransferRequest, targetHostID string, targetKind cluster.HostKind, requestID string, emit func(contract.FileTransferEvent) bool) contract.FileTransferEvent {
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
	name := ""
	if targetHostID == "" {
		name, err = s.uniqueFileTransferName(ctx, input.TargetDirectory, metadata.Name, requestID)
	} else {
		name, err = s.uniqueFileHostTransferName(ctx, targetHostID, targetKind, input.TargetDirectory, metadata.Name, requestID)
	}
	if err != nil {
		return fail("target_name_unavailable", "无法确定目标文件名。", 0)
	}
	size := metadata.SizeBytes
	if metadata.Kind == "directory" {
		size = -1
	}
	receive := contract.FileReceiveInput{Directory: input.TargetDirectory, Name: name, Kind: metadata.Kind, SizeBytes: size,
		SourceKey: transferSourceKey(input.SourceNodeID + "\n" + input.Path + "\n" + input.ResourceVersion), Mode: metadata.Mode, ModifiedAt: metadata.ModifiedAt}
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
		return s.importLegacyCrossTransfer(ctx, input.TargetDirectory, name, metadata, body, targetHostID, targetKind, requestID, emit)
	})
	if err != nil {
		if errors.Is(err, remotedownload.ErrSourceChanged) {
			return fail("source_changed", "来源文件在续传期间发生变化，已停止拼接。", loaded)
		}
		return fail("file_transfer_failed", "文件传输中断或校验失败，未发布半成品。", loaded)
	}
	event := contract.FileTransferEvent{State: "complete", Entry: &entry, Name: name, LoadedBytes: loaded, TotalBytes: max(size, 0)}
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
