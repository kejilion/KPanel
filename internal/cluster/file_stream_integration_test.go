//go:build linux

package cluster_test

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/agent"
	"github.com/kejilion/kejilion-panel/internal/cluster"
	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/filemanager"
)

type streamTelemetry struct{}

func (streamTelemetry) Telemetry(context.Context) (contract.HostTelemetry, error) {
	return contract.HostTelemetry{AgentVersion: "1.14.1", AgentProtocolVersion: "v1alpha1", Hostname: "stream-test", OS: "Linux", OSID: "debian", Kernel: "6.12", Architecture: "amd64", CollectedAt: time.Now().UTC(), CPU: contract.CPUSummary{Cores: 2}, Memory: contract.MemorySummary{TotalBytes: 1 << 30}, Disk: contract.DiskCapacitySummary{TotalBytes: 1 << 30}}, nil
}

type streamResolver struct{}

func (streamResolver) LookupNetIP(context.Context, string, string) ([]net.IP, error) {
	return []net.IP{net.ParseIP("8.8.8.8")}, nil
}

type fileStreamNodeOptions struct {
	advertiseStream bool
	rejectStream    bool
	terminal        cluster.TerminalBackend
}

type fileStreamNode struct {
	service *cluster.Service
	server  *httptest.Server
	root    string
	manager *filemanager.Manager
	origin  string
	calls   atomic.Int32
	legacy  atomic.Int32
	// terminalLegacy counts v2 POST terminal requests.
	terminalLegacy atomic.Int32
}

// A real TLS server and approved-IP RemoteClient exercise pairing and file relay.
// Only the final dial is mapped into loopback; no public server is contacted.
func newFileStreamNode(t *testing.T) *fileStreamNode {
	t.Helper()
	return newFileStreamNodeWith(t, fileStreamNodeOptions{})
}

func newFileStreamNodeWith(t *testing.T, options fileStreamNodeOptions) *fileStreamNode {
	t.Helper()
	n := &fileStreamNode{root: t.TempDir()}
	var err error
	n.manager, err = filemanager.New(filemanager.Config{Root: n.root})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { n.manager.Close() })
	n.server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == cluster.FileStreamV2Path {
			n.calls.Add(1)
			if options.rejectStream {
				http.NotFound(w, r)
				return
			}
			n.service.ServeFileStream(w, r, "198.51.100.20")
			return
		}
		if strings.Contains(r.URL.Path, "/files/") && r.URL.Path != "/api/v2/federation/files/link" {
			n.legacy.Add(1)
		}
		if strings.Contains(r.URL.Path, "/terminal/") {
			n.terminalLegacy.Add(1)
		}
		var envelope cluster.FederationEnvelopeV2
		if json.NewDecoder(r.Body).Decode(&envelope) != nil {
			http.Error(w, "invalid", 400)
			return
		}
		if r.URL.Path == "/api/v2/federation/files/open" || r.URL.Path == "/api/v2/federation/files/open-linked" {
			n.serveLegacyFileOpen(w, r, envelope)
			return
		}
		response, err := n.service.HandleFederationV2(r.Context(), "198.51.100.20", r.URL.Path, r.Header.Get(cluster.FederationCapabilitiesHeader), envelope)
		if err != nil {
			http.Error(w, err.Error(), 403)
			return
		}
		if options.advertiseStream && r.URL.Path == "/api/v2/federation/summary" {
			w.Header().Set(cluster.FederationCapabilitiesHeader, cluster.PanelStreamCapability)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	t.Cleanup(n.server.Close)
	_, port, _ := net.SplitHostPort(n.server.Listener.Addr().String())
	n.origin = "https://example.com:" + port
	roots := x509.NewCertPool()
	roots.AddCert(n.server.Certificate())
	remote, err := cluster.NewRemoteClient(cluster.RemoteClientConfig{RootCAs: roots, Resolver: streamResolver{}, Dialer: func(ctx context.Context, network, address string) (net.Conn, error) {
		_, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		return (&net.Dialer{}).DialContext(ctx, network, net.JoinHostPort("127.0.0.1", port))
	}})
	if err != nil {
		t.Fatal(err)
	}
	n.service, err = cluster.NewService(cluster.ServiceConfig{DataDir: t.TempDir(), PublicURL: n.origin, Telemetry: streamTelemetry{}, Remote: remote, PanelVersion: "1.14.1", Terminal: options.terminal})
	if err != nil {
		t.Fatal(err)
	}
	n.service.SetFileRelayHandler(agent.NewFileHandler(n.manager))
	t.Cleanup(func() { n.service.Close() })
	return n
}

