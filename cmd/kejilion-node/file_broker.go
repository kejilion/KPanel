package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/kejilion/kejilion-panel/internal/agent"
	"github.com/kejilion/kejilion-panel/internal/cluster"
	"github.com/kejilion/kejilion-panel/internal/filemanager"
)

const defaultFileBrokerConfigPath = defaultConfigPath

var errFileCapabilityConnection = errors.New("file capability connection unavailable")

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
	config, secret, err := readConfig(*configPath)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	historyHandler, stopHistory := startNodeMonitoring(ctx, "/var/lib/kejilion-node")
	defer stopHistory()
	var identity terminalIdentity
	for {
		updated, relayIdentity, capabilityErr := ensureFileCapability(ctx, *configPath, config, secret, *fileConfigPath)
		if capabilityErr == nil {
			config, identity = updated, relayIdentity
			break
		}
		if !errors.Is(capabilityErr, errFileCapabilityConnection) {
			return capabilityErr
		}
		// Keep sampling locally even when a legacy node cannot yet reach its center.
		if ctx.Err() != nil {
			return nil
		}
		slog.Warn("file capability connection unavailable; history collection continues", "error", capabilityErr)
		if !waitContext(ctx, time.Minute) {
			return nil
		}
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
	historyRelay, err := cluster.NewHistoryRelayClient(nodeHTTPClient)
	if err != nil {
		return err
	}
	historyDone := make(chan struct{})
	go func() {
		defer close(historyDone)
		runLightFileControl(ctx, config, identity, historyRelay, historyHandler)
	}()
	runLightFileControl(ctx, config, identity, relay, agent.NewFileHandler(manager))
	stop()
	<-historyDone
	return nil
}

func ensureFileCapability(
	ctx context.Context,
	configPath string,
	config nodeConfig,
	secret []byte,
	fileConfigPath string,
) (nodeConfig, terminalIdentity, error) {
	if config.TargetNodeID != "" {
		_, identity, err := readTerminalConfig(fileConfigPath)
		if err != nil {
			return nodeConfig{}, terminalIdentity{}, err
		}
		return config, identity, nil
	}

	var (
		terminal terminalConfig
		identity terminalIdentity
	)
	if _, err := os.Lstat(fileConfigPath); err == nil {
		var readErr error
		terminal, identity, readErr = readTerminalConfigMode(fileConfigPath, true)
		if readErr != nil {
			return nodeConfig{}, terminalIdentity{}, readErr
		}
	} else if errors.Is(err, os.ErrNotExist) {
		key, keyErr := cluster.GenerateFederationV2Keypair()
		if keyErr != nil {
			return nodeConfig{}, terminalIdentity{}, fmt.Errorf("generate file relay identity: %w", keyErr)
		}
		terminal = terminalConfig{
			SchemaVersion: 1,
			PrivateKey:    base64.RawURLEncoding.EncodeToString(key.Private),
			PublicKey:     base64.RawURLEncoding.EncodeToString(key.Public),
		}
		identity = terminalIdentity{Key: key}
		// Persist before sending: the center may bind this key even if its
		// response is lost. Retries and restarts must reuse the same identity.
		if err := writeTerminalConfigAtomic(fileConfigPath, terminal); err != nil {
			return nodeConfig{}, terminalIdentity{}, err
		}
	} else {
		return nodeConfig{}, terminalIdentity{}, err
	}

	request := cluster.LightFileCapabilityRequest{
		TerminalPublicKey: base64.RawURLEncoding.EncodeToString(identity.Key.Public),
	}
	body, err := json.Marshal(request)
	if err != nil {
		return nodeConfig{}, terminalIdentity{}, err
	}
	headers, err := signedLightNodeHeaders(config, cluster.LightFileCapabilityPath, body, secret, nil)
	if err != nil {
		return nodeConfig{}, terminalIdentity{}, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	requestContext, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var response cluster.LightFileCapabilityResponse
	if _, _, err := postRawJSONWithStatusAndHeaders(
		requestContext,
		config.Origin+cluster.LightFileCapabilityPath,
		body,
		headers,
		&response,
	); err != nil {
		return nodeConfig{}, terminalIdentity{}, fmt.Errorf("upgrade lightweight file capability: %w", errors.Join(errFileCapabilityConnection, err))
	}
	peer, err := decodeTerminalKey(response.TerminalPeerPublicKey)
	if err != nil || !validHexID(response.TargetNodeID) {
		return nodeConfig{}, terminalIdentity{}, errors.New("file capability response is invalid")
	}
	if len(identity.Peer) == 32 && !bytes.Equal(identity.Peer, peer) {
		return nodeConfig{}, terminalIdentity{}, cluster.ErrIdentityMismatch
	}
	terminal.PeerPublicKey = base64.RawURLEncoding.EncodeToString(peer)
	if err := writeTerminalConfigAtomic(fileConfigPath, terminal); err != nil {
		return nodeConfig{}, terminalIdentity{}, err
	}
	config.TargetNodeID = response.TargetNodeID
	if err := writeConfigAtomic(configPath, config); err != nil {
		return nodeConfig{}, terminalIdentity{}, err
	}
	identity.Peer = peer
	return config, identity, nil
}
