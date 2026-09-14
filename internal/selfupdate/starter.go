package selfupdate

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
)

const automaticUpdateUnit = "kejilion-panel-update.service"

type UpdateStarter interface {
	Start(context.Context) error
}

type SystemdStarter struct{}

func NewSystemdStarter() *SystemdStarter {
	return &SystemdStarter{}
}

func (*SystemdStarter) Start(ctx context.Context) error {
	if info, err := os.Stat("/run/systemd/system"); err != nil || !info.IsDir() {
		return errors.New("systemd is not running")
	}
	commandPath := ""
	for _, candidate := range []string{"/usr/bin/systemctl", "/bin/systemctl"} {
		if err := trustedRegularFile(candidate); err == nil {
			commandPath = candidate
			break
		}
	}
	if commandPath == "" {
		return errors.New("trusted systemctl is unavailable")
	}
	command := exec.CommandContext(ctx, commandPath, "start", "--no-block", automaticUpdateUnit)
	command.Env = fixedEnvironment(nil)
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	return command.Run()
}
