# REST API Reference

Complete reference for Glory-Hole DNS Server REST API.

## Base URL

```
http://localhost:8080/api
```

## Authentication

Currently, no authentication is required. Future versions will support:
- API keys
- JWT tokens
- Basic auth

## Error Responses

All error responses follow this format:

```json
{
  "error": "Bad Request",
  "code": 400,
  "message": "Detailed error message"
}
```

## Configuration Endpoints (used by Settings UI)

- `GET /api/config` — return the live configuration snapshot (non-secret fields).
- `PUT /api/config/upstreams` — update `upstream_dns_servers`.
- `PUT /api/config/cache` — update cache settings (enabled, TTL bounds, shard count).
- `PUT /api/config/logging` — update logging level/format/output.
- `PUT /api/config/rate-limit` — update global rate limiter (enabled, rps, burst, action, cleanup, max tracked).
- `PUT /api/config/tls` — update DoT/TLS mode (manual PEM paths, autocert HTTP-01, native ACME DNS-01) and `dot_enabled`/`dot_address`.

> Writes persist only when the server is started with `--config /path/to/config.yml` and the file is writable; otherwise changes remain in memory.

## Health Endpoints

### GET /api/health

**Description:** Check server health and uptime.

**Request:**
```bash
curl http://localhost:8080/api/health
```

**Response:** (200 OK)
```json
{
  "status": "ok",
  "uptime": "2h15m30s",
  "version": "0.7.8"
}
```

### GET /healthz

**Description:** Kubernetes liveness probe. Returns 200 if server is alive.

**Request:**
```bash
curl http://localhost:8080/healthz
```

**Response:** (200 OK)
```json
{
  "status": "alive"
}
```

### GET /readyz

**Description:** Kubernetes readiness probe. Returns 200 if server is ready to accept traffic.

**Request:**
```bash
curl http://localhost:8080/readyz
```

**Response:** (200 OK)
```json
{
  "status": "ready",
  "checks": {
    "storage": "ok",
    "blocklist": "ok",
    "policy_engine": "ok"
  }
}
```

**Response:** (503 Service Unavailable) - if not ready
```json
{
  "status": "not_ready",
  "checks": {
    "storage": "degraded",
    "blocklist": "empty",
    "policy_engine": "not_configured"
  }
}
```

## Statistics Endpoints

### GET /api/stats

**Description:** Get DNS query statistics for a time period.

**Parameters:**
| Name | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| `since` | duration | No | `24h` | Time period (e.g., `1h`, `24h`, `7d`) |

**Request:**
```bash
# Last 24 hours (default)
curl http://localhost:8080/api/stats

# Last hour
curl http://localhost:8080/api/stats?since=1h

# Last 7 days
curl http://localhost:8080/api/stats?since=7d
```

**Response:** (200 OK)
```json
{
  "total_queries": 10000,
  "blocked_queries": 2500,
  "cached_queries": 5000,
  "block_rate": 25.0,
  "cache_hit_rate": 50.0,
  "avg_response_ms": 5.2,
  "period": "24h0m0s",
  "timestamp": "2025-11-22T10:30:00Z"
}
```

**Errors:**
- `503` - Storage not available

### GET /api/traces/stats

**Description:** Get aggregated trace statistics for blocked queries. Provides insights into how queries were blocked (blocklist, policy, rate limiting) and which rules were triggered.

**Parameters:**
| Name | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| `since` | duration or timestamp | No | `24h` | Time period (e.g., `1h`, `24h`) or RFC3339 timestamp |

**Request:**
```bash
# Last 24 hours (default)
curl http://localhost:8080/api/traces/stats

# Last hour
curl http://localhost:8080/api/traces/stats?since=1h

# Since specific timestamp
curl http://localhost:8080/api/traces/stats?since=2025-11-24T00:00:00Z
```

