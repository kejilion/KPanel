package filemanager

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

const (
	archiveFormatZIP   = "zip"
	archiveFormatTAR   = "tar"
	archiveFormatTARGZ = "tar.gz"
)

var ErrInvalidArchive = errors.New("压缩包格式或内容无效")

type archiveDirectoryTime struct {
	virtual  string
	modified time.Time
}

type archiveSourceEntry struct {
	virtual     string
	archiveName string
	info        os.FileInfo
}

type archiveEntryWriter func(
	ctx context.Context,
	archiveName string,
	sourceVirtual string,
	info os.FileInfo,
) error

type archiveSourceRootKey struct{}

func (m *Manager) prepareArchiveSources(
	sources []string,
	expectedVersions map[string]string,
	outputVirtual string,
) ([]archiveSourceEntry, error) {
	if len(sources) == 0 {
		return nil, ErrAction
	}
	if len(sources) > MaxBatchItems {
		return nil, ErrBatchTooLarge
	}
	prepared := make([]archiveSourceEntry, 0, len(sources))
	seenSources := make(map[string]struct{}, len(sources))
	for _, source := range sources {
		_, normalizedSource, err := m.resolveExisting(source)
		if err != nil {
			return nil, err
		}
		if normalizedSource == "/" {
			return nil, ErrRootOperation
		}
		if err := m.checkExpectedVersion(normalizedSource, expectedVersions[source]); err != nil {
			return nil, err
		}
		info, err := m.rootFS.Lstat(rootName(normalizedSource))
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, ErrSymlink
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			return nil, ErrNotRegular
		}
		if outputVirtual != "" && info.IsDir() && isWithin(outputVirtual, normalizedSource) {
			return nil, ErrInvalidPath
		}
		if _, exists := seenSources[normalizedSource]; exists {
			return nil, ErrAlreadyExists
		}
		seenSources[normalizedSource] = struct{}{}
		prepared = append(prepared, archiveSourceEntry{
			virtual: normalizedSource, archiveName: path.Base(normalizedSource), info: info,
		})
	}
	assignUniqueArchiveNames(prepared)
	return prepared, nil
}

func assignUniqueArchiveNames(prepared []archiveSourceEntry) {
	reserved := make([]string, 0, len(prepared))
	for _, source := range prepared {
		reserved = append(reserved, source.archiveName)
	}
	used := make([]string, 0, len(prepared))
	for index := range prepared {
		original := prepared[index].archiveName
		candidate := original
		if archiveNameExists(used, candidate) {
			for suffix := 2; ; suffix++ {
				candidate = suffixedArchiveName(original, suffix)
				if !archiveNameExists(used, candidate) && !archiveNameExists(reserved, candidate) {
					break
				}
			}
		}
		prepared[index].archiveName = candidate
		used = append(used, candidate)
	}
}

func archiveNameExists(names []string, candidate string) bool {
	for _, name := range names {
		if strings.EqualFold(name, candidate) {
			return true
		}
	}
	return false
}

func suffixedArchiveName(name string, suffix int) string {
	extension := path.Ext(name)
	stem := strings.TrimSuffix(name, extension)
	if stem == "" {
		stem = name
		extension = ""
	}
	return fmt.Sprintf("%s (%d)%s", stem, suffix, extension)
}

func (m *Manager) verifyArchiveSources(prepared []archiveSourceEntry) error {
	for _, source := range prepared {
		current, err := m.rootFS.Lstat(rootName(source.virtual))
		if err != nil || !os.SameFile(source.info, current) ||
			resourceVersion(source.virtual, current) != resourceVersion(source.virtual, source.info) {
			return ErrConflict
		}
	}
	return nil
}

