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

	"github.com/kejilion/kejilion-panel/internal/agent"
	"github.com/kejilion/kejilion-panel/internal/cluster"
	"github.com/kejilion/kejilion-panel/internal/filemanager"
)

const defaultFileBrokerConfigPath = defaultConfigPath

// runFileBroker is a root-only service. It reuses the Agent file-manager
// policy, but exposes no local socket: every request arrives through the
// authenticated outbound Noise relay owned by the center Panel.
func runFileBroker(arguments []string) error {
	flags := flag.NewFlagSet("kejilion-node file-broker", flag.ContinueOnError)
	configPath := flags.String("config", defaultFileBrokerConfigPath, "node configuration path")
	fileConfigPath := flags.String("terminal-config", defaultTerminalConfigPath, "root-only relay configuration path")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if flags.NArg() != 0 || !filepath.IsAbs(*configPath) || filepath.Clean(*configPath) == string(filepath.Separator) ||
		!filepath.IsAbs(*fileConfigPath) || filepath.Clean(*fileConfigPath) == string(filepath.Separator) {
		return errors.New("file broker configuration path is invalid")
	}
	if runtime.GOOS == "linux" && os.Geteuid() != 0 {
		return errors.New("file broker requires root")
	}
	config, _, err := readConfig(*configPath)
	if err != nil {
		return err
	}
	if config.TargetNodeID == "" {
		return errors.New("file relay is not enrolled")
	}
	_, identity, err := readTerminalConfig(*fileConfigPath)
	if err != nil {
		return err
	}
	manager, err := filemanager.New(agent.DefaultFileManagerConfig("/var/lib/kejilion-node"))
	if err != nil {
		return err
	}
	defer manager.Close()
	relay, err := cluster.NewFileRelayClient(nodeHTTPClient)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	runLightFileControl(ctx, config, identity, relay, agent.NewFileHandler(manager))
	return nil
}
