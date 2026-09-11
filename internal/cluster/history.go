package cluster

import (
	"context"
	"crypto/ed25519"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/flynn/noise"
	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/monitoring"
)

const (
	HistoryPath        = "/v1/monitoring/history"
	HistoryV1Path      = "/api/v1/federation/monitoring/history"
	HistoryV2Path      = "/api/v2/federation/monitoring/history"
	HistoryRelayV2Path = "/api/v2/federation/monitoring/relay"
	HistoryTimeout     = 2 * time.Minute
)

var ErrHistoryUnsupported = errors.New("remote history is not supported")
var ErrHistoryUnavailable = errors.New("remote history is unavailable")

type remoteHistoryV1API interface {
	OpenHistoryV1(context.Context, string, string, string, ed25519.PrivateKey, time.Time, monitoring.Query) (*http.Response, error)
}
type remoteHistoryV2API interface {
	OpenHistoryV2(context.Context, string, string, string, noise.DHKey, []byte, time.Time, monitoring.Query) (*http.Response, error)
}

func historyScopeAllowed(scope string) bool {
	return scope == SummaryScope || scope == SummaryTerminalScope || scope == SummaryTerminalFilesScope
}

// History reads the selected node's own bounded history. The center never
// substitutes a summary sample or another host when a query fails.
func (s *Service) History(ctx context.Context, id, requestedRange string, start, end time.Time) (contract.MonitoringHistory, error) {
	var result contract.MonitoringHistory
	query := monitoring.Query{Range: requestedRange, Start: start, End: end, Gzip: true}
	if err := query.Validate(); err != nil {
		return result, err
	}
	if !validID(id) {
		return result, ErrNotFound
	}
	host, err := s.Host(ctx, id)
	if err != nil {
		return result, err
	}
	select {
	case s.historyQueries <- struct{}{}:
		defer func() { <-s.historyQueries }()
	default:
		return result, monitoring.ErrBusy
	}
	ctx, cancel := context.WithTimeout(ctx, HistoryTimeout)
	defer cancel()
	response, err := s.openHistory(ctx, host, query)
	if err != nil {
		return result, err
	}
	defer response.Body.Close()
	stop := context.AfterFunc(ctx, func() { _ = response.Body.Close() })
	defer stop()
	if err := historyResponseStatus(response.StatusCode); err != nil {
		return result, err
	}
	contentType := strings.ToLower(strings.TrimSpace(strings.SplitN(response.Header.Get("Content-Type"), ";", 2)[0]))
	compressed := contentType == monitoring.GzipHistoryContentType
	if contentType != "application/json" && !compressed {
		return result, ErrHistoryUnavailable
	}
	result, err = decodeHistoryPayload(response.Body, query, compressed)
	if err != nil || ctx.Err() != nil {
		return contract.MonitoringHistory{}, ErrHistoryUnavailable
	}
	// Deletion during an in-flight query must not return data from a removed node.
	if _, err := s.Host(ctx, id); err != nil {
		return contract.MonitoringHistory{}, err
	}
	return result, nil
}

