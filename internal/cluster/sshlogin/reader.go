package sshlogin

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

const (
	// EventPath is the fixed, credential-free relay shared by the privileged
	// lightweight broker and its unprivileged telemetry process.
	EventPath           = "/run/kejilion-node-ssh/ssh-login.json"
	cacheTTL            = 15 * time.Second
	journalMaxBytes     = int64(256 << 10)
	logFileTailBytes    = int64(1 << 20)
	eventFileMaxBytes   = int64(4 << 10)
	journalLines        = 50
	journalOutputFields = "__CURSOR,__REALTIME_TIMESTAMP,MESSAGE,_SYSTEMD_UNIT,UNIT,OBJECT_SYSTEMD_UNIT,COREDUMP_UNIT,SYSLOG_IDENTIFIER,_COMM"
)

var (
	acceptedPattern          = regexp.MustCompile(`(?i)\bAccepted\s+([^\s]+)\s+for\s+([^\s]+)\s+from\s+([^\s]+)`)
	dropbearPasswordPattern  = regexp.MustCompile(`^Password auth succeeded for '([^']+)' from (\S+)$`)
	dropbearPublicKeyPattern = regexp.MustCompile(`^Pubkey auth succeeded for '([^']+)' with [A-Za-z0-9@._+-]+ key [A-Za-z0-9:/+=._-]+ from (\S+)$`)
	dropbearSyslogPattern    = regexp.MustCompile(`^(?:[A-Z][a-z]{2}\s+[0-9]{1,2} [0-9]{2}:[0-9]{2}:[0-9]{2} \S+ )?dropbear\[[0-9]+\]: (.+)$`)
	logreadPattern           = regexp.MustCompile(`^[A-Z][a-z]{2} [A-Z][a-z]{2}\s+[0-9]{1,2} [0-9]{2}:[0-9]{2}:[0-9]{2} [0-9]{4} \[([0-9]{1,11})\.([0-9]{3})\] auth(?:priv)?\.[a-z]+ (sshd|dropbear)(?:\[[0-9]+\])?: (.+)$`)
	loginToken               = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._@+:/-]{0,127}$`)
	loginAddress             = regexp.MustCompile(`^[A-Za-z0-9:][A-Za-z0-9.:%_-]{0,252}$`)
)

var (
	ErrUnavailable = errors.New("SSH login events are unavailable")
	ErrInvalid     = errors.New("SSH login event is invalid")
)

// Runner is intentionally small so journal fallback and parser behavior can
// be tested without making a host command during unit tests.
type Runner interface {
	Run(context.Context, string, ...string) ([]byte, error)
	LookPath(string) (string, error)
}

type commandRunner struct{}

func (commandRunner) Run(ctx context.Context, name string, arguments ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, name, arguments...)
	output := &boundedLogOutput{cancel: cancel}
	command.Stdout = output
	command.Stderr = io.Discard
	command.WaitDelay = 100 * time.Millisecond
	err := command.Run()
	if output.overflow {
		return nil, fmt.Errorf("%w: log output exceeds limit", ErrUnavailable)
	}
	return output.buffer.Bytes(), err
}

func (commandRunner) LookPath(name string) (string, error) {
	if name == "logread" {
		// Never execute a root log collector found in an inherited PATH.
		for _, candidate := range []string{"/sbin/logread", "/usr/sbin/logread", "/bin/logread", "/usr/bin/logread"} {
			if trustedLogExecutable(candidate) {
				return candidate, nil
			}
		}
		return "", exec.ErrNotFound
	}
	return exec.LookPath(name)
}

type boundedLogOutput struct {
	buffer   bytes.Buffer
	cancel   context.CancelFunc
	overflow bool
}

func (output *boundedLogOutput) Write(value []byte) (int, error) {
	if int64(output.buffer.Len())+int64(len(value)) > journalMaxBytes {
		output.overflow = true
		output.cancel()
		return 0, errors.New("log output exceeds limit")
	}
	return output.buffer.Write(value)
}

// Config controls the source used by a telemetry process. EventPath selects
// the fixed relay; an empty EventPath selects the root-readable host logs.
type Config struct {
	EventPath    string
	LogRoot      string
	RunRoot      string
	Now          func() time.Time
	Runner       Runner
	EffectiveUID func() int
}

type Reader struct {
	eventPath    string
	logRoot      string
	runRoot      string
	now          func() time.Time
	runner       Runner
	effectiveUID func() int

	mu        sync.Mutex
	cache     *contract.SSHLoginEvent
	checkedAt time.Time
}

func NewReader(config Config) *Reader {
	if config.LogRoot == "" {
		config.LogRoot = "/var/log"
	}
	if config.RunRoot == "" {
		config.RunRoot = "/run"
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	if config.Runner == nil {
		config.Runner = commandRunner{}
	}
	if config.EffectiveUID == nil {
		config.EffectiveUID = os.Geteuid
	}
	eventPath := strings.TrimSpace(config.EventPath)
	if eventPath != "" {
		eventPath = filepath.Clean(eventPath)
	}
	return &Reader{
		eventPath: eventPath, logRoot: filepath.Clean(config.LogRoot), runRoot: filepath.Clean(config.RunRoot),
		now: config.Now, runner: config.Runner, effectiveUID: config.EffectiveUID,
	}
}

// LatestSSHLogin returns only the newest accepted SSH authentication event.
// It never returns the original log line, credentials, commands or host
// configuration.
func (r *Reader) LatestSSHLogin(ctx context.Context) (*contract.SSHLoginEvent, error) {
	if r == nil {
		return nil, fmt.Errorf("%w: reader is nil", ErrUnavailable)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now().UTC()
	if !r.checkedAt.IsZero() && now.Sub(r.checkedAt) >= 0 && now.Sub(r.checkedAt) < cacheTTL {
		return cloneEvent(r.cache), nil
	}

	var (
		event *contract.SSHLoginEvent
		err   error
	)
	if r.eventPath != "" {
		if runtime.GOOS != "linux" {
			return nil, fmt.Errorf("%w: relay is Linux-only", ErrUnavailable)
		}
		event, err = readEventFile(r.eventPath)
	} else if runtime.GOOS != "linux" || r.effectiveUID() != 0 {
		return nil, fmt.Errorf("%w: root host log access is required", ErrUnavailable)
	} else {
		event, err = r.latestFromLogs(ctx, now)
	}
	if err != nil {
		return nil, err
	}
	r.cache, r.checkedAt = event, now
	return cloneEvent(r.cache), nil
}

func (r *Reader) latestFromLogs(ctx context.Context, observedAt time.Time) (*contract.SSHLoginEvent, error) {
	if r.openRCRuntimeActive() {
		entries, err := r.readFixedAuthLog()
		if err != nil {
			return nil, err
		}
		return latest(entries, observedAt), nil
	}
	entries, journalErr := r.readJournal(ctx)
	if journalErr == nil {
		if event := latest(entries, observedAt); event != nil {
			return event, nil
		}
	} else if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, ctxErr
	}

	entries, logreadErr := r.readLogread(ctx)
	if logreadErr == nil {
		if event := latest(entries, observedAt); event != nil {
			return event, nil
		}
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	entries, fileErr := r.readFixedAuthLog()
	if fileErr == nil {
		return latest(entries, observedAt), nil
	}
	if journalErr == nil || logreadErr == nil {
		return nil, nil
	}
	return nil, errors.Join(journalErr, fileErr, logreadErr)
}

func (r *Reader) readLogread(ctx context.Context) ([]logEntry, error) {
	command, err := r.runner.LookPath("logread")
	if err != nil {
		return nil, fmt.Errorf("%w: trusted logread is unavailable", ErrUnavailable)
	}
	// ubox logread: one bounded snapshot of authentication facilities, with
	// epoch timestamps. No follow, output file, socket or network arguments.
	output, err := r.runner.Run(ctx, command, "-l", strconv.Itoa(journalLines), "-z", "4", "-z", "10", "-t")
	if err != nil {
		return nil, fmt.Errorf("%w: read logd: %w", ErrUnavailable, err)
	}
	if int64(len(output)) > journalMaxBytes {
		return nil, fmt.Errorf("%w: logread output exceeds limit", ErrUnavailable)
	}
	entries := make([]logEntry, 0, journalLines)
	lines := strings.Split(strings.TrimSuffix(string(output), "\n"), "\n")
	if len(lines) > journalLines {
		return nil, fmt.Errorf("%w: logread returned too many lines", ErrUnavailable)
	}
	for _, line := range lines {
		match := logreadPattern.FindStringSubmatch(line)
		if len(match) != 5 {
			continue
		}
		seconds, secondsErr := strconv.ParseInt(match[1], 10, 64)
		milliseconds, millisErr := strconv.ParseInt(match[2], 10, 64)
		if secondsErr != nil || millisErr != nil {
			continue
		}
		timestamp := time.Unix(seconds, milliseconds*int64(time.Millisecond)).UTC()
		// The human ctime prefix follows the host timezone; it must not make
		// the same logd event look new after a timezone change.
		hash := sha256.Sum256([]byte("logd\x00" + strings.Join(match[1:], "\x00")))
		entries = append(entries, logEntry{cursor: "sha256:" + hex.EncodeToString(hash[:]), identifier: match[3], message: match[4], timestamp: &timestamp})
	}
	return entries, nil
}

func (r *Reader) openRCRuntimeActive() bool {
	info, err := os.Stat(filepath.Join(r.runRoot, "openrc"))
	return err == nil && info.IsDir()
}

type logEntry struct {
	cursor     string
	identifier string
	unit       string
	message    string
	timestamp  *time.Time
}

func (r *Reader) readJournal(ctx context.Context) ([]logEntry, error) {
	if _, err := r.runner.LookPath("journalctl"); err != nil {
		return nil, fmt.Errorf("%w: journalctl is unavailable", ErrUnavailable)
	}
	output, err := r.runner.Run(ctx, "journalctl",
		"--no-pager", "--quiet", "--reverse", "--lines="+strconv.Itoa(journalLines+1),
		"--output=json", "--output-fields="+journalOutputFields,
		"SYSLOG_FACILITY=4", "SYSLOG_FACILITY=10",
	)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, fmt.Errorf("%w: read journal: %v", ErrUnavailable, err)
	}
	if int64(len(output)) > journalMaxBytes {
		return nil, fmt.Errorf("%w: journal output is too large", ErrUnavailable)
	}
	entries, invalid := parseJournalEntries(output)
	if invalid && len(entries) == 0 && len(bytes.TrimSpace(output)) > 0 {
		return nil, fmt.Errorf("%w: journalctl returned invalid JSON", ErrUnavailable)
	}
	reverseEntries(entries)
	if len(entries) > journalLines {
		entries = entries[len(entries)-journalLines:]
	}
	return entries, nil
}

func parseJournalEntries(output []byte) ([]logEntry, bool) {
	entries := make([]logEntry, 0, journalLines+1)
	invalid := false
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal([]byte(line), &fields); err != nil {
			invalid = true
			continue
		}
		entry := logEntry{
			cursor:     journalString(fields["__CURSOR"]),
			identifier: journalString(fields["SYSLOG_IDENTIFIER"]),
			unit:       journalString(fields["_SYSTEMD_UNIT"]),
			message:    journalString(fields["MESSAGE"]),
		}
		if entry.unit == "" {
			for _, field := range []string{"UNIT", "OBJECT_SYSTEMD_UNIT", "COREDUMP_UNIT"} {
				if entry.unit = journalString(fields[field]); entry.unit != "" {
					break
				}
			}
		}
		if entry.identifier == "" {
			entry.identifier = journalString(fields["_COMM"])
		}
		if micros, err := strconv.ParseInt(journalString(fields["__REALTIME_TIMESTAMP"]), 10, 64); err == nil && micros >= 0 {
			timestamp := time.UnixMicro(micros).UTC()
			entry.timestamp = &timestamp
		}
		entries = append(entries, entry)
	}
	return entries, invalid
}

func journalString(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var value string
	if json.Unmarshal(raw, &value) == nil {
		return value
	}
	var values []string
	if json.Unmarshal(raw, &values) == nil && len(values) > 0 {
		return values[0]
	}
	return ""
}

func (r *Reader) readFixedAuthLog() ([]logEntry, error) {
	var path string
	for _, name := range []string{"secure", "auth.log", "messages"} {
		candidate := filepath.Join(r.logRoot, name)
		info, err := os.Lstat(candidate)
		if err == nil && info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
			path = candidate
			break
		}
	}
	if path == "" {
		return nil, os.ErrNotExist
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() {
		return nil, os.ErrInvalid
	}
	current, err := os.Lstat(path)
	if err != nil || !os.SameFile(opened, current) || current.Mode()&os.ModeSymlink != 0 {
		return nil, os.ErrInvalid
	}
	start := int64(0)
	if opened.Size() > logFileTailBytes {
		start = opened.Size() - logFileTailBytes
	}
	if _, err := file.Seek(start, io.SeekStart); err != nil {
		return nil, err
	}
	content, err := io.ReadAll(io.LimitReader(file, logFileTailBytes))
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(content), "\n")
	if start > 0 && len(lines) > 0 {
		lines = lines[1:]
	}
	entries := make([]logEntry, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if filepath.Base(path) == "messages" && !strings.Contains(strings.ToLower(line), "sshd") &&
			!strings.Contains(strings.ToLower(line), "dropbear") {
			continue
		}
		hash := sha256.Sum256([]byte(path + "\x00" + line))
		entries = append(entries, logEntry{
			cursor:     "sha256:" + hex.EncodeToString(hash[:]),
			identifier: filepath.Base(path),
			message:    line,
		})
	}
	if len(entries) > journalLines {
		entries = entries[len(entries)-journalLines:]
	}
	return entries, nil
}

func latest(entries []logEntry, observedAt time.Time) *contract.SSHLoginEvent {
	for index := len(entries) - 1; index >= 0; index-- {
		if event, ok := parseSSHLoginEntry(entries[index], observedAt); ok {
			return &event
		}
	}
	return nil
}

func parseSSHLoginEntry(entry logEntry, observedAt time.Time) (contract.SSHLoginEvent, bool) {
	if strings.ContainsAny(entry.message, "\r\n") {
		return contract.SSHLoginEvent{}, false
	}
	var match []string
	fixedLog := entry.identifier == "secure" || entry.identifier == "auth.log" || entry.identifier == "messages"
	if entry.identifier == "dropbear" || (fixedLog && dropbearSyslogPattern.MatchString(entry.message)) {
		match = dropbearLoginFields(entry)
		if match == nil {
			return contract.SSHLoginEvent{}, false
		}
	} else {
		identity := strings.ToLower(entry.identifier + " " + entry.unit)
		if !strings.Contains(identity, "sshd") && !strings.Contains(strings.ToLower(entry.message), "sshd") {
			return contract.SSHLoginEvent{}, false
		}
		match = acceptedPattern.FindStringSubmatch(entry.message)
	}
	if len(match) != 4 || !loginToken.MatchString(match[1]) ||
		!loginToken.MatchString(match[2]) || !loginAddress.MatchString(match[3]) {
		return contract.SSHLoginEvent{}, false
	}
	occurredAt := observedAt.UTC()
	if entry.timestamp != nil {
		occurredAt = entry.timestamp.UTC()
	}
	id := strings.TrimSpace(entry.cursor)
	if id == "" {
		hash := sha256.Sum256([]byte(strings.Join([]string{
			occurredAt.Format(time.RFC3339Nano), match[1], match[2], match[3], entry.message,
		}, "\x00")))
		id = "sha256:" + hex.EncodeToString(hash[:])
	}
	event := contract.SSHLoginEvent{
		ID: id, OccurredAt: occurredAt, Method: match[1],
		Username: match[2], RemoteAddress: match[3],
	}
	return event, contract.ValidSSHLoginEvent(event)
}

func dropbearLoginFields(entry logEntry) []string {
	message := entry.message
	if entry.identifier != "dropbear" {
		match := dropbearSyslogPattern.FindStringSubmatch(message)
		if len(match) != 2 {
			return nil
		}
		message = match[1]
	}
	method := "password"
	match := dropbearPasswordPattern.FindStringSubmatch(message)
	if len(match) != 3 {
		method = "publickey"
		match = dropbearPublicKeyPattern.FindStringSubmatch(message)
	}
	if len(match) != 3 {
		return nil
	}
	// Dropbear writes a numeric address followed by its source port. IPv6
	// versions may omit brackets, so split only the final colon.
	endpoint := match[2]
	separator := strings.LastIndexByte(endpoint, ':')
	if separator < 1 {
		return nil
	}
	port, err := strconv.ParseUint(endpoint[separator+1:], 10, 16)
	address := endpoint[:separator]
	if strings.HasPrefix(address, "[") && strings.HasSuffix(address, "]") {
		address = address[1 : len(address)-1]
	}
	if err != nil || port == 0 {
		return nil
	}
	if _, err := netip.ParseAddr(address); err != nil {
		return nil
	}
	return []string{message, method, match[1], address}
}

func cloneEvent(event *contract.SSHLoginEvent) *contract.SSHLoginEvent {
	if event == nil {
		return nil
	}
	clone := *event
	return &clone
}

func readEventFile(path string) (*contract.SSHLoginEvent, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) == string(filepath.Separator) {
		return nil, fmt.Errorf("%w: relay path is invalid", ErrInvalid)
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !trustedEventFile(info) || info.Size() < 0 || info.Size() > eventFileMaxBytes {
		return nil, fmt.Errorf("%w: relay file is not trusted", ErrInvalid)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) || !trustedEventFile(opened) || opened.Size() > eventFileMaxBytes {
		return nil, fmt.Errorf("%w: relay file changed while reading", ErrInvalid)
	}
	content, err := io.ReadAll(io.LimitReader(file, eventFileMaxBytes+1))
	if err != nil || int64(len(content)) > eventFileMaxBytes {
		return nil, fmt.Errorf("%w: relay file is invalid", ErrInvalid)
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	var event contract.SSHLoginEvent
	if err := decoder.Decode(&event); err != nil || !contract.ValidSSHLoginEvent(event) {
		return nil, fmt.Errorf("%w: relay file is invalid", ErrInvalid)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%w: relay file has trailing data", ErrInvalid)
	}
	return &event, nil
}

func trustedEventFile(info os.FileInfo) bool {
	return info != nil && info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 && trustedMode(info.Mode())
}

func trustedMode(mode os.FileMode) bool {
	return runtime.GOOS == "windows" || mode.Perm()&0o022 == 0
}

// WriteSSHLoginEvent atomically publishes one validated, credential-free
// event. The caller is expected to be the root-only broker service; the
// service unit supplies the group-readable runtime directory.
func WriteSSHLoginEvent(path string, event contract.SSHLoginEvent) error {
	if !contract.ValidSSHLoginEvent(event) {
		return fmt.Errorf("%w: event fields are invalid", ErrInvalid)
	}
	if !filepath.IsAbs(path) || filepath.Clean(path) == string(filepath.Separator) {
		return fmt.Errorf("%w: relay path is invalid", ErrInvalid)
	}
	content, err := json.Marshal(event)
	if err != nil {
		return err
	}
	content = append(content, '\n')
	if int64(len(content)) > eventFileMaxBytes {
		return fmt.Errorf("%w: relay event is too large", ErrInvalid)
	}
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o750); err != nil {
		return err
	}
	if info, err := os.Lstat(directory); err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || !trustedMode(info.Mode()) {
		return fmt.Errorf("%w: relay directory is not trusted", ErrInvalid)
	}
	temporary, err := os.CreateTemp(directory, ".ssh-login-*")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o640); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(content); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryName, path); err != nil {
		return err
	}
	return nil
}

func reverseEntries(entries []logEntry) {
	for left, right := 0, len(entries)-1; left < right; left, right = left+1, right-1 {
		entries[left], entries[right] = entries[right], entries[left]
	}
}
