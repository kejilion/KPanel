package filemanager

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"hash"
	"io"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

const (
	maxReceiveSessions            = 32
	maxReceiveRecords             = 256
	maxReceiveStateBytes          = 512 << 10
	maxReceiveReservedBytes int64 = 40 << 30
	receiveLifetime               = 24 * time.Hour
)

var (
	ErrReceiveUnavailable = errors.New("传输恢复记录不可用")
	ErrReceiveChecksum    = errors.New("传输校验失败，来源内容可能已变化")
)

// A checkpoint acknowledges only bytes fsynced before the atomic journal write.
// The bounded private journal contains identities/hashes, never source URLs.
type receiveRecord struct {
	Session         contract.FileReceiveSession `json:"session"`
	Input           contract.FileReceiveInput   `json:"input"`
	OriginalVersion string                      `json:"originalVersion,omitempty"`
	PublishVersion  string                      `json:"publishVersion,omitempty"`
	hasher          hash.Hash
	identity        os.FileInfo
}

type receiveSessions struct {
	mu          cancellableMutex
	initialized bool
	closed      bool
	store       *os.Root
	records     map[string]*receiveRecord
}

func validReceiveKey(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32 && hex.EncodeToString(decoded) == value
}

func validateReceiveInput(input contract.FileReceiveInput) error {
	directory, err := normalizeVirtual(input.Directory)
	if err != nil || directory != input.Directory || validateName(input.Name) != nil ||
		!validReceiveKey(input.SourceKey) || (input.Kind != "file" && input.Kind != "directory") ||
		input.SizeBytes < -1 || input.Kind == "directory" && input.Overwrite {
		return ErrInvalidPath
	}
	if input.SizeBytes > receiveLimit(input) {
		return ErrTooLarge
	}
	if input.Mode != "" {
		mode, err := strconv.ParseUint(input.Mode, 8, 32)
		if err != nil || len(input.Mode) > 4 || mode > 0777 {
			return ErrInvalidPath
		}
	}
	if input.ModifiedAt != nil && (input.ModifiedAt.Year() < 1970 || input.ModifiedAt.Year() > 9999) {
		return ErrInvalidPath
	}
	return nil
}

func receiveLimit(input contract.FileReceiveInput) int64 {
	if input.Kind == "directory" {
		// TAR headers/padding are bounded separately from the extracted byte budget.
		return contract.MaxFileTransferBytes + (32 << 20)
	}
	return contract.MaxFileTransferBytes
}

func receiveReservation(input contract.FileReceiveInput) int64 {
	size := input.SizeBytes
	if size < 0 {
		size = receiveLimit(input)
	}
	if input.Kind == "directory" {
		size += contract.MaxFileTransferBytes
	}
	return size
}

func receiveTemp(record *receiveRecord) string {
	return joinVirtual(record.Input.Directory, ".kpanel-upload-"+record.Session.ID)
}

func receiveExtract(record *receiveRecord) string {
	return joinVirtual(record.Input.Directory, ".kpanel-extract-"+record.Session.ID)
}

