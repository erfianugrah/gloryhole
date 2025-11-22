# Usage Guide

This guide covers day-to-day operations and common tasks with Glory-Hole DNS Server.

## Table of Contents

- [Starting and Stopping](#starting-and-stopping)
- [Viewing Logs](#viewing-logs)
- [Managing Blocklists](#managing-blocklists)
- [Managing Policies](#managing-policies)
- [Viewing Statistics](#viewing-statistics)
- [Query History](#query-history)
- [Local DNS Records](#local-dns-records)
- [Backup and Restore](#backup-and-restore)
- [Performance Monitoring](#performance-monitoring)
- [Common Tasks](#common-tasks)

## Starting and Stopping

### Binary Installation

**Start the server:**
```bash
# With default config location (./config.yml)
glory-hole

# With custom config
glory-hole -config /etc/glory-hole/config.yml

# Run in background
nohup glory-hole -config config.yml > glory-hole.log 2>&1 &
```

**Stop the server:**
```bash
# Find process ID
ps aux | grep glory-hole

# Send termination signal
kill -TERM <PID>

# Or use SIGINT (Ctrl+C if running in foreground)
kill -INT <PID>
```

**Graceful shutdown:**
- Glory-Hole handles SIGTERM and SIGINT signals
- Flushes query buffer to database
- Closes connections gracefully
- 5-second timeout for shutdown

### Systemd Service

**Start the service:**
```bash
sudo systemctl start glory-hole
```

**Stop the service:**
```bash
sudo systemctl stop glory-hole
```

**Restart the service:**
```bash
sudo systemctl restart glory-hole
```

**Reload configuration:**
```bash
# Note: Full restart required for config changes
sudo systemctl restart glory-hole
```

**Enable auto-start on boot:**
```bash
sudo systemctl enable glory-hole
```

**Check service status:**
```bash
sudo systemctl status glory-hole
```

Example output:
```
● glory-hole.service - Glory-Hole DNS Server
     Loaded: loaded (/etc/systemd/system/glory-hole.service; enabled)
     Active: active (running) since Thu 2025-11-22 10:30:00 UTC; 2h 15min ago
   Main PID: 1234 (glory-hole)
      Tasks: 12 (limit: 4915)
     Memory: 45.2M
        CPU: 1min 23s
     CGroup: /system.slice/glory-hole.service
             └─1234 /usr/local/bin/glory-hole -config /etc/glory-hole/config.yml
```

### Docker

**Start container:**
```bash
docker start glory-hole
```

**Stop container:**
```bash
docker stop glory-hole
```

**Restart container:**
```bash
docker restart glory-hole
```

**View running containers:**
```bash
docker ps | grep glory-hole
```

**Remove and recreate (after config changes):**
```bash
docker rm -f glory-hole
docker run -d \
  --name glory-hole \
  -p 53:53/udp \
  -p 53:53/tcp \
  -p 8080:8080 \
  -v $(pwd)/config.yml:/etc/glory-hole/config.yml:ro \
  erfianugrah/gloryhole:latest
```

### Docker Compose

**Start services:**
```bash
docker-compose up -d
```

**Stop services:**
```bash
docker-compose down
```

**Restart specific service:**
```bash
docker-compose restart glory-hole
```

**View logs:**
```bash
docker-compose logs -f glory-hole
```

### Kubernetes

**Scale deployment:**
```bash
# Scale to 3 replicas
kubectl scale deployment glory-hole --replicas=3
```

**Restart pods:**
```bash
# Delete pods (they will be recreated)
kubectl delete pods -l app=glory-hole

# Or rollout restart
kubectl rollout restart deployment glory-hole
```

**Check pod status:**
```bash
kubectl get pods -l app=glory-hole
kubectl describe pod <pod-name>
```

## Viewing Logs

### Systemd Service

**Follow logs in real-time:**
```bash
sudo journalctl -u glory-hole -f
```

**View recent logs:**
```bash
sudo journalctl -u glory-hole -n 100
```

**View logs since timestamp:**
```bash
sudo journalctl -u glory-hole --since "2025-11-22 10:00:00"
```

**View logs for today:**
```bash
sudo journalctl -u glory-hole --since today
```

**Search logs:**
```bash
sudo journalctl -u glory-hole | grep ERROR
```

### Docker

**Follow container logs:**
```bash
docker logs -f glory-hole
```

**View last N lines:**
```bash
docker logs --tail 100 glory-hole
```

**View logs since timestamp:**
```bash
docker logs --since 2025-11-22T10:00:00 glory-hole
```

### Kubernetes

**Follow pod logs:**
```bash
kubectl logs -f -l app=glory-hole
```

**View logs from specific pod:**
```bash
kubectl logs <pod-name>
```

**View previous container logs (after crash):**
```bash
kubectl logs <pod-name> --previous
```

### Log Levels

Change log verbosity in `config.yml`:

```yaml
logging:
  level: "debug"  # debug, info, warn, error
```

**Debug level** shows:
- All DNS queries
- Cache hits/misses
- Blocklist matches
- Policy evaluations
- Internal operations

**Info level** shows:
- Server startup/shutdown
- Blocklist updates
- Configuration changes
- Errors and warnings

## Managing Blocklists

### Automatic Updates

Configure automatic updates in `config.yml`:

```yaml
auto_update_blocklists: true
update_interval: "24h"  # Options: "6h", "12h", "24h", "7d"
```

Blocklists update automatically at the specified interval.

### Manual Reload via API

**Trigger reload:**
```bash
curl -X POST http://localhost:8080/api/blocklist/reload
```

**Response:**
```json
{
  "status": "ok",
  "domains": 101348,
  "message": "Blocklists reloaded successfully"
}
```

### Manual Reload via Web UI

1. Open `http://localhost:8080/settings`
2. Click "Reload Blocklists" button
3. Wait for completion (30-60 seconds)
4. View updated domain count

### Check Blocklist Status

**Via API:**
```bash
curl http://localhost:8080/api/stats
```

Look for blocklist metrics in response.

**Via Logs:**
```bash
# Systemd
sudo journalctl -u glory-hole | grep "Blocklist"

# Docker
docker logs glory-hole | grep "Blocklist"
```

Example output:
```
INFO Blocklist manager started domains=101348 auto_update=true
INFO Blocklists reloaded successfully domains=101348 duration=15.3s
```

### Adding/Removing Blocklists

Edit `config.yml`:

```yaml
blocklists:
  - "https://raw.githubusercontent.com/StevenBlack/hosts/master/hosts"
  - "https://adguardteam.github.io/HostlistsRegistry/assets/filter_1.txt"
  # Add new source
  - "https://big.oisd.nl/domainswild"
```

Restart the server:
```bash
sudo systemctl restart glory-hole
```

### Whitelist Management

Edit `config.yml`:

```yaml
whitelist:
  - "analytics.google.com"
  - "github-cloud.s3.amazonaws.com"
  # Add domains to never block
  - "example-important-site.com"
```

Restart the server after changes.

## Managing Policies

### Via Web UI

**Access policy management:**
```
http://localhost:8080/policies
```

**Add a new policy:**
1. Click "Add Policy" button
2. Fill in form:
   - **Name**: Descriptive name
   - **Logic**: Expression (e.g., `Hour >= 22`)
   - **Action**: BLOCK, ALLOW, or REDIRECT
   - **Enabled**: Toggle on/off
3. Click "Save Policy"

**Edit existing policy:**
1. Click "Edit" button on policy card
2. Modify fields
3. Click "Update Policy"

**Delete policy:**
1. Click "Delete" button on policy card
2. Confirm deletion

**Enable/Disable policy:**
- Use toggle switch on policy card
- Changes apply immediately

### Via REST API

**List all policies:**
```bash
curl http://localhost:8080/api/policies
```

**Get specific policy:**
```bash
curl http://localhost:8080/api/policies/0
```

**Add new policy:**
```bash
curl -X POST http://localhost:8080/api/policies \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Block social media after hours",
    "logic": "Hour >= 22 && DomainMatches(Domain, \"facebook\")",
    "action": "BLOCK",
    "enabled": true
  }'
```

**Update policy:**
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

**Delete policy:**
```bash
curl -X DELETE http://localhost:8080/api/policies/0
```

### Via Configuration File

Edit `config.yml`:

```yaml
policy:
  enabled: true
  rules:
    - name: "Block after hours"
      logic: "Hour >= 22 || Hour < 6"
      action: "BLOCK"
      enabled: true

    - name: "Allow internal network"
      logic: "IPInCIDR(ClientIP, '192.168.1.0/24')"
      action: "ALLOW"
      enabled: true
```

Restart the server:
```bash
sudo systemctl restart glory-hole
```

### Policy Examples

**Time-based blocking:**
```json
{
  "name": "Block gaming sites during work",
  "logic": "Hour >= 9 && Hour < 17 && (Weekday >= 1 && Weekday <= 5) && DomainMatches(Domain, 'steam')",
  "action": "BLOCK",
  "enabled": true
}
```

**Client-based allow:**
```json
{
  "name": "Admin subnet bypass",
  "logic": "IPInCIDR(ClientIP, '192.168.100.0/24')",
  "action": "ALLOW",
  "enabled": true
}
```

**Domain pattern blocking:**
```json
{
  "name": "Block tracking domains",
  "logic": "DomainMatches(Domain, 'tracker') || DomainMatches(Domain, 'analytics')",
  "action": "BLOCK",
  "enabled": true
}
```

## Viewing Statistics

### Web UI Dashboard

Access: `http://localhost:8080/`

**Real-time stats cards:**
- Total Queries
- Blocked Queries (with percentage)
- Cached Queries (with hit rate)
- Average Response Time

**Query activity chart:**
- Hourly query counts (last 24 hours)
- Line chart showing trends
- Auto-refreshes every 30 seconds

**Top domains:**
- Top 10 allowed domains
- Top 10 blocked domains
- Query counts per domain

### REST API

**Get overall statistics:**
```bash
# Last 24 hours (default)
curl http://localhost:8080/api/stats

# Last hour
curl http://localhost:8080/api/stats?since=1h

# Last 7 days
curl http://localhost:8080/api/stats?since=7d
```

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
  "timestamp": "2025-11-22T10:30:00Z"
}
```

**Get top domains:**
```bash
# Top 10 allowed domains
curl http://localhost:8080/api/top-domains?limit=10

# Top 20 blocked domains
curl http://localhost:8080/api/top-domains?limit=20&blocked=true
```

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

### Prometheus Metrics

Access: `http://localhost:9090/metrics`

**Key metrics:**
```
# Total DNS queries
glory_hole_dns_queries_total{type="A",result="success"} 10000

# Blocked queries
glory_hole_dns_blocked_queries_total 2500

# Cache hits
glory_hole_cache_hits_total 5000

# Query duration (histogram)
glory_hole_dns_query_duration_seconds_bucket{le="0.005"} 8000
glory_hole_dns_query_duration_seconds_bucket{le="0.01"} 9500
glory_hole_dns_query_duration_seconds_sum 52.5
glory_hole_dns_query_duration_seconds_count 10000
```

**Query with PromQL:**
```bash
# Cache hit rate (last 5 minutes)
rate(glory_hole_cache_hits_total[5m]) / rate(glory_hole_dns_queries_total[5m])

# Block rate
rate(glory_hole_dns_blocked_queries_total[5m]) / rate(glory_hole_dns_queries_total[5m])

# 95th percentile latency
histogram_quantile(0.95, rate(glory_hole_dns_query_duration_seconds_bucket[5m]))
```

## Query History

### Web UI Query Log

Access: `http://localhost:8080/queries`

**Features:**
- Real-time query stream (updates every 2 seconds)
- Filter by domain, client IP, status
- Color-coded badges:
  - Green: Allowed
  - Red: Blocked
  - Blue: Cached
- Pagination controls
- Auto-scroll to new queries

**Columns shown:**
- Timestamp
- Client IP
- Domain
- Query Type (A, AAAA, CNAME, etc.)
- Status (Allowed/Blocked/Cached)
- Response Time (ms)
- Upstream Server (if forwarded)

### REST API Query History

**Get recent queries:**
```bash
# Last 100 queries (default)
curl http://localhost:8080/api/queries

# Last 50 queries
curl http://localhost:8080/api/queries?limit=50

# Pagination
curl http://localhost:8080/api/queries?limit=50&offset=100
```

**Response:**
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

### Direct Database Query

If using SQLite backend:

```bash
# Connect to database
sqlite3 ./glory-hole.db

# Query recent logs
SELECT timestamp, client_ip, domain, query_type, blocked
FROM query_logs
ORDER BY timestamp DESC
LIMIT 10;

# Count queries by domain
SELECT domain, COUNT(*) as count
FROM query_logs
WHERE timestamp > datetime('now', '-24 hours')
GROUP BY domain
ORDER BY count DESC
LIMIT 20;

# Block rate by hour
SELECT
  strftime('%Y-%m-%d %H:00', timestamp) as hour,
  COUNT(*) as total,
  SUM(CASE WHEN blocked THEN 1 ELSE 0 END) as blocked
FROM query_logs
WHERE timestamp > datetime('now', '-7 days')
GROUP BY hour
ORDER BY hour DESC;
```

## Local DNS Records

### Manage via Configuration

Edit `config.yml`:

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
```

Restart server after changes.

### Test Local Records

```bash
# Test A record
dig @localhost server.local

# Test CNAME
dig @localhost nas.local

# Test wildcard
dig @localhost api.dev.local
```

### Common Use Cases

**Development environment:**
```yaml
local_records:
  enabled: true
  records:
    - domain: "*.test.local"
      type: "A"
      wildcard: true
      ips: ["127.0.0.1"]
```

**Internal services:**
```yaml
local_records:
  enabled: true
  records:
    - domain: "gitlab.company.local"
      type: "A"
      ips: ["10.0.0.50"]

    - domain: "jenkins.company.local"
      type: "A"
      ips: ["10.0.0.51"]
```

**Load-balanced services:**
```yaml
local_records:
  enabled: true
  records:
    - domain: "app.local"
      type: "A"
      ips:
        - "192.168.1.10"
        - "192.168.1.11"
        - "192.168.1.12"
```

## Backup and Restore

### Backup Configuration

```bash
# Backup config file
cp /etc/glory-hole/config.yml /backup/config.yml.$(date +%Y%m%d)

# Backup with systemd service
sudo systemctl stop glory-hole
tar czf glory-hole-backup.tar.gz \
  /etc/glory-hole/config.yml \
  /var/lib/glory-hole/glory-hole.db
sudo systemctl start glory-hole
```

### Backup Database

```bash
# SQLite backup (hot backup, no downtime)
sqlite3 ./glory-hole.db ".backup glory-hole-backup.db"

# Or copy file (stop server first)
sudo systemctl stop glory-hole
cp /var/lib/glory-hole/glory-hole.db /backup/glory-hole.db.$(date +%Y%m%d)
sudo systemctl start glory-hole
```

### Restore from Backup

```bash
# Restore config
sudo systemctl stop glory-hole
cp /backup/config.yml /etc/glory-hole/config.yml

# Restore database
cp /backup/glory-hole.db /var/lib/glory-hole/glory-hole.db
sudo chown glory-hole:glory-hole /var/lib/glory-hole/glory-hole.db

sudo systemctl start glory-hole
```

### Docker Backup

```bash
# Backup volumes
docker run --rm \
  -v glory-hole-data:/data \
  -v $(pwd):/backup \
  alpine tar czf /backup/glory-hole-data.tar.gz /data

# Restore volumes
docker run --rm \
  -v glory-hole-data:/data \
  -v $(pwd):/backup \
  alpine tar xzf /backup/glory-hole-data.tar.gz -C /
```

## Performance Monitoring

### Health Checks

**Basic health:**
```bash
curl http://localhost:8080/api/health
```

**Liveness probe (Kubernetes):**
```bash
curl http://localhost:8080/healthz
```

**Readiness probe (Kubernetes):**
```bash
curl http://localhost:8080/readyz
```

### Resource Usage

**Linux:**
```bash
# CPU and memory
top -p $(pgrep glory-hole)

# Detailed stats
ps aux | grep glory-hole

# Memory breakdown
sudo pmap $(pgrep glory-hole)
```

**Docker:**
```bash
# Container stats
docker stats glory-hole

# Detailed inspect
docker inspect glory-hole
```

### Query Performance

**Measure DNS query latency:**
```bash
# Using dig with timing
dig @localhost google.com | grep "Query time"

# Multiple queries with average
for i in {1..10}; do
  dig @localhost google.com +stats | grep "Query time"
done
```

**Benchmark with dnsperf:**
```bash
# Install dnsperf
sudo apt-get install dnsperf

# Create query file
cat > queries.txt <<EOF
google.com A
facebook.com A
twitter.com A
EOF

# Run benchmark
dnsperf -s 127.0.0.1 -d queries.txt -l 30
```

## Common Tasks

### Change DNS Port

Edit `config.yml`:
```yaml
server:
  listen_address: ":5353"  # Use port 5353 instead of 53
```

Restart server.

### Enable Debug Logging Temporarily

```yaml
logging:
  level: "debug"
```

Restart server, then revert after debugging.

### Clear Cache

Restart the server:
```bash
sudo systemctl restart glory-hole
```

Cache is in-memory only and cleared on restart.

### Test if Domain is Blocked

```bash
# Query via Glory-Hole
dig @localhost doubleclick.net

# Check response
# - NXDOMAIN = blocked
# - IP address = allowed
```

### Force Blocklist Update

```bash
curl -X POST http://localhost:8080/api/blocklist/reload
```

### Export Query Logs

```bash
# SQLite export to CSV
sqlite3 -header -csv ./glory-hole.db \
  "SELECT * FROM query_logs WHERE timestamp > datetime('now', '-7 days')" \
  > queries-export.csv

# JSON export
sqlite3 ./glory-hole.db \
  "SELECT json_object('timestamp', timestamp, 'domain', domain) FROM query_logs" \
  > queries-export.json
```

### Rotate Log Files

If using file logging:

```yaml
logging:
  output: "file"
  file_path: "/var/log/glory-hole/glory-hole.log"
  max_size: 100     # MB
  max_backups: 3    # Old files to keep
  max_age: 7        # Days
```

Rotation happens automatically when size limit is reached.

### Update Glory-Hole Version

**Binary:**
```bash
# Download new version
wget https://github.com/erfianugrah/gloryhole/releases/latest/download/glory-hole-linux-amd64
chmod +x glory-hole-linux-amd64

# Stop server
sudo systemctl stop glory-hole

# Replace binary
sudo mv glory-hole-linux-amd64 /usr/local/bin/glory-hole

# Start server
sudo systemctl start glory-hole
```

**Docker:**
```bash
# Pull new image
docker pull erfianugrah/gloryhole:latest

# Recreate container
docker-compose down
docker-compose up -d
```

**Kubernetes:**
```bash
# Update image
kubectl set image deployment/glory-hole glory-hole=erfianugrah/gloryhole:latest

# Rollout restart
kubectl rollout restart deployment/glory-hole
```

## Next Steps

- [Troubleshooting Guide](troubleshooting.md) - Solve common issues
- [REST API Reference](../api/rest-api.md) - Full API documentation
- [Monitoring Setup](../deployment/monitoring.md) - Prometheus and Grafana
- [Configuration Guide](configuration.md) - Advanced configuration options
