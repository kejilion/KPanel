package cluster

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"
)

const (
	lightBatchTokenPrefix           = "kpb1."
	LightBatchEnrollPath            = "/api/v3/federation/light/batch-enroll"
	defaultLightBatchDuration       = 24 * time.Hour
	minimumLightBatchDuration       = 5 * time.Minute
	maximumLightBatchDuration       = 7 * 24 * time.Hour
	maxLightBatchNamePrefixRunes    = 40
	maxLightBatchPolicyRateSubjects = 256
	defaultLightBatchEnrollmentUse  = MaxHosts
)

func (s *Service) CreateLightBatchEnrollment(input CreateLightBatchEnrollmentInput) (LightBatchEnrollment, error) {
	return s.CreateLightBatchEnrollmentForOrigin(s.publicURL, input)
}

func (s *Service) CreateLightBatchEnrollmentForOrigin(
	origin string,
	input CreateLightBatchEnrollmentInput,
) (LightBatchEnrollment, error) {
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	validatedOrigin, err := validateLightOrigin(origin)
	if err != nil {
		return LightBatchEnrollment{}, ErrLightHTTPSOrigin
	}
	namePrefix, err := validateLightBatchNamePrefix(input.NamePrefix)
	if err != nil {
		return LightBatchEnrollment{}, err
	}
	maxUses := input.MaxUses
	if maxUses == 0 {
		maxUses = defaultLightBatchEnrollmentUse
	}
	if maxUses < 1 || maxUses > MaxHosts {
		return LightBatchEnrollment{}, ErrLightBatchInvalid
	}
	duration := defaultLightBatchDuration
	if input.ExpiresInSeconds != 0 {
		minimumSeconds := int(minimumLightBatchDuration / time.Second)
		maximumSeconds := int(maximumLightBatchDuration / time.Second)
		if input.ExpiresInSeconds < minimumSeconds || input.ExpiresInSeconds > maximumSeconds {
			return LightBatchEnrollment{}, ErrLightBatchInvalid
		}
		duration = time.Duration(input.ExpiresInSeconds) * time.Second
	}
	now := s.now().UTC()
	id, err := randomHex(16)
	if err != nil {
		return LightBatchEnrollment{}, err
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return LightBatchEnrollment{}, err
	}
	expiresAt := now.Add(duration)
	hash := sha256.Sum256(secret)
	record := lightBatchEnrollmentRecord{
		ID: id, SecretHash: hex.EncodeToString(hash[:]), NamePrefix: namePrefix,
		MaxUses: maxUses, CreatedAt: now, ExpiresAt: expiresAt,
		Attempts: []lightBatchAttemptRecord{},
	}
	if err := s.lightBatches.Create(record, now); err != nil {
		return LightBatchEnrollment{}, err
	}
	wire, err := json.Marshal(lightTokenWire{
		Version: 1, Origin: validatedOrigin, ID: id,
		Secret: base64.RawURLEncoding.EncodeToString(secret), ExpiresAt: expiresAt.Unix(),
	})
	if err != nil {
		_ = s.lightBatches.Delete(id, now)
		return LightBatchEnrollment{}, err
	}
	token := lightBatchTokenPrefix + base64.RawURLEncoding.EncodeToString(wire)
	result := publicLightBatchEnrollment(record)
	result.Command = "bash <(curl -fsSL https://kejilion.sh) kpanel node join '" + token + "'"
	return result, nil
}

func (s *Service) LightBatchEnrollments() LightBatchEnrollmentList {
	now := s.now().UTC()
	records := s.lightBatches.List(now)
	items := make([]LightBatchEnrollment, 0, len(records))
	for _, record := range records {
		items = append(items, publicLightBatchEnrollment(record))
	}
	return LightBatchEnrollmentList{Items: items, Total: len(items)}
}

func (s *Service) DeleteLightBatchEnrollment(id string) error {
	if !validID(strings.TrimSpace(id)) {
		return ErrNotFound
	}
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	return s.lightBatches.Delete(strings.TrimSpace(id), s.now().UTC())
}

