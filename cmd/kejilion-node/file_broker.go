package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
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
	config, identity, err := ensureFileCapability(ctx, *configPath, config, secret, *fileConfigPath)
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
	runLightFileControl(ctx, config, identity, relay, agent.NewFileHandler(manager))
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
		terminal, identity, readErr = readTerminalConfig(fileConfigPath)
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
	headers, err := signedLightNodeHeaders(config, cluster.LightFileCapabilityPath, body, secret)
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
		return nodeConfig{}, terminalIdentity{}, fmt.Errorf("upgrade lightweight file capability: %w", err)
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
