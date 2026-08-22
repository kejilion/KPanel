package filemanager

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

const (
	MaxDirectoryEntries = 500
	MaxBatchItems       = 100
	MaxTextBytes        = 2 << 20
	MaxUploadBytes      = 512 << 20
	MaxDirectoryScan    = 20_000
	MaxSearchBytes      = 128
	MaxTrashEntries     = 10_000
	MaxTrashListEntries = 500
	maxPathBytes        = 4096
	maxCopyEntries      = 10_000
	maxCopyBytes        = 10 << 30
)

var (
	ErrInvalidPath     = errors.New("文件路径无效")
	ErrProtected       = errors.New("KPanel 保护目录不可访问")
	ErrReadOnly        = errors.New("该系统目录仅支持查看，不能通过文件管理写入")
	ErrSymlink         = errors.New("不允许通过符号链接访问文件")
	ErrNotRegular      = errors.New("目标不是普通文件")
	ErrNotDirectory    = errors.New("目标不是目录")
	ErrConflict        = errors.New("文件状态已变化，请刷新后重试")
	ErrAlreadyExists   = errors.New("目标已存在")
	ErrTooLarge        = errors.New("文件超过允许的大小")
	ErrBatchTooLarge   = errors.New("批量操作最多支持 100 个项目")
	ErrAction          = errors.New("不支持的文件操作")
	ErrRootOperation   = errors.New("不能对文件根目录执行此操作")
	ErrInvalidEncoding = errors.New("文本内容必须使用 UTF-8 编码")
	ErrBusy            = errors.New("文件传输任务繁忙，请稍后重试")
	ErrTrashFull       = errors.New("回收站项目已达到上限，请先清理回收站")
	ErrTrashMetadata   = errors.New("回收站项目缺少恢复信息，只能彻底删除")
)

type Manager struct {
	root           string
	rootFS         *os.Root
	protected      []string
	readOnly       []string
	trashRoot      string
	trashInfo      string
	now            func() time.Time
	uploadGate     chan struct{}
	downloadGate   chan struct{}
	maxCopyEntries int
	maxCopyBytes   int64
	writeMu        sync.Mutex
}

type Config struct {
	Root             string
	ProtectedVirtual []string
	ReadOnlyVirtual  []string
	TrashVirtual     string
	Now              func() time.Time
	MaxCopyEntries   int
	MaxCopyBytes     int64
}

type ListOptions struct {
	Limit  int
	Offset int
	Search string
}

type trashMetadata struct {
	ID           string    `json:"id"`
	OriginalPath string    `json:"originalPath"`
	Name         string    `json:"name"`
	DeletedAt    time.Time `json:"deletedAt"`
}

func New(config Config) (*Manager, error) {
	if strings.TrimSpace(config.Root) == "" || !filepath.IsAbs(config.Root) {
		return nil, errors.New("file manager root must be absolute")
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	if config.MaxCopyEntries <= 0 || config.MaxCopyEntries > maxCopyEntries {
		config.MaxCopyEntries = maxCopyEntries
	}
	if config.MaxCopyBytes <= 0 || config.MaxCopyBytes > maxCopyBytes {
		config.MaxCopyBytes = maxCopyBytes
	}
	rootPath := filepath.Clean(config.Root)
	rootFS, err := os.OpenRoot(rootPath)
	if err != nil {
		return nil, fmt.Errorf("open file manager root: %w", err)
	}
	manager := &Manager{
		root:           rootPath,
		rootFS:         rootFS,
		now:            config.Now,
		uploadGate:     make(chan struct{}, 2),
		downloadGate:   make(chan struct{}, 4),
		maxCopyEntries: config.MaxCopyEntries,
		maxCopyBytes:   config.MaxCopyBytes,
	}
	trashVirtual := strings.TrimSpace(config.TrashVirtual)
	if trashVirtual == "" {
		trashVirtual = "/.kpanel-trash"
	}
	trashVirtual, err = normalizeVirtual(trashVirtual)
	if err != nil || trashVirtual == "/" {
		_ = rootFS.Close()
		return nil, fmt.Errorf("invalid trash path %q", config.TrashVirtual)
	}
	manager.trashRoot = joinVirtual(trashVirtual, "files")
	manager.trashInfo = joinVirtual(trashVirtual, "info")
	protectedValues := append([]string{trashVirtual}, config.ProtectedVirtual...)
	seenProtected := make(map[string]struct{}, len(protectedValues))
	for _, value := range protectedValues {
		normalized, err := normalizeVirtual(value)
		if err != nil || normalized == "/" {
			_ = rootFS.Close()
			return nil, fmt.Errorf("invalid protected file path %q", value)
		}
		if _, exists := seenProtected[normalized]; exists {
			continue
		}
		seenProtected[normalized] = struct{}{}
		manager.protected = append(manager.protected, normalized)
	}
	sort.Strings(manager.protected)
	for _, value := range config.ReadOnlyVirtual {
		normalized, normalizeErr := normalizeVirtual(value)
		if normalizeErr != nil || normalized == "/" {
			_ = rootFS.Close()
			return nil, fmt.Errorf("invalid read-only file path %q", value)
		}
		manager.readOnly = append(manager.readOnly, normalized)
	}
	sort.Strings(manager.readOnly)
	return manager, nil
}

func (m *Manager) Available() error {
	info, err := m.rootFS.Stat(".")
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return ErrNotDirectory
	}
	return nil
}

func (m *Manager) Close() error {
	return m.rootFS.Close()
}

func (m *Manager) List(ctx context.Context, virtual string, limit int) (contract.FileDirectory, error) {
	return m.ListPage(ctx, virtual, ListOptions{Limit: limit})
}

