package systemmanage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/jobcontrol"
)

const (
	rebootDelay    = 15 * time.Second
	rebootUnitName = "kejilion-panel-reboot"
)

func (m *Manager) scheduleReboot(
	ctx context.Context,
	input contract.SystemActionRequest,
) (bool, string, error) {
	if !validRebootRequest(input) {
		return false, "", fmt.Errorf(
			"%w: reboot accepts only the typed action and no unrelated fields",
			ErrInvalidInput,
		)
	}
	if m.rebootScheduled {
		return false, "", fmt.Errorf("%w: a reboot task is already scheduled", ErrConflict)
	}
	backend, backendErr := jobcontrol.Detect(m.runner)
	if backendErr != nil {
		return false, "", fmt.Errorf("%w: reboot task backend is unavailable", ErrUnsupported)
	}
	if backend == jobcontrol.BackendOpenRC {
		if _, err := m.runner.LookPath("reboot"); err != nil {
			return false, "", fmt.Errorf("%w: reboot is unavailable", ErrUnsupported)
		}
		executable, err := m.backgroundExecutable()
		if err != nil {
			return false, "", err
		}
		spec := m.backgroundJobSpec(
			rebootUnitName,
			executable,
			[]string{"reboot-run", "--delay-seconds", fmt.Sprint(int(rebootDelay / time.Second))},
			nil,
			"0077",
			0,
			5*time.Second,
		)
		spec.NoNewPrivileges = true
		if err := jobcontrol.Launch(ctx, m.runner, spec); err != nil {
			return false, "", fmt.Errorf("%w: schedule OpenRC reboot: %v", ErrUnsupported, err)
		}
		m.rebootScheduled = true
		return true, rebootScheduledMessage(), nil
	}
	systemctlPath, err := m.runner.LookPath("systemctl")
	if err != nil || !filepath.IsAbs(systemctlPath) || filepath.Base(systemctlPath) != "systemctl" {
		return false, "", fmt.Errorf("%w: systemctl is unavailable", ErrUnsupported)
	}
	arguments := []string{
		"--unit=" + rebootUnitName,
		"--collect",
		"--no-block",
		"--on-active=" + rebootDelay.String(),
		"--timer-property=AccuracySec=1s",
		"--property=Type=oneshot",
		"--property=TimeoutStartSec=2min",
		"--property=User=root",
		"--property=UMask=0077",
		"--property=NoNewPrivileges=yes",
		"--property=PrivateTmp=yes",
		"--property=ProtectHome=yes",
		"--property=ProtectSystem=strict",
		"--property=RestrictAddressFamilies=AF_UNIX",
		"--property=SyslogIdentifier=kpanel-reboot",
		"--",
		filepath.Clean(systemctlPath),
		"--no-wall",
		"reboot",
	}
	if _, err := m.runner.Run(ctx, "systemd-run", arguments...); err != nil {
		return false, "", fmt.Errorf("%w: schedule reboot: %v", ErrUnsupported, err)
	}
	m.rebootScheduled = true
	return true, rebootScheduledMessage(), nil
}

func rebootScheduledMessage() string {
	return fmt.Sprintf(
		"重启任务已排队，服务器将在约 %d 秒后离线；正常情况下 KPanel 会随系统启动恢复",
		int(rebootDelay/time.Second),
	)
}

func (m *Manager) rebootAvailable() error {
	backend, err := jobcontrol.Detect(m.runner)
	if err != nil {
		return err
	}
	if backend == jobcontrol.BackendSystemd {
		path, pathErr := m.runner.LookPath("systemctl")
		if pathErr != nil || !filepath.IsAbs(path) || filepath.Base(path) != "systemctl" {
			return errors.New("systemctl is unavailable")
		}
		return nil
	}
	if _, err := m.backgroundExecutable(); err != nil {
		return err
	}
	if _, err := m.runner.LookPath("reboot"); err != nil {
		return errors.New("reboot is unavailable")
	}
	return nil
}

func RunDelayedReboot(ctx context.Context, delay time.Duration) error {
	if delay != rebootDelay {
		return errors.New("invalid reboot delay")
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
	}
	path, err := trustedRebootPath()
	if err != nil {
		return err
	}
	return exec.CommandContext(ctx, path).Run()
}

func trustedRebootPath() (string, error) {
	for _, candidate := range []string{"/sbin/reboot", "/usr/sbin/reboot"} {
		resolved, err := filepath.EvalSymlinks(candidate)
		if err != nil {
			continue
		}
		info, err := os.Stat(resolved)
		if err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o022 == 0 &&
			dnsScriptOwnerTrusted(info) {
			return resolved, nil
		}
	}
	return "", errors.New("trusted reboot executable is unavailable")
}

func validRebootRequest(input contract.SystemActionRequest) bool {
	return input.Action == "reboot" &&
		input.Hostname == "" &&
		input.Port == 0 &&
		len(input.Servers) == 0 &&
		input.Timezone == "" &&
		input.SwapSizeMiB == 0 &&
		input.MirrorPreset == "" &&
		input.Preference == "" &&
		input.Profile == "" &&
		input.MaintenancePolicy == "" &&
		input.Enabled == nil
}