func (m *Manager) compressArchive(
	ctx context.Context,
	sources []string,
	targetDirectory string,
	name string,
	format string,
	expectedVersions map[string]string,
) (contract.FileEntry, error) {
	if err := validateArchiveName(name, format); err != nil {
		return contract.FileEntry{}, err
	}
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
	parent, openedTarget, err := m.openReceiveDirectory(normalizedTarget, false)
	if err != nil {
		return contract.FileEntry{}, err
	}
	defer parent.Close()
	if !os.SameFile(targetInfo, openedTarget) {
		return contract.FileEntry{}, ErrConflict
	}
	outputVirtual := joinVirtual(normalizedTarget, name)
	if err := m.mutationError(outputVirtual); err != nil {
		return contract.FileEntry{}, err
	}
	if _, err := m.rootFS.Lstat(rootName(outputVirtual)); err == nil && fileConflicts(ctx) == nil {
		return contract.FileEntry{}, ErrAlreadyExists
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return contract.FileEntry{}, err
	}

	prepared, err := m.prepareArchiveSources(sources, expectedVersions, outputVirtual)
	if err != nil {
		return contract.FileEntry{}, err
	}

	// Only a private, pinned publication directory may name the generated inode.
	// Pinning each individual operation on the wider root is not sufficient.
	workspace, workspacePath, workspaceInfo, err := m.openArchiveWorkspace(normalizedTarget, openedTarget)
	if err != nil {
		return contract.FileEntry{}, err
	}
	defer workspace.Close()
	tempName := ".kpanel-archive-" + randomID()
	if operation := archiveOptions(ctx); operation != nil && operation.tempSuffix != "" {
		tempName = ".kpanel-archive-" + operation.tempSuffix
	}
	if err := workspace.Mkdir(tempName, 0700); err != nil {
		return contract.FileEntry{}, err
	}
	stageInfo, err := workspace.Lstat(tempName)
	if err != nil || !stageInfo.IsDir() || stageInfo.Mode()&os.ModeSymlink != 0 {
		return contract.FileEntry{}, ErrConflict
	}
	stage, err := workspace.OpenRoot(tempName)
	if err != nil {
		return contract.FileEntry{}, err
	}
	defer stage.Close()
	ownedStage, err := stage.Stat(".")
	if err != nil || !os.SameFile(stageInfo, ownedStage) || !archiveStagePrivate(ownedStage, stage) {
		return contract.FileEntry{}, ErrConflict
	}
	defer func() {
		if visible, err := workspace.Lstat(tempName); err == nil {
			_ = removeOwnedReceiveDirectory(workspace, tempName, visible, ownedStage, "")
		}
	}()
	if err := syncRootDirectory(workspace, "."); err != nil {
		return contract.FileEntry{}, err
	}
	if err := recordArchiveTemp(ctx, parent, openedTarget, workspacePath, workspace, workspaceInfo, stage, ownedStage); err != nil {
		return contract.FileEntry{}, err
	}
	if err := inheritArchiveDestinationDefaults(stage, parent); err != nil {
		return contract.FileEntry{}, err
	}
	temp, err := stage.OpenFile("body", os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return contract.FileEntry{}, err
	}
	encoderClosed := false
	var closeEncoder func() error
	defer func() {
		if !encoderClosed && closeEncoder != nil {
			_ = closeEncoder()
		}
		_ = temp.Close()
	}()

	var writeEntry archiveEntryWriter
	switch format {
	case archiveFormatZIP:
		writer := zip.NewWriter(temp)
		writeEntry = m.zipEntryWriter(writer)
		closeEncoder = writer.Close
	case archiveFormatTAR:
		writer := tar.NewWriter(temp)
		writeEntry = m.tarEntryWriter(writer)
		closeEncoder = writer.Close
	case archiveFormatTARGZ:
		gzipWriter := gzip.NewWriter(temp)
		tarWriter := tar.NewWriter(gzipWriter)
		writeEntry = m.tarEntryWriter(tarWriter)
		closeEncoder = func() error {
			tarErr := tarWriter.Close()
			gzipErr := gzipWriter.Close()
			if tarErr != nil {
				return tarErr
			}
			return gzipErr
		}
	default:
		return contract.FileEntry{}, ErrInvalidArchive
	}

	budget := archiveBudget(ctx, m)
	stripSingleDirectory := len(prepared) == 1 && prepared[0].info.IsDir()
	for _, source := range prepared {
		archiveName := source.archiveName
		if stripSingleDirectory {
			archiveName = ""
		}
		if err := m.walkArchive(ctx, source.virtual, archiveName, source.info, budget, writeEntry); err != nil {
			return contract.FileEntry{}, err
		}
	}
	if err := m.verifyArchiveSources(prepared); err != nil {
		return contract.FileEntry{}, err
	}
	encoderClosed = true
	if err := closeEncoder(); err != nil {
		return contract.FileEntry{}, err
	}
	if err := temp.Sync(); err != nil {
		return contract.FileEntry{}, err
	}
	resultInfo, err := temp.Stat()
	if err != nil {
		return contract.FileEntry{}, err
	}
	if err := temp.Close(); err != nil {
		return contract.FileEntry{}, err
	}
	if err := ctx.Err(); err != nil {
		return contract.FileEntry{}, err
	}
	if err := m.verifyArchiveOutputParent(normalizedTarget, openedTarget); err != nil {
		return contract.FileEntry{}, err
	}
	visible, err := workspace.Lstat(tempName)
	if err != nil || !os.SameFile(ownedStage, visible) {
		return contract.FileEntry{}, ErrConflict
	}
	if fileConflicts(ctx) != nil {
		return m.publishConflictTree(ctx, stage, "body", normalizedTarget, name, joinVirtual(joinVirtual(workspacePath, tempName), "body"), func() error {
			if err := m.verifyArchiveOutputParent(normalizedTarget, openedTarget); err != nil {
				return err
			}
			visible, err := workspace.Lstat(tempName)
			if err != nil || !os.SameFile(ownedStage, visible) {
				return ErrConflict
			}
			return nil
		}, 0)
	}
	if err := publishReceiveObject(stage, "body", parent, name, false); err != nil {
		if errors.Is(err, os.ErrExist) {
			return contract.FileEntry{}, ErrAlreadyExists
		}
		return contract.FileEntry{}, err
	}
	if err := syncRootDirectory(parent, "."); err != nil {
		return contract.FileEntry{}, err
	}
	if err := m.verifyArchiveOutputParent(normalizedTarget, openedTarget); err != nil {
		return contract.FileEntry{}, err
	}
	published, err := parent.Lstat(name)
	if err != nil || !os.SameFile(resultInfo, published) {
		return contract.FileEntry{}, ErrConflict
	}
	return m.entry(outputVirtual, published), nil
}

