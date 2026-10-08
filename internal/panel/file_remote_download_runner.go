package panel

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"sync/atomic"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/filemanager"
)

// executeFileRemoteDownload is the single transfer path shared by request-scoped
// compatibility downloads and background jobs. Authentication, persistence and
// audit stay with their callers; URL policy and Agent atomic upload remain here.
func (s *Server) executeLegacyFileRemoteDownload(
	ctx context.Context,
	input contract.FileRemoteDownloadRequest,
	requestID string,
	emit func(contract.FileTransferEvent) bool,
	initial *http.Response,
) contract.FileTransferEvent {
	fail := func(code, detail string, loaded, total int64, name string) contract.FileTransferEvent {
		event := contract.FileTransferEvent{
			State: "error", LoadedBytes: loaded, TotalBytes: total, Name: name, Code: code, Detail: detail,
		}
		emit(event)
		return event
	}
	if err := s.ensureFileTransferDirectory(ctx, input.TargetDirectory, requestID); err != nil {
		if ctx.Err() != nil {
			code, detail := fileRemoteDownloadError(ctx.Err())
			return fail(code, detail, 0, 0, input.Name)
		}
		return fail("target_unavailable", "目标目录不存在或不可写。", 0, 0, input.Name)
	}
	if s.remoteDownloadOpen == nil {
		return fail("remote_download_unavailable", "远程下载服务不可用。", 0, 0, input.Name)
	}
	response := initial
	var err error
	if response == nil {
		response, err = s.remoteDownloadOpen(ctx, input.URL)
	}
	if err != nil {
		code, detail := fileRemoteDownloadError(err)
		return fail(code, detail, 0, 0, input.Name)
	}
	if response == nil || response.Body == nil {
		return fail("remote_download_unavailable", "远程下载服务不可用。", 0, 0, input.Name)
	}
	limited := &fileRemoteDownloadLimitReader{source: response.Body, remaining: filemanager.MaxUploadBytes}
	var loaded atomic.Int64
	uploadBody := &fileRemoteDownloadUploadBody{source: limited, closer: response.Body, loaded: &loaded}
	defer uploadBody.Close()
	if response.ContentLength > filemanager.MaxUploadBytes {
		return fail("remote_download_too_large", "远程文件超过 512 MiB。", 0, response.ContentLength, input.Name)
	}
	totalBytes := response.ContentLength
	if totalBytes < 0 {
		totalBytes = 0
	}
	name := fileRemoteDownloadName(input.Name, response)
	name, err = s.uniqueFileTransferName(ctx, input.TargetDirectory, name, requestID)
	if err != nil {
		if ctx.Err() != nil {
			code, detail := fileRemoteDownloadError(ctx.Err())
			return fail(code, detail, 0, totalBytes, name)
		}
		return fail("target_name_unavailable", "无法确定可用的保存名称。", 0, totalBytes, name)
	}
	if !emit(contract.FileTransferEvent{State: "connecting", Name: name, TotalBytes: totalBytes}) {
		return contract.FileTransferEvent{State: "error", Name: name, TotalBytes: totalBytes, Code: "remote_download_cancelled"}
	}
	if !emit(contract.FileTransferEvent{State: "transferring", TotalBytes: totalBytes, Name: name}) {
		return contract.FileTransferEvent{State: "error", Name: name, TotalBytes: totalBytes, Code: "remote_download_cancelled"}
	}
	streamer, ok := s.agent.(agentStreamAPI)
	if !ok {
		return fail("agent_stream_unavailable", "Agent 文件流不可用。", 0, totalBytes, name)
	}
	query := url.Values{
		"path": []string{input.TargetDirectory}, "name": []string{name}, "overwrite": []string{"false"},
	}
	headers := make(http.Header)
	headers.Set("Content-Type", "application/octet-stream")
	agentContentLength := response.ContentLength
	if agentContentLength < 0 {
		agentContentLength = -1
	}
	type streamResult struct {
		response *http.Response
		err      error
	}
	resultChannel := make(chan streamResult)
	go func() {
		agentResponse, streamError := streamer.OpenStream(
			ctx, http.MethodPost, "/v1/files/upload", query.Encode(), requestID,
			uploadBody, headers, agentContentLength,
		)
		result := streamResult{response: agentResponse, err: streamError}
		select {
		case resultChannel <- result:
		case <-ctx.Done():
			if result.response != nil && result.response.Body != nil {
				_ = result.response.Body.Close()
			}
		}
	}()
	progressTicker := time.NewTicker(180 * time.Millisecond)
	defer progressTicker.Stop()
	reportedBytes := int64(0)
	lastProgressEvent := time.Now()
	var agentResponse *http.Response
	streamFinished := false
	for !streamFinished {
		select {
		case result := <-resultChannel:
			agentResponse, err = result.response, result.err
			streamFinished = true
			_ = uploadBody.Close()
		case <-progressTicker.C:
			current := loaded.Load()
			if current != reportedBytes || time.Since(lastProgressEvent) >= 10*time.Second {
				if !emit(contract.FileTransferEvent{
					State: "transferring", LoadedBytes: current, TotalBytes: totalBytes, Name: name,
				}) {
					_ = uploadBody.Close()
					return contract.FileTransferEvent{
						State: "error", LoadedBytes: current, TotalBytes: totalBytes,
						Name: name, Code: "remote_download_cancelled",
					}
				}
				reportedBytes = current
				lastProgressEvent = time.Now()
			}
		case <-ctx.Done():
			_ = uploadBody.Close()
			current := loaded.Load()
			code, detail := fileRemoteDownloadError(ctx.Err())
			return fail(code, detail, current, totalBytes, name)
		}
	}
	loadedBytes := loaded.Load()
	if err != nil {
		if limited.exceeded.Load() {
			return fail("remote_download_too_large", "远程文件超过 512 MiB，传输已停止；请刷新目录确认结果。", loadedBytes, totalBytes, name)
		}
		code, detail := fileRemoteDownloadError(err)
		if code == "remote_download_unreachable" {
			code = "agent_write_interrupted"
			detail = "目标 Agent 写入中断；请刷新目录确认结果。"
		}
		return fail(code, detail, loadedBytes, totalBytes, name)
	}
	if agentResponse == nil {
		return fail("agent_write_failed", "目标文件写入结果未知；请刷新目录确认。", loadedBytes, totalBytes, name)
	}
	defer agentResponse.Body.Close()
	stopClosingAgentResponse := context.AfterFunc(ctx, func() { _ = agentResponse.Body.Close() })
	defer stopClosingAgentResponse()
	if agentResponse.StatusCode < http.StatusOK || agentResponse.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(agentResponse.Body, 1<<20))
		switch {
		case limited.exceeded.Load() || agentResponse.StatusCode == http.StatusRequestEntityTooLarge:
			return fail("remote_download_too_large", "远程文件超过 512 MiB，传输已停止；请刷新目录确认结果。", loadedBytes, totalBytes, name)
		case agentResponse.StatusCode == http.StatusConflict:
			return fail("target_conflict", "保存名称刚被占用，请重试。", loadedBytes, totalBytes, name)
		case agentResponse.StatusCode == http.StatusTooManyRequests:
			return fail("agent_write_busy", "目标 Agent 当前繁忙，请稍后重试。", loadedBytes, totalBytes, name)
		case agentResponse.StatusCode == http.StatusForbidden:
			return fail("target_permission_denied", "目标目录不再允许写入；请刷新目录确认结果。", loadedBytes, totalBytes, name)
		case agentResponse.StatusCode == http.StatusNotFound:
			return fail("target_unavailable", "目标目录已不存在；请刷新目录确认结果。", loadedBytes, totalBytes, name)
		case agentResponse.StatusCode == http.StatusInsufficientStorage:
			return fail("target_storage_full", "目标磁盘空间或配额不足；请刷新目录确认结果。", loadedBytes, totalBytes, name)
		default:
			return fail("agent_write_failed", "目标文件写入失败；请刷新目录确认结果。", loadedBytes, totalBytes, name)
		}
	}
	if !emit(contract.FileTransferEvent{
		State: "confirming", LoadedBytes: loadedBytes, TotalBytes: totalBytes, Name: name,
	}) {
		return contract.FileTransferEvent{
			State: "error", LoadedBytes: loadedBytes, TotalBytes: totalBytes,
			Name: name, Code: "agent_write_interrupted",
		}
	}
	var entry contract.FileEntry
	decoder := json.NewDecoder(io.LimitReader(agentResponse.Body, 1<<20))
	decoder.DisallowUnknownFields()
	var extra any
	decodeError := decoder.Decode(&entry)
	extraError := decoder.Decode(&extra)
	if ctx.Err() != nil {
		code, detail := fileRemoteDownloadError(ctx.Err())
		return fail(code, detail, loadedBytes, totalBytes, name)
	}
	if decodeError != nil || !errors.Is(extraError, io.EOF) ||
		!validFileRemoteDownloadEntry(entry, input.TargetDirectory, name, loadedBytes) {
		return fail("agent_result_invalid", "目标 Agent 返回无效结果，请刷新目录确认。", loadedBytes, totalBytes, name)
	}
	result := contract.FileTransferEvent{
		State: "complete", LoadedBytes: loadedBytes, TotalBytes: totalBytes, Name: name, Entry: &entry,
	}
	emit(result)
	return result
}
