package main

import (
	"context"
	"log/slog"
	"slices"
	"time"

	"github.com/kejilion/kejilion-panel/internal/cluster"
	"github.com/kejilion/kejilion-panel/internal/desktopbridge"
	"github.com/kejilion/kejilion-panel/internal/desktopcredentials"
	"github.com/kejilion/kejilion-panel/internal/windowsnode"
)

func init() {
	windowsDesktopBroker = runWindowsDesktopBroker
	windowsDesktopReason = func() string { return desktopbridge.Code(desktopbridge.Probe()) }
}

func runWindowsDesktopBroker(ctx context.Context, config nodeConfig) {
	// This runs only in the existing SYSTEM terminal service, never in the
	// telemetry service. Do not advertise a control connection before the local
	// installation capability and authenticated center capability both agree.
	if requireBrokerIdentity() != nil {
		return
	}
	var prepare func(context.Context) (desktopcredentials.Credentials, func() error, error)
	if slices.Contains(config.Capabilities, "desktop-managed") {
		manager, err := windowsnode.OpenDesktopAccountManager()
		if err != nil {
			slog.Error("managed desktop account recovery failed; desktop access remains disabled")
			return
		}
		prepare = manager.Prepare
	}
	_, identity, err := readTerminalConfig(defaultTerminalConfigPath)
	if err != nil {
		slog.Warn("desktop identity unavailable")
		return
	}
	relay, err := cluster.NewTerminalRelayClient(nodeHTTPClient)
	if err != nil {
		return
	}
	backoff := time.Second
	for ctx.Err() == nil {
		if err := desktopbridge.Probe(); err != nil {
			slog.Debug("desktop listener unavailable", "code", desktopbridge.Code(err))
			if !waitContext(ctx, 30*time.Second) {
				return
			}
			continue
		}
		if err := requireWindowsCapability(ctx, config, "desktop"); err != nil {
			slog.Debug("desktop center capability unavailable")
			if !waitContext(ctx, time.Minute) {
				return
			}
			continue
		}
		connected := false
		err = relay.RunManagedDesktopStream(ctx, config.Origin, config.NodeID, config.TargetNodeID, identity.Key, identity.Peer,
			desktopbridge.Serve, prepare, func() { connected = true })
		if ctx.Err() != nil {
			return
		}
		delay := backoff
		if connected {
			backoff, delay = time.Second, time.Second
		} else {
			backoff = min(time.Minute, backoff*2)
		}
		slog.Debug("desktop stream disconnected", "code", desktopbridge.Code(err), "retry", delay)
		if !waitContext(ctx, delay) {
			return
		}
	}
}
