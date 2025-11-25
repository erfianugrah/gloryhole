# Configuration Files

This directory contains example configuration files for Glory-Hole DNS Server.

## Available Configurations

### config.example.yml
Complete reference configuration with all available options documented.

**Use for:** Understanding all configuration options and creating your own custom config.

```bash
cp config.example.yml config.yml
nano config.yml
```

### config.blocklist-test.yml
Configuration optimized for testing blocklist functionality with 3 major sources.

**Features:**
- StevenBlack hosts file (111K domains)
- Hagezi Ultimate blocklist (232K domains)
- OISD Big list (260K domains)
- Total: ~473K domains after deduplication

**Use for:** Testing ad-blocking and blocklist performance.

### config.test.yml
Minimal configuration for automated testing.

**Features:**
- Single upstream DNS
- Minimal blocklist
- Short cache TTLs
- Debug logging enabled

**Use for:** Running the test suite and CI/CD pipelines.

## Creating Your Own Configuration

1. Start with the example:
   ```bash
   cp config.example.yml my-config.yml
   ```

2. Edit the key sections:
   ```yaml
   server:
     listen_address: ":53"  # DNS port

   upstreams:
     - "8.8.8.8:53"          # Google DNS
     - "1.1.1.1:53"          # Cloudflare DNS

   blocklists:
     - "https://raw.githubusercontent.com/StevenBlack/hosts/master/hosts"

   cache:
     enabled: true
     max_entries: 10000
   ```

3. Test the configuration (choose either):
   ```bash
   # Quick validation without binding sockets
   ./glory-hole --config my-config.yml --validate-config

   # Or start the server (Ctrl+C after it confirms startup)
   ./glory-hole --config my-config.yml
   ```

## Configuration Sections

### Server
- **listen_address**: IP and port to bind (default: ":53")
- **web_ui_address**: HTTP API/UI endpoint (default: ":8080")
- **tcp_enabled**: Enable TCP DNS queries (default: true)
- **udp_enabled**: Enable UDP DNS queries (default: true)
- **enable_blocklist**: Runtime kill-switch for blocklist evaluation (default: true)
- **enable_policies**: Runtime kill-switch for the policy engine (default: true)
- **decision_trace**: Include per-stage breadcrumbs for blocked queries (default: false)

### Upstreams
List of upstream DNS servers to forward queries to.

### Blocklists
URLs of blocklist sources (hosts, adblock, or plain text format).

### Cache
DNS response caching configuration with optional sharding for improved concurrency.

**Performance Tip:** Enable cache sharding (`shard_count: 64`) for high-traffic deployments to reduce lock contention and improve multi-core scalability.

### Database
SQLite query logging configuration.

### Policy
Advanced rule-based filtering engine.

### Local Records
Custom DNS records for your network.

### Logging
Log level, format, and output configuration.

### Telemetry
OpenTelemetry and Prometheus metrics configuration.

## Validation

Use `--validate-config` for a dry-run check (no sockets opened):

```bash
./glory-hole --config my-config.yml --validate-config
```

Starting the server without that flag also validates the configuration and exits with an error if anything is invalid.

## Environment Variables

Override config values with environment variables:

```bash
export GLORY_HOLE_LISTEN_ADDRESS=":5353"
export GLORY_HOLE_WEB_UI_ADDRESS=":8081"
./glory-hole --config config.yml
```

## See Also

- [Configuration Guide](../docs/guide/configuration.md) - Detailed configuration documentation
- [Examples](../examples/) - Real-world configuration examples
- [Troubleshooting](../docs/guide/troubleshooting.md) - Common configuration issues
