package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/kejilion/kejilion-panel/internal/systemmanage"
)

const (
	sshLoginBrokerPollInterval = 5 * time.Second
	sshLoginBrokerReadTimeout  = 10 * time.Second
)

// runSSHLoginBroker is the privileged half of lightweight SSH telemetry. It
// reads the same narrow event as a regular Agent and publishes only the
// validated, credential-free event to a group-readable runtime file. The
// reporting process remains non-root and the Telegram credential never leaves
// the center panel.
func runSSHLoginBroker(arguments []string) error {
	flags := flag.NewFlagSet("kejilion-node ssh-login-broker", flag.ContinueOnError)
	outputPath := flags.String("output", systemmanage.SSHLoginEventPath, "SSH login event output path")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if flags.NArg() != 0 || !filepath.IsAbs(*outputPath) || filepath.Clean(*outputPath) == string(filepath.Separator) {
		return errors.New("SSH login broker output path is invalid")
	}
	if runtime.GOOS == "linux" && os.Geteuid() != 0 {
		return errors.New("SSH login broker requires root")
	}

	manager := systemmanage.NewManager(systemmanage.Config{Enabled: false})
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	lastEventID := ""
	for {
		if err := pollAndPublishSSHLogin(ctx, manager, filepath.Clean(*outputPath), &lastEventID); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			slog.Warn("SSH login event collection failed", "error", err)
		}
		if !waitContext(ctx, sshLoginBrokerPollInterval) {
			return nil
		}
	}
}

func pollAndPublishSSHLogin(
	parent context.Context,
	manager *systemmanage.Manager,
	outputPath string,
	lastEventID *string,
) error {
	if manager == nil || lastEventID == nil {
		return errors.New("SSH login broker state is invalid")
	}
	ctx, cancel := context.WithTimeout(parent, sshLoginBrokerReadTimeout)
	event, err := manager.LatestSSHLogin(ctx)
	cancel()
	if err != nil {
		return err
	}
	if event == nil || event.ID == *lastEventID {
		return nil
	}
	if err := systemmanage.WriteSSHLoginEvent(outputPath, *event); err != nil {
		return err
	}
	*lastEventID = event.ID
	return nil
}
