package panel

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/kejilion/kejilion-panel/internal/backup"
	"github.com/kejilion/kejilion-panel/internal/store"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBackupPanelIdentityAndStagingValidation(t *testing.T) {
	s, token := newTestServer(t)
	bootstrapCookies(t, s, token)
	// Match configured key paths as a real LoadConfig instance does.
	s.config.TOTPKeyPath = filepath.Join(s.config.DataDir, "totp.key")
	if err := s.EnableAI(); err != nil {
		t.Fatal(err)
	}
	data, err := s.exportPanelBackup(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	value, err := decodePanelBackup(data)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.validatePanelRestore(value, filepath.Join(s.backups.Root, "validate-fixture")); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte(`"messages"`)) || bytes.Contains(data, []byte(`"encrypted_key"`)) {
		t.Fatal("AI history or source ciphertext exported")
	}
	before := value.Files["cluster-secrets-v2/node-identity.v2key"]
	if len(before) == 0 {
		t.Fatal("cluster identity omitted")
	}
	if _, err := os.Stat(filepath.Join(s.backups.Root, "validate-fixture")); !os.IsNotExist(err) {
		t.Fatal("validation plaintext retained")
	}
}

func TestBackupHTTPAuthAndExport(t *testing.T) {
	s, token := newTestServer(t)
	session, csrf := bootstrapCookies(t, s, token)
	s.config.TOTPKeyPath = filepath.Join(s.config.DataDir, "totp.key")
	body := []byte(`{"password":"long-test-password","modules":["panel"]}`)
	request := func(auth, origin, check bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/api/v1/backups/export", bytes.NewReader(body))
		r.Host = "panel.test"
		r.Header.Set("Content-Type", "application/json")
		if auth {
			r.AddCookie(session)
			r.AddCookie(csrf)
		}
		if origin {
			r.Header.Set("Origin", "http://panel.test")
		}
		if check {
			r.Header.Set("X-CSRF-Token", csrf.Value)
		}
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		return w
	}
	for _, flags := range [][3]bool{{false, true, true}, {true, false, true}, {true, true, false}} {
		if w := request(flags[0], flags[1], flags[2]); w.Code < 400 {
			t.Fatal("accepted missing authorization", flags)
		}
	}
	w := request(true, true, true)
	if w.Code != http.StatusAccepted {
		t.Fatal(w.Code, w.Body.String())
	}
	var r backup.Record
	if err := json.Unmarshal(w.Body.Bytes(), &r); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		r, _ = s.backups.Get(r.ID)
		if r.Status == "completed" {
			break
		}
		if r.Status == "failed" {
			t.Fatal(r)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if r.Status != "completed" || r.Size == 0 {
		t.Fatal(r)
	}
	f, err := backup.OpenRegular(filepath.Join(s.backups.Root, r.ID, "backup.kpb"), backup.MaxEncryptedBytes)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := backup.Read(f, "long-test-password", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	var upload bytes.Buffer
	multipartWriter := multipart.NewWriter(&upload)
	if err := multipartWriter.WriteField("password", "long-test-password"); err != nil {
		t.Fatal(err)
	}
	part, err := multipartWriter.CreateFormFile("file", "fixture.kpb")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(part, f); err != nil {
		t.Fatal(err)
	}
	if err := multipartWriter.Close(); err != nil {
		t.Fatal(err)
	}
	post := func(path, contentType string, body io.Reader) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", path, body)
		req.Host = "panel.test"
		req.AddCookie(session)
		req.AddCookie(csrf)
		req.Header.Set("Origin", "http://panel.test")
		req.Header.Set("X-CSRF-Token", csrf.Value)
		req.Header.Set("Content-Type", contentType)
		response := httptest.NewRecorder()
		s.ServeHTTP(response, req)
		return response
	}
	w = post("/api/v1/backups/import", multipartWriter.FormDataContentType(), &upload)
	if w.Code != 202 || json.Unmarshal(w.Body.Bytes(), &r) != nil {
		t.Fatal(w.Code, w.Body.String())
	}
	wait := func(status string) {
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			r, _ = s.backups.Get(r.ID)
			if r.Status == status {
				return
			}
			if r.Status == "failed" {
				t.Fatal(r)
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatal("job did not reach", status, r)
	}
	wait("ready")
	if PanelRestorePending(s.config) {
		t.Fatal("upload wrote restore intent before confirmation")
	}
	w = post("/api/v1/backups/"+r.ID+"/preview", "application/json", bytes.NewBufferString(`{}`))
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &r) != nil {
		t.Fatal(w.Code, w.Body.String())
	}
	confirmation, err := json.Marshal(backupRequest{Modules: []string{"panel"}, Revision: r.TargetRevision})
	if err != nil {
		t.Fatal(err)
	}
	w = post("/api/v1/backups/"+r.ID+"/restore", "application/json", bytes.NewReader(confirmation))
	if w.Code != 202 || json.Unmarshal(w.Body.Bytes(), &r) != nil {
		t.Fatal(w.Code, w.Body.String())
	}
	wait("restarting")
	select {
	case <-s.BackupRestart():
	case <-time.After(5 * time.Second):
		t.Fatal("confirmed restore did not request controlled restart")
	}
	if !PanelRestorePending(s.config) {
		t.Fatal("confirmed restore has no durable intent")
	}
}

func TestBackupRevisionIgnoresHeartbeatsButIncludesKeys(t *testing.T) {
	base := panelBackupData{Version: 1, Identity: json.RawMessage(`{"users":[{"username":"admin","totpLastUsedStep":1}]}`), Files: map[string][]byte{"cluster-state-v2.json": []byte(`{"controllers":[{"id":"x","lastSeenAt":"old","state":"active"}]}`), "cluster-secrets-v2/node-identity.v2key": []byte("key")}}
	first, _ := json.Marshal(base)
	a, err := panelBackupRevision(first)
	if err != nil {
		t.Fatal(err)
	}
	base.Files["cluster-state-v2.json"] = []byte(`{"controllers":[{"id":"x","lastSeenAt":"new","state":"active"}]}`)
	second, _ := json.Marshal(base)
	b, _ := panelBackupRevision(second)
	if a != b {
		t.Fatal("heartbeat invalidates restore preview")
	}
	base.Files["cluster-secrets-v2/node-identity.v2key"] = []byte("different-key")
	third, _ := json.Marshal(base)
	c, _ := panelBackupRevision(third)
	if c == b {
		t.Fatal("key change was ignored")
	}
}

func TestBackupPanelOfflineTransactionRecovery(t *testing.T) {
	for _, commit := range []bool{false, true} {
		t.Run(map[bool]string{false: "rollback-after-apply", true: "finish-committed"}[commit], func(t *testing.T) {
			s, token := newTestServer(t)
			bootstrapCookies(t, s, token)
			s.config.TOTPKeyPath = filepath.Join(s.config.DataDir, "totp.key")
			data, err := s.exportPanelBackup(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			original, err := s.store.UserByUsername("admin")
			if err != nil {
				t.Fatal(err)
			}
			if err := s.store.ReplaceUserUsername(original.ID, "admin", "changed-admin", time.Now()); err != nil {
				t.Fatal(err)
			}
			record, err := s.backups.Reserve("restore", []string{"panel"})
			if err != nil {
				t.Fatal(err)
			}
			if err := s.backups.Update(record.ID, func(r *backup.Record) { r.Status = "restarting" }); err != nil {
				t.Fatal(err)
			}
			if err := stagePanelRestore(s.config, record.ID, data); err != nil {
				t.Fatal(err)
			}
			s.Close()
			if err := s.store.Close(); err != nil {
				t.Fatal(err)
			}
			if err := ApplyPanelRestore(s.config); err != nil {
				t.Fatal(err)
			}
			if commit {
				body, err := backup.ReadFile(panelRestorePath(s.config), 96<<20)
				if err != nil {
					t.Fatal(err)
				}
				var j panelRestoreJournal
				if backup.Decode(body, &j) != nil {
					t.Fatal("invalid journal")
				}
				j.Phase = "committed"
				if err := backup.WriteJSON(panelRestorePath(s.config), j); err != nil {
					t.Fatal(err)
				}
			}
			if err := RecoverPanelRestore(s.config); err != nil {
				t.Fatal(err)
			}
			reopened, err := store.Open(s.config.StorePath)
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			expected := "changed-admin"
			if commit {
				expected = "admin"
			}
			if _, err := reopened.UserByUsername(expected); err != nil {
				t.Fatal("wrong account after recovery", err)
			}
			if PanelRestorePending(s.config) {
				t.Fatal("completed recovery left active journal")
			}
			if err := RecoverPanelRestore(s.config); err != nil {
				t.Fatal("recovery not idempotent", err)
			}
		})
	}
}

func TestBackupPendingCorruptionDoesNotBlockSubsequentStartup(t *testing.T) {
	s, token := newTestServer(t)
	bootstrapCookies(t, s, token)
	data, err := s.exportPanelBackup(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.backups.Reserve("restore", []string{"panel"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.backups.Update(r.ID, func(r *backup.Record) { r.Status = "restarting" }); err != nil {
		t.Fatal(err)
	}
	if err := stagePanelRestore(s.config, r.ID, data); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.backups.Root, r.ID, "panel-restore.payload"), []byte("damaged"), 0600); err != nil {
		t.Fatal(err)
	}
	s.Close()
	if err := s.store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := ApplyPanelRestore(s.config); err == nil {
		t.Fatal("damaged intent accepted")
	}
	if PanelRestorePending(s.config) {
		t.Fatal("preflight failure left replayable intent")
	}
	if err := RecoverPanelRestore(s.config); err != nil {
		t.Fatal("subsequent startup blocked", err)
	}
	body, err := backup.ReadFile(filepath.Join(s.backups.Root, r.ID, "record.json"), 32<<10)
	if err != nil || backup.Decode(body, &r) != nil || r.Status != "failed" {
		t.Fatal("failure not recorded", err)
	}
	st, err := store.Open(s.config.StorePath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := st.UserByUsername("admin"); err != nil {
		t.Fatal("live account was changed", err)
	}
}