**Response:** (200 OK)
```json
{
  "since": "2025-11-23T10:30:00Z",
  "until": "2025-11-24T10:30:00Z",
  "total_blocked": 15432,
  "by_stage": {
    "blocklist": 12000,
    "policy": 3200,
    "cache": 232
  },
  "by_action": {
    "block": 12000,
    "BLOCK": 3200,
    "blocked_hit": 232
  },
  "by_rule": {
    "Block ads and trackers": 2100,
    "Block social media": 1100
  },
  "by_source": {
    "manager": 12000,
    "policy_engine": 3200,
    "response_cache": 232
  }
}
```

**Response Fields:**
| Field | Type | Description |
|-------|------|-------------|
| `since` | string | Start of time period (ISO 8601) |
| `until` | string | End of time period (ISO 8601) |
| `total_blocked` | int | Total number of blocked queries |
| `by_stage` | object | Counts by decision stage (blocklist, policy, cache, rate_limit) |
| `by_action` | object | Counts by action taken (block, BLOCK, blocked_hit, rate_limited) |
| `by_rule` | object | Counts by policy rule name |
| `by_source` | object | Counts by decision source (manager, policy_engine, response_cache, rate_limiter) |

**Use Cases:**
- Identify which blocklists or policies are most effective
- Understand the breakdown of blocking decisions
- Monitor policy rule effectiveness
- Debug why queries are being blocked

**Errors:**
- `503` - Storage not available

## Query Endpoints

### GET /api/queries

**Description:** Get recent DNS queries with pagination and optional trace filtering.

**Parameters:**
| Name | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| `limit` | int | No | `100` | Number of results (1-1000) |
| `offset` | int | No | `0` | Pagination offset |
| `stage` | string | No | - | Filter by decision stage (blocklist, policy, cache, rate_limit) |
| `action` | string | No | - | Filter by action (block, BLOCK, blocked_hit, rate_limited) |
| `rule` | string | No | - | Filter by policy rule name |
| `source` | string | No | - | Filter by source (manager, policy_engine, response_cache, rate_limiter) |

**Request:**
```bash
# Last 100 queries (default)
curl http://localhost:8080/api/queries

# Last 50 queries
curl http://localhost:8080/api/queries?limit=50

# Pagination
curl http://localhost:8080/api/queries?limit=50&offset=100

# Filter by stage - only policy blocks
curl 'http://localhost:8080/api/queries?stage=policy'

# Filter by source - only blocklist manager
curl 'http://localhost:8080/api/queries?source=manager'

# Filter by specific rule
curl 'http://localhost:8080/api/queries?rule=Block+social+media'

# Combine filters - policy blocks from policy engine
curl 'http://localhost:8080/api/queries?stage=policy&source=policy_engine'
```

**Response:** (200 OK)
```json
{
  "queries": [
    {
      "id": 12345,
      "timestamp": "2025-11-22T10:30:00Z",
      "client_ip": "192.168.1.100",
      "domain": "example.com",
      "query_type": "A",
      "response_code": 0,
      "blocked": false,
      "cached": true,
      "response_time_ms": 5,
      "upstream": "1.1.1.1:53",
      "block_trace": [
        {
          "stage": "blocklist",
          "action": "block",
          "source": "manager",
          "detail": "rule matched"
        }
      ]
    }
  ],
  "total": 1,
  "limit": 100,
  "offset": 0
}
```

**Query Object Fields:**
| Field | Type | Description |
|-------|------|-------------|
| `id` | int | Unique query ID |
| `timestamp` | string | ISO 8601 timestamp |
| `client_ip` | string | Client IP address |
| `domain` | string | Queried domain |
| `query_type` | string | DNS query type (A, AAAA, etc.) |
| `response_code` | int | DNS response code (0 = NOERROR) |
| `blocked` | bool | Was query blocked? |
| `cached` | bool | Was response cached? |
| `response_time_ms` | float | Response time in milliseconds |
| `upstream` | string | Upstream server used (if forwarded) |
| `block_trace` | array | (Optional) Detailed decision breadcrumbs, when `server.decision_trace` is enabled |

**Errors:**
- `503` - Storage not available

### GET /api/top-domains

**Description:** Get most queried domains.

**Parameters:**
| Name | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| `limit` | int | No | `10` | Number of results (1-100) |
| `blocked` | bool | No | `false` | Filter by blocked status |

