package hostpty

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// Task PTY output is kept as a rolling log of fixed-size segments. A single
// capped file stopped recording once full, so a long-running or full-screen
// program (a TUI redraws continuously) stopped reaching the browser while the
// script kept running. Rotating segments keeps the newest output flowing and
// bounds the retained history, and so disk use, to the configured budget.
//
// Offsets stay absolute: segment k holds bytes [k*size, (k+1)*size). Segment
// 0 is the log path itself and segment k is `<path>.<k>`, so logs written as
// one file by an older Agent and short runs read exactly as before.
const (
	OutputSegmentBytes = 2 << 20
	// DefaultOutputLogBytes is the retained history of one task log.
	DefaultOutputLogBytes = 8 << 20
)

// OutputRead is one read from a rolling output log.
type OutputRead struct {
	Data       []byte
	NextOffset int64
	// Truncated reports that the requested offset was rotated away and the
	// read skipped ahead to the oldest retained byte.
	Truncated bool
}

// OutputLog writes a rolling output log. It is not safe for concurrent use;
// each task has exactly one writer, the PTY copy loop.
type OutputLog struct {
	path     string
	segments int64
	index    int64
	file     *os.File
	size     int64
}

// CreateOutputLog starts an empty log at path that retains about maxBytes
// (at least one segment) and removes segments left by an earlier writer.
func CreateOutputLog(path string, maxBytes int64) (*OutputLog, error) {
	removeRotatedSegments(path)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return nil, err
	}
	return &OutputLog{path: path, segments: max(maxBytes/OutputSegmentBytes, 1), file: file}, nil
}

func (log *OutputLog) Write(data []byte) (int, error) {
	written := 0
	for len(data) > 0 {
		if log.size == OutputSegmentBytes {
			if err := log.rotate(); err != nil {
				return written, err
			}
		}
		room := min(int64(len(data)), OutputSegmentBytes-log.size)
		count, err := log.file.Write(data[:room])
		written += count
		log.size += int64(count)
		if err != nil {
			return written, err
		}
		data = data[count:]
	}
	return written, nil
}

