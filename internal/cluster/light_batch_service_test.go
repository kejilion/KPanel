package cluster

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func lightBatchTokenForTest(t *testing.T, enrollment LightBatchEnrollment) string {
	t.Helper()
	fields := strings.Fields(enrollment.Command)
	if len(fields) == 0 {
		t.Fatalf("batch enrollment command is empty: %q", enrollment.Command)
	}
	return strings.Trim(fields[len(fields)-1], "'")
}

func TestLightBatchEnrollmentDefaultsToAndAllowsOneHundredHosts(t *testing.T) {
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	clock := &serviceTestClock{now: now}
	service := newLightServiceForTest(t, clock)

	enrollment, err := service.CreateLightBatchEnrollment(CreateLightBatchEnrollmentInput{})
	if err != nil {
		t.Fatal(err)
	}
	if enrollment.MaxUses != MaxHosts || enrollment.RemainingCount != MaxHosts ||
		!enrollment.ExpiresAt.Equal(now.Add(defaultLightBatchDuration)) ||
		!strings.Contains(enrollment.Command, "kpanel node join 'kpb1.") {
		t.Fatalf("unexpected default batch enrollment: %#v", enrollment)
	}
	if _, err := service.CreateLightBatchEnrollment(CreateLightBatchEnrollmentInput{MaxUses: MaxHosts + 1}); !errors.Is(err, ErrLightBatchInvalid) {
		t.Fatalf("101-use batch error = %v, want ErrLightBatchInvalid", err)
	}
	if _, err := service.CreateLightBatchEnrollment(CreateLightBatchEnrollmentInput{
		ExpiresInSeconds: int(^uint(0) >> 1),
	}); !errors.Is(err, ErrLightBatchInvalid) {
		t.Fatalf("overflowing duration error = %v, want ErrLightBatchInvalid", err)
	}
	token := lightBatchTokenForTest(t, enrollment)
	seenNodes := make(map[string]struct{}, MaxHosts)
	for index := 0; index < MaxHosts; index++ {
		response, _, err := service.EnrollLightNodeBatch(
			"198.51.100.10",
			"https://panel.example",
			LightEnrollRequest{Token: token, AttemptID: fmt.Sprintf("%032x", index+1)},
		)
		if err != nil {
			t.Fatalf("batch enrollment %d failed: %v", index+1, err)
		}
		if _, duplicate := seenNodes[response.NodeID]; duplicate {
			t.Fatalf("batch enrollment %d reused node %q", index+1, response.NodeID)
		}
		seenNodes[response.NodeID] = struct{}{}
	}
	if list := service.LightBatchEnrollments(); list.Total != 1 || len(list.Items) != 1 ||
		list.Items[0].UsedCount != MaxHosts || list.Items[0].RemainingCount != 0 {
		t.Fatalf("hundred-host batch counters are incorrect: %#v", list)
	}
	if _, _, err := service.EnrollLightNodeBatch(
		"198.51.100.10",
		"https://panel.example",
		LightEnrollRequest{Token: token, AttemptID: strings.Repeat("f", 32)},
	); !errors.Is(err, ErrHostLimit) {
		t.Fatalf("101st host error = %v, want ErrHostLimit", err)
	}
}