**Request:**
```bash
# Top 10 allowed domains
curl http://localhost:8080/api/top-domains?limit=10

# Top 20 blocked domains
curl http://localhost:8080/api/top-domains?limit=20&blocked=true
```

**Response:** (200 OK)
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

**Errors:**
- `503` - Storage not available

## Blocklist Endpoints

### POST /api/blocklist/reload

**Description:** Manually trigger blocklist reload from configured sources.

**Request:**
```bash
curl -X POST http://localhost:8080/api/blocklist/reload
```

**Response:** (200 OK)
```json
{
  "status": "ok",
  "domains": 101348,
  "message": "Blocklists reloaded successfully"
}
```

**Errors:**
- `503` - Blocklist manager not available
- `500` - Reload failed

## Cache Management Endpoints

### POST /api/cache/purge

**Description:** Purge the DNS cache, clearing all cached DNS responses. Useful after blocklist updates or when troubleshooting DNS issues.

**Use Cases:**
- After reloading blocklists to ensure blocked domains aren't served from cache
- When DNS responses seem stale or incorrect
- For testing/debugging DNS resolution without cache interference

**Request:**
```bash
curl -X POST http://localhost:8080/api/cache/purge
```

**Response:** (200 OK)
```json
{
  "status": "ok",
  "message": "DNS cache purged successfully",
  "entries_cleared": 1523
}
```

**Errors:**
- `503` - Cache not available (caching disabled in config)
- `405` - Method not allowed (only POST is supported)

**Recommended Workflow:**

When updating blocklists, use this order:
```bash
# 1. Reload blocklists
curl -X POST http://localhost:8080/api/blocklist/reload

# 2. Purge DNS cache to ensure blocked domains aren't served from cache
curl -X POST http://localhost:8080/api/cache/purge
```

**Notes:**
- Cache will automatically rebuild as new queries come in
- Purging cache may temporarily increase upstream DNS load
- Cache statistics will reset to zero after purge

## Policy Endpoints

### GET /api/policies

**Description:** List all policy rules.

**Request:**
```bash
curl http://localhost:8080/api/policies
```

**Response:** (200 OK)
```json
{
  "policies": [
    {
      "id": 0,
      "name": "Block social media after hours",
      "logic": "Hour >= 22 && DomainMatches(Domain, \"facebook\")",
      "action": "BLOCK",
      "action_data": "",
      "enabled": true
    }
  ],
  "total": 1
}
```

**Errors:**
- `503` - Policy engine not configured

### GET /api/policies/{id}

**Description:** Get specific policy by ID.

**Request:**
```bash
curl http://localhost:8080/api/policies/0
```

**Response:** (200 OK)
```json
{
  "id": 0,
  "name": "Block social media after hours",
  "logic": "Hour >= 22 && DomainMatches(Domain, \"facebook\")",
  "action": "BLOCK",
  "action_data": "",
  "enabled": true
}
```

**Errors:**
- `400` - Invalid policy ID
- `404` - Policy not found
- `503` - Policy engine not configured

### POST /api/policies

**Description:** Create new policy rule.

**Request:**
```bash
curl -X POST http://localhost:8080/api/policies \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Block after hours",
    "logic": "Hour >= 22 || Hour < 6",
    "action": "BLOCK",
    "enabled": true
  }'
```

**Request Body:**
```json
{
  "name": "Rule name (required)",
  "logic": "Expression (required)",
  "action": "BLOCK|ALLOW|REDIRECT (required)",
  "action_data": "Optional data for action",
  "enabled": true
}
```

**Response:** (201 Created)
```json
{
  "id": 1,
  "name": "Block after hours",
  "logic": "Hour >= 22 || Hour < 6",
  "action": "BLOCK",
  "action_data": "",
  "enabled": true
}
```

**Errors:**
- `400` - Invalid request (missing fields, invalid action, etc.)
- `503` - Policy engine not configured

### PUT /api/policies/{id}

**Description:** Update existing policy rule.

**Request:**
```bash
curl -X PUT http://localhost:8080/api/policies/0 \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Updated rule name",
    "logic": "Hour >= 23",
    "action": "BLOCK",
    "enabled": true
  }'
```

