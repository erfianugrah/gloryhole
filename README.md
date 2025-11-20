# Glory-Hole

A high-performance DNS server written in Go, designed as a modern, extensible replacement for Pi-hole and similar solutions. Glory-Hole provides advanced DNS filtering, caching, and analytics capabilities in a single, self-contained binary.

## 🚧 Project Status

**Current Phase**: Phase 0 (Foundation) ✅ Complete
**Next Phase**: Phase 1 (MVP - Basic DNS Server)
**Version**: 0.1.0-dev

> **Note**: This project is under active development. The foundation layer (configuration, logging, telemetry) is complete and production-ready. Core DNS functionality is being implemented in Phase 1.

### Quick Links

- 📊 **[Current Status](STATUS.md)** - What's working and what's not
- 🗺️ **[Development Roadmap](PHASES.md)** - Detailed phase-by-phase plan
- 🏗️ **[Architecture Guide](ARCHITECTURE.md)** - System architecture (3,700+ lines)
- 📐 **[Design Document](DESIGN.md)** - Feature specifications (1,900+ lines)
- 📝 **[Example Configuration](config.example.yml)** - Complete config example

## Features

- **DNS Filtering**: Block unwanted domains using customizable blocklists
- **Local DNS Overrides**: Define custom A/AAAA records for local network hosts
- **CNAME Aliases**: Create CNAME mappings for local services
- **Whitelist Support**: Ensure critical domains are never blocked
- **Policy Engine**: Advanced rule-based filtering with complex expressions
- **Response Caching**: TTL-aware caching for improved performance
- **Query Logging**: Comprehensive logging to SQLite database
- **Auto-Updating Blocklists**: Automatic periodic updates from remote sources
- **Web UI**: Built-in web interface for monitoring and configuration
- **Single Binary**: No external dependencies, easy deployment

## Architecture

Glory-Hole is built following Domain-Driven Design principles with a clean separation of concerns:

```
/
├── cmd/glory-hole/          Main application entry point
├── pkg/
│   ├── config/              Configuration management
│   ├── dns/                 Core DNS server and request handling
│   ├── policy/              Policy engine for rule evaluation
│   └── storage/             SQLite database interaction
└── ui/                      Web interface assets
```

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
upstream_dns_servers:
  - "1.1.1.1:53"
  - "8.8.8.8:53"

update_interval: "24h"

blocklists:
  - "https://raw.githubusercontent.com/StevenBlack/hosts/master/hosts"

whitelist:
  - "whitelisted-domain.com"

overrides:
  my-nas.local: "192.168.1.100"
  router.local: "192.168.1.1"

cname_overrides:
  my-service.local: "actual-service-name.com."
```

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

## DNS Request Processing Order

Glory-Hole processes DNS requests in the following order:

1. **Cache Check**: Return cached response if available and not expired
2. **Policy Engine**: Evaluate against user-defined rules
3. **Local Overrides**: Check for exact A/AAAA record overrides
4. **CNAME Overrides**: Check for CNAME alias definitions
5. **Allowlist**: If domain is allowlisted, bypass blocking and forward
6. **Blocklist**: If domain is blocklisted, return blocked response
7. **Forward**: Forward to upstream DNS resolver

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
