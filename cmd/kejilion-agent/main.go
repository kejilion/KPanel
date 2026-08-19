package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/kejilion/kejilion-panel/internal/agent"
	"github.com/kejilion/kejilion-panel/internal/agentclient"
	"github.com/kejilion/kejilion-panel/internal/appmarket"
	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/diagnostics"
	"github.com/kejilion/kejilion-panel/internal/dockerx"
	"github.com/kejilion/kejilion-panel/internal/monitoring"
	"github.com/kejilion/kejilion-panel/internal/sites"
	"github.com/kejilion/kejilion-panel/internal/systeminfo"
	"github.com/kejilion/kejilion-panel/internal/systemmanage"
	"github.com/kejilion/kejilion-panel/internal/terminal"
	"github.com/kejilion/kejilion-panel/internal/version"
	"github.com/kejilion/kejilion-panel/internal/webenv"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		slog.Error("kejilion-agent stopped", "error", err)
		os.Exit(1)
	}
}

func run(arguments []string) error {
	if len(arguments) > 0 && arguments[0] == "version" {
		if len(arguments) != 1 {
			return errors.New("kejilion-agent version does not accept arguments")
		}
		fmt.Printf("%s %s\n", version.Version, version.ProtocolVersion)
		return nil
	}
	if len(arguments) > 0 && arguments[0] == "healthcheck" {
		return runHealthcheck(arguments[1:])
	}
	if len(arguments) > 0 && arguments[0] == "maintenance-run" {
		return runMaintenance(arguments[1:])
	}
	if len(arguments) > 0 && arguments[0] == "swap-run" {
		return runSwap(arguments[1:])
	}
	if len(arguments) > 0 && arguments[0] == "app-run" {
		return runAppJob(arguments[1:], false)
	}
	if len(arguments) > 0 && arguments[0] == "app-pty-run" {
		return runAppJob(arguments[1:], true)
	}
	if len(arguments) > 0 && arguments[0] == "diagnostic-run" {
		return runDiagnosticJob(arguments[1:])
	}
	if len(arguments) > 0 && arguments[0] == "site-pty-run" {
		return runSiteJob(arguments[1:])
	}
	if len(arguments) > 0 && arguments[0] == "environment-run" {
		return runEnvironmentJob(arguments[1:])
	}

	flags := flag.NewFlagSet("kejilion-agent", flag.ContinueOnError)
	socketPath := flags.String("socket", env("KEJILION_AGENT_SOCKET", "/run/kejilion-panel/agent.sock"), "Unix Socket path")
	socketGroup := flags.String("socket-group", env("KEJILION_AGENT_SOCKET_GROUP", "kejilion-panel"), "Unix Socket group")
	tokenFile := flags.String("token-file", env("KEJILION_AGENT_TOKEN_FILE", "/etc/kejilion-panel/agent.token"), "shared token file")
	stateDir := flags.String("state-dir", env("KEJILION_AGENT_STATE_DIR", "/var/lib/kejilion-panel"), "Agent state directory")
	webRoot := flags.String("web-root", env("KEJILION_WEB_ROOT", "/home/web"), "Kejilion Web root")
	dockerSocket := flags.String("docker-socket", env("KEJILION_DOCKER_SOCKET", "/var/run/docker.sock"), "Docker Engine Unix Socket")
	dockerPIDFile := flags.String("docker-pid-file", env("KEJILION_DOCKER_PID_FILE", "/run/docker.pid"), "Docker daemon PID file")
	allowDockerSocketActivation := flags.Bool(
		"allow-docker-socket-activation",
		envBool("KEJILION_DOCKER_ALLOW_SOCKET_ACTIVATION", false),
		"allow connecting to Docker Socket when no running dockerd PID can be verified",
	)
	enableSystemWrites := flags.Bool(
		"enable-system-writes",
		envBool("KEJILION_SYSTEM_WRITES_ENABLED", true),
		"enable typed, audited host system mutations",
	)
	enablePublicNetworkLookup := flags.Bool(
		"public-network-lookup",
		envBool("KEJILION_PUBLIC_NETWORK_LOOKUP_ENABLED", true),
		"query fixed IPinfo endpoints for cached public IP, ISP, and location metadata",
	)
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected argument: %s", flags.Arg(0))
	}

	token, err := agent.PrepareTokenFile(*tokenFile, *socketGroup)
	if err != nil {
		return err
	}
	dockerClient := dockerx.New(*dockerSocket, *webRoot, *stateDir)
	dockerClient.ConfigureDaemonAccess(*dockerPIDFile, *allowDockerSocketActivation)
	systemCollector := systeminfo.NewCollector()
	systemCollector.PublicNetworkLookupEnabled = *enablePublicNetworkLookup
	dockerClient.ConfigureImageUpdateFallback(func(context.Context) (string, error) {
		if !*enablePublicNetworkLookup {
			return "ZZ", nil
		}
		country := strings.ToUpper(strings.TrimSpace(
			systemCollector.CachedPublicNetwork().CountryCode,
		))
		if len(country) != 2 {
			return "", errors.New("public country is unavailable")
		}
		return country, nil
	})
	appMarket, err := appmarket.NewWithOfficialCatalog(dockerClient, "/home/docker")
	if err != nil {
		return fmt.Errorf("initialize application market: %w", err)
	}
	if err := appMarket.ConfigureIconCache(filepath.Join(*stateDir, "app-icons")); err != nil {
		slog.Warn("application icon cache is unavailable; dynamic icons will use the fallback", "error", err)
	}
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve Agent executable: %w", err)
	}
	if err := appMarket.ConfigureJobs(filepath.Join(*stateDir, "app-jobs"), executable); err != nil {
		return fmt.Errorf("initialize application jobs: %w", err)
	}
	diagnosticService, err := diagnostics.New(
		filepath.Join(*stateDir, "diagnostic-jobs"),
		executable,
	)
	if err != nil {
		return fmt.Errorf("initialize diagnostic jobs: %w", err)
	}
	historyService, historyErr := monitoring.New(monitoring.Config{
		StateDir:        filepath.Join(*stateDir, "monitoring"),
		System:          systemCollector,
		Docker:          dockerClient,
		OperatorLatency: monitoring.NewOperatorLatencyProber(),
	})
	if historyErr != nil {
		slog.Warn("history monitoring is unavailable", "error", historyErr)
	}
	terminalManager := terminal.New(terminal.Config{})
	handler, err := agent.NewServer(agent.Config{
		Token: token, Version: version.Version, ProtocolVersion: version.ProtocolVersion,
		WebRoot: *webRoot, StateDir: *stateDir, System: systemCollector,
		SystemManager: systemmanage.NewManager(systemmanage.Config{
			Enabled: *enableSystemWrites, StateDir: filepath.Join(*stateDir, "system"),
			Executable: executable,
		}),
		Sites: sites.NewDiscoverer(*webRoot), Docker: dockerClient, AppMarket: appMarket,
		Diagnostics: diagnosticService, Monitoring: historyService, Terminals: terminalManager,
	})
	clear(token)
	if err != nil {
		terminalManager.CloseAll()
		return err
	}
	defer handler.Close()
	listener, cleanup, err := agent.ListenUnix(*socketPath, *socketGroup)
	if err != nil {
		return err
	}
	defer cleanup()

	httpServer := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      2 * time.Minute,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if historyService != nil {
		go historyService.Run(ctx)
	}
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- httpServer.Serve(listener)
	}()
	slog.Info("kejilion-agent listening", "socket", *socketPath, "version", version.Version)
	select {
	case err := <-serverErr:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve Agent API: %w", err)
		}
	case <-ctx.Done():
		shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownContext); err != nil {
			return fmt.Errorf("shutdown Agent API: %w", err)
		}
	}
	return nil
}