**Request Body:** Same as POST

**Response:** (200 OK)
```json
{
  "id": 0,
  "name": "Updated rule name",
  "logic": "Hour >= 23",
  "action": "BLOCK",
  "action_data": "",
  "enabled": true
}
```

**Errors:**
- `400` - Invalid policy ID or request body
- `404` - Policy not found
- `503` - Policy engine not configured

### DELETE /api/policies/{id}

**Description:** Delete policy rule.

**Request:**
```bash
curl -X DELETE http://localhost:8080/api/policies/0
```

**Response:** (200 OK)
```json
{
  "message": "Policy deleted successfully",
  "id": 0,
  "name": "Block after hours"
}
```

**Errors:**
- `400` - Invalid policy ID
- `404` - Policy not found
- `500` - Failed to remove policy
- `503` - Policy engine not configured

## Web UI Endpoints

These endpoints return HTML partials for the web interface.

### GET /api/ui/stats

**Description:** Get statistics as HTML partial.

**Used by:** Dashboard auto-refresh

### GET /api/ui/queries

**Description:** Get query log as HTML partial.

**Used by:** Query log auto-refresh

### GET /api/ui/top-domains

**Description:** Get top domains as HTML partial.

**Used by:** Dashboard top domains section

## Whitelist Management Endpoints

### GET /api/whitelist

**Description:** Get all whitelisted domains and patterns.

**Request:**
```bash
curl http://localhost:8080/api/whitelist
```

**Response:** (200 OK)
```json
{
  "total": 4,
  "exact": ["analytics.google.com", "github-cloud.s3.amazonaws.com"],
  "wildcard": ["*.taskassist-pa.clients6.google.com"],
  "regex": ["^cdn\\d+\\.example\\.com$"]
}
```

**Response Fields:**
| Field | Type | Description |
|-------|------|-------------|
| `total` | int | Total number of whitelist entries |
| `exact` | []string | Exact domain matches |
| `wildcard` | []string | Wildcard patterns (e.g., `*.example.com`) |
| `regex` | []string | Regular expression patterns |

### POST /api/whitelist

**Description:** Add a domain or pattern to the whitelist. Whitelisted domains will never be blocked, even if they appear in blocklists.

**Request:**
```bash
# Add exact domain
curl -X POST http://localhost:8080/api/whitelist \
  -H "Content-Type: application/json" \
  -d '{"domain": "analytics.google.com"}'

# Add wildcard pattern
curl -X POST http://localhost:8080/api/whitelist \
  -H "Content-Type: application/json" \
  -d '{"domain": "*.cdn.example.com"}'

# Add regex pattern
curl -X POST http://localhost:8080/api/whitelist \
  -H "Content-Type: application/json" \
  -d '{"domain": "^api\\d+\\.example\\.com$"}'
```

**Request Body:**
| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `domain` | string | Yes | Domain or pattern to whitelist |

**Response:** (200 OK)
```json
{
  "message": "Domain added to whitelist",
  "total": 5,
  "exact": 3,
  "wildcard": 1,
  "regex": 1
}
```

**Errors:**
- `400` - Invalid request (missing domain)
- `409` - Domain already whitelisted
- `500` - Failed to save configuration

### DELETE /api/whitelist/{domain}

**Description:** Remove a domain or pattern from the whitelist.

**Request:**
```bash
# Remove exact domain
curl -X DELETE "http://localhost:8080/api/whitelist/analytics.google.com"

# Remove wildcard pattern (URL-encode the *)
curl -X DELETE "http://localhost:8080/api/whitelist/*.cdn.example.com"
```

**Response:** (200 OK)
```json
{
  "message": "Domain removed from whitelist",
  "total": 4,
  "exact": 2,
  "wildcard": 1,
  "regex": 1
}
```

**Errors:**
- `404` - Domain not found in whitelist
- `500` - Failed to save configuration

## Local Records Management Endpoints

### GET /api/localrecords

**Description:** Get all local DNS records.

**Request:**
```bash
curl http://localhost:8080/api/localrecords
```