func (n *fileStreamNode) serveLegacyFileOpen(w http.ResponseWriter, r *http.Request, envelope cluster.FederationEnvelopeV2) {
	linked := r.URL.Path == "/api/v2/federation/files/open-linked"
	var input cluster.FederationFileOpenRequest
	var authorization *cluster.FederationFileAuthorization
	var err error
	if linked {
		input, authorization, err = n.service.AuthorizeLinkedFederationFileV2("198.51.100.20", envelope)
	} else {
		input, authorization, err = n.service.AuthorizeFederationFileV2("198.51.100.20", envelope)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	defer authorization.Close()
	query := url.Values{"path": {input.Path}, "resourceVersion": {input.ResourceVersion}}
	request := httptest.NewRequest(http.MethodGet, "/v1/files/transfer/export?"+query.Encode(), nil)
	recorder := httptest.NewRecorder()
	agent.NewFileHandler(n.manager).ServeHTTP(recorder, request)
	response := recorder.Result()
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		http.Error(w, "agent export failed", http.StatusBadGateway)
		return
	}
	rawMetadata, err := base64.RawURLEncoding.DecodeString(response.Header.Get("X-KPanel-File-Metadata"))
	var metadata contract.FileTransferMetadata
	if err != nil || json.Unmarshal(rawMetadata, &metadata) != nil {
		http.Error(w, "invalid metadata", http.StatusBadGateway)
		return
	}
	sealed, cipher, err := authorization.SealMetadata(metadata)
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	w.Header().Set("Content-Type", "application/x-kpanel-noise-stream")
	w.WriteHeader(http.StatusOK)
	if err := cluster.WriteFederationFileHeader(w, sealed); err != nil {
		return
	}
	writer := cluster.NewFederationFileWriter(w, cipher)
	_, copyErr := io.CopyBuffer(writer, response.Body, make([]byte, 60<<10))
	if copyErr == nil && response.Trailer.Get("X-KPanel-Transfer-Result") != "ok" {
		copyErr = errors.New("agent transfer failed")
	}
	_ = writer.Finish(copyErr)
}

func streamRead(t *testing.T, response *http.Response, err error, status int) []byte {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil || response.StatusCode != status {
		t.Fatalf("status=%d error=%v body=%s", response.StatusCode, err, data)
	}
	return data
}

func TestLegacyPanelFileRelayRealOperationsAndCrossNodeCopy(t *testing.T) {
	center, target := newFileStreamNode(t), newFileStreamNode(t)
	code, err := target.service.CreatePairingCodeV2()
	if err != nil {
		t.Fatal(err)
	}
	ctx, stop := context.WithTimeout(context.Background(), 25*time.Second)
	defer stop()
	host, err := center.service.AddHost(ctx, cluster.AddHostInput{Origin: target.origin, PairingCode: code.Code, ControllerOrigin: center.origin})
	if err != nil {
		t.Fatal(err)
	}
	content := bytes.Repeat([]byte("real-file\x00\xff"), 250000)
	open := func(input cluster.LightFileRequest) (*http.Response, error) {
		return center.service.OpenRemotePanelFile(ctx, host.ID, input)
	}
	response, err := open(cluster.LightFileRequest{Method: "POST", Path: "/v1/files/upload", RawQuery: "path=%2F&name=large.bin", Headers: map[string]string{"Content-Type": "application/octet-stream"}, Body: bytes.NewReader(content), BodyLength: int64(len(content))})
	streamRead(t, response, err, 201)
	stored, err := os.ReadFile(filepath.Join(target.root, "large.bin"))
	if err != nil || !bytes.Equal(stored, content) {
		t.Fatalf("upload stored incorrectly: %v", err)
	}
	response, err = open(cluster.LightFileRequest{Method: "GET", Path: "/v1/files", RawQuery: "path=%2F"})
	listing := streamRead(t, response, err, 200)
	if !bytes.Contains(listing, []byte("large.bin")) {
		t.Fatal("uploaded file missing in directory")
	}
	response, err = open(cluster.LightFileRequest{Method: "GET", Path: "/v1/files/content", RawQuery: "path=%2Flarge.bin", Headers: map[string]string{"Range": "bytes=11-31"}})
	if got := streamRead(t, response, err, 206); !bytes.Equal(got, content[11:32]) {
		t.Fatal("range mismatch")
	}
	response, err = open(cluster.LightFileRequest{Method: "GET", Path: "/v1/files/content", RawQuery: "path=%2Flarge.bin"})
	if got := streamRead(t, response, err, 200); !bytes.Equal(got, content) {
		t.Fatal("download mismatch")
	}
	entry, err := target.manager.Stat("/large.bin")
	if err != nil {
		t.Fatal(err)
	}
	reader, metadata, err := center.service.OpenRemoteFileV2(ctx, target.service.NodeID(), cluster.FederationFileOpenRequest{Path: entry.Path, ResourceVersion: entry.ResourceVersion})
	if err != nil {
		t.Fatal(err)
	}
	// Import the authenticated remote file into the local Agent.
	request := httptest.NewRequest("POST", "/v1/files/transfer/import?path=%2F&name=copied.bin&kind=file&size="+fmt.Sprint(metadata.SizeBytes), reader)
	request.Header.Set("Content-Type", "application/octet-stream")
	request.ContentLength = -1
	recorder := httptest.NewRecorder()
	agent.NewFileHandler(center.manager).ServeHTTP(recorder, request)
	reader.Close()
	if recorder.Code != 201 {
		t.Fatalf("import: %d %s", recorder.Code, recorder.Body.String())
	}
	stored, err = os.ReadFile(filepath.Join(center.root, "copied.bin"))
	if err != nil || !bytes.Equal(stored, content) {
		t.Fatalf("copied data: %v", err)
	}
	// The reverse linked grant keeps the previous export-only relay scope.
	entry, _ = center.manager.Stat("/copied.bin")
	reader, _, err = target.service.OpenRemoteFileV2(ctx, center.service.NodeID(), cluster.FederationFileOpenRequest{Path: entry.Path, ResourceVersion: entry.ResourceVersion})
	if err != nil {
		t.Fatal(err)
	}
	stored, err = io.ReadAll(reader)
	reader.Close()
	if err != nil || !bytes.Equal(stored, content) {
		t.Fatalf("linked copy: %v", err)
	}
	if target.calls.Load() != 0 || center.calls.Load() != 0 || target.legacy.Load() == 0 || center.legacy.Load() == 0 {
		t.Fatalf("Panel relay did not stay on legacy endpoints: target stream=%d legacy=%d center stream=%d legacy=%d",
			target.calls.Load(), target.legacy.Load(), center.calls.Load(), center.legacy.Load())
	}
	t.Logf("real TLS + Agent: %d bytes upload/download/copy, range and reverse linked export verified over the v1.14.1 Panel relay", len(content))
}