func (m *Manager) ListPage(
	ctx context.Context,
	virtual string,
	options ListOptions,
) (contract.FileDirectory, error) {
	if options.Limit <= 0 || options.Limit > MaxDirectoryEntries {
		options.Limit = MaxDirectoryEntries
	}
	if options.Offset < 0 || options.Offset >= MaxDirectoryScan {
		return contract.FileDirectory{}, ErrInvalidPath
	}
	search := strings.TrimSpace(options.Search)
	if len(search) > MaxSearchBytes {
		return contract.FileDirectory{}, ErrInvalidPath
	}
	search = strings.ToLower(search)
	_, normalized, err := m.resolveExisting(virtual)
	if err != nil {
		return contract.FileDirectory{}, err
	}
	info, err := m.rootFS.Lstat(rootName(normalized))
	if err != nil {
		return contract.FileDirectory{}, err
	}
	if !info.IsDir() {
		return contract.FileDirectory{}, ErrNotDirectory
	}
	directory, err := m.rootFS.Open(rootName(normalized))
	if err != nil {
		return contract.FileDirectory{}, err
	}
	defer directory.Close()
	entries := make([]contract.FileEntry, 0, options.Limit)
	scanned := 0
	scanTruncated := false
	hasMore := false
	reachedEOF := false
	matched := 0
	for {
		values, readErr := directory.ReadDir(128)
		for _, value := range values {
			if err := ctx.Err(); err != nil {
				return contract.FileDirectory{}, err
			}
			childVirtual := joinVirtual(normalized, value.Name())
			if m.isProtected(childVirtual) || isInternalComponent(value.Name()) {
				continue
			}
			if scanned == MaxDirectoryScan {
				scanTruncated = true
				break
			}
			scanned++
			if search != "" && !strings.Contains(strings.ToLower(value.Name()), search) {
				continue
			}
			if matched < options.Offset {
				matched++
				continue
			}
			if len(entries) == options.Limit {
				hasMore = true
				break
			}
			matched++
			childInfo, infoErr := value.Info()
			if infoErr != nil {
				continue
			}
			entries = append(entries, m.entry(childVirtual, childInfo))
		}
		if hasMore || scanTruncated {
			break
		}
		if errors.Is(readErr, io.EOF) {
			reachedEOF = true
			break
		}
		if readErr != nil {
			return contract.FileDirectory{}, readErr
		}
	}
	sort.Slice(entries, func(left, right int) bool {
		leftDir := entries[left].Kind == "directory"
		rightDir := entries[right].Kind == "directory"
		if leftDir != rightDir {
			return leftDir
		}
		return strings.ToLower(entries[left].Name) < strings.ToLower(entries[right].Name)
	})
	nextOffset := 0
	if hasMore {
		nextOffset = matched
	}
	total := 0
	if reachedEOF {
		total = matched
	}
	return contract.FileDirectory{
		Path: normalized, Entries: entries,
		Offset: options.Offset, NextOffset: nextOffset, Total: total, TotalKnown: reachedEOF,
		Truncated: hasMore || scanTruncated, ScanTruncated: scanTruncated, ReadAt: m.now().UTC(),
	}, nil
}

func (m *Manager) Stat(virtual string) (contract.FileEntry, error) {
	_, normalized, err := m.resolveExisting(virtual)
	if err != nil {
		return contract.FileEntry{}, err
	}
	info, err := m.rootFS.Lstat(rootName(normalized))
	if err != nil {
		return contract.FileEntry{}, err
	}
	return m.entry(normalized, info), nil
}

func (m *Manager) Open(ctx context.Context, virtual string) (io.ReadSeekCloser, contract.FileEntry, error) {
	if err := acquireNow(ctx, m.downloadGate); err != nil {
		return nil, contract.FileEntry{}, err
	}
	releaseDownload := true
	defer func() {
		if releaseDownload {
			release(m.downloadGate)
		}
	}()
	_, normalized, err := m.resolveExisting(virtual)
	if err != nil {
		return nil, contract.FileEntry{}, err
	}
	info, err := m.rootFS.Lstat(rootName(normalized))
	if err != nil {
		return nil, contract.FileEntry{}, err
	}
	if !info.Mode().IsRegular() {
		return nil, contract.FileEntry{}, ErrNotRegular
	}
	file, err := m.rootFS.Open(rootName(normalized))
	if err != nil {
		return nil, contract.FileEntry{}, err
	}
	opened, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, contract.FileEntry{}, err
	}
	if !os.SameFile(info, opened) || !opened.Mode().IsRegular() {
		file.Close()
		return nil, contract.FileEntry{}, ErrConflict
	}
	releaseDownload = false
	return &gatedFile{File: file, gate: m.downloadGate}, m.entry(normalized, opened), nil
}

func (m *Manager) WriteText(
	ctx context.Context,
	virtual string,
	input contract.FileWriteRequest,
) (contract.FileEntry, error) {
	if len(input.Content) > MaxTextBytes {
		return contract.FileEntry{}, ErrTooLarge
	}
	if !utf8.ValidString(input.Content) || strings.ContainsRune(input.Content, 0) {
		return contract.FileEntry{}, ErrInvalidEncoding
	}
	m.writeMu.Lock()
	defer m.writeMu.Unlock()
	_, normalized, err := m.resolveExisting(virtual)
	if err != nil {
		return contract.FileEntry{}, err
	}
	if err := m.mutationError(normalized); err != nil {
		return contract.FileEntry{}, err
	}
	info, err := m.rootFS.Lstat(rootName(normalized))
	if err != nil {
		return contract.FileEntry{}, err
	}
	if !info.Mode().IsRegular() {
		return contract.FileEntry{}, ErrNotRegular
	}
	current := m.entry(normalized, info)
	if input.ExpectedResourceVersion == "" ||
		input.ExpectedResourceVersion != current.ResourceVersion {
		return contract.FileEntry{}, ErrConflict
	}
	if err := ctx.Err(); err != nil {
		return contract.FileEntry{}, err
	}
	source, err := m.rootFS.Open(rootName(normalized))
	if err != nil {
		return contract.FileEntry{}, err
	}
	defer source.Close()
	opened, err := source.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return contract.FileEntry{}, ErrConflict
	}
	parentVirtual := path.Dir(normalized)
	temp, tempVirtual, err := m.createTemp(parentVirtual, ".kpanel-edit-")
	if err != nil {
		return contract.FileEntry{}, err
	}
	success := false
	defer func() {
		temp.Close()
		if !success {
			_ = m.rootFS.Remove(rootName(tempVirtual))
		}
	}()
	if err := temp.Chmod(info.Mode().Perm()); err != nil {
		return contract.FileEntry{}, err
	}
	if err := preserveFileOwnership(temp, info); err != nil {
		return contract.FileEntry{}, err
	}
	if err := preserveFileExtendedAttributes(temp, source); err != nil {
		return contract.FileEntry{}, err
	}
	if _, err := io.WriteString(temp, input.Content); err != nil {
		return contract.FileEntry{}, err
	}
	if err := temp.Sync(); err != nil {
		return contract.FileEntry{}, err
	}
	if err := temp.Close(); err != nil {
		return contract.FileEntry{}, err
	}
	if err := m.rootFS.Rename(rootName(tempVirtual), rootName(normalized)); err != nil {
		return contract.FileEntry{}, err
	}
	if err := syncRootDirectory(m.rootFS, rootName(parentVirtual)); err != nil {
		return contract.FileEntry{}, err
	}
	success = true
	return m.Stat(normalized)
}

