# Cloudflare D1 Integration Guide

This guide explains how to deploy Glory-Hole with Cloudflare D1 as the database backend for query logging.

---

## What is D1?

Cloudflare D1 is a serverless SQL database built on SQLite, designed for edge computing:

- **Serverless**: No database servers to manage
- **Distributed**: Runs at the edge, close to your users
- **SQLite-compatible**: Standard SQL syntax
- **Integrated**: Native integration with Cloudflare Workers
- **Scalable**: Designed for horizontal scale-out with many small databases
- **Cost-effective**: Pay only for reads/writes and storage

---

## When to Use D1 vs SQLite

### Use D1 When:
- ✅ Deploying to Cloudflare Workers/Pages
- ✅ Need multi-region deployment
- ✅ Want automatic backups (Time Travel)
- ✅ Building multi-tenant applications (per-user databases)
- ✅ Need zero-maintenance databases
- ✅ Have modest query volume (<1000 QPS per database)

### Use SQLite When:
- ✅ Self-hosting on your own infrastructure
- ✅ Need maximum performance (local disk access)
- ✅ Have very high query volume (>10,000 QPS)
- ✅ Want zero cloud dependencies
- ✅ Need complex transactions or long-running queries
- ✅ Have sensitive data that must stay local

---

## D1 Characteristics

### Performance
- **Throughput**: ~1,000 queries/second per database (single-threaded)
- **Latency**: HTTP overhead (~50-100ms per batch)
- **Optimization**: Use batching to reduce round trips

### Limits (Workers Paid)
- **Database size**: 10 GB max per database
- **Databases per account**: 50,000 (can be increased)
- **Queries per invocation**: 1,000 max
- **Query duration**: 30 seconds max
- **Storage per account**: 1 TB total

### Pricing (as of 2025)
- **Free Tier**: 10 databases, 500 MB each, 5 GB total
- **Paid Tier**:
  - $0.75 per 1M reads
  - $0.50 per 1M writes
  - $0.75 per GB-month storage
- **Time Travel**: Included (30 days paid, 7 days free)

---

## Architecture: Glory-Hole + D1

### Deployment Scenario

```
┌─────────────────────────────────────────────────┐
│           Cloudflare Workers/Pages              │
│                                                 │
│  ┌───────────────────────────────────────────┐ │
│  │         Glory-Hole DNS Server             │ │
│  │                                           │ │
│  │  ┌─────────────┐    ┌─────────────┐     │ │
│  │  │   DNS       │    │  Blocklist  │     │ │
│  │  │   Handler   │───▶│  Manager    │     │ │
│  │  └─────────────┘    └─────────────┘     │ │
│  │         │                                 │ │
│  │         ▼                                 │ │
│  │  ┌─────────────┐                         │ │
│  │  │   Storage   │                         │ │
│  │  │ (D1 Backend)│                         │ │
│  │  └─────────────┘                         │ │
│  │         │                                 │ │
│  └─────────┼─────────────────────────────────┘ │
│            │                                   │
│            ▼  HTTP API                        │
│  ┌─────────────────────┐                     │
│  │   Cloudflare D1     │                     │
│  │    Database         │                     │
│  └─────────────────────┘                     │
│                                               │
└───────────────────────────────────────────────┘
           │
           ▼
  Global Edge Network
```

### Data Flow

1. **DNS Query** → Worker receives DNS query
2. **Process** → Check blocklist, cache, forward if needed
3. **Log (Async)** → Buffer query metadata in memory
4. **Batch Write** → Every 10s or 500 queries, write to D1
5. **Statistics** → Hourly aggregation job updates stats table
6. **Cleanup** → Daily retention job removes old data

---

## Setup Guide

### Prerequisites

1. **Cloudflare Account**: Sign up at https://dash.cloudflare.com
2. **Workers Plan**: Free or Paid (Paid recommended for production)
3. **Wrangler CLI**: Install with `npm install -g wrangler`
4. **Go 1.25+**: For building the Glory-Hole binary

### Step 1: Create D1 Database

```bash
# Login to Cloudflare
wrangler login

# Create D1 database
wrangler d1 create glory-hole-queries

# Output:
✅ Successfully created DB 'glory-hole-queries' in region WEUR

[[d1_databases]]
binding = "DB"
database_name = "glory-hole-queries"
database_id = "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
```

