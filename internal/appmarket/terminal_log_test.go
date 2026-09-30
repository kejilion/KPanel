package appmarket

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

// patternedOutput makes every byte depend on its absolute offset, so a chunk
// read from the wrong segment or position is detected.
func patternedOutput(start, length int64) []byte {
	data := make([]byte, length)
	for index := range data {
		data[index] = byte((start + int64(index)) % 251)
	}
	return data
}

func writeTerminalLog(t *testing.T, registry *appJobRegistry, id string, total int64) {
	t.Helper()
	writer, err := newTerminalLogWriter(registry, id)
	if err != nil {
		t.Fatal(err)
	}
	// Uneven writes cross segment boundaries mid-write.
	for written := int64(0); written < total; {
		size := min(int64(300_001), total-written)
		if count, err := writer.Write(patternedOutput(written, size)); err != nil || int64(count) != size {
			t.Fatalf("write = %d, %v", count, err)
		}
		written += size
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
}

func readTerminalChunk(t *testing.T, service *Service, id string, offset int64) (TerminalChunk, []byte) {
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

func TestTerminalLogKeepsStreamingPastTheOldCapWithBoundedDisk(t *testing.T) {
	const id = "0123456789abcdef0123456789abcdef"
	service, registry := newTerminalLogTestService(t, id, "running")
	total := int64(terminalLogSegmentBytes*terminalLogSegments*2 + 12345)
	writeTerminalLog(t, registry, id, total)

	segments, err := registry.terminalSegments(id)
	if err != nil {
		t.Fatal(err)
	}
	if len(segments) != terminalLogSegments || segments[0] != total/terminalLogSegmentBytes-terminalLogSegments+1 {
		t.Fatalf("retained segments = %v", segments)
	}
	onDisk := int64(0)
	for _, index := range segments {
		info, err := os.Stat(registry.terminalSegmentPath(id, index))
		if err != nil {
			t.Fatal(err)
		}
		onDisk += info.Size()
	}
	if onDisk > terminalLogSegmentBytes*terminalLogSegments {
		t.Fatalf("retained %d bytes, above the %d byte budget", onDisk, terminalLogSegmentBytes*terminalLogSegments)
	}

	// A reader that fell behind skips to the oldest retained byte and says so.
	chunk, data := readTerminalChunk(t, service, id, 0)
	oldest := segments[0] * terminalLogSegmentBytes
	if !chunk.Truncated || chunk.NextOffset != oldest+int64(len(data)) || !bytes.Equal(data, patternedOutput(oldest, int64(len(data)))) {
		t.Fatalf("catch-up chunk = next %d truncated %v, want data from %d", chunk.NextOffset, chunk.Truncated, oldest)
	}

	// A reader that keeps up crosses segment boundaries with exact bytes.
	offset := oldest
	for offset < total {
		chunk, data = readTerminalChunk(t, service, id, offset)
		if chunk.Truncated || chunk.Finished || !bytes.Equal(data, patternedOutput(offset, int64(len(data)))) {
			t.Fatalf("chunk at %d: truncated %v finished %v, %d bytes", offset, chunk.Truncated, chunk.Finished, len(data))
		}
		if len(data) == 0 {
			t.Fatalf("no progress at offset %d of %d", offset, total)
		}
		offset = chunk.NextOffset
	}
	chunk, data = readTerminalChunk(t, service, id, total)
	if len(data) != 0 || chunk.NextOffset != total || chunk.Truncated {
		t.Fatalf("end of log chunk = %#v", chunk)
	}

	tail := registry.logTail(id, 1)
	if len(tail) != 1 || !strings.HasSuffix(string(patternedOutput(0, total)), tail[0]) {
		t.Fatalf("log tail does not come from the latest segment")
	}
}

func TestTerminalLogTreatsAFullSegmentEndAsTheEndOfTheLog(t *testing.T) {
	const id = "abcdef0123456789abcdef0123456789"
	service, registry := newTerminalLogTestService(t, id, "running")
	writeTerminalLog(t, registry, id, terminalLogSegmentBytes)
	chunk, data := readTerminalChunk(t, service, id, terminalLogSegmentBytes)
	if len(data) != 0 || chunk.NextOffset != terminalLogSegmentBytes || chunk.Truncated {
		t.Fatalf("boundary chunk = %#v, want an empty read at the end", chunk)
	}
}

func TestFinishedTerminalLogFinishesOnlyAfterTheLastSegment(t *testing.T) {
	const id = "fedcba9876543210fedcba9876543210"
	service, registry := newTerminalLogTestService(t, id, "succeeded")
	total := int64(terminalLogSegmentBytes + 100)
	writeTerminalLog(t, registry, id, total)
	chunk, _ := readTerminalChunk(t, service, id, terminalLogSegmentBytes-10)
	if chunk.Finished {
		t.Fatalf("finished before the last segment: %#v", chunk)
	}
	chunk, _ = readTerminalChunk(t, service, id, terminalLogSegmentBytes)
	if !chunk.Finished || chunk.NextOffset != total {
		t.Fatalf("last chunk = %#v, want finished at %d", chunk, total)
	}

	registry.removeTerminalSegments(id)
	if _, err := os.Stat(registry.terminalSegmentPath(id, 1)); !os.IsNotExist(err) {
		t.Fatalf("rotated segment survived cleanup: %v", err)
	}
	if _, err := os.Stat(registry.logPath(id)); err != nil {
		t.Fatalf("cleanup of rotated segments removed the primary log: %v", err)
	}
}
