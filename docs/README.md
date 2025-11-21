# Glory-Hole Documentation

Welcome to the Glory-Hole DNS server documentation!

## Documentation Index

### Core Documentation

- **[Policy Engine](POLICY_ENGINE.md)** - Comprehensive guide to the Policy Engine, including:
  - Getting started
  - Rule syntax and structure
  - All 10 helper functions
  - Common filtering patterns
  - Advanced examples
  - REST API management
  - Performance tuning
  - Troubleshooting guide

- **[Performance](PERFORMANCE.md)** - Performance benchmarks, architecture decisions, and optimization strategies:
  - Blocklist performance (8ns lookups, 372M QPS)
  - DNS cache performance (<1ms cache hits)
  - Memory usage analysis
  - Lock-free design patterns
  - Benchmarking guide

- **[Testing](TESTING.md)** - Comprehensive testing guide and coverage report:
  - Test coverage (82.5% average, 208 tests, 9,209 test lines)
  - Running tests (unit, integration, E2E, benchmarks)
  - CI/CD testing with race detection
  - Writing new tests
  - Best practices

### Configuration Examples

Located in `../examples/`:

- **[home-network.yml](../examples/home-network.yml)** - Family-friendly home network setup
  - Ad blocking
  - Parental controls
  - Time-based restrictions
  - Local device DNS records

- **[small-office.yml](../examples/small-office.yml)** - Business network configuration
  - Productivity enforcement
  - Network segmentation
  - Bandwidth management
  - Guest network restrictions

- **[advanced-filtering.yml](../examples/advanced-filtering.yml)** - Showcase of all features
  - Complex policy rules
  - Time and IP-based filtering
  - REDIRECT action examples
  - Multi-condition logic

- **[minimal.yml](../examples/minimal.yml)** - Lightweight setup for resource-constrained environments

## Quick Start

### 1. Basic Setup

```bash
# Create config file
cp config.example.yml config.yml

# Edit configuration
nano config.yml

# Start server
./glory-hole -config config.yml
```

### 2. Enable Policy Engine

Add to your `config.yml`:

```yaml
policy:
  enabled: true
  rules:
    - name: "Block ads"
      logic: 'DomainMatches(Domain, "ads")'
      action: "BLOCK"
      enabled: true
```

### 3. Test Your Setup

```bash
# Query the DNS server
dig @localhost example.com

# Check if blocking works
dig @localhost ads.example.com  # Should return NXDOMAIN
```

## Key Features

### 🛡️ Policy Engine
- **Expression-based rules**: Write complex filtering logic
- **10+ helper functions**: Domain matching, regex, time-based rules
- **3 actions**: BLOCK, ALLOW, REDIRECT
- **REST API**: Manage rules without restart
- **High performance**: 64ns per rule evaluation

### 📋 Blocklist Management
- **Multi-source support**: Load from multiple blocklist URLs
- **Auto-update**: Scheduled blocklist refreshes
- **Lock-free**: High-performance concurrent access
- **Real-time reload**: Update blocklists via API

### 💾 Query Logging
- **SQLite backend**: Efficient query storage
- **Statistics API**: Query counts, blocked domains
- **Top domains**: Most queried domains report
- **Retention management**: Automatic old data cleanup

### 🚀 Performance
- **DNS caching**: Configurable TTL and cache size
- **Concurrent handling**: Thousands of queries per second
- **Minimal latency**: Sub-millisecond policy evaluation
- **Efficient forwarding**: Multiple upstream DNS servers

### 🏠 Local DNS Records
- **Custom records**: A, AAAA, CNAME, MX, SRV, TXT, PTR
- **Wildcard support**: `*.local` patterns
- **CNAME resolution**: Automatic CNAME following

### 📊 Monitoring
- **Prometheus metrics**: Query rates, cache stats, block rates
- **REST API**: Health, statistics, top domains
- **Structured logging**: JSON or text format
- **Telemetry**: Optional OpenTelemetry integration

## Architecture

```
┌─────────────┐
│   Client    │
└──────┬──────┘
       │ DNS Query
       ▼
┌─────────────────┐
│   DNS Handler   │
│  - Local Recs   │
│  - Blocklist    │
│  - Policy Eng   │
│  - Cache        │
└──────┬──────────┘
       │
       ├─ Match Local Record → Return
       ├─ Check Blocklist   → Block if match
       ├─ Evaluate Policy   → BLOCK/ALLOW/REDIRECT
       ├─ Check Cache       → Return if cached
       └─ Forward Upstream  → Resolve & cache
```

## REST API Endpoints

### Health & Stats
- `GET /api/health` - Server health check
- `GET /api/stats` - Query statistics
- `GET /api/queries` - Recent queries
- `GET /api/top-domains` - Most queried domains

### Policy Management
- `GET /api/policies` - List all policies
- `POST /api/policies` - Add new policy
- `GET /api/policies/{id}` - Get policy details
- `PUT /api/policies/{id}` - Update policy
- `DELETE /api/policies/{id}` - Delete policy

### Blocklist Management
- `POST /api/blocklist/reload` - Reload blocklists

## Common Use Cases

### Home Network
- Block ads and trackers
- Parental controls with time restrictions
- Custom DNS for local devices (NAS, printer, etc.)
- Weekend vs weekday rules

### Small Office
- Block social media during work hours
- Network segmentation (guest, employee, admin)
- Bandwidth management (block streaming)
- Productivity enforcement

### Advanced Filtering
- Regex-based blocking patterns
- IP-based access control
- Query type filtering
- Multi-condition complex rules

## Testing

```bash
# Run all tests
go test ./...

# Run integration tests
go test ./test/... -v

# Check coverage
go test ./... -cover

# Run benchmarks
go test ./... -bench=. -benchmem
```

## Performance Tuning

### For High Load

```yaml
cache:
  enabled: true
  max_entries: 50000
  min_ttl: "300s"
  max_ttl: "24h"

policy:
  enabled: true
  # Keep rules under 50 for best performance
```

### For Low Memory

```yaml
cache:
  enabled: true
  max_entries: 1000

database:
  enabled: false  # Disable query logging

blocklists: []   # Use policy engine instead
```

## Security Considerations

1. **Validate external input**: Especially if exposing API externally
2. **Restrict API access**: Use firewall rules or reverse proxy auth
3. **Regular updates**: Keep blocklists fresh
4. **Monitor logs**: Watch for unusual patterns
5. **Test rules**: Validate policy rules in staging first

## Troubleshooting

### DNS Not Resolving

```bash
# Check server is running
curl http://localhost:8080/api/health

# Check DNS port
sudo netstat -tulpn | grep :53

# Test with dig
dig @localhost example.com
```

### Policy Not Working

```bash
# Enable debug logging
# In config.yml:
logging:
  level: "debug"

# Check policy list
curl http://localhost:8080/api/policies

# Check policy is enabled
```

### High Memory Usage

```bash
# Check cache size
curl http://localhost:8080/api/stats

# Reduce cache size in config
cache:
  max_entries: 5000  # Lower this

# Disable query logging if not needed
database:
  enabled: false
```

## Contributing

See main repository README for contribution guidelines.

## Support

- **Issues**: Report bugs on GitHub
- **Documentation**: This docs/ directory
- **Examples**: See examples/ directory

## License

See main repository LICENSE file.
