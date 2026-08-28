package cluster

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

type v2RoundTripper func(*http.Request) (*http.Response, error)

func (f v2RoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestNormalizeV2OriginSupportsHTTPSAndExplicitHTTPIP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		raw  string
		want string
	}{
		{raw: "https://panel.example.com", want: "https://panel.example.com"},
		{raw: "https://8.8.8.8:8443", want: "https://8.8.8.8:8443"},
		{raw: "http://8.8.8.8:8080", want: "http://8.8.8.8:8080"},
		{
			raw:  "http://[2606:4700:4700:0:0:0:0:1111]:1801",
			want: "http://[2606:4700:4700::1111]:1801",
		},
		{raw: "http://10.0.0.8:1801", want: "http://10.0.0.8:1801"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.raw, func(t *testing.T) {
			t.Parallel()
			got, err := NormalizeV2Origin(test.raw)
			if err != nil {
				t.Fatalf("NormalizeV2Origin(%q) error = %v", test.raw, err)
			}
			if got != test.want {
				t.Fatalf("NormalizeV2Origin(%q) = %q, want %q", test.raw, got, test.want)
			}
		})
	}
}

func TestNormalizeV2OriginRejectsAmbiguousHTTPAndUnsafeShapes(t *testing.T) {
	t.Parallel()

	for _, input := range []string{
		"http://panel.example.com:8080",
		"http://8.8.8.8",
		"http://8.8.8.8:80",
		"http://8.8.8.8:0",
		"http://8.8.8.8:65536",
		"http://user@example.com:8080",
		"http://8.8.8.8:8080/",
		"http://8.8.8.8:8080/path",
		"http://8.8.8.8:8080?query=1",
		"http://8.8.8.8:8080#fragment",
		"http://2130706433:8080",
		"http://0177.0.0.1:8080",
		"http://[fe80::1%25eth0]:8080",
		"ftp://8.8.8.8:8080",
	} {
		input := input
		t.Run(input, func(t *testing.T) {
			t.Parallel()
			if got, err := NormalizeV2Origin(input); !errors.Is(err, ErrInvalidOrigin) {
				t.Fatalf("NormalizeV2Origin(%q) = %q, %v; want ErrInvalidOrigin", input, got, err)
			}
		})
	}
}

func TestRemoteClientV2OriginPolicyAllowsOnlyConfiguredPrivateRanges(t *testing.T) {
	t.Parallel()

	client, err := NewRemoteClient(RemoteClientConfig{
		PrivateCIDRs: []string{"10.20.0.0/16"},
		Resolver: staticResolver{
			"panel.example.com": []net.IP{net.ParseIP("8.8.8.8")},
		},
		Dialer: func(context.Context, string, string) (net.Conn, error) {
			return nil, errors.New("dial stopped by test")
		},
	})
	if err != nil {
		t.Fatalf("NewRemoteClient() error = %v", err)
	}
	for _, origin := range []string{
		"http://8.8.8.8:1801",
		"http://10.20.1.5:1801",
		"https://panel.example.com",
	} {
		if _, err := client.ValidateV2Origin(context.Background(), origin); err != nil {
			t.Fatalf("ValidateV2Origin(%q) error = %v", origin, err)
		}
	}
	for _, origin := range []string{
		"http://127.0.0.1:1801",
		"http://169.254.169.254:1801",
		"http://10.30.1.5:1801",
		"http://[64:ff9b::a9fe:a9fe]:1801",
		"http://[2002:a9fe:a9fe::1]:1801",
	} {
		if _, err := client.ValidateV2Origin(context.Background(), origin); !errors.Is(err, ErrPrivateOrigin) {
			t.Fatalf("ValidateV2Origin(%q) error = %v, want ErrPrivateOrigin", origin, err)
		}
	}
}

