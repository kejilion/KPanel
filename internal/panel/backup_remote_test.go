package panel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/kejilion/kejilion-panel/internal/backup"
	"github.com/kejilion/kejilion-panel/internal/backupremote"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type memoryBackupRemote struct {
	mu      sync.Mutex
	objects map[string][]byte
	fail    bool
	deleted []string
}

func (m *memoryBackupRemote) Upload(ctx context.Context, key string, f *os.File, size int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fail {
		return errors.New("fixture credentials must not leak")
	}
	b, e := io.ReadAll(f)
	m.objects[key] = b
	return e
}
func (m *memoryBackupRemote) Download(ctx context.Context, key, target string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return backup.CopyFile(target, bytes.NewReader(m.objects[key]), backup.MaxEncryptedBytes)
}
func (m *memoryBackupRemote) Delete(ctx context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deleted = append(m.deleted, key)
	delete(m.objects, key)
	return nil
}
func (m *memoryBackupRemote) List(context.Context) ([]backupremote.Object, error) {
	return []backupremote.Object{}, nil
}
func (m *memoryBackupRemote) Test(context.Context) error { return nil }
func (m *memoryBackupRemote) Close()                     {}

type gatedBackupRemote struct {
	*memoryBackupRemote
	entered chan struct{}
	release chan struct{}
}

func (m *gatedBackupRemote) Upload(ctx context.Context, key string, f *os.File, size int64) error {
	close(m.entered)
	select {
	case <-m.release:
		return m.memoryBackupRemote.Upload(ctx, key, f, size)
	case <-ctx.Done():
		return ctx.Err()
	}
}

