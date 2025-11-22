# Monitoring Glory-Hole DNS Server

This guide covers setting up comprehensive monitoring for Glory-Hole DNS Server using Prometheus and Grafana.

## Table of Contents

- [Overview](#overview)
- [Quick Start](#quick-start)
- [Prometheus Setup](#prometheus-setup)
- [Grafana Setup](#grafana-setup)
- [Available Metrics](#available-metrics)
- [Dashboards](#dashboards)
- [Alert Rules](#alert-rules)
- [Query Examples](#query-examples)
- [Integration with External Systems](#integration-with-external-systems)
- [Troubleshooting](#troubleshooting)
- [Best Practices](#best-practices)

## Overview

Glory-Hole DNS Server exposes metrics in Prometheus format, allowing you to monitor:

- DNS query performance (QPS, latency, success rates)
- Cache efficiency (hit rate, size, evictions)
- Blocklist effectiveness (blocked queries, block rate)
- System resources (memory, CPU, goroutines)
- Rate limiting activity
- Storage performance

The monitoring stack consists of:

- **Prometheus**: Time-series database that scrapes and stores metrics
- **Grafana**: Visualization and dashboarding platform
- **Alertmanager** (optional): Alert routing and notification management

## Quick Start

### Using Docker Compose

The easiest way to get started is using the provided Docker Compose configuration:

```bash
# Start Glory-Hole with monitoring stack
docker-compose up -d

# Access the services
# Glory-Hole: http://localhost:8080
# Prometheus: http://localhost:9090
# Grafana: http://localhost:3000 (admin/admin)
```

### Manual Setup

If you're running Glory-Hole manually, ensure telemetry is enabled in your configuration:

```yaml
telemetry:
  enabled: true
  service_name: "glory-hole"
  service_version: "0.5.0"
  prometheus:
    enabled: true
    port: 9090
```

Then start Prometheus and Grafana pointing to Glory-Hole's metrics endpoint.

## Prometheus Setup

### Installation

#### Docker

```bash
docker run -d \
  --name prometheus \
  -p 9090:9090 \
  -v $(pwd)/deploy/prometheus/prometheus.yml:/etc/prometheus/prometheus.yml \
  -v $(pwd)/deploy/prometheus/alerts:/etc/prometheus/alerts \
  prom/prometheus:latest
```

#### Linux/macOS

```bash
# Download and extract
wget https://github.com/prometheus/prometheus/releases/latest/download/prometheus-linux-amd64.tar.gz
tar xvfz prometheus-*.tar.gz
cd prometheus-*

# Run with configuration
./prometheus --config.file=/path/to/glory-hole/deploy/prometheus/prometheus.yml
```

### Configuration

The Prometheus configuration is located at `deploy/prometheus/prometheus.yml`:

```yaml
scrape_configs:
  - job_name: 'glory-hole'
    scrape_interval: 10s
    static_configs:
      - targets: ['glory-hole:9090']  # Update hostname/port as needed
        labels:
          service: 'dns-server'
          instance: 'glory-hole-01'
```

**Key configuration options:**

- `scrape_interval`: How often to collect metrics (default: 10s for DNS, 15s for other services)
- `scrape_timeout`: Maximum time to wait for metrics (default: 10s)
- `targets`: List of Glory-Hole instances to monitor
- `labels`: Additional labels to attach to all metrics

### Enable Alert Rules

To enable alerting, uncomment the rule_files section in `prometheus.yml`:

```yaml
rule_files:
  - "alerts/glory-hole.yml"
```

### Verifying Prometheus Setup

1. Open Prometheus UI: http://localhost:9090
2. Go to Status > Targets
3. Verify glory-hole target is "UP"
4. Query a metric: `dns_queries_total`

## Grafana Setup

### Installation

#### Docker

```bash
docker run -d \
  --name grafana \
  -p 3000:3000 \
  -v $(pwd)/deploy/grafana/provisioning:/etc/grafana/provisioning \
  -v $(pwd)/deploy/grafana/dashboards:/var/lib/grafana/dashboards \
  grafana/grafana:latest
```

#### Linux/macOS

```bash
# Ubuntu/Debian
sudo apt-get install -y software-properties-common
sudo add-apt-repository "deb https://packages.grafana.com/oss/deb stable main"
sudo apt-get update
sudo apt-get install grafana

# Start service
sudo systemctl start grafana-server
```

### Accessing Grafana

1. Open http://localhost:3000
2. Login with default credentials:
   - Username: `admin`
   - Password: `admin`
3. Change password when prompted

### Configure Prometheus Data Source

The data source is automatically provisioned from `deploy/grafana/provisioning/datasources/prometheus.yml`.

To manually add:

1. Go to Configuration > Data Sources
2. Click "Add data source"
3. Select "Prometheus"
4. Set URL: `http://prometheus:9090` (or `http://localhost:9090` if running locally)
5. Click "Save & Test"

### Import Dashboards

#### Automatic (Provisioned)

Dashboards are automatically loaded from `deploy/grafana/dashboards/` when using the provided configuration.

#### Manual Import

1. Go to Dashboards > Import
2. Upload JSON file or paste JSON content
3. Select Prometheus data source
4. Click "Import"

Available dashboards:

- **glory-hole-overview.json**: Main dashboard with key metrics
- **glory-hole-performance.json**: Detailed performance analysis

## Available Metrics

Glory-Hole DNS Server exposes the following metrics:

### DNS Query Metrics

| Metric | Type | Description | Labels |
|--------|------|-------------|--------|
| `dns_queries_total` | Counter | Total number of DNS queries received | - |
| `dns_queries_by_type` | Counter | DNS queries by query type | `type` (A, AAAA, CNAME, etc.) |
| `dns_queries_blocked` | Counter | Number of blocked DNS queries | - |
| `dns_queries_forwarded` | Counter | Number of forwarded DNS queries | - |
| `dns_query_duration` | Histogram | DNS query processing duration in milliseconds | - |

**Example queries:**

```promql
# Queries per second
rate(dns_queries_total[5m])

# Block rate percentage
100 * (rate(dns_queries_blocked[5m]) / rate(dns_queries_total[5m]))

# P95 latency
histogram_quantile(0.95, sum(rate(dns_query_duration_bucket[5m])) by (le))
```

### Cache Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `dns_cache_hits` | Counter | Number of DNS cache hits |
| `dns_cache_misses` | Counter | Number of DNS cache misses |
| `cache_size` | Gauge | Number of entries in DNS cache |

**Example queries:**

```promql
# Cache hit rate
100 * (rate(dns_cache_hits[5m]) / (rate(dns_cache_hits[5m]) + rate(dns_cache_misses[5m])))

# Cache entries
cache_size
```

### Blocklist Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `blocklist_size` | Gauge | Number of domains in blocklist |

**Example queries:**

```promql
# Blocklist size
blocklist_size

# Blocked queries per minute
rate(dns_queries_blocked[1m]) * 60
```

### Rate Limiting Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `rate_limit_violations` | Counter | Number of rate limit violations |
| `rate_limit_dropped` | Counter | Number of dropped requests due to rate limiting |

**Example queries:**

```promql
# Rate limit violations per second
rate(rate_limit_violations[5m])

# Dropped requests per second
rate(rate_limit_dropped[5m])
```

### System Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `clients_active` | Gauge | Number of active clients |
| `go_goroutines` | Gauge | Number of goroutines |
| `go_memstats_alloc_bytes` | Gauge | Bytes allocated and in use |
| `go_memstats_sys_bytes` | Gauge | Bytes obtained from system |
| `go_memstats_heap_inuse_bytes` | Gauge | Bytes in use by heap |
| `go_memstats_heap_alloc_bytes` | Gauge | Bytes allocated to heap |
| `go_memstats_stack_inuse_bytes` | Gauge | Bytes in use by stack |
| `go_gc_duration_seconds` | Summary | Garbage collection duration |

**Example queries:**

```promql
# Memory usage percentage
100 * (go_memstats_alloc_bytes / go_memstats_sys_bytes)

# Goroutine count
go_goroutines

# GC frequency
rate(go_gc_duration_seconds_count[5m])
```

### Metric Naming Conventions

Glory-Hole follows Prometheus naming best practices:

- Metric names use snake_case
- Metric names describe what is measured (not how)
- Suffixes indicate unit or type:
  - `_total`: Counters that accumulate
  - `_bytes`: Memory/size measurements
  - `_seconds`: Time measurements
  - `_duration`: Histogram of time measurements
- Base unit is used (seconds, not milliseconds in name)

## Dashboards

### Glory-Hole Overview Dashboard

The main dashboard (`glory-hole-overview.json`) provides:

- **DNS Query Rate**: Total, blocked, and forwarded queries per second
- **DNS Query Latency**: P95 latency gauge and percentile trends
- **Cache Performance**: Hit rate gauge and hits/misses over time
- **System State**: Cache size, blocklist size, active clients
- **Rate Limiting**: Violations and dropped requests
- **Block Rate**: Percentage of blocked queries
- **Query Types**: Distribution of A, AAAA, CNAME, etc.
- **Memory Usage**: Allocated memory, heap, and stack usage
- **Goroutines**: Goroutine count over time
- **Garbage Collection**: GC frequency

**When to use:**
- Daily operations monitoring
- Quick health checks
- Identifying immediate issues

### Glory-Hole Performance Dashboard

The performance dashboard (`glory-hole-performance.json`) provides:

- **Latency Percentiles**: P50, P75, P90, P95, P99, P99.9 over time
- **Latency Heatmap**: Distribution visualization
- **Average vs P95**: Comparison of average and tail latency
- **Cache Efficiency**: Hit rate trends and cache size
- **Memory Details**: Allocated, system, heap, and in-use memory
- **GC Performance**: Duration and frequency analysis
- **Goroutine Tracking**: Goroutine count with leak detection
- **Pipeline Breakdown**: Query processing stages

**When to use:**
- Performance tuning
- Capacity planning
- Investigating latency issues
- Memory leak detection

### Dashboard Variables

Both dashboards support template variables for filtering:

- `$DS_PROMETHEUS`: Prometheus datasource
- `$job`: Job name (default: glory-hole)
- `$instance`: Instance name (supports multiple instances)

## Alert Rules

Alert rules are defined in `deploy/prometheus/alerts/glory-hole.yml`.

### Critical Alerts

These require immediate attention:

| Alert | Condition | Threshold | Duration |
|-------|-----------|-----------|----------|
| DNSServerDown | Server not responding | N/A | 1m |
| HighDNSErrorRate | Query failures | >5% | 5m |
| HighDNSLatency | P95 latency | >100ms | 5m |
| MemoryUsageCritical | Memory usage | >90% | 5m |
| GoroutineLeak | Goroutine count | >10000 | 10m |

### Warning Alerts

These need attention soon:

| Alert | Condition | Threshold | Duration |
|-------|-----------|-----------|----------|
| DNSLatencyWarning | P95 latency elevated | >50ms | 10m |
| LowCacheHitRate | Cache efficiency low | <50% | 15m |
| HighBlockRate | Blocking aggressively | >30% | 15m |
| BlocklistEmpty | No blocklist loaded | 0 domains | 5m |
| HighRateLimitViolations | Rate limit hits | >10/s | 10m |
| MemoryUsageHigh | Memory usage elevated | >80% | 10m |
| HighGCFrequency | Frequent GC cycles | >5/s | 10m |
| CacheSizeNearLimit | Cache near capacity | >9000 entries | 10m |

### Info Alerts

These are informational:

| Alert | Condition | Threshold | Duration |
|-------|-----------|-----------|----------|
| DNSQueryRateChange | Query rate changed | ±50% vs 1h ago | 10m |
| BlocklistUpdateNeeded | Stale blocklist | >7 days old | 1h |
| LowQueryRate | Unusually quiet | <0.1/s | 15m |
| HighForwardedQueryRate | Cache not helping | >90% forwarded | 15m |

### Configuring Alertmanager

To receive alert notifications, configure Alertmanager:

```yaml
# alertmanager.yml
global:
  resolve_timeout: 5m

route:
  group_by: ['alertname', 'cluster', 'service']
  group_wait: 10s
  group_interval: 10s
  repeat_interval: 12h
  receiver: 'default'

receivers:
  - name: 'default'
    email_configs:
      - to: 'alerts@example.com'
        from: 'alertmanager@example.com'
        smarthost: 'smtp.example.com:587'
        auth_username: 'alertmanager'
        auth_password: 'password'
```

## Query Examples

### Performance Analysis

```promql
# Average query latency (ms)
sum(rate(dns_query_duration_sum[5m])) / sum(rate(dns_query_duration_count[5m]))

# Query throughput (queries per second)
sum(rate(dns_queries_total[5m]))

# Cache effectiveness
100 * sum(rate(dns_cache_hits[5m])) / sum(rate(dns_queries_total[5m]))
```

### Capacity Planning

```promql
# Predict when cache will be full (assuming linear growth)
predict_linear(cache_size[1h], 3600)

# Memory growth rate (bytes per second)
deriv(go_memstats_alloc_bytes[5m])

# Query rate trend (7-day average)
avg_over_time(rate(dns_queries_total[5m])[7d:1h])
```

### Troubleshooting

```promql
# Queries taking longer than 100ms
sum(rate(dns_query_duration_bucket{le="100"}[5m]))

# Blocked domains in last hour
increase(dns_queries_blocked[1h])

# Rate limit violations by client (if client label available)
topk(10, sum(rate(rate_limit_violations[5m])) by (client))
```

### Resource Utilization

```promql
# Memory utilization percentage
100 * (go_memstats_alloc_bytes / go_memstats_sys_bytes)

# Heap utilization
100 * (go_memstats_heap_inuse_bytes / go_memstats_heap_sys_bytes)

# GC pause time (99th percentile)
histogram_quantile(0.99, rate(go_gc_duration_seconds_bucket[5m]))
```

## Integration with External Systems

### PagerDuty

Configure Alertmanager to send critical alerts to PagerDuty:

```yaml
receivers:
  - name: 'pagerduty'
    pagerduty_configs:
      - service_key: 'YOUR_PAGERDUTY_SERVICE_KEY'
        severity: '{{ .GroupLabels.severity }}'
        description: '{{ .CommonAnnotations.summary }}'
        details:
          firing: '{{ .Alerts.Firing | len }}'
          resolved: '{{ .Alerts.Resolved | len }}'
```

### Slack

Send alerts to Slack channels:

```yaml
receivers:
  - name: 'slack'
    slack_configs:
      - api_url: 'https://hooks.slack.com/services/YOUR/SLACK/WEBHOOK'
        channel: '#dns-alerts'
        title: 'Glory-Hole DNS Alert'
        text: '{{ range .Alerts }}{{ .Annotations.description }}{{ end }}'
```

### Email

Configure email notifications:

```yaml
receivers:
  - name: 'email'
    email_configs:
      - to: 'dns-team@example.com'
        from: 'monitoring@example.com'
        smarthost: 'smtp.gmail.com:587'
        auth_username: 'monitoring@example.com'
        auth_password: 'app-password'
        headers:
          Subject: 'Glory-Hole DNS Alert: {{ .GroupLabels.alertname }}'
```

### Webhook

Send alerts to custom webhook:

```yaml
receivers:
  - name: 'webhook'
    webhook_configs:
      - url: 'http://your-webhook-service/alerts'
        send_resolved: true
```

## Troubleshooting

### Metrics Not Appearing

**Problem**: Prometheus can't scrape metrics from Glory-Hole.

**Solutions**:

1. Verify telemetry is enabled:
   ```yaml
   telemetry:
     enabled: true
     prometheus:
       enabled: true
       port: 9090
   ```

2. Check if metrics endpoint is accessible:
   ```bash
   curl http://localhost:9090/metrics
   ```

3. Verify Prometheus target configuration:
   - Open http://localhost:9090/targets
   - Check if glory-hole target shows as "UP"

4. Check network connectivity:
   ```bash
   # From Prometheus container/host
   telnet glory-hole 9090
   ```

### Dashboard Shows No Data

**Problem**: Grafana dashboard panels are empty.

**Solutions**:

1. Verify data source connection:
   - Go to Configuration > Data Sources
   - Click "Test" on Prometheus data source

2. Check time range:
   - Dashboards default to "Last 1 hour"
   - Adjust time range if Glory-Hole hasn't been running long

3. Verify metrics exist:
   - Go to Explore in Grafana
   - Query: `dns_queries_total`
   - Should see data points

4. Check metric names:
   - Metric names in dashboard queries must match exactly
   - Use Prometheus UI to verify metric names

### High Memory Usage

**Problem**: Glory-Hole consuming excessive memory.

**Diagnostic queries**:

```promql
# Current memory usage
go_memstats_alloc_bytes

# Memory allocation rate
rate(go_memstats_alloc_bytes_total[5m])

# Cache size
cache_size

# Goroutine count (potential leak)
go_goroutines
```

**Solutions**:

1. Reduce cache size in configuration
2. Check for goroutine leaks (alert: GoroutineLeak)
3. Reduce blocklist size if very large
4. Monitor GC frequency and tune GOGC if needed

### Alerts Not Firing

**Problem**: Expected alerts not triggering.

**Solutions**:

1. Verify alert rules are loaded:
   - Open http://localhost:9090/rules
   - Check if glory-hole rules appear

2. Check alert evaluation:
   - Click on alert name
   - View current state and history

3. Verify Alertmanager connection:
   - Open http://localhost:9090/config
   - Check alertmanager targets

4. Test alert expression manually:
   - Copy alert expression to Prometheus query
   - Verify it returns data

### Slow Dashboard Loading

**Problem**: Grafana dashboards load slowly.

**Solutions**:

1. Reduce query time range (e.g., 1h instead of 24h)
2. Increase scrape_interval in Prometheus
3. Use recording rules for expensive queries:
   ```yaml
   groups:
     - name: glory_hole_recordings
       interval: 30s
       rules:
         - record: job:dns_queries_total:rate5m
           expr: sum(rate(dns_queries_total[5m]))
   ```

4. Optimize dashboard queries (use simpler aggregations)

## Best Practices

### Metric Collection

1. **Scrape Interval**:
   - Use 10s for DNS metrics (fast-changing)
   - Use 30s for system metrics
   - Use 60s for storage metrics

2. **Retention**:
   - Keep 15 days of data for troubleshooting
   - Use recording rules for long-term trends
   - Archive to long-term storage if needed

3. **Cardinality**:
   - Avoid high-cardinality labels (client IPs, domains)
   - Use label aggregation in queries
   - Consider using recording rules for common queries

### Dashboard Design

1. **Overview First**: Start with high-level metrics
2. **Drill-Down**: Link to detailed dashboards
3. **Consistent Colors**: Use same colors for same metrics
4. **Time Alignment**: Keep time ranges synchronized
5. **Annotations**: Mark deployments, incidents, maintenance

### Alert Configuration

1. **Severity Levels**:
   - Critical: Requires immediate attention (wake up on-call)
   - Warning: Needs attention during business hours
   - Info: FYI, no action required

2. **Alert Fatigue**:
   - Set appropriate thresholds
   - Use `for` duration to avoid flapping
   - Group related alerts
   - Send critical alerts to pager, warnings to chat

3. **Runbooks**:
   - Document each alert's meaning
   - Include troubleshooting steps
   - Link to relevant dashboards
   - Specify escalation path

### Security

1. **Authentication**:
   - Enable authentication in Grafana
   - Use strong passwords
   - Enable HTTPS for web UIs

2. **Network Security**:
   - Restrict Prometheus/Grafana to trusted networks
   - Use firewall rules
   - Consider VPN for remote access

3. **Data Retention**:
   - Query logs may contain sensitive domains
   - Set appropriate retention policies
   - Consider data anonymization

### Performance

1. **Prometheus**:
   - Monitor Prometheus itself
   - Watch TSDB size and growth
   - Enable remote write for long-term storage

2. **Grafana**:
   - Use caching for expensive queries
   - Limit concurrent queries
   - Use query result caching

3. **Glory-Hole**:
   - Metrics collection is low overhead (<1% CPU)
   - Histogram buckets are pre-configured optimally
   - No code changes needed for new metrics

### Backup and Recovery

1. **Prometheus Data**:
   ```bash
   # Backup Prometheus data directory
   tar czf prometheus-backup-$(date +%Y%m%d).tar.gz /path/to/prometheus/data
   ```

2. **Grafana Dashboards**:
   ```bash
   # Export all dashboards
   for dash in $(curl -s "http://admin:admin@localhost:3000/api/search?query=&" | jq -r '.[] | .uid'); do
     curl -s "http://admin:admin@localhost:3000/api/dashboards/uid/$dash" | jq . > "dashboard-${dash}.json"
   done
   ```

3. **Alert Rules**:
   - Version control alert rules in Git
   - Test changes in non-production
   - Document all changes

## Additional Resources

- [Prometheus Documentation](https://prometheus.io/docs/)
- [Grafana Documentation](https://grafana.com/docs/)
- [PromQL Tutorial](https://prometheus.io/docs/prometheus/latest/querying/basics/)
- [OpenTelemetry Documentation](https://opentelemetry.io/docs/)
- [Glory-Hole Architecture](../architecture/overview.md)
- [Glory-Hole Configuration Guide](../guide/configuration.md)

## Support

For issues or questions about monitoring:

1. Check [Troubleshooting Guide](../guide/troubleshooting.md)
2. Review [GitHub Issues](https://github.com/yourusername/glory-hole/issues)
3. Join our community chat
4. Open a new issue with:
   - Prometheus version
   - Grafana version
   - Glory-Hole version
   - Relevant configuration files
   - Screenshot of issue
