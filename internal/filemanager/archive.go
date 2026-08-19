package filemanager

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"errors"
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
	virtual string
	info    os.FileInfo
}

type archiveEntryWriter func(
	ctx context.Context,
	archiveName string,
	sourceVirtual string,
	info os.FileInfo,
) error

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
	seenNames := make(map[string]struct{}, len(sources))
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
		base := path.Base(normalizedSource)
		if _, exists := seenNames[base]; exists {
			return nil, ErrAlreadyExists
		}
		seenNames[base] = struct{}{}
		prepared = append(prepared, archiveSourceEntry{virtual: normalizedSource, info: info})
	}
	return prepared, nil
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
	outputVirtual := joinVirtual(normalizedTarget, name)
	if err := m.mutationError(outputVirtual); err != nil {
		return contract.FileEntry{}, err
	}
	if _, err := m.rootFS.Lstat(rootName(outputVirtual)); err == nil {
		return contract.FileEntry{}, ErrAlreadyExists
	} else if !errors.Is(err, os.ErrNotExist) {
		return contract.FileEntry{}, err
	}

	prepared, err := m.prepareArchiveSources(sources, expectedVersions, outputVirtual)
	if err != nil {
		return contract.FileEntry{}, err
	}

	temp, tempVirtual, err := m.createTemp(normalizedTarget, ".kpanel-archive-")
	if err != nil {
		return contract.FileEntry{}, err
	}
	success := false
	encoderClosed := false
	var closeEncoder func() error
	defer func() {
		if !encoderClosed && closeEncoder != nil {
			_ = closeEncoder()
		}
		_ = temp.Close()
		if !success {
			_ = m.rootFS.Remove(rootName(tempVirtual))
		}
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

	budget := &copyBudget{maxEntries: m.maxCopyEntries, maxBytes: m.maxCopyBytes}
	stripSingleDirectory := len(prepared) == 1 && prepared[0].info.IsDir()
	for _, source := range prepared {
		archiveName := path.Base(source.virtual)
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
	if err := temp.Close(); err != nil {
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

func (m *Manager) walkArchive(
	ctx context.Context,
	sourceVirtual string,
	archiveName string,
	info os.FileInfo,
	budget *copyBudget,
	writeEntry archiveEntryWriter,
) error {
	if err := ctx.Err(); err != nil {
		return err
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
		if err := writeEntry(ctx, archiveName, sourceVirtual, info); err != nil {
			return err
		}
	}
	if !info.IsDir() {
		return nil
	}
	directory, err := m.rootFS.Open(rootName(sourceVirtual))
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
		childInfo, statErr := m.rootFS.Lstat(rootName(childVirtual))
		if statErr != nil {
			return statErr
		}
		childArchiveName := entry.Name()
		if archiveName != "" {
			childArchiveName = path.Join(archiveName, entry.Name())
		}
		if err := m.walkArchive(ctx, childVirtual, childArchiveName, childInfo, budget, writeEntry); err != nil {
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
	source, err := m.rootFS.Open(rootName(sourceVirtual))
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

	source, err := m.rootFS.Open(rootName(normalizedSource))
	if err != nil {
		return contract.FileEntry{}, err
	}
	defer source.Close()
	openedInfo, err := source.Stat()
	if err != nil || !os.SameFile(sourceInfo, openedInfo) {
		return contract.FileEntry{}, ErrConflict
	}
	budget := &copyBudget{maxEntries: m.maxCopyEntries, maxBytes: m.maxCopyBytes}
	seen := make(map[string]struct{})
	directoryTimes := make([]archiveDirectoryTime, 0)
	switch format {
	case archiveFormatZIP:
		if err := validateZIPDirectory(source, sourceInfo.Size(), budget.maxEntries); err != nil {
			return contract.FileEntry{}, err
		}
		reader, zipErr := zip.NewReader(source, sourceInfo.Size())
		if zipErr != nil {
			return contract.FileEntry{}, ErrInvalidArchive
		}
		if err := m.extractZIP(ctx, reader, tempVirtual, budget, seen, &directoryTimes); err != nil {
			return contract.FileEntry{}, err
		}
	case archiveFormatTAR, archiveFormatTARGZ:
		if format == archiveFormatTAR && sourceInfo.Size() == 0 {
			return contract.FileEntry{}, ErrInvalidArchive
		}
		var reader io.Reader = source
		var gzipReader *gzip.Reader
		if format == archiveFormatTARGZ {
			gzipReader, err = gzip.NewReader(source)
			if err != nil {
				return contract.FileEntry{}, ErrInvalidArchive
			}
			defer gzipReader.Close()
			reader = gzipReader
		}
		if err := m.extractTAR(ctx, tar.NewReader(&contextReader{ctx: ctx, reader: reader}), tempVirtual, budget, seen, &directoryTimes); err != nil {
			return contract.FileEntry{}, err
		}
	default:
		return contract.FileEntry{}, ErrInvalidArchive
	}
	if err := m.applyArchiveDirectoryTimes(directoryTimes); err != nil {
		return contract.FileEntry{}, err
	}
	currentInfo, err := source.Stat()
	if err != nil || resourceVersion(normalizedSource, currentInfo) != resourceVersion(normalizedSource, sourceInfo) {
		return contract.FileEntry{}, ErrConflict
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

func validateZIPDirectory(reader io.ReaderAt, size int64, maxEntries int) error {
	const (
		endRecordSize  = 22
		maxCommentSize = 1<<16 - 1
	)
	if size < endRecordSize {
		return ErrInvalidArchive
	}
	tailSize := int64(endRecordSize + maxCommentSize)
	if size < tailSize {
		tailSize = size
	}
	tail := make([]byte, tailSize)
	if _, err := reader.ReadAt(tail, size-tailSize); err != nil {
		return ErrInvalidArchive
	}
	signature := []byte{'P', 'K', 0x05, 0x06}
	index := len(tail) - endRecordSize
	for index >= 0 {
		candidate := bytes.LastIndex(tail[:index+len(signature)], signature)
		if candidate < 0 {
			return ErrInvalidArchive
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
		return ErrInvalidArchive
	}
	record := tail[index:]
	if binary.LittleEndian.Uint16(record[4:6]) != 0 ||
		binary.LittleEndian.Uint16(record[6:8]) != 0 {
		return ErrInvalidArchive
	}
	entriesOnDisk := binary.LittleEndian.Uint16(record[8:10])
	totalEntries := binary.LittleEndian.Uint16(record[10:12])
	if entriesOnDisk != totalEntries {
		return ErrInvalidArchive
	}
	if int(totalEntries) > maxEntries {
		return ErrTooLarge
	}
	return nil
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
		if info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
			return ErrInvalidArchive
		}
		budget.entries++
		if budget.entries > budget.maxEntries || entry.UncompressedSize64 > uint64(budget.maxBytes-budget.bytes) {
			return ErrTooLarge
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
			return ErrInvalidArchive
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
