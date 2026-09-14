package systemmanage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

func TestParseJournalDiskUsage(t *testing.T) {
	tests := []struct {
		value string
		want  uint64
		ok    bool
	}{
		{"Archived and active journals take up 176.0M in the file system.", 176 << 20, true},
		{"Journals take up 1.5 GiB.", 3 << 29, true},
		{"0B", 0, true},
		{"unavailable", 0, false},
	}
	for _, test := range tests {
		got, ok := parseJournalDiskUsage(test.value)
		if got != test.want || ok != test.ok {
			t.Fatalf("parseJournalDiskUsage(%q) = %d, %v; want %d, %v", test.value, got, ok, test.want, test.ok)
		}
	}
}

func TestVarLogUsageUsesActualSameFilesystemBlocks(t *testing.T) {
	runner := &fakeRunner{run: func(_ context.Context, name string, _ ...string) ([]byte, error) {
		if name == "du" {
			return []byte("12\t/var/log\n"), nil
		}
		return nil, nil
	}}
	manager, _, _, _ := testManager(t, runner)
	manager.logRoot = filepath.Join(t.TempDir(), "log")
	usage := manager.varLogUsage(context.Background())
	if !usage.Available || usage.Bytes != 12*1024 {
		t.Fatalf("unexpected /var/log usage: %#v", usage)
	}
	if runner.commands[0] != "du -skx -- "+manager.logRoot {
		t.Fatalf("unexpected du command: %q", runner.commands[0])
	}
}

func TestSystemLogCapabilitiesUseDedicatedReadAndWriteIDs(t *testing.T) {
	manager, _, _, _ := testManager(t, &fakeRunner{})
	capabilities := manager.SystemLogCapabilities()
	if len(capabilities) != 2 || capabilities[0].ID != "system.logs.read" || capabilities[1].ID != "system.logs.write" {
		t.Fatalf("unexpected log capabilities: %#v", capabilities)
	}
}

func TestSystemLogSummaryKeepsJournalSourceWhenDiskUsageFails(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("system log summary is Linux-only")
	}
	runner := &fakeRunner{run: func(_ context.Context, name string, arguments ...string) ([]byte, error) {
		switch name {
		case "du":
			return []byte("4\t/var/log\n"), nil
		case "journalctl":
			if len(arguments) > 0 && arguments[0] == "--disk-usage" {
				return nil, errors.New("disk usage unavailable")
			}
			return []byte(`{"MESSAGE":"journal remains readable"}` + "\n"), nil
		case "systemctl":
			return nil, nil
		default:
			return nil, nil
		}
	}}
	manager, _, _, _ := testManager(t, runner)
	summary, err := manager.SystemLogSummary(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if summary.Journal.Available || !summary.Sources.Journal.Available || !summary.Sources.System.Available ||
		!summary.Sources.System.SupportsPriority || !summary.Sources.Security.Available ||
		summary.AuthSource != "journal" {
		t.Fatalf("journal source was coupled to usage probe: %#v", summary)
	}
	snapshot, err := manager.SystemLogs(context.Background(), contract.SystemLogQuery{
		Source: "system", Limit: 50, Priority: "all",
	})
	if err != nil || len(snapshot.Entries) != 1 || snapshot.Entries[0].Message != "journal remains readable" {
		t.Fatalf("journal query failed after usage degradation: snapshot=%#v err=%v", snapshot, err)
	}
}