func (m *Manager) initializeReceivesLocked() error {
	s := &m.receives
	if s.closed {
		return ErrReceiveUnavailable
	}
	if s.initialized {
		if s.store == nil {
			return ErrReceiveUnavailable
		}
		return nil
	}
	s.initialized = true
	s.records = make(map[string]*receiveRecord)
	private := path.Dir(m.trashRoot)
	current := "/"
	for _, component := range strings.Split(strings.TrimPrefix(private, "/"), "/") {
		current = joinVirtual(current, component)
		if err := m.rootFS.Mkdir(rootName(current), 0700); err != nil && !errors.Is(err, os.ErrExist) {
			return ErrReceiveUnavailable
		}
		info, err := m.rootFS.Lstat(rootName(current))
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return ErrReceiveUnavailable
		}
	}
	store, err := m.rootFS.OpenRoot(rootName(private))
	if err != nil {
		return ErrReceiveUnavailable
	}
	ok := false
	defer func() {
		if !ok {
			_ = store.Close()
		}
	}()
	info, err := store.Lstat("receive-sessions.json")
	if errors.Is(err, os.ErrNotExist) {
		s.store = store
		ok = true
		return nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxReceiveStateBytes {
		return ErrReceiveUnavailable
	}
	file, err := store.Open("receive-sessions.json")
	if err != nil {
		return ErrReceiveUnavailable
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) || opened.Size() > maxReceiveStateBytes {
		return ErrReceiveUnavailable
	}
	decoder := json.NewDecoder(io.LimitReader(file, maxReceiveStateBytes+1))
	decoder.DisallowUnknownFields()
	var state struct {
		Version int              `json:"version"`
		Records []*receiveRecord `json:"records"`
	}
	var extra any
	if decoder.Decode(&state) != nil || decoder.Decode(&extra) != io.EOF || state.Version != 1 || len(state.Records) > maxReceiveRecords {
		return ErrReceiveUnavailable
	}
	var reserved int64
	active := 0
	for _, record := range state.Records {
		if record == nil || validateReceiveInput(record.Input) != nil || !validReceiveKey(record.Session.ID) ||
			record.Session.Offset < 0 || record.Session.Offset > receiveLimit(record.Input) ||
			!validReceiveKey(record.Session.PrefixSHA256) || record.Session.ExpiresAt.IsZero() ||
			record.Session.SizeBytes != record.Input.SizeBytes || record.Session.ChunkBytes != contract.FileTransferChunkBytes ||
			(record.Input.SizeBytes >= 0 && record.Session.Offset > record.Input.SizeBytes) ||
			(record.Session.State != "receiving" && record.Session.State != "committing" && record.Session.State != "complete" && record.Session.State != "aborted") {
			return ErrReceiveUnavailable
		}
		if _, exists := s.records[record.Session.ID]; exists {
			return ErrReceiveUnavailable
		}
		if record.Session.State == "complete" && (record.Session.Entry == nil || record.Session.Entry.Path != joinVirtual(record.Input.Directory, record.Input.Name) || record.Session.Entry.Kind != record.Input.Kind) {
			return ErrReceiveUnavailable
		}
		if record.Session.State == "receiving" || record.Session.State == "committing" {
			reserved += receiveReservation(record.Input)
			active++
		}
		s.records[record.Session.ID] = record
	}
	if reserved > maxReceiveReservedBytes || active > maxReceiveSessions {
		return ErrReceiveUnavailable
	}
	s.store = store
	ok = true
	return nil
}

func (m *Manager) persistReceivesLocked() error {
	s := &m.receives
	records := make([]*receiveRecord, 0, len(s.records))
	for _, record := range s.records {
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool { return records[i].Session.ID < records[j].Session.ID })
	payload, err := json.Marshal(struct {
		Version int              `json:"version"`
		Records []*receiveRecord `json:"records"`
	}{1, records})
	if err != nil || len(payload) > maxReceiveStateBytes {
		return ErrReceiveUnavailable
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return err
	}
	name := ".kpanel-edit-receives-" + hex.EncodeToString(random[:])
	file, err := s.store.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close(); _ = s.store.Remove(name) }()
	if _, err := file.Write(payload); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := s.store.Rename(name, "receive-sessions.json"); err != nil {
		return err
	}
	return syncRootDirectory(s.store, ".")
}

func (m *Manager) cleanupReceivesLocked() error {
	changed := false
	for id, record := range m.receives.records {
		if m.now().Before(record.Session.ExpiresAt) {
			continue
		}
		if err := m.removeReceiveTemp(record); err != nil && record.Session.State != "complete" && record.Session.State != "aborted" {
			return err
		}
		delete(m.receives.records, id)
		changed = true
	}
	// Completed acknowledgements must not consume active transfer slots forever.
	// Keep a bounded replay window and evict only the oldest terminal record.
	if len(m.receives.records) >= maxReceiveRecords {
		var oldest *receiveRecord
		for _, record := range m.receives.records {
			if record.Session.State == "complete" && (oldest == nil || record.Session.ExpiresAt.Before(oldest.Session.ExpiresAt)) {
				oldest = record
			}
		}
		if oldest != nil {
			delete(m.receives.records, oldest.Session.ID)
			changed = true
		}
	}
	if changed {
		return m.persistReceivesLocked()
	}
	return nil
}

