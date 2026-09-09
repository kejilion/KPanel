package filemanager

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"os"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

const maxArchiveIndexBytes = 8 << 20

type archiveContextKey struct{}
type archiveOperation struct {
	selection  []string
	found      map[string]bool
	inspect    func(string, os.FileInfo, int64) error
	budget     *copyBudget
	progress   func(*copyBudget)
	tempSuffix string
}

type archiveIndex struct {
	mu      sync.Mutex
	key     string
	expires time.Time
	entries []contract.FileArchiveEntry
}

func archiveOptions(ctx context.Context) *archiveOperation {
	value, _ := ctx.Value(archiveContextKey{}).(*archiveOperation)
	return value
}

func archiveBudget(ctx context.Context, m *Manager) *copyBudget {
	if value := archiveOptions(ctx); value != nil && value.budget != nil {
		return value.budget
	}
	return &copyBudget{maxEntries: m.maxCopyEntries, maxBytes: m.maxCopyBytes}
}

func archiveProgress(ctx context.Context, budget *copyBudget) {
	if value := archiveOptions(ctx); value != nil && value.progress != nil {
		value.progress(budget)
	}
}

func archiveInclude(ctx context.Context, name string, info os.FileInfo, size int64) (bool, error) {
	value := archiveOptions(ctx)
	if value == nil {
		return true, nil
	}
	if value.inspect != nil {
		return false, value.inspect(name, info, size)
	}
	if len(value.selection) == 0 {
		return true, nil
	}
	include := false
	for _, selected := range value.selection {
		if name == selected || strings.HasPrefix(name, selected+"/") {
			value.found[selected] = true
			include = true
		}
	}
	return include, nil
}

func archiveFormatForName(name string) string {
	for _, format := range []string{archiveFormatTARGZ, archiveFormatZIP, archiveFormatTAR} {
		if validateArchiveSourceName(name, format) == nil {
			return format
		}
	}
	return ""
}

func validateArchiveSelection(selection []string) error {
	if len(selection) > MaxBatchItems {
		return ErrBatchTooLarge
	}
	for _, name := range selection {
		normalized, err := normalizeArchiveEntry(name)
		if err != nil || normalized != name {
			return ErrInvalidArchive
		}
	}
	return nil
}

// readArchive uses the exact extraction validator for browsing and extraction.
// Inspecting TAR consumes its bounded stream; ZIP only reads its bounded directory.
func (m *Manager) readArchive(ctx context.Context, source *os.File, size int64, format, target string, budget *copyBudget, times *[]archiveDirectoryTime) error {
	seen := make(map[string]struct{})
	switch format {
	case archiveFormatZIP:
		directory, err := boundedZIPDirectory(ctx, source, size, budget.maxEntries)
		if err != nil {
			return err
		}
		reader, err := zip.NewReader(directory, directory.offset+int64(len(directory.frozen)))
		if err != nil {
			return ErrInvalidArchive
		}
		return m.extractZIP(ctx, reader, target, budget, seen, times)
	case archiveFormatTAR, archiveFormatTARGZ:
		if size == 0 {
			return ErrInvalidArchive
		}
		var reader io.Reader = &contextReader{ctx: ctx, reader: source}
		if format == archiveFormatTARGZ {
			gz, err := gzip.NewReader(reader)
			if err != nil {
				return ErrInvalidArchive
			}
			defer gz.Close()
			reader = gz
		}
		if err := m.extractTAR(ctx, tar.NewReader(&contextReader{ctx: ctx, reader: reader}), target, budget, seen, times); err != nil {
			return err
		}
		// Read trailers/checksums too, with a separate bounded allowance for padding.
		n, err := io.Copy(io.Discard, io.LimitReader(&contextReader{ctx: ctx, reader: reader}, (1<<20)+1))
		if err != nil || n > 1<<20 {
			return ErrInvalidArchive
		}
		return ctx.Err()
	default:
		return ErrInvalidArchive
	}
}

