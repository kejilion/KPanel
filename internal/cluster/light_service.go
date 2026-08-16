package cluster

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	lightTokenPrefix    = "kpl1."
	lightEnrollPath     = "/api/v3/federation/light/enroll"
	lightReportPath     = "/api/v3/federation/light/report"
	lightReportInterval = 30
	maxLightTokenAge    = time.Hour
)

type lightTokenWire struct {
	Version   int    `json:"v"`
	Origin    string `json:"origin"`
	ID        string `json:"id"`
	Secret    string `json:"secret"`
	ExpiresAt int64  `json:"expiresAt"`
}

type LightReportAuth struct {
	Source    string
	NodeID    string
	Timestamp string
	RequestID string
	Signature string
}

func (s *Service) CreateLightEnrollment() (LightEnrollment, error) {
	return s.CreateLightEnrollmentForOrigin(s.publicURL)
}

func (s *Service) CreateLightEnrollmentForOrigin(origin string) (LightEnrollment, error) {
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	origin, err := validateLightOrigin(origin)
	if err != nil {
		return LightEnrollment{}, ErrLightHTTPSOrigin
	}
	now := s.now().UTC()
	id, err := randomHex(16)
	if err != nil {
		return LightEnrollment{}, err
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return LightEnrollment{}, err
	}
	expiresAt := now.Add(5 * time.Minute)
	hash := sha256.Sum256(secret)
	if err := s.light.AddEnrollment(lightEnrollmentRecord{
		ID: id, SecretHash: hex.EncodeToString(hash[:]), ExpiresAt: expiresAt,
	}, now); err != nil {
		return LightEnrollment{}, err
	}
	wire, err := json.Marshal(lightTokenWire{
		Version: 1, Origin: origin, ID: id,
		Secret: base64.RawURLEncoding.EncodeToString(secret), ExpiresAt: expiresAt.Unix(),
	})
	if err != nil {
		return LightEnrollment{}, err
	}
	token := lightTokenPrefix + base64.RawURLEncoding.EncodeToString(wire)
	command := "bash <(curl -fsSL https://kejilion.sh) kpanel node join '" + token + "'"
	return LightEnrollment{Command: command, ExpiresAt: expiresAt}, nil
}

func (s *Service) EnrollLightNode(source string, input LightEnrollRequest) (LightEnrollResponse, error) {
	return s.EnrollLightNodeAtOrigin(source, s.publicURL, input)
}

func (s *Service) EnrollLightNodeAtOrigin(
	source string,
	origin string,
	input LightEnrollRequest,
) (LightEnrollResponse, error) {
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	now := s.now().UTC()
	if !s.lightEnrolls.Allow(cleanRateSubject(source), now) {
		return LightEnrollResponse{}, ErrRateLimited
	}
	wire, secret, err := parseLightToken(input.Token, now)
	validatedOrigin, originErr := validateLightOrigin(origin)
	if err != nil || originErr != nil || wire.Origin != validatedOrigin {
		return LightEnrollResponse{}, ErrPairingCode
	}
	name, err := validateOptionalName(input.Name)
	if err != nil {
		return LightEnrollResponse{}, err
	}
	if len(s.store.Hosts())+len(s.storeV2.Hosts())+len(s.light.Hosts()) >= MaxHosts {
		return LightEnrollResponse{}, ErrHostLimit
	}
	nodeID, err := randomHex(16)
	if err != nil {
		return LightEnrollResponse{}, err
	}
	reportingKey := make([]byte, 32)
	if _, err := rand.Read(reportingKey); err != nil {
		return LightEnrollResponse{}, err
	}
	if name == "" {
		name = "轻量节点 " + nodeID[:8]
	}
	hash := sha256.Sum256(secret)
	if err := s.light.EnrollHost(wire.ID, hex.EncodeToString(hash[:]), lightHostRecord{
		ID: nodeID, Name: name, NodeVersion: cleanDisplayText(input.NodeVersion, 64),
		CreatedAt: now, UpdatedAt: now,
	}, reportingKey, now); err != nil {
		return LightEnrollResponse{}, err
	}
	return LightEnrollResponse{
		NodeID: nodeID, ReportingKey: base64.RawURLEncoding.EncodeToString(reportingKey),
		ReportInterval: lightReportInterval,
	}, nil
}