**Response:** (200 OK)
```json
{
  "total": 3,
  "records": [
    {
      "id": "router.local.:A:0",
      "domain": "router.local.",
      "type": "A",
      "ips": ["192.168.1.1"],
      "target": "",
      "ttl": 300,
      "wildcard": false
    },
    {
      "id": "mail.local.:AAAA:0",
      "domain": "mail.local.",
      "type": "AAAA",
      "ips": ["2001:db8::1"],
      "target": "",
      "ttl": 600,
      "wildcard": false
    },
    {
      "id": "www.local.:CNAME:0",
      "domain": "www.local.",
      "type": "CNAME",
      "ips": [],
      "target": "router.local.",
      "ttl": 300,
      "wildcard": false
    }
  ]
}
```

**Response Fields:**
| Field | Type | Description |
|-------|------|-------------|
| `total` | int | Total number of local records |
| `records` | []object | Array of DNS records |
| `records[].id` | string | Unique identifier (`domain:type:index`) |
| `records[].domain` | string | Domain name (with trailing dot) |
| `records[].type` | string | Record type (`A`, `AAAA`, or `CNAME`) |
| `records[].ips` | []string | IP addresses (for A/AAAA records) |
| `records[].target` | string | Target domain (for CNAME records) |
| `records[].ttl` | int | Time-to-live in seconds |
| `records[].wildcard` | bool | Whether this is a wildcard record |

### POST /api/localrecords

**Description:** Add a new local DNS record.

**Request:**
```bash
# Add A record
curl -X POST http://localhost:8080/api/localrecords \
  -H "Content-Type: application/json" \
  -d '{
    "domain": "nas.local",
    "type": "A",
    "ips": ["192.168.1.100"],
    "ttl": 300
  }'

# Add AAAA record
curl -X POST http://localhost:8080/api/localrecords \
  -H "Content-Type: application/json" \
  -d '{
    "domain": "server.local",
    "type": "AAAA",
    "ips": ["2001:db8::100"],
    "ttl": 600
  }'

# Add CNAME record
curl -X POST http://localhost:8080/api/localrecords \
  -H "Content-Type: application/json" \
  -d '{
    "domain": "www.local",
    "type": "CNAME",
    "target": "nas.local",
    "ttl": 300
  }'

# Add A record with multiple IPs (round-robin)
curl -X POST http://localhost:8080/api/localrecords \
  -H "Content-Type: application/json" \
  -d '{
    "domain": "web.local",
    "type": "A",
    "ips": ["192.168.1.10", "192.168.1.11", "192.168.1.12"],
    "ttl": 300
  }'
```

**Request Body:**
| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `domain` | string | Yes | Domain name (trailing dot optional) |
| `type` | string | Yes | Record type: `A`, `AAAA`, or `CNAME` |
| `ips` | []string | For A/AAAA | IP addresses (can specify multiple for round-robin) |
| `target` | string | For CNAME | Target domain name |
| `ttl` | int | No | Time-to-live in seconds (default: 300) |

**Response:** (200 OK)
```json
{
  "message": "Local record added successfully",
  "record": {
    "id": "nas.local.:A:0",
    "domain": "nas.local.",
    "type": "A",
    "ips": ["192.168.1.100"],
    "ttl": 300,
    "wildcard": false
  }
}
```

**Errors:**
- `400` - Invalid request (missing required fields, invalid IP format, etc.)
- `500` - Failed to save configuration

**Validation Rules:**
- Domain is required
- Type must be `A`, `AAAA`, or `CNAME`
- A/AAAA records require at least one valid IP address
- CNAME records require a target domain
- TTL must be positive

### DELETE /api/localrecords/{id}

**Description:** Remove a local DNS record.

**Request:**
```bash
curl -X DELETE "http://localhost:8080/api/localrecords/nas.local.:A:0"
```

**Path Parameters:**
| Name | Type | Description |
|------|------|-------------|
| `id` | string | Record identifier in format `domain:type:index` |

**Response:** (200 OK)
```json
{
  "message": "Local record removed successfully",
  "total": 2
}
```

