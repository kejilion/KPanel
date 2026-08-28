package cluster

import (
	"encoding/base64"
	"strings"
	"time"

	"github.com/kejilion/kejilion-panel/internal/terminal"
)

const (
	lightTerminalPollWait       = 25 * time.Second
	lightTerminalActivePollWait = 750 * time.Millisecond
	lightTerminalLiveness       = 2 * time.Minute
	lightTerminalCommandTTL     = 2 * time.Minute
	lightTerminalQueueLimit     = 128
	lightTerminalEventLimit     = 32
	lightTerminalBufferBytes    = terminal.DefaultBufferBytes
)

func validateTerminalRelayPoll(input TerminalRelayPollRequest) error {
	if len(input.SessionIDs) > terminal.DefaultMaxOwnerSessions || len(input.Events) > lightTerminalEventLimit {
		return ErrAuthentication
	}
	sessionIDs := make(map[string]struct{}, len(input.SessionIDs))
	for _, sessionID := range input.SessionIDs {
		if !validID(sessionID) {
			return ErrAuthentication
		}
		if _, exists := sessionIDs[sessionID]; exists {
			return ErrAuthentication
		}
		sessionIDs[sessionID] = struct{}{}
	}
	for _, event := range input.Events {
		if !validID(event.SessionID) || event.Offset < 0 || event.NextOffset < event.Offset ||
			len(event.Error) > 512 || len(event.ExitError) > 512 ||
			(event.Error != "" && cleanDisplayText(event.Error, 512) != event.Error) ||
			(event.ExitError != "" && cleanDisplayText(event.ExitError, 512) != event.ExitError) {
			return ErrAuthentication
		}
		if event.CommandID != "" && !validID(event.CommandID) {
			return ErrAuthentication
		}
		switch event.Kind {
		case "opened":
			if event.CommandID == "" || event.Offset != 0 || event.NextOffset != 0 || len(event.Data) != 0 || event.Error != "" {
				return ErrAuthentication
			}
		case "accepted":
			if event.CommandID == "" || event.Offset != 0 || event.NextOffset != 0 || len(event.Data) != 0 || event.Error != "" {
				return ErrAuthentication
			}
		case "closed":
			if event.CommandID == "" || event.Offset != 0 || event.NextOffset != 0 || len(event.Data) != 0 || event.Error != "" {
				return ErrAuthentication
			}
		case "error":
			if event.CommandID == "" || event.Error == "" || len(event.Data) != 0 {
				return ErrAuthentication
			}
		case "output":
			if event.CommandID != "" || len(event.Data) > maxFederationTerminalOutputBytes || event.Error != "" ||
				event.NextOffset-event.Offset != int64(len(event.Data)) {
				return ErrAuthentication
			}
		default:
			return ErrAuthentication
		}
	}
	return nil
}

func validateTerminalRelayCommand(command TerminalRelayCommand, now time.Time) error {
	if !validID(command.ID) || !validID(command.SessionID) || len(command.Payload) == 0 || len(command.Payload) > MaxSummaryBytes {
		return ErrAuthentication
	}
	if command.ExpiresAt <= now.UTC().Unix() || command.ExpiresAt > now.UTC().Add(5*time.Minute).Unix() {
		return ErrAuthentication
	}
	switch command.Path {
	case v2TerminalOpenPath:
		var input TerminalOpenRequest
		if decodeV2Payload(command.Payload, &input) != nil || input.Rows == 0 || input.Columns == 0 || input.Rows > 500 || input.Columns > 1000 || command.SessionID == "" {
			return ErrAuthentication
		}
	case v2TerminalInputPath:
		var input TerminalInputRequest
		if decodeV2Payload(command.Payload, &input) != nil || input.SessionID != command.SessionID {
			return ErrAuthentication
		}
		data, err := decodeTerminalPayload(input.Data)
		if err != nil || len(data) == 0 || len(data) > terminal.MaxInputBytes {
			return ErrAuthentication
		}
	case v2TerminalResizePath:
		var input TerminalResizeRequest
		if decodeV2Payload(command.Payload, &input) != nil || input.SessionID != command.SessionID || input.Rows == 0 || input.Columns == 0 || input.Rows > 500 || input.Columns > 1000 {
			return ErrAuthentication
		}
	case v2TerminalClosePath:
		var input TerminalCloseRequest
		if decodeV2Payload(command.Payload, &input) != nil || input.SessionID != command.SessionID {
			return ErrAuthentication
		}
	default:
		return ErrAuthentication
	}
	return nil
}

func decodeTerminalRelayPublicKey(value string) ([]byte, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(value))
	if err != nil || len(decoded) != 32 {
		return nil, ErrAuthentication
	}
	return decoded, nil
}
