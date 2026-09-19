// Package mcpbridge adapts a delegated KPanel HTTP connection to MCP stdio.
// It has no host privileges, local Agent access, database, or listening socket.
package mcpbridge

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/kejilion/kejilion-panel/internal/version"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func ValidateEndpoint(endpoint string) error {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "/mcp" || u.RawPath != "" {
		return errors.New("invalid KPanel MCP endpoint")
	}
	ip := net.ParseIP(u.Hostname())
	if u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || (ip != nil && ip.IsLoopback()))) {
		return errors.New("KPanel MCP requires HTTPS or loopback")
	}
	return nil
}

type authorizedTransport struct {
	base            http.RoundTripper
	endpoint, token string
}

func (t authorizedTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.String() != t.endpoint {
		return nil, errors.New("unexpected MCP destination")
	}
	copy := r.Clone(r.Context())
	copy.Header.Set("Authorization", "Bearer "+t.token)
	return t.base.RoundTrip(copy)
}

// Run only forwards discovered tools. Authorization, approvals, host identity,
// pagination and replay protection remain authoritative on the Panel server.
func Run(ctx context.Context, endpoint, token string, transport mcp.Transport) error {
	if err := ValidateEndpoint(endpoint); err != nil {
		return err
	}
	if token == "" || len(token) > 4096 || strings.ContainsAny(token, "\r\n") {
		return errors.New("set KPANEL_MCP_TOKEN to a delegated client credential")
	}
	base := http.DefaultTransport.(*http.Transport).Clone()
	base.DisableCompression = true
	defer base.CloseIdleConnections()
	httpClient := &http.Client{Transport: authorizedTransport{base, endpoint, token}, Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	// Protocol logging must not write to stdout or echo remote content/secrets.
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	client := mcp.NewClient(&mcp.Implementation{Name: "kpanel-mcp-bridge", Version: version.Version}, &mcp.ClientOptions{Logger: logger})
	remote, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: endpoint, HTTPClient: httpClient, MaxRetries: -1, DisableStandaloneSSE: true, MaxEventSize: 256 << 10}, nil)
	if err != nil {
		return errors.New("cannot connect to KPanel MCP; check endpoint, HTTPS and credential")
	}
	defer remote.Close()
	server := mcp.NewServer(&mcp.Implementation{Name: "kpanel", Version: version.Version}, &mcp.ServerOptions{Logger: logger, Instructions: remote.InitializeResult().Instructions})
	count := 0
	for tool, err := range remote.Tools(ctx, nil) {
		if err != nil {
			return errors.New("cannot discover KPanel MCP tools")
		}
		count++
		if count > 128 {
			return errors.New("KPanel tool limit exceeded")
		}
		name := tool.Name
		server.AddTool(tool, func(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			callCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
			defer cancel()
			result, err := remote.CallTool(callCtx, &mcp.CallToolParams{Name: name, Arguments: request.Params.Arguments})
			if err != nil {
				return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "kpanel_connection_failed_check_operation_status_before_retry"}}}, nil
			}
			return result, nil
		})
	}
	return server.Run(ctx, transport)
}