Save the `database_id` for later use.

### Step 2: Initialize Database Schema

Create `schemas/d1-schema.sql`:

```sql
-- Drop existing tables
DROP TABLE IF EXISTS queries;
DROP TABLE IF EXISTS statistics;
DROP TABLE IF EXISTS domain_stats;

-- Queries table
CREATE TABLE queries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
    client_ip TEXT NOT NULL,
    domain TEXT NOT NULL,
    query_type TEXT NOT NULL,
    response_code INTEGER NOT NULL,
    blocked BOOLEAN NOT NULL,
    cached BOOLEAN NOT NULL,
    response_time_ms REAL NOT NULL,
    upstream TEXT
);

CREATE INDEX idx_queries_timestamp ON queries(timestamp);
CREATE INDEX idx_queries_domain ON queries(domain);
CREATE INDEX idx_queries_blocked ON queries(blocked);

-- Statistics table
CREATE TABLE statistics (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
    hour DATETIME NOT NULL,
    total_queries INTEGER NOT NULL,
    blocked_queries INTEGER NOT NULL,
    cached_queries INTEGER NOT NULL,
    avg_response_time_ms REAL NOT NULL,
    unique_domains INTEGER NOT NULL
);

CREATE INDEX idx_statistics_hour ON statistics(hour);

-- Domain stats table
CREATE TABLE domain_stats (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    domain TEXT NOT NULL,
    query_count INTEGER NOT NULL,
    last_queried DATETIME NOT NULL,
    blocked BOOLEAN NOT NULL
);

CREATE INDEX idx_domain_stats_domain ON domain_stats(domain);
CREATE INDEX idx_domain_stats_count ON domain_stats(query_count DESC);
```

Apply the schema:

```bash
# Apply to remote (production) database
wrangler d1 execute glory-hole-queries --remote --file=./schemas/d1-schema.sql
```

### Step 3: Configure Glory-Hole

Update `config.yml`:

```yaml
database:
  enabled: true
  backend: "d1"  # Use D1 backend

  d1:
    account_id: "${CF_ACCOUNT_ID}"       # From dashboard
    database_id: "${D1_DATABASE_ID}"     # From step 1
    api_token: "${CF_API_TOKEN}"         # API token with D1 access

  # Buffering settings (important for D1)
  buffer_size: 500          # Buffer up to 500 queries
  flush_interval: "10s"     # Flush every 10 seconds
  batch_size: 500           # Max 500 queries per batch

  # Retention
  retention_days: 30        # Keep logs for 30 days

  # Statistics
  statistics:
    enabled: true
    aggregation_interval: "1h"
```

### Step 4: Get API Credentials

1. Go to https://dash.cloudflare.com/profile/api-tokens
2. Click **Create Token**
3. Use template: **Edit Cloudflare Workers**
4. Add permission: **D1: Edit**
5. Select account and save token
6. Copy token to `.env` file:

```bash
# .env
CF_ACCOUNT_ID=your-account-id
D1_DATABASE_ID=your-database-id
CF_API_TOKEN=your-api-token
```

### Step 5: Build & Deploy

```bash
# Build Glory-Hole
go build -o glory-hole ./cmd/glory-hole/

# Run locally (uses D1 via HTTP API)
./glory-hole -config config.yml

# Or build for Workers deployment
GOOS=js GOARCH=wasm go build -o glory-hole.wasm ./cmd/glory-hole/
```

---

## D1 HTTP API Reference

Glory-Hole uses the D1 REST API for database operations:

### Execute Single Query

```http
POST https://api.cloudflare.com/client/v4/accounts/{account_id}/d1/database/{database_id}/query
Authorization: Bearer {api_token}
Content-Type: application/json

{
  "sql": "INSERT INTO queries (client_ip, domain, query_type, blocked) VALUES (?, ?, ?, ?)",
  "params": ["192.168.1.1", "example.com", "A", false]
}
```

### Execute Batch (Recommended)

