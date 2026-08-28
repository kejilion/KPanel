package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/kejilion/kejilion-panel/internal/cluster"
	"github.com/kejilion/kejilion-panel/internal/terminal"
)

const (
	lightTerminalUnsupportedRetry = 5 * time.Minute
	lightTerminalFailureRetry     = 5 * time.Second
	lightTerminalOwnerPrefix      = "light:"
	lightTerminalEventLimit       = 32
	lightTerminalOutputLimit      = 32 << 10
)

type lightNodeTerminalSession struct {
	centralID string
	localID   string
	offset    int64
}

type lightTerminalControl struct {
	config        nodeConfig
	identity      terminalIdentity
	relay         *cluster.TerminalRelayClient
	manager       *terminal.Manager
	sessions      map[string]lightNodeTerminalSession
	pendingEvents []cluster.TerminalRelayEvent
	processed     map[string]cluster.TerminalRelayEvent
	processedIDs  []string
	centerEpoch   string
}

func runLightTerminalControl(
	ctx context.Context,
	config nodeConfig,
	identity terminalIdentity,
	relay *cluster.TerminalRelayClient,
	manager *terminal.Manager,
) {
	if manager == nil || relay == nil {
		return
	}
	control := &lightTerminalControl{
		config: config, identity: identity, relay: relay, manager: manager,
		sessions:  make(map[string]lightNodeTerminalSession),
		processed: make(map[string]cluster.TerminalRelayEvent),
	}
	unsupportedUntil := time.Time{}
	for {
		if ctx.Err() != nil {
			return
		}
		if time.Now().Before(unsupportedUntil) {
			if !waitContext(ctx, time.Until(unsupportedUntil)) {
				return
			}
			continue
		}

		events, _ := control.collectEvents(ctx)
		request := cluster.TerminalRelayPollRequest{SessionIDs: control.sessionIDs(), Events: events}
		response, err := relay.PollV2(
			ctx, config.Origin, config.NodeID, config.TargetNodeID,
			identity.Key, identity.Peer, time.Now().UTC(), request,
		)
		if err != nil {
			if terminalRelayUnsupported(err) {
				unsupportedUntil = time.Now().Add(lightTerminalUnsupportedRetry)
			} else {
				slog.Warn("lightweight terminal relay failed", "error", err)
				if !waitContext(ctx, lightTerminalFailureRetry) {
					return
				}
			}
			continue
		}
		control.acceptRelayResponse(response)
		if response.Command == nil {
			continue
		}
		if err := control.executeCommand(*response.Command); err != nil {
			control.queueCommandError(*response.Command, err)
		}
	}
}

func (control *lightTerminalControl) acceptRelayResponse(response cluster.TerminalRelayPollResponse) {
	control.acknowledgeEvents()
	if response.Epoch == "" {
		return
	}
	if control.centerEpoch != "" && control.centerEpoch != response.Epoch {
		control.resetSessions()
	}
	control.centerEpoch = response.Epoch
}

func (control *lightTerminalControl) resetSessions() {
	owner := lightTerminalOwner(control.config.NodeID)
	for _, item := range control.sessions {
		_ = control.manager.Close(owner, item.localID)
	}
	control.sessions = make(map[string]lightNodeTerminalSession)
	control.pendingEvents = nil
	control.processed = make(map[string]cluster.TerminalRelayEvent)
	control.processedIDs = nil
}

func terminalRelayUnsupported(err error) bool {
	var remoteErr *cluster.RemoteError
	if !errors.As(err, &remoteErr) {
		return false
	}
	return remoteErr.StatusCode == 404 || remoteErr.StatusCode == 405 || remoteErr.StatusCode == 426
}