func (m *Manager) verifyArchiveOutputParent(virtual string, expected os.FileInfo) error {
	parent, current, err := m.openReceiveDirectory(virtual, false)
	if err != nil {
		return err
	}
	defer parent.Close()
	if !os.SameFile(expected, current) {
		return ErrConflict
	}
	return nil
}

// mkdir does not return an inode handle. Bootstrap only inside a held directory
// whose entries cannot be replaced by an unprivileged local writer. The target
// itself may remain writable by business users; publication still uses its fd.
func (m *Manager) openArchiveWorkspace(target string, targetInfo os.FileInfo) (*fileRoot, string, os.FileInfo, error) {
	for current := target; ; current = path.Dir(current) {
		root, info, err := m.openReceiveDirectory(current, false)
		if err != nil {
			return nil, "", nil, err
		}
		if !archiveSameFilesystem(targetInfo, info) {
			_ = root.Close()
			return nil, "", nil, ErrConflict
		}
		if archiveWorkspaceProtected(info, root) {
			return root, current, info, nil
		}
		_ = root.Close()
		if current == "/" {
			return nil, "", nil, ErrConflict
		}
	}
}

func (m *Manager) walkArchive(
	ctx context.Context,
	sourceVirtual string,
	archiveName string,
	info os.FileInfo,
	budget *copyBudget,
	writeEntry archiveEntryWriter,
) error {
	parent, _, err := m.openReceiveDirectory(path.Dir(sourceVirtual), false)
	if err != nil {
		return err
	}
	defer parent.Close()
	return m.walkArchiveRoot(ctx, parent, sourceVirtual, archiveName, info, budget, writeEntry)
}

func (m *Manager) walkArchiveRoot(
	ctx context.Context,
	parent *fileRoot,
	sourceVirtual string,
	archiveName string,
	info os.FileInfo,
	budget *copyBudget,
	writeEntry archiveEntryWriter,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	sourceName := rootName(path.Base(sourceVirtual))
	visible, err := parent.Lstat(sourceName)
	if err != nil || !os.SameFile(info, visible) {
		return ErrConflict
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return ErrSymlink
	}
	if !info.IsDir() && !info.Mode().IsRegular() {
		return ErrNotRegular
	}
	budget.entries++
	if info.Mode().IsRegular() {
		budget.bytes += info.Size()
	}
	if budget.entries > budget.maxEntries || budget.bytes > budget.maxBytes {
		return ErrTooLarge
	}
	if archiveName != "" {
		if err := writeEntry(context.WithValue(ctx, archiveSourceRootKey{}, parent), archiveName, sourceVirtual, info); err != nil {
			return err
		}
	}
	archiveProgress(ctx, budget)
	if !info.IsDir() {
		return nil
	}
	directoryRoot, err := parent.OpenRoot(sourceName)
	if err != nil {
		return err
	}
	defer directoryRoot.Close()
	opened, err := directoryRoot.Stat(".")
	if err != nil || !os.SameFile(info, opened) {
		return ErrConflict
	}
	directory, err := directoryRoot.Open(".")
	if err != nil {
		return err
	}
	remaining := budget.maxEntries - budget.entries
	if remaining < 0 {
		_ = directory.Close()
		return ErrTooLarge
	}
	entries, readErr := directory.ReadDir(remaining + 1)
	closeErr := directory.Close()
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return readErr
	}
	if closeErr != nil {
		return closeErr
	}
	if len(entries) > remaining {
		return ErrTooLarge
	}
	sort.Slice(entries, func(left, right int) bool { return entries[left].Name() < entries[right].Name() })
	for _, entry := range entries {
		childVirtual := joinVirtual(sourceVirtual, entry.Name())
		if m.isProtected(childVirtual) || isInternalComponent(entry.Name()) {
			continue
		}
		childInfo, statErr := directoryRoot.Lstat(entry.Name())
		if statErr != nil {
			return statErr
		}
		childArchiveName := entry.Name()
		if archiveName != "" {
			childArchiveName = path.Join(archiveName, entry.Name())
		}
		if err := m.walkArchiveRoot(ctx, directoryRoot, childVirtual, childArchiveName, childInfo, budget, writeEntry); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) zipEntryWriter(writer *zip.Writer) archiveEntryWriter {
	return func(ctx context.Context, archiveName, sourceVirtual string, info os.FileInfo) error {
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = archiveName
		if info.IsDir() {
			header.Name += "/"
		} else {
			header.Method = zip.Deflate
		}
		destination, err := writer.CreateHeader(header)
		if err != nil || info.IsDir() {
			return err
		}
		return m.copyArchiveFile(ctx, sourceVirtual, info, destination)
	}
}