func (m *Manager) removeReceiveTemp(record *receiveRecord) error {
	if _, _, err := m.resolveExisting(record.Input.Directory); err != nil {
		return err
	}
	if record.Input.Kind == "directory" {
		if err := m.removeReceiveExtract(record); err != nil {
			return err
		}
	}
	info, err := m.rootFS.Lstat(rootName(receiveTemp(record)))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return ErrConflict
	}
	return m.rootFS.Remove(rootName(receiveTemp(record)))
}

func (m *Manager) removeReceiveExtract(record *receiveRecord) error {
	info, err := m.rootFS.Lstat(rootName(receiveExtract(record)))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return ErrConflict
	}
	return m.rootFS.RemoveAll(rootName(receiveExtract(record)))
}

func (m *Manager) BeginReceive(ctx context.Context, input contract.FileReceiveInput) (contract.FileReceiveSession, error) {
	if err := validateReceiveInput(input); err != nil {
		return contract.FileReceiveSession{}, err
	}
	if err := m.receives.mu.LockContext(ctx); err != nil {
		return contract.FileReceiveSession{}, err
	}
	defer m.receives.mu.Unlock()
	if err := m.initializeReceivesLocked(); err != nil {
		return contract.FileReceiveSession{}, err
	}
	if err := m.cleanupReceivesLocked(); err != nil {
		return contract.FileReceiveSession{}, err
	}
	var reserved int64
	active := 0
	for _, record := range m.receives.records {
		if record.Session.State == "receiving" || record.Session.State == "committing" {
			reserved += receiveReservation(record.Input)
			active++
		}
	}
	if active >= maxReceiveSessions || len(m.receives.records) >= maxReceiveRecords || reserved+receiveReservation(input) > maxReceiveReservedBytes {
		return contract.FileReceiveSession{}, ErrBusy
	}
	_, directory, err := m.resolveExisting(input.Directory)
	if err != nil {
		return contract.FileReceiveSession{}, err
	}
	info, err := m.rootFS.Lstat(rootName(directory))
	if err != nil {
		return contract.FileReceiveSession{}, err
	}
	if !info.IsDir() {
		return contract.FileReceiveSession{}, ErrNotDirectory
	}
	target := joinVirtual(directory, input.Name)
	if err := m.mutationError(target); err != nil {
		return contract.FileReceiveSession{}, err
	}
	var source *os.File
	var original string
	if existing, err := m.rootFS.Lstat(rootName(target)); err == nil {
		if !input.Overwrite {
			return contract.FileReceiveSession{}, ErrAlreadyExists
		}
		if !existing.Mode().IsRegular() {
			return contract.FileReceiveSession{}, ErrNotRegular
		}
		source, err = m.rootFS.Open(rootName(target))
		if err != nil {
			return contract.FileReceiveSession{}, err
		}
		defer source.Close()
		opened, err := source.Stat()
		if err != nil || !os.SameFile(existing, opened) {
			return contract.FileReceiveSession{}, ErrConflict
		}
		original = resourceVersion(target, existing)
	} else if !errors.Is(err, os.ErrNotExist) {
		return contract.FileReceiveSession{}, err
	}
	var random [32]byte
	if _, err := rand.Read(random[:]); err != nil {
		return contract.FileReceiveSession{}, err
	}
	hasher := sha256.New()
	record := &receiveRecord{
		Input: input, OriginalVersion: original, hasher: hasher,
		Session: contract.FileReceiveSession{ID: hex.EncodeToString(random[:]), State: "receiving", SizeBytes: input.SizeBytes,
			PrefixSHA256: hex.EncodeToString(hasher.Sum(nil)), ChunkBytes: contract.FileTransferChunkBytes, ExpiresAt: m.now().UTC().Add(receiveLifetime)},
	}
	temp, err := createFileWithSourceAccess(m.rootFS, rootName(receiveTemp(record)), os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600, source)
	if err != nil {
		return contract.FileReceiveSession{}, err
	}
	defer temp.Close()
	keep := false
	defer func() {
		if !keep {
			_ = m.rootFS.Remove(rootName(receiveTemp(record)))
		}
	}()
	if source != nil {
		info, err := source.Stat()
		if err != nil {
			return contract.FileReceiveSession{}, err
		}
		if err := preserveFileOwnership(temp, info); err != nil {
			return contract.FileReceiveSession{}, err
		}
		if err := preserveFileExtendedAttributes(temp, source); err != nil {
			return contract.FileReceiveSession{}, err
		}
	}
	if err := temp.Sync(); err != nil {
		return contract.FileReceiveSession{}, err
	}
	record.identity, err = temp.Stat()
	if err != nil {
		return contract.FileReceiveSession{}, err
	}
	m.receives.records[record.Session.ID] = record
	if err := m.persistReceivesLocked(); err != nil {
		delete(m.receives.records, record.Session.ID)
		return contract.FileReceiveSession{}, err
	}
	keep = true
	return record.Session, nil
}