func (control *lightTerminalControl) collectEvents(ctx context.Context) ([]cluster.TerminalRelayEvent, error) {
	if len(control.pendingEvents) >= lightTerminalEventLimit {
		return append([]cluster.TerminalRelayEvent(nil), control.pendingEvents...), nil
	}
	ids := make([]string, 0, len(control.sessions))
	for id := range control.sessions {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var firstErr error
	for _, centralID := range ids {
		if len(control.pendingEvents) >= lightTerminalEventLimit {
			break
		}
		if hasPendingTerminalRelayOutput(control.pendingEvents, centralID) {
			continue
		}
		item := control.sessions[centralID]
		output, err := control.manager.Output(ctx, lightTerminalOwner(control.config.NodeID), item.localID, item.offset, 0)
		if err != nil {
			if errors.Is(err, terminal.ErrNotFound) || errors.Is(err, terminal.ErrClosed) {
				if control.queueEvent(cluster.TerminalRelayEvent{
					Kind: "output", SessionID: centralID, Offset: item.offset, NextOffset: item.offset, Closed: true,
				}) {
					delete(control.sessions, centralID)
				}
				continue
			}
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if len(output.Data) == 0 && output.ExitedAt == nil && !output.Closed {
			continue
		}
		availableNext := output.NextOffset
		if len(output.Data) > lightTerminalOutputLimit {
			output.Data = output.Data[:lightTerminalOutputLimit]
			output.NextOffset = output.Offset + int64(len(output.Data))
		}
		event := cluster.TerminalRelayEvent{
			Kind: "output", SessionID: centralID, Offset: output.Offset,
			NextOffset: output.NextOffset, Data: append([]byte(nil), output.Data...),
			Truncated: output.Truncated, ExitedAt: output.ExitedAt,
			ExitError: output.ExitError, Closed: output.Closed,
		}
		var fits bool
		if event, fits = control.fitOutputEvent(event); !fits {
			if firstErr == nil {
				firstErr = errors.New("terminal relay output exceeds the v2 payload limit")
			}
			continue
		}
		if event.NextOffset < availableNext {
			// The manager still has unread bytes. Do not mark the session as
			// exited/closed until a later poll has delivered the remaining tail.
			event.ExitedAt = nil
			event.ExitError = ""
			event.Closed = false
		}
		if !control.queueEvent(event) {
			continue
		}
		item.offset = event.NextOffset
		control.sessions[centralID] = item
		if event.ExitedAt != nil || event.Closed {
			delete(control.sessions, centralID)
		}
	}
	return append([]cluster.TerminalRelayEvent(nil), control.pendingEvents...), firstErr
}

func (control *lightTerminalControl) sessionIDs() []string {
	ids := make([]string, 0, len(control.sessions))
	for centralID := range control.sessions {
		ids = append(ids, centralID)
	}
	sort.Strings(ids)
	return ids
}

func hasPendingTerminalRelayOutput(events []cluster.TerminalRelayEvent, sessionID string) bool {
	for _, event := range events {
		if event.Kind == "output" && event.SessionID == sessionID {
			return true
		}
	}
	return false
}

func (control *lightTerminalControl) acknowledgeEvents() {
	control.pendingEvents = nil
}

func (control *lightTerminalControl) executeCommand(command cluster.TerminalRelayCommand) error {
	if event, ok := control.processed[command.ID]; ok {
		control.queueEvent(event)
		return nil
	}
	owner := lightTerminalOwner(control.config.NodeID)
	switch command.Path {
	case cluster.V2TerminalOpenPath:
		var input cluster.TerminalOpenRequest
		if err := decodeTerminalRelayPayload(command.Payload, &input); err != nil {
			return err
		}
		snapshot, err := control.manager.Open(owner, input.Rows, input.Columns)
		if err != nil {
			return err
		}
		control.sessions[command.SessionID] = lightNodeTerminalSession{
			centralID: command.SessionID, localID: snapshot.ID, offset: snapshot.Offset,
		}
		event := cluster.TerminalRelayEvent{CommandID: command.ID, Kind: "opened", SessionID: command.SessionID, Offset: snapshot.Offset}
		if !control.queueEvent(event) {
			delete(control.sessions, command.SessionID)
			_ = control.manager.Close(owner, snapshot.ID)
			return errors.New("terminal relay event queue is full")
		}
		control.rememberCommandResult(command.ID, event)
	case cluster.V2TerminalInputPath:
		var input cluster.TerminalInputRequest
		if err := decodeTerminalRelayPayload(command.Payload, &input); err != nil {
			return err
		}
		data, err := decodeTerminalRelayData(input.Data)
		if err != nil {
			return err
		}
		item, ok := control.sessions[command.SessionID]
		if !ok {
			return terminal.ErrNotFound
		}
		if err := control.manager.Input(owner, item.localID, data); err != nil {
			return err
		}
		event := cluster.TerminalRelayEvent{CommandID: command.ID, Kind: "accepted", SessionID: command.SessionID}
		control.rememberCommandResult(command.ID, event)
		control.queueEvent(event)
	case cluster.V2TerminalResizePath:
		var input cluster.TerminalResizeRequest
		if err := decodeTerminalRelayPayload(command.Payload, &input); err != nil {
			return err
		}
		item, ok := control.sessions[command.SessionID]
		if !ok {
			return terminal.ErrNotFound
		}
		if err := control.manager.Resize(owner, item.localID, input.Rows, input.Columns); err != nil {
			return err
		}
		event := cluster.TerminalRelayEvent{CommandID: command.ID, Kind: "accepted", SessionID: command.SessionID}
		control.rememberCommandResult(command.ID, event)
		control.queueEvent(event)
	case cluster.V2TerminalClosePath:
		var input cluster.TerminalCloseRequest
		if err := decodeTerminalRelayPayload(command.Payload, &input); err != nil {
			return err
		}
		item, ok := control.sessions[command.SessionID]
		if ok {
			err := control.manager.Close(owner, item.localID)
			if err != nil && !errors.Is(err, terminal.ErrNotFound) && !errors.Is(err, terminal.ErrClosed) {
				return err
			}
			delete(control.sessions, command.SessionID)
		}
		event := cluster.TerminalRelayEvent{CommandID: command.ID, Kind: "closed", SessionID: command.SessionID}
		control.rememberCommandResult(command.ID, event)
		control.queueEvent(event)
	default:
		return errors.New("unsupported terminal relay command")
	}
	return nil
}

func decodeTerminalRelayPayload(payload json.RawMessage, target any) error {
	if len(payload) == 0 || target == nil {
		return errors.New("terminal relay payload is invalid")
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return errors.New("terminal relay payload is invalid")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("terminal relay payload is invalid")
	}
	return nil
}

func decodeTerminalRelayData(value string) ([]byte, error) {
	data, err := base64.RawStdEncoding.DecodeString(value)
	if err == nil {
		return data, nil
	}
	return base64.StdEncoding.DecodeString(value)
}

func (control *lightTerminalControl) queueCommandError(command cluster.TerminalRelayCommand, err error) {
	message := strings.TrimSpace(err.Error())
	if message == "" {
		message = "terminal relay command failed"
	}
	if len(message) > 512 {
		message = message[:512]
	}
	event := cluster.TerminalRelayEvent{
		CommandID: command.ID, Kind: "error", SessionID: command.SessionID, Error: message,
	}
	control.rememberCommandResult(command.ID, event)
	control.queueEvent(event)
}

func (control *lightTerminalControl) rememberCommandResult(commandID string, event cluster.TerminalRelayEvent) {
	if control.processed == nil {
		control.processed = make(map[string]cluster.TerminalRelayEvent)
	}
	if _, exists := control.processed[commandID]; exists {
		return
	}
	control.processed[commandID] = event
	control.processedIDs = append(control.processedIDs, commandID)
	if len(control.processedIDs) <= 128 {
		return
	}
	delete(control.processed, control.processedIDs[0])
	control.processedIDs = control.processedIDs[1:]
}

func (control *lightTerminalControl) queueEvent(event cluster.TerminalRelayEvent) bool {
	if len(control.pendingEvents) < lightTerminalEventLimit {
		candidate := append(append([]cluster.TerminalRelayEvent(nil), control.pendingEvents...), event)
		if !control.pollPayloadFits(candidate) {
			return false
		}
		control.pendingEvents = append(control.pendingEvents, event)
		return true
	}
	if event.CommandID == "" {
		return false
	}
	for index, pending := range control.pendingEvents {
		if pending.CommandID != "" {
			continue
		}
		candidate := make([]cluster.TerminalRelayEvent, 0, len(control.pendingEvents))
		candidate = append(candidate, control.pendingEvents[:index]...)
		candidate = append(candidate, control.pendingEvents[index+1:]...)
		candidate = append(candidate, event)
		if !control.pollPayloadFits(candidate) {
			return false
		}
		copy(control.pendingEvents[index:], control.pendingEvents[index+1:])
		control.pendingEvents[len(control.pendingEvents)-1] = event
		return true
	}
	return false
}

func (control *lightTerminalControl) fitOutputEvent(event cluster.TerminalRelayEvent) (cluster.TerminalRelayEvent, bool) {
	if control.pollPayloadFits(append(append([]cluster.TerminalRelayEvent(nil), control.pendingEvents...), event)) {
		return event, true
	}
	if len(event.Data) == 0 {
		return cluster.TerminalRelayEvent{}, false
	}
	low, high := 1, len(event.Data)
	best := 0
	for low <= high {
		middle := low + (high-low)/2
		candidate := event
		candidate.Data = event.Data[:middle]
		candidate.NextOffset = candidate.Offset + int64(middle)
		if control.pollPayloadFits(append(append([]cluster.TerminalRelayEvent(nil), control.pendingEvents...), candidate)) {
			best = middle
			low = middle + 1
			continue
		}
		high = middle - 1
	}
	if best == 0 {
		return cluster.TerminalRelayEvent{}, false
	}
	event.Data = event.Data[:best]
	event.NextOffset = event.Offset + int64(best)
	return event, true
}

func (control *lightTerminalControl) pollPayloadFits(events []cluster.TerminalRelayEvent) bool {
	payload, err := json.Marshal(cluster.TerminalRelayPollRequest{
		SessionIDs: control.sessionIDs(), Events: events,
	})
	return err == nil && len(payload) <= cluster.MaxSummaryBytes
}

func lightTerminalOwner(nodeID string) string {
	return lightTerminalOwnerPrefix + nodeID
}
