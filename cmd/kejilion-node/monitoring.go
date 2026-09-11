package main

import (
	"context"
	"log/slog"
	"net/http"
	"path/filepath"

	"github.com/kejilion/kejilion-panel/internal/agent"
	"github.com/kejilion/kejilion-panel/internal/dockerx"
	"github.com/kejilion/kejilion-panel/internal/monitoring"
	"github.com/kejilion/kejilion-panel/internal/systeminfo"
)

// Monitoring shares the existing root broker process, not its file routes.
// Collection has a separate collector and lifetime from network relay retries.
func startNodeMonitoring(parent context.Context, stateDir string) (http.Handler, func()) {
	ctx, cancel := context.WithCancel(parent)
	history, err := monitoring.New(monitoring.Config{
		StateDir:        filepath.Join(stateDir, "monitoring"),
		System:          systeminfo.NewCollector(),
		Docker:          dockerx.New("/var/run/docker.sock", "/home/web", stateDir),
		OperatorLatency: monitoring.NewOperatorLatencyProber(),
	})
	if err != nil {
		slog.Warn("history monitoring is unavailable", "error", err)
		return agent.NewMonitoringHandler(nil), cancel
	}
	done := make(chan struct{})
	go func() { defer close(done); history.Run(ctx) }()
	return agent.NewMonitoringHandler(history), func() { cancel(); <-done }
}
