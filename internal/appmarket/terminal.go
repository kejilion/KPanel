package appmarket

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	maxTerminalInputBytes = 16 << 10
	maxTerminalChunkBytes = 64 << 10
	maxTerminalLogBytes   = 8 << 20
)

type TerminalChunk struct {
	DataBase64 string `json:"dataBase64"`
	NextOffset int64  `json:"nextOffset"`
	InputOpen  bool   `json:"inputOpen"`
	Finished   bool   `json:"finished"`
}

func RunInteractiveAppJob(ctx context.Context, stateDir, id string) error {
	if os.Geteuid() != 0 {
		return errors.New("app-pty-run requires root")
	}
	if !appJobIDPattern.MatchString(id) {
		return errors.New("invalid application job identity")
	}
	cleanStateDir := filepath.Clean(strings.TrimSpace(stateDir))
	if !filepath.IsAbs(cleanStateDir) || cleanStateDir == string(filepath.Separator) {
		return errors.New("application jobs require a dedicated absolute state path")
	}
	registry := &appJobRegistry{
		stateDir: cleanStateDir,
		jobs:     make(map[string]appJobRecord),
	}
	defer os.Remove(registry.cancelPath(id))
	if err := ensureAppJobDirectory(registry.stateDir); err != nil {
		return err
	}
	record, err := registry.read(id)
	if err != nil {
		return err
	}
	if record.Adapter != "kejilion" || !record.Interactive ||
		!supportedScriptJobAction(record.Action) ||
		!appSelectorPattern.MatchString(record.Selector) {
		return errors.New("application job contains an unsupported interactive request")
	}
	if record.Action != "install" && record.Action != "manage" &&
		!containerIDPattern.MatchString(record.ExpectedContainerID) {
		return errors.New("application job contains an invalid expected container")
	}
	if record.Action == "manage" && record.ExpectedContainerID != "" {
		return errors.New("application recovery job unexpectedly targets a container")
	}
	if record.Action == "direct_access" &&
		record.AccessMode != "direct" && record.AccessMode != "domain_only" {
		return errors.New("application job contains an invalid access policy")
	}
	scriptCompatible := isKPanelInteractiveCompatibleScript
	if record.Action == "manage" {
		scriptCompatible = isKPanelInteractiveManageCompatibleScript
	}
	script, err := findKejilionScriptMatching(
		appScriptCompatible(scriptCompatible, record.AppID, record.Selector),
	)
	if err != nil {
		return registry.fail(record, "script_unavailable", err)
	}
	input, err := openTerminalInput(registry.inputPath(id))
	if err != nil {
		return registry.fail(record, "terminal_unavailable", err)
	}
	defer func() {
		_ = input.Close()
		_ = removeTerminalInput(registry.inputPath(id))
	}()

	command := exec.CommandContext(ctx, "/bin/bash", script, "app", record.Selector)
	command.Env = append(os.Environ(), interactiveAppJobEnvironment(record)...)
	terminal, err := startTerminalProcess(command, 36, 120)
	if err != nil {
		return registry.fail(record, "terminal_unavailable", err)
	}
	defer terminal.Close()

	started := time.Now().UTC()
	record.Status = "running"
	record.Stage = "interactive"
	record.Progress = 5
	record.Message = "kejilion.sh 交互终端已就绪，请按脚本提示输入"
	record.StartedAt = &started
	record.FinishedAt = nil
	record.InputOpen = true
	if err := registry.put(record); err != nil {
		_ = terminal.Kill()
		return err
	}

	inputDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(terminal, input)
		close(inputDone)
	}()

	logFile, err := os.OpenFile(
		registry.logPath(id),
		os.O_CREATE|os.O_WRONLY|os.O_TRUNC,
		0o600,
	)
	if err != nil {
		_ = terminal.Kill()
		return registry.fail(record, "log_unavailable", err)
	}
	readErr := copyTerminalOutput(registry, &record, logFile, terminal)
	if readErr != nil && !isTerminalEnd(readErr) {
		_ = terminal.Kill()
	}
	_ = logFile.Sync()
	_ = logFile.Close()
	waitErr := terminal.Wait()
	_ = input.Close()
	select {
	case <-inputDone:
	case <-time.After(250 * time.Millisecond):
	}

	finished := time.Now().UTC()
	record.InputOpen = false
	record.Progress = 100
	record.FinishedAt = &finished
	if registry.cancelRequested(id) {
		record.Status = "cancelled"
		record.Stage = "cancelled"
		record.Message = "交互任务已由管理员手动结束，应用状态将按宿主机实际产物重新读取"
		return registry.put(record)
	}
	if readErr != nil && !isTerminalEnd(readErr) && waitErr == nil {
		waitErr = readErr
	}
	if waitErr != nil {
		record.Status = "failed"
		record.Stage = "failed"
		record.Message = appActionLabel(record.Action) +
			"失败，请查看交互终端记录后修复并重试"
		_ = registry.put(record)
		return waitErr
	}
	record.Status = "succeeded"
	record.Stage = "completed"
	record.Message = "应用" + appActionLabel(record.Action) +
		"交互流程已结束，面板将按实际产物刷新状态"
	return registry.put(record)
}

