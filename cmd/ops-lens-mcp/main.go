package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/frank/ops-lens-mcp/internal/config"
	"github.com/frank/ops-lens-mcp/internal/server"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

var (
	version = "0.1.0"
	commit  = "none"
	date    = "unknown"
)

func main() {
	configPath := flag.String("config", "config.yaml", "Path to targets configuration YAML file")
	showVersion := flag.Bool("version", false, "Print version information and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("ops-lens-mcp version %s (commit: %s, built: %s)\n", version, commit, date)
		os.Exit(0)
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("failed to load configuration from %s: %v", *configPath, err)
	}

	srv, err := server.New(cfg, version)
	if err != nil {
		log.Fatalf("failed to initialize ops-lens server: %v", err)
	}
	defer srv.Close()

	// Run MCP over standard I/O with signal handling
	if err := mcpserver.ServeStdio(srv.MCPServer()); err != nil {
		log.Fatalf("mcp server exited: %v", err)
	}
}
