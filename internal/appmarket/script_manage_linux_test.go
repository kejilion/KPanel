//go:build linux

package appmarket

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kejilion/kejilion-panel/internal/dockerx"
)

func TestStoppedEngineBindingsPreserveDockerActionsWithoutScriptManagement(t *testing.T) {
	for _, state := range []string{"created", "exited"} {
		for _, ip := range []string{"127.0.0.1", "::1", "0.0.0.0"} {
			t.Run(state+"/"+ip, func(t *testing.T) {
				root, err := os.MkdirTemp("", "app-bind-")
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = os.RemoveAll(root) })
				socket := filepath.Join(root, "docker.sock")
				listener, err := net.Listen("unix", socket)
				if err != nil {
					t.Fatal(err)
				}
				id := strings.Repeat("a", 64)
				server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Method != http.MethodGet {
						t.Errorf("unexpected Docker write: %s", r.Method)
						w.WriteHeader(500)
						return
					}
					w.Header().Set("Content-Type", "application/json")
					switch r.URL.Path {
					case "/containers/json":
						fmt.Fprintf(w, `[{"Id":%q,"Names":["/speedtest"],"State":%q}]`, id, state)
					case "/containers/" + id + "/json":
						fmt.Fprintf(w, `{"Id":%q,"Name":"/speedtest","Config":{"Image":"ghcr.io/librespeed/speedtest"},"State":{"Status":%q,"Running":false},"HostConfig":{"PortBindings":{"8080/tcp":[{"HostIp":%q,"HostPort":"8028"}]}},"NetworkSettings":{"Ports":null}}`, id, state, ip)
					default:
						http.NotFound(w, r)
					}
				})}
				go func() { _ = server.Serve(listener) }()
				t.Cleanup(func() { _ = server.Close() })
				client := dockerx.New(socket, root, root)
				client.ConfigureDaemonAccess("", true)
				service, err := New(client, root)
				if err != nil {
					t.Fatal(err)
				}
				if err := service.configureJobs(filepath.Join(root, "jobs"), filepath.Join(root, "agent"), &fakeJobRunner{}); err != nil {
					t.Fatal(err)
				}
				service.scriptInteractiveFinder = func() (string, error) { return "/usr/local/bin/k", nil }
				service.scriptInteractiveManageFinder = service.scriptInteractiveFinder
				service.scriptManageFinder = service.scriptInteractiveFinder
				item, err := service.Find(context.Background(), "builtin-28")
				if err != nil {
					t.Fatal(err)
				}
				if len(item.Runtime.Ports) != 1 || item.Runtime.Ports[0].IP != ip {
					t.Fatalf("configured binding lost: %#v", item.Runtime)
				}
				manage := item.Capabilities["manage"]
				expectedManageReason := "该应用使用 KPanel 常规管理入口"
				if ip != "0.0.0.0" {
					expectedManageReason = "当前脚本不支持保留或转换 Docker 回环端口绑定；需先完成端口绑定兼容处理"
				}
				if manage.Enabled || manage.Reason != expectedManageReason {
					t.Fatalf("docker app exposed script management: %#v", item.Capabilities)
				}
				for _, action := range []string{"update", "direct_access"} {
					if item.Capabilities[action].Enabled != (ip == "0.0.0.0") {
						t.Fatalf("%s ignored configured binding: %#v", action, item.Capabilities)
					}
				}
			})
		}
	}
}