func TestLightBatchEnrollmentCreatesUniqueNodesAndRetriesIdempotently(t *testing.T) {
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	clock := &serviceTestClock{now: now}
	service := newLightServiceForTest(t, clock)
	enrollment, err := service.CreateLightBatchEnrollment(CreateLightBatchEnrollmentInput{
		NamePrefix: "edge", MaxUses: 100, ExpiresInSeconds: 3600,
	})
	if err != nil {
		t.Fatal(err)
	}
	token := lightBatchTokenForTest(t, enrollment)
	terminalKey, err := GenerateFederationV2Keypair()
	if err != nil {
		t.Fatal(err)
	}
	firstInput := LightEnrollRequest{
		Token: token, Name: "worker", NodeVersion: "1.18.0", AttemptID: strings.Repeat("a", 32),
		TerminalPublicKey: base64.RawURLEncoding.EncodeToString(terminalKey.Public),
	}
	first, policyID, err := service.EnrollLightNodeBatch("198.51.100.10", "https://panel.example", firstInput)
	if err != nil || policyID != enrollment.ID {
		t.Fatalf("first enrollment = %#v, %q, %v", first, policyID, err)
	}
	retry, retryPolicyID, err := service.EnrollLightNodeBatch("198.51.100.10", "https://panel.example", firstInput)
	if err != nil || retry != first || retryPolicyID != policyID {
		t.Fatalf("idempotent retry = %#v, %q, %v; want %#v, %q", retry, retryPolicyID, err, first, policyID)
	}
	second, _, err := service.EnrollLightNodeBatch("198.51.100.11", "https://panel.example", LightEnrollRequest{
		Token: token, Name: "worker", NodeVersion: "1.18.0", AttemptID: strings.Repeat("b", 32),
	})
	if err != nil || second.NodeID == first.NodeID || second.ReportingKey == first.ReportingKey {
		t.Fatalf("second enrollment = %#v, %v", second, err)
	}

	for _, nodeID := range []string{first.NodeID, second.NodeID} {
		host, err := service.Host(context.Background(), nodeID)
		if err != nil || host.Kind != HostKindLightNode || host.Name != "edge · worker" {
			t.Fatalf("batch host %q = %#v, %v", nodeID, host, err)
		}
	}
	list := service.LightBatchEnrollments()
	if list.Total != 1 || len(list.Items) != 1 || list.Items[0].Command != "" ||
		list.Items[0].UsedCount != 2 || list.Items[0].RemainingCount != 98 {
		t.Fatalf("unexpected batch list: %#v", list)
	}

	wireBytes, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(token, lightBatchTokenPrefix))
	if err != nil {
		t.Fatal(err)
	}
	var wire lightTokenWire
	if err := json.Unmarshal(wireBytes, &wire); err != nil {
		t.Fatal(err)
	}
	state, err := os.ReadFile(filepath.Join(filepath.Dir(service.light.path), lightBatchStateFileName))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(state, []byte(token)) || bytes.Contains(state, []byte(wire.Secret)) {
		t.Fatal("raw batch enrollment secret persisted")
	}
	reopened, err := openLightBatchStore(filepath.Join(filepath.Dir(service.light.path), lightBatchStateFileName))
	if err != nil {
		t.Fatal(err)
	}
	records := reopened.List(now)
	if len(records) != 1 || len(records[0].Attempts) != 2 || records[0].ID != enrollment.ID {
		t.Fatalf("batch state did not survive reopen: %#v", records)
	}
	statePath := filepath.Join(filepath.Dir(service.light.path), lightBatchStateFileName)
	if err := os.WriteFile(statePath+".previous", state, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath, []byte("{\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	recovered, err := openLightBatchStore(statePath)
	if err != nil {
		t.Fatal(err)
	}
	records = recovered.List(now)
	if len(records) != 1 || len(records[0].Attempts) != 2 || records[0].ID != enrollment.ID {
		t.Fatalf("batch state did not recover its atomic backup: %#v", records)
	}
	if _, err := os.Lstat(statePath + ".previous"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("batch recovery retained its backup: %v", err)
	}
}

func TestLightBatchEnrollmentRejectsChangedRetryAndStopsAtPolicyLimit(t *testing.T) {
	clock := &serviceTestClock{now: time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)}
	service := newLightServiceForTest(t, clock)
	enrollment, err := service.CreateLightBatchEnrollment(CreateLightBatchEnrollmentInput{MaxUses: 1, ExpiresInSeconds: 300})
	if err != nil {
		t.Fatal(err)
	}
	token := lightBatchTokenForTest(t, enrollment)
	input := LightEnrollRequest{Token: token, Name: "edge-1", NodeVersion: "1.18.0", AttemptID: strings.Repeat("c", 32)}
	if _, _, err := service.EnrollLightNodeBatch("198.51.100.10", "https://panel.example", input); err != nil {
		t.Fatal(err)
	}
	changed := input
	changed.Name = "renamed"
	if _, _, err := service.EnrollLightNodeBatch("198.51.100.10", "https://panel.example", changed); !errors.Is(err, ErrIdentityMismatch) {
		t.Fatalf("changed retry error = %v, want ErrIdentityMismatch", err)
	}
	second := input
	second.AttemptID = strings.Repeat("d", 32)
	if _, _, err := service.EnrollLightNodeBatch("198.51.100.11", "https://panel.example", second); !errors.Is(err, ErrPairingCode) {
		t.Fatalf("exhausted policy error = %v, want ErrPairingCode", err)
	}
}

