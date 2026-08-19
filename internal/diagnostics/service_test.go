package diagnostics

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

const testCatalog = "startup noise\n" +
	"KPANEL_TEST_CATEGORY\taccess\tIP 与解锁\n" +
	"KPANEL_TEST_CATEGORY\thardware\t硬件性能\n" +
	"KPANEL_TEST_ITEM\tchatgpt\taccess\tChatGPT 解锁检测\t检测出口 IP\thttps://example.com/chatgpt.sh\t2\tlight\n" +
	"KPANEL_TEST_ITEM\tyabs\thardware\tYABS 性能测试\t测试 CPU、磁盘与网络\thttps://example.com/yabs.sh\t30\tintensive\n"

func fixedNow() time.Time {
	return time.Date(2026, time.July, 27, 12, 0, 0, 0, time.UTC)
}

func TestTerminalHeaderReturnsToColumnZero(t *testing.T) {
	var output bytes.Buffer
	writeTerminalHeader(&output, "三网线路测试", "https://example.com/backtrace/install.sh")

	want := "KPanel 体检：三网线路测试\r\n" +
		"来源：https://example.com/backtrace/install.sh\r\n\r\n"
	if output.String() != want {
		t.Fatalf("terminal header = %q, want %q", output.String(), want)
	}
}

type fakeRunner struct {
	mu    sync.Mutex
	calls [][]string
}

func (runner *fakeRunner) Run(_ context.Context, name string, arguments ...string) ([]byte, error) {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	runner.calls = append(runner.calls, append([]string{name}, arguments...))
	if name == "env" {
		return []byte(testCatalog), nil
	}
	if name == "systemd-run" {
		return []byte("queued"), nil
	}
	return nil, errors.New("unexpected command")
}

func (*fakeRunner) LookPath(name string) (string, error) {
	switch name {
	case "env", "bash", "systemd-run":
		return "/usr/bin/" + name, nil
	default:
		return "", errors.New("not found")
	}
}

func TestParseCatalogAcceptsScriptProtocol(t *testing.T) {
	catalog, err := parseCatalog([]byte(testCatalog))
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Categories) != 2 || len(catalog.Items) != 2 {
		t.Fatalf("catalog = %#v", catalog)
	}
	if catalog.Items[1].ID != "yabs" || catalog.Items[1].EstimatedMinutes != 30 ||
		catalog.Items[1].Impact != "intensive" {
		t.Fatalf("YABS item = %#v", catalog.Items[1])
	}
}

func TestParseCatalogRejectsUnknownCategoryAndInsecureSource(t *testing.T) {
	for _, input := range []string{
		"KPANEL_TEST_CATEGORY\taccess\tIP\n" +
			"KPANEL_TEST_ITEM\tbad\tmissing\tBad\tBad\thttps://example.com/run.sh\t1\tlight\n",
		"KPANEL_TEST_CATEGORY\taccess\tIP\n" +
			"KPANEL_TEST_ITEM\tbad\taccess\tBad\tBad\thttp://example.com/run.sh\t1\tlight\n",
	} {
		if _, err := parseCatalog([]byte(input)); !errors.Is(err, ErrUnsupported) {
			t.Fatalf("parseCatalog(%q) error = %v", input, err)
		}
	}
}

func TestStartUsesCatalogSelectorAndFixedSystemdWorker(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("diagnostic execution is Linux-only")
	}
	root := t.TempDir()
	runner := &fakeRunner{}
	service := &Service{
		runner:       runner,
		scriptFinder: func() (string, error) { return "/usr/local/bin/k", nil },
		now:          fixedNow,
		jobs:         make(map[string]record),
	}
	if err := service.configure(filepath.Join(root, "jobs"), filepath.Join(root, "agent"), runner); err != nil {
		t.Fatal(err)
	}
	job, err := service.Start(context.Background(), "yabs")
	if err != nil {
		t.Fatal(err)
	}
	if job.CheckID != "yabs" || job.Status != "queued" || job.SourceURL != "https://example.com/yabs.sh" {
		t.Fatalf("job = %#v", job)
	}
	if _, err := service.Start(context.Background(), "chatgpt"); !errors.Is(err, ErrConflict) {
		t.Fatalf("parallel diagnostic error = %v", err)
	}
	calls := strings.Join(flattenCalls(runner.calls), "\n")
	for _, required := range []string{
		"env KJ_TEST_NONINTERACTIVE=1",
		"systemd-run --unit=" + jobUnitPrefix + job.ID,
		"diagnostic-run --state-dir",
	} {
		if !strings.Contains(calls, required) {
			t.Fatalf("missing %q in calls:\n%s", required, calls)
		}
	}
}

