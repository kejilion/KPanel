// kpanel-mcp is the optional desktop stdio bridge. Native HTTP clients do not
// need this binary or any additional server-side process.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"

	"github.com/kejilion/kejilion-panel/internal/mcpbridge"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	endpoint := flag.String("url", os.Getenv("KPANEL_MCP_URL"), "KPanel HTTPS /mcp endpoint")
	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: kpanel-mcp --url https://panel.example/mcp (credential: KPANEL_MCP_TOKEN)")
		os.Exit(2)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	if err := mcpbridge.Run(ctx, *endpoint, os.Getenv("KPANEL_MCP_TOKEN"), &mcp.StdioTransport{}); err != nil && ctx.Err() == nil {
		fmt.Fprintln(os.Stderr, "kpanel-mcp:", err)
		os.Exit(1)
	}
}