func TestUnifiedFileStreamLightRealUploadDownloadAndCopy(t *testing.T) {
	center := newFileStreamNode(t)
	root := t.TempDir()
	manager, err := filemanager.New(filemanager.Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	enrollment, err := center.service.CreateLightEnrollment()
	if err != nil {
		t.Fatal(err)
	}
	fields := strings.Fields(enrollment.Command)
	key, err := cluster.GenerateFederationV2Keypair()
	if err != nil {
		t.Fatal(err)
	}
	enrolled, err := center.service.EnrollLightNode("198.51.100.10", cluster.LightEnrollRequest{Token: strings.Trim(fields[len(fields)-1], "'"), Name: "real-light", NodeVersion: "1.14.1", TerminalPublicKey: base64.RawURLEncoding.EncodeToString(key.Public)})
	if err != nil {
		t.Fatal(err)
	}
	peer, _ := base64.RawURLEncoding.DecodeString(enrolled.TerminalPeerPublicKey)
	relay, _ := cluster.NewFileRelayClient(center.server.Client())
	ctx, stop := context.WithTimeout(context.Background(), 20*time.Second)
	defer stop()
	done := make(chan error, 1)
	go func() {
		done <- relay.RunFileStream(ctx, center.server.URL, enrolled.NodeID, enrolled.TargetNodeID, key, peer, agent.NewFileHandler(manager))
	}()
	for {
		host, err := center.service.Host(ctx, enrolled.NodeID)
		if err == nil && host.FileManagementAvailable {
			break
		}
		select {
		case err := <-done:
			t.Fatal(err)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(5 * time.Millisecond):
		}
	}
	content := bytes.Repeat([]byte("light-real\x00"), 180000)
	response, err := center.service.OpenLightFile(ctx, enrolled.NodeID, cluster.LightFileRequest{Method: "POST", Path: "/v1/files/upload", RawQuery: "path=%2F&name=light.bin", Headers: map[string]string{"Content-Type": "application/octet-stream"}, Body: bytes.NewReader(content), BodyLength: -1})
	streamRead(t, response, err, 201)
	response, err = center.service.OpenLightFile(ctx, enrolled.NodeID, cluster.LightFileRequest{Method: "GET", Path: "/v1/files/content", RawQuery: "path=%2Flight.bin"})
	if got := streamRead(t, response, err, 200); !bytes.Equal(got, content) {
		t.Fatal("light download mismatch")
	}
	entry, _ := manager.Stat("/light.bin")
	reader, metadata, err := center.service.OpenLightFileTransfer(ctx, enrolled.NodeID, cluster.FederationFileOpenRequest{Path: entry.Path, ResourceVersion: entry.ResourceVersion})
	if err != nil {
		t.Fatal(err)
	}
	query := url.Values{"path": {"/"}, "name": {"copy.bin"}, "kind": {metadata.Kind}, "size": {fmt.Sprint(metadata.SizeBytes)}}
	response, err = center.service.OpenLightFile(ctx, enrolled.NodeID, cluster.LightFileRequest{Method: "POST", Path: "/v1/files/transfer/import", RawQuery: query.Encode(), Headers: map[string]string{"Content-Type": "application/octet-stream"}, Body: reader, BodyLength: -1})
	streamRead(t, response, err, 201)
	reader.Close()
	stored, err := os.ReadFile(filepath.Join(root, "copy.bin"))
	if err != nil || !bytes.Equal(stored, content) {
		t.Fatalf("light copy: %v", err)
	}
	if center.calls.Load() == 0 || center.legacy.Load() != 0 {
		t.Fatalf("light file operations left WebSocket transport: stream=%d polling=%d", center.calls.Load(), center.legacy.Load())
	}
	stop()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("light broker did not stop")
	}
	t.Logf("real light Agent: %d bytes upload/download/stream-to-stream copy verified", len(content))
}
