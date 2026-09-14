package selfupdate

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type CommandExecutor struct {
	StateDir      string
	ScriptPath    string
	AgentPath     string
	LifecyclePath string
	Stdout        io.Writer
	Stderr        io.Writer
}

func (e CommandExecutor) Validate() error {
	for name, value := range map[string]string{
		"state directory": e.StateDir, "script": e.ScriptPath,
		"Agent": e.AgentPath, "lifecycle config": e.LifecyclePath,
	} {
		if value == "" || !filepath.IsAbs(value) || filepath.Clean(value) != value {
			return fmt.Errorf("automatic update %s path must be canonical and absolute", name)
		}
	}
	return nil
}

func (e CommandExecutor) Recover(ctx context.Context) error {
	if err := e.Validate(); err != nil {
		return err
	}
	if _, err := os.Lstat(filepath.Join(e.StateDir, "transaction")); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	if err := trustedRegularFile(e.LifecyclePath); err != nil {
		return err
	}
	const recovery = `set -eu
docker_app_plus() { :; }
kpanel_recovery_entry() {
  . "$1"
  kpanel_recover_automatic_update
}
kpanel_recovery_entry "$1"`
	command := exec.CommandContext(ctx, "/bin/bash", "-c", recovery, "kpanel-recover", e.LifecyclePath)
	command.Stdout = writerOrDiscard(e.Stdout)
	command.Stderr = writerOrDiscard(e.Stderr)
	command.Env = fixedEnvironment(nil)
	return command.Run()
}

func (e CommandExecutor) InstalledVersion(ctx context.Context) (string, error) {
	if err := e.Validate(); err != nil {
		return "", err
	}
	if err := trustedRegularFile(e.AgentPath); err != nil {
		return "", err
	}
	command := exec.CommandContext(ctx, e.AgentPath, "version")
	var output bytes.Buffer
	command.Stdout = &limitedWriter{writer: &output, remaining: 256}
	command.Stderr = io.Discard
	command.Env = fixedEnvironment(nil)
	if err := command.Run(); err != nil {
		return "", err
	}
	fields := strings.Fields(output.String())
	if len(fields) != 2 || fields[1] != "v1alpha1" {
		return "", errors.New("installed Agent returned an invalid version contract")
	}
	version := normalizeStableVersion(fields[0])
	if version == "" {
		return "", errors.New("installed Agent returned an invalid version")
	}
	return version, nil
}

func (e CommandExecutor) Update(ctx context.Context, targetVersion, imageDigest string) error {
	if err := e.Validate(); err != nil {
		return err
	}
	targetVersion = normalizeStableVersion(targetVersion)
	if targetVersion == "" {
		return errors.New("automatic update target version is invalid")
	}
	imageDigest = normalizeImageDigest(imageDigest)
	if imageDigest == "" {
		return errors.New("automatic update image digest is invalid")
	}
	if err := trustedRegularFile(e.ScriptPath); err != nil {
		return err
	}
	if err := trustedRegularFile(e.LifecyclePath); err != nil {
		return err
	}
	const update = `set -eu
docker_app_plus() { :; }
kpanel_update_entry() {
  . "$1"
  docker_app_update
}
kpanel_update_entry "$1"`
	command := exec.CommandContext(ctx, "/bin/bash", "-c", update, "kpanel-update", e.LifecyclePath)
	command.Stdout = writerOrDiscard(e.Stdout)
	command.Stderr = writerOrDiscard(e.Stderr)
	command.Env = fixedEnvironment(map[string]string{
		"KJ_KPANEL_AUTOMATIC":      "1",
		"KJ_KPANEL_TARGET_VERSION": targetVersion,
		"KJ_KPANEL_TARGET_IMAGE":   officialImageName + "@" + imageDigest,
	})
	return command.Run()
}

func trustedRegularFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0022 != 0 || !trustedFileOwner(info) {
		return fmt.Errorf("automatic update file is unsafe: %s", filepath.Base(path))
	}
	return nil
}

func fixedEnvironment(values map[string]string) []string {
	environment := make([]string, 0, len(values)+2)
	environment = append(environment,
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"LANG=C.UTF-8",
	)
	for key, value := range values {
		environment = append(environment, key+"="+value)
	}
	return environment
}

func writerOrDiscard(writer io.Writer) io.Writer {
	if writer == nil {
		return io.Discard
	}
	return writer
}

type limitedWriter struct {
	writer    io.Writer
	remaining int
}

func (w *limitedWriter) Write(value []byte) (int, error) {
	length := len(value)
	if w.remaining > 0 {
		write := value
		if len(write) > w.remaining {
			write = write[:w.remaining]
		}
		_, _ = w.writer.Write(write)
		w.remaining -= len(write)
	}
	return length, nil
}