func (m *Manager) ArchiveContents(ctx context.Context, query contract.FileArchiveQuery) (contract.FileArchiveDirectory, error) {
	result := contract.FileArchiveDirectory{Path: query.Path, ResourceVersion: query.ResourceVersion, Directory: query.Directory, Entries: []contract.FileArchiveEntry{}}
	if query.ResourceVersion == "" || query.Offset < 0 || query.Offset >= m.maxCopyEntries || len(query.Search) > MaxSearchBytes {
		return result, ErrInvalidPath
	}
	if query.Directory != "" {
		if err := validateArchiveSelection([]string{query.Directory}); err != nil {
			return result, err
		}
	}
	// A single 5 minute, byte-bounded metadata cache per Agent prevents duplicate scans.
	if !m.archiveIndex.mu.TryLock() {
		return result, ErrBusy
	}
	defer m.archiveIndex.mu.Unlock()
	_, sourcePath, err := m.resolveExisting(query.Path)
	if err != nil {
		return result, err
	}
	info, err := m.rootFS.Lstat(rootName(sourcePath))
	if err != nil {
		return result, err
	}
	if !info.Mode().IsRegular() {
		return result, ErrNotRegular
	}
	if resourceVersion(sourcePath, info) != query.ResourceVersion {
		return result, ErrConflict
	}
	key := sourcePath + "\x00" + query.ResourceVersion
	if m.archiveIndex.key != key || !m.now().Before(m.archiveIndex.expires) {
		// Drop the old index before allocating its replacement.
		m.archiveIndex.entries = nil
		m.archiveIndex.key = ""
		file, err := m.rootFS.Open(rootName(sourcePath))
		if err != nil {
			return result, err
		}
		defer file.Close()
		opened, err := file.Stat()
		if err != nil || !os.SameFile(info, opened) {
			return result, ErrConflict
		}
		entries := make(map[string]contract.FileArchiveEntry)
		bytesUsed := 0
		add := func(name string, directory bool, size int64, modified time.Time) error {
			if existing, ok := entries[name]; ok {
				if (existing.Kind == "directory") != directory {
					return ErrInvalidArchive
				}
				return nil
			}
			bytesUsed += len(name) + len(path.Base(name)) + 128
			if bytesUsed > maxArchiveIndexBytes || len(entries) >= m.maxCopyEntries {
				return ErrTooLarge
			}
			kind := "file"
			if directory {
				kind = "directory"
			}
			entries[name] = contract.FileArchiveEntry{Path: name, Name: path.Base(name), Kind: kind, SizeBytes: size, ModifiedAt: modified}
			return nil
		}
		operation := &archiveOperation{inspect: func(name string, info os.FileInfo, size int64) error {
			if err := add(name, info.IsDir(), size, info.ModTime()); err != nil {
				return err
			}
			for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
				if err := add(parent, true, 0, time.Time{}); err != nil {
					return err
				}
			}
			return nil
		}}
		bounded, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		times := []archiveDirectoryTime{}
		if err := m.readArchive(context.WithValue(bounded, archiveContextKey{}, operation), file, info.Size(), archiveFormatForName(sourcePath), "", archiveBudget(ctx, m), &times); err != nil {
			return result, err
		}
		current, err := file.Stat()
		if err != nil || resourceVersion(sourcePath, current) != query.ResourceVersion {
			return result, ErrConflict
		}
		if err := m.checkExpectedVersion(sourcePath, query.ResourceVersion); err != nil {
			return result, err
		}
		for _, entry := range entries {
			m.archiveIndex.entries = append(m.archiveIndex.entries, entry)
		}
		m.archiveIndex.key = key
		m.archiveIndex.expires = m.now().Add(5 * time.Minute)
	}
	matching := []contract.FileArchiveEntry{}
	for _, entry := range m.archiveIndex.entries {
		parent := path.Dir(entry.Path)
		if parent == "." {
			parent = ""
		}
		if query.Search != "" {
			if query.Directory != "" && !strings.HasPrefix(entry.Path, query.Directory+"/") {
				continue
			}
			if !strings.Contains(strings.ToLower(entry.Path), strings.ToLower(query.Search)) {
				continue
			}
		} else if parent != query.Directory {
			continue
		}
		matching = append(matching, entry)
	}
	sort.Slice(matching, func(i, j int) bool {
		if matching[i].Kind != matching[j].Kind {
			return matching[i].Kind == "directory"
		}
		return matching[i].Path < matching[j].Path
	})
	result.Total = len(matching)
	start := min(query.Offset, len(matching))
	end := min(start+MaxDirectoryEntries, len(matching))
	result.Entries = matching[start:end]
	if end < len(matching) {
		result.Truncated = true
		result.NextOffset = end
	}
	return result, nil
}

func archiveFailureDetail(err error) string {
	switch {
	case errors.Is(err, ErrConflict):
		return "文件状态已变化，请刷新后重试"
	case errors.Is(err, ErrAlreadyExists):
		return "目标已存在，请修改名称后重试"
	case errors.Is(err, ErrTooLarge), errors.Is(err, ErrBatchTooLarge):
		return "压缩包超过条目或容量上限"
	case errors.Is(err, ErrInvalidArchive):
		return "压缩包格式或内容无效"
	case errors.Is(err, context.Canceled):
		return "操作已停止，请核对目标目录"
	case errors.Is(err, context.DeadlineExceeded):
		return "操作超时，请核对目标目录"
	case errors.Is(err, os.ErrPermission):
		return "没有目标目录的读写权限"
	case errors.Is(err, os.ErrNotExist):
		return "文件或目标目录已不存在"
	default:
		return "文件操作未完成，请检查目标目录、可用空间及文件权限"
	}
}