func TestSystemLogsPreferLiveOpenRCSyslogOverStrayJournalctl(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("system logs are Linux-only")
	}
	runner := &fakeRunner{run: func(_ context.Context, name string, _ ...string) ([]byte, error) {
		if name == "journalctl" {
			t.Fatalf("live OpenRC attempted to use stray journalctl")
		}
		if name == "du" {
			return []byte("4\t/var/log\n"), nil
		}
		return nil, nil
	}}
	manager, _, _, _ := testManager(t, runner)
	if err := os.MkdirAll(filepath.Join(manager.runRoot, "openrc"), 0o755); err != nil {
		t.Fatal(err)
	}
	manager.logRoot = t.TempDir()
	messagesPath := filepath.Join(manager.logRoot, "messages")
	if err := os.WriteFile(messagesPath, []byte(
		"Sep 14 12:00:00 alpine sshd[42]: Accepted publickey for root from 192.0.2.1 port 22 ssh2\n",
	), 0o600); err != nil {
		t.Fatal(err)
	}

	summary, err := manager.SystemLogSummary(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !summary.Sources.System.Available || summary.Sources.System.SupportsPriority ||
		summary.Sources.Journal.Available || summary.Sources.Journal.Reason == "" ||
		!summary.Sources.Security.Available || summary.AuthSource != messagesPath {
		t.Fatalf("unexpected OpenRC log summary: %#v", summary)
	}

	systemLogs, err := manager.SystemLogs(context.Background(), contract.SystemLogQuery{
		Source: "system", Limit: 50, Priority: "all",
	})
	if err != nil || len(systemLogs.Entries) != 1 || systemLogs.Entries[0].Identifier != "sshd" ||
		systemLogs.Entries[0].PID != 42 {
		t.Fatalf("OpenRC system logs = %#v, %v", systemLogs, err)
	}
	securityLogs, err := manager.SystemLogs(context.Background(), contract.SystemLogQuery{
		Source: "security", Limit: 50, Priority: "all",
	})
	if err != nil || securityLogs.AuthSource != messagesPath || len(securityLogs.Entries) != 1 {
		t.Fatalf("OpenRC security logs = %#v, %v", securityLogs, err)
	}
	login, err := manager.LatestSSHLogin(context.Background())
	if err != nil || login == nil || login.Username != "root" || login.RemoteAddress != "192.0.2.1" {
		t.Fatalf("OpenRC latest SSH login = %#v, %v", login, err)
	}
	_, err = manager.SystemLogs(context.Background(), contract.SystemLogQuery{
		Source: "service", Limit: 50, Priority: "warning",
	})
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("fixed syslog priority error = %v", err)
	}
}