func runMaintenance(arguments []string) error {
	flags := flag.NewFlagSet("kejilion-agent maintenance-run", flag.ContinueOnError)
	stateDir := flags.String(
		"state-dir",
		env("KEJILION_AGENT_STATE_DIR", "/var/lib/kejilion-panel/system"),
		"system maintenance state directory",
	)
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return errors.New("maintenance-run requires exactly one fixed mode")
	}
	if os.Geteuid() != 0 {
		return errors.New("maintenance-run requires root")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 45*time.Minute)
	defer cancel()
	manager := systemmanage.NewManager(systemmanage.Config{
		Enabled:  true,
		StateDir: *stateDir,
	})
	return manager.RunMaintenance(ctx, flags.Arg(0))
}

func runSwap(arguments []string) error {
	flags := flag.NewFlagSet("kejilion-agent swap-run", flag.ContinueOnError)
	stateDir := flags.String(
		"state-dir",
		env("KEJILION_AGENT_STATE_DIR", "/var/lib/kejilion-panel/system"),
		"system state directory",
	)
	swapPath := flags.String("swap-path", "/swapfile", "kejilion.sh-compatible swapfile path")
	sizeMiB := flags.Int("size-mib", -1, "target swap size in MiB; zero disables it")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("swap-run does not accept positional arguments")
	}
	if os.Geteuid() != 0 {
		return errors.New("swap-run requires root")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	manager := systemmanage.NewManager(systemmanage.Config{
		Enabled: true, StateDir: *stateDir, SwapPath: *swapPath,
	})
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(manager.RunSwapTransaction(ctx, *sizeMiB))
}

