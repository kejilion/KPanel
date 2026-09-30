package appmarket

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/hostpty"
)

func newTerminalLogTestService(t *testing.T, id, status string) (*Service, *appJobRegistry) {
	t.Helper()
	registry := &appJobRegistry{
		stateDir: filepath.Join(t.TempDir(), "jobs"),
		jobs:     make(map[string]appJobRecord),
	}
	if err := ensureAppJobDirectory(registry.stateDir); err != nil {
		t.Fatal(err)
	}
	record := appJobRecord{
		AppJob: AppJob{
			ID: id, AppID: "builtin-4", AppName: "test", Action: "manage",
			Interactive: true, InputOpen: status == "running", Status: status, Stage: "interactive",
			CreatedAt: time.Now().UTC(), Logs: []string{},
		},
		Adapter:  "kejilion",
		Selector: "4",
	}
	if err := registry.put(record); err != nil {
		t.Fatal(err)
	}
	return &Service{jobs: registry}, registry
}

func writeRollingTaskLog(t *testing.T, path string, total int64) []byte {
	t.Helper()
	log, err := hostpty.CreateOutputLog(path, hostpty.DefaultOutputLogBytes)
	if err != nil {
		t.Fatal(err)
	}
	line := []byte("0123456789 abcdefghij ABCDEFGHIJ\n")
	all := bytes.Repeat(line, int(total)/len(line)+1)[:total]
	if _, err := log.Write(all); err != nil {
		t.Fatal(err)
	}
	if err := log.Close(); err != nil {
		t.Fatal(err)
	}
	return all
}

func readAppTerminalChunk(t *testing.T, service *Service, id string, offset int64) (TerminalChunk, []byte) {
	t.Helper()
	chunk, err := service.AppJobTerminal(id, offset)
	if err != nil {
		t.Fatal(err)
	}
	data, err := base64.StdEncoding.DecodeString(chunk.DataBase64)
	if err != nil {
		t.Fatal(err)
	}
	return chunk, data
}

func TestAppJobTerminalStreamsARotatedLogAndReportsSkippedOutput(t *testing.T) {
	const id = "0123456789abcdef0123456789abcdef"
	service, registry := newTerminalLogTestService(t, id, "succeeded")
	total := int64(hostpty.DefaultOutputLogBytes*2 + 12345)
	all := writeRollingTaskLog(t, registry.logPath(id), total)

	chunk, data := readAppTerminalChunk(t, service, id, 0)
	oldest := total/hostpty.OutputSegmentBytes*hostpty.OutputSegmentBytes - hostpty.DefaultOutputLogBytes + hostpty.OutputSegmentBytes
	if !chunk.Truncated || chunk.Finished || chunk.NextOffset != oldest+int64(len(data)) || !bytes.Equal(data, all[oldest:chunk.NextOffset]) {
		t.Fatalf("catch-up chunk = next %d truncated %v finished %v, want data from %d", chunk.NextOffset, chunk.Truncated, chunk.Finished, oldest)
	}
	offset := chunk.NextOffset
	for !chunk.Finished {
		chunk, data = readAppTerminalChunk(t, service, id, offset)
		if chunk.Truncated || !bytes.Equal(data, all[offset:chunk.NextOffset]) || (len(data) == 0 && !chunk.Finished) {
			t.Fatalf("chunk at %d: next %d truncated %v finished %v", offset, chunk.NextOffset, chunk.Truncated, chunk.Finished)
		}
		offset = chunk.NextOffset
	}
	if offset != total {
		t.Fatalf("finished at %d, want %d", offset, total)
	}

	tail := registry.logTail(id, 1)
	if len(tail) != 1 || !strings.HasSuffix(strings.TrimRight(string(all), "\n"), tail[0]) {
		t.Fatalf("log tail %q does not come from the latest segment", tail)
	}
}

func TestPrunedAppJobsRemoveEveryLogSegment(t *testing.T) {
	const id = "fedcba9876543210fedcba9876543210"
	_, registry := newTerminalLogTestService(t, id, "succeeded")
	writeRollingTaskLog(t, registry.logPath(id), hostpty.OutputSegmentBytes*3)
	record, err := registry.read(id)
	if err != nil {
		t.Fatal(err)
	}
	record.CreatedAt = time.Unix(1, 0).UTC()
	registry.jobs[id] = record
	// One more finished job than the retention limit makes the oldest one expire.
	for index := range 100 {
		other := record
		other.ID = fmt.Sprintf("%032d", index)
		other.CreatedAt = time.Unix(int64(100+index), 0).UTC()
		registry.jobs[other.ID] = other
	}
	registry.pruneLocked()
	matches, err := filepath.Glob(registry.logPath(id) + "*")
	if err != nil || len(matches) != 0 {
		t.Fatalf("log files left after pruning: %v %v", matches, err)
	}
	if _, err := os.Stat(registry.stateDir); err != nil {
		t.Fatal(err)
	}
}
