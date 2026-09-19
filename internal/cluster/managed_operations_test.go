package cluster

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestManagedOperationsRequireSeparateTargetGrant(t *testing.T) {
	clock := &serviceTestClock{now: time.Now().UTC()}
	targetRemote, _ := newServiceV2Remote(t)
	target, err := NewService(ServiceConfig{DataDir: t.TempDir(), Remote: targetRemote, Now: clock.Now, Telemetry: serviceTestTelemetry{now: clock.Now, hostname: "target"}})
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	remote, wire := newServiceV2Remote(t)
	wire.target = target
	center, err := NewService(ServiceConfig{DataDir: t.TempDir(), Remote: remote, Now: clock.Now, Telemetry: serviceTestTelemetry{now: clock.Now, hostname: "center"}})
	if err != nil {
		t.Fatal(err)
	}
	defer center.Close()
	code, err := target.CreatePairingCodeV2()
	if err != nil {
		t.Fatal(err)
	}
	host, err := center.AddHost(context.Background(), AddHostInput{Name: "target", Origin: "http://8.8.8.8:1801", PairingCode: code.Code})
	if err != nil {
		t.Fatal(err)
	}
	called := 0
	target.SetManagedOperationHandler(func(_ context.Context, _ string, _ ManagedGrant, _ ManagedRequest, active func() bool) (json.RawMessage, error) {
		called++
		if !active() {
			t.Fatal("grant inactive at dispatch")
		}
		return json.RawMessage(`{"state":"ready"}`), nil
	})
	capability, err := center.ManagedRemote(context.Background(), host.ID, ManagedRequest{Version: 1, Mode: "capabilities"})
	if err != nil || !capability.Supported || capability.Authorized {
		t.Fatalf("pairing silently granted management: %#v %v", capability, err)
	}
	input := ManagedRequest{Version: 1, Mode: "read", ClientID: strings.Repeat("a", 32), Tool: "host_apps_list", Signature: strings.Repeat("b", 64), Arguments: json.RawMessage(`{}`)}
	denied, err := center.ManagedRemote(context.Background(), host.ID, input)
	if err != nil || denied.ErrorCode != "remote_operations_not_authorized" || called != 0 {
		t.Fatalf("missing grant bypass: %#v %v calls=%d", denied, err, called)
	}
	controllers := target.ManagedControllers()
	if len(controllers) != 1 {
		t.Fatal("missing paired controller")
	}
	policy := ManagedPolicy{OperationVersions: map[string]string{input.Tool: input.Signature}}
	if err := target.SetManagedGrant(controllers[0].ID, policy, time.Hour, target.ManagedGrants().ResourceVersion); err != nil {
		t.Fatal(err)
	}
	result, err := center.ManagedRemote(context.Background(), host.ID, input)
	if err != nil || result.ErrorCode != "" || string(result.Data) != `{"state":"ready"}` || called != 1 {
		t.Fatalf("authorized read: %#v %v calls=%d", result, err, called)
	}
	for _, body := range wire.requestBodies() {
		if bytes.Contains(body, []byte(input.Tool)) {
			t.Fatal("operation leaked outside encrypted payload")
		}
	}
	bad := input
	bad.Signature = strings.Repeat("c", 64)
	result, err = center.ManagedRemote(context.Background(), host.ID, bad)
	if err != nil || result.ErrorCode != "remote_operation_not_granted_or_version_changed" || called != 1 {
		t.Fatal("operation version not enforced")
	}
	if err := target.RevokeManagedGrant(controllers[0].ID, target.ManagedGrants().ResourceVersion); err != nil {
		t.Fatal(err)
	}
	result, err = center.ManagedRemote(context.Background(), host.ID, input)
	if err != nil || result.Authorized || called != 1 {
		t.Fatal("revoked grant still usable")
	}
	if err := target.SetManagedGrant(controllers[0].ID, policy, time.Hour, target.ManagedGrants().ResourceVersion); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Dir(target.managed.path)
	if err := ResetManagedGrants(dir); err != nil {
		t.Fatal(err)
	}
	// Simulate reopening after offline restore, retaining the same pairing
	// identity and controller key. Pairing must not recreate deleted authority.
	target.managed = openManagedControl(dir)
	target.SetManagedOperationHandler(func(context.Context, string, ManagedGrant, ManagedRequest, func() bool) (json.RawMessage, error) {
		called++
		return json.RawMessage(`{}`), nil
	})
	result, err = center.ManagedRemote(context.Background(), host.ID, input)
	if err != nil || result.Authorized || called != 1 {
		t.Fatal("restored pairing resurrected managed grant")
	}
}

func TestManagedRegrantInvalidatesInFlightAuthority(t *testing.T) {
	clock := &serviceTestClock{now: time.Now().UTC()}
	targetRemote, _ := newServiceV2Remote(t)
	target, err := NewService(ServiceConfig{DataDir: t.TempDir(), Remote: targetRemote, Now: clock.Now, Telemetry: serviceTestTelemetry{now: clock.Now, hostname: "target"}})
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	remote, wire := newServiceV2Remote(t)
	wire.target = target
	center, err := NewService(ServiceConfig{DataDir: t.TempDir(), Remote: remote, Now: clock.Now, Telemetry: serviceTestTelemetry{now: clock.Now, hostname: "center"}})
	if err != nil {
		t.Fatal(err)
	}
	defer center.Close()
	code, _ := target.CreatePairingCodeV2()
	host, err := center.AddHost(context.Background(), AddHostInput{Origin: "http://8.8.8.8:1801", PairingCode: code.Code})
	if err != nil {
		t.Fatal(err)
	}
	controller := target.ManagedControllers()[0].ID
	policy := ManagedPolicy{OperationVersions: map[string]string{"host_apps_list": strings.Repeat("b", 64)}}
	if err := target.SetManagedGrant(controller, policy, time.Hour, target.ManagedGrants().ResourceVersion); err != nil {
		t.Fatal(err)
	}
	target.SetManagedOperationHandler(func(_ context.Context, _ string, _ ManagedGrant, _ ManagedRequest, active func() bool) (json.RawMessage, error) {
		if err := target.SetManagedGrant(controller, policy, time.Hour, target.ManagedGrants().ResourceVersion); err != nil {
			t.Fatal(err)
		}
		if active() {
			t.Fatal("same-clock regrant retained old authority")
		}
		return json.RawMessage(`{"secret":"must-not-return"}`), nil
	})
	result, err := center.ManagedRemote(context.Background(), host.ID, ManagedRequest{Version: 1, Mode: "read", ClientID: strings.Repeat("a", 32), Tool: "host_apps_list", Signature: strings.Repeat("b", 64), Arguments: json.RawMessage(`{}`)})
	if err != nil || len(result.Data) > 0 || result.Authorized {
		t.Fatalf("in-flight regrant: %#v %v", result, err)
	}
}