func runAppJob(arguments []string, interactive bool) error {
	commandName := "app-run"
	if interactive {
		commandName = "app-pty-run"
	}
	flags := flag.NewFlagSet("kejilion-agent "+commandName, flag.ContinueOnError)
	stateDir := flags.String(
		"state-dir",
		env("KEJILION_AGENT_STATE_DIR", "/var/lib/kejilion-panel/app-jobs"),
		"application job state directory",
	)
	id := flags.String("id", "", "application job identity")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if flags.NArg() != 0 || *id == "" {
		return fmt.Errorf("%s requires exactly one job identity", commandName)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 45*time.Minute)
	defer cancel()
	if interactive {
		return appmarket.RunInteractiveAppJob(ctx, *stateDir, *id)
	}
	return appmarket.RunAppJob(ctx, *stateDir, *id)
}

func runDiagnosticJob(arguments []string) error {
	flags := flag.NewFlagSet("kejilion-agent diagnostic-run", flag.ContinueOnError)
	stateDir := flags.String(
		"state-dir",
		env("KEJILION_AGENT_STATE_DIR", "/var/lib/kejilion-panel/diagnostic-jobs"),
		"diagnostic job state directory",
	)
	id := flags.String("id", "", "diagnostic job identity")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if flags.NArg() != 0 || *id == "" {
		return errors.New("diagnostic-run requires exactly one job identity")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 90*time.Minute)
	defer cancel()
	return diagnostics.RunJob(ctx, *stateDir, *id)
}

func runEnvironmentJob(arguments []string) error {
	flags := flag.NewFlagSet("kejilion-agent environment-run", flag.ContinueOnError)
	stateDir := flags.String(
		"state-dir",
		env("KEJILION_AGENT_STATE_DIR", "/var/lib/kejilion-panel/environment-jobs"),
		"environment job state directory",
	)
	id := flags.String("id", "", "environment job identity")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if flags.NArg() != 0 || *id == "" {
		return errors.New("environment-run requires exactly one job identity")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 90*time.Minute)
	defer cancel()
	return webenv.RunJob(ctx, *stateDir, *id)
}

func runSiteJob(arguments []string) error {
	flags := flag.NewFlagSet("kejilion-agent site-pty-run", flag.ContinueOnError)
	stateDir := flags.String(
		"state-dir",
		env("KEJILION_AGENT_STATE_DIR", "/var/lib/kejilion-panel/site-recipe-jobs"),
		"website job state directory",
	)
	webRoot := flags.String(
		"web-root",
		env("KEJILION_WEB_ROOT", "/home/web"),
		"Kejilion Web root",
	)
	id := flags.String("id", "", "website job identity")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if flags.NArg() != 0 || *id == "" {
		return errors.New("site-pty-run requires exactly one job identity")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 60*time.Minute)
	defer cancel()
	return sites.RunRecipeJob(ctx, *stateDir, *webRoot, *id)
}

func runHealthcheck(arguments []string) error {
	flags := flag.NewFlagSet("kejilion-agent healthcheck", flag.ContinueOnError)
	socketPath := flags.String("socket", env("KEJILION_AGENT_SOCKET", "/run/kejilion-panel/agent.sock"), "Unix Socket path")
	tokenFile := flags.String("token-file", env("KEJILION_AGENT_TOKEN_FILE", "/etc/kejilion-panel/agent.token"), "shared token file")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected healthcheck argument: %s", flags.Arg(0))
	}

	client, err := agentclient.New(*socketPath, *tokenFile)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	var health contract.AgentHealth
	if err := client.Get(ctx, "/v1/health", &health); err != nil {
		return fmt.Errorf("Agent healthcheck request: %w", err)
	}
	return validateAgentHealth(health)
}

func validateAgentHealth(health contract.AgentHealth) error {
	if health.Version != version.Version {
		return fmt.Errorf("Agent version mismatch: running %q, expected %q", health.Version, version.Version)
	}
	if health.ProtocolVersion != version.ProtocolVersion {
		return fmt.Errorf(
			"Agent protocol mismatch: running %q, expected %q",
			health.ProtocolVersion,
			version.ProtocolVersion,
		)
	}
	if !health.CoreReady() {
		return fmt.Errorf("Agent is not ready: status %q, reasons %v", health.Status, health.Reasons)
	}
	if health.ReadOnly {
		return errors.New("Agent is unexpectedly read-only")
	}
	return nil
}

func env(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func envBool(name string, fallback bool) bool {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func clear(value []byte) {
	for i := range value {
		value[i] = 0
	}
}