func TestRemoteClientV2EncryptsPairCommitSummaryAndRevoke(t *testing.T) {
	now := time.Date(2026, 7, 29, 8, 0, 0, 0, time.UTC)
	targetKey, err := v2NoiseSuite.GenerateKeypair(rand.Reader)
	if err != nil {
		t.Fatalf("target GenerateKeypair() error = %v", err)
	}
	controllerKey, err := v2NoiseSuite.GenerateKeypair(rand.Reader)
	if err != nil {
		t.Fatalf("controller GenerateKeypair() error = %v", err)
	}
	code, _, _, err := makeV2PairingCode(
		strings.Repeat("a", 32), targetKey.Public, now.Add(5*time.Minute),
	)
	if err != nil {
		t.Fatalf("makeV2PairingCode() error = %v", err)
	}
	pairing, err := parseV2PairingCode(code, now)
	if err != nil {
		t.Fatalf("parseV2PairingCode() error = %v", err)
	}
	controllerID := strings.Repeat("b", 32)
	transactionID := strings.Repeat("c", 32)
	client := &RemoteClient{}
	client.client = &http.Client{Transport: v2RoundTripper(func(request *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			return nil, err
		}
		var envelope v2Envelope
		if err := json.Unmarshal(body, &envelope); err != nil {
			return nil, err
		}
		psk := []byte(nil)
		if request.URL.Path == v2PairPath {
			psk = pairing.PairingKey
		}
		plaintext, peerStatic, handshake, err := openV2Request(
			http.MethodPost, request.URL.Path, envelope, targetKey, psk,
		)
		if err != nil {
			return nil, err
		}
		if !bytes.Equal(peerStatic, controllerKey.Public) {
			t.Fatal("request did not authenticate controller static key")
		}
		var response any
		switch request.URL.Path {
		case v2PairPath:
			var input v2PairPayload
			if err := json.Unmarshal(plaintext, &input); err != nil ||
				input.ControllerName != "controller" ||
				input.TransactionID != transactionID {
				t.Fatalf("unexpected pair payload %q: %v", plaintext, err)
			}
			response = v2PairResult{
				TransactionID: transactionID, NodeID: pairing.NodeID,
				Hostname: "target", PanelVersion: "v0.27.0",
				FederationProtocol: FederationProtocolV2,
			}
		case v2CommitPath:
			response = v2CommitResult{TransactionID: transactionID, Active: true}
		case v2SummaryPath:
			response = FederationSummary{
				NodeID: pairing.NodeID, PanelVersion: "v0.27.0",
				FederationProtocol: FederationProtocolV2,
				Telemetry:          contract.HostTelemetry{Hostname: "target", CollectedAt: now},
			}
		case v2RevokePath:
			response = v2RevokeResult{Revoked: true}
		default:
			t.Fatalf("unexpected path %q", request.URL.Path)
		}
		payload, err := json.Marshal(response)
		if err != nil {
			return nil, err
		}
		sealed, err := sealV2Response(envelope, handshake, payload)
		if err != nil {
			return nil, err
		}
		content, err := json.Marshal(sealed)
		if err != nil {
			return nil, err
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"application/json"}},
			Body:       io.NopCloser(bytes.NewReader(content)),
		}, nil
	})}
	origin := "http://8.8.8.8:1801"
	pair, err := client.PairV2(
		context.Background(), origin, controllerID, "controller", transactionID,
		controllerKey, pairing, now,
	)
	if err != nil || pair.TransactionID != transactionID {
		t.Fatalf("PairV2() = %#v, %v", pair, err)
	}
	commit, err := client.CommitV2(
		context.Background(), origin, controllerID, pairing.NodeID,
		transactionID, controllerKey, targetKey.Public, now,
	)
	if err != nil || !commit.Active {
		t.Fatalf("CommitV2() = %#v, %v", commit, err)
	}
	summary, err := client.SummaryV2(
		context.Background(), origin, controllerID, pairing.NodeID,
		controllerKey, targetKey.Public, now,
	)
	if err != nil || summary.Telemetry.Hostname != "target" {
		t.Fatalf("SummaryV2() = %#v, %v", summary, err)
	}
	if err := client.RevokeV2(
		context.Background(), origin, controllerID, pairing.NodeID,
		controllerKey, targetKey.Public, now,
	); err != nil {
		t.Fatalf("RevokeV2() error = %v", err)
	}
}

func TestTerminalRelayClientV2UsesSharedNoiseAndTerminalPayloads(t *testing.T) {
	now := time.Date(2026, 8, 29, 8, 0, 0, 0, time.UTC)
	targetKey, err := GenerateFederationV2Keypair()
	if err != nil {
		t.Fatalf("target GenerateFederationV2Keypair() error = %v", err)
	}
	controllerKey, err := GenerateFederationV2Keypair()
	if err != nil {
		t.Fatalf("controller GenerateFederationV2Keypair() error = %v", err)
	}
	controllerID := strings.Repeat("a", 32)
	targetID := strings.Repeat("b", 32)
	sessionID := strings.Repeat("c", 32)
	commandID := strings.Repeat("d", 32)
	client := &TerminalRelayClient{}
	client.client = &http.Client{Transport: v2RoundTripper(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodPost || request.URL.Path != v2TerminalRelayPath {
			return nil, errors.New("unexpected terminal relay request shape")
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			return nil, err
		}
		var envelope v2Envelope
		if err := json.Unmarshal(body, &envelope); err != nil {
			return nil, err
		}
		plaintext, peerStatic, handshake, err := openV2Request(
			http.MethodPost, request.URL.Path, envelope, targetKey, nil,
		)
		if err != nil {
			return nil, err
		}
		if !bytes.Equal(peerStatic, controllerKey.Public) {
			return nil, errors.New("terminal relay request did not authenticate controller")
		}
		var poll TerminalRelayPollRequest
		if err := decodeV2Payload(plaintext, &poll); err != nil ||
			len(poll.SessionIDs) != 1 || poll.SessionIDs[0] != sessionID {
			return nil, errors.New("terminal relay poll payload was not decoded")
		}
		payload, err := json.Marshal(TerminalRelayPollResponse{
			Epoch: strings.Repeat("e", 32),
			Command: &TerminalRelayCommand{
				ID: commandID, Path: v2TerminalOpenPath, SessionID: sessionID,
				Payload: func() json.RawMessage {
					value, _ := json.Marshal(TerminalOpenRequest{Rows: 24, Columns: 80})
					return value
				}(), ExpiresAt: now.Add(time.Minute).Unix(),
			},
		})
		if err != nil {
			return nil, err
		}
		sealed, err := sealV2Response(envelope, handshake, payload)
		if err != nil {
			return nil, err
		}
		content, err := json.Marshal(sealed)
		if err != nil {
			return nil, err
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"application/json"}},
			Body:       io.NopCloser(bytes.NewReader(content)),
		}, nil
	})}
	relay, err := NewTerminalRelayClient(client.client)
	if err != nil {
		t.Fatalf("NewTerminalRelayClient() error = %v", err)
	}
	got, err := relay.PollV2(
		context.Background(), "https://panel.example.com:443", controllerID, targetID,
		controllerKey, targetKey.Public, now,
		TerminalRelayPollRequest{SessionIDs: []string{sessionID}},
	)
	if err != nil {
		t.Fatalf("PollV2() error = %v", err)
	}
	if got.Epoch != strings.Repeat("e", 32) || got.Command == nil || got.Command.ID != commandID || got.Command.Path != v2TerminalOpenPath {
		t.Fatalf("PollV2() = %#v, want a validated open command", got)
	}
}