func (m *Manager) tarEntryWriter(writer *tar.Writer) archiveEntryWriter {
	return func(ctx context.Context, archiveName, sourceVirtual string, info os.FileInfo) error {
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = archiveName
		if info.IsDir() {
			header.Name += "/"
		}
		if err := writer.WriteHeader(header); err != nil || info.IsDir() {
			return err
		}
		return m.copyArchiveFile(ctx, sourceVirtual, info, writer)
	}
}

func (m *Manager) copyArchiveFile(
	ctx context.Context,
	sourceVirtual string,
	expected os.FileInfo,
	destination io.Writer,
) error {
	parent, ok := ctx.Value(archiveSourceRootKey{}).(*fileRoot)
	if !ok {
		return ErrConflict
	}
	source, err := parent.Open(path.Base(sourceVirtual))
	if err != nil {
		return err
	}
	defer source.Close()
	opened, err := source.Stat()
	if err != nil || !os.SameFile(expected, opened) || !opened.Mode().IsRegular() {
		return ErrConflict
	}
	written, err := io.CopyBuffer(
		destination,
		io.LimitReader(&contextReader{ctx: ctx, reader: source}, expected.Size()+1),
		make([]byte, 64<<10),
	)
	if err != nil {
		return err
	}
	current, err := source.Stat()
	if err != nil || written != expected.Size() ||
		resourceVersion(sourceVirtual, current) != resourceVersion(sourceVirtual, expected) {
		return ErrConflict
	}
	return nil
}