```http
POST https://api.cloudflare.com/client/v4/accounts/{account_id}/d1/database/{database_id}/batch
Authorization: Bearer {api_token}
Content-Type: application/json

[
  {
    "sql": "INSERT INTO queries (...) VALUES (?, ?, ?)",
    "params": ["192.168.1.1", "example.com", "A"]
  },
  {
    "sql": "INSERT INTO queries (...) VALUES (?, ?, ?)",
    "params": ["192.168.1.2", "blocked.com", "A"]
  }
]
```

### Response Format

```json
{
  "success": true,
  "errors": [],
  "messages": [],
  "result": {
    "results": [...],
    "success": true,
    "meta": {
      "changed_db": true,
      "changes": 1,
      "duration": 0.123,
      "last_row_id": 42
    }
  }
}
```

---

## Performance Optimization

### 1. Batching Strategy

```go
// Buffer queries in memory
buffer := make([]*QueryLog, 0, 500)

// Flush when:
// - Buffer reaches 500 queries
// - 10 seconds have elapsed
// - Graceful shutdown

func (d *D1Storage) flushBuffer() error {
    if len(buffer) == 0 {
        return nil
    }

    // Build batch request
    queries := make([]D1Query, len(buffer))
    for i, log := range buffer {
        queries[i] = D1Query{
            SQL: "INSERT INTO queries (...) VALUES (?, ?, ?, ?, ?, ?, ?)",
            Params: []interface{}{
                log.ClientIP, log.Domain, log.QueryType,
                log.ResponseCode, log.Blocked, log.Cached,
                log.ResponseTimeMs,
            },
        }
    }

    // Execute batch (single HTTP request)
    return d.client.Batch(queries)
}
```

### 2. Read Optimization

```go
// Use indexes for fast lookups
SELECT * FROM queries WHERE domain = ? ORDER BY timestamp DESC LIMIT 100

// Pre-aggregate statistics
SELECT COUNT(*), AVG(response_time_ms)
FROM queries
WHERE timestamp > datetime('now', '-1 hour')
```

### 3. Write Optimization

- Use prepared statements with parameterized queries
- Batch up to 500-1000 queries per request
- Use async writes (non-blocking DNS)
- Buffer in memory during high load

---

## Monitoring & Observability

### D1 Dashboard

Monitor your database:
1. Go to https://dash.cloudflare.com
2. Navigate to **Storage & Databases** → **D1**
3. Select your database
4. View metrics:
   - Query volume
   - Storage usage
   - Error rates
   - Query duration

### Custom Metrics

```go
// Add Prometheus metrics
var (
    d1QueriesTotal = prometheus.NewCounter(...)
    d1QueryDuration = prometheus.NewHistogram(...)
    d1Errors = prometheus.NewCounter(...)
    d1BatchSize = prometheus.NewHistogram(...)
)

// Track in code
func (d *D1Storage) LogQuery(...) error {
    start := time.Now()
    defer func() {
        d1QueryDuration.Observe(time.Since(start).Seconds())
        d1QueriesTotal.Inc()
    }()

    // Log query...
}
```

### Alerting

Set up alerts for:
- High error rate (>1%)
- High query latency (>500ms p99)
- Database approaching size limit (>8GB)
- Approaching query limits (>800 QPS)

---

## Cost Estimation

### Example: 1M DNS Queries/Day

**Query Logging** (1M writes):
- 1,000,000 writes × $0.50 / 1M = **$0.50/day**

**Statistics Reads** (assume 100K reads):
- 100,000 reads × $0.75 / 1M = **$0.075/day**

**Storage** (assuming 1GB after 30 days):
- 1 GB × $0.75 / month = **$0.75/month**

**Total Monthly Cost**: ~$16

### Cost Optimization Tips

1. **Batch aggressively**: Reduce API calls
2. **Pre-aggregate**: Store hourly stats instead of querying raw data
3. **Retention policy**: Keep only 7-30 days of detailed logs
4. **Cache reads**: Cache statistics in memory for dashboard
5. **Archive old data**: Export to R2 for long-term storage ($0.015/GB/month)

---

## Backup & Recovery

### Time Travel (Point-in-Time Recovery)

D1 includes Time Travel for backups:

```bash
# Restore to 24 hours ago
wrangler d1 time-travel glory-hole-queries --timestamp="2025-01-20T12:00:00Z"

# List available restore points
wrangler d1 time-travel glory-hole-queries --list
```

