package agent

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kejilion/kejilion-panel/internal/backup"
	"github.com/kejilion/kejilion-panel/internal/hostbackup"
	"github.com/kejilion/kejilion-panel/internal/terminal"
)

type backupTerminalProcess struct {
	*io.PipeReader
	writer *io.PipeWriter
}

func (p *backupTerminalProcess) Write(b []byte) (int, error) { return len(b), nil }
func (p *backupTerminalProcess) Wait() error                 { return nil }
func (p *backupTerminalProcess) Kill() error                 { return p.writer.Close() }
func (p *backupTerminalProcess) Resize(uint16, uint16) error { return nil }

func TestBackupGateIncludesExistingShellAndAllowsClosingIt(t *testing.T) {
	s := testServer(t)
	r, w := io.Pipe()
	s.terminals = terminal.New(terminal.Config{Starter: func(uint16, uint16) (terminal.Process, error) { return &backupTerminalProcess{r, w}, nil }})
	defer s.terminals.CloseAll()
	session, err := s.terminals.Open("fixture", 24, 80)
	if err != nil {
		t.Fatal(err)
	}
	call := func(path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", path, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 32))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		return w
	}
	for _, path := range []string{"/v1/backups", "/v1/backups/" + strings.Repeat("a", 32) + "/recover"} {
		if result := call(path, `{}`); result.Code != 409 || !strings.Contains(result.Body.String(), "backup_host_busy") {
			t.Fatal(result.Code, result.Body.String())
		}
	}
	jobs, err := backup.OpenManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer jobs.Close()
	s.backups = &hostbackup.Service{Jobs: jobs}
	defer func() { s.backups = nil }()
	if _, err := jobs.Reserve("export", []string{"web"}); err != nil {
		t.Fatal(err)
	}
	if result := call("/v1/terminals/"+session.ID+"/input", `{"owner":"fixture","data":"aWQ="}`); result.Code != 409 {
		t.Fatal(result.Code)
	}
	if result := call("/v1/terminals/"+session.ID+"/close", `{"owner":"fixture"}`); result.Code != 200 {
		t.Fatal(result.Code, result.Body.String())
	}
	if s.terminals.Busy() {
		t.Fatal("closed terminal retained backup gate")
	}
}
