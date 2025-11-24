# Glory-Hole

[![CI](https://github.com/erfianugrah/gloryhole/actions/workflows/ci.yml/badge.svg)](https://github.com/erfianugrah/gloryhole/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/erfianugrah/gloryhole)](https://goreportcard.com/report/github.com/erfianugrah/gloryhole)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Coverage](https://img.shields.io/badge/coverage-82.4%25-brightgreen.svg)](https://github.com/erfianugrah/gloryhole)

A high-performance DNS server written in Go, designed as a modern, extensible replacement for Pi-hole and similar solutions. Glory-Hole provides advanced DNS filtering, caching, policy engine, web UI, and analytics capabilities in a single, self-contained binary.

## Project Status

**Version**: 0.7.8
**Test Coverage**: 82.4% average across packages (242 tests, 0 race conditions)
**CI Status**: All checks passing
**Stability**: Production Ready

> Glory-Hole is a production-ready DNS server with advanced features including Web UI, Policy Engine, comprehensive monitoring, and complete documentation. Ready for production deployment.

### Recent Updates (v0.7.8)

**Critical DNSSEC Fix**: Fixed SERVFAIL pass-through behavior that was causing 2-4 second delays on DNSSEC validation failures. SERVFAIL responses now return in <200ms (10-50x faster). See [CHANGELOG.md](CHANGELOG.md) for details.

### Quick Links

- **[Documentation](docs/)** - Complete guides and references
  - **[Getting Started](docs/guide/getting-started.md)** - Installation and setup
  - **[Configuration](docs/guide/configuration.md)** - Configuration reference
  - **[User Guide](docs/guide/usage.md)** - Day-to-day operations
  - **[Troubleshooting](docs/guide/troubleshooting.md)** - Common issues
- **[Architecture](docs/architecture/)** - System design and components
  - **[Overview](docs/architecture/overview.md)** - High-level architecture (3,700+ lines)
  - **[Components](docs/architecture/components.md)** - Detailed component guide
  - **[Performance](docs/architecture/performance.md)** - Benchmarks and optimizations
  - **[Design Decisions](docs/architecture/design-decisions.md)** - Architecture decision records
- **[Deployment](docs/deployment/)** - Production deployment guides
  - **[Docker](docs/deployment/docker.md)** - Containerized deployment
  - **[Kubernetes](deploy/kubernetes/)** - Kubernetes manifests
  - **[Monitoring](docs/deployment/monitoring.md)** - Observability setup
- **[API Reference](docs/api/)** - REST API and integrations
  - **[Web UI](docs/api/web-ui.md)** - Web interface guide
  - **[REST API](docs/api/rest-api.md)** - HTTP API reference
  - **[Policy Engine](docs/api/policy-engine.md)** - Policy configuration
- **[Development](docs/development/)** - Contributing and development
  - **[Setup](docs/development/setup.md)** - Development environment
  - **[Testing](docs/development/testing.md)** - Test coverage guide
  - **[Roadmap](docs/development/roadmap.md)** - Future plans

## Features

### Core DNS Functionality

- **High-Performance DNS Server**
  - Full UDP + TCP support with concurrent request handling
  - Sub-millisecond query processing
  - Zero-copy operations where possible
  - Graceful shutdown and restart

- **Advanced Blocklist System**
  - Multi-source blocklist support (StevenBlack, AdGuard, OISD)
  - Lock-free atomic updates (8ns lookup, 372M QPS)
  - Automatic deduplication across sources
  - 474K+ domains blocked out of the box
  - Auto-updating with configurable intervals
  - Whitelist support for critical domains

- **Local DNS Records**
  - A/AAAA records for IPv4/IPv6 hosts
  - CNAME records with automatic chain resolution
  - TXT records (SPF, DKIM, domain verification)
  - MX records (mail exchange with priority)
  - PTR records (reverse DNS)
  - SRV records (service discovery)
  - NS records (nameserver delegation)
  - SOA records (zone authority)
  - CAA records (certificate authority authorization)
  - Wildcard domain support (*.local)
  - Multiple IPs per record (round-robin)
  - Custom TTL values per record
  - EDNS0 support (RFC 6891 compliant)

- **Conditional Forwarding**
  - Route queries to different upstream DNS servers based on rules
  - Domain pattern matching (*.local, *.corp, exact domains, regex)
  - Client IP-based routing with CIDR support
  - Query type filtering (A, AAAA, PTR, etc.)
  - Priority-based rule evaluation (first-match-wins)
  - Split-DNS for hybrid cloud/on-premise setups
  - VPN client routing to corporate DNS
  - Sub-200ns rule evaluation with zero allocations

- **Intelligent Caching**
  - LRU cache with TTL-aware eviction
  - 63% performance boost on repeated queries
  - Configurable cache size and TTL ranges
  - Negative response caching

### Policy Engine

- **Expression-Based Rules**
  - Complex filtering logic using expr language
  - Time-based filtering (hour, minute, day, month, weekday)
  - Client IP matching and CIDR range support
  - Domain pattern matching (contains, starts with, ends with)
  - Actions: BLOCK, ALLOW, REDIRECT, FORWARD
  - FORWARD action for dynamic conditional forwarding
  - Thread-safe concurrent evaluation
  - First-match rule semantics

### Web UI

- **Modern Dashboard**
  - Real-time statistics with auto-refresh
  - Live query activity charts (Chart.js)
  - Top domains display (allowed & blocked)
  - System health and uptime

- **Query Log Viewer**
  - Real-time query stream
  - Filter by domain, status, client
  - Pagination for large volumes
  - Color-coded status badges

- **Policy Management**
  - CRUD operations for policies
  - Expression editor with syntax help
  - Enable/disable toggles
  - Visual policy cards

- **Settings & Configuration**
  - Configuration review
  - Blocklist reload
  - System information
  - Mobile-friendly responsive design

### Monitoring & Observability

- **Query Logging**
  - Comprehensive async logging to SQLite
  - <10µs overhead per query (non-blocking)
  - Configurable retention policy
  - Statistics aggregation

- **REST API**
  - Health check endpoints (/health, /ready, /api/health)
  - Query statistics with time periods
  - Recent queries with pagination
  - Top domains (allowed and blocked)
  - Policy management endpoints
  - Blocklist reload
  - CORS support

- **Metrics & Telemetry**
  - OpenTelemetry + Prometheus metrics
  - 21 production-ready alerts
  - Grafana dashboards (overview & performance)
  - DNS query rates, cache hit rates, error rates
  - Resource usage tracking

### Deployment & Operations

- **Production-Ready**
  - Single binary deployment
  - Docker support with multi-stage builds
  - Kubernetes manifests with health checks
  - Systemd service integration
  - Cloudflare D1 edge deployment guide

- **CI/CD Pipeline**
  - Automated testing (242 tests, 71.6% coverage)
  - Linting and security scanning (gosec, trivy)
  - Multi-architecture builds (amd64, arm64, armv7)
  - Automated releases with GitHub Actions
  - Docker image publishing

### Documentation

- **17,000+ Lines of Documentation**
  - 4 user guides (getting started, configuration, usage, troubleshooting)
  - 3 API references (REST API, Web UI, Policy Engine)
  - 4 architecture documents (overview, components, performance, design decisions)
  - 3 deployment guides (Docker, Kubernetes, monitoring)
  - 3 development guides (setup, testing, roadmap)
  - Complete configuration examples

## Architecture

Glory-Hole is built following Domain-Driven Design principles with a clean separation of concerns:

```
/
├── cmd/glory-hole/          Main application entry point
├── pkg/
│   ├── api/                 REST API + Web UI (63.3% coverage)
│   ├── blocklist/           Lock-free blocklist management (77.1% coverage)
│   ├── cache/               LRU cache with TTL support (82.4% coverage)
│   ├── config/              Configuration management (82.7% coverage)
│   ├── dns/                 Core DNS server and request handling (82.0% coverage)
│   ├── forwarder/           Upstream DNS forwarding with retry (87.1% coverage)
│   ├── localrecords/        Local DNS records (A/AAAA/CNAME, wildcards) (92.9% coverage)
│   ├── logging/             Structured logging with levels (75.0% coverage)
│   ├── pattern/             Pattern matching (wildcard, regex) (94.1% coverage)
│   ├── policy/              Policy engine for rule evaluation (95.2% coverage)
│   ├── resolver/            DNS resolver utilities (87.9% coverage)
│   ├── storage/             Multi-backend storage (SQLite, D1) (78.1% coverage)
│   └── telemetry/           OpenTelemetry + Prometheus metrics (76.7% coverage)
├── config/                  Configuration files
├── deploy/                  Deployment manifests
│   ├── kubernetes/          Kubernetes manifests
│   ├── grafana/             Grafana dashboards (2 dashboards)
│   └── prometheus/          Prometheus alerts (21 alerts)
├── docs/                    Comprehensive documentation (17,000+ lines)
│   ├── guide/               User guides
│   ├── api/                 API reference
│   ├── architecture/        System architecture
│   ├── deployment/          Deployment guides
│   └── development/         Development guides
├── scripts/                 Utility scripts
└── test/                    Integration and load tests
```

## Installation

### From Binary (Recommended)

Download the latest release from [GitHub Releases](https://github.com/erfianugrah/gloryhole/releases):

```bash
# Linux (amd64)
wget https://github.com/erfianugrah/gloryhole/releases/latest/download/glory-hole-linux-amd64
chmod +x glory-hole-linux-amd64
sudo mv glory-hole-linux-amd64 /usr/local/bin/glory-hole

# Linux (arm64)
wget https://github.com/erfianugrah/gloryhole/releases/latest/download/glory-hole-linux-arm64
chmod +x glory-hole-linux-arm64
sudo mv glory-hole-linux-arm64 /usr/local/bin/glory-hole

# macOS (amd64)
wget https://github.com/erfianugrah/gloryhole/releases/latest/download/glory-hole-darwin-amd64
chmod +x glory-hole-darwin-amd64
sudo mv glory-hole-darwin-amd64 /usr/local/bin/glory-hole
```

### Docker

```bash
# Pull and run
docker pull erfianugrah/gloryhole:latest
docker run -d \
  -p 53:53/udp \
  -p 53:53/tcp \
  -p 8080:8080 \
  -v ./config.yml:/config/config.yml \
  --name glory-hole \
  erfianugrah/gloryhole:latest

# Or use Docker Compose
docker-compose up -d
```

See [Docker Deployment Guide](docs/deployment/docker.md) for detailed instructions.

### Kubernetes

```bash
# Apply manifests
kubectl apply -f deploy/kubernetes/

# Or use Helm (coming soon)
```

See [Kubernetes Deployment Guide](deploy/kubernetes/README.md) for detailed instructions.

### From Source

```bash
# Clone repository
git clone https://github.com/erfianugrah/gloryhole.git
cd gloryhole

# Build with Makefile (recommended - includes version info)
make build

# Or build manually
go build -ldflags "-X main.version=$(git describe --tags --always --dirty) -X main.buildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ) -X main.gitCommit=$(git rev-parse --short HEAD)" -o bin/glory-hole ./cmd/glory-hole

# Install (optional)
sudo mv bin/glory-hole /usr/local/bin/

# Run tests
make test

# Run linter
make lint

# See all available commands
make help
```

**Available Make Commands:**
- `make build` - Build binary for current platform with version info
- `make build-all` - Build for all platforms (Linux, macOS, Windows)
- `make test` - Run all tests
- `make test-race` - Run tests with race detector
- `make test-coverage` - Generate coverage report
- `make lint` - Run golangci-lint
- `make clean` - Remove build artifacts
- `make run` - Build and run the server
- `make version` - Display version information

Requirements: Go 1.24 or later

## Configuration

Create a `config.yml` file:

```yaml
# DNS Server Settings
server:
  listen_address: ":53"
  tcp_enabled: true
  udp_enabled: true
  web_ui_address: ":8080"    # REST API and Web UI
  decision_trace: false       # Enable for detailed block breadcrumbs

# Upstream DNS Servers
upstream_dns_servers:
  - "1.1.1.1:53"
  - "8.8.8.8:53"

# Blocklists (auto-updated)
auto_update_blocklists: true
update_interval: "24h"
blocklists:
  - "https://raw.githubusercontent.com/StevenBlack/hosts/master/hosts"
  - "https://adguardteam.github.io/HostlistsRegistry/assets/filter_1.txt"
  - "https://big.oisd.nl/domainswild"

# Whitelist (never block these)
whitelist:
  - "whitelisted-domain.com"

# DNS Cache
cache:
  enabled: true
  max_entries: 10000
  min_ttl: "60s"
  max_ttl: "24h"

# Database (Query Logging)
database:
  enabled: true
  backend: "sqlite"
  sqlite:
    path: "./glory-hole.db"
    wal_mode: true
  buffer_size: 1000
  flush_interval: "5s"
  retention_days: 7

# Telemetry
telemetry:
  enabled: true
  prometheus_enabled: true
  prometheus_port: 9090
```

See [config/config.example.yml](config/config.example.yml) for complete options or check the [Configuration Guide](docs/guide/configuration.md).

## Quick Start

### 1. Create Configuration

```bash
# Copy example config
cp config/config.example.yml config.yml

# Edit configuration
nano config.yml
```

### 2. Start the Server

```bash
# Run directly
./glory-hole

# Or with custom config path
./glory-hole -config /path/to/config.yml

# Run in Docker
docker-compose up -d
```

### 3. Access Web UI

Open your browser to [http://localhost:8080](http://localhost:8080)

### 4. Configure Your Devices

Point your device's DNS to the Glory-Hole server:

```bash
# Linux/macOS - temporarily
sudo networksetup -setdnsservers Wi-Fi 192.168.1.100

# Or edit /etc/resolv.conf
nameserver 192.168.1.100

# Windows
# Control Panel > Network > Change adapter settings > Properties > IPv4 > Use the following DNS server
```

### 5. Test DNS Resolution

```bash
# Test blocked domain
dig @localhost ads.example.com

# Test allowed domain
dig @localhost google.com

# Check Web UI for live statistics
curl http://localhost:8080/api/stats
```

See [Getting Started Guide](docs/guide/getting-started.md) for detailed setup instructions.

### Systemd Service

Create `/etc/systemd/system/glory-hole.service`:

```ini
[Unit]
Description=Glory-Hole DNS Server
After=network.target

[Service]
Type=simple
User=glory-hole
ExecStart=/usr/local/bin/glory-hole
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

Enable and start:

```bash
sudo systemctl enable glory-hole
sudo systemctl start glory-hole
```

## Local DNS Records

Glory-Hole supports custom DNS records for your local network, perfect for resolving internal hostnames without relying on external DNS servers.

### Features

- **A/AAAA Records**: Define IPv4 and IPv6 addresses for local hosts
- **CNAME Records**: Create aliases that point to other domains
- **TXT Records**: SPF, DKIM, domain verification, arbitrary text data
- **MX Records**: Mail exchange records with priority-based routing
- **PTR Records**: Reverse DNS (IP to hostname)
- **SRV Records**: Service discovery with priority/weight load balancing
- **NS Records**: Nameserver delegation for subdomains
- **SOA Records**: Start of Authority for zone management
- **CAA Records**: Certificate Authority Authorization for SSL/TLS security
- **Wildcard Domains**: Use `*.dev.local` to match any subdomain (one level only)
- **Multiple IPs**: Assign multiple IP addresses to a single domain
- **Custom TTL**: Set custom time-to-live values (default: 300 seconds)
- **EDNS0**: Automatic Extended DNS support with buffer size negotiation
- **CNAME Chain Resolution**: Automatically follows CNAME chains with loop detection

### Example Configuration

```yaml
local_records:
  enabled: true
  records:
    # Simple A record
    - domain: "nas.local"
      type: "A"
      ips:
        - "192.168.1.100"

    # A record with multiple IPs (round-robin)
    - domain: "server.local"
      type: "A"
      ips:
        - "192.168.1.10"
        - "192.168.1.11"
        - "192.168.1.12"

    # IPv6 AAAA record
    - domain: "server.local"
      type: "AAAA"
      ips:
        - "fe80::1"
        - "fe80::2"

    # CNAME record (alias)
    - domain: "storage.local"
      type: "CNAME"
      target: "nas.local"

    # Wildcard record (matches any *.dev.local)
    # Note: Only matches one level deep
    # api.dev.local → matches
    # web.dev.local → matches
    # api.staging.dev.local → does not match
    - domain: "*.dev.local"
      type: "A"
      wildcard: true
      ips:
        - "192.168.1.200"

    # Custom TTL example
    - domain: "gateway.local"
      type: "A"
      ips:
        - "192.168.1.1"
      ttl: 600  # 10 minutes
```

### CNAME Chain Resolution

Glory-Hole automatically resolves CNAME chains up to 10 levels deep with loop detection:

```yaml
local_records:
  enabled: true
  records:
    # Final A record
    - domain: "server.local"
      type: "A"
      ips:
        - "192.168.1.100"

    # CNAME chain: storage -> nas -> server
    - domain: "nas.local"
      type: "CNAME"
      target: "server.local"

    - domain: "storage.local"
      type: "CNAME"
      target: "nas.local"

# Query for storage.local will automatically resolve to 192.168.1.100
```

### Local DNS Records

Define custom DNS responses using the local records feature:

```yaml
local_records:
  enabled: true
  records:
    - domain: "nas.local"
      type: "A"
      ips: ["192.168.1.100"]
    - domain: "storage.local"
      type: "CNAME"
      target: "nas.local"
```

Features:
- Multiple IPs per domain (round-robin)
- IPv6 (AAAA) records
- Wildcard domains (*.dev.local)
- Custom TTLs
- CNAME chain resolution with loop detection

## DNS Request Processing Order

Glory-Hole processes DNS requests in the following order:

1. **Cache Check**: Return cached response if available and not expired
2. **Local DNS Records**: Check for custom DNS records (highest priority for authoritative answers)
   - Supports A, AAAA, CNAME, TXT, MX, PTR, SRV, NS, SOA, CAA record types
   - Exact domain matches and wildcard patterns (*.dev.local)
   - CNAME chain resolution with loop detection
   - EDNS0 support for all responses
3. **Policy Engine**: Evaluate against user-defined rules
4. **Allowlist**: If domain is allowlisted, bypass blocking and forward
5. **Blocklist**: If domain is blocklisted, return blocked response
6. **Forward**: Forward to upstream DNS resolver

## Policy Engine

The policy engine enables advanced filtering rules using expression-based logic. Rules are evaluated in order, and the first matching rule's action is executed.

### Features

- **Expression Language**: Powerful expression syntax using [expr-lang/expr](https://github.com/expr-lang/expr)
- **Time-Based Rules**: Filter by hour, minute, day, month, or weekday
- **Network Rules**: Match client IPs, CIDR ranges, or specific networks
- **Domain Matching**: Pattern matching with contains, starts with, ends with
- **Actions**: BLOCK (deny request), ALLOW (bypass blocklist), REDIRECT (future)
- **Thread-Safe**: Concurrent evaluation with read-write locks
- **Pre-Compiled**: Rules are compiled at startup for fast evaluation

### Configuration

```yaml
policy:
  enabled: true
  rules:
    # Block social media after hours for specific devices
    - name: "Block Social Media After Hours"
      logic: "(Hour >= 22 || Hour < 6) && ClientIP == '192.168.1.50'"
      action: "BLOCK"
      enabled: true

    # Allow work domains during business hours
    - name: "Allow Work Domains"
      logic: "Hour >= 9 && Hour <= 17 && (Domain == 'work.com' || Domain == 'company.com')"
      action: "ALLOW"
      enabled: true

    # Block specific client on weekends
    - name: "Block Gaming PC on Weekends"
      logic: "(Weekday == 0 || Weekday == 6) && ClientIP == '192.168.1.100'"
      action: "BLOCK"
      enabled: true

    # Allow specific subnet
    - name: "Allow Admin Subnet"
      logic: "ClientIP == '192.168.100.1' || ClientIP == '192.168.100.2'"
      action: "ALLOW"
      enabled: false
```

### Available Context Fields

Policy rules have access to the following context fields:

- `Domain` (string): The queried domain (e.g., "example.com")
- `ClientIP` (string): The client's IP address
- `QueryType` (string): DNS query type (e.g., "A", "AAAA", "CNAME")
- `Hour` (int): Current hour (0-23)
- `Minute` (int): Current minute (0-59)
- `Day` (int): Day of month (1-31)
- `Month` (int): Month (1-12)
- `Weekday` (int): Day of week (0-6, Sunday=0)
- `Time` (time.Time): Full timestamp

### Helper Functions

The policy engine provides helper functions for common operations:

- `DomainMatches(domain, pattern)`: Pattern matching (e.g., `DomainMatches(Domain, "facebook")`)
- `DomainEndsWith(domain, suffix)`: Suffix check (e.g., `DomainEndsWith(Domain, ".com")`)
- `DomainStartsWith(domain, prefix)`: Prefix check (e.g., `DomainStartsWith(Domain, "www")`)
- `IPInCIDR(ip, cidr)`: CIDR range check (e.g., `IPInCIDR(ClientIP, "192.168.1.0/24")`)

### Example Rules

```yaml
policy:
  enabled: true
  rules:
    # Time-based blocking
    - name: "Block After Bedtime"
      logic: "Hour >= 22 || Hour < 7"
      action: "BLOCK"
      enabled: true

    # Domain pattern matching
    - name: "Block Social Media"
      logic: "DomainMatches(Domain, 'facebook') || DomainMatches(Domain, 'instagram')"
      action: "BLOCK"
      enabled: true

    # CIDR-based rules
    - name: "Allow Guest Network"
      logic: "IPInCIDR(ClientIP, '192.168.2.0/24')"
      action: "ALLOW"
      enabled: true

    # Complex conditions
    - name: "Block Gaming Sites During Weekdays"
      logic: "(Weekday >= 1 && Weekday <= 5) && (Hour >= 8 && Hour <= 17) && DomainEndsWith(Domain, '.game.com')"
      action: "BLOCK"
      enabled: true
```

### Rule Actions

- **BLOCK**: Return NXDOMAIN (blocked) response
- **ALLOW**: Bypass blocklist and forward to upstream DNS
- **REDIRECT**: (Coming soon) Redirect to a different domain
- **FORWARD**: Route query to specific upstream DNS servers (for conditional forwarding)

### Processing Order

Rules are evaluated in the order they appear in the configuration. The first rule that matches will have its action executed. If no rules match, normal processing continues (blocklist check, then forward).

## Conditional Forwarding

Conditional forwarding allows you to route DNS queries to different upstream servers based on domain patterns, client IPs, or query types. This is essential for split-DNS setups, VPN configurations, and hybrid cloud/on-premise environments.

### Use Cases

- **Split-DNS**: Forward `*.local` domains to internal DNS while using public DNS for everything else
- **VPN Routing**: Route VPN clients (`10.8.0.0/24`) to corporate DNS for internal domains
- **Reverse DNS**: Send PTR queries to local network DNS for IP address lookups
- **Multi-Site**: Route different domain suffixes to site-specific DNS servers

### Configuration

There are two ways to configure conditional forwarding:

#### 1. Declarative Rules (Recommended for simple cases)

```yaml
conditional_forwarding:
  enabled: true
  rules:
    # Forward local domain queries to internal DNS server
    - name: "Local domains"
      priority: 90                    # Higher priority = evaluated first (1-100)
      domains:
        - "*.local"                   # Wildcard suffix match
        - "*.lan"
        - "home.arpa"                 # Exact match
      upstreams:
        - "192.168.1.1:53"           # Internal DNS server
        - "192.168.1.2:53"           # Backup internal DNS
      enabled: true

    # Forward VPN client queries to corporate DNS
    - name: "VPN clients to corporate DNS"
      priority: 80
      client_cidrs:
        - "10.8.0.0/24"              # VPN subnet
      domains:
        - "*.corp.example.com"       # Corporate domains
        - "*.internal"
      upstreams:
        - "10.0.0.53:53"             # Corporate DNS
      enabled: true

    # Forward reverse DNS (PTR) queries to local DNS
    - name: "Reverse DNS"
      priority: 70
      query_types:
        - "PTR"                      # DNS reverse lookups
      domains:
        - "*.in-addr.arpa"           # IPv4 reverse DNS
        - "*.ip6.arpa"               # IPv6 reverse DNS
      upstreams:
        - "192.168.1.1:53"
      enabled: true

    # Combined rule: All conditions must match (AND logic)
    - name: "VPN clients accessing local domains"
      priority: 95                   # Highest priority
      domains:
        - "*.local"
      client_cidrs:
        - "10.8.0.0/24"
      query_types:
        - "A"
        - "AAAA"
      upstreams:
        - "192.168.1.1:53"
      enabled: true
```

#### 2. Policy Engine FORWARD Action (For complex logic)

For advanced scenarios requiring time-based routing or complex expressions:

```yaml
policy:
  enabled: true
  rules:
    # Forward internal queries from VPN clients during business hours
    - name: "VPN work hours forwarding"
      logic: >
        IPInCIDR(ClientIP, "10.8.0.0/24") &&
        Hour >= 9 && Hour <= 17 &&
        (DomainMatches(Domain, ".local") || DomainMatches(Domain, ".internal"))
      action: "FORWARD"
      action_data: "192.168.1.1:53,192.168.1.2:53"  # Comma-separated upstreams
      enabled: true
```

### Domain Pattern Matching

Conditional forwarding supports multiple pattern types:

- **Exact**: `nas.local` - Matches only "nas.local"
- **Wildcard Suffix**: `*.local` - Matches "nas.local", "router.local", "sub.nas.local", etc.
- **Wildcard Prefix**: `internal.*` - Matches "internal.corp", "internal.net", etc.
- **Regex**: `/^[a-z]+\.local$/` - Advanced pattern matching (slower)

### Priority and Evaluation

- Rules are evaluated by **priority** (highest first: 100 → 1)
- **First-match-wins**: Once a rule matches, its upstreams are used
- Default priority is **50** if not specified
- Multiple conditions within a rule use **AND logic** (all must match)

### Performance

Conditional forwarding is highly optimized:
- **Sub-200ns** rule evaluation per query
- **Zero allocations** during evaluation
- **Hash-based** exact domain matching (O(1))
- **Efficient** wildcard and CIDR matching

### Processing Order

DNS queries are processed in this order:

1. **Policy Engine** (if enabled) - FORWARD action takes precedence
2. **Local Records** - Custom A/AAAA/CNAME records
3. **Blocklist Check** - Block ads/malware domains
4. **Conditional Forwarding** - Route to specific upstreams if rules match
5. **Default Forwarding** - Use default upstream DNS servers

## REST API

Glory-Hole provides a comprehensive REST API for monitoring and management. The API server runs on port 8080 by default (configurable via `server.web_ui_address`).

### Health Check

**GET /api/health**

Returns server health and uptime information.

**Response:**
```json
{
  "status": "ok",
  "uptime": "2h15m30s",
  "version": "0.5.0"
}
```

### Statistics

**GET /api/stats**

Returns query statistics for a time period.

**Query Parameters:**
- `since` - Duration (e.g., "1h", "24h", "7d") - defaults to 24h

**Response:**
```json
{
  "total_queries": 10000,
  "blocked_queries": 2500,
  "cached_queries": 5000,
  "block_rate": 25.0,
  "cache_hit_rate": 50.0,
  "avg_response_ms": 5.2,
  "period": "24h0m0s",
  "timestamp": "2025-11-21T10:30:00Z"
}
```

**Example:**
```bash
curl http://localhost:8080/api/stats?since=1h
```

### Recent Queries

**GET /api/queries**

Returns recent DNS queries with optional filtering.

**Query Parameters:**
- `limit` - Number of results (1-1000, default: 100)
- `offset` - Pagination offset (default: 0)

**Response:**
```json
{
  "queries": [
    {
      "id": 12345,
      "timestamp": "2025-11-21T10:30:00Z",
      "client_ip": "192.168.1.100",
      "domain": "example.com",
      "query_type": "A",
      "response_code": 0,
      "blocked": false,
      "cached": true,
      "response_time_ms": 5,
      "upstream": "1.1.1.1:53"
    }
  ],
  "total": 1,
  "limit": 100,
  "offset": 0
}
```

**Example:**
```bash
curl http://localhost:8080/api/queries?limit=50
```

### Top Domains

**GET /api/top-domains**

Returns most queried domains.

**Query Parameters:**
- `limit` - Number of results (1-100, default: 10)
- `blocked` - Filter by blocked status (true/false, default: false)

**Response:**
```json
{
  "domains": [
    {
      "domain": "google.com",
      "queries": 1250,
      "blocked": false
    },
    {
      "domain": "facebook.com",
      "queries": 850,
      "blocked": false
    }
  ],
  "limit": 10
}
```

**Examples:**
```bash
# Top allowed domains
curl http://localhost:8080/api/top-domains?limit=20

# Top blocked domains
curl http://localhost:8080/api/top-domains?limit=20&blocked=true
```

### Blocklist Management

**POST /api/blocklist/reload**

Manually triggers a blocklist reload from configured sources.

**Response:**
```json
{
  "status": "ok",
  "domains": 101348,
  "message": "Blocklists reloaded successfully"
}
```

**Example:**
```bash
curl -X POST http://localhost:8080/api/blocklist/reload
```

### CORS

The API includes CORS headers allowing cross-origin requests, making it easy to build web-based dashboards and monitoring tools.

## Performance

Glory-Hole achieves exceptional performance through careful optimization:

### Benchmarks

- **DNS Query Processing**: Sub-millisecond average latency
- **Blocklist Lookup**: 8ns average (lock-free atomic operations)
- **Concurrent Throughput**: 372M queries/second (blocklist)
- **Cache Hit Boost**: 63% performance improvement on cached queries
- **Query Logging Overhead**: <10µs (non-blocking async writes)
- **Database Throughput**: >10,000 writes/second (batched inserts)

### Load Testing Results

From our comprehensive load testing suite:

- **Sustained Load**: 1.5M+ queries/second for 30+ seconds
- **Memory Efficiency**: ~40MB memory increase under heavy load
- **Cache Hit Rate**: Up to 99% on repeated queries
- **Zero Memory Leaks**: Stable memory usage over time
- **Zero Race Conditions**: All tests pass with `-race` flag

### Optimizations

- Lock-free atomic blocklist updates (10x faster than mutex)
- Zero-allocation DNS message handling where possible
- Concurrent request processing with goroutine pooling
- Efficient LRU cache with TTL-aware eviction
- Buffered async database writes
- CGO-free SQLite implementation for easy cross-compilation
- Pre-compiled policy expressions

See [Performance Documentation](docs/architecture/performance.md) for detailed benchmarks and analysis.

## Development

### Requirements

- Go 1.24 or later
- Docker (optional, for containerized development)
- golangci-lint (optional, for linting)

### Development Setup

```bash
# Clone repository
git clone https://github.com/erfianugrah/gloryhole.git
cd glory-hole

# Install dependencies
go mod download

# Build
go build -o glory-hole ./cmd/glory-hole

# Run tests
go test ./...

# Run with race detector
go test -race ./...

# Run linter
golangci-lint run
```

See [Development Setup Guide](docs/development/setup.md) for detailed instructions.

### Testing

```bash
# Run all tests
go test ./...

# Run with coverage
go test -cover ./...

# Run with race detector
go test -race ./...

# Run specific package
go test ./pkg/dns/...

# Run load tests
go test -v ./test/load/...

# Generate coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

Current test statistics:
- **242 tests** across 13 packages
- **71.6% coverage** (5,874 production lines, 9,170 test lines)
- **0 race conditions** detected
- **All tests passing**

See [Testing Guide](docs/development/testing.md) for comprehensive testing documentation.

### Linting

```bash
# Run all linters
golangci-lint run

# Run with auto-fix
golangci-lint run --fix

# Run specific linters
golangci-lint run --enable=gosec
```

### CI/CD Pipeline

The project uses GitHub Actions for:
- **Testing**: Run all tests with race detector
- **Linting**: golangci-lint with comprehensive checks
- **Security Scanning**: gosec for security vulnerabilities, trivy for container scanning
- **Building**: Multi-architecture builds (amd64, arm64, armv7)
- **Docker**: Build and push Docker images
- **Releases**: Automated release creation with binaries

See [.github/workflows/](.github/workflows/) for workflow definitions.

## Contributing

We welcome contributions! Glory-Hole is open source and benefits from community involvement.

### How to Contribute

1. **Fork the repository** on GitHub
2. **Clone your fork** locally
3. **Create a feature branch** (`git checkout -b feature/amazing-feature`)
4. **Make your changes** with clear commit messages
5. **Add tests** for new functionality
6. **Run tests** (`go test ./...`) and linting (`golangci-lint run`)
7. **Ensure all tests pass** with race detector (`go test -race ./...`)
8. **Update documentation** as needed
9. **Submit a pull request** with a clear description

### Contribution Guidelines

- **Code Style**: Follow Go conventions and project patterns
- **Testing**: Maintain or improve test coverage (target 70%+)
- **Documentation**: Update docs for user-facing changes
- **Commits**: Write clear, descriptive commit messages
- **Performance**: Consider performance implications
- **Security**: Follow security best practices

See [CONTRIBUTING.md](CONTRIBUTING.md) for comprehensive guidelines (940 lines).

### Areas for Contribution

- Bug fixes and issue resolution
- New features from the [roadmap](docs/development/roadmap.md)
- Documentation improvements
- Additional tests and test coverage
- Performance optimizations
- Internationalization
- Package management (Homebrew, apt, etc.)

## License

MIT License - see LICENSE file for details

## Acknowledgments

- Built with [codeberg.org/miekg/dns](https://codeberg.org/miekg/dns) - Go DNS library
- Uses [modernc.org/sqlite](https://modernc.org/sqlite) - CGO-free SQLite implementation
- Policy engine powered by [github.com/expr-lang/expr](https://github.com/expr-lang/expr)

## Web UI

Glory-Hole includes a modern, responsive web interface for monitoring and managing your DNS server without needing to edit configuration files or use command-line tools.

> **Note**: The dashboard activity chart currently displays mock data for visualization purposes. Real-time chart integration is planned for a future release. All other statistics and features display live data.

### Features

- **Real-Time Dashboard**
  - Live statistics (total/blocked/cached queries, response times)
  - Auto-refreshing stats cards (every 5 seconds)
  - Query activity chart with Chart.js visualization
  - Top allowed and blocked domains
  - Recent query log with live updates

- **Query Log Viewer**
  - Real-time query stream (updates every 2 seconds)
  - Filter by domain, status (blocked/allowed/cached)
  - Detailed information: timestamp, client IP, domain, query type, response time
  - Color-coded badges for quick status identification
  - Pagination support for large query volumes

- **Policy Management**
  - Create, edit, and delete filtering policies via UI
  - Expression editor with syntax help
  - Enable/disable policies with toggle switches
  - Visual policy cards showing rule logic and action
  - Support for time-based, client-based, and domain pattern rules

- **Settings Page**
  - View current configuration
  - System information (version, uptime)
  - Quick blocklist reload
  - Configuration review (DNS, cache, storage, telemetry)

### Accessing the UI

The Web UI is available at **`http://localhost:8080`** by default (configurable via `api.address` in config.yml).

```bash
# Start the server
./glory-hole

# Access Web UI
open http://localhost:8080
```

### UI Pages

| Page | URL | Description |
|------|-----|-------------|
| **Dashboard** | `/` | Main overview with stats and charts |
| **Queries** | `/queries` | Query log viewer with real-time updates |
| **Policies** | `/policies` | Policy management (CRUD operations) |
| **Settings** | `/settings` | Configuration review and system info |

### Technology Stack

- **Backend**: Go templates with embedded filesystem
- **Frontend**: Vanilla JavaScript + HTMX for reactivity
- **Styling**: Custom CSS (no frameworks needed)
- **Charts**: Chart.js for visualizations
- **Updates**: HTMX automatic polling for real-time data

### Screenshots

*Dashboard with live statistics and query activity chart*
![Dashboard](docs/screenshots/dashboard.png)

*Query log viewer with real-time updates*
![Queries](docs/screenshots/queries.png)

*Policy management interface*
![Policies](docs/screenshots/policies.png)

### Creating Policies via UI

1. Navigate to `/policies`
2. Click "Add Policy"
3. Enter policy details:
   - **Name**: Descriptive name (e.g., "Block Social Media After Hours")
   - **Logic**: Expression (e.g., `Hour >= 22 && DomainMatches(Domain, 'facebook')`)
   - **Action**: BLOCK, ALLOW, or REDIRECT
   - **Enabled**: Toggle to activate/deactivate
4. Click "Save Policy"

**Available Functions:**
- `Hour`, `Minute`, `Day`, `Month`, `Weekday` - Time-based conditions
- `Domain`, `ClientIP`, `QueryType` - Request properties
- `DomainMatches()`, `DomainEndsWith()`, `DomainStartsWith()` - Domain patterns
- `IPInCIDR()` - Client IP matching

**Example Policies:**
```javascript
// Block social media during work hours (9 AM - 5 PM, Mon-Fri)
Weekday >= 1 && Weekday <= 5 && Hour >= 9 && Hour < 17 && 
(DomainMatches(Domain, 'facebook') || DomainMatches(Domain, 'twitter'))

// Allow specific client bypass blocklist
ClientIP == "192.168.1.100"

// Block all .ru domains except whitelisted
DomainEndsWith(Domain, '.ru') && !DomainMatches(Domain, 'whitelisted')
```

### API Endpoints (for UI)

The UI uses these RESTful endpoints:

```
GET  /                           Dashboard page
GET  /queries                    Query log page
GET  /policies                   Policy management page
GET  /settings                   Settings page

GET  /api/ui/stats              Stats HTML partial
GET  /api/ui/queries            Queries HTML partial
GET  /api/ui/top-domains        Top domains HTML partial

GET  /api/health                Health check (JSON)
GET  /api/stats                 Statistics (JSON)
GET  /api/queries               Recent queries (JSON)
GET  /api/top-domains           Top domains (JSON)

GET  /api/policies              List all policies
POST /api/policies              Create new policy
GET  /api/policies/{id}         Get policy by ID
PUT  /api/policies/{id}         Update policy
DELETE /api/policies/{id}       Delete policy

POST /api/blocklist/reload      Reload blocklists
```

### Customization

The Web UI is embedded in the binary but can be customized by modifying:
- `pkg/api/ui/templates/*.html` - HTML templates
- `pkg/api/ui/static/css/style.css` - Styling
- Rebuild with `go build ./cmd/glory-hole`

### Mobile Support

The UI is fully responsive and works on:
- Desktop browsers (Chrome, Firefox, Safari, Edge)
- Tablets (iPad, Android tablets)
- Mobile phones (iOS, Android)

