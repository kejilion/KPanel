package bittorrent

import (
	"archive/tar"
	"context"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/storage"
)

var payloadName = regexp.MustCompile(`^payload-[0-9]+$`)

// Cache is disposable, single-task staging, never a second task journal. The
// receive layer exclusively owns publication into the user's target directory.
type Cache struct {
	root *os.Root
	gate chan struct{}
}

func OpenCache(directory string) (*Cache, error) {
	if !filepath.IsAbs(directory) {
		return nil, ErrStorage
	}
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, ErrStorage
	}
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, ErrStorage
	}
	if os.Chmod(directory, 0700) != nil {
		return nil, ErrStorage
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, ErrStorage
	}
	keep := false
	defer func() {
		if !keep {
			root.Close()
		}
	}()
	dir, err := root.Open(".")
	if err != nil {
		return nil, ErrStorage
	}
	entries, err := dir.ReadDir(3)
	dir.Close()
	if err != nil && err != io.EOF || len(entries) > 1 {
		return nil, ErrStorage
	}
	// A previous process can leave at most one payload. Reject unexpected data
	// instead of recursively deleting paths we do not own.
	for _, entry := range entries {
		if !payloadName.MatchString(entry.Name()) || !entry.Type().IsRegular() {
			return nil, ErrStorage
		}
		if root.Remove(entry.Name()) != nil {
			return nil, ErrStorage
		}
	}
	keep = true
	return &Cache{root: root, gate: make(chan struct{}, 1)}, nil
}

func (c *Cache) Close() error { return c.root.Close() }

type stagedFile struct {
	file        *os.File
	metadata    Metadata
	pieceLength int64
	mu          sync.Mutex
	complete    []bool
	writeNanos  atomic.Int64
	closeOnce   sync.Once
	cleanup     func()
	failed      chan error
}

func (c *Cache) create(ctx context.Context) (*stagedFile, error) {
	select {
	case c.gate <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	name := "payload-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	file, err := c.root.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		<-c.gate
		return nil, ErrStorage
	}
	s := &stagedFile{file: file, failed: make(chan error, 1)}
	s.cleanup = func() { file.Close(); _ = c.root.Remove(name); <-c.gate }
	return s, nil
}

func (s *stagedFile) Close() error { s.closeOnce.Do(s.cleanup); return nil }
func (s *stagedFile) OpenTorrent(_ context.Context, info *metainfo.Info, hash metainfo.Hash) (storage.TorrentImpl, error) {
	m, err := validateInfo(info)
	if err != nil {
		return storage.TorrentImpl{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.metadata.Size != 0 {
		return storage.TorrentImpl{}, ErrMetadata
	}
	if err = checkSpace(s.file.Name(), m.Size); err != nil {
		return storage.TorrentImpl{}, err
	}
	if s.file.Truncate(m.Size) != nil {
		return storage.TorrentImpl{}, ErrStorage
	}
	m.Hash = hash
	s.metadata = m
	s.pieceLength = info.PieceLength
	s.complete = make([]bool, info.NumPieces())
	return storage.TorrentImpl{Piece: func(p metainfo.Piece) storage.PieceImpl {
		return &stagedPiece{stage: s, index: p.Index(), offset: p.Offset(), length: p.Length()}
	}, Close: func() error { return nil }}, nil
}

type stagedPiece struct {
	stage          *stagedFile
	index          int
	offset, length int64
}

func (p *stagedPiece) ReadAt(data []byte, offset int64) (int, error) {
	if offset < 0 || offset > p.length || int64(len(data)) > p.length-offset {
		return 0, io.ErrUnexpectedEOF
	}
	return p.stage.file.ReadAt(data, p.offset+offset)
}
func (p *stagedPiece) WriteAt(data []byte, offset int64) (int, error) {
	if offset < 0 || offset > p.length || int64(len(data)) > p.length-offset {
		return 0, ErrStorage
	}
	start := time.Now()
	n, err := p.stage.file.WriteAt(data, p.offset+offset)
	p.stage.writeNanos.Add(int64(time.Since(start)))
	if err != nil {
		select {
		case p.stage.failed <- ErrStorage:
		default:
		}
		return n, ErrStorage
	}
	return n, nil
}
func (p *stagedPiece) MarkComplete() error {
	p.stage.mu.Lock()
	p.stage.complete[p.index] = true
	p.stage.mu.Unlock()
	return nil
}
func (p *stagedPiece) MarkNotComplete() error {
	p.stage.mu.Lock()
	p.stage.complete[p.index] = false
	p.stage.mu.Unlock()
	return nil
}
func (p *stagedPiece) Completion() storage.Completion {
	p.stage.mu.Lock()
	defer p.stage.mu.Unlock()
	return storage.Completion{Ok: true, Complete: p.stage.complete[p.index]}
}

type Result struct {
	Metadata Metadata
	stage    *stagedFile
}

func (r *Result) Close() error    { return r.stage.Close() }
func (r *Result) Directory() bool { return r.Metadata.Multi }

type directoryStream struct {
	*io.PipeReader
	done <-chan struct{}
}

func (s *directoryStream) Close() error {
	err := s.PipeReader.Close()
	<-s.done
	return err
}

// Each reopen deterministically regenerates the exact same TAR stream. Agent
// checkpoints still validate the complete replayed prefix before proceeding.
func (r *Result) Open(ctx context.Context) io.ReadCloser {
	if !r.Directory() {
		return io.NopCloser(io.NewSectionReader(r.stage.file, 0, r.Metadata.Size))
	}
	reader, writer := io.Pipe()
	done := make(chan struct{})
	go func() {
		defer close(done)
		archive := tar.NewWriter(writer)
		var err error
		for _, file := range r.Metadata.Files {
			if err = ctx.Err(); err != nil {
				break
			}
			err = archive.WriteHeader(&tar.Header{Name: file.Path, Mode: 0600, Size: file.Length, Typeflag: tar.TypeReg, Format: tar.FormatPAX})
			if err != nil {
				break
			}
			_, err = io.CopyBuffer(archive, io.NewSectionReader(r.stage.file, file.Offset, file.Length), make([]byte, 64<<10))
			if err != nil {
				break
			}
		}
		if err == nil {
			err = archive.Close()
		}
		_ = writer.CloseWithError(err)
	}()
	// The receiver closes its source before deleting staging. Join the TAR
	// producer here so cancellation never leaves it reading a removed payload.
	return &directoryStream{PipeReader: reader, done: done}
}
