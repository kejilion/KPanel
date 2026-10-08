package panel

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/cluster"
	"github.com/kejilion/kejilion-panel/internal/contract"
)

func TestPairedReceiveNegotiatesBeforeSendingUnsupportedRoutes(t *testing.T) {
	for _, transport := range []string{"v2", "v3"} {
		for _, receiver := range []string{"legacy", "legacy-panel-new-agent", "current", "denied"} {
			t.Run(transport+"/"+receiver, func(t *testing.T) {
				target, _ := newTestServer(t)
				_, _, root := realTransferAgent(t, target)
				handler := target.federatedFileHandler()
				var sessionCalls, imports atomic.Int32
				target.cluster.SetFileRelayHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/v1/files" {
						if receiver == "denied" {
							http.Error(w, "denied", http.StatusForbidden)
							return
						}
						if receiver != "current" {
							// A legacy Panel passes Agent JSON through unchanged,
							// even if that Agent independently supports sessions.
							list := contract.FileDirectory{Path: "/home", Entries: []contract.FileEntry{}, ReadAt: time.Now().UTC()}
							if receiver == "legacy-panel-new-agent" {
								list.FileReceiveVersion = 1
							}
							w.Header().Set("Content-Type", "application/json")
							_ = json.NewEncoder(w).Encode(list)
							return
						}
					}
					if r.URL.Path == "/v1/files/transfer/sessions" {
						sessionCalls.Add(1)
						if receiver != "current" {
							http.Error(w, "legacy route denied", http.StatusUnauthorized)
							return
						}
					}
					if r.URL.Path == "/v1/files/transfer/import" {
						imports.Add(1)
					}
					handler.ServeHTTP(w, r)
				}))
				network := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if transport == "v2" && r.URL.Path == "/api/v2/federation/summary" {
						recorded := httptest.NewRecorder()
						target.ServeHTTP(recorded, r)
						for key, values := range recorded.Header() {
							w.Header()[key] = values
						}
						w.Header().Del(cluster.FederationCapabilitiesHeader)
						w.WriteHeader(recorded.Code)
						_, _ = w.Write(recorded.Body.Bytes())
						return
					}
					target.ServeHTTP(w, r)
				}))
				t.Cleanup(network.Close)
				remote, err := cluster.NewRemoteClient(cluster.RemoteClientConfig{Dialer: func(ctx context.Context, networkName, _ string) (net.Conn, error) {
					return (&net.Dialer{}).DialContext(ctx, networkName, network.Listener.Addr().String())
				}})
				if err != nil {
					t.Fatal(err)
				}
				center, _ := newTestServer(t)
				_ = center.cluster.Close()
				center.cluster, err = cluster.NewService(cluster.ServiceConfig{DataDir: t.TempDir(), Telemetry: historyTestTelemetry{}, Remote: remote})
				if err != nil {
					t.Fatal(err)
				}
				code, err := target.cluster.CreatePairingCodeV2()
				if err != nil {
					t.Fatal(err)
				}
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()
				host, err := center.cluster.AddHost(ctx, cluster.AddHostInput{Origin: "http://8.8.8.8:1801", PairingCode: code.Code})
				if err != nil {
					t.Fatal(err)
				}
				input := contract.FileReceiveInput{Directory: "/home", Name: "copy.txt", Kind: "file", SizeBytes: 7, SourceKey: strings.Repeat("a", 64)}
				legacy := func(body io.ReadCloser) (contract.FileEntry, error) {
					response, err := center.openFileHostRequest(ctx, host.ID, host.Kind, cluster.LightFileRequest{Method: http.MethodPost, Path: "/v1/files/transfer/import", RawQuery: "path=%2Fhome&name=copy.txt&kind=file&size=7", Headers: map[string]string{"Content-Type": "application/octet-stream"}, Body: body, BodyLength: 7})
					if err != nil {
						return contract.FileEntry{}, err
					}
					defer response.Body.Close()
					var entry contract.FileEntry
					if response.StatusCode != http.StatusCreated {
						t.Fatalf("legacy status=%d", response.StatusCode)
					}
					err = json.NewDecoder(response.Body).Decode(&entry)
					return entry, err
				}
				entry, _, err := center.receiveFileTransfer(ctx, input, io.NopCloser(bytes.NewBufferString("payload")), nil, center.fileReceiver(host.ID, host.Kind, "compat"), func(contract.FileTransferEvent) bool { return true }, legacy)
				if receiver == "denied" {
					var denied *fileReceiveHTTPError
					if !errors.As(err, &denied) || denied.status != http.StatusForbidden || imports.Load() != 0 || sessionCalls.Load() != 0 {
						t.Fatalf("denial downgraded: %v imports=%d sessions=%d", err, imports.Load(), sessionCalls.Load())
					}
					return
				}
				if err != nil || entry.Path != "/home/copy.txt" {
					t.Fatalf("copy=%#v err=%v", entry, err)
				}
				got, err := os.ReadFile(filepath.Join(root, "home", "copy.txt"))
				if err != nil || string(got) != "payload" {
					t.Fatalf("target content=%q err=%v", got, err)
				}
				if receiver == "current" {
					if imports.Load() != 0 || sessionCalls.Load() == 0 {
						t.Fatal("current receiver took legacy route")
					}
				} else if imports.Load() != 1 || sessionCalls.Load() != 0 {
					t.Fatalf("unsupported route sent: imports=%d sessions=%d", imports.Load(), sessionCalls.Load())
				}
			})
		}
	}
}