func (m *Manager) receiveLocked(id, sourceKey string) (*receiveRecord, error) {
	if !validReceiveKey(id) || !validReceiveKey(sourceKey) {
		return nil, ErrInvalidPath
	}
	if err := m.initializeReceivesLocked(); err != nil {
		return nil, err
	}
	record, exists := m.receives.records[id]
	if !exists || record.Input.SourceKey != sourceKey || record.Session.State == "aborted" {
		return nil, os.ErrNotExist
	}
	if !m.now().Before(record.Session.ExpiresAt) {
		return nil, os.ErrNotExist
	}
	if _, _, err := m.resolveExisting(record.Input.Directory); err != nil {
		return nil, err
	}
	if err := m.mutationError(joinVirtual(record.Input.Directory, record.Input.Name)); err != nil {
		return nil, err
	}
	return record, nil
}

// On recovery, verify the acknowledged prefix and discard any unacknowledged
// tail left by a crash. An inode substituted during this process is rejected.
func (m *Manager) openReceiveFile(ctx context.Context, record *receiveRecord) (*os.File, error) {
	info, err := m.rootFS.Lstat(rootName(receiveTemp(record)))
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() < record.Session.Offset {
		return nil, ErrConflict
	}
	file, err := m.rootFS.OpenFile(rootName(receiveTemp(record)), os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	ok := false
	defer func() {
		if !ok {
			_ = file.Close()
		}
	}()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) || record.identity != nil && !os.SameFile(record.identity, opened) {
		return nil, ErrConflict
	}
	if record.hasher == nil || record.identity == nil || resourceVersion(receiveTemp(record), record.identity) != resourceVersion(receiveTemp(record), opened) {
		hasher := sha256.New()
		if _, err := io.CopyBuffer(hasher, &contextReader{ctx, io.LimitReader(file, record.Session.Offset)}, make([]byte, 64<<10)); err != nil {
			return nil, err
		}
		if hex.EncodeToString(hasher.Sum(nil)) != record.Session.PrefixSHA256 {
			return nil, ErrReceiveChecksum
		}
		record.hasher = hasher
	}
	if opened.Size() != record.Session.Offset {
		if err := file.Truncate(record.Session.Offset); err != nil {
			return nil, err
		}
		if err := file.Sync(); err != nil {
			return nil, err
		}
	}
	if _, err := file.Seek(record.Session.Offset, io.SeekStart); err != nil {
		return nil, err
	}
	record.identity, err = file.Stat()
	if err != nil {
		return nil, err
	}
	ok = true
	return file, nil
}

func (m *Manager) ReceiveStatus(ctx context.Context, id, sourceKey string) (contract.FileReceiveSession, error) {
	if err := m.receives.mu.LockContext(ctx); err != nil {
		return contract.FileReceiveSession{}, err
	}
	defer m.receives.mu.Unlock()
	record, err := m.receiveLocked(id, sourceKey)
	if err != nil {
		return contract.FileReceiveSession{}, err
	}
	if record.Session.State == "committing" {
		if err := m.reconcileReceiveCommit(record); err != nil {
			return contract.FileReceiveSession{}, err
		}
	}
	if record.Session.State == "receiving" {
		file, err := m.openReceiveFile(ctx, record)
		if err != nil {
			return contract.FileReceiveSession{}, err
		}
		_ = file.Close()
	}
	return record.Session, nil
}

