# Development Environment Setup

**Last Updated:** 2025-11-22
**Version:** 0.6.0

Complete guide to setting up a development environment for Glory-Hole DNS server.

---

## Table of Contents

1. [Prerequisites](#prerequisites)
2. [Repository Structure](#repository-structure)
3. [Initial Setup](#initial-setup)
4. [Building the Project](#building-the-project)
5. [Running Tests](#running-tests)
6. [Running Locally](#running-locally)
7. [IDE Setup](#ide-setup)
8. [Debugging](#debugging)
9. [Code Generation](#code-generation)
10. [Profiling and Benchmarking](#profiling-and-benchmarking)
11. [Database Management](#database-management)
12. [Troubleshooting](#troubleshooting)

---

## Prerequisites

### Required

**Go 1.24.0 or later**

Check your Go version:
```bash
go version
# Should show: go version go1.24.0 or higher
```

Install/upgrade Go:
- Download from https://go.dev/dl/
- Or use version manager like [gvm](https://github.com/moovweb/gvm)

**Git**
```bash
git --version
```

### Recommended Tools

**golangci-lint** - Comprehensive linter
```bash
# macOS/Linux
curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin

# Verify installation
golangci-lint --version
```

**gopls** - Go Language Server (for IDE integration)
```bash
go install golang.org/x/tools/gopls@latest
```

**delve** - Go debugger
```bash
go install github.com/go-delve/delve/cmd/dlv@latest
```

**air** - Live reload for development (optional)
```bash
go install github.com/cosmtrek/air@latest
```

---

## Repository Structure

```
glory-hole/
├── cmd/
│   └── glory-hole/
│       └── main.go              # Application entry point
├── pkg/
│   ├── api/                     # REST API and Web UI
│   │   ├── api.go               # Server setup
│   │   ├── handlers.go          # HTTP handlers
│   │   ├── handlers_policy.go   # Policy API handlers
│   │   ├── ui_handlers.go       # Web UI handlers
│   │   ├── middleware.go        # HTTP middleware
│   │   ├── responses.go         # Response types
│   │   └── ui/                  # Embedded UI assets
│   │       ├── templates/       # Go HTML templates
│   │       └── static/          # CSS, JS, images
│   ├── blocklist/               # Blocklist management
│   │   ├── blocklist.go         # Parsing and loading
│   │   ├── manager.go           # Auto-update manager
│   │   └── downloader.go        # HTTP downloader
│   ├── cache/                   # DNS response cache
│   │   └── cache.go             # LRU cache with TTL
│   ├── config/                  # Configuration management
│   │   ├── config.go            # Config types and loading
│   │   └── watcher.go           # File watcher for hot-reload
│   ├── dns/                     # DNS server
│   │   ├── server.go            # Server lifecycle
│   │   ├── server_impl.go       # UDP/TCP server implementation
│   │   └── handler.go           # DNS request handler (Note: Not in repo)
│   ├── forwarder/               # Upstream DNS forwarding
│   │   └── forwarder.go         # Round-robin forwarder
│   ├── localrecords/            # Local DNS records
│   │   ├── records.go           # A/AAAA/CNAME records
│   │   ├── util.go              # Helper functions
│   │   └── errors.go            # Error types
│   ├── logging/                 # Structured logging
│   │   └── logger.go            # slog wrapper
│   ├── policy/                  # Policy engine
│   │   └── engine.go            # Expression-based filtering
│   ├── storage/                 # Database layer
│   │   ├── storage.go           # Storage interface
│   │   ├── sqlite.go            # SQLite implementation
│   │   ├── factory.go           # Factory for backends
│   │   └── errors.go            # Storage errors
│   └── telemetry/               # Metrics and tracing
│       └── telemetry.go         # OpenTelemetry setup
├── test/
│   ├── integration_test.go      # Integration tests
│   └── load/
│       ├── dns_load_test.go     # Load tests
│       └── benchmark_test.go    # Performance benchmarks
├── docs/                        # Documentation
├── config/                      # Example configurations
├── deploy/                      # Deployment configs (Docker, K8s)
├── scripts/                     # Utility scripts
├── config.yml                   # Local config (gitignored)
├── config.example.yml           # Example configuration
├── go.mod                       # Go module definition
├── go.sum                       # Dependency checksums
├── .golangci.yml                # Linter configuration
└── README.md                    # Project overview
```

---

## Initial Setup

### 1. Fork and Clone

**Fork the repository** on GitHub (if contributing):
```bash
# Go to https://github.com/ORIGINAL_OWNER/glory-hole
# Click "Fork" button
```

**Clone your fork:**
```bash
git clone https://github.com/YOUR_USERNAME/glory-hole.git
cd glory-hole
```

**Add upstream remote:**
```bash
git remote add upstream https://github.com/ORIGINAL_OWNER/glory-hole.git
git remote -v
# Should show:
#   origin    https://github.com/YOUR_USERNAME/glory-hole.git (fetch)
#   origin    https://github.com/YOUR_USERNAME/glory-hole.git (push)
#   upstream  https://github.com/ORIGINAL_OWNER/glory-hole.git (fetch)
#   upstream  https://github.com/ORIGINAL_OWNER/glory-hole.git (push)
```

### 2. Install Dependencies

**Download Go modules:**
```bash
go mod download
```

**Verify dependencies:**
```bash
go mod verify
# Should show: all modules verified
```

### 3. Create Local Configuration

**Copy example config:**
```bash
cp config.example.yml config.yml
```

**Edit config.yml** for local development:
```yaml
server:
  listen_address: ":5353"      # Use non-privileged port for development
  tcp_enabled: true
  udp_enabled: true
  web_ui_address: ":8080"

upstream_dns_servers:
  - "1.1.1.1:53"
  - "8.8.8.8:53"

auto_update_blocklists: false  # Disable auto-update during development
blocklists:
  - "https://raw.githubusercontent.com/StevenBlack/hosts/master/hosts"

cache:
  enabled: true
  max_entries: 1000            # Smaller cache for development

database:
  enabled: true
  backend: "sqlite"
  sqlite:
    path: "./dev.db"           # Separate dev database
    wal_mode: true
  buffer_size: 100
  flush_interval: "5s"

logging:
  level: "debug"               # Verbose logging for development
  format: "text"               # Human-readable format
  output: "stdout"

telemetry:
  enabled: true
  prometheus_enabled: true
  prometheus_port: 9090
```

---

## Building the Project

### Basic Build

**Build binary:**
```bash
go build -o glory-hole ./cmd/glory-hole
```

**Run the binary:**
```bash
./glory-hole
```

### Development Build (with race detector)

**Build with race detection:**
```bash
go build -race -o glory-hole-race ./cmd/glory-hole
```

**Run with race detector:**
```bash
./glory-hole-race
```

### Optimized Release Build

**Build with optimizations:**
```bash
go build -ldflags="-s -w" -o glory-hole ./cmd/glory-hole
```

Flags:
- `-s`: Strip symbol table
- `-w`: Strip DWARF debug info
- Result: Smaller binary

### Cross-Platform Build

**Build for Linux:**
```bash
GOOS=linux GOARCH=amd64 go build -o glory-hole-linux-amd64 ./cmd/glory-hole
```

**Build for macOS:**
```bash
GOOS=darwin GOARCH=amd64 go build -o glory-hole-darwin-amd64 ./cmd/glory-hole
GOOS=darwin GOARCH=arm64 go build -o glory-hole-darwin-arm64 ./cmd/glory-hole
```

**Build for Windows:**
```bash
GOOS=windows GOARCH=amd64 go build -o glory-hole-windows-amd64.exe ./cmd/glory-hole
```

### Build All Platforms

**Script for building all platforms:**
```bash
#!/bin/bash
# scripts/build-all.sh

set -e

VERSION=${1:-dev}
OUTPUT_DIR="./dist"

mkdir -p $OUTPUT_DIR

platforms=(
    "linux/amd64"
    "linux/arm64"
    "darwin/amd64"
    "darwin/arm64"
    "windows/amd64"
)

for platform in "${platforms[@]}"; do
    IFS='/' read -r GOOS GOARCH <<< "$platform"
    output="$OUTPUT_DIR/glory-hole-$VERSION-$GOOS-$GOARCH"

    if [ "$GOOS" = "windows" ]; then
        output="${output}.exe"
    fi

    echo "Building for $GOOS/$GOARCH..."
    GOOS=$GOOS GOARCH=$GOARCH go build -ldflags="-s -w" -o "$output" ./cmd/glory-hole

    echo "✓ Built: $output"
done

echo "All builds complete!"
```

---

## Running Tests

### Unit Tests

**Run all tests:**
```bash
go test ./...
```

**Run with verbose output:**
```bash
go test -v ./...
```

**Run with race detector:**
```bash
go test -race ./...
```

**Run specific package:**
```bash
go test -v ./pkg/cache
go test -v ./pkg/policy
```

**Run specific test:**
```bash
go test -v -run TestCache_Get ./pkg/cache
go test -v -run TestPolicyEngine ./pkg/policy
```

### Test Coverage

**Generate coverage report:**
```bash
go test -coverprofile=coverage.out ./...
```

**View coverage in terminal:**
```bash
go tool cover -func=coverage.out
```

**View coverage in browser:**
```bash
go tool cover -html=coverage.out
```

**Coverage by package:**
```bash
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out | grep -E '^glory-hole/pkg/.*:' | sort -t: -k2 -rn
```

### Integration Tests

**Run integration tests:**
```bash
go test -v ./test/integration_test.go
```

**Run with timeout:**
```bash
go test -v -timeout 30s ./test/
```

### Benchmark Tests

**Run benchmarks:**
```bash
go test -bench=. ./...
```

**Run specific benchmark:**
```bash
go test -bench=BenchmarkCache_Get ./pkg/cache
go test -bench=BenchmarkBlocklist_Lookup ./pkg/blocklist
```

**Benchmark with memory stats:**
```bash
go test -bench=. -benchmem ./pkg/cache
```

**Compare benchmarks:**
```bash
# Save baseline
go test -bench=. ./pkg/cache > old.txt

# Make changes, then compare
go test -bench=. ./pkg/cache > new.txt
benchstat old.txt new.txt
```

### Load Tests

**Run load tests:**
```bash
cd test/load
go test -v -run TestDNSLoad -timeout 60s
```

**Run with custom parameters:**
```bash
go test -v -run TestDNSLoad -args -queries=100000 -workers=100
```

---

## Running Locally

### Start Server

**Standard startup:**
```bash
./glory-hole
```

**With custom config:**
```bash
./glory-hole -config /path/to/config.yml
```

**Using go run:**
```bash
go run ./cmd/glory-hole
```

### Test DNS Queries

**Using dig:**
```bash
# Query on development port 5353
dig @127.0.0.1 -p 5353 example.com

# Query with specific type
dig @127.0.0.1 -p 5353 example.com A
dig @127.0.0.1 -p 5353 example.com AAAA

# Query blocked domain
dig @127.0.0.1 -p 5353 ads.example.com
```

**Using nslookup:**
```bash
nslookup example.com 127.0.0.1 -port=5353
```

**Using curl (for API):**
```bash
# Health check
curl http://localhost:8080/api/health

# Statistics
curl http://localhost:8080/api/stats

# Recent queries
curl http://localhost:8080/api/queries?limit=10

# Top domains
curl http://localhost:8080/api/top-domains?limit=10
```

### Access Web UI

Open browser to:
```
http://localhost:8080/
```

Pages:
- `/` - Dashboard
- `/queries` - Query log
- `/policies` - Policy management
- `/settings` - Settings

### Hot Reload Development

**Using air (recommended):**

Create `.air.toml`:
```toml
root = "."
tmp_dir = "tmp"

[build]
  cmd = "go build -o ./tmp/glory-hole ./cmd/glory-hole"
  bin = "./tmp/glory-hole"
  include_ext = ["go", "yml", "html", "css", "js"]
  exclude_dir = ["test", "deploy", "docs"]
  delay = 1000

[log]
  time = true
```

Run:
```bash
air
# Now changes to code/templates will auto-rebuild and restart
```

---

## IDE Setup

### Visual Studio Code

**Install Extensions:**
- [Go](https://marketplace.visualstudio.com/items?itemName=golang.go)
- [Go Test Explorer](https://marketplace.visualstudio.com/items?itemName=premparihar.go-test-explorer)

**Settings (`.vscode/settings.json`):**
```json
{
  "go.useLanguageServer": true,
  "go.lintTool": "golangci-lint",
  "go.lintOnSave": "package",
  "go.formatTool": "goimports",
  "go.testFlags": ["-v", "-race"],
  "go.coverOnSave": false,
  "go.coverOnTestPackage": true,
  "[go]": {
    "editor.formatOnSave": true,
    "editor.codeActionsOnSave": {
      "source.organizeImports": true
    }
  },
  "go.toolsManagement.autoUpdate": true
}
```

**Launch Configuration (`.vscode/launch.json`):**
```json
{
  "version": "0.2.0",
  "configurations": [
    {
      "name": "Launch Glory-Hole",
      "type": "go",
      "request": "launch",
      "mode": "debug",
      "program": "${workspaceFolder}/cmd/glory-hole",
      "args": ["-config", "${workspaceFolder}/config.yml"],
      "env": {},
      "showLog": true
    },
    {
      "name": "Debug Current Test",
      "type": "go",
      "request": "launch",
      "mode": "test",
      "program": "${file}",
      "args": ["-test.v", "-test.run", "^${selectedText}$"]
    },
    {
      "name": "Debug Current Package",
      "type": "go",
      "request": "launch",
      "mode": "test",
      "program": "${fileDirname}",
      "args": ["-test.v"]
    }
  ]
}
```

### GoLand / IntelliJ IDEA

**Run Configuration:**
1. Run → Edit Configurations
2. Add → Go Build
3. Configuration:
   - **Name:** Glory-Hole
   - **Package path:** `glory-hole/cmd/glory-hole`
   - **Working directory:** `$ProjectFileDir$`
   - **Program arguments:** `-config config.yml`
   - **Run kind:** Package

**Test Configuration:**
1. Run → Edit Configurations
2. Add → Go Test
3. Configuration:
   - **Test kind:** All packages
   - **Program arguments:** `-race -v`

---

## Debugging

### Using Delve

**Start debugger:**
```bash
dlv debug ./cmd/glory-hole -- -config config.yml
```

**Debug a test:**
```bash
dlv test ./pkg/cache -- -test.v -test.run TestCache_Get
```

**Debug commands:**
```
(dlv) break main.main           # Set breakpoint
(dlv) break pkg/dns/server.go:50
(dlv) continue                   # Continue execution
(dlv) next                       # Step over
(dlv) step                       # Step into
(dlv) print variable             # Print variable
(dlv) locals                     # Show local variables
(dlv) goroutines                 # List goroutines
(dlv) goroutine 1                # Switch to goroutine
```

### Debugging Race Conditions

**Run with race detector:**
```bash
go run -race ./cmd/glory-hole
```

**Test with race detector:**
```bash
go test -race ./...
```

**Common race patterns to look for:**
- Concurrent map access
- Shared counter updates
- Channel operations

---

## Code Generation

### Generate Mocks (if using mockery)

```bash
mockery --all --output ./test/mocks
```

### Generate Protobuf (if using)

```bash
protoc --go_out=. --go-grpc_out=. api/proto/*.proto
```

---

## Profiling and Benchmarking

### CPU Profiling

**Profile during test:**
```bash
go test -cpuprofile=cpu.prof -bench=. ./pkg/cache
```

**Analyze profile:**
```bash
go tool pprof cpu.prof
```

**Interactive commands:**
```
(pprof) top10              # Top 10 functions by CPU time
(pprof) list FunctionName  # Source code with timing
(pprof) web                # Open in browser (requires graphviz)
```

### Memory Profiling

**Profile memory:**
```bash
go test -memprofile=mem.prof -bench=. ./pkg/cache
```

**Analyze:**
```bash
go tool pprof mem.prof
```

### Live Profiling

**Start server with pprof:**
```go
import _ "net/http/pprof"

go func() {
    log.Println(http.ListenAndServe("localhost:6060", nil))
}()
```

**Access profiles:**
```bash
# CPU profile (30 seconds)
go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30

# Heap profile
go tool pprof http://localhost:6060/debug/pprof/heap

# Goroutines
go tool pprof http://localhost:6060/debug/pprof/goroutine
```

### Benchmark Comparison

**Install benchstat:**
```bash
go install golang.org/x/perf/cmd/benchstat@latest
```

**Compare benchmarks:**
```bash
# Baseline
go test -bench=. ./pkg/cache > old.txt

# Make changes...

# New results
go test -bench=. ./pkg/cache > new.txt

# Compare
benchstat old.txt new.txt
```

---

## Database Management

### SQLite CLI

**Open database:**
```bash
sqlite3 glory-hole.db
```

**Useful commands:**
```sql
-- Show tables
.tables

-- Show schema
.schema queries

-- Query recent queries
SELECT * FROM queries ORDER BY timestamp DESC LIMIT 10;

-- Statistics
SELECT COUNT(*) FROM queries;
SELECT COUNT(*) FROM queries WHERE blocked = 1;

-- Top domains
SELECT domain, COUNT(*) as count
FROM queries
GROUP BY domain
ORDER BY count DESC
LIMIT 10;

-- Exit
.quit
```

### Reset Database

```bash
rm glory-hole.db glory-hole.db-shm glory-hole.db-wal
# Database will be recreated on next run
```

---

## Troubleshooting

### Port Already in Use

**Error:** `bind: address already in use`

**Solution:**
```bash
# Find process using port 53
sudo lsof -i :53

# Kill process
sudo kill -9 <PID>

# Or use development port 5353 in config.yml
```

### Permission Denied (Port 53)

**Error:** `permission denied`

**Solution:**
Port 53 requires root privileges. Options:

1. **Use non-privileged port for development:**
   ```yaml
   server:
     listen_address: ":5353"
   ```

2. **Grant capability (Linux):**
   ```bash
   sudo setcap CAP_NET_BIND_SERVICE=+eip ./glory-hole
   ```

3. **Run as root (not recommended for development):**
   ```bash
   sudo ./glory-hole
   ```

### Tests Failing

**Check Go version:**
```bash
go version
# Must be 1.24.0 or higher
```

**Clean cache:**
```bash
go clean -testcache
go test ./...
```

**Update dependencies:**
```bash
go mod tidy
go mod download
```

### Linter Issues

**Update golangci-lint:**
```bash
curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin
```

**Run linter:**
```bash
golangci-lint run
```

**Auto-fix issues:**
```bash
golangci-lint run --fix
```

### Import Errors

**Tidy modules:**
```bash
go mod tidy
```

**Verify modules:**
```bash
go mod verify
```

**Clear module cache:**
```bash
go clean -modcache
go mod download
```

---

## Quick Reference

### Common Commands

```bash
# Build
go build -o glory-hole ./cmd/glory-hole

# Test
go test ./...
go test -race ./...
go test -cover ./...

# Lint
golangci-lint run

# Run
./glory-hole

# Test DNS
dig @127.0.0.1 -p 5353 example.com

# Test API
curl http://localhost:8080/api/health
```

### Directory Quick Access

```bash
# Source code
cd pkg/

# Tests
cd test/

# Configuration examples
cd config/

# Documentation
cd docs/
```

---

## Next Steps

1. **Read Architecture Docs:**
   - [Architecture Overview](../architecture/overview.md)
   - [Component Documentation](../architecture/components.md)
   - [Design Decisions](../architecture/design-decisions.md)

2. **Review Testing Guide:**
   - [Testing Documentation](testing.md)

3. **Contribute:**
   - [Contributing Guidelines](/home/erfi/gloryhole/CONTRIBUTING.md)

4. **Explore Features:**
   - [Getting Started Guide](../guide/getting-started.md)
   - [Configuration Guide](../guide/configuration.md)
   - [API Documentation](../api/rest-api.md)

---

**Happy Coding!**