func (m *Manager) Upload(
	ctx context.Context,
	directoryVirtual, name string,
	content io.Reader,
	contentLength int64,
	overwrite bool,
) (contract.FileEntry, error) {
	if contentLength > MaxUploadBytes {
		return contract.FileEntry{}, ErrTooLarge
	}
	if err := validateName(name); err != nil {
		return contract.FileEntry{}, err
	}
	if err := acquireNow(ctx, m.uploadGate); err != nil {
		return contract.FileEntry{}, err
	}
	defer release(m.uploadGate)
	_, normalizedDirectory, err := m.resolveExisting(directoryVirtual)
	if err != nil {
		return contract.FileEntry{}, err
	}
	info, err := m.rootFS.Lstat(rootName(normalizedDirectory))
	if err != nil {
		return contract.FileEntry{}, err
	}
	if !info.IsDir() {
		return contract.FileEntry{}, ErrNotDirectory
	}
	targetVirtual := joinVirtual(normalizedDirectory, name)
	if err := m.mutationError(targetVirtual); err != nil {
		return contract.FileEntry{}, err
	}
	var existing os.FileInfo
	if value, statErr := m.rootFS.Lstat(rootName(targetVirtual)); statErr == nil {
		if !overwrite {
			return contract.FileEntry{}, ErrAlreadyExists
		}
		if !value.Mode().IsRegular() {
			return contract.FileEntry{}, ErrNotRegular
		}
		existing = value
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return contract.FileEntry{}, statErr
	}
	temp, tempVirtual, err := m.createTemp(normalizedDirectory, ".kpanel-upload-")
	if err != nil {
		return contract.FileEntry{}, err
	}
	if existing != nil {
		source, openErr := m.rootFS.Open(rootName(targetVirtual))
		if openErr != nil {
			temp.Close()
			_ = m.rootFS.Remove(rootName(tempVirtual))
			return contract.FileEntry{}, openErr
		}
		defer source.Close()
		if err := temp.Chmod(existing.Mode().Perm()); err != nil {
			temp.Close()
			_ = m.rootFS.Remove(rootName(tempVirtual))
			return contract.FileEntry{}, err
		}
		if err := preserveFileOwnership(temp, existing); err != nil {
			temp.Close()
			_ = m.rootFS.Remove(rootName(tempVirtual))
			return contract.FileEntry{}, err
		}
		if err := preserveFileExtendedAttributes(temp, source); err != nil {
			temp.Close()
			_ = m.rootFS.Remove(rootName(tempVirtual))
			return contract.FileEntry{}, err
		}
	} else if err := temp.Chmod(0644); err != nil {
		temp.Close()
		_ = m.rootFS.Remove(rootName(tempVirtual))
		return contract.FileEntry{}, err
	}
	success := false
	defer func() {
		temp.Close()
		if !success {
			_ = m.rootFS.Remove(rootName(tempVirtual))
		}
	}()
	reader := &contextReader{ctx: ctx, reader: io.LimitReader(content, MaxUploadBytes+1)}
	written, err := io.CopyBuffer(temp, reader, make([]byte, 64<<10))
	if err != nil {
		return contract.FileEntry{}, err
	}
	if written > MaxUploadBytes {
		return contract.FileEntry{}, ErrTooLarge
	}
	if err := temp.Sync(); err != nil {
		return contract.FileEntry{}, err
	}
	if err := temp.Close(); err != nil {
		return contract.FileEntry{}, err
	}
	m.writeMu.Lock()
	defer m.writeMu.Unlock()
	if !overwrite {
		if _, err := m.rootFS.Lstat(rootName(targetVirtual)); err == nil {
			return contract.FileEntry{}, ErrAlreadyExists
		}
	} else if current, err := m.rootFS.Lstat(rootName(targetVirtual)); err == nil {
		if existing == nil ||
			resourceVersion(targetVirtual, current) != resourceVersion(targetVirtual, existing) {
			return contract.FileEntry{}, ErrConflict
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return contract.FileEntry{}, err
	}
	if overwrite {
		if err := m.rootFS.Rename(rootName(tempVirtual), rootName(targetVirtual)); err != nil {
			return contract.FileEntry{}, err
		}
	} else if err := renameNoReplaceRoot(m.rootFS, tempVirtual, targetVirtual); err != nil {
		if errors.Is(err, os.ErrExist) {
			return contract.FileEntry{}, ErrAlreadyExists
		}
		return contract.FileEntry{}, err
	}
	if err := syncRootDirectory(m.rootFS, rootName(normalizedDirectory)); err != nil {
		return contract.FileEntry{}, err
	}
	success = true
	return m.Stat(targetVirtual)
}

func (m *Manager) Action(
	ctx context.Context,
	input contract.FileActionRequest,
) (contract.FileActionResult, error) {
	m.writeMu.Lock()
	defer m.writeMu.Unlock()
	result := contract.FileActionResult{
		Action: input.Action, Succeeded: make([]contract.FileActionItem, 0),
		Failed: make([]contract.FileActionFailure, 0),
	}
	switch input.Action {
	case "mkdir":
		entry, err := m.mkdir(input.Target, input.Name)
		if err != nil {
			return result, err
		}
		result.Succeeded = append(result.Succeeded, actionItem(entry.Path, entry, ""))
		return result, nil
	case "rename":
		if len(input.Sources) != 1 {
			return result, ErrAction
		}
		entry, err := m.rename(input.Sources[0], input.Target, input.ExpectedResourceVersion)
		if err != nil {
			return result, err
		}
		result.Succeeded = append(result.Succeeded, actionItem(input.Sources[0], entry, entry.Path))
		return result, nil
	case "compress":
		if len(input.Sources) == 0 {
			return result, ErrAction
		}
		if len(input.Sources) > MaxBatchItems {
			return result, ErrBatchTooLarge
		}
		entry, err := m.compressArchive(
			ctx, input.Sources, input.Target, input.Name, input.Format,
			input.ExpectedResourceVersions,
		)
		if err != nil {
			return result, err
		}
		result.Succeeded = append(result.Succeeded, actionItem(entry.Path, entry, entry.Path))
		return result, nil
	case "extract":
		if len(input.Sources) != 1 {
			return result, ErrAction
		}
		entry, err := m.extractArchive(
			ctx, input.Sources[0], input.Target, input.Name, input.Format,
			input.ExpectedResourceVersion,
		)
		if err != nil {
			return result, err
		}
		result.Succeeded = append(result.Succeeded, actionItem(input.Sources[0], entry, entry.Path))
		return result, nil
	case "trash_restore", "trash_delete":
		if len(input.TrashIDs) == 0 {
			return result, ErrAction
		}
		if len(input.TrashIDs) > MaxBatchItems {
			return result, ErrBatchTooLarge
		}
	case "trash_empty":
		return m.emptyTrash(ctx, result)
	case "copy", "move", "trash", "chmod":
		if len(input.Sources) == 0 {
			return result, ErrAction
		}
		if len(input.Sources) > MaxBatchItems {
			return result, ErrBatchTooLarge
		}
	default:
		return result, ErrAction
	}
	budget := &copyBudget{maxEntries: m.maxCopyEntries, maxBytes: m.maxCopyBytes}
	items := input.Sources
	if input.Action == "trash_restore" || input.Action == "trash_delete" {
		items = input.TrashIDs
	}
	for _, source := range items {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		var (
			entry       contract.FileEntry
			destination string
			err         error
		)
		switch input.Action {
		case "copy":
			if err = m.checkExpectedVersion(source, input.ExpectedResourceVersions[source]); err != nil {
				break
			}
			entry, err = m.copyOne(ctx, source, input.Target, budget)
			destination = entry.Path
		case "move":
			if err = m.checkExpectedVersion(source, input.ExpectedResourceVersions[source]); err != nil {
				break
			}
			entry, err = m.moveOne(ctx, source, input.Target, budget)
			destination = entry.Path
		case "trash":
			if err = m.checkExpectedVersion(source, input.ExpectedResourceVersions[source]); err != nil {
				break
			}
			destination, err = m.trashOne(ctx, source, budget)
			entry = contract.FileEntry{Path: source}
		case "chmod":
			if err = m.checkExpectedVersion(source, input.ExpectedResourceVersions[source]); err != nil {
				break
			}
			entry, err = m.chmodOne(source, input.Mode)
		case "trash_restore":
			entry, err = m.restoreTrash(ctx, source, input.ExpectedResourceVersions[source], budget)
			destination = entry.Path
		case "trash_delete":
			err = m.deleteTrash(source, input.ExpectedResourceVersions[source])
			entry = contract.FileEntry{Path: source}
		}
		if err != nil {
			result.Failed = append(result.Failed, contract.FileActionFailure{
				Path: source, Detail: err.Error(),
			})
			continue
		}
		result.Succeeded = append(result.Succeeded, actionItem(source, entry, destination))
	}
	return result, nil
}

func (m *Manager) ListTrash(ctx context.Context) (contract.FileTrashDirectory, error) {
	if err := ctx.Err(); err != nil {
		return contract.FileTrashDirectory{}, err
	}
	m.writeMu.Lock()
	defer m.writeMu.Unlock()
	result := contract.FileTrashDirectory{Entries: make([]contract.FileTrashEntry, 0), ReadAt: m.now().UTC()}
	directory, err := m.rootFS.Open(rootName(m.trashRoot))
	if errors.Is(err, os.ErrNotExist) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	defer directory.Close()
	values, err := directory.ReadDir(MaxTrashEntries + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return result, err
	}
	if len(values) > MaxTrashEntries {
		return result, ErrTrashFull
	}
	for _, value := range values {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		id := value.Name()
		if validateTrashID(id) != nil || strings.HasPrefix(id, ".kpanel-") {
			continue
		}
		info, statErr := m.rootFS.Lstat(rootName(joinVirtual(m.trashRoot, id)))
		if statErr != nil || info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		metadata, metadataErr := m.readTrashMetadata(id)
		entry := m.entry(joinVirtual(m.trashRoot, id), info)
		name := id
		originalPath := ""
		deletedAt := info.ModTime().UTC()
		restorable := false
		if metadataErr == nil {
			name = metadata.Name
			originalPath = metadata.OriginalPath
			deletedAt = metadata.DeletedAt
			restorable = true
		}
		result.Entries = append(result.Entries, contract.FileTrashEntry{
			ID: id, Name: name, OriginalPath: originalPath, Kind: entry.Kind,
			SizeBytes: entry.SizeBytes, Mode: entry.Mode, Owner: entry.Owner, Group: entry.Group,
			DeletedAt: deletedAt, ResourceVersion: entry.ResourceVersion, Restorable: restorable,
		})
	}
	sort.Slice(result.Entries, func(left, right int) bool {
		return result.Entries[left].DeletedAt.After(result.Entries[right].DeletedAt)
	})
	result.Total = len(result.Entries)
	if len(result.Entries) > MaxTrashListEntries {
		result.Entries = result.Entries[:MaxTrashListEntries]
		result.Truncated = true
	}
	return result, nil
}

func (m *Manager) mkdir(parentVirtual, name string) (contract.FileEntry, error) {
	if err := validateName(name); err != nil {
		return contract.FileEntry{}, err
	}
	_, normalized, err := m.resolveExisting(parentVirtual)
	if err != nil {
		return contract.FileEntry{}, err
	}
	info, err := m.rootFS.Lstat(rootName(normalized))
	if err != nil {
		return contract.FileEntry{}, err
	}
	if !info.IsDir() {
		return contract.FileEntry{}, ErrNotDirectory
	}
	targetVirtual := joinVirtual(normalized, name)
	if err := m.mutationError(targetVirtual); err != nil {
		return contract.FileEntry{}, err
	}
	if err := m.rootFS.Mkdir(rootName(targetVirtual), 0755); err != nil {
		if errors.Is(err, os.ErrExist) {
			return contract.FileEntry{}, ErrAlreadyExists
		}
		return contract.FileEntry{}, err
	}
	if err := syncRootDirectory(m.rootFS, rootName(normalized)); err != nil {
		return contract.FileEntry{}, err
	}
	return m.Stat(targetVirtual)
}

func (m *Manager) rename(
	sourceVirtual, targetVirtual, expectedVersion string,
) (contract.FileEntry, error) {
	_, normalizedSource, err := m.resolveExisting(sourceVirtual)
	if err != nil {
		return contract.FileEntry{}, err
	}
	if normalizedSource == "/" {
		return contract.FileEntry{}, ErrRootOperation
	}
	if err := m.mutationError(normalizedSource); err != nil {
		return contract.FileEntry{}, err
	}
	sourceInfo, err := m.rootFS.Lstat(rootName(normalizedSource))
	if err != nil {
		return contract.FileEntry{}, err
	}
	if expectedVersion != "" &&
		m.entry(normalizedSource, sourceInfo).ResourceVersion != expectedVersion {
		return contract.FileEntry{}, ErrConflict
	}
	parentVirtual, name := path.Split(targetVirtual)
	parentVirtual = strings.TrimSuffix(parentVirtual, "/")
	if parentVirtual == "" {
		parentVirtual = "/"
	}
	if err := validateName(name); err != nil {
		return contract.FileEntry{}, err
	}
	_, normalizedParent, err := m.resolveExisting(parentVirtual)
	if err != nil {
		return contract.FileEntry{}, err
	}
	targetNormalized := joinVirtual(normalizedParent, name)
	if err := m.mutationError(targetNormalized); err != nil {
		return contract.FileEntry{}, err
	}
	if _, err := m.rootFS.Lstat(rootName(targetNormalized)); err == nil {
		return contract.FileEntry{}, ErrAlreadyExists
	} else if !errors.Is(err, os.ErrNotExist) {
		return contract.FileEntry{}, err
	}
	if err := renameNoReplaceRoot(m.rootFS, normalizedSource, targetNormalized); err != nil {
		if errors.Is(err, os.ErrExist) {
			return contract.FileEntry{}, ErrAlreadyExists
		}
		return contract.FileEntry{}, err
	}
	sourceParent := path.Dir(normalizedSource)
	if err := syncRootDirectory(m.rootFS, rootName(sourceParent)); err != nil {
		return contract.FileEntry{}, err
	}
	if sourceParent != normalizedParent {
		if err := syncRootDirectory(m.rootFS, rootName(normalizedParent)); err != nil {
			return contract.FileEntry{}, err
		}
	}
	return m.Stat(targetNormalized)
}

func (m *Manager) moveOne(
	ctx context.Context,
	sourceVirtual, targetDirectoryVirtual string,
	budget *copyBudget,
) (contract.FileEntry, error) {
	_, normalizedSource, err := m.resolveExisting(sourceVirtual)
	if err != nil {
		return contract.FileEntry{}, err
	}
	if normalizedSource == "/" {
		return contract.FileEntry{}, ErrRootOperation
	}
	if err := m.mutationError(normalizedSource); err != nil {
		return contract.FileEntry{}, err
	}
	sourceInfo, err := m.rootFS.Lstat(rootName(normalizedSource))
	if err != nil {
		return contract.FileEntry{}, err
	}
	sourceVersion := resourceVersion(normalizedSource, sourceInfo)
	_, normalizedTarget, err := m.resolveExisting(targetDirectoryVirtual)
	if err != nil {
		return contract.FileEntry{}, err
	}
	info, err := m.rootFS.Lstat(rootName(normalizedTarget))
	if err != nil {
		return contract.FileEntry{}, err
	}
	if !info.IsDir() {
		return contract.FileEntry{}, ErrNotDirectory
	}
	name := path.Base(normalizedSource)
	targetVirtual := joinVirtual(normalizedTarget, name)
	if err := m.mutationError(targetVirtual); err != nil {
		return contract.FileEntry{}, err
	}
	if targetVirtual == normalizedSource || isWithin(targetVirtual, normalizedSource) {
		return contract.FileEntry{}, ErrInvalidPath
	}
	if _, err := m.rootFS.Lstat(rootName(targetVirtual)); err == nil {
		return contract.FileEntry{}, ErrAlreadyExists
	} else if !errors.Is(err, os.ErrNotExist) {
		return contract.FileEntry{}, err
	}
	if err := renameNoReplaceRoot(m.rootFS, normalizedSource, targetVirtual); err != nil {
		if !isCrossDeviceError(err) {
			if errors.Is(err, os.ErrExist) {
				return contract.FileEntry{}, ErrAlreadyExists
			}
			return contract.FileEntry{}, err
		}
		tempVirtual := joinVirtual(normalizedTarget, ".kpanel-copy-"+randomID())
		if err := m.copyTree(ctx, normalizedSource, tempVirtual, budget); err != nil {
			_ = m.rootFS.RemoveAll(rootName(tempVirtual))
			return contract.FileEntry{}, err
		}
		currentSourceInfo, statErr := m.rootFS.Lstat(rootName(normalizedSource))
		if statErr != nil || resourceVersion(normalizedSource, currentSourceInfo) != sourceVersion {
			_ = m.rootFS.RemoveAll(rootName(tempVirtual))
			return contract.FileEntry{}, ErrConflict
		}
		if err := renameNoReplaceRoot(m.rootFS, tempVirtual, targetVirtual); err != nil {
			_ = m.rootFS.RemoveAll(rootName(tempVirtual))
			return contract.FileEntry{}, err
		}
		if err := m.rootFS.RemoveAll(rootName(normalizedSource)); err != nil {
			return contract.FileEntry{}, fmt.Errorf("目标已复制但源文件清理失败: %w", err)
		}
	}
	sourceParent := path.Dir(normalizedSource)
	if err := syncRootDirectory(m.rootFS, rootName(sourceParent)); err != nil {
		return contract.FileEntry{}, err
	}
	if sourceParent != normalizedTarget {
		if err := syncRootDirectory(m.rootFS, rootName(normalizedTarget)); err != nil {
			return contract.FileEntry{}, err
		}
	}
	return m.Stat(targetVirtual)
}

func (m *Manager) copyOne(
	ctx context.Context,
	sourceVirtual, targetDirectoryVirtual string,
	budget *copyBudget,
) (contract.FileEntry, error) {
	_, normalizedSource, err := m.resolveExisting(sourceVirtual)
	if err != nil {
		return contract.FileEntry{}, err
	}
	if normalizedSource == "/" {
		return contract.FileEntry{}, ErrRootOperation
	}
	if m.isMutationProtected(normalizedSource) {
		return contract.FileEntry{}, ErrProtected
	}
	_, normalizedTarget, err := m.resolveExisting(targetDirectoryVirtual)
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
	name := path.Base(normalizedSource)
	targetVirtual := joinVirtual(normalizedTarget, name)
	if err := m.mutationError(targetVirtual); err != nil {
		return contract.FileEntry{}, err
	}
	if targetVirtual == normalizedSource || isWithin(targetVirtual, normalizedSource) {
		return contract.FileEntry{}, ErrInvalidPath
	}
	if _, err := m.rootFS.Lstat(rootName(targetVirtual)); err == nil {
		return contract.FileEntry{}, ErrAlreadyExists
	} else if !errors.Is(err, os.ErrNotExist) {
		return contract.FileEntry{}, err
	}
	tempVirtual := joinVirtual(normalizedTarget, ".kpanel-copy-"+randomID())
	sourceInfo, err := m.rootFS.Lstat(rootName(normalizedSource))
	if err != nil {
		return contract.FileEntry{}, err
	}
	sourceVersion := resourceVersion(normalizedSource, sourceInfo)
	if err := m.copyTree(ctx, normalizedSource, tempVirtual, budget); err != nil {
		_ = m.rootFS.RemoveAll(rootName(tempVirtual))
		return contract.FileEntry{}, err
	}
	currentSourceInfo, err := m.rootFS.Lstat(rootName(normalizedSource))
	if err != nil || resourceVersion(normalizedSource, currentSourceInfo) != sourceVersion {
		_ = m.rootFS.RemoveAll(rootName(tempVirtual))
		return contract.FileEntry{}, ErrConflict
	}
	if err := renameNoReplaceRoot(m.rootFS, tempVirtual, targetVirtual); err != nil {
		_ = m.rootFS.RemoveAll(rootName(tempVirtual))
		return contract.FileEntry{}, err
	}
	if err := syncRootDirectory(m.rootFS, rootName(normalizedTarget)); err != nil {
		return contract.FileEntry{}, err
	}
	return m.Stat(targetVirtual)
}

func (m *Manager) trashOne(
	ctx context.Context,
	sourceVirtual string,
	budget *copyBudget,
) (string, error) {
	_, normalized, err := m.resolveExisting(sourceVirtual)
	if err != nil {
		return "", err
	}
	if normalized == "/" {
		return "", ErrRootOperation
	}
	if err := m.mutationError(normalized); err != nil {
		return "", err
	}
	sourceInfo, err := m.rootFS.Lstat(rootName(normalized))
	if err != nil {
		return "", err
	}
	sourceVersion := resourceVersion(normalized, sourceInfo)
	if err := m.ensureTrashDirectories(); err != nil {
		return "", err
	}
	if count, countErr := m.trashCount(); countErr != nil {
		return "", countErr
	} else if count >= MaxTrashEntries {
		return "", ErrTrashFull
	}
	id := m.now().UTC().Format("20060102T150405") + "-" + randomID()
	metadata := trashMetadata{
		ID: id, OriginalPath: normalized, Name: path.Base(normalized), DeletedAt: m.now().UTC(),
	}
	if err := m.writeTrashMetadata(metadata); err != nil {
		return "", err
	}
	metadataWritten := true
	defer func() {
		if metadataWritten {
			_ = m.rootFS.Remove(rootName(m.trashMetadataPath(id)))
		}
	}()
	targetVirtual := joinVirtual(m.trashRoot, id)
	if err := renameNoReplaceRoot(m.rootFS, normalized, targetVirtual); err != nil {
		if !isCrossDeviceError(err) {
			return "", err
		}
		tempVirtual := joinVirtual(m.trashRoot, ".kpanel-copy-"+randomID())
		if err := m.copyTree(ctx, normalized, tempVirtual, budget); err != nil {
			_ = m.rootFS.RemoveAll(rootName(tempVirtual))
			return "", err
		}
		currentSourceInfo, statErr := m.rootFS.Lstat(rootName(normalized))
		if statErr != nil || resourceVersion(normalized, currentSourceInfo) != sourceVersion {
			_ = m.rootFS.RemoveAll(rootName(tempVirtual))
			return "", ErrConflict
		}
		if err := renameNoReplaceRoot(m.rootFS, tempVirtual, targetVirtual); err != nil {
			_ = m.rootFS.RemoveAll(rootName(tempVirtual))
			return "", err
		}
		if err := m.rootFS.RemoveAll(rootName(normalized)); err != nil {
			return "", fmt.Errorf("文件已进入回收区但原文件清理失败: %w", err)
		}
	}
	metadataWritten = false
	if err := syncRootDirectory(m.rootFS, rootName(path.Dir(normalized))); err != nil {
		return "", err
	}
	if err := syncRootDirectory(m.rootFS, rootName(m.trashRoot)); err != nil {
		return "", err
	}
	return targetVirtual, nil
}

func (m *Manager) ensureTrashDirectories() error {
	for _, directory := range []string{path.Dir(m.trashRoot), m.trashRoot, m.trashInfo} {
		if err := m.rootFS.MkdirAll(rootName(directory), 0700); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) trashCount() (int, error) {
	directory, err := m.rootFS.Open(rootName(m.trashRoot))
	if err != nil {
		return 0, err
	}
	defer directory.Close()
	values, err := directory.ReadDir(MaxTrashEntries + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return 0, err
	}
	return len(values), nil
}

func (m *Manager) trashMetadataPath(id string) string {
	return joinVirtual(m.trashInfo, id+".json")
}

func (m *Manager) writeTrashMetadata(metadata trashMetadata) error {
	content, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	temp, tempVirtual, err := m.createTemp(m.trashInfo, ".kpanel-edit-")
	if err != nil {
		return err
	}
	success := false
	defer func() {
		_ = temp.Close()
		if !success {
			_ = m.rootFS.Remove(rootName(tempVirtual))
		}
	}()
	if _, err := temp.Write(content); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := renameNoReplaceRoot(m.rootFS, tempVirtual, m.trashMetadataPath(metadata.ID)); err != nil {
		return err
	}
	if err := syncRootDirectory(m.rootFS, rootName(m.trashInfo)); err != nil {
		return err
	}
	success = true
	return nil
}

func (m *Manager) readTrashMetadata(id string) (trashMetadata, error) {
	if err := validateTrashID(id); err != nil {
		return trashMetadata{}, err
	}
	content, err := m.rootFS.ReadFile(rootName(m.trashMetadataPath(id)))
	if err != nil {
		return trashMetadata{}, err
	}
	var metadata trashMetadata
	if err := json.Unmarshal(content, &metadata); err != nil {
		return trashMetadata{}, ErrTrashMetadata
	}
	if metadata.ID != id || metadata.Name == "" || metadata.DeletedAt.IsZero() ||
		validateName(metadata.Name) != nil {
		return trashMetadata{}, ErrTrashMetadata
	}
	normalized, err := normalizeVirtual(metadata.OriginalPath)
	if err != nil || normalized == "/" || normalized != metadata.OriginalPath ||
		path.Base(normalized) != metadata.Name {
		return trashMetadata{}, ErrTrashMetadata
	}
	return metadata, nil
}

func validateTrashID(id string) error {
	if id == "" || len(id) > 255 || strings.ContainsAny(id, `/\\`) || strings.ContainsRune(id, 0) ||
		id == "." || id == ".." || strings.HasPrefix(id, ".kpanel-") {
		return ErrInvalidPath
	}
	return nil
}

func (m *Manager) checkExpectedVersion(virtual, expected string) error {
	if expected == "" {
		return nil
	}
	entry, err := m.Stat(virtual)
	if err != nil {
		return err
	}
	if entry.ResourceVersion != expected {
		return ErrConflict
	}
	return nil
}

func (m *Manager) deleteTrash(id, expected string) error {
	if err := validateTrashID(id); err != nil {
		return err
	}
	dataVirtual := joinVirtual(m.trashRoot, id)
	info, err := m.rootFS.Lstat(rootName(dataVirtual))
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return ErrSymlink
	}
	if expected != "" && resourceVersion(dataVirtual, info) != expected {
		return ErrConflict
	}
	if err := m.rootFS.RemoveAll(rootName(dataVirtual)); err != nil {
		return err
	}
	_ = m.rootFS.Remove(rootName(m.trashMetadataPath(id)))
	if err := syncRootDirectory(m.rootFS, rootName(m.trashRoot)); err != nil {
		return err
	}
	return syncRootDirectory(m.rootFS, rootName(m.trashInfo))
}

func (m *Manager) restoreTrash(
	ctx context.Context, id, expected string, budget *copyBudget,
) (contract.FileEntry, error) {
	metadata, err := m.readTrashMetadata(id)
	if err != nil {
		return contract.FileEntry{}, ErrTrashMetadata
	}
	sourceVirtual := joinVirtual(m.trashRoot, id)
	sourceInfo, err := m.rootFS.Lstat(rootName(sourceVirtual))
	if err != nil {
		return contract.FileEntry{}, err
	}
	if sourceInfo.Mode()&os.ModeSymlink != 0 {
		return contract.FileEntry{}, ErrSymlink
	}
	if expected != "" && resourceVersion(sourceVirtual, sourceInfo) != expected {
		return contract.FileEntry{}, ErrConflict
	}
	targetVirtual := metadata.OriginalPath
	if err := m.mutationError(targetVirtual); err != nil {
		return contract.FileEntry{}, err
	}
	_, parentVirtual, err := m.resolveExisting(path.Dir(targetVirtual))
	if err != nil {
		return contract.FileEntry{}, err
	}
	if _, err := m.rootFS.Lstat(rootName(targetVirtual)); err == nil {
		return contract.FileEntry{}, ErrAlreadyExists
	} else if !errors.Is(err, os.ErrNotExist) {
		return contract.FileEntry{}, err
	}
	if err := renameNoReplaceRoot(m.rootFS, sourceVirtual, targetVirtual); err != nil {
		if !isCrossDeviceError(err) {
			return contract.FileEntry{}, err
		}
		tempVirtual := joinVirtual(parentVirtual, ".kpanel-copy-"+randomID())
		if err := m.copyTree(ctx, sourceVirtual, tempVirtual, budget); err != nil {
			_ = m.rootFS.RemoveAll(rootName(tempVirtual))
			return contract.FileEntry{}, err
		}
		currentInfo, statErr := m.rootFS.Lstat(rootName(sourceVirtual))
		if statErr != nil || resourceVersion(sourceVirtual, currentInfo) != resourceVersion(sourceVirtual, sourceInfo) {
			_ = m.rootFS.RemoveAll(rootName(tempVirtual))
			return contract.FileEntry{}, ErrConflict
		}
		if err := renameNoReplaceRoot(m.rootFS, tempVirtual, targetVirtual); err != nil {
			_ = m.rootFS.RemoveAll(rootName(tempVirtual))
			return contract.FileEntry{}, err
		}
		if err := m.rootFS.RemoveAll(rootName(sourceVirtual)); err != nil {
			return contract.FileEntry{}, err
		}
	}
	_ = m.rootFS.Remove(rootName(m.trashMetadataPath(id)))
	if err := syncRootDirectory(m.rootFS, rootName(parentVirtual)); err != nil {
		return contract.FileEntry{}, err
	}
	if err := syncRootDirectory(m.rootFS, rootName(m.trashRoot)); err != nil {
		return contract.FileEntry{}, err
	}
	return m.Stat(targetVirtual)
}

func (m *Manager) emptyTrash(ctx context.Context, result contract.FileActionResult) (contract.FileActionResult, error) {
	directory, err := m.rootFS.Open(rootName(m.trashRoot))
	if errors.Is(err, os.ErrNotExist) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	values, err := directory.ReadDir(MaxTrashEntries + 1)
	_ = directory.Close()
	if err != nil && !errors.Is(err, io.EOF) {
		return result, err
	}
	if len(values) > MaxTrashEntries {
		return result, ErrTrashFull
	}
	for _, value := range values {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		id := value.Name()
		if err := validateTrashID(id); err != nil {
			continue
		}
		if err := m.deleteTrash(id, ""); err != nil {
			result.Failed = append(result.Failed, contract.FileActionFailure{Path: id, Detail: err.Error()})
			continue
		}
		result.Succeeded = append(result.Succeeded, contract.FileActionItem{Path: id})
	}
	return result, nil
}

func (m *Manager) chmodOne(virtual, rawMode string) (contract.FileEntry, error) {
	if len(rawMode) != 3 && len(rawMode) != 4 {
		return contract.FileEntry{}, ErrAction
	}
	value, err := strconv.ParseUint(rawMode, 8, 32)
	if err != nil || value > 0777 {
		return contract.FileEntry{}, ErrAction
	}
	_, normalized, err := m.resolveExisting(virtual)
	if err != nil {
		return contract.FileEntry{}, err
	}
	if err := m.mutationError(normalized); err != nil {
		return contract.FileEntry{}, err
	}
	if err := m.rootFS.Chmod(rootName(normalized), os.FileMode(value)); err != nil {
		return contract.FileEntry{}, err
	}
	return m.Stat(normalized)
}

func (m *Manager) createTemp(directoryVirtual, prefix string) (*os.File, string, error) {
	for attempt := 0; attempt < 32; attempt++ {
		tempVirtual := joinVirtual(directoryVirtual, prefix+randomID())
		file, err := m.rootFS.OpenFile(
			rootName(tempVirtual),
			os.O_RDWR|os.O_CREATE|os.O_EXCL,
			0600,
		)
		if err == nil {
			return file, tempVirtual, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, "", err
		}
	}
	return nil, "", ErrConflict
}

func (m *Manager) entry(virtual string, info os.FileInfo) contract.FileEntry {
	kind := "file"
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		kind = "symlink"
	case info.IsDir():
		kind = "directory"
	case !info.Mode().IsRegular():
		kind = "special"
	}
	mimeType := ""
	if kind == "file" {
		mimeType = mime.TypeByExtension(strings.ToLower(filepath.Ext(info.Name())))
		if separator := strings.IndexByte(mimeType, ';'); separator >= 0 {
			mimeType = mimeType[:separator]
		}
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}
	}
	owner, group := fileOwner(info)
	editable, previewable := viewerSupport(info.Name(), mimeType, info.Size(), kind)
	return contract.FileEntry{
		Name: filepath.Base(virtual), Path: virtual, Kind: kind, MIME: mimeType,
		SizeBytes: info.Size(), Mode: info.Mode().String(), Owner: owner, Group: group,
		ModifiedAt: info.ModTime().UTC(), ResourceVersion: resourceVersion(virtual, info),
		Editable: editable, Previewable: previewable, ShareVersion: shareVersion(virtual, info),
	}
}

