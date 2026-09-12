package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/kejilion/kejilion-panel/internal/agentclient"
	"github.com/kejilion/kejilion-panel/internal/auth"
	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/panel"
	"github.com/kejilion/kejilion-panel/internal/store"
	"github.com/kejilion/kejilion-panel/internal/version"
	"golang.org/x/term"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		slog.Error("paneld stopped", "error", err)
		os.Exit(1)
	}
}

func run(arguments []string) error {
	if len(arguments) > 0 && arguments[0] == "healthcheck" {
		return runHealthcheck(arguments[1:])
	}
	if len(arguments) > 0 && arguments[0] == "agent-healthcheck" {
		return runAgentHealthcheck(arguments[1:])
	}
	if len(arguments) > 0 && arguments[0] == "reset-password" {
		return runPasswordReset(arguments[1:], os.Stdin, os.Stdout)
	}

	flags := flag.NewFlagSet("paneld", flag.ContinueOnError)
	configPath := flags.String("config", "", "path to the JSON configuration file")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	config, err := panel.LoadConfig(*configPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(config.DataDir, 0o700); err != nil {
		return fmt.Errorf("create data directory: %w", err)
	}
	if err := panel.RecoverPanelRestore(config); err != nil {
		return fmt.Errorf("recover panel restore: %w", err)
	}
	for {
		err := runPanelInstance(config)
		if !errors.Is(err, panel.ErrBackupRestart) {
			if err != nil && panel.PanelRestorePending(config) {
				if recoveryErr := panel.RecoverPanelRestore(config); recoveryErr != nil {
					return errors.Join(err, recoveryErr)
				}
				slog.Error("restore startup failed; previous configuration restored", "error", err)
				return runPanelInstance(config)
			}
			return err
		}
		if err := panel.ApplyPanelRestore(config); err != nil {
			slog.Error("panel restore failed; recovery retained", "error", err)
			if recoveryErr := panel.RecoverPanelRestore(config); recoveryErr != nil {
				return recoveryErr
			}
		}
	}
}

func runPanelInstance(config panel.Config) error {

	storage, err := store.Open(config.StorePath)
	if err != nil {
		return err
	}
	defer storage.Close()
	hasher, err := auth.NewArgon2idHasher(auth.DefaultArgon2idParams())
	if err != nil {
		return err
	}
	authService, err := auth.NewService(storage, hasher, auth.Config{
		BootstrapTokenPath: config.BootstrapTokenPath,
		TOTPKeyPath:        config.TOTPKeyPath,
		SessionTTL:         config.SessionTTL,
		LoginWindow:        config.LoginWindow,
		MaxLoginFailures:   config.MaxLoginFailures,
	})
	if err != nil {
		return err
	}
	if err := authService.EnsureBootstrapToken(); err != nil {
		return err
	}

	agent := panel.NewAgentClient(config.AgentSocket, config.AgentTokenFile, config.MaxAgentBytes)
	handler, err := panel.NewServer(config, authService, storage, agent)
	if err != nil {
		return err
	}
	defer handler.Close()
	if err := handler.EnableAI(); err != nil {
		slog.Error("AI module disabled", "error", err)
	}
	server := &http.Server{
		Addr:              config.Listen,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       90 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	shutdownSignal, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	restoreRequested := make(chan struct{})
	shutdownDone := make(chan error, 1)
	go func() {
		select {
		case <-shutdownSignal.Done():
		case <-handler.BackupRestart():
			close(restoreRequested)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		shutdownErr := server.Shutdown(ctx)
		if shutdownErr != nil {
			_ = server.Close()
		}
		shutdownDone <- shutdownErr
	}()

	slog.Info("starting paneld",
		"version", version.Version,
		"protocol", version.ProtocolVersion,
		"listen", config.Listen,
		"initialized", authService.IsInitialized(),
	)
	listener, err := net.Listen("tcp", config.Listen)
	if err != nil {
		return err
	}
	if err := handler.FinalizePanelRestore(); err != nil {
		listener.Close()
		return err
	}
	handler.StartBackground(shutdownSignal)
	err = server.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) {
		if shutdownErr := <-shutdownDone; shutdownErr != nil {
			return shutdownErr
		}
		select {
		case <-restoreRequested:
			return panel.ErrBackupRestart
		default:
		}
		return nil
	}
	return err
}

type passwordReader func(prompt string) ([]byte, error)

func runPasswordReset(arguments []string, input *os.File, output io.Writer) error {
	if !term.IsTerminal(int(input.Fd())) && !passwordResetHelpRequested(arguments) {
		return errors.New("password reset requires an interactive terminal; do not pass passwords through arguments or environment variables")
	}
	return runPasswordResetWithReader(arguments, output, func(prompt string) ([]byte, error) {
		if _, err := fmt.Fprint(output, prompt); err != nil {
			return nil, err
		}
		password, err := term.ReadPassword(int(input.Fd()))
		_, newlineErr := fmt.Fprintln(output)
		if err != nil {
			return nil, err
		}
		if newlineErr != nil {
			clearPassword(password)
			return nil, newlineErr
		}
		return password, nil
	})
}

func passwordResetHelpRequested(arguments []string) bool {
	for _, argument := range arguments {
		if argument == "-h" || argument == "--help" {
			return true
		}
	}
	return false
}

func runPasswordResetWithReader(arguments []string, output io.Writer, readPassword passwordReader) error {
	flags := flag.NewFlagSet("paneld reset-password", flag.ContinueOnError)
	flags.SetOutput(output)
	configPath := flags.String("config", "", "path to the JSON configuration file")
	username := flags.String("username", "", "administrator username; optional for a single-user panel")
	disableTOTP := flags.Bool("disable-2fa", false, "also disable TOTP and invalidate recovery codes")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected reset-password argument: %s", flags.Arg(0))
	}

	config, err := panel.LoadConfig(*configPath)
	if err != nil {
		return err
	}
	storage, err := store.Open(config.StorePath)
	if err != nil {
		if errors.Is(err, store.ErrStoreLocked) {
			return errors.New("panel state is in use; stop the panel service before resetting the password")
		}
		return err
	}
	defer storage.Close()

	user, err := passwordRecoveryUser(storage, *username)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(output, "Administrator: %s\n", user.Username); err != nil {
		return err
	}
	password, err := readPassword("New password (12-256 bytes): ")
	if err != nil {
		return fmt.Errorf("read new password: %w", err)
	}
	defer clearPassword(password)
	confirmation, err := readPassword("Confirm new password: ")
	if err != nil {
		return fmt.Errorf("read password confirmation: %w", err)
	}
	defer clearPassword(confirmation)
	if !bytes.Equal(password, confirmation) {
		return errors.New("password confirmation does not match")
	}

	hasher, err := auth.NewArgon2idHasher(auth.DefaultArgon2idParams())
	if err != nil {
		return err
	}
	authService, err := auth.NewService(storage, hasher, auth.Config{
		BootstrapTokenPath: config.BootstrapTokenPath,
		TOTPKeyPath:        config.TOTPKeyPath,
		SessionTTL:         config.SessionTTL,
		LoginWindow:        config.LoginWindow,
		MaxLoginFailures:   config.MaxLoginFailures,
	})
	if err != nil {
		return err
	}
	recovered, err := authService.RecoverPassword(user.ID, string(password), *disableTOTP)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintln(output, "Password reset completed. All existing sessions were revoked.")
	switch {
	case *disableTOTP:
		_, _ = fmt.Fprintln(output, "Two-factor authentication and recovery codes were disabled as requested.")
	case recovered.TOTPEnabled:
		_, _ = fmt.Fprintln(output, "Two-factor authentication remains enabled.")
	default:
		_, _ = fmt.Fprintln(output, "Two-factor authentication remains disabled.")
	}
	return nil
}

func passwordRecoveryUser(storage *store.Store, username string) (store.User, error) {
	username = strings.TrimSpace(username)
	if username != "" {
		user, err := storage.UserByUsername(username)
		if errors.Is(err, store.ErrNotFound) {
			return store.User{}, fmt.Errorf("administrator %q was not found", username)
		}
		return user, err
	}
	usernames := storage.Usernames()
	switch len(usernames) {
	case 0:
		return store.User{}, errors.New("panel administrator is not initialized")
	case 1:
		return storage.UserByUsername(usernames[0])
	default:
		return store.User{}, errors.New("multiple administrators exist; select one with --username")
	}
}

func clearPassword(password []byte) {
	for index := range password {
		password[index] = 0
	}
}

func runHealthcheck(arguments []string) error {
	flags := flag.NewFlagSet("paneld healthcheck", flag.ContinueOnError)
	configPath := flags.String("config", "", "path to the JSON configuration file")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	config, err := panel.LoadConfig(*configPath)
	if err != nil {
		return err
	}
	address, err := healthcheckAddress(config.Listen)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 4 * time.Second}
	response, err := client.Get("http://" + address + "/api/v1/health")
	if err != nil {
		return fmt.Errorf("healthcheck request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("healthcheck returned HTTP %d", response.StatusCode)
	}
	return nil
}

func runAgentHealthcheck(arguments []string) error {
	flags := flag.NewFlagSet("paneld agent-healthcheck", flag.ContinueOnError)
	configPath := flags.String("config", "", "path to the JSON configuration file")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected agent-healthcheck argument: %s", flags.Arg(0))
	}
	config, err := panel.LoadConfig(*configPath)
	if err != nil {
		return err
	}
	client, err := agentclient.New(config.AgentSocket, config.AgentTokenFile)
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

func healthcheckAddress(listen string) (string, error) {
	if strings.HasPrefix(listen, ":") {
		return "127.0.0.1" + listen, nil
	}
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		return "", fmt.Errorf("parse listen address: %w", err)
	}
	switch host {
	case "", "0.0.0.0", "::":
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port), nil
}
