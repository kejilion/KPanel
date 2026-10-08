package filemanager

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

func receiveDigest(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}
func testReceiveInput(size int64) contract.FileReceiveInput {
	return contract.FileReceiveInput{Directory: "/", Name: "result.bin", Kind: "file", SizeBytes: size, SourceKey: strings.Repeat("a", 64)}
}
func newReceiveManager(t *testing.T, root string) *Manager {
	t.Helper()
	m, err := New(Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Close() })
	return m
}

func TestReceiveRestartDropsUnacknowledgedTailAndCommitsExactlyOnce(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	m := newReceiveManager(t, root)
	input := testReceiveInput(11)
	stamp := time.Date(2025, 4, 5, 6, 7, 8, 0, time.UTC)
	input.ModifiedAt = &stamp
	session, err := m.BeginReceive(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	first := []byte("hello ")
	session, err = m.WriteReceiveChunk(ctx, session.ID, input.SourceKey, 0, bytes.NewReader(first), receiveDigest(first))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Stat("/result.bin"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("partial target visible: %v", err)
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	temp := filepath.Join(root, ".kpanel-upload-"+session.ID)
	file, err := os.OpenFile(temp, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = file.WriteString("unacknowledged")
	_ = file.Close()
	m = newReceiveManager(t, root)
	resumed, err := m.ReceiveStatus(ctx, session.ID, input.SourceKey)
	if err != nil || resumed.Offset != int64(len(first)) || resumed.PrefixSHA256 != receiveDigest(first) {
		t.Fatalf("resume=%#v error=%v", resumed, err)
	}
	info, err := os.Stat(temp)
	if err != nil || info.Size() != int64(len(first)) {
		t.Fatalf("unacknowledged tail retained: %v %v", info, err)
	}
	second := []byte("world")
	session, err = m.WriteReceiveChunk(ctx, session.ID, input.SourceKey, resumed.Offset, bytes.NewReader(second), receiveDigest(second))
	if err != nil {
		t.Fatal(err)
	}
	full := []byte("hello world")
	complete, err := m.CommitReceive(ctx, session.ID, input.SourceKey, 11, receiveDigest(full))
	if err != nil || complete.State != "complete" || complete.Entry == nil {
		t.Fatalf("commit=%#v error=%v", complete, err)
	}
	if !complete.Entry.ModifiedAt.Equal(stamp) {
		t.Fatalf("mtime lost: %v", complete.Entry.ModifiedAt)
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	m = newReceiveManager(t, root)
	again, err := m.CommitReceive(ctx, session.ID, input.SourceKey, 11, receiveDigest(full))
	if err != nil || again.Entry == nil || again.Entry.ResourceVersion != complete.Entry.ResourceVersion {
		t.Fatalf("commit replay=%#v error=%v", again, err)
	}
	actual, err := os.ReadFile(filepath.Join(root, "result.bin"))
	if err != nil || !bytes.Equal(actual, full) {
		t.Fatalf("destination=%q error=%v", actual, err)
	}
}

func TestReceiveDuplicateChunkChecksDurableBytesAndRejectsWrongIdentity(t *testing.T) {
	ctx := context.Background()
	m := newReceiveManager(t, t.TempDir())
	input := testReceiveInput(6)
	session, err := m.BeginReceive(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	data := []byte("first!")
	if _, err := m.WriteReceiveChunk(ctx, session.ID, input.SourceKey, 0, bytes.NewReader(data), receiveDigest(data)); err != nil {
		t.Fatal(err)
	}
	duplicate, err := m.WriteReceiveChunk(ctx, session.ID, input.SourceKey, 0, bytes.NewReader(data), receiveDigest(data))
	if err != nil || duplicate.Offset != 6 {
		t.Fatalf("duplicate=%#v error=%v", duplicate, err)
	}
	other := []byte("second")
	if _, err := m.WriteReceiveChunk(ctx, session.ID, input.SourceKey, 0, bytes.NewReader(other), receiveDigest(other)); !errors.Is(err, ErrReceiveChecksum) {
		t.Fatalf("different duplicate accepted: %v", err)
	}
	if _, err := m.ReceiveStatus(ctx, session.ID, strings.Repeat("b", 64)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("wrong identity accepted: %v", err)
	}
	if _, err := m.CommitReceive(ctx, session.ID, input.SourceKey, 6, receiveDigest(other)); !errors.Is(err, ErrReceiveChecksum) {
		t.Fatalf("wrong complete hash accepted: %v", err)
	}
}

func TestReceiveRecoveryRejectsChangedPrefixAndCorruptJournal(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	m := newReceiveManager(t, root)
	input := testReceiveInput(8)
	session, err := m.BeginReceive(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	data := []byte("prefix")
	if _, err := m.WriteReceiveChunk(ctx, session.ID, input.SourceKey, 0, bytes.NewReader(data), receiveDigest(data)); err != nil {
		t.Fatal(err)
	}
	_ = m.Close()
	if err := os.WriteFile(filepath.Join(root, ".kpanel-upload-"+session.ID), []byte("change"), 0600); err != nil {
		t.Fatal(err)
	}
	m = newReceiveManager(t, root)
	if _, err := m.ReceiveStatus(ctx, session.ID, input.SourceKey); !errors.Is(err, ErrReceiveChecksum) {
		t.Fatalf("changed prefix accepted: %v", err)
	}
	_ = m.Close()
	if err := os.WriteFile(filepath.Join(root, ".kpanel-trash", "receive-sessions.json"), []byte(`{"version":1,"records":[]}{}`), 0600); err != nil {
		t.Fatal(err)
	}
	m = newReceiveManager(t, root)
	if _, err := m.BeginReceive(ctx, input); !errors.Is(err, ErrReceiveUnavailable) {
		t.Fatalf("corrupt journal silently replaced: %v", err)
	}
}

func TestReceiveCommitPreservesExternalConflictAndAbortRemovesOnlyItsTemp(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	m := newReceiveManager(t, root)
	input := testReceiveInput(4)
	session, err := m.BeginReceive(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	data := []byte("mine")
	if _, err := m.WriteReceiveChunk(ctx, session.ID, input.SourceKey, 0, bytes.NewReader(data), receiveDigest(data)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, input.Name), []byte("external"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := m.CommitReceive(ctx, session.ID, input.SourceKey, 4, receiveDigest(data)); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("concurrent target clobbered: %v", err)
	}
	if err := m.AbortReceive(ctx, session.ID, input.SourceKey); err != nil {
		t.Fatal(err)
	}
	content, _ := os.ReadFile(filepath.Join(root, input.Name))
	if string(content) != "external" {
		t.Fatalf("abort touched committed/external target: %q", content)
	}
	if _, err := os.Stat(filepath.Join(root, ".kpanel-upload-"+session.ID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("abort retained temp: %v", err)
	}
}

type receiveTruncatedReader struct{ content *strings.Reader }

func (r *receiveTruncatedReader) Read(buffer []byte) (int, error) {
	n, err := r.content.Read(buffer)
	if err == io.EOF {
		return n, io.ErrUnexpectedEOF
	}
	return n, err
}

func TestReceiveStreamRequiresRealTransportEndBeforePublishing(t *testing.T) {
	m := newReceiveManager(t, t.TempDir())
	input := testReceiveInput(7)
	_, err := m.ReceiveStream(context.Background(), input, &receiveTruncatedReader{strings.NewReader("payload")}, contract.MaxFileTransferBytes)
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("truncation=%v", err)
	}
	if _, err := m.Stat("/result.bin"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("truncated transfer published: %v", err)
	}
}

func TestReceiveLargeFilesKeepShareLimitAndBoundReservations(t *testing.T) {
	m := newReceiveManager(t, t.TempDir())
	ctx := context.Background()
	input := testReceiveInput(600 << 20)
	if contract.MaxFileShareBytes != 512<<20 {
		t.Fatal("public share proof budget changed")
	}
	if _, err := m.BeginReceive(ctx, input); err != nil {
		t.Fatalf("large managed transfer rejected: %v", err)
	}
	for i := 0; i < 3; i++ {
		input.Name = "unknown-" + string(rune('a'+i))
		input.SizeBytes = -1
		if _, err := m.BeginReceive(ctx, input); err != nil {
			t.Fatal(err)
		}
	}
	input.Name = "overflow"
	if _, err := m.BeginReceive(ctx, input); !errors.Is(err, ErrBusy) {
		t.Fatalf("unbounded reserved disk: %v", err)
	}
}

func TestReceiveCompletedFilesDoNotExhaustActiveSlots(t *testing.T) {
	m := newReceiveManager(t, t.TempDir())
	for i := 0; i < maxReceiveSessions+4; i++ {
		input := testReceiveInput(1)
		input.Name = fmt.Sprintf("file-%d", i)
		session, err := m.BeginReceive(context.Background(), input)
		if err != nil {
			t.Fatalf("completed files consumed active slots at %d: %v", i, err)
		}
		data := []byte("x")
		if _, err := m.WriteReceiveChunk(context.Background(), session.ID, input.SourceKey, 0, bytes.NewReader(data), receiveDigest(data)); err != nil {
			t.Fatal(err)
		}
		if _, err := m.CommitReceive(context.Background(), session.ID, input.SourceKey, 1, receiveDigest(data)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestReceiveDirectoryCommitRecoveryPreservesRootMetadata(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	m := newReceiveManager(t, root)
	var archive bytes.Buffer
	writer := tar.NewWriter(&archive)
	if err := writer.WriteHeader(&tar.Header{Name: "child.txt", Mode: 0640, Size: 7, Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	_, _ = writer.Write([]byte("payload"))
	_ = writer.Close()
	data := archive.Bytes()
	input := testReceiveInput(int64(len(data)))
	input.Kind = "directory"
	input.Name = "result"
	stamp := time.Date(2025, 4, 5, 6, 7, 8, 0, time.UTC)
	input.ModifiedAt = &stamp
	input.Mode = "0750"
	session, err := m.BeginReceive(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.WriteReceiveChunk(ctx, session.ID, input.SourceKey, 0, bytes.NewReader(data), receiveDigest(data)); err != nil {
		t.Fatal(err)
	}
	complete, err := m.CommitReceive(ctx, session.ID, input.SourceKey, int64(len(data)), receiveDigest(data))
	if err != nil {
		t.Fatal(err)
	}
	if complete.Entry == nil || !complete.Entry.ModifiedAt.Equal(stamp) {
		t.Fatalf("root timestamp not preserved: %#v", complete)
	}
	// Model a crash after publish but before the final complete journal write.
	record := m.receives.records[session.ID]
	record.Session.State = "committing"
	record.Session.Entry = nil
	if err := m.persistReceivesLocked(); err != nil {
		t.Fatal(err)
	}
	_ = m.Close()
	m = newReceiveManager(t, root)
	recovered, err := m.ReceiveStatus(ctx, session.ID, input.SourceKey)
	if err != nil || recovered.State != "complete" || recovered.Entry == nil {
		t.Fatalf("ambiguous directory commit: %#v %v", recovered, err)
	}
	got, err := os.ReadFile(filepath.Join(root, "result", "child.txt"))
	if err != nil || string(got) != "payload" {
		t.Fatalf("content=%q err=%v", got, err)
	}
}
