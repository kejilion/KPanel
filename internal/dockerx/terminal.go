package dockerx

import (
	"context"
	"errors"

	"github.com/kejilion/kejilion-panel/internal/terminal"
)

var ErrContainerTerminalUnsupported = errors.New("container terminals require Linux pidfds and terminal process control")

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
	return c.startContainerTerminal(ctx, id, rows, columns)
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
