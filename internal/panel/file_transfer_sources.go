package panel

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/kejilion/kejilion-panel/internal/cluster"
	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/remotedownload"
)

func transferSourceKey(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func discardTransferPrefix(ctx context.Context, body io.ReadCloser, offset int64, expected string) error {
	stop := context.AfterFunc(ctx, func() { _ = body.Close() })
	defer stop()
	hasher := sha256.New()
	reader := &fileTransferContextReader{ctx, body}
	written, err := io.CopyBuffer(hasher, io.LimitReader(reader, offset), make([]byte, 64<<10))
	if err != nil {
		return err
	}
	if written != offset || hex.EncodeToString(hasher.Sum(nil)) != expected {
		return remotedownload.ErrSourceChanged
	}
	return nil
}

type fileTransferContextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *fileTransferContextReader) Read(data []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(data)
}

func (s *Server) executeFileRemoteDownload(ctx context.Context, input contract.FileRemoteDownloadRequest, requestID string, emit func(contract.FileTransferEvent) bool) contract.FileTransferEvent {
	fail := func(err error, loaded, total int64, name string) contract.FileTransferEvent {
		code, detail := fileRemoteDownloadError(err)
		var receiver *fileReceiveHTTPError
		if errors.As(err, &receiver) {
			code, detail = "agent_write_failed", "目标文件接收失败，未发布半成品。"
			switch receiver.status {
			case 409:
				code, detail = "target_conflict", "来源校验或目标状态发生变化，请刷新后重试。"
			case 413:
				code, detail = "remote_download_too_large", "远程文件超过 10 GiB。"
			case 429:
				code, detail = "agent_write_busy", "目标 Agent 当前繁忙，请稍后重试。"
			case 507:
				code, detail = "target_storage_full", "目标磁盘空间或配额不足。"
			case 403:
				code, detail = "target_permission_denied", "目标目录不再允许写入。"
			}
		}
		event := contract.FileTransferEvent{State: "error", Code: code, Detail: detail, Name: name, LoadedBytes: loaded, TotalBytes: total}
		emit(event)
		return event
	}
	if err := s.ensureFileTransferDirectory(ctx, input.TargetDirectory, requestID); err != nil {
		if ctx.Err() != nil {
			return fail(ctx.Err(), 0, 0, input.Name)
		}
		event := contract.FileTransferEvent{State: "error", Code: "target_unavailable", Detail: "目标目录不存在或不可写。"}
		emit(event)
		return event
	}
	if s.remoteDownloadOpen == nil {
		return fail(remotedownload.ErrUnreachable, 0, 0, input.Name)
	}
	response, err := s.remoteDownloadOpen(ctx, input.URL)
	if err != nil {
		return fail(err, 0, 0, input.Name)
	}
	if response == nil || response.Body == nil {
		return fail(remotedownload.ErrUnreachable, 0, 0, input.Name)
	}
	defer response.Body.Close()
	if response.ContentLength > contract.MaxFileTransferBytes {
		event := contract.FileTransferEvent{State: "error", Code: "remote_download_too_large", Detail: "远程文件超过 10 GiB。"}
		emit(event)
		return event
	}
	name, err := s.uniqueFileTransferName(ctx, input.TargetDirectory, fileRemoteDownloadName(input.Name, response), requestID)
	if err != nil {
		return fail(err, 0, 0, input.Name)
	}
	total := max(response.ContentLength, 0)
	if !emit(contract.FileTransferEvent{State: "connecting", Name: name, TotalBytes: total}) {
		return fail(context.Canceled, 0, total, name)
	}
	if !emit(contract.FileTransferEvent{State: "transferring", Name: name, TotalBytes: total}) {
		return fail(context.Canceled, 0, total, name)
	}
	validator := response.Header.Get("ETag")
	modified := response.Header.Get("Last-Modified")
	finalURL := ""
	if response.Request != nil && response.Request.URL != nil {
		finalURL = response.Request.URL.String()
	}
	reopen := func(ctx context.Context, offset int64, prefix string) (io.ReadCloser, error) {
		var next *http.Response
		var err error
		if offset > 0 && response.ContentLength >= offset && remotedownload.StrongETag(validator) && s.remoteDownloadRangeOpen != nil {
			next, err = s.remoteDownloadRangeOpen(ctx, input.URL, remotedownload.ResumeRequest{Offset: offset, SizeBytes: response.ContentLength, ETag: validator, FinalURL: finalURL})
		} else {
			next, err = s.remoteDownloadOpen(ctx, input.URL)
		}
		if err != nil {
			return nil, err
		}
		if next == nil || next.Body == nil {
			return nil, remotedownload.ErrUnreachable
		}
		ok := false
		defer func() {
			if !ok {
				_ = next.Body.Close()
			}
		}()
		if validator != "" && next.Header.Get("ETag") != validator || modified != "" && next.Header.Get("Last-Modified") != modified ||
			finalURL != "" && (next.Request == nil || next.Request.URL == nil || next.Request.URL.String() != finalURL) {
			return nil, remotedownload.ErrSourceChanged
		}
		if next.StatusCode == http.StatusOK {
			if response.ContentLength >= 0 && next.ContentLength != response.ContentLength {
				return nil, remotedownload.ErrSourceChanged
			}
			if err := discardTransferPrefix(ctx, next.Body, offset, prefix); err != nil {
				return nil, err
			}
		} else if next.StatusCode != http.StatusPartialContent {
			return nil, remotedownload.ErrSourceChanged
		}
		ok = true
		return next.Body, nil
	}
	receive := contract.FileReceiveInput{Directory: input.TargetDirectory, Name: name, Kind: "file", SizeBytes: response.ContentLength,
		SourceKey: transferSourceKey(input.URL + "\n" + validator + "\n" + modified + "\n" + strconv.FormatInt(response.ContentLength, 10))}
	adaptEvent := func(event contract.FileTransferEvent) bool {
		if event.State == "committing" {
			event.State = "confirming"
		}
		return emit(event)
	}
	var legacyEvent *contract.FileTransferEvent
	entry, loaded, err := s.receiveFileTransfer(ctx, receive, response.Body, reopen, s.fileReceiver("", "", requestID), adaptEvent, func(body io.ReadCloser) (contract.FileEntry, error) {
		legacy := s.executeLegacyFileRemoteDownload(ctx, input, requestID, emit, response)
		legacyEvent = &legacy
		if legacy.Entry != nil && legacy.State == "complete" {
			return *legacy.Entry, nil
		}
		return contract.FileEntry{}, &fileTransferEventError{legacy}
	})
	if legacyEvent != nil {
		return *legacyEvent
	}
	if err != nil {
		var legacy *fileTransferEventError
		if errors.As(err, &legacy) {
			return legacy.event
		}
		return fail(err, loaded, total, name)
	}
	event := contract.FileTransferEvent{State: "complete", Name: entry.Name, Entry: &entry, LoadedBytes: loaded, TotalBytes: total}
	emit(event)
	return event
}

