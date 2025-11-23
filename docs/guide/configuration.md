# Configuration Guide

This guide provides comprehensive documentation for all Glory-Hole DNS Server configuration options.

## Table of Contents

- [Configuration File](#configuration-file)
- [Server Configuration](#server-configuration)
- [Upstream DNS Servers](#upstream-dns-servers)
- [Blocklists](#blocklists)
- [Cache Configuration](#cache-configuration)
- [Database Configuration](#database-configuration)
- [Local DNS Records](#local-dns-records)
- [Policy Engine](#policy-engine)
- [Logging Configuration](#logging-configuration)
- [Telemetry Configuration](#telemetry-configuration)
- [Environment Variables](#environment-variables)
- [Configuration Validation](#configuration-validation)
- [Common Patterns](#common-patterns)

## Configuration File

Glory-Hole uses YAML for configuration. By default, it looks for `config.yml` in the current directory.

### Specifying Config File

```bash
# Default location (./config.yml)
glory-hole

# Custom location
glory-hole -config /etc/glory-hole/config.yml

# Alternative flag
glory-hole --config /path/to/config.yml
```

### Basic Configuration Template

```yaml
server:
  listen_address: ":53"
  tcp_enabled: true
  udp_enabled: true
  web_ui_address: ":8080"

upstream_dns_servers:
  - "1.1.1.1:53"
  - "8.8.8.8:53"

blocklists:
  - "https://raw.githubusercontent.com/StevenBlack/hosts/master/hosts"

cache:
  enabled: true
  max_entries: 10000

database:
  enabled: true
  backend: "sqlite"

logging:
  level: "info"

telemetry:
  enabled: true
```

## Server Configuration

Controls the DNS server and Web UI settings.

```yaml
server:
  listen_address: ":53"           # DNS server bind address
  tcp_enabled: true               # Enable DNS over TCP
  udp_enabled: true               # Enable DNS over UDP
  web_ui_address: ":8080"         # Web UI and API bind address
```

### Options

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `listen_address` | string | `:53` | DNS server address (format: `host:port` or `:port`) |
| `tcp_enabled` | bool | `true` | Enable TCP DNS queries (RFC requirement) |
| `udp_enabled` | bool | `true` | Enable UDP DNS queries (most common) |
| `web_ui_address` | string | `:8080` | Web UI and REST API address |

### Examples

**Bind to specific interface:**
```yaml
server:
  listen_address: "192.168.1.10:53"  # Only listen on specific IP
```

**Non-privileged port (no root needed):**
```yaml
server:
  listen_address: ":5353"  # Use port > 1024
```

**Custom Web UI port:**
```yaml
server:
  web_ui_address: ":3000"  # Access UI at http://localhost:3000
```

**UDP-only (faster, less memory):**
```yaml
server:
  tcp_enabled: false
  udp_enabled: true
```

## Upstream DNS Servers

Configure where Glory-Hole forwards non-blocked queries.

```yaml
upstream_dns_servers:
  - "1.1.1.1:53"          # Cloudflare DNS
  - "1.0.0.1:53"          # Cloudflare DNS (backup)
  - "8.8.8.8:53"          # Google DNS
  - "8.8.4.4:53"          # Google DNS (backup)
```

### Options

- **Format**: Array of strings in `host:port` format
- **Minimum**: At least 1 upstream server required
- **Behavior**: Queries are sent to first server; falls back to others on failure
- **Timeout**: 2 seconds per upstream (configurable via code)

### Popular Upstream DNS Providers

**Cloudflare (1.1.1.1):**
```yaml
upstream_dns_servers:
  - "1.1.1.1:53"
  - "1.0.0.1:53"
```

**Google Public DNS:**
```yaml
upstream_dns_servers:
  - "8.8.8.8:53"
  - "8.8.4.4:53"
```

**Quad9 (Security-focused):**
```yaml
upstream_dns_servers:
  - "9.9.9.9:53"
  - "149.112.112.112:53"
```

**OpenDNS:**
```yaml
upstream_dns_servers:
  - "208.67.222.222:53"
  - "208.67.220.220:53"
```

**Local DNS server:**
```yaml
upstream_dns_servers:
  - "192.168.1.1:53"  # Your router or local DNS
```

## Blocklists

Configure domain blocklists for ad and tracker blocking.

```yaml
auto_update_blocklists: true    # Enable automatic updates
update_interval: "24h"          # Update frequency

blocklists:
  - "https://raw.githubusercontent.com/StevenBlack/hosts/master/hosts"
  - "https://adguardteam.github.io/HostlistsRegistry/assets/filter_1.txt"
  - "https://big.oisd.nl/domainswild"

whitelist:
  - "analytics.google.com"      # Never block these domains
  - "github-cloud.s3.amazonaws.com"
```

### Options

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `auto_update_blocklists` | bool | `false` | Automatically update blocklists |
| `update_interval` | duration | `24h` | How often to update (e.g., `6h`, `12h`, `24h`, `7d`) |
| `blocklists` | []string | `[]` | URLs of blocklist sources |
| `whitelist` | []string | `[]` | Domains to never block (highest priority) |

### Blocklist Sources

**Comprehensive (474K+ domains):**
```yaml
blocklists:
  - "https://raw.githubusercontent.com/StevenBlack/hosts/master/hosts"
  - "https://adguardteam.github.io/HostlistsRegistry/assets/filter_1.txt"
  - "https://big.oisd.nl/domainswild"
```

**Light (fewer false positives):**
```yaml
blocklists:
  - "https://raw.githubusercontent.com/StevenBlack/hosts/master/hosts"
```

**Aggressive (maximum blocking):**
```yaml
blocklists:
  - "https://raw.githubusercontent.com/StevenBlack/hosts/master/alternates/fakenews-gambling-porn-social/hosts"
  - "https://adguardteam.github.io/HostlistsRegistry/assets/filter_2.txt"
  - "https://someonewhocares.org/hosts/hosts"
```

### Whitelist Examples

```yaml
whitelist:
  # Analytics (might break some sites)
  - "analytics.google.com"
  - "google-analytics.com"

  # CDNs (be careful blocking these)
  - "github-cloud.s3.amazonaws.com"
  - "cloudfront.net"

  # Development/Testing
  - "localhost"
  - "test.local"
```

## Cache Configuration

Configure DNS response caching for improved performance.

```yaml
cache:
  enabled: true              # Enable/disable caching
  max_entries: 10000         # Maximum cached entries
  min_ttl: "60s"            # Minimum TTL for cached entries
  max_ttl: "24h"            # Maximum TTL for cached entries
  negative_ttl: "5m"        # TTL for negative responses (NXDOMAIN)
```

### Options

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `true` | Enable DNS response caching |
| `max_entries` | int | `10000` | Maximum number of cached entries (LRU eviction) |
| `min_ttl` | duration | `60s` | Minimum TTL (overrides low TTLs from upstream) |
| `max_ttl` | duration | `24h` | Maximum TTL (caps high TTLs from upstream) |
| `negative_ttl` | duration | `5m` | TTL for NXDOMAIN responses |

### Performance Impact

- **Cache enabled**: ~63% faster queries on cache hits
- **Memory usage**: ~1KB per cached entry
- **10,000 entries**: ~10MB RAM

### Examples

**High-performance (more memory):**
```yaml
cache:
  enabled: true
  max_entries: 50000    # 50MB RAM
  min_ttl: "300s"       # 5 minutes minimum
  max_ttl: "86400s"     # 24 hours maximum
```

**Low-memory (minimal caching):**
```yaml
cache:
  enabled: true
  max_entries: 1000     # 1MB RAM
  min_ttl: "60s"
  max_ttl: "1h"
```

**Disable caching:**
```yaml
cache:
  enabled: false
```

## Database Configuration

Configure query logging and statistics storage.

```yaml
database:
  enabled: true                   # Enable query logging
  backend: "sqlite"               # Backend: "sqlite" or "d1"

  # SQLite configuration
  sqlite:
    path: "./glory-hole.db"      # Database file path
    wal_mode: true                # Write-Ahead Logging (better concurrency)
    busy_timeout: 5000            # Busy timeout in milliseconds
    cache_size: 10000             # Cache size in KB

  # Cloudflare D1 configuration (alternative to SQLite)
  d1:
    account_id: ""                # Cloudflare account ID
    database_id: ""               # D1 database ID
    api_token: ""                 # API token with D1 access

  # Buffer settings (async writes for performance)
  buffer_size: 1000               # Queries to buffer before flush
  flush_interval: "5s"            # Max time before flushing buffer
  batch_size: 100                 # Max queries per batch insert

  # Retention policy
  retention_days: 7               # Days to keep detailed logs

  # Statistics aggregation
  statistics:
    enabled: true
    aggregation_interval: "1h"   # Aggregate stats every hour
```

### Backend Options

#### SQLite (Recommended)

```yaml
database:
  backend: "sqlite"
  sqlite:
    path: "./glory-hole.db"
    wal_mode: true          # Recommended for concurrency
    busy_timeout: 5000
    cache_size: 10000       # 10MB cache
```

**Advantages:**
- No external dependencies
- Fast and reliable
- Works offline
- Automatic schema management

**Disadvantages:**
- Single file (not distributed)
- Limited to single machine

#### Cloudflare D1 (Experimental)

```yaml
database:
  backend: "d1"
  d1:
    account_id: "your-account-id"
    database_id: "your-database-id"
    api_token: "your-api-token"
```

**Advantages:**
- Serverless/edge deployment
- Automatic replication
- Global distribution

**Disadvantages:**
- Requires Cloudflare account
- Network dependency
- API rate limits

### Buffering and Performance

```yaml
database:
  buffer_size: 1000      # Higher = more memory, less I/O
  flush_interval: "5s"   # Lower = more real-time, more I/O
  batch_size: 100        # Higher = fewer transactions
```

**High throughput (10K+ QPS):**
```yaml
database:
  buffer_size: 5000
  flush_interval: "10s"
  batch_size: 500
```

**Real-time logging:**
```yaml
database:
  buffer_size: 100
  flush_interval: "1s"
  batch_size: 50
```

### Retention Policy

```yaml
database:
  retention_days: 7      # Keep 7 days of detailed logs
```

- Logs older than `retention_days` are deleted automatically
- Statistics are kept indefinitely (aggregated)
- Runs cleanup at server startup and daily

**Examples:**
- `retention_days: 1` - Keep 24 hours
- `retention_days: 7` - Keep 1 week (default)
- `retention_days: 30` - Keep 1 month
- `retention_days: 0` - No automatic cleanup

### Disable Query Logging

```yaml
database:
  enabled: false
```

This disables:
- Query logging
- Statistics tracking
- Top domains tracking
- Query history in Web UI

## Local DNS Records

Define custom DNS records for your local network.

```yaml
local_records:
  enabled: true
  records:
    # Simple A record
    - domain: "nas.local"
      type: "A"
      ips:
        - "192.168.1.100"
      ttl: 300            # Optional, defaults to 300

    # Multiple IPs (round-robin load balancing)
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

    # Wildcard record (matches *.dev.local)
    - domain: "*.dev.local"
      type: "A"
      wildcard: true
      ips:
        - "192.168.1.200"
```

### Record Types

#### A Records (IPv4)

```yaml
- domain: "host.local"
  type: "A"
  ips:
    - "192.168.1.50"
```

#### AAAA Records (IPv6)

```yaml
- domain: "host.local"
  type: "AAAA"
  ips:
    - "2001:db8::1"
    - "fe80::1"
```

#### CNAME Records (Aliases)

```yaml
- domain: "alias.local"
  type: "CNAME"
  target: "real-host.local"
```

**CNAME Chain Resolution:**

Glory-Hole automatically resolves CNAME chains up to 10 levels:

#### TXT Records (Text Records)

```yaml
- domain: "example.local"
  type: "TXT"
  txt:
    - "v=spf1 include:_spf.example.com ~all"
    - "google-site-verification=abc123def456"
  ttl: 300
```

**Use cases:**
- SPF records for email validation
- Domain verification (Google, Microsoft, etc.)
- DKIM records
- Arbitrary text data

#### MX Records (Mail Exchange)

```yaml
- domain: "example.local"
  type: "MX"
  target: "mail.example.local"
  priority: 10
  ttl: 300

- domain: "example.local"
  type: "MX"
  target: "backup-mail.example.local"
  priority: 20
```

**Priority:** Lower values have higher preference (mail.example.local with priority 10 is preferred over backup-mail with priority 20).

#### PTR Records (Reverse DNS)

```yaml
- domain: "100.1.168.192.in-addr.arpa"
  type: "PTR"
  target: "nas.local"
  ttl: 300
```

**Use cases:**
- Reverse DNS lookups (IP to hostname)
- Email server reputation

#### SRV Records (Service Discovery)

```yaml
- domain: "_http._tcp.example.local"
  type: "SRV"
  target: "web.example.local"
  priority: 10
  weight: 60
  port: 80
  ttl: 300

- domain: "_ldap._tcp.example.local"
  type: "SRV"
  target: "dc1.example.local"
  priority: 0
  weight: 100
  port: 389
```

**Fields:**
- `priority`: Lower values have higher priority (0 is highest)
- `weight`: For same priority, higher weight = more likely to be selected
- `port`: Service port number

**Use cases:**
- Service discovery (LDAP, Kerberos, SIP)
- Load balancing across services
- Active Directory integration

#### NS Records (Nameserver)

```yaml
- domain: "subdomain.example.local"
  type: "NS"
  target: "ns1.example.local"
  ttl: 86400

- domain: "subdomain.example.local"
  type: "NS"
  target: "ns2.example.local"
  ttl: 86400
```

**Use cases:**
- Delegating subdomains to other nameservers
- Zone delegation

#### SOA Records (Start of Authority)

```yaml
- domain: "example.local"
  type: "SOA"
  ns: "ns1.example.local"           # Primary nameserver
  mbox: "admin.example.local"       # Responsible person email
  serial: 2023010101                # Zone serial number
  refresh: 3600                     # Refresh interval (seconds)
  retry: 600                        # Retry interval (seconds)
  expire: 86400                     # Expiration time (seconds)
  minttl: 300                       # Minimum TTL (seconds)
  ttl: 86400
```

**Fields:**
- `ns`: Primary nameserver for the zone (required)
- `mbox`: Email address of zone administrator (required)
- `serial`: Version number of the zone (defaults to 1)
- `refresh`: How often secondaries check for updates (defaults to 3600)
- `retry`: How often to retry failed refresh (defaults to 600)
- `expire`: When zone data expires if not refreshed (defaults to 86400)
- `minttl`: Minimum TTL for negative responses (defaults to 300)

**Use cases:**
- Zone authority declaration
- Zone transfer configuration

#### CAA Records (Certificate Authority Authorization)

```yaml
- domain: "example.com"
  type: "CAA"
  caa_tag: "issue"
  caa_value: "letsencrypt.org"
  caa_flag: 0
  ttl: 300

- domain: "example.com"
  type: "CAA"
  caa_tag: "issuewild"
  caa_value: "letsencrypt.org"
  caa_flag: 0

- domain: "example.com"
  type: "CAA"
  caa_tag: "iodef"
  caa_value: "mailto:security@example.com"
  caa_flag: 0
```

**Fields:**
- `caa_tag`: Property tag - must be one of:
  - `issue`: Authorize CA to issue certificates for this domain
  - `issuewild`: Authorize CA to issue wildcard certificates
  - `iodef`: URL/email for reporting policy violations
- `caa_value`: CA domain name (for issue/issuewild) or contact URL (for iodef)
- `caa_flag`: Flags field (optional, defaults to 0)
  - `0`: Non-critical (default)
  - `128`: Critical - CAs must understand this property

**Use cases:**
- Control which Certificate Authorities can issue SSL/TLS certificates
- Prevent unauthorized certificate issuance
- Specify notification contacts for policy violations
- Enhanced domain security and certificate transparency

### EDNS0 Support

Glory-Hole automatically handles EDNS0 (Extension Mechanisms for DNS):

**Features:**
- Automatic EDNS0 detection in requests
- Buffer size negotiation (512 - 4096 bytes)
- DNSSEC OK (DO) bit preservation
- RFC 6891 compliant

**No configuration required** - EDNS0 is handled automatically for all DNS responses.

```yaml
local_records:
  enabled: true
  records:
    - domain: "server.local"
      type: "A"
      ips: ["192.168.1.100"]

    - domain: "nas.local"
      type: "CNAME"
      target: "server.local"

    - domain: "storage.local"
      type: "CNAME"
      target: "nas.local"

# Query for storage.local → nas.local → server.local → 192.168.1.100
```

### Wildcard Records

```yaml
- domain: "*.dev.local"
  type: "A"
  wildcard: true
  ips:
    - "192.168.1.200"
```

**Matching behavior:**
- `api.dev.local` → Matches ✓
- `web.dev.local` → Matches ✓
- `api.staging.dev.local` → Does NOT match (2 levels deep)

**Note:** Wildcards only match one level deep.

### Custom TTL

```yaml
- domain: "cache-me-longer.local"
  type: "A"
  ips: ["192.168.1.50"]
  ttl: 3600  # 1 hour (default is 300 seconds)
```

### Load Balancing

Multiple IPs are returned in round-robin fashion:

```yaml
- domain: "balanced.local"
  type: "A"
  ips:
    - "192.168.1.10"  # 33% of queries
    - "192.168.1.11"  # 33% of queries
    - "192.168.1.12"  # 33% of queries
```

### Complete Example

```yaml
local_records:
  enabled: true
  records:
    # Infrastructure
    - domain: "router.local"
      type: "A"
      ips: ["192.168.1.1"]

    - domain: "switch.local"
      type: "A"
      ips: ["192.168.1.2"]

    # Servers
    - domain: "proxmox.local"
      type: "A"
      ips: ["192.168.1.10"]

    - domain: "nas.local"
      type: "A"
      ips: ["192.168.1.100"]

    - domain: "media.local"
      type: "CNAME"
      target: "nas.local"

    # Development
    - domain: "*.test.local"
      type: "A"
      wildcard: true
      ips: ["127.0.0.1"]

    # High availability
    - domain: "ha-service.local"
      type: "A"
      ips:
        - "192.168.1.20"
        - "192.168.1.21"
        - "192.168.1.22"
```

## Policy Engine

Advanced rule-based filtering using expression language.

```yaml
policy:
  enabled: true
  rules:
    # Block social media during work hours
    - name: "Block social media during work hours"
      logic: 'Hour >= 9 && Hour < 17 && (DomainMatches(Domain, "facebook") || DomainMatches(Domain, "twitter"))'
      action: "BLOCK"
      enabled: true

    # Allow internal network
    - name: "Allow internal network"
      logic: 'IPInCIDR(ClientIP, "192.168.1.0/24")'
      action: "ALLOW"
      enabled: true

    # Block after hours
    - name: "Block after hours"
      logic: 'Hour < 6 || Hour >= 23'
      action: "BLOCK"
      enabled: false
```

### Rule Structure

```yaml
- name: "Human-readable name"       # Required
  logic: "Expression to evaluate"   # Required
  action: "BLOCK|ALLOW|REDIRECT"    # Required
  action_data: "optional data"      # Optional (for REDIRECT)
  enabled: true                     # Required
```

### Available Context Fields

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `Domain` | string | Queried domain | `"example.com"` |
| `ClientIP` | string | Client IP address | `"192.168.1.50"` |
| `QueryType` | string | DNS query type | `"A"`, `"AAAA"`, `"CNAME"` |
| `Hour` | int | Current hour (0-23) | `14` (2 PM) |
| `Minute` | int | Current minute (0-59) | `30` |
| `Day` | int | Day of month (1-31) | `15` |
| `Month` | int | Month (1-12) | `6` (June) |
| `Weekday` | int | Day of week (0-6) | `0` (Sunday), `6` (Saturday) |
| `Time` | time.Time | Full timestamp | (rarely used directly) |

### Helper Functions

#### Domain Functions

**DomainMatches(domain, pattern)**
- Case-insensitive substring match
- Example: `DomainMatches(Domain, "facebook")` matches `www.facebook.com`

**DomainEndsWith(domain, suffix)**
- Check if domain ends with suffix
- Example: `DomainEndsWith(Domain, ".com")`

**DomainStartsWith(domain, prefix)**
- Check if domain starts with prefix
- Example: `DomainStartsWith(Domain, "www")`

**DomainRegex(domain, pattern)**
- Regular expression matching
- Example: `DomainRegex(Domain, "^.*\\.gov$")`

**DomainLevelCount(domain)**
- Count domain levels
- Example: `DomainLevelCount(Domain) > 3` (e.g., `a.b.c.com` = 4 levels)

#### IP Functions

**IPInCIDR(ip, cidr)**
- Check if IP is in CIDR range
- Example: `IPInCIDR(ClientIP, "192.168.1.0/24")`

**IPEquals(ip1, ip2)**
- Check IP equality (handles IPv4/IPv6 normalization)
- Example: `IPEquals(ClientIP, "192.168.1.100")`

#### Query Type Functions

**QueryTypeIn(queryType, types...)**
- Check if query type matches list
- Example: `QueryTypeIn(QueryType, "A", "AAAA")`

#### Time Functions

**IsWeekend(weekday)**
- Check if day is Saturday or Sunday
- Example: `IsWeekend(Weekday)`

**InTimeRange(hour, minute, startHour, startMinute, endHour, endMinute)**
- Check if current time is in range (handles midnight crossing)
- Example: `InTimeRange(Hour, Minute, 9, 0, 17, 30)` (9:00 AM - 5:30 PM)

### Example Rules

**Time-based blocking:**
```yaml
- name: "Block after bedtime"
  logic: "Hour >= 22 || Hour < 7"
  action: "BLOCK"
  enabled: true
```

**Weekday restrictions:**
```yaml
- name: "Block gaming on weekdays"
  logic: "Weekday >= 1 && Weekday <= 5 && DomainMatches(Domain, 'steam')"
  action: "BLOCK"
  enabled: true
```

**Client-based rules:**
```yaml
- name: "Kids' device restrictions"
  logic: "IPInCIDR(ClientIP, '192.168.1.50/32') && (Hour >= 20 || Hour < 8)"
  action: "BLOCK"
  enabled: true
```

**Network-based allow:**
```yaml
- name: "Admin subnet bypass"
  logic: "IPInCIDR(ClientIP, '192.168.100.0/24')"
  action: "ALLOW"
  enabled: true
```

**Complex domain patterns:**
```yaml
- name: "Block adult content"
  logic: |
    DomainMatches(Domain, "porn") ||
    DomainMatches(Domain, "xxx") ||
    DomainEndsWith(Domain, ".adult")
  action: "BLOCK"
  enabled: true
```

**Query type filtering:**
```yaml
- name: "Block TXT queries from guests"
  logic: "QueryType == 'TXT' && IPInCIDR(ClientIP, '192.168.2.0/24')"
  action: "BLOCK"
  enabled: false
```

### Actions

**BLOCK** - Return NXDOMAIN (domain doesn't exist)
```yaml
action: "BLOCK"
```

**ALLOW** - Bypass blocklist and forward to upstream
```yaml
action: "ALLOW"
```

**REDIRECT** - Redirect to custom IP (future feature)
```yaml
action: "REDIRECT"
action_data: "192.168.1.250"
```

### Rule Evaluation Order

1. Rules are evaluated in the order they appear
2. First matching rule wins (short-circuit evaluation)
3. If no rules match, normal processing continues (blocklist, then upstream)
4. Disabled rules (`enabled: false`) are skipped

**Example priority:**
```yaml
policy:
  enabled: true
  rules:
    # Highest priority - always evaluated first
    - name: "Admin bypass"
      logic: "IPEquals(ClientIP, '192.168.1.1')"
      action: "ALLOW"
      enabled: true

    # Middle priority
    - name: "Block social media"
      logic: "DomainMatches(Domain, 'facebook')"
      action: "BLOCK"
      enabled: true

    # Lowest priority - only reached if above don't match
    - name: "Default deny external"
      logic: "!IPInCIDR(ClientIP, '192.168.1.0/24')"
      action: "BLOCK"
      enabled: false
```

## Logging Configuration

Configure structured logging output.

```yaml
logging:
  level: "info"                   # debug, info, warn, error
  format: "text"                  # text, json
  output: "stdout"                # stdout, stderr, file
  add_source: true                # Include file:line in logs
  file_path: ""                   # Required if output=file
  max_size: 100                   # MB per log file
  max_backups: 3                  # Number of old log files
  max_age: 7                      # Days to keep old logs
```

### Log Levels

| Level | Description | Use Case |
|-------|-------------|----------|
| `debug` | Verbose debugging info | Development, troubleshooting |
| `info` | General informational messages | Production (default) |
| `warn` | Warning messages | Production |
| `error` | Error messages only | Production (minimal logging) |

### Log Formats

**Text (human-readable):**
```yaml
logging:
  format: "text"
```
Output:
```
2025-11-22T10:30:45Z INFO Glory Hole DNS starting version=0.7.8
2025-11-22T10:30:45Z INFO Blocklist manager started domains=101348
```

**JSON (machine-readable):**
```yaml
logging:
  format: "json"
```
Output:
```json
{"time":"2025-11-22T10:30:45Z","level":"INFO","msg":"Glory Hole DNS starting","version":"0.7.8"}
{"time":"2025-11-22T10:30:45Z","level":"INFO","msg":"Blocklist manager started","domains":101348}
```

### Log Output

**Standard output (default):**
```yaml
logging:
  output: "stdout"
```

**File output:**
```yaml
logging:
  output: "file"
  file_path: "/var/log/glory-hole/glory-hole.log"
  max_size: 100    # 100MB per file
  max_backups: 5   # Keep 5 old files
  max_age: 30      # Keep for 30 days
```

### Source Location

```yaml
logging:
  add_source: true  # Include file:line in logs
```

Example output with source:
```
2025-11-22T10:30:45Z INFO [dns/server.go:123] DNS query received
```

**Performance impact:** ~1-2μs per log statement

### Examples

**Production (minimal):**
```yaml
logging:
  level: "warn"
  format: "json"
  output: "file"
  file_path: "/var/log/glory-hole/glory-hole.log"
```

**Development (verbose):**
```yaml
logging:
  level: "debug"
  format: "text"
  output: "stdout"
  add_source: true
```

**Docker (structured):**
```yaml
logging:
  level: "info"
  format: "json"
  output: "stdout"  # Captured by Docker logs
```

## Telemetry Configuration

Configure metrics and tracing.

```yaml
telemetry:
  enabled: true                    # Enable telemetry
  service_name: "glory-hole"       # Service identifier
  service_version: "0.7.8"         # Version string
  prometheus_enabled: true         # Enable Prometheus metrics
  prometheus_port: 9090            # Metrics endpoint port
  tracing_enabled: false           # Enable OpenTelemetry tracing
  tracing_endpoint: ""             # OTLP endpoint (e.g., Jaeger)
```

### Prometheus Metrics

```yaml
telemetry:
  prometheus_enabled: true
  prometheus_port: 9090
```

Access metrics at: `http://localhost:9090/metrics`

**Available metrics:**
- `glory_hole_dns_queries_total` - Total DNS queries (labeled by type, result)
- `glory_hole_dns_blocked_queries_total` - Blocked queries
- `glory_hole_dns_cached_queries_total` - Cache hits
- `glory_hole_dns_query_duration_seconds` - Query latency histogram
- `glory_hole_blocklist_domains_total` - Number of blocked domains
- `glory_hole_cache_size` - Current cache entries
- `glory_hole_cache_hits_total` - Cache hit counter
- `glory_hole_cache_misses_total` - Cache miss counter

### OpenTelemetry Tracing

```yaml
telemetry:
  tracing_enabled: true
  tracing_endpoint: "http://jaeger:4318"  # OTLP HTTP endpoint
```

**Supported backends:**
- Jaeger
- Zipkin
- Tempo
- Any OTLP-compatible collector

### Disable Telemetry

```yaml
telemetry:
  enabled: false
```

## Environment Variables

Environment variables can override config file settings:

| Variable | Config Field | Example |
|----------|--------------|---------|
| `GLORY_HOLE_LISTEN_ADDRESS` | `server.listen_address` | `:53` |
| `GLORY_HOLE_WEB_UI_ADDRESS` | `server.web_ui_address` | `:8080` |
| `GLORY_HOLE_LOG_LEVEL` | `logging.level` | `debug` |
| `GLORY_HOLE_DB_PATH` | `database.sqlite.path` | `/data/db.sqlite` |

**Note:** Config file takes precedence over environment variables.

## Configuration Validation

Glory-Hole validates configuration on startup:

```bash
glory-hole -config config.yml
```

Common validation errors:

**Missing upstream servers:**
```
Error: at least one upstream DNS server must be configured
```

**Invalid log level:**
```
Error: invalid logging level: "trace" (must be debug, info, warn, or error)
```

**Port conflicts:**
```
Error: bind: address already in use
```

### Validate Without Starting

```bash
# Dry-run validation (future feature)
glory-hole -config config.yml --validate
```

## Common Patterns

### Home Network Setup

```yaml
server:
  listen_address: ":53"
  web_ui_address: ":8080"

upstream_dns_servers:
  - "1.1.1.1:53"
  - "8.8.8.8:53"

blocklists:
  - "https://raw.githubusercontent.com/StevenBlack/hosts/master/hosts"

whitelist:
  - "netflix.com"
  - "hulu.com"

local_records:
  enabled: true
  records:
    - domain: "*.home"
      type: "A"
      wildcard: true
      ips: ["192.168.1.10"]

cache:
  enabled: true
  max_entries: 10000

database:
  enabled: true
  retention_days: 7

logging:
  level: "info"
  output: "stdout"
```

### Small Office Setup

```yaml
server:
  listen_address: "10.0.0.53:53"
  web_ui_address: "10.0.0.53:8080"

upstream_dns_servers:
  - "9.9.9.9:53"    # Quad9 (security-focused)
  - "1.1.1.1:53"

blocklists:
  - "https://raw.githubusercontent.com/StevenBlack/hosts/master/hosts"
  - "https://adguardteam.github.io/HostlistsRegistry/assets/filter_1.txt"

policy:
  enabled: true
  rules:
    - name: "Allow admin subnet"
      logic: "IPInCIDR(ClientIP, '10.0.0.0/28')"
      action: "ALLOW"
      enabled: true

    - name: "Block social media during work"
      logic: "Hour >= 9 && Hour < 17 && (DomainMatches(Domain, 'facebook') || DomainMatches(Domain, 'twitter'))"
      action: "BLOCK"
      enabled: true

local_records:
  enabled: true
  records:
    - domain: "intranet.company.local"
      type: "A"
      ips: ["10.0.0.10"]

cache:
  enabled: true
  max_entries: 50000

database:
  enabled: true
  retention_days: 30

logging:
  level: "info"
  format: "json"
  output: "file"
  file_path: "/var/log/glory-hole/glory-hole.log"
```

### Kubernetes Deployment

```yaml
server:
  listen_address: ":53"
  web_ui_address: ":8080"

upstream_dns_servers:
  - "8.8.8.8:53"
  - "8.8.4.4:53"

blocklists:
  - "https://raw.githubusercontent.com/StevenBlack/hosts/master/hosts"

cache:
  enabled: true
  max_entries: 20000

database:
  enabled: true
  backend: "sqlite"
  sqlite:
    path: "/data/glory-hole.db"  # Mount PVC here
    wal_mode: true

logging:
  level: "info"
  format: "json"
  output: "stdout"  # Captured by kubectl logs

telemetry:
  enabled: true
  prometheus_enabled: true
  prometheus_port: 9090
```

## Next Steps

- [Usage Guide](usage.md) - Daily operations
- [Troubleshooting](troubleshooting.md) - Common issues
- [REST API Reference](../api/rest-api.md) - API documentation
- [Policy Engine Details](../api/policy-engine.md) - Advanced filtering