func (m *Manager) extractArchive(
	ctx context.Context,
	sourceVirtual string,
	targetDirectory string,
	name string,
	format string,
	expectedVersion string,
) (contract.FileEntry, error) {
	if err := validateName(name); err != nil {
		return contract.FileEntry{}, err
	}
	if err := validateArchiveSourceName(path.Base(sourceVirtual), format); err != nil {
		return contract.FileEntry{}, err
	}
	_, normalizedSource, err := m.resolveExisting(sourceVirtual)
	if err != nil {
		return contract.FileEntry{}, err
	}
	sourceInfo, err := m.rootFS.Lstat(rootName(normalizedSource))
	if err != nil {
		return contract.FileEntry{}, err
	}
	if !sourceInfo.Mode().IsRegular() {
		return contract.FileEntry{}, ErrNotRegular
	}
	if expectedVersion != "" && resourceVersion(normalizedSource, sourceInfo) != expectedVersion {
		return contract.FileEntry{}, ErrConflict
	}
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
	parent, openedTarget, err := m.openReceiveDirectory(normalizedTarget, false)
	if err != nil {
		return contract.FileEntry{}, err
	}
	defer parent.Close()
	if !os.SameFile(targetInfo, openedTarget) {
		return contract.FileEntry{}, ErrConflict
	}
	outputVirtual := joinVirtual(normalizedTarget, name)
	if err := m.mutationError(outputVirtual); err != nil {
		return contract.FileEntry{}, err
	}
	if _, err := m.rootFS.Lstat(rootName(outputVirtual)); err == nil && fileConflicts(ctx) == nil {
		return contract.FileEntry{}, ErrAlreadyExists
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return contract.FileEntry{}, err
	}

	tempVirtual := joinVirtual(normalizedTarget, ".kpanel-extract-"+randomID())
	if operation := archiveOptions(ctx); operation != nil && operation.tempSuffix != "" {
		tempVirtual = joinVirtual(normalizedTarget, ".kpanel-extract-"+operation.tempSuffix)
	}
	tempName := path.Base(tempVirtual)
	workspace, workspacePath, workspaceInfo, err := m.openArchiveWorkspace(normalizedTarget, openedTarget)
	if err != nil {
		return contract.FileEntry{}, err
	}
	defer workspace.Close()
	if err := workspace.Mkdir(tempName, 0700); err != nil {
		return contract.FileEntry{}, err
	}
	var ownedStage os.FileInfo
	defer func() {
		if ownedStage != nil {
			if visible, err := workspace.Lstat(tempName); err == nil {
				_ = removeOwnedReceiveDirectory(workspace, tempName, visible, ownedStage, "")
			}
		}
	}()
	stageInfo, err := workspace.Lstat(tempName)
	if err != nil || !stageInfo.IsDir() || stageInfo.Mode()&os.ModeSymlink != 0 {
		return contract.FileEntry{}, ErrConflict
	}
	publication, err := workspace.OpenRoot(tempName)
	if err != nil {
		return contract.FileEntry{}, err
	}
	defer publication.Close()
	opened, err := publication.Stat(".")
	if err != nil || !os.SameFile(stageInfo, opened) || !archiveStagePrivate(opened, publication) {
		return contract.FileEntry{}, ErrConflict
	}
	ownedStage = opened
	if err := syncRootDirectory(workspace, "."); err != nil {
		return contract.FileEntry{}, err
	}
	if err := recordArchiveTemp(ctx, parent, openedTarget, workspacePath, workspace, workspaceInfo, publication, ownedStage); err != nil {
		return contract.FileEntry{}, err
	}
	if err := inheritArchiveDestinationDefaults(publication, parent); err != nil {
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
	// Resolve every extracted child against the opened staging object. A
	// replacement sibling cannot redirect writes within the wider Agent root.
	extractor := &Manager{rootFS: stage}

	source, err := m.rootFS.Open(rootName(normalizedSource))
	if err != nil {
		return contract.FileEntry{}, err
	}
	defer source.Close()
	openedInfo, err := source.Stat()
	if err != nil || !os.SameFile(sourceInfo, openedInfo) {
		return contract.FileEntry{}, ErrConflict
	}
	budget := archiveBudget(ctx, m)
	directoryTimes := make([]archiveDirectoryTime, 0)
	if err := extractor.readArchive(ctx, source, sourceInfo.Size(), format, "/", budget, &directoryTimes); err != nil {
		return contract.FileEntry{}, err
	}
	if operation := archiveOptions(ctx); operation != nil {
		for _, selected := range operation.selection {
			if !operation.found[selected] {
				return contract.FileEntry{}, ErrConflict
			}
		}
	}
	if err := extractor.applyArchiveDirectoryTimes(directoryTimes); err != nil {
		return contract.FileEntry{}, err
	}
	currentInfo, err := source.Stat()
	if err != nil || resourceVersion(normalizedSource, currentInfo) != resourceVersion(normalizedSource, sourceInfo) {
		return contract.FileEntry{}, ErrConflict
	}
	if err := m.checkExpectedVersion(normalizedSource, resourceVersion(normalizedSource, sourceInfo)); err != nil {
		return contract.FileEntry{}, err
	}
	if err := ctx.Err(); err != nil {
		return contract.FileEntry{}, err
	}
	visible, stageErr := workspace.Lstat(tempName)
	_, _, parentErr := m.resolveExisting(normalizedTarget)
	currentTarget, targetErr := m.rootFS.Lstat(rootName(normalizedTarget))
	if stageErr != nil || !os.SameFile(ownedStage, visible) || parentErr != nil || targetErr != nil || !os.SameFile(targetInfo, currentTarget) {
		return contract.FileEntry{}, ErrConflict
	}
	resultInfo, err := stage.Stat(".")
	if err != nil {
		return contract.FileEntry{}, err
	}
	if fileConflicts(ctx) != nil {
		return m.publishConflictTree(ctx, publication, "result", normalizedTarget, name, joinVirtual(joinVirtual(workspacePath, tempName), "result"), func() error {
			if err := m.verifyArchiveOutputParent(normalizedTarget, openedTarget); err != nil {
				return err
			}
			visible, err := workspace.Lstat(tempName)
			if err != nil || !os.SameFile(ownedStage, visible) {
				return ErrConflict
			}
			return nil
		}, 0)
	}
	if err := publishReceiveObject(publication, "result", parent, name, false); err != nil {
		if errors.Is(err, os.ErrExist) {
			return contract.FileEntry{}, ErrAlreadyExists
		}
		return contract.FileEntry{}, err
	}
	if err := syncRootDirectory(parent, "."); err != nil {
		return contract.FileEntry{}, err
	}
	_, _, parentErr = m.resolveExisting(normalizedTarget)
	currentTarget, targetErr = m.rootFS.Lstat(rootName(normalizedTarget))
	published, publishedErr := parent.Lstat(name)
	if parentErr != nil || targetErr != nil || !os.SameFile(targetInfo, currentTarget) || publishedErr != nil || !os.SameFile(resultInfo, published) {
		return contract.FileEntry{}, ErrConflict
	}
	return m.entry(outputVirtual, published), nil
}

func boundedZIPDirectory(ctx context.Context, reader io.ReaderAt, size int64, maxEntries int) (*archiveDirectoryReader, error) {
	const (
		endRecordSize  = 22
		maxCommentSize = 1<<16 - 1
	)
	if size < endRecordSize {
		return nil, ErrInvalidArchive
	}
	tailSize := int64(endRecordSize + maxCommentSize)
	if size < tailSize {
		tailSize = size
	}
	tail := make([]byte, tailSize)
	if _, err := reader.ReadAt(tail, size-tailSize); err != nil {
		return nil, ErrInvalidArchive
	}
	signature := []byte{'P', 'K', 0x05, 0x06}
	index := len(tail) - endRecordSize
	for index >= 0 {
		candidate := bytes.LastIndex(tail[:index+len(signature)], signature)
		if candidate < 0 {
			return nil, ErrInvalidArchive
		}
		if candidate+endRecordSize <= len(tail) {
			commentSize := int(binary.LittleEndian.Uint16(tail[candidate+20 : candidate+22]))
			if candidate+endRecordSize+commentSize == len(tail) {
				index = candidate
				break
			}
		}
		index = candidate - 1
	}
	if index < 0 {
		return nil, ErrInvalidArchive
	}
	record := tail[index:]
	if binary.LittleEndian.Uint16(record[4:6]) != 0 ||
		binary.LittleEndian.Uint16(record[6:8]) != 0 {
		return nil, ErrInvalidArchive
	}
	entriesOnDisk := binary.LittleEndian.Uint16(record[8:10])
	totalEntries := binary.LittleEndian.Uint16(record[10:12])
	if entriesOnDisk != totalEntries {
		return nil, ErrInvalidArchive
	}
	directorySize := int64(binary.LittleEndian.Uint32(record[12:16]))
	directoryOffset := int64(binary.LittleEndian.Uint32(record[16:20]))
	endOffset := size - tailSize + int64(index)
	// Reject the three ZIP64 EOCD sentinels before the standard library can
	// replace the already validated classic-directory bounds.
	if totalEntries == 0xffff || directorySize == 0xffffffff || directoryOffset == 0xffffffff {
		return nil, ErrInvalidArchive
	}
	if int(totalEntries) > maxEntries {
		return nil, ErrTooLarge
	}
	if directorySize > maxArchiveIndexBytes {
		return nil, ErrTooLarge
	}
	if directoryOffset+directorySize != endOffset {
		return nil, ErrInvalidArchive
	}
	// Freeze the entire bounded central directory and EOCD. A source changing
	// after this check cannot trick zip.NewReader into allocating a larger index.
	frozen := make([]byte, size-directoryOffset)
	if _, err := reader.ReadAt(frozen, directoryOffset); err != nil {
		return nil, ErrInvalidArchive
	}
	if !bytes.Equal(frozen[directorySize:], record) {
		return nil, ErrConflict
	}
	position := int64(0)
	for count := 0; count < int(totalEntries); count++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if position+46 > directorySize || binary.LittleEndian.Uint32(frozen[position:position+4]) != 0x02014b50 {
			return nil, ErrInvalidArchive
		}
		header := frozen[position : position+46]
		if binary.LittleEndian.Uint16(header[34:36]) != 0 {
			return nil, ErrInvalidArchive
		}
		position += 46 + int64(binary.LittleEndian.Uint16(header[28:30])) + int64(binary.LittleEndian.Uint16(header[30:32])) + int64(binary.LittleEndian.Uint16(header[32:34]))
		if position > directorySize {
			return nil, ErrInvalidArchive
		}
	}
	if position != directorySize {
		return nil, ErrInvalidArchive
	}
	// Give the standard library a single canonical EOCD. Its more permissive
	// signature search must not pick a fake EOCD embedded in the original comment.
	frozen = frozen[:directorySize+endRecordSize]
	binary.LittleEndian.PutUint16(frozen[directorySize+20:], 0)
	return &archiveDirectoryReader{ctx: ctx, source: reader, offset: directoryOffset, frozen: frozen, comment: string(record[endRecordSize:])}, nil
}