func TestLightBatchEnrollmentReservedRetryCannotExceedGlobalHostLimit(t *testing.T) {
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	clock := &serviceTestClock{now: now}
	service := newLightServiceForTest(t, clock)
	enrollment, err := service.CreateLightBatchEnrollment(CreateLightBatchEnrollmentInput{MaxUses: 2})
	if err != nil {
		t.Fatal(err)
	}
	token := lightBatchTokenForTest(t, enrollment)
	wire, secret, err := parseLightTokenForPrefix(token, lightBatchTokenPrefix, maximumLightBatchDuration, now)
	if err != nil {
		t.Fatal(err)
	}
	secretHash := sha256.Sum256(secret)
	attemptID := strings.Repeat("a", 32)
	reservedNodeID := strings.Repeat("b", 32)
	if _, _, err := service.lightBatches.Reserve(
		wire.ID,
		hex.EncodeToString(secretHash[:]),
		attemptID,
		reservedNodeID,
		"reserved",
		"1.18.0",
		"",
		true,
		now,
	); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < MaxHosts; index++ {
		nodeID := fmt.Sprintf("%032x", index+1)
		record := lightHostRecord{
			ID: nodeID, Name: "capacity", NodeVersion: "1.18.0",
			CreatedAt: now, UpdatedAt: now,
		}
		if err := service.light.AddHost(record, bytes.Repeat([]byte{byte(index + 1)}, 32)); err != nil {
			t.Fatalf("fill host capacity %d: %v", index+1, err)
		}
	}
	if _, _, err := service.EnrollLightNodeBatch(
		"198.51.100.10",
		"https://panel.example",
		LightEnrollRequest{Token: token, Name: "reserved", NodeVersion: "1.18.0", AttemptID: attemptID},
	); !errors.Is(err, ErrHostLimit) {
		t.Fatalf("reserved retry at global capacity error = %v, want ErrHostLimit", err)
	}
	if len(service.light.Hosts()) != MaxHosts {
		t.Fatalf("reserved retry exceeded global host limit: %d", len(service.light.Hosts()))
	}
}

func TestLightBatchEnrollmentRevocationAndExpiryStopFutureNodes(t *testing.T) {
	clock := &serviceTestClock{now: time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)}
	service := newLightServiceForTest(t, clock)
	create := func() LightBatchEnrollment {
		enrollment, err := service.CreateLightBatchEnrollment(CreateLightBatchEnrollmentInput{MaxUses: 2, ExpiresInSeconds: 300})
		if err != nil {
			t.Fatal(err)
		}
		return enrollment
	}
	revoked := create()
	if err := service.DeleteLightBatchEnrollment(revoked.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.EnrollLightNodeBatch("198.51.100.10", "https://panel.example", LightEnrollRequest{
		Token: lightBatchTokenForTest(t, revoked), AttemptID: strings.Repeat("e", 32),
	}); !errors.Is(err, ErrPairingCode) {
		t.Fatalf("revoked policy error = %v, want ErrPairingCode", err)
	}

	expired := create()
	clock.Advance(5 * time.Minute)
	if _, _, err := service.EnrollLightNodeBatch("198.51.100.11", "https://panel.example", LightEnrollRequest{
		Token: lightBatchTokenForTest(t, expired), AttemptID: strings.Repeat("f", 32),
	}); !errors.Is(err, ErrPairingCode) {
		t.Fatalf("expired policy error = %v, want ErrPairingCode", err)
	}
	if list := service.LightBatchEnrollments(); list.Total != 0 || len(list.Items) != 0 {
		t.Fatalf("expired or revoked policies remained visible: %#v", list)
	}
}