**Retention**:
- Free tier: 7 days
- Paid tier: 30 days

### Manual Backups

```bash
# Export database to SQL file
wrangler d1 export glory-hole-queries --output=backup.sql

# Import from backup
wrangler d1 execute glory-hole-queries --file=backup.sql --remote
```

### Disaster Recovery

1. **Automated Time Travel**: Handled by Cloudflare
2. **Manual exports**: Weekly cron job to export to R2
3. **Cross-region replication**: Use multiple D1 databases
4. **Monitoring**: Alert on unusual patterns

---

## Migration Guide

### From SQLite to D1

```bash
# 1. Export from SQLite
sqlite3 glory-hole.db .dump > export.sql

# 2. Create D1 database
wrangler d1 create glory-hole-queries-migrated

# 3. Import to D1
wrangler d1 execute glory-hole-queries-migrated --file=export.sql --remote

# 4. Update config
# Change backend: "sqlite" → "d1"
# Add D1 credentials
```

### From D1 to SQLite

```bash
# 1. Export from D1
wrangler d1 export glory-hole-queries --output=export.sql

# 2. Import to SQLite
sqlite3 glory-hole-new.db < export.sql

# 3. Update config
# Change backend: "d1" → "sqlite"
# Update sqlite.path
```

---

## Troubleshooting

### Error: "Database not found"

```bash
# Verify database exists
wrangler d1 list

# Check database_id matches config
wrangler d1 info glory-hole-queries
```

### Error: "Unauthorized"

```bash
# Verify API token has D1 permissions
# Regenerate token with correct permissions
# Update CF_API_TOKEN in config
```

### Error: "Too many queries"

```bash
# D1 limit: 1000 queries per invocation
# Solution: Increase flush_interval
#          Reduce batch_size
#          Add backpressure handling
```

### High Latency

```bash
# Issue: HTTP overhead
# Solution: Increase batching (500-1000 queries)
#          Increase flush_interval (10-30s)
#          Use read replicas for analytics
```

---

## Best Practices

### Do's ✅

- ✅ Use batching for all writes
- ✅ Set appropriate retention policies
- ✅ Pre-aggregate statistics
- ✅ Use indexes on frequently queried columns
- ✅ Monitor costs and usage
- ✅ Enable Time Travel backups
- ✅ Use async logging (non-blocking)
- ✅ Handle API errors gracefully

### Don'ts ❌

- ❌ Don't write one query at a time (inefficient)
- ❌ Don't store unbounded data (use retention)
- ❌ Don't query raw data for dashboards (pre-aggregate)
- ❌ Don't ignore API rate limits
- ❌ Don't hardcode credentials (use env vars)
- ❌ Don't block DNS queries waiting for logging
- ❌ Don't store sensitive data without encryption

---

## Comparison: SQLite vs D1

| Feature | SQLite | D1 |
|---------|--------|-------|
| **Deployment** | Self-hosted | Cloudflare Edge |
| **Performance** | ~10K QPS | ~1K QPS |
| **Latency** | <1ms (local) | ~50ms (HTTP) |
| **Scaling** | Vertical | Horizontal (multi-DB) |
| **Management** | Manual | Serverless |
| **Backups** | Manual | Automatic (Time Travel) |
| **Cost** | Infrastructure cost | Pay-per-use |
| **Best for** | High-performance | Edge deployments |

---

## Next Steps

1. ✅ Create D1 database
2. ✅ Configure Glory-Hole with D1 backend
3. ✅ Test query logging locally
4. ✅ Deploy to production
5. ✅ Monitor metrics and costs
6. ✅ Optimize based on usage patterns

---

## Resources

- **D1 Docs**: https://developers.cloudflare.com/d1/
- **D1 API Reference**: https://developers.cloudflare.com/api/resources/d1/
- **Wrangler CLI**: https://developers.cloudflare.com/workers/wrangler/
- **D1 Pricing**: https://developers.cloudflare.com/d1/platform/pricing/
- **D1 Limits**: https://developers.cloudflare.com/d1/platform/limits/

---

**Last Updated**: 2025-11-21
**Version**: 1.0
**Status**: Ready for production