func waitBackupRecord(t *testing.T, s *Server, id string) backup.Record {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		r, e := s.backups.Get(id)
		if e != nil {
			t.Fatal(e)
		}
		if r.Status != "running" && r.Status != "queued" {
			return r
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("backup did not finish")
	return backup.Record{}
}

func TestBackupRemoteAPIAndExistingRestoreFlow(t *testing.T) {
	s, token := newTestServer(t)
	session, csrf := bootstrapCookies(t, s, token)
	s.config.TOTPKeyPath = filepath.Join(s.config.DataDir, "totp.key")
	remote := &memoryBackupRemote{objects: map[string][]byte{}, fail: true}
	s.backupRemoteClient = func(backupremote.Storage) (backupRemoteTransport, error) { return remote, nil }
	request := func(method, path string, input any, authorized bool) *httptest.ResponseRecorder {
		b, _ := json.Marshal(input)
		r := httptest.NewRequest(method, "/api/v1/backups/"+path, bytes.NewReader(b))
		r.Host = "panel.test"
		r.Header.Set("Content-Type", "application/json")
		if authorized {
			r.AddCookie(session)
			r.AddCookie(csrf)
			r.Header.Set("Origin", "http://panel.test")
			r.Header.Set("X-CSRF-Token", csrf.Value)
		}
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		return w
	}
	input := map[string]any{"revision": s.backupRemote.Snapshot().Revision, "storage": backupremote.Storage{Name: "NAS", Kind: "webdav", Endpoint: "https://nas.example/dav", Prefix: "kpanel", Username: "admin", Secret: "remote-secret"}}
	if w := request("PUT", "storage", input, false); w.Code < 400 {
		t.Fatal("unauthenticated config")
	}
	if w := request("PUT", "storage", input, true); w.Code != 200 || bytes.Contains(w.Body.Bytes(), []byte("remote-secret")) {
		t.Fatal(w.Code, w.Body.String())
	}
	storage := s.backupRemote.Snapshot().Storages[0]
	w := request("POST", "export", backupRequest{Modules: []string{"panel"}, Password: "backup-password", StorageID: storage.ID}, true)
	var record backup.Record
	if w.Code != 202 || json.Unmarshal(w.Body.Bytes(), &record) != nil {
		t.Fatal(w.Code, w.Body.String())
	}
	record = waitBackupRecord(t, s, record.ID)
	if record.Status != "failed" || record.ErrorCode != "remote_upload_failed" || !record.LocalReady || record.Remote.Status != "failed" {
		t.Fatal(record)
	}
	if w = request("GET", record.ID+"/download", nil, true); w.Code != 200 {
		t.Fatal("local fallback missing", w.Code)
	}
	remote.mu.Lock()
	remote.fail = false
	remote.mu.Unlock()
	if w = request("POST", record.ID+"/upload", map[string]string{"storageId": storage.ID}, true); w.Code != 202 {
		t.Fatal(w.Code, w.Body.String())
	}
	record = waitBackupRecord(t, s, record.ID)
	if record.Status != "completed" || record.Remote.Status != "completed" || record.Automatic {
		t.Fatal(record)
	}
	w = request("POST", "remote-import", map[string]string{"storageId": storage.ID, "key": record.Remote.Key, "password": "backup-password"}, true)
	var imported backup.Record
	if w.Code != 202 || json.Unmarshal(w.Body.Bytes(), &imported) != nil {
		t.Fatal(w.Code, w.Body.String())
	}
	imported = waitBackupRecord(t, s, imported.ID)
	if imported.Status != "ready" {
		t.Fatal(imported)
	}
	if PanelRestorePending(s.config) {
		t.Fatal("remote fetch restored before confirmation")
	}
	if w = request("POST", imported.ID+"/preview", map[string]string{}, true); w.Code != 200 {
		t.Fatal("existing preview rejected remote import", w.Code, w.Body.String())
	}
	if _, e := os.Stat(filepath.Join(s.backups.Root, imported.ID, "upload.kpb")); !os.IsNotExist(e) {
		t.Fatal("import ciphertext not cleaned")
	}
	// Wrong password must never reach ready or write restore intent.
	w = request("POST", "remote-import", map[string]string{"storageId": storage.ID, "key": record.Remote.Key, "password": "wrong-password"}, true)
	if w.Code != 202 || json.Unmarshal(w.Body.Bytes(), &imported) != nil {
		t.Fatal(w.Code)
	}
	if got := waitBackupRecord(t, s, imported.ID); got.Status != "failed" || PanelRestorePending(s.config) {
		t.Fatal(got)
	}
}

func TestAutomaticBackupUploadRetryPreservesHealthAndIdentity(t *testing.T) {
	s, token := newTestServer(t)
	session, csrf := bootstrapCookies(t, s, token)
	s.config.TOTPKeyPath = filepath.Join(s.config.DataDir, "totp.key")
	remote := &memoryBackupRemote{objects: map[string][]byte{}}
	s.backupRemoteClient = func(backupremote.Storage) (backupRemoteTransport, error) { return remote, nil }
	if err := s.backupRemote.PutStorage(s.backupRemote.Snapshot().Revision, backupremote.Storage{Name: "NAS", Kind: "webdav", Endpoint: "https://nas.example/dav", Username: "backup", Secret: "secret"}); err != nil {
		t.Fatal(err)
	}
	storage := s.backupRemote.Snapshot().Storages[0]
	plan, revision := s.backupRemote.PlanWithRevision()
	plan.Enabled, plan.Password, plan.Modules, plan.StorageID = true, "backup-password", []string{"panel"}, storage.ID
	if err := s.backupRemote.PutSchedule(revision, plan, time.Now()); err != nil {
		t.Fatal(err)
	}
	start := func() backup.Record {
		t.Helper()
		plan, revision := s.backupRemote.PlanWithRevision()
		record, err := s.startScheduledBackup(plan, revision)
		if err != nil {
			t.Fatal(err)
		}
		if !s.backupRemote.Plan().LastRun.Equal(record.CreatedAt) {
			t.Fatal("immediate run is not bound to its receipt's exact start time")
		}
		return waitBackupRecord(t, s, record.ID)
	}
	first := start()
	if first.Status != "completed" || s.backupHealth().State != "healthy" {
		t.Fatal(first, s.backupHealth())
	}
	remote.mu.Lock()
	remote.fail = true
	remote.mu.Unlock()
	second := start()
	if second.Status != "failed" || s.backupHealth().State != "failed" {
		t.Fatal(second, s.backupHealth())
	}
	failedID := s.backupNotificationStatus().ID
	retry := func(target string, fail bool, automatic bool, expected string) {
		t.Helper()
		remote.mu.Lock()
		remote.fail = fail
		remote.mu.Unlock()
		gate := &gatedBackupRemote{memoryBackupRemote: remote, entered: make(chan struct{}), release: make(chan struct{})}
		s.backupRemoteClient = func(backupremote.Storage) (backupRemoteTransport, error) { return gate, nil }
		body, _ := json.Marshal(map[string]string{"storageId": target})
		r := httptest.NewRequest("POST", "/api/v1/backups/"+second.ID+"/upload", bytes.NewReader(body))
		r.Host = "panel.test"
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", "http://panel.test")
		r.Header.Set("X-CSRF-Token", csrf.Value)
		r.AddCookie(session)
		r.AddCookie(csrf)
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != 202 {
			close(gate.release)
			t.Fatalf("retry status=%d body=%s", w.Code, w.Body.String())
		}
		select {
		case <-gate.entered:
		case <-time.After(5 * time.Second):
			close(gate.release)
			t.Fatal("retry did not reach the fixture")
		}
		if automatic && s.backupHealth().State != "running" {
			close(gate.release)
			t.Fatal("in-flight retry reported a final outcome", s.backupHealth())
		}
		close(gate.release)
		got := waitBackupRecord(t, s, second.ID)
		if got.Automatic != automatic || got.Remote == nil || got.Remote.StorageID != target {
			t.Fatal("retry changed task identity", got)
		}
		if automatic && (s.backupHealth().State != expected || s.backupHealth().LastRecordID != second.ID) {
			t.Fatal("retry fell back to an older successful task", s.backupHealth())
		}
	}
	retry(storage.ID, true, true, "failed")
	if s.backupNotificationStatus().ID != failedID || s.backupNotificationStatus().Completed {
		t.Fatal("unchanged failure generated a new event or false recovery")
	}
	retry(storage.ID, false, true, "healthy")
	if !s.backupNotificationStatus().Completed || s.backupNotificationStatus().ID == failedID || !s.backupHealth().LastSuccessAt.Equal(second.CreatedAt) {
		t.Fatal("successful retry did not recover with the original backup data age")
	}
	if err := s.backups.Update(second.ID, func(r *backup.Record) { r.ErrorCode = "retention_failed" }); err != nil {
		t.Fatal(err)
	}
	retry(storage.ID, false, true, "warning")
	if s.backupNotificationStatus().Completed {
		t.Fatal("upload-only retry claimed to recover retention")
	}
	if err := s.backupRemote.PutStorage(s.backupRemote.Snapshot().Revision, backupremote.Storage{Name: "Other", Kind: "webdav", Endpoint: "https://other.example/dav", Username: "backup", Secret: "secret"}); err != nil {
		t.Fatal(err)
	}
	var otherID string
	for _, target := range s.backupRemote.Snapshot().Storages {
		if target.ID != storage.ID {
			otherID = target.ID
		}
	}
	retry(otherID, false, false, "")
}

func TestBackupAutomaticRetentionNeverDeletesManualOrForeignObjects(t *testing.T) {
	s, token := newTestServer(t)
	bootstrapCookies(t, s, token)
	s.config.TOTPKeyPath = filepath.Join(s.config.DataDir, "totp.key")
	remote := &memoryBackupRemote{objects: map[string][]byte{"foreign.kpb": []byte("owned elsewhere")}}
	s.backupRemoteClient = func(backupremote.Storage) (backupRemoteTransport, error) { return remote, nil }
	err := s.backupRemote.PutStorage(s.backupRemote.Snapshot().Revision, backupremote.Storage{Name: "NAS", Kind: "webdav", Endpoint: "https://nas.example/dav", Username: "backup", Secret: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	id := s.backupRemote.Snapshot().Storages[0].ID
	input := backupRequest{Modules: []string{"panel"}, Password: "backup-password", StorageID: id}
	var first string
	for i := 0; i < 3; i++ {
		r, e := s.startBackupExport(input, 1)
		if e != nil {
			t.Fatal(e)
		}
		r = waitBackupRecord(t, s, r.ID)
		if r.Status != "completed" {
			t.Fatal(r)
		}
		if i == 0 {
			first = r.ID
		}
	}
	if _, err := s.backups.Get(first); !errors.Is(err, backup.ErrNotFound) {
		t.Fatal("old automatic record retained")
	}
	if len(remote.deleted) != 2 {
		t.Fatal(remote.deleted)
	}
	if _, exists := remote.objects["foreign.kpb"]; !exists {
		t.Fatal("unowned remote file removed")
	}
	if s.backupRemote.Plan().LastError != "" {
		t.Fatal("successful schedule marked failed")
	}
	// Local automatic backups survive the manual seven-day TTL.
	input.StorageID = ""
	r, e := s.startBackupExport(input, 1)
	if e != nil {
		t.Fatal(e)
	}
	r = waitBackupRecord(t, s, r.ID)
	if e = s.backups.Sweep(time.Now().Add(40 * 24 * time.Hour)); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(filepath.Join(s.backups.Root, r.ID, "backup.kpb")); e != nil {
		t.Fatal(e)
	}
}
