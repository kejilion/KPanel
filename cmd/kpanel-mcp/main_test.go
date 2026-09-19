package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestCommandStdioDiscoveryCallRevocationAndExit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	binary := filepath.Join(t.TempDir(), "kpanel-mcp")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	build := exec.CommandContext(ctx, "go", "build", "-o", binary, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, output)
	}
	remote := mcp.NewServer(&mcp.Implementation{Name: "test-panel", Version: "1"}, nil)
	remote.AddTool(&mcp.Tool{Name: "echo", InputSchema: map[string]any{"type": "object"}}, func(_ context.Context, r *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(r.Params.Arguments)}}}, nil
	})
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return remote }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	var revoked atomic.Bool
	web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if revoked.Load() || r.Header.Get("Authorization") != "Bearer test-stdio-token" {
			w.WriteHeader(401)
			return
		}
		handler.ServeHTTP(w, r)
	}))
	defer web.Close()
	command := exec.CommandContext(ctx, binary, "--url", web.URL+"/mcp")
	command.Env = append(os.Environ(), "KPANEL_MCP_TOKEN=test-stdio-token")
	var stderr bytes.Buffer
	command.Stderr = &stderr
	client := mcp.NewClient(&mcp.Implementation{Name: "stdio-desktop", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: command, TerminateDuration: time.Second}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	list, err := session.ListTools(ctx, nil)
	if err != nil || len(list.Tools) != 1 {
		t.Fatalf("discovery: %#v %v", list, err)
	}
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "echo", Arguments: map[string]string{"value": "hello"}})
	if err != nil || result.IsError || result.Content[0].(*mcp.TextContent).Text != `{"value":"hello"}` {
		t.Fatalf("stdio call: %#v %v", result, err)
	}
	revoked.Store(true)
	result, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "echo", Arguments: map[string]string{}})
	if err != nil || !result.IsError {
		t.Fatalf("revoked credential forwarded: %#v %v", result, err)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	if command.ProcessState == nil || !command.ProcessState.Success() {
		t.Fatalf("bridge did not exit cleanly: %v", command.ProcessState)
	}
	if strings.Contains(stderr.String(), "test-stdio-token") || stderr.Len() != 0 {
		t.Fatal("unexpected bridge stderr", stderr.String())
	}
}