// V1 enters the target's authenticated federated handler directly; V2/V3
// enters the cluster stream handler. Fault the shared Agent boundary for V1.
type receiveV1CapabilityAgent struct {
	*transferIntegrationAgent
	capability        string
	sessions, uploads atomic.Int32
}

func (a *receiveV1CapabilityAgent) Get(ctx context.Context, route, query, id string) (AgentResponse, error) {
	if route == "/v1/files" {
		if a.capability == "denied" {
			return AgentResponse{StatusCode: http.StatusForbidden, ContentType: "application/json", Body: []byte(`{"error":"denied"}`)}, nil
		}
		if a.capability == "legacy" {
			payload, _ := json.Marshal(contract.FileDirectory{Path: "/home", Entries: []contract.FileEntry{}})
			return AgentResponse{StatusCode: http.StatusOK, ContentType: "application/json", Body: payload}, nil
		}
	}
	return a.transferIntegrationAgent.Get(ctx, route, query, id)
}

func (a *receiveV1CapabilityAgent) OpenStream(ctx context.Context, method, route, query, id string, body io.Reader, headers http.Header, size int64) (*http.Response, error) {
	if route == "/v1/files/transfer/sessions" {
		a.sessions.Add(1)
	}
	if route == "/v1/files/upload" {
		a.uploads.Add(1)
	}
	return a.transferIntegrationAgent.OpenStream(ctx, method, route, query, id, body, headers, size)
}

