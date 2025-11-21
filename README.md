# Glory-Hole

A high-performance DNS server written in Go, designed as a modern, extensible replacement for Pi-hole and similar solutions. Glory-Hole provides advanced DNS filtering, caching, and analytics capabilities in a single, self-contained binary.

## 🚀 Project Status

**Current Phase**: Phase 1 (MVP) ✅ **Complete!**
**Next Phase**: Phase 2 (Essential Features)
**Version**: 0.5.0-dev

> **Phase 1 Complete!** Glory-Hole now has a fully functional DNS server with blocklist management, caching, query logging to SQLite, and comprehensive testing. Ready for real-world testing!

### Quick Links

- 📊 **[Current Status](STATUS.md)** - What's working and what's not
- 🗺️ **[Development Roadmap](PHASES.md)** - Detailed phase-by-phase plan
- 🏗️ **[Architecture Guide](ARCHITECTURE.md)** - System architecture (3,700+ lines)
- 📐 **[Design Document](DESIGN.md)** - Feature specifications (1,900+ lines)
- 📝 **[Example Configuration](config.example.yml)** - Complete config example

## Features

### ✅ Implemented (Phase 1)

- **DNS Server**: Full UDP + TCP support with concurrent request handling
- **DNS Filtering**: Block unwanted domains using customizable blocklists
  - Multi-source blocklist support (StevenBlack, AdGuard, OISD)
  - Lock-free atomic updates (8ns lookup, 372M QPS)
  - Automatic deduplication across sources
  - 474K+ domains blocked out of the box
- **Query Logging**: Comprehensive async logging to SQLite database
  - Domain, client IP, query type, response code, blocked status
  - <10µs overhead per query (non-blocking)
  - Configurable retention policy (default 7 days)
  - Statistics aggregation and top domains tracking
- **Response Caching**: LRU cache with TTL-aware eviction
  - 63% performance boost on repeated queries
  - Configurable cache size and TTL ranges
  - Negative response caching
- **Local DNS Records**: Custom DNS records for local network
  - A/AAAA records for IPv4/IPv6 hosts
  - CNAME records with automatic chain resolution
  - Wildcard domain support (*.local)
  - Multiple IPs per record (round-robin)
- **Whitelist Support**: Ensure critical domains are never blocked
- **Auto-Updating Blocklists**: Automatic periodic updates from remote sources
- **Telemetry**: OpenTelemetry + Prometheus metrics built-in
- **Single Binary**: No external dependencies, easy deployment

### 🚧 In Development (Phase 2)

- **Policy Engine**: Advanced rule-based filtering with complex expressions
- **Web UI**: Built-in web interface for monitoring and configuration
- **REST API**: Programmatic access for stats and management

## Architecture

Glory-Hole is built following Domain-Driven Design principles with a clean separation of concerns:

```
/
├── cmd/glory-hole/          Main application entry point
├── pkg/
│   ├── blocklist/           Lock-free blocklist management
│   ├── cache/               LRU cache with TTL support
│   ├── config/              Configuration management
│   ├── dns/                 Core DNS server and request handling
│   ├── forwarder/           Upstream DNS forwarding with retry
│   ├── localrecords/        Local DNS records (A/AAAA/CNAME, wildcards) (NEW!)
│   ├── logging/             Structured logging with levels
│   ├── policy/              Policy engine for rule evaluation (stub)
│   ├── storage/             Multi-backend storage (SQLite, D1)
│   └── telemetry/           OpenTelemetry + Prometheus metrics
└── ui/                      Web interface assets (future)
```

**Stats**: 7,174 lines of code (3,533 production + 3,641 tests), 101 tests passing ✅

## Installation

### From Source

```bash
git clone https://github.com/yourusername/glory-hole.git
cd glory-hole
go build -o glory-hole ./cmd/glory-hole
```

### Using Go Install

```bash
go install github.com/yourusername/glory-hole/cmd/glory-hole@latest
```

### Docker

```bash
docker pull yourusername/glory-hole:latest
docker run -d -p 53:53/udp -p 8080:8080 -v ./config.yml:/config.yml glory-hole
```

## Configuration

Create a `config.yml` file:

```yaml
# DNS Server Settings
server:
  listen_address: ":53"
  tcp_enabled: true
  udp_enabled: true

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

See [config.example.yml](config.example.yml) for complete options.

## Usage

### Basic Usage

```bash
./glory-hole
```

The server will start on port 53 (DNS) and port 8080 (web interface).

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
- **Wildcard Domains**: Use `*.dev.local` to match any subdomain (one level only)
- **Multiple IPs**: Assign multiple IP addresses to a single domain
- **Custom TTL**: Set custom time-to-live values (default: 300 seconds)
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
    # ✅ api.dev.local → matches
    # ✅ web.dev.local → matches
    # ❌ api.staging.dev.local → does not match
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

### Legacy Overrides vs Local Records

Glory-Hole supports both legacy overrides (simpler) and the new local records feature (more powerful):

**Legacy Overrides** (still supported):
```yaml
overrides:
  nas.local: "192.168.1.100"

