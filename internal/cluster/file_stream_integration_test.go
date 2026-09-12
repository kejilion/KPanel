//go:build linux

package cluster_test

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
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

type fileStreamNode struct {
	service *cluster.Service
	server  *httptest.Server
	root    string
	manager *filemanager.Manager
	origin  string
	calls   atomic.Int32
	legacy  atomic.Int32
}

// A real TLS server and approved-IP RemoteClient exercise pairing and streaming.
// Only the final dial is mapped into loopback; no public server is contacted.
func newFileStreamNode(t *testing.T) *fileStreamNode {
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
			n.service.ServeFileStream(w, r, "198.51.100.20")
			return
		}
		if strings.Contains(r.URL.Path, "/files/") && r.URL.Path != "/api/v2/federation/files/link" {
			n.legacy.Add(1)
		}
		var envelope cluster.FederationEnvelopeV2
		if json.NewDecoder(r.Body).Decode(&envelope) != nil {
			http.Error(w, "invalid", 400)
			return
		}
		response, err := n.service.HandleFederationV2(r.Context(), "198.51.100.20", r.URL.Path, r.Header.Get(cluster.FederationCapabilitiesHeader), envelope)
		if err != nil {
			http.Error(w, err.Error(), 403)
			return
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
	n.service, err = cluster.NewService(cluster.ServiceConfig{DataDir: t.TempDir(), PublicURL: n.origin, Telemetry: streamTelemetry{}, Remote: remote, PanelVersion: "1.14.1"})
	if err != nil {
		t.Fatal(err)
	}
	n.service.SetFileRelayHandler(agent.NewFileHandler(n.manager))
	t.Cleanup(func() { n.service.Close() })
	return n
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

func TestUnifiedFileStreamRealFileOperationsAndCrossNodeCopy(t *testing.T) {
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
	// Import a real file stream into the local Agent, checking authenticated EOF.
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
	// The reverse linked grant reuses the same transport with export-only scope.
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
	if target.legacy.Load() != 0 || center.legacy.Load() != 0 {
		t.Fatal("file operations used legacy endpoints")
	}
	t.Logf("real TLS + Agent: %d bytes upload/download/copy, range and reverse linked export verified; stream sockets target=%d center=%d", len(content), target.calls.Load(), center.calls.Load())
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
	if center.legacy.Load() != 0 {
		t.Fatal("light file operations used polling")
	}
	stop()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("light broker did not stop")
	}
	t.Logf("real light Agent: %d bytes upload/download/stream-to-stream copy verified", len(content))
}
