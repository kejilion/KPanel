package filemanager

import (
	"archive/tar"
	"archive/zip"
	"context"
	"errors"
	"io"
	"os"
	"path"

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

	writer := zip.NewWriter(output)
	budget := &copyBudget{maxEntries: m.maxCopyEntries, maxBytes: m.maxCopyBytes}
	writeEntry := m.zipEntryWriter(writer)
	stripSingleDirectory := len(prepared) == 1 && prepared[0].info.IsDir()
	for _, source := range prepared {
		archiveName := path.Base(source.virtual)
		if stripSingleDirectory {
			archiveName = ""
		}
		if err := m.walkArchive(ctx, source.virtual, archiveName, source.info, budget, writeEntry); err != nil {
			return err
		}
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
	if err := validateName(name); err != nil {
		return contract.FileEntry{}, err
	}
	if err := acquireNow(ctx, m.uploadGate); err != nil {
		return contract.FileEntry{}, err
	}
	defer release(m.uploadGate)
	_, normalizedTarget, err := m.resolveExisting(targetDirectory)
	if err != nil {
		return contract.FileEntry{}, err
	}
	targetInfo, err := m.rootFS.Lstat(rootName(normalizedTarget))
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
	if _, err := m.rootFS.Lstat(rootName(outputVirtual)); err == nil {
		return contract.FileEntry{}, ErrAlreadyExists
	} else if !errors.Is(err, os.ErrNotExist) {
		return contract.FileEntry{}, err
	}

	tempVirtual := joinVirtual(normalizedTarget, ".kpanel-extract-"+randomID())
	if err := m.rootFS.Mkdir(rootName(tempVirtual), 0700); err != nil {
		return contract.FileEntry{}, err
	}
	success := false
	defer func() {
		if !success {
			_ = m.rootFS.RemoveAll(rootName(tempVirtual))
		}
	}()
	budget := &copyBudget{maxEntries: m.maxCopyEntries, maxBytes: m.maxCopyBytes}
	seen := make(map[string]struct{})
	directoryTimes := make([]archiveDirectoryTime, 0)
	reader := tar.NewReader(&contextReader{ctx: ctx, reader: content})
	if err := m.extractTAR(ctx, reader, tempVirtual, budget, seen, &directoryTimes); err != nil {
		return contract.FileEntry{}, err
	}
	// TAR end blocks are not the transport end marker. Drain the underlying
	// stream so an encrypted/truncated transfer cannot be committed early.
	if _, err := io.Copy(io.Discard, &contextReader{ctx: ctx, reader: content}); err != nil {
		return contract.FileEntry{}, err
	}
	if err := m.applyArchiveDirectoryTimes(directoryTimes); err != nil {
		return contract.FileEntry{}, err
	}
	if err := m.rootFS.Chmod(rootName(tempVirtual), 0755); err != nil {
		return contract.FileEntry{}, err
	}
	if err := renameNoReplaceRoot(m.rootFS, tempVirtual, outputVirtual); err != nil {
		if errors.Is(err, os.ErrExist) {
			return contract.FileEntry{}, ErrAlreadyExists
		}
		return contract.FileEntry{}, err
	}
	if err := syncRootDirectory(m.rootFS, rootName(normalizedTarget)); err != nil {
		return contract.FileEntry{}, err
	}
	success = true
	return m.Stat(outputVirtual)
}
