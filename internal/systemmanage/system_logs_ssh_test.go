package systemmanage

import (
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

func TestParseSSHLoginEntryKeepsOnlyAcceptedSSHFields(t *testing.T) {
	occurredAt := time.Date(2026, 8, 31, 15, 0, 0, 0, time.UTC)
	event, ok := parseSSHLoginEntry(contract.SystemLogEntry{
		Timestamp:  &occurredAt,
		Cursor:     "cursor-1",
		Identifier: "sshd",
		Message:    "sshd[42]: Accepted publickey for admin from 2001:db8::10 port 22 ssh2",
	}, occurredAt.Add(time.Minute))
	if !ok {
		t.Fatal("parseSSHLoginEntry() rejected a valid accepted login")
	}
	if event.ID != "cursor-1" || event.Username != "admin" || event.RemoteAddress != "2001:db8::10" || event.Method != "publickey" || !event.OccurredAt.Equal(occurredAt) {
		t.Fatalf("parsed SSH event = %#v", event)
	}
	for _, message := range []string{
		"sshd[42]: Failed password for admin from 203.0.113.10 port 22 ssh2",
		"sshd[42]: Accepted publickey for admin from evil.example;rm -rf /",
		"systemd[1]: Started unrelated service",
	} {
		if _, ok := parseSSHLoginEntry(contract.SystemLogEntry{Identifier: "sshd", Message: message}, occurredAt); ok {
			t.Fatalf("parseSSHLoginEntry() accepted unsafe or non-login message: %q", message)
		}
	}
}