type fileTransferEventError struct{ event contract.FileTransferEvent }

func (e *fileTransferEventError) Error() string { return e.event.Code }

func (s *Server) openCrossTransferSource(ctx context.Context, input contract.FileTransferRequest, options cluster.FederationFileOpenRequest, requestID string) (io.ReadCloser, contract.FileTransferMetadata, error) {
	open := func(options cluster.FederationFileOpenRequest) (io.ReadCloser, contract.FileTransferMetadata, error) {
		if input.SourceNodeID == s.cluster.NodeID() {
			return s.openLocalFileTransfer(ctx, options, requestID)
		}
		host, err := s.cluster.Host(ctx, input.SourceNodeID)
		if err == nil && host.Kind == cluster.HostKindLightNode {
			return s.cluster.OpenLightFileTransfer(ctx, input.SourceNodeID, options)
		}
		return s.cluster.OpenRemoteFileV2(ctx, input.SourceNodeID, options)
	}
	body, metadata, err := open(options)
	if err != nil && options.TransferVersion == 1 && ctx.Err() == nil {
		// Older Agents/Peers reject optional fields. Restart their authenticated
		// stream and verify its prefix before retaining any existing checkpoint.
		options.TransferVersion, options.Offset = 0, 0
		return open(options)
	}
	return body, metadata, err
}