func TestReloadMarksInterruptedJobFailed(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("diagnostic execution is Linux-only")
	}
	root := t.TempDir()
	runner := &fakeRunner{}
	first := &Service{
		runner:       runner,
		scriptFinder: func() (string, error) { return "/usr/local/bin/k", nil },
		now:          fixedNow,
		bootID:       func() string { return "boot-a" },
		jobs:         make(map[string]record),
	}
	stateDir := filepath.Join(root, "jobs")
	if err := first.configure(stateDir, filepath.Join(root, "agent"), runner); err != nil {
		t.Fatal(err)
	}
	job, err := first.Start(context.Background(), "yabs")
	if err != nil {
		t.Fatal(err)
	}

	reloaded := &Service{
		runner:       runner,
		scriptFinder: func() (string, error) { return "/usr/local/bin/k", nil },
		now:          func() time.Time { return fixedNow().Add(time.Minute) },
		bootID:       func() string { return "boot-b" },
		jobs:         make(map[string]record),
	}
	if err := reloaded.configure(stateDir, filepath.Join(root, "agent"), runner); err != nil {
		t.Fatal(err)
	}
	recovered, err := reloaded.Job(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Status != "failed" || recovered.Stage != "interrupted" ||
		recovered.FinishedAt == nil {
		t.Fatalf("recovered job = %#v", recovered)
	}
	if _, err := reloaded.Start(context.Background(), "chatgpt"); err != nil {
		t.Fatalf("interrupted job still blocked a new diagnostic: %v", err)
	}
}

func TestReloadMarksExpiredJobFailedWithoutBootID(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("diagnostic execution is Linux-only")
	}
	root := t.TempDir()
	runner := &fakeRunner{}
	first := &Service{
		runner:       runner,
		scriptFinder: func() (string, error) { return "/usr/local/bin/k", nil },
		now:          fixedNow,
		bootID:       func() string { return "" },
		jobs:         make(map[string]record),
	}
	stateDir := filepath.Join(root, "jobs")
	if err := first.configure(stateDir, filepath.Join(root, "agent"), runner); err != nil {
		t.Fatal(err)
	}
	job, err := first.Start(context.Background(), "yabs")
	if err != nil {
		t.Fatal(err)
	}

	reloaded := &Service{
		runner:       runner,
		scriptFinder: func() (string, error) { return "/usr/local/bin/k", nil },
		now:          func() time.Time { return fixedNow().Add(maxJobRuntime + time.Minute) },
		bootID:       func() string { return "" },
		jobs:         make(map[string]record),
	}
	if err := reloaded.configure(stateDir, filepath.Join(root, "agent"), runner); err != nil {
		t.Fatal(err)
	}
	recovered, err := reloaded.Job(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Status != "failed" || recovered.Stage != "interrupted" {
		t.Fatalf("expired job = %#v", recovered)
	}
}

func TestCompatibilityRequiresAcceptedExplicitProtocol(t *testing.T) {
	base := []byte("KJ_TEST_NONINTERACTIVE\nkpanel_test_catalog\nkpanel_run_test_noninteractive\n")
	if compatibleScript(append(base, []byte("permission_granted=\"false\"\n")...)) {
		t.Fatal("unaccepted script enabled diagnostics")
	}
	if !compatibleScript(append(base, []byte("permission_granted=\"true\"\n")...)) {
		t.Fatal("accepted diagnostic protocol was rejected")
	}
}

func TestLimitedWriterCapsOutput(t *testing.T) {
	var target bytes.Buffer
	writer := &limitedWriter{target: &target, remaining: 4}
	if count, err := writer.Write([]byte("abcdef")); err != nil || count != 6 {
		t.Fatalf("Write() = %d, %v", count, err)
	}
	if !strings.HasPrefix(target.String(), "abcd") ||
		!strings.Contains(target.String(), "后续内容已截断") {
		t.Fatalf("limited output = %q", target.String())
	}
}

func TestTerminalPreservesANSIOutputAndOffsets(t *testing.T) {
	stateDir := t.TempDir()
	id := strings.Repeat("a", 32)
	service := &Service{
		stateDir: stateDir,
		now:      fixedNow,
		bootID:   func() string { return "boot-a" },
		jobs:     make(map[string]record),
	}
	item := record{Job: Job{
		ID: id, CheckID: "ip-quality", CheckName: "IP 质量体检",
		Status: "succeeded", Stage: "completed", Progress: 100,
		Interactive: true, Logs: []string{}, CreatedAt: fixedNow(),
	}}
	if err := service.putLocked(item); err != nil {
		t.Fatal(err)
	}
	output := []byte("\x1b[31mfailed\x1b[0m\r\n")
	if err := os.WriteFile(service.logPath(id), output, 0o600); err != nil {
		t.Fatal(err)
	}
	chunk, err := service.Terminal(id, 0)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := base64.StdEncoding.DecodeString(chunk.DataBase64)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decoded, output) || chunk.NextOffset != int64(len(output)) || !chunk.Finished {
		t.Fatalf("terminal chunk = %#v, decoded = %q", chunk, decoded)
	}
}

func flattenCalls(calls [][]string) []string {
	result := make([]string, 0, len(calls))
	for _, call := range calls {
		result = append(result, strings.Join(call, " "))
	}
	return result
}
