# Azure CloudNation WAM MCP Server Makefile

.PHONY: build run test clean install deps fmt vet lint

# Build variables
BINARY_NAME=az-cn-wam-mcp
BUILD_DIR=./bin
CMD_DIR=./cmd/server

# Go variables
GO=go
GOFLAGS=-tags fts5
LDFLAGS=-ldflags "-s -w"
CGO_ENABLED=1

# Default target
all: deps fmt vet test build

# Install dependencies
deps:
	$(GO) mod download
	$(GO) mod tidy

# Format code
fmt:
	$(GO) fmt ./...

# Vet code
vet:
	$(GO) vet ./...

# Run linter (requires golangci-lint to be installed)
lint:
	golangci-lint run

# Run tests
test:
	$(GO) test -v ./...

# Build binary
build:
	mkdir -p $(BUILD_DIR)
	CGO_ENABLED=$(CGO_ENABLED) $(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(CMD_DIR)

# Build for multiple platforms
build-all: build-linux build-darwin build-windows

build-linux:
	mkdir -p $(BUILD_DIR)
	CGO_ENABLED=$(CGO_ENABLED) GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 $(CMD_DIR)

build-darwin:
	mkdir -p $(BUILD_DIR)
	CGO_ENABLED=$(CGO_ENABLED) GOOS=darwin GOARCH=amd64 $(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 $(CMD_DIR)

build-darwin-arm64:
	mkdir -p $(BUILD_DIR)
	CGO_ENABLED=$(CGO_ENABLED) GOOS=darwin GOARCH=arm64 $(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 $(CMD_DIR)

build-windows:
	mkdir -p $(BUILD_DIR)
	CGO_ENABLED=$(CGO_ENABLED) GOOS=windows GOARCH=amd64 $(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe $(CMD_DIR)

# Run the server locally (for MCP testing)
run: build
	$(BUILD_DIR)/$(BINARY_NAME) -org cloudnationhq

# Run with GitHub token
run-with-token: build
	@test -n "$(GITHUB_TOKEN)" || (echo "Error: GITHUB_TOKEN not set" && exit 1)
	$(BUILD_DIR)/$(BINARY_NAME) -org cloudnationhq -token $(GITHUB_TOKEN)

# Run with custom database path
run-custom: build
	$(BUILD_DIR)/$(BINARY_NAME) -org cloudnationhq -db $(DB_PATH)

# Install binary to GOPATH/bin
install:
	$(GO) install $(GOFLAGS) $(LDFLAGS) $(CMD_DIR)

# Clean build artifacts
clean:
	rm -rf $(BUILD_DIR)
	$(GO) clean

# Run tests with coverage
test-coverage:
	$(GO) test -v -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html

# Development setup
dev-setup: deps
	@echo "Installing development tools..."
	$(GO) install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Quick check before commit
check: fmt vet test

# Help
help:
	@echo "Available targets:"
	@echo "  all              - Run deps, fmt, vet, test, and build"
	@echo "  deps             - Download and tidy dependencies"
	@echo "  fmt              - Format Go code"
	@echo "  vet              - Run go vet"
	@echo "  lint             - Run golangci-lint"
	@echo "  test             - Run tests"
	@echo "  build            - Build binary for current platform (with FTS5 support)"
	@echo "  build-all        - Build binaries for all platforms"
	@echo "  build-darwin-arm64 - Build for macOS ARM64 (M1/M2)"
	@echo "  run              - Build and run the MCP server"
	@echo "  run-with-token   - Run with GITHUB_TOKEN env var"
	@echo "  run-custom       - Run with custom DB_PATH"
	@echo "  install          - Install binary to GOPATH/bin"
	@echo "  clean            - Clean build artifacts and database"
	@echo "  test-coverage    - Run tests with coverage report"
	@echo "  dev-setup        - Install development tools"
	@echo "  check            - Run fmt, vet, and test"
	@echo "  help             - Show this help message"
	@echo ""
	@echo "Examples:"
	@echo "  make build                          # Build with FTS5 support"
	@echo "  make run                            # Run MCP server"
	@echo "  GITHUB_TOKEN=xxx make run-with-token  # Run with GitHub token"
	@echo "  DB_PATH=custom.db make run-custom   # Use custom database"
