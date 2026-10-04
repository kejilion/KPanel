package main

import (
	"context"
	"log/slog"
	"net/http"
	"path/filepath"

	"github.com/kejilion/kejilion-panel/internal/agent"
	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/dockerx"
	"github.com/kejilion/kejilion-panel/internal/monitoring"
	"github.com/kejilion/kejilion-panel/internal/systeminfo"
)

// Monitoring shares the existing root broker process, not its file routes.
// Collection has a separate collector and lifetime from network relay retries.
func startNodeMonitoring(parent context.Context, stateDir string) (http.Handler, func()) {
	ctx, cancel := context.WithCancel(parent)
	// Rates follow the same interfaces the telemetry reports. The continuity
	// offset stays in memory: OpenWrt keeps this state directory on flash.
	collector := systeminfo.NewCollector()
	collector.TrafficSelectionPath = defaultTrafficSelectionPath
	history, err := monitoring.New(monitoring.Config{
		StateDir:        filepath.Join(stateDir, "monitoring"),
		System:          collector,
		Docker:          dockerx.New("/var/run/docker.sock", "/home/web", stateDir),
		OperatorLatency: monitoring.NewOperatorLatencyProber(),
		OnCheckStatus: func(summary contract.ServiceCheckSummary) {
			if err := publishNodeCheckStatus(summary); err != nil {
				slog.Warn("check status relay unavailable")
			}
		},
	})
	if err != nil {
		slog.Warn("history monitoring is unavailable", "error", err)
		return agent.NewMonitoringHandler(nil), cancel
	}
	done := make(chan struct{})
	go func() { defer close(done); history.Run(ctx) }()
	return agent.NewMonitoringHandler(history), func() { cancel(); <-done }
}
