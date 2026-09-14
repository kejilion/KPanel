package systemmanage

import (
	"fmt"
	"time"

	"github.com/kejilion/kejilion-panel/internal/jobcontrol"
)

func (m *Manager) backgroundJobsAvailable() error {
	if err := jobcontrol.Available(m.runner); err != nil {
		return fmt.Errorf("systemd-run 或 OpenRC start-stop-daemon 不可用: %w", err)
	}
	return nil
}

func (m *Manager) backgroundJobSpec(
	unit string,
	executable string,
	arguments []string,
	properties []string,
	umask string,
	nice int,
	stopTimeout time.Duration,
) jobcontrol.Spec {
	return jobcontrol.Spec{
		Unit:              unit,
		Executable:        executable,
		Arguments:         arguments,
		StateDir:          m.stateDir,
		SystemdProperties: properties,
		UMask:             umask,
		Nice:              nice,
		StopTimeout:       stopTimeout,
	}
}