**Errors:**
- `400` - Invalid ID format
- `404` - Record not found
- `500` - Failed to save configuration

## Conditional Forwarding Management Endpoints

### GET /api/conditionalforwarding

**Description:** Get all conditional forwarding rules. Rules are returned sorted by priority (highest first).

**Request:**
```bash
curl http://localhost:8080/api/conditionalforwarding
```

**Response:** (200 OK)
```json
{
  "enabled": true,
  "total": 2,
  "rules": [
    {
      "id": "Corporate VPN:90:0",
      "name": "Corporate VPN",
      "domains": ["*.corp.example.com", "*.internal"],
      "client_cidrs": [],
      "query_types": [],
      "upstreams": ["10.0.0.1:53", "10.0.0.2:53"],
      "priority": 90,
      "timeout": "3s",
      "max_retries": 2,
      "failover": true,
      "enabled": true
    },
    {
      "id": "Home Network:50:1",
      "name": "Home Network",
      "domains": [],
      "client_cidrs": ["192.168.1.0/24"],
      "query_types": ["PTR"],
      "upstreams": ["192.168.1.1:53"],
      "priority": 50,
      "timeout": "",
      "max_retries": 0,
      "failover": false,
      "enabled": true
    }
  ]
}
```

**Response Fields:**
| Field | Type | Description |
|-------|------|-------------|
| `enabled` | bool | Whether conditional forwarding is enabled globally |
| `total` | int | Total number of rules |
| `rules` | []object | Array of forwarding rules (sorted by priority) |
| `rules[].id` | string | Unique identifier (`name:priority:index`) |
| `rules[].name` | string | Rule name |
| `rules[].domains` | []string | Domain patterns to match (e.g., `*.local`, `example.com`) |
| `rules[].client_cidrs` | []string | Client IP ranges to match (CIDR notation) |
| `rules[].query_types` | []string | DNS query types to match (e.g., `A`, `PTR`) |
| `rules[].upstreams` | []string | Upstream DNS servers for this rule |
| `rules[].priority` | int | Rule priority (1-100, higher = evaluated first) |
| `rules[].timeout` | string | Query timeout (e.g., `2s`, `500ms`) |
| `rules[].max_retries` | int | Maximum retry attempts |
| `rules[].failover` | bool | Whether to try next upstream on failure |
| `rules[].enabled` | bool | Whether rule is active |

### POST /api/conditionalforwarding

**Description:** Add a new conditional forwarding rule.

**Request:**
```bash
# Forward local network queries to local DNS
curl -X POST http://localhost:8080/api/conditionalforwarding \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Local Network",
    "domains": ["*.local", "*.lan"],
    "upstreams": ["192.168.1.1:53"],
    "priority": 80
  }'

# Forward PTR queries from specific clients
curl -X POST http://localhost:8080/api/conditionalforwarding \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Reverse DNS",
    "client_cidrs": ["10.0.0.0/8"],
    "query_types": ["PTR"],
    "upstreams": ["10.0.0.1:53"],
    "priority": 70,
    "timeout": "3s",
    "max_retries": 2,
    "failover": true
  }'

# Complex rule with multiple matchers
curl -X POST http://localhost:8080/api/conditionalforwarding \
  -H "Content-Type: application/json" \
  -d '{
    "name": "VPN Network",
    "domains": ["*.vpn.corp"],
    "client_cidrs": ["172.16.0.0/12"],
    "query_types": ["A", "AAAA"],
    "upstreams": ["172.16.0.1:53", "172.16.0.2:53"],
    "priority": 90,
    "timeout": "5s",
    "max_retries": 3,
    "failover": true
  }'
```

**Request Body:**
| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Rule name (must be unique) |
| `domains` | []string | * | Domain patterns (wildcards supported) |
| `client_cidrs` | []string | * | Client IP ranges in CIDR notation |
| `query_types` | []string | * | DNS query types (A, AAAA, PTR, etc.) |
| `upstreams` | []string | Yes | Upstream DNS servers (IP:port format) |
| `priority` | int | No | Priority 1-100 (default: 50, higher = first) |
| `timeout` | string | No | Query timeout (e.g., `2s`, `500ms`) |
| `max_retries` | int | No | Retry attempts (default: 0) |
| `failover` | bool | No | Try next upstream on failure (default: false) |