func (s *Service) EnrollLightNodeBatch(
	source string,
	origin string,
	input LightEnrollRequest,
) (LightEnrollResponse, string, error) {
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	now := s.now().UTC()
	if !s.lightBatchSources.Allow(cleanRateSubject(source), now) {
		return LightEnrollResponse{}, "", ErrRateLimited
	}
	wire, secret, err := parseLightTokenForPrefix(input.Token, lightBatchTokenPrefix, maximumLightBatchDuration, now)
	validatedOrigin, originErr := validateLightOrigin(origin)
	if err != nil || originErr != nil || wire.Origin != validatedOrigin || !validID(input.AttemptID) {
		return LightEnrollResponse{}, "", ErrPairingCode
	}
	if !s.lightBatchPolicies.Allow(wire.ID, now) {
		return LightEnrollResponse{}, "", ErrRateLimited
	}
	name, err := validateOptionalName(input.Name)
	if err != nil {
		return LightEnrollResponse{}, "", err
	}
	terminalPublicKey, err := decodeTerminalRelayPublicKey(input.TerminalPublicKey)
	if err != nil {
		return LightEnrollResponse{}, "", ErrPairingCode
	}
	nodeID, err := randomHex(16)
	if err != nil {
		return LightEnrollResponse{}, "", err
	}
	hash := sha256.Sum256(secret)
	hasCapacity := len(s.store.Hosts())+len(s.storeV2.Hosts())+len(s.light.Hosts()) < MaxHosts
	attempt, _, err := s.lightBatches.Reserve(
		wire.ID,
		hex.EncodeToString(hash[:]),
		input.AttemptID,
		nodeID,
		name,
		input.NodeVersion,
		lightBatchTerminalKey(terminalPublicKey),
		hasCapacity,
		now,
	)
	if err != nil {
		return LightEnrollResponse{}, "", err
	}
	record, err := s.light.Host(attempt.NodeID)
	switch {
	case err == nil:
		storedTerminalKey, keyErr := s.light.ReadTerminalPublicKey(record)
		if keyErr != nil || record.Name != attempt.Name || !bytes.Equal(storedTerminalKey, terminalPublicKey) {
			return LightEnrollResponse{}, "", ErrIdentityMismatch
		}
	case errors.Is(err, ErrNotFound):
		if !hasCapacity {
			return LightEnrollResponse{}, "", ErrHostLimit
		}
		reportingKey := make([]byte, 32)
		if _, err := rand.Read(reportingKey); err != nil {
			return LightEnrollResponse{}, "", err
		}
		record = lightHostRecord{
			ID: attempt.NodeID, Name: attempt.Name, NodeVersion: attempt.NodeVersion,
			CreatedAt: now, UpdatedAt: now,
		}
		if err := s.light.AddHostWithTerminal(record, reportingKey, terminalPublicKey); err != nil {
			return LightEnrollResponse{}, "", err
		}
		record, err = s.light.Host(attempt.NodeID)
		if err != nil {
			return LightEnrollResponse{}, "", err
		}
	default:
		return LightEnrollResponse{}, "", err
	}
	reportingKey, err := s.light.ReadSecret(record)
	if err != nil {
		return LightEnrollResponse{}, "", err
	}
	response := LightEnrollResponse{
		NodeID: attempt.NodeID, ReportingKey: base64.RawURLEncoding.EncodeToString(reportingKey),
		ReportInterval: lightReportInterval,
	}
	if len(terminalPublicKey) > 0 {
		response.TerminalPeerPublicKey = base64.RawURLEncoding.EncodeToString(s.nodeIdentityV2.PublicKey)
		response.TargetNodeID = s.store.NodeID()
	}
	return response, wire.ID, nil
}

func publicLightBatchEnrollment(record lightBatchEnrollmentRecord) LightBatchEnrollment {
	used := len(record.Attempts)
	return LightBatchEnrollment{
		ID: record.ID, NamePrefix: record.NamePrefix, MaxUses: record.MaxUses,
		UsedCount: used, RemainingCount: max(0, record.MaxUses-used),
		CreatedAt: record.CreatedAt, ExpiresAt: record.ExpiresAt,
	}
}

func validateLightBatchNamePrefix(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if cleanDisplayText(value, maxLightBatchNamePrefixRunes) != value {
		return "", ErrLightBatchInvalid
	}
	return value, nil
}

func lightBatchNodeName(prefix, requestedName, nodeID string) string {
	name := strings.TrimSpace(requestedName)
	if name == "" {
		name = "轻量节点 " + nodeID[:8]
	}
	if prefix != "" {
		name = prefix + " · " + name
	}
	runes := []rune(name)
	if len(runes) > 80 {
		name = strings.TrimSpace(string(runes[:80]))
	}
	return name
}

func parseLightTokenForPrefix(
	token string,
	prefix string,
	maximumAge time.Duration,
	now time.Time,
) (lightTokenWire, []byte, error) {
	token = strings.TrimSpace(token)
	if !strings.HasPrefix(token, prefix) || len(token) > 2048 {
		return lightTokenWire{}, nil, ErrPairingCode
	}
	content, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(token, prefix))
	if err != nil || len(content) > 1536 {
		return lightTokenWire{}, nil, ErrPairingCode
	}
	var wire lightTokenWire
	decoder := json.NewDecoder(bytes.NewReader(content))
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
	if err != nil || originErr != nil || validatedOrigin != wire.Origin || len(secret) != 32 ||
		wire.Version != 1 || !validID(wire.ID) {
		return lightTokenWire{}, nil, ErrPairingCode
	}
	expiresAt := time.Unix(wire.ExpiresAt, 0).UTC()
	if !expiresAt.After(now.UTC()) || expiresAt.After(now.UTC().Add(maximumAge)) {
		return lightTokenWire{}, nil, ErrPairingCode
	}
	return wire, secret, nil
}
