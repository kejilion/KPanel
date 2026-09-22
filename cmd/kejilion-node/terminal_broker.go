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

	"github.com/kejilion/kejilion-panel/internal/cluster"
	"github.com/kejilion/kejilion-panel/internal/terminal"
)

const defaultTerminalBrokerConfigPath = defaultConfigPath

// runTerminalBroker is deliberately a separate root service. The low
// privilege telemetry process never receives a root IPC capability; this
// service owns the fixed PTY manager and the authenticated outbound terminal
// relay instead.
func runTerminalBroker(arguments []string) error {
	flags := flag.NewFlagSet("kejilion-node terminal-broker", flag.ContinueOnError)
	configPath := flags.String("config", defaultTerminalBrokerConfigPath, "node configuration path")
	terminalConfigPath := flags.String("terminal-config", defaultTerminalConfigPath, "root-only terminal configuration path")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if flags.NArg() != 0 || !filepath.IsAbs(*configPath) || filepath.Clean(*configPath) == string(filepath.Separator) ||
		!filepath.IsAbs(*terminalConfigPath) || filepath.Clean(*terminalConfigPath) == string(filepath.Separator) {
		return errors.New("terminal broker configuration path is invalid")
	}
	if runtime.GOOS == "linux" && os.Geteuid() != 0 {
		return errors.New("terminal broker requires root")
	}
	config, _, err := readConfig(*configPath)
	if err != nil {
		return err
	}
	if config.TargetNodeID == "" {
		return errors.New("terminal relay is not enrolled")
	}
	_, identity, err := readTerminalConfig(*terminalConfigPath)
	if err != nil {
		return err
	}
	relayClient, err := cluster.NewTerminalRelayClient(nodeHTTPClient)
	if err != nil {
		return err
	}
	manager := terminal.New(terminal.Config{ParentUnit: ""})
	defer manager.CloseAll()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	streamDone := make(chan struct{})
	go func() {
		defer close(streamDone)
		runLightTerminalStream(ctx, config, identity, relayClient, manager)
	}()
	runLightTerminalControl(ctx, config, identity, relayClient, manager)
	stop()
	<-streamDone
	return nil
}

// runLightTerminalStream keeps the optional push transport connected. The
// polling relay stays authoritative for compatibility: an older center
// rejects the stream role after the upgrade, so failures back off to long
// intervals instead of retrying hot.
func runLightTerminalStream(ctx context.Context, config nodeConfig, identity terminalIdentity, relay *cluster.TerminalRelayClient, manager *terminal.Manager) {
	backoff := time.Minute
	for ctx.Err() == nil {
		connected := false
		err := relay.RunTerminalStream(ctx, config.Origin, config.NodeID, config.TargetNodeID, identity.Key, identity.Peer,
			manager, lightTerminalOwner(config.NodeID), func() { connected = true })
		if ctx.Err() != nil {
			return
		}
		delay := backoff
		if connected {
			// An established stream that dropped reconnects quickly.
			backoff, delay = time.Minute, time.Second
		} else {
			backoff = min(30*time.Minute, backoff*2)
		}
		slog.Debug("terminal stream unavailable; polling relay continues", "error", err, "retry", delay)
		if !waitContext(ctx, delay) {
			return
		}
	}
}