type archiveDirectoryReader struct {
	ctx     context.Context
	source  io.ReaderAt
	offset  int64
	frozen  []byte
	comment string
}

func (r *archiveDirectoryReader) ReadAt(p []byte, offset int64) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	if offset < 0 {
		return 0, ErrInvalidArchive
	}
	n := 0
	if offset < r.offset {
		count := min(int64(len(p)), r.offset-offset)
		var err error
		n, err = r.source.ReadAt(p[:count], offset)
		if err != nil {
			return n, err
		}
		offset += int64(n)
	}
	if n == len(p) {
		return n, nil
	}
	if offset-r.offset >= int64(len(r.frozen)) {
		return n, io.EOF
	}
	n += copy(p[n:], r.frozen[offset-r.offset:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func (m *Manager) extractZIP(
	ctx context.Context,
	reader *zip.Reader,
	tempVirtual string,
	budget *copyBudget,
	seen map[string]struct{},
	directoryTimes *[]archiveDirectoryTime,
) error {
	for _, entry := range reader.File {
		if err := ctx.Err(); err != nil {
			return err
		}
		name, err := normalizeArchiveEntry(entry.Name)
		if err != nil {
			return err
		}
		if _, exists := seen[name]; exists {
			return ErrInvalidArchive
		}
		seen[name] = struct{}{}
		info := entry.FileInfo()
		if entry.Flags&1 != 0 || (entry.Method != zip.Store && entry.Method != zip.Deflate) || info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
			return ErrInvalidArchive
		}
		budget.entries++
		if budget.entries > budget.maxEntries || entry.UncompressedSize64 > uint64(budget.maxBytes-budget.bytes) {
			return ErrTooLarge
		}
		include, err := archiveInclude(ctx, name, info, int64(entry.UncompressedSize64))
		if err != nil {
			return err
		}
		if !include {
			budget.bytes += int64(entry.UncompressedSize64)
			archiveProgress(ctx, budget)
			continue
		}
		if err := budget.reserveArchivePath(m.rootFS, tempVirtual, name); err != nil {
			return err
		}
		if operation := archiveOptions(ctx); operation != nil && operation.export != nil {
			var input io.ReadCloser
			if !info.IsDir() {
				input, err = entry.Open()
				if err != nil {
					return ErrInvalidArchive
				}
			}
			err = operation.export(name, info, int64(entry.UncompressedSize64), input)
			if input != nil {
				if closeErr := input.Close(); err == nil {
					err = closeErr
				}
			}
			if err != nil {
				return err
			}
			budget.bytes += int64(entry.UncompressedSize64)
			continue
		}
		targetVirtual := joinVirtual(tempVirtual, name)
		if info.IsDir() {
			if err := m.rootFS.MkdirAll(rootName(targetVirtual), archiveMode(info.Mode(), true)); err != nil {
				return err
			}
			if err := m.rootFS.Chmod(rootName(targetVirtual), archiveMode(info.Mode(), true)); err != nil {
				return err
			}
			*directoryTimes = append(*directoryTimes, archiveDirectoryTime{
				virtual: targetVirtual, modified: info.ModTime(),
			})
			continue
		}
		input, err := entry.Open()
		if err != nil {
			return ErrInvalidArchive
		}
		err = m.writeExtractedFile(ctx, targetVirtual, input, info.Mode(), info.ModTime(), budget)
		closeErr := input.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

func (m *Manager) extractTAR(
	ctx context.Context,
	reader *tar.Reader,
	tempVirtual string,
	budget *copyBudget,
	seen map[string]struct{},
	directoryTimes *[]archiveDirectoryTime,
) error {
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			if errors.Is(err, ErrTooLarge) || ctx.Err() != nil {
				return err
			}
			return ErrInvalidArchive
		}
		if (header.Name == "." || header.Name == "./") && header.Typeflag == tar.TypeDir {
			budget.entries++
			if budget.entries > budget.maxEntries {
				return ErrTooLarge
			}
			continue
		}
		name, err := normalizeArchiveEntry(header.Name)
		if err != nil {
			return err
		}
		if _, exists := seen[name]; exists {
			return ErrInvalidArchive
		}
		seen[name] = struct{}{}
		budget.entries++
		if budget.entries > budget.maxEntries || header.Size < 0 || header.Size > budget.maxBytes-budget.bytes {
			return ErrTooLarge
		}
		if header.Typeflag != tar.TypeDir && header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
			return ErrInvalidArchive
		}
		include, err := archiveInclude(ctx, name, header.FileInfo(), header.Size)
		if err != nil {
			return err
		}
		if !include {
			written, err := io.Copy(io.Discard, &contextReader{ctx: ctx, reader: reader})
			if err != nil {
				return err
			}
			if written != header.Size {
				return ErrInvalidArchive
			}
			budget.bytes += written
			archiveProgress(ctx, budget)
			continue
		}
		if err := budget.reserveArchivePath(m.rootFS, tempVirtual, name); err != nil {
			return err
		}
		if operation := archiveOptions(ctx); operation != nil && operation.export != nil {
			if err := operation.export(name, header.FileInfo(), header.Size, reader); err != nil {
				return err
			}
			budget.bytes += header.Size
			continue
		}
		targetVirtual := joinVirtual(tempVirtual, name)
		switch header.Typeflag {
		case tar.TypeDir:
			if err := m.rootFS.MkdirAll(rootName(targetVirtual), archiveMode(header.FileInfo().Mode(), true)); err != nil {
				return err
			}
			if err := m.rootFS.Chmod(rootName(targetVirtual), archiveMode(header.FileInfo().Mode(), true)); err != nil {
				return err
			}
			*directoryTimes = append(*directoryTimes, archiveDirectoryTime{
				virtual: targetVirtual, modified: header.ModTime,
			})
		case tar.TypeReg, tar.TypeRegA:
			if err := m.writeExtractedFile(ctx, targetVirtual, reader, header.FileInfo().Mode(), header.ModTime, budget); err != nil {
				return err
			}
		default:
			return ErrInvalidArchive
		}
	}
}

