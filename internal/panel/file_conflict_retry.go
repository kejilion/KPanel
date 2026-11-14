package panel

import (
	"context"
	"errors"
	"io"

	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/filemanager"
)

// Retrying a target conflict is a new decision and a new receive identity.
// Transport retries within one receive retain the old durable checkpoint.
func (s *Server) receiveFileTransfer(ctx context.Context, input contract.FileReceiveInput, initial io.ReadCloser, reopen func(context.Context, int64, string) (io.ReadCloser, error), target fileReceiveTarget, emit func(contract.FileTransferEvent) bool, legacy func(io.ReadCloser) (contract.FileEntry, error)) (contract.FileEntry, int64, error) {
	body := initial
	name := input.RequestedName
	if name == "" {
		name = input.Name
	}
	baseKey := input.SourceKey
	var loaded int64
	for attempt := 0; attempt < 8; attempt++ {
		entry, count, err := s.receiveFileTransferOnce(ctx, input, body, reopen, target, emit, legacy)
		loaded = count
		var receiver *fileReceiveHTTPError
		if err == nil || !errors.As(err, &receiver) || receiver.status != 409 || input.Kind != "file" || filemanager.FileConflicts(ctx) == nil || target.lookup == nil || reopen == nil {
			return entry, count, err
		}
		source := contract.FileEntry{Name: name, Kind: "file", SizeBytes: input.SizeBytes}
		if input.ModifiedAt != nil {
			source.ModifiedAt = *input.ModifiedAt
		}
		choice, err := filemanager.ResolveFileConflict(ctx, source, input.Directory, filemanager.FileConflicts(ctx), true, target.lookup)
		if err != nil {
			return contract.FileEntry{}, loaded, err
		}
		if choice.Skipped {
			return contract.FileEntry{}, loaded, filemanager.ErrFileSkipped
		}
		input.Name = choice.Name
		input.Overwrite = choice.Overwrite
		input.ExpectedTargetResourceVersion = choice.Version
		input.SourceKey = transferSourceKey(baseKey + "\n" + choice.Name + "\n" + choice.Version)
		body, err = reopen(ctx, 0, transferSourceKey(""))
		if err != nil {
			return contract.FileEntry{}, loaded, err
		}
	}
	if body != nil {
		body.Close()
	}
	return contract.FileEntry{}, loaded, filemanager.ErrConflict
}
