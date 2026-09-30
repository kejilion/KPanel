package hostpty

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// patternedOutput makes every byte depend on its absolute offset, so a read
// from the wrong segment or position is detected.
func patternedOutput(start, length int64) []byte {
	data := make([]byte, length)
	for index := range data {
		data[index] = byte((start + int64(index)) % 251)
	}
	return data
}

func writeOutputLog(t *testing.T, path string, maxBytes, total int64) {
	t.Helper()
	log, err := CreateOutputLog(path, maxBytes)
	if err != nil {
		t.Fatal(err)
	}
	// Uneven writes cross segment boundaries mid-write.
	for written := int64(0); written < total; {
		size := min(int64(300_001), total-written)
		if count, err := log.Write(patternedOutput(written, size)); err != nil || int64(count) != size {
			t.Fatalf("write = %d, %v", count, err)
		}
		written += size
	}
	if err := log.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestOutputLogKeepsWritingPastItsBudgetWithBoundedDisk(t *testing.T) {
	for _, maxBytes := range []int64{DefaultOutputLogBytes, 32 << 20} {
		path := filepath.Join(t.TempDir(), "job.log")
		retained := maxBytes / OutputSegmentBytes
		total := maxBytes*2 + 12345
		writeOutputLog(t, path, maxBytes, total)

		segments, err := outputSegments(path)
		if err != nil {
			t.Fatal(err)
		}
		if int64(len(segments)) != retained || segments[0] != total/OutputSegmentBytes-retained+1 {
			t.Fatalf("budget %d: retained segments = %v", maxBytes, segments)
		}
		onDisk := int64(0)
		for _, index := range segments {
			info, err := os.Stat(segmentPath(path, index))
			if err != nil {
				t.Fatal(err)
			}
			onDisk += info.Size()
		}
		if onDisk > maxBytes {
			t.Fatalf("retained %d bytes, above the %d byte budget", onDisk, maxBytes)
		}

		// A reader that fell behind skips to the oldest retained byte and says so.
		read, err := ReadOutputLog(path, 0, 64<<10)
		oldest := segments[0] * OutputSegmentBytes
		if err != nil || !read.Truncated || read.NextOffset != oldest+int64(len(read.Data)) ||
			!bytes.Equal(read.Data, patternedOutput(oldest, int64(len(read.Data)))) {
			t.Fatalf("catch-up read = next %d truncated %v err %v, want data from %d", read.NextOffset, read.Truncated, err, oldest)
		}

		// A reader that keeps up crosses segment boundaries with exact bytes.
		for offset := oldest; offset < total; {
			read, err = ReadOutputLog(path, offset, 64<<10)
			if err != nil || read.Truncated || len(read.Data) == 0 ||
				!bytes.Equal(read.Data, patternedOutput(offset, int64(len(read.Data)))) {
				t.Fatalf("read at %d: %d bytes truncated %v err %v", offset, len(read.Data), read.Truncated, err)
			}
			offset = read.NextOffset
		}
		if end, err := OutputLogEnd(path); err != nil || end != total {
			t.Fatalf("end = %d, %v; want %d", end, err, total)
		}
		tail, err := OutputLogTail(path, 3<<20)
		if err != nil || !bytes.Equal(tail, patternedOutput(total-3<<20, 3<<20)) {
			t.Fatalf("tail = %d bytes, %v", len(tail), err)
		}
	}
}

func TestOutputLogTreatsAFullSegmentEndAsTheEndOfTheLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "job.log")
	writeOutputLog(t, path, DefaultOutputLogBytes, OutputSegmentBytes)
	read, err := ReadOutputLog(path, OutputSegmentBytes, 64<<10)
	if err != nil || len(read.Data) != 0 || read.NextOffset != OutputSegmentBytes || read.Truncated {
		t.Fatalf("boundary read = %#v, %v; want an empty read at the end", read, err)
	}
}

func TestOutputLogReadsALegacySingleFileLogPastOneSegment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "job.log")
	// Older Agents wrote one file of up to 32 MiB for every task.
	total := int64(OutputSegmentBytes*2 + 777)
	legacy := patternedOutput(0, total)
	if err := os.WriteFile(path, legacy, 0o600); err != nil {
		t.Fatal(err)
	}
	for offset := int64(OutputSegmentBytes - 1000); offset < total; {
		read, err := ReadOutputLog(path, offset, 64<<10)
		if err != nil || read.Truncated || len(read.Data) == 0 || !bytes.Equal(read.Data, legacy[offset:read.NextOffset]) {
			t.Fatalf("legacy read at %d: next %d truncated %v err %v", offset, read.NextOffset, read.Truncated, err)
		}
		offset = read.NextOffset
	}
	if end, err := OutputLogEnd(path); err != nil || end != total {
		t.Fatalf("legacy end = %d, %v", end, err)
	}
	if tail, err := OutputLogTail(path, 1<<20); err != nil || !bytes.Equal(tail, legacy[total-1<<20:]) {
		t.Fatalf("legacy tail = %d bytes, %v", len(tail), err)
	}
}

func TestOutputLogReadsMissingAndOverrunLogsFromTheStart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "job.log")
	if read, err := ReadOutputLog(path, 42, 64<<10); err != nil || read.NextOffset != 0 || len(read.Data) != 0 {
		t.Fatalf("missing log read = %#v, %v", read, err)
	}
	if end, err := OutputLogEnd(path); err != nil || end != 0 {
		t.Fatalf("missing log end = %d, %v", end, err)
	}
	if err := os.WriteFile(path, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	if read, err := ReadOutputLog(path, 99, 64<<10); err != nil || string(read.Data) != "hello" || read.Truncated {
		t.Fatalf("overrun read = %#v, %v; want a restart at zero", read, err)
	}
}

func TestRemoveOutputLogDeletesEverySegmentAndNothingElse(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "job.log")
	writeOutputLog(t, path, DefaultOutputLogBytes, OutputSegmentBytes*3)
	unrelated := []string{"job.log.input", "job.log.tmp", "other.log.1", "job.log.01"}
	for _, name := range unrelated {
		if err := os.WriteFile(filepath.Join(directory, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := RemoveOutputLog(path); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != len(unrelated) {
		names := []string{}
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		t.Fatalf("remaining files = %v, want only %v", names, unrelated)
	}
}
