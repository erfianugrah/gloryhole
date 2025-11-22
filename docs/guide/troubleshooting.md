# Troubleshooting Guide

This guide helps you diagnose and resolve common issues with Glory-Hole DNS Server.

## Table of Contents

- [Quick Diagnostics](#quick-diagnostics)
- [DNS Not Resolving](#dns-not-resolving)
- [Performance Issues](#performance-issues)
- [Memory/CPU Problems](#memorycpu-problems)
- [Policy Not Working](#policy-not-working)
- [Blocklist Issues](#blocklist-issues)
- [Database Problems](#database-problems)
- [Web UI Issues](#web-ui-issues)
- [Logs and Debugging](#logs-and-debugging)

## Quick Diagnostics

### Health Check

```bash
# Check server health
curl http://localhost:8080/api/health

# Expected: {"status":"ok","uptime":"...","version":"..."}
```

### Process Check

```bash
# Check if process is running
ps aux | grep glory-hole

# Check ports
netstat -tlnp | grep glory-hole
# or
lsof -i :53
lsof -i :8080
```

### Test DNS Query

```bash
# Basic query
dig @localhost google.com

# With timing
dig @localhost google.com +stats

# TCP query
dig @localhost google.com +tcp
```

## DNS Not Resolving

### Problem: "connection refused" or "no response"

**Symptoms:**
```bash
$ dig @localhost google.com
;; communications error to 127.0.0.1#53: connection refused
```

**Possible Causes:**

1. **Server not running**
   ```bash
   # Check if running
   systemctl status glory-hole
   # or
   docker ps | grep glory-hole
   ```

2. **Wrong address/port**
   ```bash
   # Check config
   grep listen_address config.yml
   
   # Test correct address
   dig @127.0.0.1 -p 53 google.com
   ```

3. **Firewall blocking**
   ```bash
   # Check firewall (Ubuntu/Debian)
   sudo ufw status
   sudo ufw allow 53/udp
   sudo ufw allow 53/tcp
   
   # Check firewall (CentOS/RHEL)
   sudo firewall-cmd --list-ports
   sudo firewall-cmd --add-port=53/udp --permanent
   sudo firewall-cmd --add-port=53/tcp --permanent
   sudo firewall-cmd --reload
   ```

4. **Permission denied on port 53**
   ```bash
   # Error in logs: "bind: permission denied"
   
   # Solution 1: Run as root
   sudo glory-hole -config config.yml
   
   # Solution 2: Grant capability
   sudo setcap 'cap_net_bind_service=+ep' /usr/local/bin/glory-hole
   
   # Solution 3: Use non-privileged port
   # Edit config.yml: listen_address: ":5353"
   ```

### Problem: Queries timing out

**Symptoms:**
```bash
$ dig @localhost google.com
;; connection timed out; no servers could be reached
```

**Solutions:**

1. **Check upstream DNS servers**
   ```bash
   # Test upstream connectivity
   dig @1.1.1.1 google.com
   dig @8.8.8.8 google.com
   
   # If fails, check network
   ping 1.1.1.1
   ```

2. **Verify configuration**
   ```yaml
   upstream_dns_servers:
     - "1.1.1.1:53"  # Must include :53
     - "8.8.8.8:53"
   ```

3. **Check logs for errors**
   ```bash
   journalctl -u glory-hole | grep -i "upstream\|forward\|error"
   ```

### Problem: NXDOMAIN for all queries

**Symptoms:**
- All domains return NXDOMAIN (domain doesn't exist)
- Even known-good domains like google.com fail

**Solutions:**

1. **Upstream DNS not configured**
   ```yaml
   # Must have at least one upstream
   upstream_dns_servers:
     - "1.1.1.1:53"
   ```

2. **Blocklist too aggressive**
   ```bash
   # Temporarily disable blocklists
   # Comment out in config.yml:
   # blocklists: []
   ```

3. **Policy blocking everything**
   ```yaml
   # Check for overly broad policy rules
   policy:
     enabled: false  # Temporarily disable
   ```

## Performance Issues

### Problem: Slow DNS responses

**Symptoms:**
- Queries take > 100ms
- Web pages load slowly
- `dig` shows high query times

**Diagnostics:**

```bash
# Measure query time
for i in {1..10}; do
  dig @localhost google.com | grep "Query time"
done

# Check stats
curl http://localhost:8080/api/stats

# Check cache hit rate (should be > 50%)
```

**Solutions:**

1. **Enable/increase cache**
   ```yaml
   cache:
     enabled: true
     max_entries: 50000  # Increase if you have memory
   ```

2. **Use faster upstream DNS**
   ```yaml
   upstream_dns_servers:
     - "1.1.1.1:53"      # Usually fastest
     - "8.8.8.8:53"
   ```

3. **Reduce blocklist count**
   ```yaml
   # Use fewer blocklists
   blocklists:
     - "https://raw.githubusercontent.com/StevenBlack/hosts/master/hosts"
     # Remove others temporarily
   ```

4. **Disable query logging**
   ```yaml
   database:
     enabled: false  # Temporary for testing
   ```

### Problem: High latency spikes

**Symptoms:**
- Occasional very slow queries (> 500ms)
- Most queries are fast

**Solutions:**

1. **Enable persistent connections**
   - Already enabled by default in forwarder

2. **Check network latency**
   ```bash
   ping -c 10 1.1.1.1
   mtr 1.1.1.1
   ```

3. **Increase buffer sizes**
   ```yaml
   database:
     buffer_size: 5000
     flush_interval: "10s"
   ```

## Memory/CPU Problems

### Problem: High memory usage

**Symptoms:**
```bash
$ ps aux | grep glory-hole
USER  PID  %CPU  %MEM    VSZ   RSS
user  123   5.0   15.0  2GB   1.5GB
```

**Normal Memory Usage:**
- Base: ~50MB
- Cache (10K entries): ~10MB
- Blocklists (100K domains): ~20MB
- Database buffer: ~1-5MB
- **Total expected: 80-100MB**

**Solutions:**

1. **Reduce cache size**
   ```yaml
   cache:
     max_entries: 5000  # Reduce from 10000
   ```

2. **Reduce blocklists**
   ```yaml
   blocklists:
     # Use fewer sources
     - "https://raw.githubusercontent.com/StevenBlack/hosts/master/hosts"
   ```

3. **Reduce database buffer**
   ```yaml
   database:
     buffer_size: 500  # Reduce from 1000
   ```

4. **Check for memory leaks**
   ```bash
   # Monitor over time
   while true; do
     ps aux | grep glory-hole | awk '{print $6}'
     sleep 60
   done
   ```

### Problem: High CPU usage

**Symptoms:**
- Constant > 50% CPU
- Server feels slow

**Solutions:**

1. **Check query load**
   ```bash
   # Check queries per second
   curl http://localhost:8080/api/stats
   ```

2. **Disable policy engine temporarily**
   ```yaml
   policy:
     enabled: false
   ```

3. **Simplify policy expressions**
   ```yaml
   # Bad: Complex regex in hot path
   logic: 'DomainRegex(Domain, "^(.*\\.)?facebook\\.com$")'
   
   # Good: Simple string match
   logic: 'DomainMatches(Domain, "facebook")'
   ```

4. **Reduce log verbosity**
   ```yaml
   logging:
     level: "warn"  # Instead of "debug"
   ```

## Policy Not Working

### Problem: Policy rule not matching

**Symptoms:**
- Policy should block/allow but doesn't
- Queries bypass policy rules

**Diagnostics:**

```bash
# Enable debug logging
# Edit config.yml:
logging:
  level: "debug"

# Restart and check logs
journalctl -u glory-hole -f | grep -i policy
```

**Solutions:**

1. **Check rule is enabled**
   ```yaml
   policy:
     enabled: true
     rules:
       - name: "My rule"
         enabled: true  # Must be true
   ```

2. **Verify expression syntax**
   ```yaml
   # Wrong: Missing quotes
   logic: Hour >= 22 && Domain == facebook.com
   
   # Correct: Quoted strings
   logic: 'Hour >= 22 && Domain == "facebook.com"'
   
   # Better: Use helper function
   logic: 'Hour >= 22 && DomainMatches(Domain, "facebook")'
   ```

3. **Check rule order**
   ```yaml
   # Rules are evaluated in order!
   # First match wins
   rules:
     # This catches everything first
     - name: "Allow all"
       logic: "true"
       action: "ALLOW"
       enabled: true
     
     # This never runs!
     - name: "Block facebook"
       logic: 'DomainMatches(Domain, "facebook")'
       action: "BLOCK"
       enabled: true
   ```

4. **Test expression**
   ```bash
   # Add test policy via API
   curl -X POST http://localhost:8080/api/policies \
     -H "Content-Type: application/json" \
     -d '{
       "name": "Test rule",
       "logic": "true",
       "action": "BLOCK",
       "enabled": true
     }'
   
   # If this blocks everything, logic works
   # Now refine the expression
   ```

### Problem: Policy compilation error

**Symptoms:**
```
ERROR Failed to add policy rule name="Block social" error="failed to compile rule"
```

**Solutions:**

1. **Fix syntax errors**
   ```yaml
   # Wrong: Missing operator
   logic: 'Hour 22'
   
   # Correct
   logic: 'Hour >= 22'
   ```

2. **Use correct field names**
   ```yaml
   # Wrong: Invalid field
   logic: 'Time > 22'
   
   # Correct: Use Hour
   logic: 'Hour >= 22'
   ```

3. **Valid context fields:**
   - `Domain`, `ClientIP`, `QueryType`
   - `Hour`, `Minute`, `Day`, `Month`, `Weekday`
   - `Time` (time.Time object)

## Blocklist Issues

### Problem: Blocklist not loading

**Symptoms:**
```
INFO Blocklist manager started domains=0
```

**Solutions:**

1. **Check internet connectivity**
   ```bash
   curl https://raw.githubusercontent.com/StevenBlack/hosts/master/hosts
   ```

2. **Verify URLs are correct**
   ```yaml
   blocklists:
     - "https://raw.githubusercontent.com/StevenBlack/hosts/master/hosts"
     # Not: "http://" (some require HTTPS)
   ```

3. **Check logs for errors**
   ```bash
   journalctl -u glory-hole | grep -i "blocklist\|error"
   ```

4. **Test manual reload**
   ```bash
   curl -X POST http://localhost:8080/api/blocklist/reload
   ```

### Problem: Domain should be blocked but isn't

**Solutions:**

1. **Check whitelist**
   ```yaml
   whitelist:
     # Remove if accidentally whitelisted
     # - "doubleclick.net"
   ```

2. **Check policy rules**
   ```yaml
   # ALLOW rules bypass blocklist
   policy:
     rules:
       - name: "Allow everything"
         logic: "true"
         action: "ALLOW"  # This bypasses blocklist!
   ```

3. **Verify domain is in blocklist**
   ```bash
   # Check if domain is blocked
   curl http://localhost:8080/api/stats
   
   # Test query
   dig @localhost doubleclick.net
   # Should return NXDOMAIN if blocked
   ```

4. **Check blocklist format**
   - Some lists use different formats
   - Glory-Hole supports: hosts file format, domain-only lists

## Database Problems

### Problem: Database lock errors

**Symptoms:**
```
ERROR Failed to write query log error="database is locked"
```

**Solutions:**

1. **Enable WAL mode**
   ```yaml
   database:
     sqlite:
       wal_mode: true  # Write-Ahead Logging
   ```

2. **Increase busy timeout**
   ```yaml
   database:
     sqlite:
       busy_timeout: 10000  # 10 seconds
   ```

3. **Check disk I/O**
   ```bash
   # Check disk usage
   df -h
   
   # Check I/O wait
   iostat -x 1
   ```

### Problem: Database growing too large

**Symptoms:**
```bash
$ ls -lh glory-hole.db
-rw-r--r-- 1 user user 5.0G glory-hole.db
```

**Solutions:**

1. **Reduce retention period**
   ```yaml
   database:
     retention_days: 3  # Instead of 7 or 30
   ```

2. **Manually clean old data**
   ```bash
   sqlite3 glory-hole.db "DELETE FROM query_logs WHERE timestamp < datetime('now', '-7 days');"
   sqlite3 glory-hole.db "VACUUM;"
   ```

3. **Disable query logging**
   ```yaml
   database:
     enabled: false
   ```

## Web UI Issues

### Problem: Web UI not accessible

**Symptoms:**
- `http://localhost:8080` doesn't load
- Connection refused

**Solutions:**

1. **Check API server is running**
   ```bash
   curl http://localhost:8080/api/health
   ```

2. **Verify port configuration**
   ```yaml
   server:
     web_ui_address: ":8080"
   ```

3. **Check firewall**
   ```bash
   sudo ufw allow 8080/tcp
   ```

4. **Check if port is in use**
   ```bash
   lsof -i :8080
   # If another process is using it, change port
   ```

### Problem: Web UI shows no data

**Symptoms:**
- Dashboard shows 0 queries
- Query log is empty

**Solutions:**

1. **Check database is enabled**
   ```yaml
   database:
     enabled: true
   ```

2. **Verify queries are being made**
   ```bash
   dig @localhost google.com
   ```

3. **Check API endpoints**
   ```bash
   curl http://localhost:8080/api/stats
   curl http://localhost:8080/api/queries
   ```

## Logs and Debugging

### Enable Debug Logging

```yaml
logging:
  level: "debug"
  add_source: true  # Shows file:line numbers
```

### Useful Log Patterns

**DNS query errors:**
```bash
journalctl -u glory-hole | grep -i "query.*error"
```

**Blocklist issues:**
```bash
journalctl -u glory-hole | grep -i "blocklist"
```

**Policy evaluation:**
```bash
journalctl -u glory-hole | grep -i "policy"
```

**Database errors:**
```bash
journalctl -u glory-hole | grep -i "database\|storage\|sqlite"
```

### Capture Packet Traces

```bash
# Capture DNS traffic
sudo tcpdump -i any -n port 53

# Save to file
sudo tcpdump -i any -n port 53 -w dns-capture.pcap

# Analyze with Wireshark
wireshark dns-capture.pcap
```

### Enable Prometheus Metrics

```yaml
telemetry:
  enabled: true
  prometheus_enabled: true
  prometheus_port: 9090
```

Access metrics: `http://localhost:9090/metrics`

### Test Network Connectivity

```bash
# Test upstream DNS
dig @1.1.1.1 google.com

# Test Glory-Hole
dig @localhost google.com

# Compare response times
dig @1.1.1.1 google.com | grep "Query time"
dig @localhost google.com | grep "Query time"
```

## Getting Help

If you can't resolve the issue:

1. **Collect diagnostic information:**
   ```bash
   # Version
   glory-hole --version
   
   # Config (sanitized)
   cat config.yml
   
   # Logs (last 100 lines)
   journalctl -u glory-hole -n 100
   
   # System info
   uname -a
   cat /etc/os-release
   ```

2. **Enable debug logging and reproduce:**
   ```yaml
   logging:
     level: "debug"
   ```

3. **Open GitHub issue:**
   - https://github.com/erfianugrah/gloryhole/issues
   - Include version, logs, config
   - Describe steps to reproduce

4. **Community support:**
   - Discord: https://discord.gg/glory-hole
   - Reddit: r/gloryhole-dns
