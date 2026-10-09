package panel

import (
	"context"
	"errors"
	"io"
	"strings"

	"github.com/kejilion/kejilion-panel/internal/bittorrent"
	"github.com/kejilion/kejilion-panel/internal/contract"
)

func (s *Server) executeTorrentDownload(ctx context.Context, input contract.FileRemoteDownloadRequest, requestID string, emit func(contract.FileTransferEvent) bool) contract.FileTransferEvent {
	fail := func(err error) contract.FileTransferEvent {
		code, detail := fileRemoteDownloadError(err)
		var receiver *fileReceiveHTTPError
		if errors.As(err, &receiver) {
			code, detail = "agent_write_failed", "目标接收失败，未发布半成品。"
			if receiver.status == 507 {
				code, detail = "target_storage_full", "目标磁盘空间或配额不足。"
			}
			if receiver.status == 409 {
				code, detail = "target_conflict", "目标状态发生变化，请刷新后重试。"
			}
		}
		if errors.Is(err, errReceiveUnsupported) {
			code, detail = "target_receive_unsupported", "请更新目标 Agent 后重试。"
		}
		event := contract.FileTransferEvent{State: "error", Code: code, Detail: detail}
		emit(event)
		return event
	}
	if s.btDownload == nil {
		return fail(bittorrent.ErrStorage)
	}
	if err := s.ensureFileTransferDirectory(ctx, input.TargetDirectory, requestID); err != nil {
		return fail(err)
	}
	if err := s.fileReceiveSupport(ctx, "", "", input.TargetDirectory, requestID); err != nil {
		return fail(err)
	}
	if !emit(contract.FileTransferEvent{State: "connecting", Name: input.Name}) {
		return fail(context.Canceled)
	}
	data := input.Torrent
	raw := input.URL
	if len(data) == 0 && !strings.HasPrefix(raw, "magnet:") {
		response, err := s.remoteDownloadOpen(ctx, raw)
		if err != nil {
			return fail(err)
		}
		if response == nil || response.Body == nil {
			return fail(bittorrent.ErrMetadata)
		}
		defer response.Body.Close()
		data, err = io.ReadAll(io.LimitReader(response.Body, bittorrent.MaxMetadataBytes+1))
		if err != nil {
			return fail(err)
		}
		raw = ""
	}
	source, err := bittorrent.Parse(raw, data)
	if err != nil {
		return fail(err)
	}
	result, err := s.btDownload(ctx, source, input.Acceleration != "off", func(p bittorrent.Progress) bool {
		return emit(contract.FileTransferEvent{State: "transferring", Name: p.Name, SourceBytes: p.Bytes, TotalBytes: p.Total, Peers: p.Peers, SpeedBytes: p.Speed, TransferMode: p.Mode})
	})
	if err != nil {
		return fail(err)
	}
	defer result.Close()
	name := input.Name
	if name == "" {
		name = result.Metadata.Name
	}
	name, err = s.uniqueFileTransferName(ctx, input.TargetDirectory, name, requestID)
	if err != nil {
		return fail(err)
	}
	kind, size := "file", result.Metadata.Size
	if result.Directory() {
		kind, size = "directory", -1
	}
	receive := contract.FileReceiveInput{Directory: input.TargetDirectory, Name: name, Kind: kind, SizeBytes: size, SourceKey: transferSourceKey("bittorrent:" + result.Metadata.Hash.HexString())}
	reopen := func(ctx context.Context, offset int64, prefix string) (io.ReadCloser, error) {
		body := result.Open(ctx)
		if err := discardTransferPrefix(ctx, body, offset, prefix); err != nil {
			body.Close()
			return nil, err
		}
		return body, nil
	}
	adapt := func(event contract.FileTransferEvent) bool {
		event.TransferMode = "bt-publish"
		event.SourceBytes = result.Metadata.Size
		event.TotalBytes = result.Metadata.Size
		if event.State == "committing" {
			event.State = "confirming"
		}
		return emit(event)
	}
	entry, loaded, err := s.receiveFileTransfer(ctx, receive, result.Open(ctx), reopen, s.fileReceiver("", "", requestID), adapt, nil)
	if err != nil {
		return fail(err)
	}
	event := contract.FileTransferEvent{State: "complete", Name: entry.Name, Entry: &entry, LoadedBytes: loaded, TotalBytes: result.Metadata.Size, SourceBytes: result.Metadata.Size}
	emit(event)
	return event
}
