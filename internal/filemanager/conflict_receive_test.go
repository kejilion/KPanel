package filemanager

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

func TestConflictReceiveDirectoryWaitsAndMerges(t *testing.T) {
	m, root := newTestManager(t)
	mustMkdirAll(t, filepath.Join(root, "target", "folder"))
	mustWrite(t, filepath.Join(root, "target", "folder", "a.txt"), "old")
	mustWrite(t, filepath.Join(root, "target", "folder", "only.txt"), "retain")
	var data bytes.Buffer
	writer := tar.NewWriter(&data)
	for _, name := range []string{"a.txt", "b.txt"} {
		body := []byte("incoming")
		if err := writer.WriteHeader(&tar.Header{Name: name, Mode: 0644, Size: int64(len(body))}); err != nil {
			t.Fatal(err)
		}
		writer.Write(body)
	}
	writer.Close()
	payload := data.Bytes()
	digest := sha256.Sum256(payload)
	hash := hex.EncodeToString(digest[:])
	keyDigest := sha256.Sum256([]byte("directory-conflict"))
	key := hex.EncodeToString(keyDigest[:])
	ctx := context.Background()
	session, err := m.BeginReceive(ctx, contract.FileReceiveInput{Directory: "/target", Name: "folder", Kind: "directory", SizeBytes: int64(len(payload)), SourceKey: key, ConflictPolicies: &contract.FileConflictPolicies{}})
	if err != nil {
		t.Fatal(err)
	}
	session, err = m.WriteReceiveChunk(ctx, session.ID, key, 0, bytes.NewReader(payload), hash)
	if err != nil {
		t.Fatal(err)
	}
	session, err = m.CommitReceive(ctx, session.ID, key, int64(len(payload)), hash)
	if err != nil || session.PublishJobID == "" {
		t.Fatalf("commit %+v %v", session, err)
	}
	waiting := waitConflictJob(t, m, session.PublishJobID, "waiting_input")
	if _, err = m.DecideReceive(ctx, session.ID, key, contract.FileConflictDecision{Revision: waiting.Revision, Policy: "merge"}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		session, err = m.ReceiveStatus(ctx, session.ID, key)
		if err != nil {
			t.Fatal(err)
		}
		if session.Conflict != nil && session.Conflict.Source.Kind == "file" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if session.Conflict == nil || session.Conflict.Source.Kind != "file" {
		t.Fatalf("file decision lost %+v", session)
	}
	if _, err = m.DecideReceive(ctx, session.ID, key, contract.FileConflictDecision{Revision: session.Revision, Policy: "overwrite", ApplyToRest: true}); err != nil {
		t.Fatal(err)
	}
	for time.Now().Before(deadline) {
		session, err = m.ReceiveStatus(ctx, session.ID, key)
		if err != nil {
			t.Fatal(err)
		}
		if session.State == "complete" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if session.State != "complete" || session.Entry == nil {
		t.Fatalf("not completed %+v", session)
	}
	for name, want := range map[string]string{"a.txt": "incoming", "b.txt": "incoming", "only.txt": "retain"} {
		body, err := os.ReadFile(filepath.Join(root, "target", "folder", name))
		if err != nil || string(body) != want {
			t.Fatalf("%s=%q %v", name, body, err)
		}
	}
}
