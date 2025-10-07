package main

import (
	"context"
	"flag"
	"log"
	"os"

	"github.com/cloudnationhq/az-cn-wam-mcp/pkg/mcp"
)

func main() {
	org := flag.String("org", "cloudnationhq", "GitHub organization name")
	token := flag.String("token", "", "GitHub personal access token (optional, for higher rate limits)")
	dbPath := flag.String("db", "index.db", "Path to SQLite database file")
	flag.Parse()

	log.SetOutput(os.Stderr)
	log.Println("Starting Azure CloudNation WAM MCP Server")
	log.Printf("Database will be initialized at: %s (on first sync)", *dbPath)

	server := mcp.NewServer(*dbPath, *token, *org)
	if err := server.Run(context.Background(), os.Stdin, os.Stdout); err != nil {
		log.Printf("Server stopped: %v", err)
	}
}
