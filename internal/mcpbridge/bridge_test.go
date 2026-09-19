package mcpbridge

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestBridgeRoundTrip(t *testing.T) {
	remote := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	remote.AddTool(&mcp.Tool{Name: "echo", InputSchema: map[string]any{"type": "object"}}, func(_ context.Context, r *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(r.Params.Arguments)}}}, nil
	})
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return remote }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			w.WriteHeader(401)
			return
		}
		handler.ServeHTTP(w, r)
	}))
	defer web.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	left, right := mcp.NewInMemoryTransports()
	done := make(chan error, 1)
	go func() { done <- Run(ctx, web.URL+"/mcp", "test-token", left) }()
	client := mcp.NewClient(&mcp.Implementation{Name: "desktop", Version: "1"}, nil)
	session, err := client.Connect(ctx, right, nil)
	if err != nil {
		t.Fatal(err)
	}
	list, err := session.ListTools(ctx, nil)
	if err != nil || len(list.Tools) != 1 || list.Tools[0].Name != "echo" {
		t.Fatalf("tool discovery: %#v %v", list, err)
	}
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "echo", Arguments: map[string]any{"value": "hello"}})
	if err != nil || result.IsError || result.Content[0].(*mcp.TextContent).Text != `{"value":"hello"}` {
		t.Fatalf("tool forwarding: %#v %v", result, err)
	}
	session.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("bridge did not stop after stdio disconnect")
	}
}

func TestBridgeDestinationBoundary(t *testing.T) {
	for _, endpoint := range []string{"http://remote.example/mcp", "https://user:password@panel.example/mcp", "https://panel.example/mcp?token=secret", "https://panel.example/api/v1/terminal"} {
		if ValidateEndpoint(endpoint) == nil {
			t.Fatalf("accepted %s", endpoint)
		}
	}
	for _, endpoint := range []string{"https://panel.example/mcp", "http://127.0.0.1:9000/mcp", "http://[::1]:9000/mcp"} {
		if err := ValidateEndpoint(endpoint); err != nil {
			t.Fatal(err)
		}
	}
}