func TestBrowserReceiveCapabilityUsesGrantedV1Relay(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("V1 credential permission validation requires Linux; covered by the Linux gate")
	}
	for _, capability := range []string{"current", "legacy", "denied"} {
		t.Run(capability, func(t *testing.T) {
			target, _ := newTestServerWithPublicURL(t, "https://example.com")
			integration, _, root := realTransferAgent(t, target)
			capabilityAgent := &receiveV1CapabilityAgent{transferIntegrationAgent: integration, capability: capability}
			target.agent = capabilityAgent
			_ = target.cluster.Close()
			var err error
			target.cluster, err = cluster.NewService(cluster.ServiceConfig{DataDir: t.TempDir(), Telemetry: historyTestTelemetry{}})
			if err != nil {
				t.Fatal(err)
			}
			network := httptest.NewTLSServer(target)
			t.Cleanup(network.Close)
			roots := x509.NewCertPool()
			roots.AddCert(network.Certificate())
			remote, err := cluster.NewRemoteClient(cluster.RemoteClientConfig{RootCAs: roots, Resolver: historyTestResolver{}, Dialer: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "tcp", network.Listener.Addr().String())
			}})
			if err != nil {
				t.Fatal(err)
			}
			center, tokenPath := newTestServer(t)
			cookie, csrf := bootstrapCookies(t, center, tokenPath)
			_ = center.cluster.Close()
			center.cluster, err = cluster.NewService(cluster.ServiceConfig{DataDir: t.TempDir(), Telemetry: historyTestTelemetry{}, Remote: remote})
			if err != nil {
				t.Fatal(err)
			}
			code, err := target.cluster.CreatePairingCode()
			if err != nil {
				t.Fatal(err)
			}
			host, err := center.cluster.AddHost(context.Background(), cluster.AddHostInput{Origin: "https://example.com", PairingCode: code.Code})
			if err != nil {
				t.Fatal(err)
			}
			controllers := target.cluster.Controllers()
			if len(controllers) != 1 {
				t.Fatal("controller missing")
			}
			if _, err := target.cluster.SetControllerFileRelay(controllers[0].ID, true); err != nil {
				t.Fatal(err)
			}
			center.cluster.Start(context.Background())
			_, err = center.cluster.Refresh(context.Background(), host.ID)
			if err != nil {
				t.Fatal(err)
			}
			deadline := time.Now().Add(3 * time.Second)
			for time.Now().Before(deadline) {
				host, err = center.cluster.Host(context.Background(), host.ID)
				if err != nil || host.FileManagementAvailable {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			if err != nil || !host.FileManagementAvailable {
				t.Fatalf("grant not available: %#v %v", host, err)
			}
			request := func(method, route string, payload []byte, media string) *httptest.ResponseRecorder {
				r := httptest.NewRequest(method, "http://panel.test"+route, bytes.NewReader(payload))
				r.Host = "panel.test"
				r.Header.Set("Origin", "http://panel.test")
				r.Header.Set("X-CSRF-Token", csrf.Value)
				r.Header.Set("Content-Type", media)
				r.AddCookie(cookie)
				r.AddCookie(csrf)
				w := httptest.NewRecorder()
				center.ServeHTTP(w, r)
				return w
			}
			input := contract.FileReceiveInput{Directory: "/home", Name: "v1.txt", Kind: "file", SizeBytes: 7, SourceKey: strings.Repeat("a", 64)}
			payload, _ := json.Marshal(contract.FileReceiveRequest{Operation: "create", Input: &input})
			created := request("POST", "/api/v1/files/transfer/sessions?hostId="+host.ID, payload, "application/json")
			if capability == "denied" {
				if created.Code != 503 || capabilityAgent.sessions.Load() != 0 || capabilityAgent.uploads.Load() != 0 {
					t.Fatalf("denial downgraded: %d", created.Code)
				}
				return
			}
			if capability == "legacy" {
				if created.Code != 404 || capabilityAgent.sessions.Load() != 0 {
					t.Fatalf("legacy capability=%d %s", created.Code, created.Body.String())
				}
				uploaded := request("POST", "/api/v1/files/upload?hostId="+host.ID+"&path=%2Fhome&name=v1.txt&overwrite=false", []byte("payload"), "application/octet-stream")
				if uploaded.Code != 201 || capabilityAgent.uploads.Load() != 1 {
					t.Fatalf("legacy upload=%d %s", uploaded.Code, uploaded.Body.String())
				}
			} else {
				if created.Code != 200 {
					t.Fatalf("v1 create=%d %s", created.Code, created.Body.String())
				}
				var session contract.FileReceiveSession
				_ = json.Unmarshal(created.Body.Bytes(), &session)
				digest := transferSourceKey("payload")
				chunk := request("PUT", "/api/v1/files/transfer/sessions?hostId="+host.ID+"&id="+session.ID+"&sourceKey="+input.SourceKey+"&offset=0&sha256="+digest, []byte("payload"), "application/octet-stream")
				if chunk.Code != 200 {
					t.Fatalf("v1 chunk=%d %s", chunk.Code, chunk.Body.String())
				}
				payload, _ = json.Marshal(contract.FileReceiveRequest{Operation: "commit", ID: session.ID, SourceKey: input.SourceKey, SizeBytes: 7, SHA256: digest})
				committed := request("POST", "/api/v1/files/transfer/sessions?hostId="+host.ID, payload, "application/json")
				if committed.Code != 200 {
					t.Fatalf("v1 commit=%d %s", committed.Code, committed.Body.String())
				}
			}
			got, err := os.ReadFile(filepath.Join(root, "home", "v1.txt"))
			if err != nil || string(got) != "payload" {
				t.Fatalf("upload result=%q %v", got, err)
			}
		})
	}
}
