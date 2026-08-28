package main

import (
	"context"
	"errors"
	"flag"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"

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
	runLightTerminalControl(ctx, config, identity, relayClient, manager)
	return nil
}