func interactiveAppJobEnvironment(record appJobRecord) []string {
	result := []string{
		"KJ_APP_INTERACTIVE=1",
		"KJ_APP_ACTION=" + record.Action,
		"LC_ALL=C.UTF-8",
		"LANG=C.UTF-8",
		"TERM=xterm-256color",
	}
	if record.AccessMode != "" {
		result = append(result, "KJ_APP_ACCESS_MODE="+record.AccessMode)
	}
	if record.Action == "install" && record.HostPort > 0 {
		result = append(result, "KJ_APP_PORT="+strconv.Itoa(int(record.HostPort)))
	}
	if record.Action == "manage" {
		result = append(result, "KJ_APP_MARKER_RECOVERY=1")
	}
	if record.Action == "update" || record.Action == "uninstall" ||
		record.Action == "direct_access" {
		result = append(result, "KJ_APP_RECONCILE_MARKER=1")
	}
	if record.ExpectedContainerID != "" {
		result = append(
			result,
			"KJ_APP_EXPECTED_CONTAINER_ID="+record.ExpectedContainerID,
		)
	}
	return result
}

func copyTerminalOutput(
	registry *appJobRegistry,
	record *appJobRecord,
	logFile *os.File,
	terminal terminalProcess,
) error {
	buffer := make([]byte, 4096)
	pending := ""
	written := int64(0)
	for {
		count, err := terminal.Read(buffer)
		if count > 0 {
			chunk := buffer[:count]
			if written < maxTerminalLogBytes {
				remaining := int64(maxTerminalLogBytes) - written
				if int64(len(chunk)) > remaining {
					chunk = chunk[:remaining]
				}
				size, writeErr := logFile.Write(chunk)
				written += int64(size)
				if writeErr != nil {
					return writeErr
				}
			}
			pending += stripTerminalControls(string(buffer[:count]))
			lines := strings.FieldsFunc(pending, func(value rune) bool {
				return value == '\n' || value == '\r'
			})
			if strings.HasSuffix(pending, "\n") || strings.HasSuffix(pending, "\r") {
				pending = ""
			} else if len(lines) > 0 {
				pending = lines[len(lines)-1]
				lines = lines[:len(lines)-1]
			}
			for _, line := range lines {
				updateInteractiveProgress(registry, record, strings.TrimSpace(line))
			}
			if len(pending) > 4096 {
				pending = pending[len(pending)-4096:]
			}
		}
		if err != nil {
			return err
		}
	}
}

func updateInteractiveProgress(
	registry *appJobRegistry,
	record *appJobRecord,
	line string,
) {
	matches := appProgressPattern.FindStringSubmatch(line)
	if len(matches) != 3 {
		return
	}
	progress, _ := strconv.Atoi(matches[1])
	if progress < 0 {
		progress = 0
	}
	if progress > 100 {
		progress = 100
	}
	record.Progress = progress
	record.Stage = appJobStage(progress)
	record.Message = safeAppJobMessage(errors.New(matches[2]))
	_ = registry.put(*record)
}

func stripTerminalControls(value string) string {
	var result strings.Builder
	result.Grow(len(value))
	escape := false
	csi := false
	for _, character := range value {
		if escape {
			if character == '[' {
				csi = true
				escape = false
				continue
			}
			escape = false
			continue
		}
		if csi {
			if character >= 0x40 && character <= 0x7e {
				csi = false
			}
			continue
		}
		if character == 0x1b {
			escape = true
			continue
		}
		if character == '\t' || character == '\n' || character == '\r' ||
			character >= 0x20 {
			result.WriteRune(character)
		}
	}
	return result.String()
}

func (s *Service) WriteAppJobInput(id, value string) error {
	if s.jobs == nil || !appJobIDPattern.MatchString(id) {
		return ErrNotFound
	}
	data := []byte(value)
	if len(data) == 0 || len(data) > maxTerminalInputBytes ||
		bytes.IndexByte(data, 0) >= 0 {
		return fmt.Errorf("%w: interactive input is invalid", ErrForbidden)
	}
	record, err := s.jobs.read(id)
	if err != nil {
		return ErrNotFound
	}
	if !record.Interactive || !record.InputOpen ||
		(record.Status != "queued" && record.Status != "running") ||
		s.jobs.cancelRequested(id) {
		return fmt.Errorf("%w: interactive input is not open", ErrConflict)
	}
	if err := writeTerminalInput(s.jobs.inputPath(id), data); err != nil {
		return fmt.Errorf("%w: interactive input is unavailable: %v", ErrConflict, err)
	}
	return nil
}

func (s *Service) AppJobTerminal(id string, offset int64) (TerminalChunk, error) {
	if s.jobs == nil || !appJobIDPattern.MatchString(id) || offset < 0 {
		return TerminalChunk{}, ErrNotFound
	}
	record, err := s.jobs.read(id)
	if err != nil || !record.Interactive {
		return TerminalChunk{}, ErrNotFound
	}
	path := s.jobs.logPath(id)
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return TerminalChunk{
			NextOffset: 0,
			InputOpen:  record.InputOpen,
			Finished: record.Status == "succeeded" || record.Status == "failed" ||
				record.Status == "cancelled",
		}, nil
	}
	if err != nil {
		return TerminalChunk{}, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return TerminalChunk{}, err
	}
	if offset > info.Size() {
		offset = 0
	}
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return TerminalChunk{}, err
	}
	data, err := io.ReadAll(io.LimitReader(file, maxTerminalChunkBytes))
	if err != nil {
		return TerminalChunk{}, err
	}
	nextOffset := offset + int64(len(data))
	return TerminalChunk{
		DataBase64: base64.StdEncoding.EncodeToString(data),
		NextOffset: nextOffset,
		InputOpen:  record.InputOpen,
		Finished: (record.Status == "succeeded" || record.Status == "failed" ||
			record.Status == "cancelled") &&
			nextOffset >= info.Size(),
	}, nil
}

func (registry *appJobRegistry) inputPath(id string) string {
	return registry.statePath(id) + ".input"
}
