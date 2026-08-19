package cluster

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

type fileV2RoundTripper func(*http.Request) (*http.Response, error)

func (f fileV2RoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestFederationFileStreamUsesHandshakeTransportCipherAndAuthenticatedEnd(t *testing.T) {
	controllerKey, err := v2NoiseSuite.GenerateKeypair(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	targetKey, err := v2NoiseSuite.GenerateKeypair(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	request := v2Envelope{
		Protocol: FederationProtocolV2, ControllerID: "aabbccddeeff00112233445566778899",
		TargetID: "11223344556677889900aabbccddeeff", Timestamp: time.Now().UTC().Unix(),
		RequestID: "00112233445566778899aabbccddeeff",
	}
	sealed, initiator, err := sealV2Request(
		"POST", v2FileOpenPath, request, controllerKey, targetKey.Public, nil,
		[]byte(`{"path":"/app","resourceVersion":"sha256:source"}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	_, _, responder, err := openV2Request("POST", v2FileOpenPath, sealed, targetKey, nil)
	if err != nil {
		t.Fatal(err)
	}
	authorization := &FederationFileAuthorization{request: sealed, handshake: responder}
	response, encrypt, err := authorization.SealMetadata(contract.FileTransferMetadata{
		Name: "app", Kind: "directory", ResourceVersion: "sha256:source",
	})
	if err != nil {
		t.Fatal(err)
	}
	message, err := base64.RawURLEncoding.DecodeString(response.Message)
	if err != nil {
		t.Fatal(err)
	}
	_, _, decrypt, err := initiator.ReadMessage(nil, message)
	if err != nil || decrypt == nil {
		t.Fatalf("finish initiator handshake: decrypt=%v err=%v", decrypt, err)
	}

	content := bytes.Repeat([]byte("kpanel-transfer-"), 8_000)
	var stream bytes.Buffer
	writer := NewFederationFileWriter(&stream, encrypt)
	if _, err := writer.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := writer.Finish(nil); err != nil {
		t.Fatal(err)
	}
	reader := &federationFileReader{
		source: bufio.NewReader(bytes.NewReader(stream.Bytes())),
		body:   io.NopCloser(bytes.NewReader(nil)), cipher: decrypt,
	}
	plain, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(plain, content) {
		t.Fatalf("stream content mismatch: got=%d want=%d", len(plain), len(content))
	}
}

func TestOpenLinkedFileV2UsesReverseStaticKeysAndDedicatedPath(t *testing.T) {
	nodeKey, err := v2NoiseSuite.GenerateKeypair(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	controllerKey, err := v2NoiseSuite.GenerateKeypair(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	linkID := "aabbccddeeff00112233445566778899"
	targetID := "11223344556677889900aabbccddeeff"
	input := FederationFileOpenRequest{
		Path: "/reports/status.txt", ResourceVersion: "sha256:linked-source",
	}
	metadata := contract.FileTransferMetadata{
		Name: "status.txt", Kind: "file", SizeBytes: 18,
		ResourceVersion: input.ResourceVersion,
	}
	content := []byte("linked file stream")
	metadata.SizeBytes = int64(len(content))

	client := &RemoteClient{}
	client.streamClient = &http.Client{Transport: fileV2RoundTripper(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodPost || request.URL.Path != v2FileLinkedOpenPath {
			t.Fatalf("linked request = %s %s", request.Method, request.URL.Path)
		}
		var envelope v2Envelope
		decoder := json.NewDecoder(request.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&envelope); err != nil {
			t.Fatalf("decode linked envelope: %v", err)
		}
		payload, peerStatic, responder, err := openV2Request(
			http.MethodPost, v2FileLinkedOpenPath, envelope, controllerKey, nil,
		)
		if err != nil {
			t.Fatalf("open linked request: %v", err)
		}
		if !bytes.Equal(peerStatic, nodeKey.Public) {
			t.Fatal("linked request did not authenticate the peer node identity")
		}
		var opened FederationFileOpenRequest
		if err := decodeV2Payload(payload, &opened); err != nil || opened != input {
			t.Fatalf("linked payload = %#v, err=%v", opened, err)
		}
		authorization := &FederationFileAuthorization{request: envelope, handshake: responder}
		sealedMetadata, encrypt, err := authorization.SealMetadata(metadata)
		if err != nil {
			t.Fatalf("seal linked metadata: %v", err)
		}
		var body bytes.Buffer
		if err := WriteFederationFileHeader(&body, sealedMetadata); err != nil {
			t.Fatalf("write linked header: %v", err)
		}
		writer := NewFederationFileWriter(&body, encrypt)
		if _, err := writer.Write(content); err != nil {
			t.Fatalf("write linked content: %v", err)
		}
		if err := writer.Finish(nil); err != nil {
			t.Fatalf("finish linked content: %v", err)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{fileStreamContentType}},
			Body:       io.NopCloser(bytes.NewReader(body.Bytes())),
			Request:    request,
		}, nil
	})}

	reader, openedMetadata, err := client.OpenLinkedFileV2(
		context.Background(), "https://center.example", linkID, targetID,
		nodeKey, controllerKey.Public, now, input,
	)
	if err != nil {
		t.Fatalf("OpenLinkedFileV2() error = %v", err)
	}
	defer reader.Close()
	if openedMetadata != metadata {
		t.Fatalf("linked metadata = %#v, want %#v", openedMetadata, metadata)
	}
	openedContent, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read linked content: %v", err)
	}
	if !bytes.Equal(openedContent, content) {
		t.Fatalf("linked content = %q, want %q", openedContent, content)
	}
}

func TestAuthorizeLinkedFederationFileV2RequiresActiveMatchingParentHost(t *testing.T) {
	now := time.Date(2026, 8, 16, 8, 30, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	service, err := NewService(ServiceConfig{
		DataDir: t.TempDir(), Hostname: "center", Now: clock,
		Telemetry: serviceTestTelemetry{now: clock, hostname: "center"},
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	t.Cleanup(func() { _ = service.Close() })

	controllerKey, err := v2NoiseSuite.GenerateKeypair(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	peerNodeKey, err := v2NoiseSuite.GenerateKeypair(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	hostID := "11111111111111111111111111111111"
	credentialFile, err := service.secretsV2.WriteHostCredential(hostID, v2Credential{
		ControllerPrivate: controllerKey.Private,
		ControllerPublic:  controllerKey.Public,
		TargetPublic:      peerNodeKey.Public,
	})
	if err != nil {
		t.Fatalf("WriteHostCredential() error = %v", err)
	}
	host := hostRecordV2{
		ID: hostID, Name: "peer", Origin: "https://peer.example",
		TransportSecurity:  TransportSecurityTLS,
		RemoteNodeID:       "22222222222222222222222222222222",
		ControllerID:       "33333333333333333333333333333333",
		State:              hostStateV2Active,
		TransactionID:      "44444444444444444444444444444444",
		CredentialFile:     credentialFile,
		TargetPublicKey:    base64.RawURLEncoding.EncodeToString(peerNodeKey.Public),
		PeerFingerprint:    fingerprintV2(peerNodeKey.Public),
		FederationProtocol: FederationProtocolV2,
		Scope:              SummaryTerminalFilesScope,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := service.storeV2.AddHost(host); err != nil {
		t.Fatalf("AddHost() error = %v", err)
	}
	host, err = service.storeV2.Host(hostID)
	if err != nil {
		t.Fatalf("Host() error = %v", err)
	}
	grant, err := service.filePeersV2.PrepareGrant(host, "https://center.example", now)
	if err != nil {
		t.Fatalf("PrepareGrant() error = %v", err)
	}
	grant, err = service.filePeersV2.ActivateGrant(grant.LinkID, now)
	if err != nil {
		t.Fatalf("ActivateGrant() error = %v", err)
	}

	input := FederationFileOpenRequest{
		Path: "/reports/status.txt", ResourceVersion: "sha256:linked-source",
	}
	seal := func(requestID string) v2Envelope {
		t.Helper()
		payload, marshalErr := json.Marshal(input)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		envelope, _, sealErr := sealV2Request(
			http.MethodPost,
			v2FileLinkedOpenPath,
			v2Envelope{
				Protocol: FederationProtocolV2, ControllerID: grant.LinkID,
				TargetID: service.store.NodeID(), Timestamp: now.Unix(), RequestID: requestID,
			},
			peerNodeKey,
			controllerKey.Public,
			nil,
			payload,
		)
		if sealErr != nil {
			t.Fatalf("seal linked request: %v", sealErr)
		}
		return envelope
	}

	opened, authorization, err := service.AuthorizeLinkedFederationFileV2(
		"203.0.113.10", seal("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
	)
	if err != nil {
		t.Fatalf("AuthorizeLinkedFederationFileV2() error = %v", err)
	}
	if opened != input || authorization == nil {
		t.Fatalf("linked authorization = %#v, %#v", opened, authorization)
	}
	_, secondAuthorization, err := service.AuthorizeLinkedFederationFileV2(
		"203.0.113.10", seal("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"),
	)
	if err != nil || secondAuthorization == nil {
		t.Fatalf("second linked authorization = %#v, err=%v", secondAuthorization, err)
	}
	_, thirdAuthorization, err := service.AuthorizeLinkedFederationFileV2(
		"203.0.113.10", seal("cccccccccccccccccccccccccccccccc"),
	)
	if !errors.Is(err, ErrRateLimited) || thirdAuthorization != nil {
		t.Fatalf("third linked authorization = %#v, err=%v, want ErrRateLimited", thirdAuthorization, err)
	}
	if err := authorization.Close(); err != nil {
		t.Fatal(err)
	}
	if err := authorization.Close(); err != nil {
		t.Fatalf("idempotent authorization close: %v", err)
	}
	_, replacementAuthorization, err := service.AuthorizeLinkedFederationFileV2(
		"203.0.113.10", seal("dddddddddddddddddddddddddddddddd"),
	)
	if err != nil || replacementAuthorization == nil {
		t.Fatalf("replacement linked authorization = %#v, err=%v", replacementAuthorization, err)
	}
	_ = secondAuthorization.Close()
	_ = replacementAuthorization.Close()

	host.State = hostStateV2PendingRevoke
	host.UpdatedAt = now.Add(time.Second)
	if _, err := service.storeV2.UpdateHost(host, host.ResourceVersion); err != nil {
		t.Fatalf("UpdateHost() error = %v", err)
	}
	_, _, err = service.AuthorizeLinkedFederationFileV2(
		"203.0.113.10", seal("eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"),
	)
	if !errors.Is(err, ErrAuthentication) {
		t.Fatalf("revoked parent authorization error = %v, want ErrAuthentication", err)
	}
}

func TestFileStreamLimiterBoundsGlobalAndPerPeer(t *testing.T) {
	global := newFileStreamLimiter(8, 2)
	releases := make([]func(), 0, 8)
	for index, digit := range []string{"1", "2", "3", "4", "5", "6", "7", "8"} {
		release, ok := global.acquire(strings.Repeat(digit, 32))
		if !ok {
			t.Fatalf("global acquire %d rejected", index+1)
		}
		releases = append(releases, release)
	}
	if release, ok := global.acquire(strings.Repeat("9", 32)); ok || release != nil {
		t.Fatal("ninth global file stream unexpectedly acquired")
	}
	releases[0]()
	replacement, ok := global.acquire(strings.Repeat("9", 32))
	if !ok || replacement == nil {
		t.Fatal("global file stream slot was not released")
	}
	replacement()
	for _, release := range releases[1:] {
		release()
	}

	perPeer := newFileStreamLimiter(8, 2)
	peerID := strings.Repeat("a", 32)
	first, firstOK := perPeer.acquire(peerID)
	second, secondOK := perPeer.acquire(peerID)
	third, thirdOK := perPeer.acquire(peerID)
	if !firstOK || !secondOK || thirdOK || third != nil {
		t.Fatalf("per-peer acquisitions = %v/%v/%v", firstOK, secondOK, thirdOK)
	}
	first()
	first()
	replacement, ok = perPeer.acquire(peerID)
	if !ok || replacement == nil {
		t.Fatal("per-peer file stream slot was not released idempotently")
	}
	second()
	replacement()
}

func TestOpenRemoteFileV2UsesLinkedRouteOnlyWithoutAnyDirectHost(t *testing.T) {
	now := time.Date(2026, 8, 16, 9, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	service, err := NewService(ServiceConfig{
		DataDir: t.TempDir(), Hostname: "peer", Now: clock,
		Telemetry: serviceTestTelemetry{now: clock, hostname: "peer"},
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	t.Cleanup(func() { _ = service.Close() })

	controllerKey, err := v2NoiseSuite.GenerateKeypair(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	controller := controllerRecordV2{
		ID: "55555555555555555555555555555555", Name: "center",
		PublicKey:     base64.RawURLEncoding.EncodeToString(controllerKey.Public),
		Fingerprint:   fingerprintV2(controllerKey.Public),
		Scope:         SummaryTerminalFilesScope,
		State:         controllerStateV2Active,
		TransactionID: "66666666666666666666666666666666",
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	service.storeV2.mu.Lock()
	service.storeV2.state.Controllers = append(service.storeV2.state.Controllers, controller)
	persistErr := service.storeV2.persistValidatedLocked()
	service.storeV2.mu.Unlock()
	if persistErr != nil {
		t.Fatalf("persist controller: %v", persistErr)
	}
	peerNodeID := "77777777777777777777777777777777"
	linkID := "88888888888888888888888888888888"
	if _, err := service.filePeersV2.GrantRoute(
		controller, linkID, peerNodeID, "https://center.example", now,
	); err != nil {
		t.Fatalf("GrantRoute() error = %v", err)
	}

	client, ok := service.remoteV2.(*RemoteClient)
	if !ok {
		t.Fatalf("remoteV2 = %T, want *RemoteClient", service.remoteV2)
	}
	requestCount := 0
	content := []byte("route content")
	metadata := contract.FileTransferMetadata{
		Name: "status.txt", Kind: "file", SizeBytes: int64(len(content)),
		ResourceVersion: "sha256:route-source",
	}
	client.streamClient = &http.Client{Transport: fileV2RoundTripper(func(request *http.Request) (*http.Response, error) {
		requestCount++
		if request.URL.Path != v2FileLinkedOpenPath {
			t.Fatalf("route request path = %q", request.URL.Path)
		}
		var envelope v2Envelope
		if err := json.NewDecoder(request.Body).Decode(&envelope); err != nil {
			t.Fatalf("decode route envelope: %v", err)
		}
		_, peerStatic, responder, err := openV2Request(
			http.MethodPost, v2FileLinkedOpenPath, envelope, controllerKey, nil,
		)
		if err != nil {
			t.Fatalf("open route request: %v", err)
		}
		if !bytes.Equal(peerStatic, service.nodeIdentityV2.PublicKey) {
			t.Fatal("route did not use the node identity as initiator")
		}
		authorization := &FederationFileAuthorization{request: envelope, handshake: responder}
		sealedMetadata, encrypt, err := authorization.SealMetadata(metadata)
		if err != nil {
			t.Fatalf("seal route metadata: %v", err)
		}
		var body bytes.Buffer
		if err := WriteFederationFileHeader(&body, sealedMetadata); err != nil {
			t.Fatal(err)
		}
		writer := NewFederationFileWriter(&body, encrypt)
		if _, err := writer.Write(content); err != nil {
			t.Fatal(err)
		}
		if err := writer.Finish(nil); err != nil {
			t.Fatal(err)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{fileStreamContentType}},
			Body:       io.NopCloser(bytes.NewReader(body.Bytes())),
			Request:    request,
		}, nil
	})}

	input := FederationFileOpenRequest{
		Path: "/reports/status.txt", ResourceVersion: metadata.ResourceVersion,
	}
	sealForward := func(requestID string) v2Envelope {
		t.Helper()
		payload, marshalErr := json.Marshal(input)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		envelope, _, sealErr := sealV2Request(
			http.MethodPost,
			v2FileOpenPath,
			v2Envelope{
				Protocol: FederationProtocolV2, ControllerID: controller.ID,
				TargetID: service.store.NodeID(), Timestamp: now.Unix(), RequestID: requestID,
			},
			controllerKey,
			service.nodeIdentityV2.PublicKey,
			nil,
			payload,
		)
		if sealErr != nil {
			t.Fatalf("seal forward file request: %v", sealErr)
		}
		return envelope
	}
	_, forwardFirst, err := service.AuthorizeFederationFileV2(
		"203.0.113.20", sealForward(strings.Repeat("1", 32)),
	)
	if err != nil || forwardFirst == nil {
		t.Fatalf("first forward authorization = %#v, err=%v", forwardFirst, err)
	}
	_, forwardSecond, err := service.AuthorizeFederationFileV2(
		"203.0.113.20", sealForward(strings.Repeat("2", 32)),
	)
	if err != nil || forwardSecond == nil {
		t.Fatalf("second forward authorization = %#v, err=%v", forwardSecond, err)
	}
	_, forwardThird, err := service.AuthorizeFederationFileV2(
		"203.0.113.20", sealForward(strings.Repeat("3", 32)),
	)
	if !errors.Is(err, ErrRateLimited) || forwardThird != nil {
		t.Fatalf("third forward authorization = %#v, err=%v, want ErrRateLimited", forwardThird, err)
	}
	_ = forwardFirst.Close()
	_, forwardReplacement, err := service.AuthorizeFederationFileV2(
		"203.0.113.20", sealForward(strings.Repeat("4", 32)),
	)
	if err != nil || forwardReplacement == nil {
		t.Fatalf("replacement forward authorization = %#v, err=%v", forwardReplacement, err)
	}
	_ = forwardSecond.Close()
	_ = forwardReplacement.Close()

	reader, openedMetadata, err := service.OpenRemoteFileV2(
		context.Background(), peerNodeID, input,
	)
	if err != nil {
		t.Fatalf("OpenRemoteFileV2(route) error = %v", err)
	}
	openedContent, readErr := io.ReadAll(reader)
	_ = reader.Close()
	if readErr != nil || !bytes.Equal(openedContent, content) || openedMetadata != metadata {
		t.Fatalf("route result metadata=%#v content=%q err=%v", openedMetadata, openedContent, readErr)
	}
	if requestCount != 1 {
		t.Fatalf("route requests = %d, want 1", requestCount)
	}

	directHostID := "99999999999999999999999999999999"
	directTargetPublic := bytes.Repeat([]byte{9}, 32)
	directHost := hostRecordV2{
		ID: directHostID, Name: "inactive direct host", Origin: "https://direct.example",
		TransportSecurity:  TransportSecurityTLS,
		RemoteNodeID:       peerNodeID,
		ControllerID:       "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		State:              hostStateV2PendingRevoke,
		TransactionID:      "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		CredentialFile:     "host-" + directHostID + ".v2key",
		TargetPublicKey:    base64.RawURLEncoding.EncodeToString(directTargetPublic),
		PeerFingerprint:    fingerprintV2(directTargetPublic),
		FederationProtocol: FederationProtocolV2,
		Scope:              SummaryTerminalFilesScope,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := service.storeV2.AddHost(directHost); err != nil {
		t.Fatalf("AddHost(inactive direct) error = %v", err)
	}
	directHost, err = service.storeV2.Host(directHostID)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.OpenRemoteFileV2(context.Background(), peerNodeID, input); !errors.Is(err, ErrNotFound) {
		t.Fatalf("inactive direct host error = %v, want ErrNotFound", err)
	}
	if requestCount != 1 {
		t.Fatalf("inactive direct host fell back to route; requests=%d", requestCount)
	}
	if _, err := service.storeV2.DeleteHost(directHostID, directHost.ResourceVersion); err != nil {
		t.Fatalf("DeleteHost(inactive direct) error = %v", err)
	}
	if _, err := service.storeV2.RevokeController(
		controller.ID, controller.TransactionID, now.Add(time.Second), now.Add(10*time.Minute),
	); err != nil {
		t.Fatalf("RevokeController() error = %v", err)
	}
	if _, _, err := service.OpenRemoteFileV2(context.Background(), peerNodeID, input); !errors.Is(err, ErrNotFound) {
		t.Fatalf("revoked route parent error = %v, want ErrNotFound", err)
	}
	if requestCount != 1 {
		t.Fatalf("revoked route parent reached remote; requests=%d", requestCount)
	}
}

func TestFederationFileStreamRejectsTruncation(t *testing.T) {
	reader := &federationFileReader{
		source: bufio.NewReader(bytes.NewReader(nil)),
		body:   io.NopCloser(bytes.NewReader(nil)),
	}
	if _, err := reader.Read(make([]byte, 1)); err != io.ErrUnexpectedEOF {
		t.Fatalf("truncated stream error=%v", err)
	}
}

func TestFederationFileStreamClosesIdleSource(t *testing.T) {
	input, output := io.Pipe()
	reader := newIdleReadCloser(input, 20*time.Millisecond)
	t.Cleanup(func() {
		_ = reader.Close()
		_ = output.Close()
	})
	started := time.Now()
	_, err := reader.Read(make([]byte, 1))
	if err == nil {
		t.Fatal("idle read unexpectedly succeeded")
	}
	if elapsed := time.Since(started); elapsed < 15*time.Millisecond || elapsed > time.Second {
		t.Fatalf("idle read elapsed=%v", elapsed)
	}
}
