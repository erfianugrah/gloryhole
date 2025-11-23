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
  "version": "0.7.7"
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

## Query Endpoints

### GET /api/queries

**Description:** Get recent DNS queries with pagination.

**Parameters:**
| Name | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| `limit` | int | No | `100` | Number of results (1-1000) |
| `offset` | int | No | `0` | Pagination offset |

**Request:**
```bash
# Last 100 queries (default)
curl http://localhost:8080/api/queries

# Last 50 queries
curl http://localhost:8080/api/queries?limit=50

# Pagination
curl http://localhost:8080/api/queries?limit=50&offset=100
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
      "upstream": "1.1.1.1:53"
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
- **Query Filtering**: Filter queries by domain, IP, status
- **Statistics Export**: CSV/JSON export
- **Real-time WebSocket**: Live query stream