func (m *Manager) writeExtractedFile(
	ctx context.Context,
	targetVirtual string,
	input io.Reader,
	mode os.FileMode,
	modified time.Time,
	budget *copyBudget,
) error {
	if err := m.rootFS.MkdirAll(rootName(path.Dir(targetVirtual)), 0755); err != nil {
		return err
	}
	output, err := m.rootFS.OpenFile(
		rootName(targetVirtual), os.O_WRONLY|os.O_CREATE|os.O_EXCL, archiveMode(mode, false),
	)
	if err != nil {
		return err
	}
	success := false
	defer func() {
		_ = output.Close()
		if !success {
			_ = m.rootFS.Remove(rootName(targetVirtual))
		}
	}()
	remaining := budget.maxBytes - budget.bytes
	written, err := io.CopyBuffer(
		output,
		io.LimitReader(&contextReader{ctx: ctx, reader: input}, remaining+1),
		make([]byte, 64<<10),
	)
	if err != nil {
		return err
	}
	if written > remaining {
		return ErrTooLarge
	}
	budget.bytes += written
	archiveProgress(ctx, budget)
	if err := output.Sync(); err != nil {
		return err
	}
	if err := output.Close(); err != nil {
		return err
	}
	if !modified.IsZero() {
		if err := m.rootFS.Chtimes(rootName(targetVirtual), modified, modified); err != nil {
			return err
		}
	}
	success = true
	return nil
}