cname_overrides:
  storage.local: "nas.local."
```

**New Local Records** (recommended):
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

Use local records for:
- Multiple IPs per domain
- IPv6 (AAAA) records
- Wildcard domains
- Custom TTLs
- CNAME chain resolution

## DNS Request Processing Order

Glory-Hole processes DNS requests in the following order:

1. **Cache Check**: Return cached response if available and not expired
2. **Local DNS Records**: Check for custom A/AAAA/CNAME records (highest priority for authoritative answers)
   - Supports exact domain matches
   - Wildcard domain matches (*.dev.local)
   - CNAME chain resolution with loop detection
3. **Policy Engine**: Evaluate against user-defined rules
4. **Local Overrides**: Check for legacy exact A/AAAA record overrides
5. **CNAME Overrides**: Check for legacy CNAME alias definitions
6. **Allowlist**: If domain is allowlisted, bypass blocking and forward
7. **Blocklist**: If domain is blocklisted, return blocked response
8. **Forward**: Forward to upstream DNS resolver

## Policy Engine

Define complex filtering rules using expressions:

```yaml
rules:
  - name: "Block social media after 10 PM for kids"
    logic: "Hour >= 22 && ClientIP in ['192.168.1.50', '192.168.1.51'] && Domain matches '.*(facebook|tiktok|instagram)\\.com'"
    action: "BLOCK"

  - name: "Allow work domains during business hours"
    logic: "Hour >= 9 && Hour <= 17 && Domain endsWith '.company.com'"
    action: "ALLOW"
```

## API Endpoints

- `GET /api/stats` - Query statistics
- `GET /api/queries` - Recent queries
- `GET /api/top-domains` - Most queried domains
- `POST /api/blocklist/reload` - Reload blocklists
- `GET /api/health` - Health check

## Performance

Glory-Hole is designed for high performance:

- Zero-allocation DNS message handling where possible
- Concurrent request processing
- Efficient in-memory caching with TTL support
- Buffered database writes to minimize I/O overhead
- CGO-free SQLite implementation for easy cross-compilation

## Development

### Requirements

- Go 1.25.4 or later

### Building

```bash
go build -v ./...
```

### Testing

```bash
go test -v ./...
```

All tests currently pass (26 tests in foundation packages):
```
ok      glory-hole/pkg/config           0.271s
ok      glory-hole/pkg/logging          0.002s
ok      glory-hole/pkg/telemetry        0.005s
```

### Linting

```bash
golangci-lint run
```

### Project Structure

```
glory-hole/
├── cmd/glory-hole/          # Main application entry point
├── pkg/
│   ├── config/              # ✅ Configuration management (COMPLETE)
│   ├── logging/             # ✅ Structured logging (COMPLETE)
│   ├── telemetry/           # ✅ OpenTelemetry metrics (COMPLETE)
│   ├── dns/                 # 🔴 DNS server (TODO - Phase 1)
│   ├── policy/              # 🔴 Policy engine (TODO - Phase 2)
│   └── storage/             # 🔴 Database layer (TODO - Phase 1)
├── config.example.yml       # Example configuration
├── PHASES.md                # Development roadmap
├── STATUS.md                # Current project status
├── ARCHITECTURE.md          # System architecture
└── DESIGN.md                # Feature specifications
```

### Current Implementation Status

**✅ Complete (Phase 0)**:
- Configuration system with YAML loading
- Hot-reload capability (file watching)
- Structured logging (slog)
- OpenTelemetry metrics integration
- Prometheus exporter
- Comprehensive test coverage

**🔴 TODO (Phase 1 - MVP)**:
- DNS server implementation
- Blocklist management
- Upstream forwarding
- DNS caching
- Query logging
- Basic API

See [PHASES.md](PHASES.md) for the complete roadmap.

## Contributing

Contributions are welcome! Please:

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests for new functionality
5. Ensure all tests pass
6. Submit a pull request

## License

MIT License - see LICENSE file for details

## Acknowledgments

- Built with [codeberg.org/miekg/dns](https://codeberg.org/miekg/dns) - Go DNS library
- Uses [modernc.org/sqlite](https://modernc.org/sqlite) - CGO-free SQLite implementation
- Policy engine powered by [github.com/expr-lang/expr](https://github.com/expr-lang/expr)