func TestReadJournalLogsUsesFixedArgumentsAndStructuresEntries(t *testing.T) {
	longMessage := strings.Repeat("界", contract.SystemLogMaxMessageBytes)
	runner := &fakeRunner{run: func(_ context.Context, name string, _ ...string) ([]byte, error) {
		if name != "journalctl" {
			return nil, fmt.Errorf("unexpected command %s", name)
		}
		return []byte(
			`{"__CURSOR":"cursor-2","PRIORITY":"3","MESSAGE":` + strconv.Quote(longMessage) + `}` + "\n" +
				`{"__CURSOR":"cursor-1","__REALTIME_TIMESTAMP":"1785034800000000","PRIORITY":"4","UNIT":"ssh.service","SYSLOG_IDENTIFIER":"sshd","_PID":"42","MESSAGE":"password=hunter2 Bearer abc.def"}` + "\n",
		), nil
	}}
	manager, _, _, _ := testManager(t, runner)
	entries, truncated, err := manager.readJournalLogs(context.Background(), contract.SystemLogQuery{
		Source: "service", Limit: 50, Priority: "warning",
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if truncated || len(entries) != 2 {
		t.Fatalf("entries=%d truncated=%v", len(entries), truncated)
	}
	if entries[0].Timestamp == nil || entries[0].Cursor != "cursor-1" || entries[0].Priority != "warning" ||
		entries[0].Unit != "ssh.service" || entries[0].Identifier != "sshd" || entries[0].PID != 42 {
		t.Fatalf("unexpected structured entry: %#v", entries[0])
	}
	if strings.Contains(entries[0].Message, "hunter2") || strings.Contains(entries[0].Message, "abc.def") {
		t.Fatalf("secret was not redacted: %q", entries[0].Message)
	}
	if len(entries[1].Message) > contract.SystemLogMaxMessageBytes {
		t.Fatalf("message length = %d", len(entries[1].Message))
	}
	if len(runner.resourceLimits) != 1 || runner.resourceLimits[0] != contract.SystemLogMaxOutputBytes {
		t.Fatalf("journal output limits = %#v", runner.resourceLimits)
	}
	command := strings.Join(runner.commands, "\n")
	for _, required := range []string{
		"journalctl --no-pager --quiet --reverse --lines=51 --output=json",
		"__CURSOR,__REALTIME_TIMESTAMP",
		"--unit=*.service",
		"--priority=0..4",
	} {
		if !strings.Contains(command, required) {
			t.Fatalf("journal command missing %q: %s", required, command)
		}
	}
	if strings.Contains(command, "sh -c") || strings.Contains(command, "bash -c") {
		t.Fatalf("journal query used a shell: %s", command)
	}
}

func TestReadJournalLogsKeepsNewestWindowInChronologicalOrder(t *testing.T) {
	var output strings.Builder
	for index := 51; index >= 1; index-- {
		fmt.Fprintf(&output, `{"__CURSOR":"cursor-%02d","MESSAGE":"message-%02d"}`+"\n", index, index)
	}
	runner := &fakeRunner{run: func(_ context.Context, name string, _ ...string) ([]byte, error) {
		if name == "journalctl" {
			return []byte(output.String()), nil
		}
		return nil, nil
	}}
	manager, _, _, _ := testManager(t, runner)
	entries, truncated, err := manager.readJournalLogs(context.Background(), contract.SystemLogQuery{
		Source: "system", Limit: 50, Priority: "all",
	}, false)
	if err != nil || !truncated || len(entries) != 50 {
		t.Fatalf("entries=%d truncated=%v err=%v", len(entries), truncated, err)
	}
	if entries[0].Cursor != "cursor-02" || entries[len(entries)-1].Cursor != "cursor-51" {
		t.Fatalf("journal window is not oldest-to-newest: %q ... %q", entries[0].Cursor, entries[len(entries)-1].Cursor)
	}
}

func TestReadJournalLogsKeepsLatestBoundedPrefixAfterOutputLimit(t *testing.T) {
	output := strings.Join([]string{
		`{"__CURSOR":"cursor-3","MESSAGE":"newest"}`,
		`{"__CURSOR":"cursor-2","MESSAGE":"middle"}`,
		`{"__CURSOR":"cursor-1","MESSAGE":"oldest"}`,
	}, "\n") + "\n"
	runner := &fakeRunner{run: func(_ context.Context, name string, _ ...string) ([]byte, error) {
		if name == "journalctl" {
			return []byte(output), errResourceOutputTooLarge
		}
		return nil, nil
	}}
	manager, _, _, _ := testManager(t, runner)
	entries, truncated, err := manager.readJournalLogs(context.Background(), contract.SystemLogQuery{
		Source: "system", Limit: 50, Priority: "all",
	}, false)
	if err != nil || !truncated || len(entries) != 3 {
		t.Fatalf("entries=%#v truncated=%v err=%v", entries, truncated, err)
	}
	if entries[0].Cursor != "cursor-1" || entries[1].Cursor != "cursor-2" || entries[2].Cursor != "cursor-3" {
		t.Fatalf("bounded journal prefix was not restored to chronological order: %#v", entries)
	}
	if len(runner.resourceLimits) != 1 || runner.resourceLimits[0] != contract.SystemLogMaxOutputBytes ||
		!strings.Contains(runner.commands[0], "--reverse") {
		t.Fatalf("journal limit/ordering command mismatch: limits=%#v commands=%#v", runner.resourceLimits, runner.commands)
	}
}

func TestReadJournalLogsUsesSharedStatefulCredentialRedaction(t *testing.T) {
	chronological := []string{
		strings.Join([]string{
			"window-private-body",
			"-----END RSA PRIVATE KEY-----",
			"safe-after-orphan",
			"-----BEGIN RSA PRIVATE KEY-----",
			"later-private-body",
			"-----END RSA PRIVATE KEY-----",
			"safe-after-block",
		}, "\n"),
		`refresh_token=refresh-secret`,
		`AWS_SECRET_ACCESS_KEY=aws-secret`,
		`pwd=pwd-secret`,
		`password=p@ss#2026`,
		`token=abc&def`,
		`Authorization: Basic YmFzaWMtc2VjcmV0`,
		`Authorization: ApiKey arbitrary-auth-secret`,
		`Cookie: session=cookie-secret; csrf=csrf-secret`,
		`https://url-user:url-pass@example.test/path?access_token=query-secret&safe=visible`,
		`https://token-only@example.test/path`,
		`-----BEGIN OPENSSH PRIVATE KEY-----`,
		`cHJpdmF0ZS1rZXktbWF0ZXJpYWw=`,
		`-----END OPENSSH PRIVATE KEY-----`,
	}
	var output strings.Builder
	for index := len(chronological) - 1; index >= 0; index-- {
		record, err := json.Marshal(map[string]string{
			"__CURSOR": fmt.Sprintf("cursor-%d", index),
			"MESSAGE":  chronological[index],
		})
		if err != nil {
			t.Fatal(err)
		}
		output.Write(record)
		output.WriteByte('\n')
	}
	runner := &fakeRunner{run: func(_ context.Context, name string, _ ...string) ([]byte, error) {
		if name == "journalctl" {
			return []byte(output.String()), nil
		}
		return nil, nil
	}}
	manager, _, _, _ := testManager(t, runner)
	entries, _, err := manager.readJournalLogs(context.Background(), contract.SystemLogQuery{
		Source: "system", Limit: 50, Priority: "all",
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	messages := make([]string, 0, len(entries))
	for _, entry := range entries {
		messages = append(messages, entry.Message)
	}
	joined := strings.Join(messages, "\n")
	for _, secret := range []string{
		"refresh-secret", "aws-secret", "pwd-secret", "p@ss", "#2026", "abc", "&def",
		"YmFzaWMtc2VjcmV0", "ApiKey", "arbitrary-auth-secret",
		"cookie-secret", "csrf-secret", "url-user", "url-pass", "query-secret", "token-only",
		"window-private-body", "later-private-body", "cHJpdmF0ZS1rZXktbWF0ZXJpYWw",
	} {
		if strings.Contains(joined, secret) {
			t.Fatalf("system log redaction leaked %q: %s", secret, joined)
		}
	}
	if !strings.Contains(joined, "safe=visible") || !strings.Contains(joined, "safe-after-orphan") ||
		!strings.Contains(joined, "safe-after-block") || !strings.Contains(joined, "[REDACTED PRIVATE KEY]") {
		t.Fatalf("system log redaction removed safe data or marker: %s", joined)
	}
}

func TestReadJournalLogsRedactsBeforeDroppingOldestOverflow(t *testing.T) {
	chronological := make([]string, 51)
	chronological[0] = "-----BEGIN RSA PRIVATE KEY-----"
	chronological[1] = "cHJpdmF0ZS1ib2R5"
	chronological[2] = "-----END RSA PRIVATE KEY-----"
	for index := 3; index < len(chronological); index++ {
		chronological[index] = fmt.Sprintf("safe-%02d", index)
	}
	var output strings.Builder
	for index := len(chronological) - 1; index >= 0; index-- {
		record, err := json.Marshal(map[string]string{
			"__CURSOR": fmt.Sprintf("cursor-%02d", index),
			"MESSAGE":  chronological[index],
		})
		if err != nil {
			t.Fatal(err)
		}
		output.Write(record)
		output.WriteByte('\n')
	}
	runner := &fakeRunner{run: func(_ context.Context, name string, _ ...string) ([]byte, error) {
		if name == "journalctl" {
			return []byte(output.String()), nil
		}
		return nil, nil
	}}
	manager, _, _, _ := testManager(t, runner)
	entries, truncated, err := manager.readJournalLogs(context.Background(), contract.SystemLogQuery{
		Source: "system", Limit: 50, Priority: "all",
	}, false)
	if err != nil || !truncated || len(entries) != 50 {
		t.Fatalf("entries=%#v truncated=%v err=%v", entries, truncated, err)
	}
	messages := make([]string, 0, len(entries))
	for _, entry := range entries {
		messages = append(messages, entry.Message)
	}
	if entries[0].Cursor != "cursor-01" || entries[len(entries)-1].Cursor != "cursor-50" ||
		strings.Contains(strings.Join(messages, "\n"), "cHJpdmF0ZS1ib2R5") {
		t.Fatalf("overflow window leaked a key after dropping BEGIN: %#v", entries[:3])
	}
}

func TestReadSecurityJournalUsesAuthAndAuthprivAsOneORQuery(t *testing.T) {
	runner := &fakeRunner{run: func(_ context.Context, name string, _ ...string) ([]byte, error) {
		if name == "journalctl" {
			return []byte(`{"MESSAGE":"accepted","SYSLOG_IDENTIFIER":"sshd"}` + "\n"), nil
		}
		return nil, nil
	}}
	manager, _, _, _ := testManager(t, runner)
	entries, _, err := manager.readJournalLogs(context.Background(), contract.SystemLogQuery{
		Source: "security", Limit: 50, Priority: "all",
	}, true)
	if err != nil || len(entries) != 1 {
		t.Fatalf("entries=%#v err=%v", entries, err)
	}
	if len(runner.commands) != 1 {
		t.Fatalf("auth facilities were split across commands: %#v", runner.commands)
	}
	command := runner.commands[0]
	if !strings.Contains(command, "SYSLOG_FACILITY=4") || !strings.Contains(command, "SYSLOG_FACILITY=10") {
		t.Fatalf("auth OR matches missing: %s", command)
	}
}

func TestReadLoginLogsUsesFixedLastLimit(t *testing.T) {
	runner := &fakeRunner{run: func(_ context.Context, name string, _ ...string) ([]byte, error) {
		if name == "last" {
			return []byte("new-user pts/0 203.0.113.1 Mon Aug 24 09:00 still logged in\nold-user pts/1 203.0.113.2 Mon Aug 24 08:00 - 08:30\nwtmp begins Mon Aug 24 08:00:00 2026\n"), nil
		}
		return nil, nil
	}}
	manager, _, _, _ := testManager(t, runner)
	entries, truncated, err := manager.readLoginLogs(context.Background(), 50)
	if err != nil || truncated || len(entries) != 2 || entries[0].Identifier != "last" {
		t.Fatalf("entries=%#v truncated=%v err=%v", entries, truncated, err)
	}
	if !strings.HasPrefix(entries[0].Message, "old-user") || !strings.HasPrefix(entries[1].Message, "new-user") {
		t.Fatalf("login history is not oldest-to-newest: %#v", entries)
	}
	if runner.commands[0] != "last -n 51" {
		t.Fatalf("unexpected last command: %q", runner.commands[0])
	}
}

func TestSystemLogReadersPreserveDeadlineExceeded(t *testing.T) {
	contextWithDeadline, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	cancel()
	runner := &fakeRunner{run: func(ctx context.Context, _ string, _ ...string) ([]byte, error) {
		return nil, ctx.Err()
	}}
	manager, _, _, _ := testManager(t, runner)
	if _, _, err := manager.readJournalLogs(contextWithDeadline, contract.SystemLogQuery{
		Source: "system", Limit: 50, Priority: "all",
	}, false); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("journal error = %v, want deadline exceeded", err)
	}
	if _, _, err := manager.readLoginLogs(contextWithDeadline, 50); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("last error = %v, want deadline exceeded", err)
	}
}

func TestFixedAuthLogTailUsesOnlyKnownRegularFiles(t *testing.T) {
	runner := &fakeRunner{}
	manager, _, _, _ := testManager(t, runner)
	manager.logRoot = t.TempDir()
	if err := os.WriteFile(filepath.Join(manager.logRoot, "secure"), []byte("old\npassword=secret\naccepted\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path, entries, truncated, err := manager.readFixedAuthLog(2)
	if err != nil || !truncated || path != filepath.Join(manager.logRoot, "secure") || len(entries) != 2 {
		t.Fatalf("path=%q entries=%#v truncated=%v err=%v", path, entries, truncated, err)
	}
	if strings.Contains(entries[0].Message, "secret") || entries[1].Message != "accepted" {
		t.Fatalf("unexpected auth tail: %#v", entries)
	}

	if err := os.Remove(filepath.Join(manager.logRoot, "secure")); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(manager.logRoot, "outside")
	if err := os.WriteFile(target, []byte("must not read\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(manager.logRoot, "auth.log")); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}
	if _, _, _, err := manager.readFixedAuthLog(50); err == nil {
		t.Fatal("authentication log symlink was accepted")
	}
}

func TestFixedAuthLogRedactsBeforeTakingLatestLineWindow(t *testing.T) {
	manager, _, _, _ := testManager(t, &fakeRunner{})
	manager.logRoot = t.TempDir()
	content := strings.Join([]string{
		"-----BEGIN RSA PRIVATE KEY-----",
		"cHJpdmF0ZS1ib2R5",
		"-----END RSA PRIVATE KEY-----",
		"accepted",
	}, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(manager.logRoot, "secure"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	_, entries, truncated, err := manager.readFixedAuthLog(3)
	if err != nil || !truncated || len(entries) != 3 {
		t.Fatalf("entries=%#v truncated=%v err=%v", entries, truncated, err)
	}
	messages := make([]string, 0, len(entries))
	for _, entry := range entries {
		messages = append(messages, entry.Message)
	}
	if strings.Contains(strings.Join(messages, "\n"), "cHJpdmF0ZS1ib2R5") || entries[2].Message != "accepted" {
		t.Fatalf("fixed authentication log leaked a key after taking the tail: %#v", entries)
	}
}

func TestLogCleanupPlansRotateBeforeExactVacuumPolicy(t *testing.T) {
	manager, _, _, _ := testManager(t, &fakeRunner{})
	tests := map[string]string{
		"retain-7d": "--vacuum-time=7d",
		"retain-3d": "--vacuum-time=3d",
		"max-500m":  "--vacuum-size=500M",
	}
	for policy, vacuum := range tests {
		action, gotPolicy, steps, err := manager.maintenanceSteps("log-cleanup-" + policy)
		if err != nil {
			t.Fatal(err)
		}
		if action != "log-cleanup" || gotPolicy != policy || len(steps) != 2 ||
			steps[0].command != "journalctl" || strings.Join(steps[0].arguments, " ") != "--rotate" ||
			steps[1].command != "journalctl" || strings.Join(steps[1].arguments, " ") != vacuum {
			t.Fatalf("policy %s returned action=%q policy=%q steps=%#v", policy, action, gotPolicy, steps)
		}
	}
	if _, _, _, err := manager.maintenanceSteps("log-cleanup-forever"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("unknown policy error = %v", err)
	}
}

func TestLogCleanupSuccessMessagesDoNotAssumeJournald(t *testing.T) {
	for _, policy := range []string{"retain-7d", "retain-3d", "max-500m"} {
		message := maintenanceSuccessMessage("log-cleanup", policy, false)
		if !strings.Contains(message, "系统日志归档") || strings.Contains(message, "journal") {
			t.Fatalf("policy %s returned backend-specific message %q", policy, message)
		}
	}
}

func TestLogCleanupUsesLiveOpenRCSyslogDespiteStrayJournalctl(t *testing.T) {
	runner := &fakeRunner{}
	manager, _, _, _ := testManager(t, runner)
	if err := os.MkdirAll(filepath.Join(manager.runRoot, "openrc"), 0o755); err != nil {
		t.Fatal(err)
	}
	manager.logRoot = t.TempDir()
	if err := os.WriteFile(filepath.Join(manager.logRoot, "messages"), []byte("current\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(manager.logRoot, "messages.1.gz")
	recent := filepath.Join(manager.logRoot, "messages.0")
	unrelated := filepath.Join(manager.logRoot, "fail2ban.log.9.gz")
	for _, path := range []string{old, recent, unrelated} {
		if err := os.WriteFile(path, []byte("archive\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	manager.now = func() time.Time { return now }
	if err := os.Chtimes(old, now.Add(-8*24*time.Hour), now.Add(-8*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(recent, now.Add(-time.Hour), now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	action, policy, steps, err := manager.maintenanceSteps("log-cleanup-retain-7d")
	if err != nil || action != "log-cleanup" || policy != "retain-7d" || len(steps) != 1 ||
		steps[0].operation != maintenanceOperationSyslogCleanup {
		t.Fatalf("OpenRC syslog plan = %q %q %#v, %v", action, policy, steps, err)
	}
	if err := manager.pruneRotatedSystemLogs(context.Background(), policy); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(old); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("old rotated log remains: %v", err)
	}
	for _, path := range []string{filepath.Join(manager.logRoot, "messages"), recent, unrelated} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("protected log %s was removed: %v", path, err)
		}
	}
}

func TestValidSystemLogCleanupRequestRejectsUnrelatedFields(t *testing.T) {
	if !validSystemLogCleanupRequest(contract.SystemActionRequest{
		Action: "log-cleanup", MaintenancePolicy: "retain-7d",
	}) {
		t.Fatal("valid log cleanup request was rejected")
	}
	if validSystemLogCleanupRequest(contract.SystemActionRequest{
		Action: "log-cleanup", MaintenancePolicy: "retain-7d", Hostname: "ignored",
	}) {
		t.Fatal("unrelated system action field was accepted")
	}
	if validSystemLogCleanupRequest(contract.SystemActionRequest{
		Action: "log-cleanup", MaintenancePolicy: "retain-7d", Confirmation: "anything",
	}) {
		t.Fatal("confirmation text was accepted as log cleanup input")
	}
	if validSystemLogCleanupRequest(contract.SystemActionRequest{
		Action: "log-cleanup", MaintenancePolicy: "forever",
	}) {
		t.Fatal("unknown log cleanup policy was accepted")
	}
}

func TestStartLogCleanupUsesNarrowSystemdSandbox(t *testing.T) {
	runner := &fakeRunner{}
	manager, _, _, _ := testManager(t, runner)
	manager.executable = filepath.Join(t.TempDir(), "kejilion-agent")
	changed, _, taskID, err := manager.startMaintenanceTask(context.Background(), "log-cleanup", "retain-3d")
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	status := manager.readMaintenance()
	if taskID == "" || taskID != status.ID || status.Action != "log-cleanup" || status.Policy != "retain-3d" {
		t.Fatalf("task identity does not match persisted maintenance: taskID=%q status=%#v", taskID, status)
	}
	command := runner.commands[len(runner.commands)-1]
	for _, required := range []string{
		"systemd-run ",
		"--property=TimeoutStartSec=10min",
		"--property=ProtectSystem=strict",
		"--property=ReadWritePaths=" + manager.stateDir + " -/var/log/journal -/run/log/journal",
		"--property=NoNewPrivileges=yes",
		"--property=CapabilityBoundingSet=",
		"--property=RestrictAddressFamilies=AF_UNIX",
		"maintenance-run --state-dir " + manager.stateDir + " log-cleanup-retain-3d",
	} {
		if !strings.Contains(command, required) {
			t.Fatalf("systemd sandbox missing %q: %s", required, command)
		}
	}
	if strings.Contains(command, "AF_INET") || strings.Contains(command, "NoNewPrivileges=no") {
		t.Fatalf("log cleanup inherited broad maintenance sandbox: %s", command)
	}
}

func TestExecuteLogCleanupReturnsExactPersistedTaskIdentity(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("system actions are Linux-only")
	}
	manager, _, _, _ := testManager(t, &fakeRunner{})
	result, err := manager.Execute(context.Background(), contract.SystemActionRequest{
		Action: "log-cleanup", MaintenancePolicy: "retain-3d",
	})
	if err != nil {
		t.Fatal(err)
	}
	status := manager.MaintenanceStatus()
	if !result.Changed || result.Status != "accepted" || result.Action != "log-cleanup" ||
		result.MaintenancePolicy != "retain-3d" || result.TaskID == "" || result.TaskID != status.ID ||
		status.Action != result.Action || status.Policy != result.MaintenancePolicy {
		t.Fatalf("result=%#v maintenance=%#v", result, status)
	}
}

func TestRunLogCleanupPersistsRealVacuumFailure(t *testing.T) {
	call := 0
	runner := &fakeRunner{run: func(_ context.Context, name string, arguments ...string) ([]byte, error) {
		if name != "journalctl" {
			return nil, nil
		}
		call++
		if len(arguments) == 1 && arguments[0] == "--vacuum-size=500M" {
			return nil, errors.New("vacuum failed")
		}
		return nil, nil
	}}
	manager, _, _, _ := testManager(t, runner)
	err := manager.RunMaintenance(context.Background(), "log-cleanup-max-500m")
	if err == nil || call != 2 {
		t.Fatalf("call=%d err=%v", call, err)
	}
	status := manager.readMaintenance()
	if status.State != "failed" || status.Stage != "log_journal_max_500m" || status.FinishedAt == nil {
		t.Fatalf("failure was not persisted: %#v", status)
	}
}
