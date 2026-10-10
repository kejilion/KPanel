//go:build !linux

package dockerx

import (
	"context"

	"github.com/kejilion/kejilion-panel/internal/terminal"
)

func (c *Client) startContainerTerminal(context.Context, string, uint16, uint16) (terminal.Process, error) {
	return nil, ErrContainerTerminalUnsupported
}

func (c *Client) recoverContainerTerminals(context.Context) error { return nil }
