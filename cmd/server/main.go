package main

import (
	"context"
	"flag"
	"log"
	"os"

	"github.com/cloudnationhq/az-cn-wam-mcp/internal/database"
	"github.com/cloudnationhq/az-cn-wam-mcp/internal/indexer"
	"github.com/cloudnationhq/az-cn-wam-mcp/pkg/mcp"
)

func main() {
	org := flag.String("org", "cloudnationhq", "GitHub organization name")
	token := flag.String("token", "", "GitHub personal access token (optional, for higher rate limits)")
	dbPath := flag.String("db", "index.db", "Path to SQLite database file")
	flag.Parse()

	log.SetOutput(os.Stderr)
	log.Println("Starting Azure CloudNation WAM MCP Server")

	db, err := database.New(*dbPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	log.Printf("Database initialized at: %s", *dbPath)

	syncer := indexer.NewSyncer(db, *token, *org)
	log.Println("Ready to sync repositories")

	server := mcp.NewServer(db, syncer)
	if err := server.Run(context.Background(), os.Stdin, os.Stdout); err != nil {
		log.Printf("Server stopped: %v", err)
	}
}
