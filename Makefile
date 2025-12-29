.PHONY: help build build-all install clean test test-race test-coverage lint fmt vet run dev version npm-install npm-build

# Variables
BINARY_NAME=glory-hole
MAIN_PATH=./cmd/glory-hole
BUILD_DIR=./bin

# Version info from git or environment
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
GIT_COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
GO_VERSION=$(shell go version | awk '{print $$3}')

# Build flags
LDFLAGS=-ldflags "\
	-X main.version=$(VERSION) \
	-X main.buildTime=$(BUILD_TIME) \
	-X main.gitCommit=$(GIT_COMMIT) \
	-s -w"

# Go build flags
GOFLAGS=-trimpath

## help: Display this help message
help:
	@echo "Glory-Hole DNS Server - Build System"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^## ' Makefile | sed 's/## /  /'

## npm-install: Install npm dependencies
npm-install:
	@echo "Installing npm dependencies..."
	@npm install

## npm-build: Build frontend assets from npm packages
npm-build: npm-install
	@echo "Building frontend assets..."
	@npm run build:vendor

## build: Build the binary for current platform
build: npm-build
	@echo "Building $(BINARY_NAME) $(VERSION)..."
	@mkdir -p $(BUILD_DIR)
	go build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PATH)
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)"

## build-all: Build for all platforms (Linux, macOS, Windows)
build-all:
	@echo "Building for all platforms..."
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 go build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 $(MAIN_PATH)
	GOOS=linux GOARCH=arm64 go build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 $(MAIN_PATH)
	GOOS=darwin GOARCH=amd64 go build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 $(MAIN_PATH)
	GOOS=darwin GOARCH=arm64 go build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 $(MAIN_PATH)
	GOOS=windows GOARCH=amd64 go build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe $(MAIN_PATH)
	@echo "Cross-compilation complete"

## install: Install the binary to $GOPATH/bin
install:
	@echo "Installing $(BINARY_NAME)..."
	go install $(GOFLAGS) $(LDFLAGS) $(MAIN_PATH)

## clean: Remove build artifacts
clean:
	@echo "Cleaning build artifacts..."
	rm -rf $(BUILD_DIR)
	rm -f $(BINARY_NAME)
	rm -rf node_modules
	rm -rf pkg/api/ui/static/js/vendor
	rm -rf pkg/api/ui/static/fonts
	go clean

## test: Run tests
test:
	@echo "Running tests..."
	go test -v ./...

## bench-whitelist: Run whitelist/blocklist lookup benchmarks and load test
bench-whitelist:
	@echo "Running whitelist/blocklist microbenchmarks..."
	go test -run=^$$ -bench=BenchmarkWhitelistBlocklistLookups -benchmem ./pkg/dns
	@echo "Running whitelist bypass load test..."
	go test -run TestDNSLoadWhitelistBypass ./test/load

## test-race: Run tests with race detector
test-race:
	@echo "Running tests with race detector..."
	go test -race -v ./...

## test-coverage: Run tests with coverage report
test-coverage:
	@echo "Running tests with coverage..."
	go test -cover -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

LINT_DIRS := $(shell go list -f '{{.Dir}}' ./... | sed -e 's#$(CURDIR)/##')

## lint: Run golangci-lint
lint:
	@echo "Running linter..."
	@golangci-lint cache clean
	@set -e; \
	for dir in $(LINT_DIRS); do \
		echo "  • $$dir"; \
		golangci-lint run --timeout=5m ./$$dir; \
	done

## fmt: Format Go code
fmt:
	@echo "Formatting code..."
	go fmt ./...

## vet: Run go vet
vet:
	@echo "Running go vet..."
	go vet ./...

## run: Build and run the server
run: build
	@echo "Starting $(BINARY_NAME)..."
	$(BUILD_DIR)/$(BINARY_NAME)

## dev: Run directly with go run (no build)
dev:
	@echo "Running in development mode..."
	go run $(MAIN_PATH)

## version: Display version information
version:
	@echo "Version:     $(VERSION)"
	@echo "Git Commit:  $(GIT_COMMIT)"
	@echo "Build Time:  $(BUILD_TIME)"
	@echo "Go Version:  $(GO_VERSION)"

## release: Prepare release (lint, test, build)
release: lint test build
	@echo "Release build complete: $(VERSION)"
