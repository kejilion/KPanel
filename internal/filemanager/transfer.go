package filemanager

import (
	"archive/tar"
	"archive/zip"
	"context"
	"errors"
	"io"
	"os"
	"path"
	"strconv"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

// ExportZIP streams one or more regular files or directories as a restricted
// ZIP archive. It never creates a server-side archive file and reuses the same
// traversal, version, concurrency, and resource budgets as regular downloads.
func (m *Manager) ExportZIP(
	ctx context.Context,
	sources []string,
	expectedVersions map[string]string,
	output io.Writer,
) error {
	if err := acquireNow(ctx, m.downloadGate); err != nil {
		return err
	}
	defer release(m.downloadGate)
	prepared, err := m.prepareArchiveSources(sources, expectedVersions, "")
	if err != nil {
		return err
	}

	stripSingleDirectory := len(prepared) == 1 && prepared[0].info.IsDir()
	walkPrepared := func(budget *copyBudget, writeEntry archiveEntryWriter) error {
		for _, source := range prepared {
			archiveName := source.archiveName
			if stripSingleDirectory {
				archiveName = ""
			}
			if err := m.walkArchive(ctx, source.virtual, archiveName, source.info, budget, writeEntry); err != nil {
				return err
			}
		}
		return nil
	}
	preflightBudget := &copyBudget{maxEntries: m.maxCopyEntries, maxBytes: m.maxCopyBytes}
	if err := walkPrepared(preflightBudget, func(context.Context, string, string, os.FileInfo) error {
		return nil
	}); err != nil {
		return err
	}
	if err := m.verifyArchiveSources(prepared); err != nil {
		return err
	}

	writer := zip.NewWriter(output)
	budget := &copyBudget{maxEntries: m.maxCopyEntries, maxBytes: m.maxCopyBytes}
	if err := walkPrepared(budget, m.zipEntryWriter(writer)); err != nil {
		return err
	}
	if err := m.verifyArchiveSources(prepared); err != nil {
		return err
	}
	return writer.Close()
}

// ExportDirectory writes the contents of one directory as a restricted TAR
// stream. It shares the archive traversal budget and rejects links and special
// files in exactly the same way as the regular archive feature.
func (m *Manager) ExportDirectory(
	ctx context.Context,
	virtual string,
	expectedResourceVersion string,
	output io.Writer,
) (contract.FileEntry, error) {
	if err := acquireNow(ctx, m.downloadGate); err != nil {
		return contract.FileEntry{}, err
	}
	defer release(m.downloadGate)
	_, normalized, err := m.resolveExisting(virtual)
	if err != nil {
		return contract.FileEntry{}, err
	}
	if normalized == "/" {
		return contract.FileEntry{}, ErrRootOperation
	}
	info, err := m.rootFS.Lstat(rootName(normalized))
	if err != nil {
		return contract.FileEntry{}, err
	}
	if !info.IsDir() {
		return contract.FileEntry{}, ErrNotDirectory
	}
	entry := m.entry(normalized, info)
	if expectedResourceVersion != "" && entry.ResourceVersion != expectedResourceVersion {
		return contract.FileEntry{}, ErrConflict
	}

	writer := tar.NewWriter(output)
	budget := &copyBudget{maxEntries: m.maxCopyEntries, maxBytes: m.maxCopyBytes}
	type sourceVersion struct {
		virtual string
		info    os.FileInfo
		version string
	}
	versions := make([]sourceVersion, 0)
	writeEntry := m.tarEntryWriter(writer)
	trackingWriter := func(ctx context.Context, archiveName, sourceVirtual string, sourceInfo os.FileInfo) error {
		versions = append(versions, sourceVersion{
			virtual: sourceVirtual, info: sourceInfo,
			version: resourceVersion(sourceVirtual, sourceInfo),
		})
		return writeEntry(ctx, archiveName, sourceVirtual, sourceInfo)
	}
	if err := m.walkArchive(ctx, normalized, "", info, budget, trackingWriter); err != nil {
		_ = writer.Close()
		return contract.FileEntry{}, err
	}
	if err := writer.Close(); err != nil {
		return contract.FileEntry{}, err
	}
	current, err := m.rootFS.Lstat(rootName(normalized))
	if err != nil || !os.SameFile(info, current) ||
		resourceVersion(normalized, current) != entry.ResourceVersion {
		return contract.FileEntry{}, ErrConflict
	}
	for _, version := range versions {
		current, err := m.rootFS.Lstat(rootName(version.virtual))
		if err != nil || !os.SameFile(version.info, current) ||
			resourceVersion(version.virtual, current) != version.version {
			return contract.FileEntry{}, ErrConflict
		}
	}
	return entry, nil
}

// ImportDirectory extracts a restricted TAR stream into a hidden sibling and
// publishes it with a no-replace rename. A failed or cancelled transfer never
// exposes a partially populated destination.
func (m *Manager) ImportDirectory(
	ctx context.Context,
	targetDirectory string,
	name string,
	content io.Reader,
) (contract.FileEntry, error) {
	input := contract.FileReceiveInput{Directory: targetDirectory, Name: name, Kind: "directory", SizeBytes: -1}
	return m.ReceiveStream(ctx, input, content, receiveLimit(input))
}

func (m *Manager) importDirectory(ctx context.Context, input contract.FileReceiveInput, content io.Reader, parent *fileRoot, tempVirtual string, stageCreated func(os.FileInfo) error, beforePublish func(*fileRoot, os.FileInfo) error) (contract.FileEntry, error) {
	targetDirectory, name := input.Directory, input.Name
	if err := validateName(name); err != nil {
		return contract.FileEntry{}, err
	}
	if err := acquireNow(ctx, m.uploadGate); err != nil {
		return contract.FileEntry{}, err
	}
	defer release(m.uploadGate)
	normalizedTarget := targetDirectory
	targetInfo, err := parent.Stat(".")
	if err != nil {
		return contract.FileEntry{}, err
	}
	if !targetInfo.IsDir() {
		return contract.FileEntry{}, ErrNotDirectory
	}
	outputVirtual := joinVirtual(normalizedTarget, name)
	if err := m.mutationError(outputVirtual); err != nil {
		return contract.FileEntry{}, err
	}
	if _, err := parent.Lstat(name); err == nil {
		return contract.FileEntry{}, ErrAlreadyExists
	} else if !errors.Is(err, os.ErrNotExist) {
		return contract.FileEntry{}, err
	}

	tempName := path.Base(tempVirtual)
	if err := parent.Mkdir(tempName, 0700); err != nil {
		return contract.FileEntry{}, err
	}
	var ownedStage os.FileInfo
	defer func() {
		if ownedStage != nil {
			if current, err := parent.Lstat(tempName); err == nil {
				_ = removeOwnedReceiveDirectory(parent, tempName, current, ownedStage, "")
			}
		}
	}()
	stageInfo, err := parent.Lstat(tempName)
	if err != nil || !stageInfo.IsDir() || stageInfo.Mode()&os.ModeSymlink != 0 {
		return contract.FileEntry{}, ErrConflict
	}
	publication, err := parent.OpenRoot(tempName)
	if err != nil {
		return contract.FileEntry{}, err
	}
	defer publication.Close()
	opened, err := publication.Stat(".")
	if err != nil || !os.SameFile(stageInfo, opened) {
		return contract.FileEntry{}, ErrConflict
	}
	ownedStage = opened
	if err := stageCreated(opened); err != nil {
		return contract.FileEntry{}, err
	}
	if err := publication.Mkdir("result", 0700); err != nil {
		return contract.FileEntry{}, err
	}
	stage, err := publication.OpenRoot("result")
	if err != nil {
		return contract.FileEntry{}, err
	}
	defer stage.Close()
	// Every extracted child stays under the opened staging object even if its
	// visible sibling name is replaced. It cannot redirect privileged writes
	// into another location within the wider Agent filesystem root.
	extractor := &Manager{rootFS: stage}
	budget := &copyBudget{maxEntries: m.maxCopyEntries, maxBytes: m.maxCopyBytes}
	seen := make(map[string]struct{})
	directoryTimes := make([]archiveDirectoryTime, 0)
	reader := tar.NewReader(&contextReader{ctx: ctx, reader: content})
	if err := extractor.extractTAR(ctx, reader, "/", budget, seen, &directoryTimes); err != nil {
		return contract.FileEntry{}, err
	}
	// TAR end blocks are not the transport end marker. Drain the underlying
	// stream so an encrypted/truncated transfer cannot be committed early.
	if _, err := io.Copy(io.Discard, &contextReader{ctx: ctx, reader: content}); err != nil {
		return contract.FileEntry{}, err
	}
	if err := extractor.applyArchiveDirectoryTimes(directoryTimes); err != nil {
		return contract.FileEntry{}, err
	}
	mode := os.FileMode(0755)
	if input.Mode != "" {
		value, _ := strconv.ParseUint(input.Mode, 8, 32)
		mode = os.FileMode(value)
	}
	if err := stage.Chmod(".", mode); err != nil {
		return contract.FileEntry{}, err
	}
	if input.ModifiedAt != nil {
		if err := stage.Chtimes(".", *input.ModifiedAt, *input.ModifiedAt); err != nil {
			return contract.FileEntry{}, err
		}
	}
	if err := syncRootDirectory(stage, "."); err != nil {
		return contract.FileEntry{}, err
	}
	info, err := stage.Lstat(".")
	if err != nil {
		return contract.FileEntry{}, err
	}
	if err := beforePublish(stage, info); err != nil {
		return contract.FileEntry{}, err
	}
	if err := ctx.Err(); err != nil {
		return contract.FileEntry{}, err
	}
	_, _, parentErr := m.resolveExisting(normalizedTarget)
	currentParent, statErr := m.rootFS.Lstat(rootName(normalizedTarget))
	if parentErr != nil || statErr != nil || !os.SameFile(targetInfo, currentParent) {
		return contract.FileEntry{}, ErrConflict
	}
	visible, err := parent.Lstat(tempName)
	if err != nil || !os.SameFile(ownedStage, visible) {
		return contract.FileEntry{}, ErrConflict
	}
	if err := publishReceiveObject(publication, "result", parent, name, false); err != nil {
		if errors.Is(err, os.ErrExist) {
			return contract.FileEntry{}, ErrAlreadyExists
		}
		return contract.FileEntry{}, err
	}
	if err := syncRootDirectory(publication, "."); err != nil {
		return contract.FileEntry{}, err
	}
	if err := syncRootDirectory(parent, "."); err != nil {
		return contract.FileEntry{}, err
	}
	_, _, parentErr = m.resolveExisting(normalizedTarget)
	currentParent, statErr = m.rootFS.Lstat(rootName(normalizedTarget))
	if parentErr != nil || statErr != nil || !os.SameFile(targetInfo, currentParent) {
		return contract.FileEntry{}, ErrConflict
	}
	published, err := parent.Lstat(name)
	if err != nil || !os.SameFile(info, published) {
		return contract.FileEntry{}, ErrConflict
	}
	return m.entry(outputVirtual, published), nil
}