// rotate creates the next segment before removing the expired one, so a
// reader always finds a contiguous run of segments.
func (log *OutputLog) rotate() error {
	next := log.index + 1
	file, err := os.OpenFile(segmentPath(log.path, next), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	_ = log.file.Close()
	log.file, log.index, log.size = file, next, 0
	if expired := next - log.segments; expired >= 0 {
		// Best effort: a failed remove only delays reclaiming that segment.
		_ = os.Remove(segmentPath(log.path, expired))
	}
	return nil
}

// Sync flushes the current segment to disk.
func (log *OutputLog) Sync() error {
	return log.file.Sync()
}

// Close flushes the current segment to disk and closes it.
func (log *OutputLog) Close() error {
	return errors.Join(log.file.Sync(), log.file.Close())
}

// ReadOutputLog returns up to limit bytes starting at an absolute offset. An
// offset whose segment was rotated away resumes at the oldest retained byte
// and reports truncated; an offset past the end of the log restarts at the
// oldest retained byte, as the single-file readers restarted at zero. A log
// that does not exist yet reads as empty at offset zero.
func ReadOutputLog(path string, offset, limit int64) (OutputRead, error) {
	if offset < 0 || limit <= 0 {
		return OutputRead{}, errors.New("invalid output log read")
	}
	data, found, err := readSegment(path, offset, limit)
	if err != nil || found {
		return OutputRead{Data: data, NextOffset: offset + int64(len(data))}, err
	}
	segments, err := outputSegments(path)
	if err != nil || len(segments) == 0 {
		return OutputRead{}, err
	}
	restart := segments[0] * OutputSegmentBytes
	data, _, err = readSegment(path, restart, limit)
	return OutputRead{Data: data, NextOffset: restart + int64(len(data)), Truncated: offset < restart}, err
}

// OutputLogEnd returns the absolute end offset of the log, zero when absent.
func OutputLogEnd(path string) (int64, error) {
	if info, err := os.Stat(path); err == nil && info.Size() < OutputSegmentBytes {
		return info.Size(), nil
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return 0, err
	}
	segments, err := outputSegments(path)
	if err != nil || len(segments) == 0 {
		return 0, err
	}
	latest := segments[len(segments)-1]
	info, err := os.Stat(segmentPath(path, latest))
	if err != nil {
		return 0, err
	}
	return latest*OutputSegmentBytes + info.Size(), nil
}

// OutputLogTail returns up to limit trailing bytes of the retained log.
func OutputLogTail(path string, limit int64) ([]byte, error) {
	end, err := OutputLogEnd(path)
	if err != nil || end == 0 || limit <= 0 {
		return nil, err
	}
	start := max(end-limit, 0)
	result := make([]byte, 0, end-start)
	for offset := start; offset < end; {
		data, found, readErr := readSegment(path, offset, end-offset)
		if readErr != nil {
			return nil, readErr
		}
		if !found {
			// Rotated away since the end was measured: keep what follows.
			result = result[:0]
			offset = (offset/OutputSegmentBytes + 1) * OutputSegmentBytes
			continue
		}
		if len(data) == 0 {
			break
		}
		result = append(result, data...)
		offset += int64(len(data))
	}
	return result, nil
}

// RemoveOutputLog deletes the log and all of its rotated segments.
func RemoveOutputLog(path string) error {
	removeRotatedSegments(path)
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func segmentPath(path string, index int64) string {
	if index == 0 {
		return path
	}
	return path + "." + strconv.FormatInt(index, 10)
}

// outputSegments lists the retained segment indexes in ascending order.
func outputSegments(path string) ([]int64, error) {
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	base := filepath.Base(path)
	indexes := []int64{}
	for _, entry := range entries {
		if !entry.Type().IsRegular() {
			continue
		}
		name := entry.Name()
		if name == base {
			indexes = append(indexes, 0)
			continue
		}
		suffix, found := strings.CutPrefix(name, base+".")
		if !found {
			continue
		}
		index, parseErr := strconv.ParseInt(suffix, 10, 64)
		if parseErr != nil || index <= 0 || strconv.FormatInt(index, 10) != suffix {
			continue
		}
		indexes = append(indexes, index)
	}
	slices.Sort(indexes)
	return indexes, nil
}

func removeRotatedSegments(path string) {
	segments, err := outputSegments(path)
	if err != nil {
		return
	}
	for _, index := range segments {
		if index > 0 {
			_ = os.Remove(segmentPath(path, index))
		}
	}
}

// segmentLocation maps an absolute offset to the file holding it and the
// position inside that file. Older Agents wrote one file per task (up to
// 32 MiB); a segment 0 larger than a segment can only be such a log, and it
// is read as the single file it is.
func segmentLocation(path string, offset int64) (string, int64, bool) {
	if offset >= OutputSegmentBytes {
		if info, err := os.Stat(path); err == nil && info.Size() > OutputSegmentBytes {
			return path, offset, true
		}
	}
	return segmentPath(path, offset/OutputSegmentBytes), offset % OutputSegmentBytes, false
}

// readSegment reads from the file holding offset. found is false when that
// segment was rotated away or offset lies beyond the written log.
func readSegment(path string, offset, limit int64) ([]byte, bool, error) {
	file, position, legacy := segmentLocation(path, offset)
	index := offset / OutputSegmentBytes
	handle, err := os.Open(file)
	if errors.Is(err, os.ErrNotExist) {
		// The end of a full segment whose successor has not been written yet
		// is the end of the log, not a missing segment.
		if !legacy && position == 0 && index > 0 {
			if info, statErr := os.Stat(segmentPath(path, index-1)); statErr == nil &&
				info.Size() == OutputSegmentBytes {
				return nil, true, nil
			}
		}
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	defer handle.Close()
	info, err := handle.Stat()
	if err != nil {
		return nil, false, err
	}
	if position > info.Size() {
		return nil, false, nil
	}
	if _, err := handle.Seek(position, io.SeekStart); err != nil {
		return nil, false, err
	}
	data, err := io.ReadAll(io.LimitReader(handle, limit))
	return data, true, err
}