func (m *Manager) WriteReceiveChunk(ctx context.Context, id, sourceKey string, offset int64, content io.Reader, digest string) (contract.FileReceiveSession, error) {
	if offset < 0 || !validReceiveKey(digest) {
		return contract.FileReceiveSession{}, ErrInvalidPath
	}
	if err := acquireNow(ctx, m.uploadGate); err != nil {
		return contract.FileReceiveSession{}, err
	}
	defer release(m.uploadGate)
	// Buffer exactly one bounded chunk so failed checksums cannot alter a prefix.
	data, err := io.ReadAll(&contextReader{ctx, io.LimitReader(content, contract.FileTransferChunkBytes+1)})
	if err != nil {
		return contract.FileReceiveSession{}, err
	}
	if len(data) == 0 || len(data) > contract.FileTransferChunkBytes {
		return contract.FileReceiveSession{}, ErrTooLarge
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != digest {
		return contract.FileReceiveSession{}, ErrReceiveChecksum
	}
	if err := m.receives.mu.LockContext(ctx); err != nil {
		return contract.FileReceiveSession{}, err
	}
	defer m.receives.mu.Unlock()
	record, err := m.receiveLocked(id, sourceKey)
	if err != nil {
		return contract.FileReceiveSession{}, err
	}
	if record.Session.State != "receiving" || offset > record.Session.Offset {
		return contract.FileReceiveSession{}, ErrConflict
	}
	end := offset + int64(len(data))
	if end > receiveLimit(record.Input) || record.Input.SizeBytes >= 0 && end > record.Input.SizeBytes {
		return contract.FileReceiveSession{}, ErrTooLarge
	}
	file, err := m.openReceiveFile(ctx, record)
	if err != nil {
		return contract.FileReceiveSession{}, err
	}
	defer file.Close()
	if offset < record.Session.Offset {
		// Retried acknowledgements are idempotent only for identical durable data.
		if end > record.Session.Offset {
			return contract.FileReceiveSession{}, ErrConflict
		}
		prior := make([]byte, len(data))
		if _, err := file.ReadAt(prior, offset); err != nil || !bytes.Equal(prior, data) {
			return contract.FileReceiveSession{}, ErrReceiveChecksum
		}
		return record.Session, nil
	}
	previous := record.Session
	if _, err := file.Write(data); err != nil {
		record.hasher = nil
		return contract.FileReceiveSession{}, err
	}
	if err := file.Sync(); err != nil {
		record.hasher = nil
		return contract.FileReceiveSession{}, err
	}
	_, _ = record.hasher.Write(data)
	record.Session.Offset = end
	record.Session.PrefixSHA256 = hex.EncodeToString(record.hasher.Sum(nil))
	record.Session.ExpiresAt = m.now().UTC().Add(receiveLifetime)
	record.identity, err = file.Stat()
	if err == nil {
		err = m.persistReceivesLocked()
	}
	if err != nil {
		record.Session = previous
		record.hasher = nil
		return contract.FileReceiveSession{}, err
	}
	return record.Session, nil
}

func (m *Manager) reconcileReceiveCommit(record *receiveRecord) error {
	target := joinVirtual(record.Input.Directory, record.Input.Name)
	if info, err := m.rootFS.Lstat(rootName(target)); err == nil {
		if record.Input.Overwrite && record.OriginalVersion != "" && resourceVersion(target, info) == record.OriginalVersion {
			if temp, err := m.rootFS.Lstat(rootName(receiveTemp(record))); err == nil && temp.Mode().IsRegular() {
				record.Session.State = "receiving"
				return m.persistReceivesLocked()
			}
		}
		if record.PublishVersion == "" || resourceVersion(target, info) != record.PublishVersion {
			return ErrConflict
		}
		entry, err := m.Stat(target)
		if err != nil {
			return err
		}
		record.Session.State, record.Session.Entry = "complete", &entry
		return m.persistReceivesLocked()
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if record.Input.Overwrite && record.OriginalVersion != "" {
		return ErrConflict
	}
	record.Session.State = "receiving"
	return m.persistReceivesLocked()
}

func (m *Manager) CommitReceive(ctx context.Context, id, sourceKey string, size int64, digest string) (contract.FileReceiveSession, error) {
	if !validReceiveKey(digest) || size < 0 {
		return contract.FileReceiveSession{}, ErrInvalidPath
	}
	if err := m.receives.mu.LockContext(ctx); err != nil {
		return contract.FileReceiveSession{}, err
	}
	defer m.receives.mu.Unlock()
	record, err := m.receiveLocked(id, sourceKey)
	if err != nil {
		return contract.FileReceiveSession{}, err
	}
	if size != record.Session.Offset || digest != record.Session.PrefixSHA256 || record.Input.SizeBytes >= 0 && size != record.Input.SizeBytes {
		return contract.FileReceiveSession{}, ErrReceiveChecksum
	}
	if record.Session.State == "committing" {
		if err := m.reconcileReceiveCommit(record); err != nil {
			return contract.FileReceiveSession{}, err
		}
	}
	if record.Session.State == "complete" {
		return record.Session, nil
	}
	file, err := m.openReceiveFile(ctx, record)
	if err != nil {
		return contract.FileReceiveSession{}, err
	}
	defer file.Close()
	target := joinVirtual(record.Input.Directory, record.Input.Name)
	if record.Input.Kind == "directory" {
		if err := m.removeReceiveExtract(record); err != nil {
			return contract.FileReceiveSession{}, err
		}
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return contract.FileReceiveSession{}, err
		}
		record.Session.State = "committing"
		if err := m.persistReceivesLocked(); err != nil {
			record.Session.State = "receiving"
			return contract.FileReceiveSession{}, err
		}
		entry, err := m.importDirectory(ctx, record.Input, file, receiveExtract(record), func(info os.FileInfo) error {
			record.PublishVersion = resourceVersion(target, info)
			return m.persistReceivesLocked()
		})
		if err != nil {
			return contract.FileReceiveSession{}, err
		}
		record.Session.Entry, record.Session.State = &entry, "complete"
		_ = file.Close()
		if err := m.removeReceiveTemp(record); err != nil {
			return contract.FileReceiveSession{}, err
		}
	} else {
		if err := m.writeMu.LockContext(ctx); err != nil {
			return contract.FileReceiveSession{}, err
		}
		defer m.writeMu.Unlock()
		replace := false
		mode := os.FileMode(0644)
		if record.Input.Mode != "" {
			value, _ := strconv.ParseUint(record.Input.Mode, 8, 32)
			mode = os.FileMode(value)
		}
		if info, err := m.rootFS.Lstat(rootName(target)); err == nil {
			if !record.Input.Overwrite {
				return contract.FileReceiveSession{}, ErrAlreadyExists
			}
			if record.OriginalVersion == "" || !info.Mode().IsRegular() || resourceVersion(target, info) != record.OriginalVersion {
				return contract.FileReceiveSession{}, ErrConflict
			}
			mode, replace = info.Mode().Perm(), true
		} else if !errors.Is(err, os.ErrNotExist) {
			return contract.FileReceiveSession{}, err
		}
		if err := file.Chmod(mode); err != nil {
			return contract.FileReceiveSession{}, err
		}
		if record.Input.ModifiedAt != nil {
			if err := m.rootFS.Chtimes(rootName(receiveTemp(record)), *record.Input.ModifiedAt, *record.Input.ModifiedAt); err != nil {
				return contract.FileReceiveSession{}, err
			}
		}
		if err := file.Sync(); err != nil {
			return contract.FileReceiveSession{}, err
		}
		info, err := file.Stat()
		if err != nil {
			return contract.FileReceiveSession{}, err
		}
		record.PublishVersion = resourceVersion(target, info)
		record.Session.State = "committing"
		if err := m.persistReceivesLocked(); err != nil {
			record.Session.State = "receiving"
			return contract.FileReceiveSession{}, err
		}
		if err := file.Close(); err != nil {
			return contract.FileReceiveSession{}, err
		}
		if err := ctx.Err(); err != nil {
			return contract.FileReceiveSession{}, err
		}
		if replace {
			err = m.rootFS.Rename(rootName(receiveTemp(record)), rootName(target))
		} else {
			err = renameNoReplaceRoot(m.rootFS, receiveTemp(record), target)
		}
		if err != nil {
			return contract.FileReceiveSession{}, err
		}
		if err := syncRootDirectory(m.rootFS, rootName(record.Input.Directory)); err != nil {
			return contract.FileReceiveSession{}, err
		}
		entry, err := m.Stat(target)
		if err != nil {
			return contract.FileReceiveSession{}, err
		}
		record.Session.Entry, record.Session.State = &entry, "complete"
	}
	if err := m.persistReceivesLocked(); err != nil {
		return contract.FileReceiveSession{}, err
	}
	return record.Session, nil
}

func (m *Manager) AbortReceive(ctx context.Context, id, sourceKey string) error {
	if err := m.receives.mu.LockContext(ctx); err != nil {
		return err
	}
	defer m.receives.mu.Unlock()
	record, err := m.receiveLocked(id, sourceKey)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if record.Session.State == "committing" {
		if err := m.reconcileReceiveCommit(record); err != nil {
			return err
		}
	}
	if record.Session.State == "complete" {
		return nil
	}
	if err := m.removeReceiveTemp(record); err != nil {
		return err
	}
	delete(m.receives.records, id)
	return m.persistReceivesLocked()
}

func (m *Manager) closeReceiveSessions() {
	m.receives.mu.Lock()
	defer m.receives.mu.Unlock()
	m.receives.closed = true
	if m.receives.store != nil {
		_ = m.receives.store.Close()
	}
}

// ReceiveStream is the compatibility adapter for one-shot uploads/imports.
// New clients retain the same receiving session across reconnects instead.
func (m *Manager) ReceiveStream(ctx context.Context, input contract.FileReceiveInput, content io.Reader, limit int64) (contract.FileEntry, error) {
	if input.SourceKey == "" {
		var identity [32]byte
		if _, err := rand.Read(identity[:]); err != nil {
			return contract.FileEntry{}, err
		}
		input.SourceKey = hex.EncodeToString(identity[:])
	}
	session, err := m.BeginReceive(ctx, input)
	if err != nil {
		return contract.FileEntry{}, err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = m.AbortReceive(cleanup, session.ID, input.SourceKey)
	}()
	reader := &receiveEOFReader{Reader: &contextReader{ctx, content}}
	buffer := make([]byte, contract.FileTransferChunkBytes)
	for {
		count, readErr := io.ReadFull(reader, buffer)
		if readErr != nil && (readErr != io.EOF && readErr != io.ErrUnexpectedEOF || !reader.eof) {
			return contract.FileEntry{}, readErr
		}
		if session.Offset+int64(count) > limit {
			return contract.FileEntry{}, ErrTooLarge
		}
		if count > 0 {
			digest := sha256.Sum256(buffer[:count])
			session, err = m.WriteReceiveChunk(ctx, session.ID, input.SourceKey, session.Offset, bytes.NewReader(buffer[:count]), hex.EncodeToString(digest[:]))
			if err != nil {
				return contract.FileEntry{}, err
			}
		}
		if readErr != nil {
			break
		}
	}
	session, err = m.CommitReceive(ctx, session.ID, input.SourceKey, session.Offset, session.PrefixSHA256)
	if err != nil {
		return contract.FileEntry{}, err
	}
	if session.Entry == nil {
		return contract.FileEntry{}, ErrConflict
	}
	return *session.Entry, nil
}

type receiveEOFReader struct {
	io.Reader
	eof bool
}

func (r *receiveEOFReader) Read(buffer []byte) (int, error) {
	n, err := r.Reader.Read(buffer)
	if err == io.EOF {
		r.eof = true
	}
	return n, err
}
