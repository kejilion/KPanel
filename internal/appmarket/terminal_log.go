package appmarket

import (
	"errors"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"
)

// Interactive task output is kept in fixed-size segments. A long-running
// full-screen program (a TUI redraws continuously) used to stop reaching the
// browser once a single capped log file filled up, while the script kept
// running. Rotating segments keeps the output flowing and bounds the retained
// history, and so disk use, to the same 8 MiB as before.
//
// Offsets stay absolute: segment k holds bytes [k*size, (k+1)*size). Segment 0
// keeps the legacy `<id>.log` name, so logs of jobs started by an older Agent
// and short interactive runs read exactly as before.
const (
	terminalLogSegmentBytes = 2 << 20
	terminalLogSegments     = 4
)

func (registry *appJobRegistry) terminalSegmentPath(id string, index int64) string {
	if index == 0 {
		return registry.logPath(id)
	}
	return registry.logPath(id) + "." + strconv.FormatInt(index, 10)
}

// terminalSegments lists the retained segment indexes in ascending order.
func (registry *appJobRegistry) terminalSegments(id string) ([]int64, error) {
	entries, err := os.ReadDir(registry.stateDir)
	if err != nil {
		return nil, err
	}
	base := id + ".log"
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

func (registry *appJobRegistry) removeTerminalSegments(id string) {
	segments, err := registry.terminalSegments(id)
	if err != nil {
		return
	}
	for _, index := range segments {
		if index > 0 {
			_ = os.Remove(registry.terminalSegmentPath(id, index))
		}
	}
}

type terminalLogWriter struct {
	registry *appJobRegistry
	id       string
	index    int64
	file     *os.File
	size     int64
}

func newTerminalLogWriter(registry *appJobRegistry, id string) (*terminalLogWriter, error) {
	registry.removeTerminalSegments(id)
	file, err := os.OpenFile(registry.terminalSegmentPath(id, 0), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return nil, err
	}
	return &terminalLogWriter{registry: registry, id: id, file: file}, nil
}

func (writer *terminalLogWriter) Write(data []byte) (int, error) {
	written := 0
	for len(data) > 0 {
		if writer.size == terminalLogSegmentBytes {
			if err := writer.rotate(); err != nil {
				return written, err
			}
		}
		room := min(int64(len(data)), terminalLogSegmentBytes-writer.size)
		count, err := writer.file.Write(data[:room])
		written += count
		writer.size += int64(count)
		if err != nil {
			return written, err
		}
		data = data[count:]
	}
	return written, nil
}

// rotate creates the next segment before removing the expired one, so a reader
// always finds a contiguous run of segments.
func (writer *terminalLogWriter) rotate() error {
	next := writer.index + 1
	file, err := os.OpenFile(writer.registry.terminalSegmentPath(writer.id, next), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	_ = writer.file.Close()
	writer.file, writer.index, writer.size = file, next, 0
	if expired := next - terminalLogSegments; expired >= 0 {
		_ = os.Remove(writer.registry.terminalSegmentPath(writer.id, expired))
	}
	return nil
}

func (writer *terminalLogWriter) Close() error {
	return errors.Join(writer.file.Sync(), writer.file.Close())
}

// readTerminalLog returns up to maxTerminalChunkBytes starting at an absolute
// offset. An offset whose segment was rotated away resumes at the oldest
// retained byte and reports the skipped output as truncated; an offset past
// the end of the log restarts at the oldest retained byte, as the single-file
// reader restarted at zero.
func (registry *appJobRegistry) readTerminalLog(id string, offset int64) ([]byte, int64, bool, error) {
	data, found, err := registry.readTerminalSegment(id, offset)
	if err != nil || found {
		return data, offset + int64(len(data)), false, err
	}
	segments, err := registry.terminalSegments(id)
	if err != nil || len(segments) == 0 {
		return nil, 0, false, err
	}
	restart := segments[0] * terminalLogSegmentBytes
	data, _, err = registry.readTerminalSegment(id, restart)
	return data, restart + int64(len(data)), offset < restart, err
}

// terminalLogLocation maps an absolute offset to the file holding it and the
// position inside that file. Jobs started by an Agent without segments wrote
// one `<id>.log` of up to 8 MiB; a segment 0 larger than a segment can only be
// such a log, and it is read as the single file it is.
func (registry *appJobRegistry) terminalLogLocation(id string, offset int64) (string, int64, bool) {
	if offset >= terminalLogSegmentBytes {
		if info, err := os.Stat(registry.logPath(id)); err == nil && info.Size() > terminalLogSegmentBytes {
			return registry.logPath(id), offset, true
		}
	}
	index := offset / terminalLogSegmentBytes
	return registry.terminalSegmentPath(id, index), offset % terminalLogSegmentBytes, false
}

// readTerminalSegment reads from the segment holding offset. found is false
// when that segment was rotated away or offset lies beyond the written log.
func (registry *appJobRegistry) readTerminalSegment(id string, offset int64) ([]byte, bool, error) {
	path, position, legacy := registry.terminalLogLocation(id, offset)
	index := offset / terminalLogSegmentBytes
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		// The end of a full segment whose successor has not been written yet
		// is the end of the log, not a missing segment.
		if !legacy && position == 0 && index > 0 {
			if info, statErr := os.Stat(registry.terminalSegmentPath(id, index-1)); statErr == nil &&
				info.Size() == terminalLogSegmentBytes {
				return nil, true, nil
			}
		}
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, false, err
	}
	if position > info.Size() {
		return nil, false, nil
	}
	if _, err := file.Seek(position, io.SeekStart); err != nil {
		return nil, false, err
	}
	data, err := io.ReadAll(io.LimitReader(file, maxTerminalChunkBytes))
	return data, true, err
}

// terminalLogEnd returns the absolute end offset of a log that is no longer
// written, so a finished job is reported finished only once fully read.
func (registry *appJobRegistry) terminalLogEnd(id string) (int64, error) {
	if info, err := os.Stat(registry.logPath(id)); err == nil && info.Size() < terminalLogSegmentBytes {
		return info.Size(), nil
	}
	segments, err := registry.terminalSegments(id)
	if err != nil || len(segments) == 0 {
		return 0, err
	}
	latest := segments[len(segments)-1]
	info, err := os.Stat(registry.terminalSegmentPath(id, latest))
	if err != nil {
		return 0, err
	}
	return latest*terminalLogSegmentBytes + info.Size(), nil
}

// terminalLogTail returns up to limit trailing bytes of an interactive log,
// reading across the latest segments.
func (registry *appJobRegistry) terminalLogTail(id string, limit int64) ([]byte, error) {
	end, err := registry.terminalLogEnd(id)
	if err != nil || end == 0 {
		return nil, err
	}
	start := max(end-limit, 0)
	result := make([]byte, 0, end-start)
	for offset := start; offset < end; {
		data, found, readErr := registry.readTerminalSegment(id, offset)
		if readErr != nil {
			return nil, readErr
		}
		if !found {
			// Rotated away since the end was measured: keep what follows.
			result = result[:0]
			offset = (offset/terminalLogSegmentBytes + 1) * terminalLogSegmentBytes
			continue
		}
		if len(data) == 0 {
			break
		}
		data = data[:min(int64(len(data)), end-offset)]
		result = append(result, data...)
		offset += int64(len(data))
	}
	return result, nil
}
