//go:build !windows

package main

import (
	"context"
	"github.com/kejilion/kejilion-panel/internal/cluster/sshlogin"
	"github.com/kejilion/kejilion-panel/internal/contract"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

const defaultConfigPath = "/etc/kejilion-node/node.json"
const defaultTerminalConfigPath = "/etc/kejilion-node/terminal.json"
const updateHealthPath = "/etc/kejilion-node/update-status.json"
const nodeCheckStatusPath = "/run/kejilion-node-monitoring/check-status.json"

func runPlatformCommand([]string) (bool, error)                           { return false, nil }
func requireEnrollmentIdentity() error                                    { return nil }
func requireEnrollmentCenter(http.Header) error                           { return nil }
func requireBrokerIdentity() error                                        { return nil }
func enrollmentCapabilities(string) ([]string, error)                     { return nil, nil }
func requireWindowsCapability(context.Context, nodeConfig, string) error  { return nil }
func platformTerminalEnabled(nodeConfig) bool                             { return true }
func requirePlatformBrokerCapabilities(context.Context, nodeConfig) error { return nil }
func startPlatformDesktop(context.Context, nodeConfig) func()             { return func() {} }
func startPlatformHealthPublisher(context.Context) func()                 { return func() {} }
func applyPlatformReport(*reportRequest, nodeConfig)                      {}
func validatePlatformConfig(*os.File, bool) error                         { return nil }
func writePlatformConfig(string, any, bool) (bool, error)                 { return false, nil }
func platformHealth(context.Context) *contract.LightNodeHealth            { return nil }
func platformStateDirectory() string                                      { return "" }
func nodeSignalContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}

func latestPlatformLogin(ctx context.Context, reader *sshlogin.Reader) (*contract.SSHLoginEvent, error) {
	return reader.LatestSSHLogin(ctx)
}
