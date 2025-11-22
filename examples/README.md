# Glory-Hole DNS Configuration Examples

This directory contains ready-to-use configuration examples for common scenarios.

## Quick Start

1. Choose a configuration that matches your use case
2. Copy it to your project root as `config.yml`
3. Edit the values to match your network setup
4. Start the DNS server

```bash
cp examples/home-network.yml config.yml
# Edit config.yml with your settings
./glory-hole
```

## Available Configurations

### 🏠 [home-network.yml](./home-network.yml)

**Best for:** Home networks, family use

**Features:**
- Ad and tracker blocking with comprehensive blocklists
- Local device resolution (nas.home, printer.home, etc.)
- Parental controls with time-based restrictions
- Social media blocking during school hours
- Bedtime internet cutoff (optional)
- Privacy-focused DNS providers (Cloudflare, Quad9)

**Resource Usage:** Medium
**Complexity:** Moderate

---

### 🏢 [small-office.yml](./small-office.yml)

**Best for:** Small businesses, offices with 5-50 employees

**Features:**
- Work hours productivity enforcement (blocks social media 9 AM - 5 PM)
- Security-focused filtering (malware, phishing protection)
- Streaming service blocking during business hours
- Internal service resolution (intranet.company.local)
- Comprehensive logging for compliance
- Admin network bypass rules
- 30-day query retention

**Resource Usage:** High
**Complexity:** Advanced

---

###  [advanced-filtering.yml](./advanced-filtering.yml)

**Best for:** Power users, complex network setups, learning the Policy Engine

**Features:**
- Showcase of all Policy Engine capabilities
- Complex time-based rules (lunch breaks, after hours)
- IP-based access control (guest, admin, kids networks)
- Multi-condition logic examples
- Domain pattern matching (TLDs, subdomains)
- Detailed comments explaining each rule
- Debug logging enabled

**Resource Usage:** Medium
**Complexity:** Expert

---

### 🪶 [minimal.yml](./minimal.yml)

**Best for:** Raspberry Pi, low-power devices, constrained environments

**Features:**
- Basic ad blocking only
- Minimal memory footprint (~50MB RAM)
- Low CPU usage
- No database, no telemetry
- Single upstream DNS
- Small cache (1000 entries)
- TCP disabled to save resources

**Resource Usage:** Very Low
**Complexity:** Simple

---

## Configuration Comparison

| Feature | Home Network | Small Office | Advanced | Minimal |
|---------|-------------|--------------|----------|---------|
| **RAM Usage** | ~100MB | ~200MB | ~150MB | ~50MB |
| **Ad Blocking** |  Comprehensive |  Security-focused |  Basic |  Basic |
| **Policy Engine** |  Parental controls |  Work hours |  Advanced |  |
| **Query Logging** |  7 days |  30 days |  7 days |  |
| **Telemetry** |  Basic |  Full (Prometheus + Jaeger) |  Basic |  |
| **Best For** | Families | Businesses | Power users | Raspberry Pi |

## Policy Engine Quick Reference

All configurations (except `minimal.yml`) support the Policy Engine for advanced filtering.

### Available Functions

```yaml
# Check if domain contains substring
DomainMatches(Domain, "facebook")

# Check if domain ends with suffix
DomainEndsWith(Domain, ".com")

# Check if domain starts with prefix
DomainStartsWith(Domain, "www.")

# Check if IP is in CIDR range
IPInCIDR(ClientIP, "192.168.1.0/24")
```

### Available Variables

- `Domain` - The queried domain (e.g., "www.example.com")
- `ClientIP` - The IP of the DNS client
- `QueryType` - The DNS query type (e.g., "A", "AAAA")
- `Hour` - Current hour (0-23)
- `Weekday` - true if Monday-Friday, false otherwise

### Example Rules

```yaml
# Block social media during work hours
logic: 'Weekday && Hour >= 9 && Hour < 17 && DomainMatches(Domain, "facebook")'
action: "block"

# Allow admin network full access
logic: 'IPInCIDR(ClientIP, "10.0.0.0/28")'
action: "allow"

# Block after midnight
logic: 'Hour >= 23 || Hour < 6'
action: "block"
```

## Customization Tips

### Adding More Blocklists

```yaml
blocklists:
  # Ad blocking
  - "https://adaway.org/hosts.txt"

  # Malware
  - "https://urlhaus.abuse.ch/downloads/hostfile/"

  # Trackers
  - "https://v.firebog.net/hosts/Easyprivacy.txt"

  # Social media (if desired)
  - "https://raw.githubusercontent.com/StevenBlack/hosts/master/alternates/fakenews-gambling-porn-social/hosts"
```

### Network-Specific Settings

Replace IP addresses and CIDR ranges with your network:

```yaml
# For home networks typically
listen_address: "192.168.1.X:53"

# For office networks typically
listen_address: "10.0.0.X:53"

# Guest network CIDR
IPInCIDR(ClientIP, "192.168.2.0/24")  # Adjust to your guest network
```

### Testing Your Configuration

```bash
# Validate configuration syntax
./glory-hole --config config.yml --validate

# Test DNS resolution
dig @localhost example.com

# Test blocked domain
dig @localhost doubleclick.net

# Check if policy rules work
dig @localhost facebook.com  # Should be blocked during work hours
```

## Performance Tuning

### For High-Traffic Networks (500+ queries/sec)

```yaml
cache:
  max_entries: 50000  # Increase cache size
  min_ttl: "300s"     # Cache longer

database:
  buffer_size: 5000   # Larger buffer
  flush_interval: "10s"  # Less frequent writes
```

### For Low-Memory Devices (< 512MB RAM)

```yaml
cache:
  max_entries: 1000   # Small cache

database:
  enabled: false      # Disable logging

telemetry:
  enabled: false      # No metrics
```

## Troubleshooting

### DNS Not Resolving

1. Check if server is running: `sudo systemctl status glory-hole`
2. Test with dig: `dig @localhost example.com`
3. Check logs: `tail -f /var/log/glory-hole/dns-server.log`
4. Verify firewall: `sudo ufw allow 53/udp`

### Too Many Domains Blocked

1. Check your blocklists - some are very aggressive
2. Add domains to whitelist:
   ```yaml
   whitelist:
     - "accidentally-blocked-domain.com"
   ```
3. Temporarily disable policy rules to isolate the issue

### High Memory Usage

1. Reduce cache size: `max_entries: 5000`
2. Disable query logging: `database.enabled: false`
3. Use minimal configuration as a baseline

## Need Help?

- [Main Documentation](../README.md)
- [Policy Engine Guide](../docs/policy-engine.md)
- [GitHub Issues](https://github.com/erfianugrah/gloryhole/issues)

---

**Pro Tip:** Start with the configuration closest to your use case, then gradually customize it. Don't try to use `advanced-filtering.yml` as your first config unless you're already familiar with the system!
