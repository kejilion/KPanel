package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/cluster/sshlogin"
	"github.com/kejilion/kejilion-panel/internal/contract"
)

type sshLoginBrokerTestRunner struct {
	journal []byte
}

func (r sshLoginBrokerTestRunner) Run(context.Context, string, ...string) ([]byte, error) {
	return r.journal, nil
}

func (r sshLoginBrokerTestRunner) LookPath(name string) (string, error) {
	return "/usr/bin/" + name, nil
}

func TestPollAndPublishSSHLoginWritesOnlyValidatedEvent(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("SSH login broker is Linux-only")
	}
	when := time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC)
	reader := sshlogin.NewReader(sshlogin.Config{
		Runner: sshLoginBrokerTestRunner{journal: []byte(`{"__CURSOR":"cursor-1","__REALTIME_TIMESTAMP":"` +
			strconv.FormatInt(when.UnixMicro(), 10) +
			`","SYSLOG_IDENTIFIER":"sshd","MESSAGE":"sshd: Accepted publickey for admin from 203.0.113.9 port 22 ssh2"}` + "\n")},
		EffectiveUID: func() int { return 0 },
	})
	outputPath := filepath.Join(t.TempDir(), "ssh-login.json")
	lastEventID := ""
	if err := pollAndPublishSSHLogin(context.Background(), reader, outputPath, &lastEventID); err != nil {
		t.Fatalf("pollAndPublishSSHLogin() error = %v", err)
	}
	if lastEventID != "cursor-1" {
		t.Fatalf("last event ID = %q, want cursor-1", lastEventID)
	}
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	var event contract.SSHLoginEvent
	if err := json.Unmarshal(content, &event); err != nil {
		t.Fatal(err)
	}
	if event.ID != "cursor-1" || !event.OccurredAt.Equal(when) || event.Username != "admin" || event.RemoteAddress != "203.0.113.9" || event.Method != "publickey" {
		t.Fatalf("published SSH event = %#v", event)
	}

	if err := pollAndPublishSSHLogin(context.Background(), reader, outputPath, &lastEventID); err != nil {
		t.Fatalf("duplicate poll error = %v", err)
	}
}
