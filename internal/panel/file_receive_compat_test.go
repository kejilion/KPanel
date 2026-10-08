package panel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