func (s *Service) openHistory(ctx context.Context, host Host, query monitoring.Query) (*http.Response, error) {
	if host.Kind == HostKindLightNode {
		if _, err := s.light.Host(host.ID); err != nil {
			return nil, ErrNotFound
		}
		if !s.lightHistory.available(host.ID) {
			return nil, ErrHistoryUnavailable
		}
		return s.lightHistory.Open(ctx, host.ID, LightFileRequest{Method: http.MethodGet, Path: HistoryPath, RawQuery: query.Encode(), Body: http.NoBody})
	}
	if host.FederationProtocol == FederationProtocolV2 {
		record, err := s.storeV2.Host(host.ID)
		if err != nil || record.State != hostStateV2Active || (record.Scope != "" && !historyScopeAllowed(record.Scope)) {
			return nil, ErrAuthentication
		}
		credential, err := s.secretsV2.ReadCredential(record.CredentialFile)
		if err != nil {
			return nil, err
		}
		remote, ok := s.remoteV2.(remoteHistoryV2API)
		if !ok {
			return nil, ErrHistoryUnsupported
		}
		return remote.OpenHistoryV2(ctx, record.Origin, record.ControllerID, record.RemoteNodeID, noiseKeyV2(credential), credential.TargetPublic, s.now().UTC(), query)
	}
	record, err := s.store.Host(host.ID)
	if err != nil {
		return nil, err
	}
	key, err := s.secrets.Read(record.CredentialFile)
	if err != nil {
		return nil, err
	}
	remote, ok := s.remote.(remoteHistoryV1API)
	if !ok {
		return nil, ErrHistoryUnsupported
	}
	return remote.OpenHistoryV1(ctx, record.Origin, record.ControllerID, record.RemoteNodeID, key, s.now().UTC(), query)
}

func historyResponseStatus(status int) error {
	switch status {
	case http.StatusOK:
		return nil
	case http.StatusNotFound, http.StatusMethodNotAllowed, http.StatusUpgradeRequired:
		return ErrHistoryUnsupported
	case http.StatusTooManyRequests:
		return monitoring.ErrBusy
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		return monitoring.ErrInvalidWindow
	case http.StatusUnauthorized, http.StatusForbidden:
		return ErrAuthentication
	default:
		return ErrHistoryUnavailable
	}
}

func validHistoryResponse(value contract.MonitoringHistory, query monitoring.Query) bool {
	wantRange := query.Range
	if wantRange == "" {
		wantRange = "24h"
	}
	if value.Range != wantRange || value.BucketSeconds < 1 || !value.StartedAt.Before(value.EndedAt) ||
		value.Host == nil || value.Containers == nil ||
		len(value.Host) > 720 || len(value.Containers) > 32 || len(value.OperatorLatency) > 9 {
		return false
	}
	for _, series := range value.Containers {
		if series.Points == nil || len(series.Points) > 720 || len(series.ContainerID) > 128 || len(series.Name) > 256 || len(series.Image) > 256 {
			return false
		}
	}
	for _, series := range value.OperatorLatency {
		if len(series.Points) > 720 || len(series.ID) > 128 || len(series.Address) > 256 {
			return false
		}
	}
	return true
}

func validHistoryRelayRequest(input LightFileRequest) bool {
	if input.Path != HistoryPath || input.Method != http.MethodGet || input.BodyLength != 0 || len(input.Headers) != 0 {
		return false
	}
	_, err := monitoring.ParseQuery(input.RawQuery)
	return err == nil
}

func newLightHistoryRelay(now func() time.Time) *lightFileRelay {
	relay := newLightFileRelay(now)
	relay.history = true
	return relay
}

// NewHistoryRelayClient reuses Noise framing with its own endpoint and
// history-only command validator. File relay clients remain files-only.
func NewHistoryRelayClient(client *http.Client) (*FileRelayClient, error) {
	relay, err := NewFileRelayClient(client)
	if err == nil {
		relay.history = true
	}
	return relay, err
}

func validHistoryRelayCommand(command FileRelayCommand, now time.Time) error {
	if command.Kind == "cancel" {
		return validateFileRelayCommand(command, now)
	}
	if command.Kind != "request" || !validID(command.ID) || !validID(command.RequestID) || command.ExpiresAt <= now.Unix() || command.ExpiresAt > now.Add(5*time.Minute).Unix() || command.Offset != 0 || len(command.Data) != 0 || command.Final {
		return ErrAuthentication
	}
	if !validHistoryRelayRequest(LightFileRequest{Method: command.Method, Path: command.Path, RawQuery: command.Query, Headers: command.Headers, BodyLength: command.BodyLength}) {
		return ErrAuthentication
	}
	return nil
}
