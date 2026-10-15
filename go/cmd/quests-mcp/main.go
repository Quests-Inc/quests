// Command quests-mcp is the Quests MCP server (stdio) for AI agents.
//
//	QUESTS_API=https://host QUESTS_API_TOKEN=… quests-mcp
//
// QUESTS_GUIDE_NOTE (optional) is the id of a vault note with a fuller agent guide.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/quests-inc/quests/go/internal/mcp"
)

var version = "dev"

func main() {
	api := flag.String("api", "", "Quests API base URL (else QUESTS_API, else http://127.0.0.1:8765)")
	flag.Parse()
	base := strings.TrimSpace(*api)
	if base == "" {
		base = strings.TrimSpace(os.Getenv("QUESTS_API"))
	}
	if base == "" {
		base = "http://127.0.0.1:8765"
	}
	srv := mcp.New(mcp.NewClient(base, os.Getenv("QUESTS_API_TOKEN")), version)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := srv.Serve(ctx, os.Stdin, os.Stdout); err != nil && ctx.Err() == nil {
		fmt.Fprintln(os.Stderr, "quests-mcp:", err)
		os.Exit(1)
	}
}