func (s *Service) AcceptLightReport(auth LightReportAuth, rawBody []byte, input LightReportRequest) (LightReportResponse, error) {
	now := s.now().UTC()
	if !s.lightSources.Allow(cleanRateSubject(auth.Source), now) {
		return LightReportResponse{}, ErrRateLimited
	}
	if !validID(auth.NodeID) || !validNonce(auth.RequestID) || len(rawBody) == 0 || len(rawBody) > MaxSummaryBytes {
		return LightReportResponse{}, ErrAuthentication
	}
	record, err := s.light.Host(auth.NodeID)
	if err != nil {
		return LightReportResponse{}, ErrAuthentication
	}
	timestamp, err := strconv.ParseInt(auth.Timestamp, 10, 64)
	if err != nil || absDuration(now.Sub(time.Unix(timestamp, 0).UTC())) > 2*time.Minute {
		return LightReportResponse{}, ErrAuthentication
	}
	secret, err := s.light.ReadSecret(record)
	if err != nil {
		return LightReportResponse{}, ErrAuthentication
	}
	bodyHash := sha256.Sum256(rawBody)
	material := strings.Join([]string{"POST", lightReportPath, auth.NodeID, auth.Timestamp, auth.RequestID, hex.EncodeToString(bodyHash[:])}, "\n")
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(material))
	expected := mac.Sum(nil)
	provided, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(auth.Signature))
	if err != nil || len(provided) != len(expected) || subtle.ConstantTimeCompare(provided, expected) != 1 {
		return LightReportResponse{}, ErrAuthentication
	}
	if !s.lightReports.Allow(auth.NodeID, now) {
		return LightReportResponse{}, ErrRateLimited
	}
	if err := s.replays.Accept(auth.NodeID, auth.RequestID, now); err != nil {
		return LightReportResponse{}, err
	}
	if err := validateTelemetry(input.Telemetry, now); err != nil {
		return LightReportResponse{}, err
	}
	snapshot := HostSnapshot{Telemetry: cloneTelemetry(input.Telemetry), ReceivedAt: now}
	if _, err := s.light.UpdateReport(auth.NodeID, snapshot, input.Telemetry.AgentVersion, now); err != nil {
		return LightReportResponse{}, err
	}
	return LightReportResponse{AcceptedAt: now, NextReport: lightReportInterval}, nil
}

func (s *Service) deleteLightHostLocked(id string, input DeleteHostInput) (DeleteHostResult, error) {
	_, credentialRemoved, err := s.light.DeleteHost(id, input.ExpectedResourceVersion)
	if err != nil {
		return DeleteHostResult{}, err
	}
	return DeleteHostResult{Deleted: true, CredentialRemoved: credentialRemoved}, nil
}

func publicLightHost(record lightHostRecord, now time.Time) Host {
	state := HostUnknown
	if record.LastSnapshot != nil && record.LastSuccessAt != nil {
		since := now.Sub(record.LastSuccessAt.UTC())
		switch {
		case since <= 90*time.Second:
			state = HostOnline
		case since <= 5*time.Minute:
			state = HostStale
		default:
			state = HostOffline
		}
	}
	return Host{
		ID: record.ID, Name: record.Name, Kind: HostKindLightNode,
		TransportSecurity: TransportSecurityTLS, RemoteNodeID: record.ID,
		FederationProtocol: LightNodeProtocol, PanelVersion: record.NodeVersion,
		Scope: SummaryScope, TerminalAvailable: false, BrowseAvailable: false, BrowseWSAvailable: false,
		State: state, LastSnapshot: cloneSnapshot(record.LastSnapshot),
		LastAttemptAt: cloneTime(record.LastAttemptAt), LastSuccessAt: cloneTime(record.LastSuccessAt),
		ConsecutiveFailures: record.ConsecutiveFailures, LastErrorCode: record.LastErrorCode,
		LastError: record.LastError, ResourceVersion: record.ResourceVersion,
		CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}
}

func parseLightToken(token string, now time.Time) (lightTokenWire, []byte, error) {
	token = strings.TrimSpace(token)
	if !strings.HasPrefix(token, lightTokenPrefix) || len(token) > 2048 {
		return lightTokenWire{}, nil, ErrPairingCode
	}
	content, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(token, lightTokenPrefix))
	if err != nil || len(content) > 1536 {
		return lightTokenWire{}, nil, ErrPairingCode
	}
	var wire lightTokenWire
	decoder := json.NewDecoder(strings.NewReader(string(content)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return lightTokenWire{}, nil, ErrPairingCode
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return lightTokenWire{}, nil, ErrPairingCode
	}
	secret, err := base64.RawURLEncoding.DecodeString(wire.Secret)
	validatedOrigin, originErr := validateLightOrigin(wire.Origin)
	if err != nil || originErr != nil || validatedOrigin != wire.Origin || len(secret) != 32 || wire.Version != 1 ||
		!validID(wire.ID) {
		return lightTokenWire{}, nil, ErrPairingCode
	}
	expiresAt := time.Unix(wire.ExpiresAt, 0).UTC()
	if !expiresAt.After(now.UTC()) || expiresAt.After(now.UTC().Add(maxLightTokenAge)) {
		return lightTokenWire{}, nil, ErrPairingCode
	}
	return wire, secret, nil
}

func validateLightOrigin(value string) (string, error) {
	value = strings.TrimRight(strings.TrimSpace(value), "/")
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.Hostname() == "" ||
		parsed.User != nil || parsed.Opaque != "" || parsed.Path != "" || parsed.RawPath != "" ||
		parsed.RawQuery != "" || parsed.Fragment != "" || parsed.ForceQuery ||
		strings.ContainsAny(parsed.Host, "\r\n\t ") {
		return "", ErrInvalidOrigin
	}
	return value, nil
}

func absDuration(value time.Duration) time.Duration {
	if value < 0 {
		return -value
	}
	return value
}