// Header count and payload bytes do not account for implicit parent
// directories. Bound distinct expanded objects before MkdirAll or export.
type archiveObjectKey struct {
	root *fileRoot
	path string
}

func (b *copyBudget) reserveArchivePath(root *fileRoot, target, name string) error {
	if b.archiveObjects == nil {
		b.archiveObjects = make(map[archiveObjectKey]struct{})
	}
	for current := name; current != "." && current != "/"; current = path.Dir(current) {
		// A batch shares its budget across distinct extraction destinations.
		// Each extraction has its own pinned root and starts at virtual "/".
		// Holding the root identity also prevents reuse after that root closes.
		key := archiveObjectKey{root: root, path: path.Join(target, current)}
		if _, exists := b.archiveObjects[key]; exists {
			return nil
		}
		if len(b.archiveObjects) >= b.maxEntries {
			return ErrTooLarge
		}
		b.archiveObjects[key] = struct{}{}
	}
	return nil
}

func (m *Manager) applyArchiveDirectoryTimes(values []archiveDirectoryTime) error {
	for index := len(values) - 1; index >= 0; index-- {
		value := values[index]
		if value.modified.IsZero() {
			continue
		}
		if err := m.rootFS.Chtimes(rootName(value.virtual), value.modified, value.modified); err != nil {
			return err
		}
	}
	return nil
}

func validateArchiveName(name, format string) error {
	if err := validateName(name); err != nil {
		return err
	}
	return validateArchiveSourceName(name, format)
}

func validateArchiveSourceName(name, format string) error {
	lower := strings.ToLower(name)
	switch format {
	case archiveFormatZIP:
		if strings.HasSuffix(lower, ".zip") {
			return nil
		}
	case archiveFormatTAR:
		if strings.HasSuffix(lower, ".tar") {
			return nil
		}
	case archiveFormatTARGZ:
		if strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".tgz") {
			return nil
		}
	}
	return ErrInvalidArchive
}

func normalizeArchiveEntry(value string) (string, error) {
	for strings.HasPrefix(value, "./") {
		value = strings.TrimPrefix(value, "./")
	}
	if value == "" || len(value) > maxPathBytes || strings.HasPrefix(value, "/") ||
		strings.Contains(value, `\`) || strings.ContainsRune(value, 0) {
		return "", ErrInvalidArchive
	}
	trimmed := strings.TrimSuffix(value, "/")
	if trimmed == "" {
		return "", ErrInvalidArchive
	}
	for _, component := range strings.Split(trimmed, "/") {
		if component == "" || component == "." || component == ".." || len(component) > 255 ||
			isInternalComponent(component) {
			return "", ErrInvalidArchive
		}
	}
	clean := path.Clean(trimmed)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", ErrInvalidArchive
	}
	return clean, nil
}

func archiveMode(mode os.FileMode, directory bool) os.FileMode {
	permission := mode.Perm()
	if permission != 0 {
		return permission
	}
	if directory {
		return 0755
	}
	return 0644
}
