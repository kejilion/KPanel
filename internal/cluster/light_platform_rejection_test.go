package cluster

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestUnsupportedPlatformsAreRejectedBeforeEnrollment(t *testing.T) {
	s := newLightServiceForTest(t, &serviceTestClock{now: time.Now().UTC()})
	for name, action := range map[string]func() error{
		"single enrollment": func() error {
			_, err := s.EnrollLightNode("198.51.100.1", LightEnrollRequest{Platform: "unsupported"})
			return err
		},
		"batch enrollment": func() error {
			_, _, err := s.EnrollLightNodeBatch("198.51.100.1", s.publicURL,
				LightEnrollRequest{Platform: "unsupported", AttemptID: strings.Repeat("a", 32)})
			return err
		},
		"report": func() error {
			return validateLightPlatform(LightReportRequest{Platform: "unsupported"})
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := action(); !errors.Is(err, ErrLightPlatformUnsupported) {
				t.Fatalf("unsupported platform was not rejected: %v", err)
			}
		})
	}
	if got := s.LightBatchEnrollments(); got.Total != 0 {
		t.Fatalf("rejected request left batch enrollment state: %#v", got)
	}
}

func TestLinuxLightNodeEnrollmentCommandRemainsAvailable(t *testing.T) {
	s := newLightServiceForTest(t, &serviceTestClock{now: time.Now().UTC()})
	command, err := s.lightEnrollmentCommand("kpl1.token", "linux node")
	if err != nil || !strings.Contains(command, "kpanel node join 'kpl1.token'") || !strings.Contains(command, "--name 'linux node'") {
		t.Fatalf("Linux command = %q, %v", command, err)
	}
}

func TestUnsupportedLightNodeIsAbsentFromInventoryAndLookup(t *testing.T) {
	now := time.Now().UTC()
	s := newLightServiceForTest(t, &serviceTestClock{now: now})
	linux := enrollLightHostForTest(t, s, "supported Linux")
	unsupportedID := strings.Repeat("b", 32)
	if err := s.light.AddHost(lightHostRecord{
		Platform: "windows", ID: unsupportedID, Name: "legacy node", CreatedAt: now, UpdatedAt: now,
	}, bytes.Repeat([]byte{5}, 32)); err != nil {
		t.Fatal(err)
	}

	inventory := s.Hosts(context.Background())
	if inventory.RemoteTotal != 1 || inventory.Total != 2 || len(inventory.Items) != 2 {
		t.Fatalf("unsupported node leaked into inventory totals: %#v", inventory)
	}
	for _, host := range inventory.Items {
		if host.ID == unsupportedID {
			t.Fatal("unsupported node remained visible in cluster inventory")
		}
	}
	if _, err := s.Host(context.Background(), unsupportedID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unsupported direct lookup = %v, want not found", err)
	}
	if _, err := s.Refresh(context.Background(), unsupportedID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unsupported refresh = %v, want not found", err)
	}
	unsupported, err := s.light.Host(unsupportedID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.RenameHost(unsupportedID, UpdateHostInput{
		Name: "renamed", ExpectedResourceVersion: unsupported.ResourceVersion,
	}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unsupported rename = %v, want not found", err)
	}
	if s.lightControlAllowed(unsupportedID, "terminal") || s.lightControlAllowed(unsupportedID, "files") {
		t.Fatal("unsupported node retained derived controls")
	}
	if _, err := s.Host(context.Background(), linux.NodeID); err != nil {
		t.Fatalf("supported Linux lookup failed: %v", err)
	}
}

func TestRetiredLightNodeRecordsDoNotConsumeCurrentHostCapacity(t *testing.T) {
	now := time.Now().UTC()
	s := newLightServiceForTest(t, &serviceTestClock{now: now})
	for index := 0; index < MaxHosts; index++ {
		id := fmt.Sprintf("%032x", index+1)
		if err := s.light.AddHost(lightHostRecord{
			Platform: "windows", ID: id, Name: fmt.Sprintf("retired-%d", index),
			CreatedAt: now, UpdatedAt: now,
		}, bytes.Repeat([]byte{byte(index%251 + 1)}, 32)); err != nil {
			t.Fatalf("add retired host %d: %v", index, err)
		}
	}
	enrollLightHostForTest(t, s, "new Linux node")
}

func TestUnsupportedLightNodeCannotPollDerivedFeatureRelays(t *testing.T) {
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	s := newLightServiceForTest(t, &serviceTestClock{now: now})
	key, err := GenerateFederationV2Keypair()
	if err != nil {
		t.Fatal(err)
	}
	nodeID := strings.Repeat("c", 32)
	if err := s.light.AddHostWithTerminal(lightHostRecord{
		Platform: "windows", ID: nodeID, Name: "retired node", CreatedAt: now, UpdatedAt: now,
	}, bytes.Repeat([]byte{7}, 32), key.Public); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name    string
		path    string
		payload any
	}{
		{name: "file", path: v2FileRelayPath, payload: FileRelayPollRequest{}},
		{name: "history", path: HistoryRelayV2Path, payload: FileRelayPollRequest{}},
		{name: "terminal", path: v2TerminalRelayPath, payload: TerminalRelayPollRequest{}},
	}
	for index, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			payload, err := json.Marshal(test.payload)
			if err != nil {
				t.Fatal(err)
			}
			envelope, _, err := sealV2Request(http.MethodPost, test.path, v2Envelope{
				Protocol: FederationProtocolV2, ControllerID: nodeID, TargetID: s.NodeID(),
				Timestamp: now.Unix(), RequestID: fmt.Sprintf("%032x", index+1),
			}, key, nodeNoiseKeyV2(s.nodeIdentityV2).Public, nil, payload)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if _, err := s.HandleFederationV2(ctx, "198.51.100.10", test.path, "", envelope); !errors.Is(err, ErrAuthentication) {
				t.Fatalf("retired relay poll error = %v, want ErrAuthentication", err)
			}
		})
	}
}