func (m *Manager) resolveExisting(virtual string) (string, string, error) {
	normalized, err := normalizeVirtual(virtual)
	if err != nil {
		return "", "", err
	}
	if m.isProtected(normalized) {
		return "", "", ErrProtected
	}
	relative := strings.TrimPrefix(normalized, "/")
	absolute := m.root
	if relative != "" {
		absolute = filepath.Join(m.root, filepath.FromSlash(relative))
	}
	if !pathInside(m.root, absolute) {
		return "", "", ErrInvalidPath
	}
	if relative == "" {
		if info, statErr := m.rootFS.Lstat("."); statErr != nil {
			return "", "", statErr
		} else if info.Mode()&os.ModeSymlink != 0 {
			return "", "", ErrSymlink
		}
		return absolute, normalized, nil
	}
	currentVirtual := "/"
	for _, component := range strings.Split(relative, "/") {
		if isInternalComponent(component) {
			return "", "", ErrProtected
		}
		currentVirtual = joinVirtual(currentVirtual, component)
		info, statErr := m.rootFS.Lstat(rootName(currentVirtual))
		if statErr != nil {
			return "", "", statErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", "", ErrSymlink
		}
	}
	return absolute, normalized, nil
}

func (m *Manager) isProtected(virtual string) bool {
	for _, protected := range m.protected {
		if virtual == protected || isWithin(virtual, protected) {
			return true
		}
	}
	return false
}

