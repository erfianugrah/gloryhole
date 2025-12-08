# Policy Engine Documentation

## Table of Contents

1. [Overview](#overview)
2. [Getting Started](#getting-started)
3. [Rule Structure](#rule-structure)
4. [Actions](#actions)
5. [Helper Functions](#helper-functions)
6. [Context Variables](#context-variables)
7. [Expression Language](#expression-language)
8. [Common Patterns](#common-patterns)
9. [Advanced Examples](#advanced-examples)
10. [REST API Management](#rest-api-management)
11. [Performance Considerations](#performance-considerations)
12. [Troubleshooting](#troubleshooting)

---

## Overview

The Policy Engine is a powerful, flexible DNS filtering system that allows you to create custom rules for handling DNS queries. Unlike simple blocklists, the Policy Engine uses expression-based logic to make intelligent decisions about how to handle each DNS query.

> Persistence: Rules added via the REST API or UI are written back to the config file when the server is started with a writable `--config` path. If the config is read-only or unset, changes remain in memory until restart.

### Key Features

- **Expression-Based Rules**: Write complex logic using a simple expression language
- **Multiple Actions**: BLOCK, ALLOW, or REDIRECT queries
- **Rich Context**: Access domain, client IP, query type, time, and more
- **Helper Functions**: 10+ built-in functions for common filtering patterns
- **Dynamic Management**: Update rules via REST API without server restart
- **High Performance**: Rules are compiled for fast evaluation (64ns per rule)
- **Order Matters**: First matching rule wins

### Use Cases

- **Parental Controls**: Block inappropriate content during specific hours
- **Network Policy**: Enforce different rules for different IP ranges
- **Ad Blocking**: Advanced ad filtering with pattern matching
- **Security**: Block malicious domains based on patterns
- **Productivity**: Block social media during work hours
- **Custom Routing**: Redirect specific domains to internal servers

---

## Getting Started

### Enabling the Policy Engine

Add policy rules to your `config.yml`:

```yaml
policy:
  enabled: true
  rules:
    - name: "Block social media during work hours"
      logic: 'Weekday && Hour >= 9 && Hour < 17 && DomainMatches(Domain, "facebook")'
      action: "BLOCK"
      enabled: true

    - name: "Allow all other queries"
      logic: 'true'
      action: "ALLOW"
      enabled: true
```

### Rule Evaluation Order

**Important**: Rules are evaluated in the order they appear in the configuration. The first rule that matches determines the action taken.

```yaml
rules:
  # Rule 1: Checked first
  - name: "Admin bypass"
    logic: 'IPEquals(ClientIP, "192.168.1.100")'
    action: "ALLOW"
    enabled: true

  # Rule 2: Checked second (only if Rule 1 doesn't match)
  - name: "Block social media"
    logic: 'DomainMatches(Domain, "social")'
    action: "BLOCK"
    enabled: true
```

---

## Rule Structure

Each policy rule has the following fields:

```yaml
- name: "Rule Name"              # Human-readable description
  logic: 'expression'            # Boolean expression (must evaluate to true/false)
  action: "BLOCK"                # Action to take: BLOCK, ALLOW, or REDIRECT
  action_data: "192.168.1.1"    # Optional data for REDIRECT action
  enabled: true                  # Enable/disable without removing rule
```

### Required Fields

- **name**: Unique identifier for the rule (used in logs and API)
- **logic**: Expression that evaluates to true or false
- **action**: One of `BLOCK`, `ALLOW`, or `REDIRECT`
- **enabled**: Boolean to enable/disable the rule

### Optional Fields

- **action_data**: Required for `REDIRECT` action (target IP address)

---

## Actions

### BLOCK

Returns `NXDOMAIN` (domain not found) to the client.

```yaml
- name: "Block ads"
  logic: 'DomainMatches(Domain, "ads")'
  action: "BLOCK"
  enabled: true
```

**Use Cases:**
- Ad blocking
- Malware protection
- Parental controls
- Productivity enforcement

### ALLOW

Bypasses all filtering (including blocklists) and forwards query to upstream DNS.

```yaml
- name: "Always allow corporate domains"
  logic: 'DomainEndsWith(Domain, ".company.com")'
  action: "ALLOW"
  enabled: true
```

**Use Cases:**
- Whitelisting trusted domains
- Admin bypass
- Override blocklists for specific domains

### REDIRECT

Returns a custom IP address instead of the real DNS resolution. Supports both IPv4 and IPv6.

```yaml
- name: "Redirect ads to blackhole"
  logic: 'DomainMatches(Domain, "doubleclick")'
  action: "REDIRECT"
  action_data: "0.0.0.0"
  enabled: true
```

**Use Cases:**
- Redirect to captive portal
- Redirect to warning page
- Redirect to internal mirror/cache
- Ad blocking with pixel tracking prevention

**Important Notes:**
- Query type must match redirect IP version (A for IPv4, AAAA for IPv6)
- If query type doesn't match, returns NODATA response

---

## Helper Functions

The Policy Engine provides 10 built-in helper functions to simplify common filtering patterns.

### Domain Matching Functions

#### DomainMatches(domain, substring)

Checks if domain contains substring (case-insensitive).

```yaml
logic: 'DomainMatches(Domain, "facebook")'
```

**Matches:**
- `www.facebook.com`
- `m.facebook.com`
- `facebook.net`

**Special Behavior:**
- If pattern starts with `.`, matches suffix: `DomainMatches(Domain, ".facebook.com")` matches `www.facebook.com` but NOT `facebook.com.malicious.site`

#### DomainEndsWith(domain, suffix)

Checks if domain ends with suffix (case-insensitive).

```yaml
logic: 'DomainEndsWith(Domain, ".ru")'
```

**Matches:**
- `example.ru`
- `site.example.ru`

**Use Cases:**
- Block specific TLDs
- Filter by country code
- Match all subdomains

#### DomainStartsWith(domain, prefix)

Checks if domain starts with prefix (case-insensitive).

```yaml
logic: 'DomainStartsWith(Domain, "cdn")'
```

**Matches:**
- `cdn.example.com`
- `cdn123.site.com`

#### DomainRegex(domain, pattern)

Match domain against a regular expression pattern.

```yaml
logic: 'DomainRegex(Domain, "^cdn\\d+\\.")'
```

**Matches:**
- `cdn1.example.com`
- `cdn999.site.com`

**Does NOT Match:**
- `cdn.example.com` (no number)
- `mycdn1.example.com` (doesn't start with cdn)

**Use Cases:**
- Complex pattern matching
- Numeric subdomain filtering
- Advanced ad blocking patterns

**Performance Note:** Regex is slower than simple string matching. Use sparingly.

#### DomainLevelCount(domain) → int

Returns the number of labels in a domain.

```yaml
logic: 'DomainLevelCount(Domain) > 4'
```

**Examples:**
- `example.com` → 2
- `www.example.com` → 3
- `cdn.assets.www.example.com` → 5

**Use Cases:**
- Block suspiciously deep subdomains
- Filter by domain structure
- Detect DGA (Domain Generation Algorithm) patterns

### IP Matching Functions

#### IPInCIDR(ip, cidr)

Checks if IP address is within a CIDR range.

```yaml
logic: 'IPInCIDR(ClientIP, "192.168.1.0/24")'
```

**Examples:**
- Guest network: `192.168.2.0/24`
- Admin network: `10.0.0.0/28`
- Private networks: `10.0.0.0/8`

**Use Cases:**
- Network-based policies
- Guest network restrictions
- Admin bypass rules

#### IPEquals(ip1, ip2)

Exact IP address comparison with IPv4/IPv6 normalization.

```yaml
logic: 'IPEquals(ClientIP, "192.168.1.100")'
```

**Handles:**
- IPv4: `192.168.1.1`
- IPv6: `2001:db8::1`
- IPv6 normalization: `2001:0db8::1` equals `2001:db8::1`

### Query Type Functions

#### QueryTypeIn(type, ...types)

Checks if query type matches any in the list (case-insensitive).

```yaml
logic: 'QueryTypeIn(QueryType, "A", "AAAA")'
```

**Common Types:**
- `A` - IPv4 address
- `AAAA` - IPv6 address
- `CNAME` - Canonical name
- `MX` - Mail exchange
- `TXT` - Text record
- `NS` - Name server
- `SOA` - Start of authority

**Use Cases:**
- Block specific query types
- Filter DNS tunneling attempts
- Policy based on resolution type

### Time Functions

#### IsWeekend(weekday) → bool

Checks if day is Saturday (6) or Sunday (0).

```yaml
logic: 'IsWeekend(Weekday)'
```

**Use Cases:**
- Relax filtering on weekends
- Weekend-only blocks
- Time-based parental controls

#### InTimeRange(hour, minute, startH, startM, endH, endM) → bool

Checks if current time is within range (handles overnight ranges).

```yaml
logic: 'InTimeRange(Hour, Minute, 9, 30, 17, 0)'
```

**Examples:**
- Work hours: `InTimeRange(Hour, Minute, 9, 0, 17, 0)`
- Lunch break: `InTimeRange(Hour, Minute, 12, 0, 13, 0)`
- Overnight: `InTimeRange(Hour, Minute, 22, 0, 6, 0)` (10 PM - 6 AM)

**Important:** Automatically handles ranges that cross midnight.

---

## Context Variables

These variables are available in all rule expressions:

| Variable | Type | Description | Example |
|----------|------|-------------|---------|
| `Domain` | string | Queried domain (without trailing dot) | `www.example.com` |
| `ClientIP` | string | IP address of DNS client | `192.168.1.100` |
| `QueryType` | string | DNS query type | `A`, `AAAA`, `CNAME` |
| `Hour` | int | Current hour (0-23) | `14` (2 PM) |
| `Minute` | int | Current minute (0-59) | `30` |
| `Day` | int | Day of month (1-31) | `15` |
| `Month` | int | Month (1-12) | `6` (June) |
| `Weekday` | int | Day of week (0-6, Sunday=0) | `1` (Monday) |
| `Time` | time.Time | Full timestamp | - |

**Note:** Domains are normalized (lowercase, no trailing dot) before evaluation.

---

## Expression Language

The Policy Engine uses [expr-lang](https://github.com/expr-lang/expr) for expressions.

### Boolean Operators

```yaml
# AND
logic: 'Weekday && Hour >= 9'

# OR
logic: 'DomainMatches(Domain, "facebook") || DomainMatches(Domain, "twitter")'

# NOT
logic: '!IsWeekend(Weekday)'

# Parentheses
logic: '(Hour >= 9 && Hour < 17) && !IsWeekend(Weekday)'
```

### Comparison Operators

```yaml
# Equals
logic: 'Domain == "blocked.com"'

# Not equals
logic: 'QueryType != "A"'

# Greater than, Less than
logic: 'Hour >= 9 && Hour < 17'

# Contains (for strings)
logic: 'Domain contains "ad"'  # Built-in string function
```

### String Functions (Built-in)

```yaml
# Contains
logic: 'Domain contains "tracking"'

# String equality
logic: 'Domain == "exact-match.com"'
```

### Numeric Comparisons

```yaml
# Range check
logic: 'Hour >= 9 && Hour <= 17'

# Greater than
logic: 'DomainLevelCount(Domain) > 4'
```

---

## Common Patterns

### Block During Specific Hours

```yaml
- name: "Block social media during work hours"
  logic: 'Weekday && Hour >= 9 && Hour < 17 && (DomainMatches(Domain, "facebook") || DomainMatches(Domain, "twitter"))'
  action: "BLOCK"
  enabled: true
```

### Network-Based Rules

```yaml
- name: "Guest network restrictions"
  logic: 'IPInCIDR(ClientIP, "192.168.2.0/24") && DomainMatches(Domain, "gaming")'
  action: "BLOCK"
  enabled: true
```

### Admin Bypass

```yaml
# Place this rule FIRST
- name: "Admin full access"
  logic: 'IPInCIDR(ClientIP, "10.0.0.0/28")'
  action: "ALLOW"
  enabled: true
```

### Redirect Ads to Blackhole

```yaml
- name: "Block ads via redirect"
  logic: 'DomainMatches(Domain, "ads") || DomainMatches(Domain, "tracker")'
  action: "REDIRECT"
  action_data: "0.0.0.0"
  enabled: true
```

### Block Suspicious Domains

```yaml
- name: "Block deep subdomains"
  logic: 'DomainLevelCount(Domain) > 5'
  action: "BLOCK"
  enabled: true

- name: "Block numeric CDNs"
  logic: 'DomainRegex(Domain, "^cdn\\d+\\.")'
  action: "BLOCK"
  enabled: true
```

### Time-Based Filtering

```yaml
- name: "Bedtime internet cutoff"
  logic: 'InTimeRange(Hour, Minute, 22, 0, 7, 0)'
  action: "BLOCK"
  enabled: true

- name: "Weekend freedom"
  logic: 'IsWeekend(Weekday)'
  action: "ALLOW"
  enabled: true
```

---

## Advanced Examples

### Comprehensive Home Network Setup

```yaml
policy:
  enabled: true
  rules:
    # Rule 1: Admin bypass (checked first)
    - name: "Admin full access"
      logic: 'IPEquals(ClientIP, "192.168.1.100")'
      action: "ALLOW"
      enabled: true

    # Rule 2: Block adult content 24/7
    - name: "Block adult content"
      logic: 'DomainMatches(Domain, "porn") || DomainMatches(Domain, "xxx")'
      action: "BLOCK"
      enabled: true

    # Rule 3: Kids network - strict filtering
    - name: "Kids network social media"
      logic: 'IPInCIDR(ClientIP, "192.168.3.0/24") && (DomainMatches(Domain, "social") || DomainMatches(Domain, "gaming"))'
      action: "BLOCK"
      enabled: true

    # Rule 4: School hours restrictions
    - name: "School hours blocking"
      logic: 'Weekday && InTimeRange(Hour, Minute, 9, 0, 15, 0) && DomainMatches(Domain, "gaming")'
      action: "BLOCK"
      enabled: true

    # Rule 5: Bedtime enforcement
    - name: "Bedtime cutoff"
      logic: 'InTimeRange(Hour, Minute, 22, 0, 7, 0) && !IPEquals(ClientIP, "192.168.1.100")'
      action: "BLOCK"
      enabled: false  # Enable per family needs

    # Rule 6: Default allow
    - name: "Allow everything else"
      logic: 'true'
      action: "ALLOW"
      enabled: true
```

### Small Office Network

```yaml
policy:
  enabled: true
  rules:
    # Admin and management bypass
    - name: "Management bypass"
      logic: 'IPInCIDR(ClientIP, "10.0.0.0/24")'
      action: "ALLOW"
      enabled: true

    # Block social media during work hours
    - name: "Work hours social media"
      logic: 'Weekday && InTimeRange(Hour, Minute, 9, 0, 17, 0) && (DomainMatches(Domain, "facebook") || DomainMatches(Domain, "twitter") || DomainMatches(Domain, "instagram"))'
      action: "BLOCK"
      enabled: true

    # Allow lunch break
    - name: "Lunch break exception"
      logic: 'Weekday && InTimeRange(Hour, Minute, 12, 0, 13, 0)'
      action: "ALLOW"
      enabled: true

    # Block streaming to save bandwidth
    - name: "Block video streaming"
      logic: 'Weekday && Hour >= 9 && Hour < 17 && (DomainMatches(Domain, "youtube") || DomainMatches(Domain, "netflix"))'
      action: "BLOCK"
      enabled: true

    # Guest network restrictions
    - name: "Guest network limits"
      logic: 'IPInCIDR(ClientIP, "10.0.100.0/24")'
      action: "BLOCK"
      enabled: true

    # Default allow
    - name: "Allow everything else"
      logic: 'true'
      action: "ALLOW"
      enabled: true
```

### Advanced Security Filtering

```yaml
policy:
  enabled: true
  rules:
    # Block DGA-like domains (deep subdomains)
    - name: "Block suspicious deep domains"
      logic: 'DomainLevelCount(Domain) > 5'
      action: "BLOCK"
      enabled: true

    # Block numeric CDN domains (often malware)
    - name: "Block numeric CDNs"
      logic: 'DomainRegex(Domain, "^(cdn|img|data)\\d+\\.")'
      action: "BLOCK"
      enabled: true

    # Block suspicious TLDs
    - name: "Block risky TLDs"
      logic: 'DomainEndsWith(Domain, ".xyz") || DomainEndsWith(Domain, ".top") || DomainEndsWith(Domain, ".click")'
      action: "BLOCK"
      enabled: false  # May have false positives

    # Block DNS over HTTPS attempts (enterprise)
    - name: "Block DoH providers"
      logic: 'DomainMatches(Domain, "dns.google") || DomainMatches(Domain, "cloudflare-dns")'
      action: "BLOCK"
      enabled: false

    # Allow everything else
    - name: "Default allow"
      logic: 'true'
      action: "ALLOW"
      enabled: true
```

### Multi-Tenant Network

```yaml
policy:
  enabled: true
  rules:
    # Tenant A - Full access
    - name: "Tenant A unrestricted"
      logic: 'IPInCIDR(ClientIP, "10.1.0.0/16")'
      action: "ALLOW"
      enabled: true

    # Tenant B - Restricted
    - name: "Tenant B restrict social"
      logic: 'IPInCIDR(ClientIP, "10.2.0.0/16") && DomainMatches(Domain, "social")'
      action: "BLOCK"
      enabled: true

    # Tenant C - Highly restricted
    - name: "Tenant C whitelist only"
      logic: 'IPInCIDR(ClientIP, "10.3.0.0/16") && !(DomainEndsWith(Domain, ".company.com") || DomainEndsWith(Domain, ".google.com"))'
      action: "BLOCK"
      enabled: true

    # Default allow
    - name: "Default allow"
      logic: 'true'
      action: "ALLOW"
      enabled: true
```

---

## REST API Management

The Policy Engine can be managed dynamically via REST API without restarting the server.

### API Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/policies` | List all policies |
| `POST` | `/api/policies` | Add new policy |
| `GET` | `/api/policies/{id}` | Get specific policy |
| `PUT` | `/api/policies/{id}` | Update policy |
| `DELETE` | `/api/policies/{id}` | Delete policy |

### List Policies

```bash
curl http://localhost:8080/api/policies
```

**Response:**
```json
{
  "policies": [
    {
      "id": 0,
      "name": "Block ads",
      "logic": "DomainMatches(Domain, \"ads\")",
      "action": "BLOCK",
      "enabled": true
    }
  ],
  "total": 1
}
```

### Add Policy

```bash
curl -X POST http://localhost:8080/api/policies \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Block gaming during work",
    "logic": "Weekday && Hour >= 9 && Hour < 17 && DomainMatches(Domain, \"gaming\")",
    "action": "BLOCK",
    "enabled": true
  }'
```

**Response:**
```json
{
  "id": 1,
  "name": "Block gaming during work",
  "logic": "Weekday && Hour >= 9 && Hour < 17 && DomainMatches(Domain, \"gaming\")",
  "action": "BLOCK",
  "enabled": true
}
```

### Update Policy

```bash
curl -X PUT http://localhost:8080/api/policies/1 \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Block gaming (updated)",
    "logic": "DomainMatches(Domain, \"gaming\")",
    "action": "BLOCK",
    "enabled": false
  }'
```

### Delete Policy

```bash
curl -X DELETE http://localhost:8080/api/policies/1
```

**Response:**
```json
{
  "message": "Policy deleted successfully",
  "id": 1,
  "name": "Block gaming (updated)"
}
```

### Error Responses

```json
{
  "error": "Bad Request",
  "code": 400,
  "message": "Policy name is required"
}
```

**Common Error Codes:**
- `400` - Invalid request (validation failure)
- `404` - Policy not found
- `503` - Policy engine not configured

---

## Performance Considerations

### Rule Compilation

Rules are compiled once when loaded, not on every query.

**Compilation happens:**
- At server startup
- When adding rules via API
- When updating rules via API

**Performance:**
- Rule compilation: ~1ms per rule
- Rule evaluation: ~64ns per rule
- Typical overhead: <1µs for 10 rules

### Optimization Tips

1. **Place common rules first**: First matching rule wins
   ```yaml
   rules:
     - name: "Admin bypass"  # Checked for every admin query
       logic: 'IPEquals(ClientIP, "192.168.1.100")'
       action: "ALLOW"

     - name: "Complex rule"  # Only checked if admin rule doesn't match
       logic: 'complex expression...'
       action: "BLOCK"
   ```

2. **Avoid expensive operations in hot paths**:
   - `DomainRegex` is slower than `DomainMatches`
   - Use simple comparisons when possible
   - Combine conditions efficiently

3. **Use specific rules over broad rules**:
   ```yaml
   # Better
   logic: 'Domain == "specific.com"'

   # Slower (if checking many rules)
   logic: 'DomainMatches(Domain, "specific")'
   ```

4. **Disable unused rules**: Set `enabled: false` instead of deleting

5. **Monitor rule evaluation**: Check logs for slow rules

### Benchmarks

From actual testing:

```
Policy evaluation:          64 ns/op
Concurrent DNS handling:   116 ns/op
DNS with policy:          ~500 ns/op
```

**Capacity:**
- 10,000+ queries/second with 20 rules
- Sub-millisecond latency added
- Scales linearly with rule count

---

## Troubleshooting

### Rule Not Matching

**Problem**: Rule doesn't block/allow as expected

**Solutions:**

1. **Check domain normalization**:
   ```yaml
   # WRONG - domains don't have trailing dots in policy context
   logic: 'Domain == "example.com."'

   # CORRECT
   logic: 'Domain == "example.com"'
   ```

2. **Check rule order**: Earlier rules take precedence
   ```yaml
   rules:
     - name: "Allow all"  # This will match everything!
       logic: 'true'
       action: "ALLOW"

     - name: "Block ads"  # This will NEVER run
       logic: 'DomainMatches(Domain, "ads")'
       action: "BLOCK"
   ```

3. **Enable debug logging**:
   ```yaml
   logging:
     level: "debug"  # Shows policy evaluation
   ```

4. **Test expression syntax**: Use API to test rules interactively

### REDIRECT Not Working

**Problem**: REDIRECT action returns NXDOMAIN or wrong IP

**Solutions:**

1. **Check IP version compatibility**:
   ```yaml
   # For A queries (IPv4)
   action: "REDIRECT"
   action_data: "192.168.1.1"  # IPv4

   # For AAAA queries (IPv6)
   action: "REDIRECT"
   action_data: "2001:db8::1"  # IPv6
   ```

2. **Verify action_data is valid**:
   ```yaml
   action_data: "192.168.1.1"  # Valid
   action_data: "invalid"       # Will fail
   ```

3. **Check query type**:
   - A query with IPv6 redirect → NODATA response
   - AAAA query with IPv4 redirect → NODATA response

### Performance Issues

**Problem**: DNS queries are slow

**Solutions:**

1. **Count your rules**: Fewer rules = faster
   ```bash
   # Check rule count
   curl http://localhost:8080/api/policies | jq '.total'
   ```

2. **Profile rule evaluation**: Enable timing logs
   ```yaml
   logging:
     level: "info"
     add_source: true
   ```

3. **Avoid regex in hot paths**:
   ```yaml
   # Faster
   logic: 'DomainMatches(Domain, "cdn")'

   # Slower (but more precise)
   logic: 'DomainRegex(Domain, "^cdn\\d+\\.")'
   ```

4. **Use cache**: Policy results benefit from DNS caching
   ```yaml
   cache:
     enabled: true
     max_entries: 10000
   ```

### Syntax Errors

**Problem**: Rule won't compile

**Common Mistakes:**

1. **Unmatched quotes**:
   ```yaml
   logic: 'Domain == "example.com'  # Missing closing quote
   logic: 'Domain == "example.com"'  # Correct
   ```

2. **Wrong operator**:
   ```yaml
   logic: 'Domain = "example.com"'   # Wrong (single =)
   logic: 'Domain == "example.com"'  # Correct (double ==)
   ```

3. **Invalid function name**:
   ```yaml
   logic: 'Contains(Domain, "ads")'     # Wrong function name
   logic: 'DomainMatches(Domain, "ads")'  # Correct
   ```

4. **Wrong parameter types**:
   ```yaml
   logic: 'Hour >= "9"'  # Wrong (string)
   logic: 'Hour >= 9'    # Correct (integer)
   ```

### API Returns 503

**Problem**: `Policy engine not configured`

**Solution:**

Ensure policy engine is enabled in config:
```yaml
policy:
  enabled: true
  rules: []  # Can be empty
```

And server is started with policy engine:
```go
// In main.go
if cfg.Policy.Enabled {
    policyEngine := policy.NewEngine()
    // ... add rules
    handler.SetPolicyEngine(policyEngine)

    // Pass to API
    apiServer := api.New(&api.Config{
        PolicyEngine: policyEngine,  // Must pass this
    })
}
```

---

## Best Practices

### Security

1. **Always validate user input**: When accepting rules via API
2. **Limit rule complexity**: Prevent denial of service
3. **Log policy decisions**: Monitor what's being blocked
4. **Regular review**: Audit rules periodically
5. **Test in staging**: Validate rules before production

### Organization

1. **Use clear names**: Make rules self-documenting
2. **Group related rules**: Keep similar rules together
3. **Comment complex logic**: Add comments in config
4. **Version control**: Track rule changes in git
5. **Document exceptions**: Explain why rules exist

### Performance

1. **Minimize rule count**: Combine similar rules
2. **Order by frequency**: Most common matches first
3. **Cache aggressive**: Reduce repeated evaluations
4. **Monitor metrics**: Track policy evaluation time
5. **Benchmark changes**: Test before deploying

### Maintainability

1. **Start simple**: Add complexity only when needed
2. **One rule, one purpose**: Don't combine unrelated logic
3. **Test thoroughly**: Verify rules work as expected
4. **Document behavior**: Write comments for future you
5. **Regular cleanup**: Remove obsolete rules

---

## FAQ

**Q: Can I reload rules without restarting the server?**
A: Yes! Use the REST API to add/update/delete rules dynamically.

**Q: What happens if no rule matches?**
A: The query proceeds normally (forwards to upstream DNS).

**Q: Can I use regular expressions?**
A: Yes, use the `DomainRegex()` function, but be aware it's slower.

**Q: How do I debug why a rule isn't matching?**
A: Enable debug logging (`level: "debug"`) to see policy evaluation.

**Q: Can rules access request history?**
A: No, rules only see the current query context.

**Q: What's the maximum number of rules?**
A: No hard limit, but performance degrades linearly. Keep under 100 for best performance.

**Q: Can I use external data sources?**
A: Not directly. Rules must be self-contained expressions.

**Q: How do I block a whole category (ads, social, etc.)?**
A: Use `DomainMatches()` with key terms, or maintain a blocklist file.

**Q: Can I schedule rule changes?**
A: Not built-in. Use cron + API calls to modify rules on schedule.

**Q: Is there a rule testing tool?**
A: Use the API to add test rules with `enabled: false`, then check logs.

---

## Additional Resources

- **Example Configs**: See `examples/` directory
  - `home-network.yml` - Family-friendly setup
  - `small-office.yml` - Business network
  - `advanced-filtering.yml` - All features showcase
- **Source Code**: `pkg/policy/engine.go`
- **API Reference**: `/api/policies` endpoints
- **Expression Language**: [expr-lang documentation](https://github.com/expr-lang/expr)

---

## Changelog

### v2.0 (Current)
- Added REDIRECT action with IPv4/IPv6 support
- Added 6 new helper functions (DomainRegex, DomainLevelCount, IPEquals, QueryTypeIn, IsWeekend, InTimeRange)
- Added REST API for runtime policy management
- Improved error messages and validation
- 97% test coverage

### v1.0 (Initial)
- Basic BLOCK/ALLOW actions
- 4 helper functions (DomainMatches, DomainEndsWith, DomainStartsWith, IPInCIDR)
- Expression-based rule engine
- Time-based filtering (Hour, Weekday variables)
