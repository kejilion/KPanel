package dockerx

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/kejilion/kejilion-panel/internal/terminal"
)

var ErrContainerTerminalUnsupported = errors.New("container terminals require Linux pidfds and terminal process control")
var ErrContainerShellUnavailable = errors.New("container does not contain the required /bin/sh")
var errContainerShellStart = errors.New("container shell failed to start")

// OpenContainerTerminal binds a persistent PTY to the exact container snapshot.
// Commands go to this container's shell, never to a host shell.
func (c *Client) OpenContainerTerminal(ctx context.Context, id, resourceVersion string, rows, columns uint16) (terminal.Process, error) {
	if !dockerExecIDPattern.MatchString(id) || resourceVersion == "" || rows == 0 || columns == 0 || rows > 500 || columns > 1000 {
		return nil, ErrInvalidDockerExec
	}
	inspect, err := c.inspectVerifiedContainer(ctx, id, resourceVersion)
	if err != nil {
		return nil, err
	}
	if !inspect.State.Running || inspect.State.Paused || inspect.State.Restarting {
		return nil, ErrActionUnsupported
	}
	if err := c.RecoverContainerTerminals(ctx); err != nil {
		return nil, err
	}
	process, err := c.startContainerTerminal(ctx, id, rows, columns)
	if process == nil {
		err = c.containerTerminalStartError(ctx, id, resourceVersion, err)
	}
	return process, err
}

// Diagnose only a failed shell launch after its exec has been reclaimed. A
// missing file is distinct from permissions, a broken loader, or a transport
// failure; successful terminals need no additional Docker requests.
func (c *Client) containerTerminalStartError(ctx context.Context, id, resourceVersion string, startErr error) error {
	if !errors.Is(startErr, errContainerShellStart) {
		return startErr
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	_, _, err := c.nginxDockerRequest(ctx, http.MethodHead, "/containers/"+id+"/archive?path=%2Fbin%2Fsh", nil, 4<<10)
	if !isDockerStatus(err, http.StatusNotFound) {
		return startErr
	}
	// HEAD also returns 404 when the container disappeared. Revalidate the
	// exact snapshot before attributing the failure to its filesystem.
	inspect, err := c.inspectVerifiedContainer(ctx, id, resourceVersion)
	if err != nil || !inspect.State.Running || inspect.State.Paused || inspect.State.Restarting {
		return startErr
	}
	return ErrContainerShellUnavailable
}

// RecoverContainerTerminals completes once before accepting new sessions.
// Recovery failures keep their records and fail closed without affecting other
// Docker management functions or existing host terminals.
func (c *Client) RecoverContainerTerminals(ctx context.Context) error {
	c.terminalRecoveryMu.Lock()
	defer c.terminalRecoveryMu.Unlock()
	if c.terminalRecoveryDone {
		return nil
	}
	if err := c.recoverContainerTerminals(ctx); err != nil {
		return err
	}
	c.terminalRecoveryDone = true
	return nil
}