func (m *Manager) isMutationProtected(virtual string) bool {
	if m.isProtected(virtual) {
		return true
	}
	for _, protected := range m.protected {
		if isWithin(protected, virtual) {
			return true
		}
	}
	return false
}

func (m *Manager) mutationError(virtual string) error {
	if m.isMutationProtected(virtual) {
		return ErrProtected
	}
	for _, readOnly := range m.readOnly {
		if virtual == readOnly || isWithin(virtual, readOnly) || isWithin(readOnly, virtual) {
			return ErrReadOnly
		}
	}
	return nil
}

func normalizeVirtual(value string) (string, error) {
	if value == "" {
		value = "/"
	}
	if len(value) > maxPathBytes || !strings.HasPrefix(value, "/") ||
		strings.ContainsRune(value, 0) || strings.Contains(value, `\`) {
		return "", ErrInvalidPath
	}
	for _, component := range strings.Split(strings.TrimPrefix(value, "/"), "/") {
		if component == "." || component == ".." {
			return "", ErrInvalidPath
		}
	}
	normalized := path.Clean(value)
	if !strings.HasPrefix(normalized, "/") {
		return "", ErrInvalidPath
	}
	return normalized, nil
}

func validateName(value string) error {
	if value == "" || value == "." || value == ".." || len(value) > 255 ||
		strings.ContainsAny(value, `/\`) || strings.ContainsRune(value, 0) ||
		isInternalComponent(value) {
		return ErrInvalidPath
	}
	return nil
}

func isInternalComponent(value string) bool {
	return strings.HasPrefix(value, ".kpanel-edit-") ||
		strings.HasPrefix(value, ".kpanel-upload-") ||
		strings.HasPrefix(value, ".kpanel-copy-") ||
		strings.HasPrefix(value, ".kpanel-archive-") ||
		strings.HasPrefix(value, ".kpanel-extract-")
}

func joinVirtual(parent, name string) string {
	if parent == "/" {
		return "/" + name
	}
	return parent + "/" + name
}

func rootName(virtual string) string {
	relative := strings.TrimPrefix(virtual, "/")
	if relative == "" {
		return "."
	}
	return filepath.FromSlash(relative)
}

func isWithin(candidate, parent string) bool {
	return strings.HasPrefix(candidate, strings.TrimSuffix(parent, "/")+"/")
}

func pathInside(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	return err == nil && relative != ".." &&
		!strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func resourceVersion(virtual string, info os.FileInfo) string {
	owner, group := fileOwner(info)
	sum := sha256.Sum256([]byte(fmt.Sprintf(
		"%s\x00%d\x00%d\x00%d\x00%s\x00%s",
		virtual, info.Size(), info.ModTime().UnixNano(), info.Mode(), owner, group,
	)))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func shareVersion(virtual string, info os.FileInfo) string {
	sum := sha256.Sum256([]byte(resourceVersion(virtual, info) + "\x00" + shareFileIdentity(info)))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func viewerSupport(name, mimeType string, size int64, kind string) (bool, bool) {
	if kind != "file" {
		return false, false
	}
	extension := strings.ToLower(filepath.Ext(name))
	textExtensions := map[string]bool{
		"": true, ".txt": true, ".log": true, ".md": true, ".json": true,
		".yaml": true, ".yml": true, ".toml": true, ".ini": true, ".conf": true,
		".sh": true, ".bash": true, ".zsh": true, ".go": true, ".js": true,
		".mjs": true, ".cjs": true, ".ts": true, ".tsx": true, ".jsx": true,
		".vue": true, ".html": true, ".htm": true, ".css": true, ".scss": true,
		".xml": true, ".svg": true, ".env": true, ".sql": true, ".py": true, ".rb": true,
		".php": true, ".java": true, ".c": true, ".h": true, ".cpp": true,
	}
	if textExtensions[extension] || strings.HasPrefix(mimeType, "text/") {
		return size <= MaxTextBytes, size <= MaxTextBytes
	}
	previewable := strings.HasPrefix(mimeType, "image/") ||
		strings.HasPrefix(mimeType, "audio/") ||
		strings.HasPrefix(mimeType, "video/") ||
		mimeType == "application/pdf"
	if extension == ".svg" || extension == ".html" || extension == ".htm" {
		previewable = size <= MaxTextBytes
	}
	return false, previewable
}

func actionItem(path string, entry contract.FileEntry, destination string) contract.FileActionItem {
	return contract.FileActionItem{
		Path: path, Destination: destination, ResourceVersion: entry.ResourceVersion,
	}
}

func acquireNow(ctx context.Context, gate chan struct{}) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case gate <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		return ErrBusy
	}
}

func release(gate chan struct{}) {
	<-gate
}

type gatedFile struct {
	*os.File
	gate chan struct{}
	once sync.Once
}

func (file *gatedFile) Close() error {
	err := file.File.Close()
	file.once.Do(func() { release(file.gate) })
	return err
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (reader *contextReader) Read(buffer []byte) (int, error) {
	if err := reader.ctx.Err(); err != nil {
		return 0, err
	}
	return reader.reader.Read(buffer)
}

type copyBudget struct {
	entries    int
	bytes      int64
	maxEntries int
	maxBytes   int64
}

func (m *Manager) copyTree(
	ctx context.Context,
	sourceVirtual, targetVirtual string,
	budget *copyBudget,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	info, err := m.rootFS.Lstat(rootName(sourceVirtual))
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return ErrSymlink
	}
	budget.entries++
	budget.bytes += info.Size()
	if budget.entries > budget.maxEntries || budget.bytes > budget.maxBytes {
		return ErrTooLarge
	}
	if info.IsDir() {
		if err := m.rootFS.Mkdir(rootName(targetVirtual), info.Mode().Perm()); err != nil {
			return err
		}
		targetDirectory, err := m.rootFS.Open(rootName(targetVirtual))
		if err != nil {
			return err
		}
		if err := preserveFileOwnership(targetDirectory, info); err != nil {
			targetDirectory.Close()
			return err
		}
		if err := targetDirectory.Close(); err != nil {
			return err
		}
		directory, err := m.rootFS.Open(rootName(sourceVirtual))
		if err != nil {
			return err
		}
		defer directory.Close()
		openedInfo, err := directory.Stat()
		if err != nil || !os.SameFile(info, openedInfo) || !openedInfo.IsDir() {
			return ErrConflict
		}
		for {
			values, readErr := directory.ReadDir(256)
			for _, value := range values {
				if err := m.copyTree(
					ctx,
					joinVirtual(sourceVirtual, value.Name()),
					joinVirtual(targetVirtual, value.Name()),
					budget,
				); err != nil {
					return err
				}
			}
			if errors.Is(readErr, io.EOF) {
				break
			}
			if readErr != nil {
				return readErr
			}
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		return m.rootFS.Chtimes(rootName(targetVirtual), info.ModTime(), info.ModTime())
	}
	if !info.Mode().IsRegular() {
		return ErrNotRegular
	}
	input, err := m.rootFS.Open(rootName(sourceVirtual))
	if err != nil {
		return err
	}
	defer input.Close()
	openedInfo, err := input.Stat()
	if err != nil || !os.SameFile(info, openedInfo) || !openedInfo.Mode().IsRegular() {
		return ErrConflict
	}
	output, err := m.rootFS.OpenFile(
		rootName(targetVirtual),
		os.O_CREATE|os.O_EXCL|os.O_WRONLY,
		info.Mode().Perm(),
	)
	if err != nil {
		return err
	}
	if err := preserveFileOwnership(output, info); err != nil {
		output.Close()
		_ = m.rootFS.Remove(rootName(targetVirtual))
		return err
	}
	success := false
	defer func() {
		output.Close()
		if !success {
			_ = m.rootFS.Remove(rootName(targetVirtual))
		}
	}()
	if _, err := io.CopyBuffer(output, &contextReader{ctx: ctx, reader: input}, make([]byte, 64<<10)); err != nil {
		return err
	}
	if err := output.Sync(); err != nil {
		return err
	}
	if err := output.Close(); err != nil {
		return err
	}
	if err := m.rootFS.Chtimes(rootName(targetVirtual), info.ModTime(), info.ModTime()); err != nil {
		return err
	}
	success = true
	return nil
}

func randomID() string {
	var value [8]byte
	if _, err := rand.Read(value[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(value[:])
}
