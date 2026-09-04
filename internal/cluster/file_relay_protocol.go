package cluster

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

const (
	lightFilePollWait       = 25 * time.Second
	lightFileActivePollWait = 250 * time.Millisecond
	lightFileLiveness       = 2 * time.Minute
	lightFileCommandTTL     = 2 * time.Minute
	lightFileQueueLimit     = 128
	lightFileEventLimit     = 16
	lightFileChunkBytes     = 32 << 10
	lightFileMaxQueryBytes  = 48 << 10
	lightFileMaxHeaders     = 16
)

func validateFileRelayPoll(input FileRelayPollRequest) error {
	if len(input.RequestIDs) > lightFileQueueLimit || len(input.Events) > lightFileEventLimit {
		return ErrAuthentication
	}
	if input.SessionID != "" || input.Command != nil {
		return ErrAuthentication
	}
	seen := make(map[string]struct{}, len(input.RequestIDs))
	for _, id := range input.RequestIDs {
		if !validID(id) {
			return ErrAuthentication
		}
		if _, exists := seen[id]; exists {
			return ErrAuthentication
		}
		seen[id] = struct{}{}
	}
	for _, event := range input.Events {
		if err := validateFileRelayEvent(event); err != nil {
			return err
		}
	}
	return nil
}

func validatePanelFileRelayPoll(input FileRelayPollRequest, now time.Time) error {
	if input.SessionID == "" || !validID(input.SessionID) ||
		len(input.RequestIDs) != 0 || len(input.Events) != 0 {
		return ErrAuthentication
	}
	if input.Command == nil {
		return nil
	}
	if input.Command.RequestID != input.SessionID ||
		validateFileRelayCommand(*input.Command, now.UTC()) != nil {
		return ErrAuthentication
	}
	return nil
}

func validatePanelFileRelayResponse(output FileRelayPollResponse) error {
	if output.Epoch != "" || output.Command != nil || len(output.Events) > 1 {
		return ErrAuthentication
	}
	for _, event := range output.Events {
		if err := validateFileRelayEvent(event); err != nil {
			return err
		}
	}
	return nil
}

func validateFileRelayEvent(event FileRelayEvent) error {
	if !validID(event.RequestID) || len(event.Error) > 512 ||
		(event.Error != "" && cleanDisplayText(event.Error, 512) != event.Error) ||
		event.Offset < 0 || len(event.Data) > lightFileChunkBytes ||
		!validFileRelayHeaders(event.Headers) {
		return ErrAuthentication
	}
	switch event.Kind {
	case "ready", "accepted":
		if event.CommandID == "" || !validID(event.CommandID) || event.Status != 0 || len(event.Data) != 0 || event.Offset != 0 || event.Error != "" {
			return ErrAuthentication
		}
	case "response":
		if event.CommandID != "" || event.Status < 100 || event.Status > 599 || len(event.Data) != 0 || event.Offset != 0 || event.Error != "" {
			return ErrAuthentication
		}
	case "data":
		if event.CommandID != "" || event.Status != 0 || len(event.Data) == 0 || event.Error != "" {
			return ErrAuthentication
		}
	case "end":
		if event.CommandID != "" || event.Status != 0 || len(event.Data) != 0 || event.Error != "" {
			return ErrAuthentication
		}
	case "error":
		if event.CommandID == "" || !validID(event.CommandID) || event.Status != 0 || len(event.Data) != 0 || event.Error == "" {
			return ErrAuthentication
		}
	default:
		return ErrAuthentication
	}
	return nil
}

func validateFileRelayCommand(command FileRelayCommand, now time.Time) error {
	if !validID(command.ID) || !validID(command.RequestID) || command.ExpiresAt <= now.UTC().Unix() ||
		command.ExpiresAt > now.UTC().Add(5*time.Minute).Unix() {
		return ErrAuthentication
	}
	if len(command.Data) > lightFileChunkBytes || command.Offset < 0 || !validFileRelayHeaders(command.Headers) {
		return ErrAuthentication
	}
	switch command.Kind {
	case "request":
		if command.Method != http.MethodGet && command.Method != http.MethodHead && command.Method != http.MethodPost && command.Method != http.MethodPut {
			return ErrAuthentication
		}
		if !v2FileRelayRequestPath(command.Path) || len(command.Query) > lightFileMaxQueryBytes || strings.ContainsAny(command.Query, "\r\n\x00") ||
			command.Offset != 0 || len(command.Data) != 0 || command.Final {
			return ErrAuthentication
		}
		if command.BodyLength < -1 || command.BodyLength > 512<<20 {
			return ErrAuthentication
		}
	case "body":
		if command.Method != "" || command.Path != "" || command.Query != "" || command.BodyLength != 0 || len(command.Headers) != 0 ||
			len(command.Data) == 0 && !command.Final {
			return ErrAuthentication
		}
	case "cancel":
		if command.Method != "" || command.Path != "" || command.Query != "" || command.BodyLength != 0 || command.Offset != 0 || len(command.Data) != 0 || command.Final || len(command.Headers) != 0 {
			return ErrAuthentication
		}
	default:
		return ErrAuthentication
	}
	return nil
}

func validFileRelayHeaders(headers map[string]string) bool {
	if len(headers) > lightFileMaxHeaders {
		return false
	}
	for key, value := range headers {
		if key == "" || len(key) > 128 || len(value) > 4096 || strings.ContainsAny(key+value, "\r\n\x00") {
			return false
		}
	}
	return true
}

func fileRelayPayloadFits(input FileRelayPollRequest) bool {
	payload, err := json.Marshal(input)
	return err == nil && len(payload) <= MaxSummaryBytes
}