**Note:** At least one matching condition is required (`domains`, `client_cidrs`, or `query_types`).

**Response:** (200 OK)
```json
{
  "message": "Conditional forwarding rule added successfully",
  "rule": {
    "id": "Local Network:80:0",
    "name": "Local Network",
    "domains": ["*.local", "*.lan"],
    "client_cidrs": [],
    "query_types": [],
    "upstreams": ["192.168.1.1:53"],
    "priority": 80,
    "timeout": "",
    "max_retries": 0,
    "failover": false,
    "enabled": true
  }
}
```

**Errors:**
- `400` - Invalid request (missing required fields, invalid priority/timeout, etc.)
- `409` - Rule with same name already exists
- `500` - Failed to save configuration

**Validation Rules:**
- Name is required and must be unique
- At least one matching condition required (domains, client_cidrs, or query_types)
- At least one upstream DNS server required
- Priority must be between 1 and 100
- Timeout must be valid duration format (if specified)
- Client CIDRs must be valid CIDR notation

### DELETE /api/conditionalforwarding/{id}

**Description:** Remove a conditional forwarding rule.

**Request:**
```bash
curl -X DELETE "http://localhost:8080/api/conditionalforwarding/Local%20Network:80:0"
```

**Path Parameters:**
| Name | Type | Description |
|------|------|-------------|
| `id` | string | Rule identifier in format `name:priority:index` |

**Response:** (200 OK)
```json
{
  "message": "Conditional forwarding rule removed successfully",
  "total": 1
}
```

**Errors:**
- `400` - Invalid ID format
- `404` - Rule not found
- `500` - Failed to save configuration

## CORS Headers

All API endpoints include CORS headers:

```
Access-Control-Allow-Origin: *
Access-Control-Allow-Methods: GET, POST, PUT, DELETE, OPTIONS
Access-Control-Allow-Headers: Content-Type
```

## Rate Limiting

Currently, no rate limiting is implemented. Future versions will support:
- Per-IP rate limits
- Per-API-key rate limits
- Configurable limits

## Examples

### Monitor Queries in Real-Time

```bash
# Poll query endpoint every 2 seconds
watch -n 2 "curl -s http://localhost:8080/api/queries?limit=10"
```

### Track Block Rate Over Time

```bash
# Log block rate every minute
while true; do
  curl -s http://localhost:8080/api/stats | jq '.block_rate'
  sleep 60
done
```

### Export Queries to JSON

```bash
# Export last 1000 queries
curl "http://localhost:8080/api/queries?limit=1000" > queries-export.json
```

### Automate Policy Updates

```bash
# Add blocking rule during business hours
curl -X POST http://localhost:8080/api/policies \
  -H "Content-Type: application/json" \
  -d @- <<EOF
{
  "name": "Block gaming during work",
  "logic": "Hour >= 9 && Hour < 17 && (Weekday >= 1 && Weekday <= 5) && DomainMatches(Domain, 'steam')",
  "action": "BLOCK",
  "enabled": true
}
EOF
```

## Response Codes

| Code | Meaning | Description |
|------|---------|-------------|
| 200 | OK | Request successful |
| 201 | Created | Resource created successfully |
| 400 | Bad Request | Invalid request parameters |
| 404 | Not Found | Resource not found |
| 405 | Method Not Allowed | HTTP method not allowed |
| 500 | Internal Server Error | Server error occurred |
| 503 | Service Unavailable | Service not available (disabled or error) |

## Future API Additions

Planned for future releases:

- **Authentication**: API keys, JWT tokens
- **Rate Limiting**: Configurable limits per IP/key
- **Webhooks**: Notify on events (blocklist update, high block rate, etc.)
- **Bulk Operations**: Batch create/update/delete policies
- **Enhanced Query Filtering**: Filter by domain, IP, date range
- **Statistics Export**: CSV/JSON export
- **Real-time WebSocket**: Live query stream
