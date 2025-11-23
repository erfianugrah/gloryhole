# Pattern Matching Guide

**Last Updated**: 2025-11-23
**Version**: 0.7.8

This guide explains Glory-Hole's multi-tier pattern matching system for blocklists and whitelists, including exact matching, wildcard patterns, and regular expressions.

---

## Table of Contents

- [Overview](#overview)
- [Pattern Types](#pattern-types)
  - [Exact Matching](#exact-matching)
  - [Wildcard Patterns](#wildcard-patterns)
  - [Regular Expressions](#regular-expressions)
- [Performance Characteristics](#performance-characteristics)
- [Configuration Examples](#configuration-examples)
- [Best Practices](#best-practices)
- [Common Patterns](#common-patterns)
- [Troubleshooting](#troubleshooting)

---

## Overview

Glory-Hole supports three types of domain pattern matching:

1. **Exact Match** - Fast O(1) hash map lookup
2. **Wildcard** - Domain suffix matching with wildcards
3. **Regex** - Full regular expression support

Patterns are evaluated in order of performance: exact → wildcard → regex. This ensures the fastest possible lookups for the most common cases.

---

## Pattern Types

### Exact Matching

**Format**: Plain domain name
**Performance**: O(1) - ~8ns lookup
**Use case**: Block specific known domains

#### Syntax

```yaml
blocklists:
  exact:
    - "doubleclick.net"
    - "ads.google.com"
    - "facebook.com"
```

Or in standard blocklist format:
```
doubleclick.net
ads.google.com
facebook.com
```

#### Behavior

- **Matches only exact domain**: `ads.google.com` matches `ads.google.com` but not `malware.ads.google.com`
- **Case-insensitive**: `ADS.GOOGLE.COM` == `ads.google.com`
- **No subdomain matching**: Specify each subdomain explicitly

#### Examples

| Pattern | Matches | Doesn't Match |
|---------|---------|---------------|
| `example.com` | `example.com` | `www.example.com`, `sub.example.com` |
| `ads.example.com` | `ads.example.com` | `malware.ads.example.com`, `example.com` |
| `tracker.net` | `tracker.net` | `sub.tracker.net` |

---

### Wildcard Patterns

**Format**: `*.domain.com` or `domain.*`
**Performance**: O(n) - ~50ns per pattern
**Use case**: Block entire subdomains or domain families

#### Syntax

```yaml
whitelist_patterns:
  wildcard:
    - "*.cloudfront.net"      # Matches all cloudfront subdomains
    - "*.s3.amazonaws.com"    # Matches all S3 buckets
    - "github.*"              # Matches github.com, github.io, etc.
```

Or using the simplified format in config:
```yaml
whitelist_patterns:
  - "*.cloudfront.net"
  - "*.s3.amazonaws.com"
```

#### Wildcard Syntax

| Pattern | Description | Matches | Doesn't Match |
|---------|-------------|---------|---------------|
| `*.example.com` | Any subdomain | `www.example.com`, `api.example.com` | `example.com`, `example.org` |
| `example.*` | Any TLD | `example.com`, `example.org`, `example.net` | `test.example.com` |
| `*example.com` | Substring match | `myexample.com`, `example.com` | `example.org` |

**Important**: Wildcards are not regex. Use `*` for "any characters", not `.` or other regex syntax.

#### Behavior

```yaml
# Pattern: *.cdn.cloudflare.net
Matches:
  ✅ images.cdn.cloudflare.net
  ✅ video.cdn.cloudflare.net
  ✅ a.b.cdn.cloudflare.net

Doesn't match:
  ❌ cdn.cloudflare.net (no subdomain)
  ❌ cloudflare.net
  ❌ other.cloudflare.net
```

---

### Regular Expressions

**Format**: Regex pattern enclosed in `/`
**Performance**: O(n) - ~200ns per pattern
**Use case**: Complex matching logic, advanced filtering

#### Syntax

```yaml
blocklist_patterns:
  regex:
    - "/^ad[s]?\\..*$/"           # Matches ad.*, ads.*
    - "/^.*-ad-.*\\.com$/"        # Matches *-ad-*.com
    - "/^track(er|ing)?\\..+$/"   # Matches track.*, tracker.*, tracking.*
```

Or using the pattern field:
```yaml
blocklist_patterns:
  - pattern: "^ad[s]?\\..*$"
    type: "regex"
```

#### Regex Syntax

Glory-Hole uses Go's `regexp` package (RE2 syntax):

**Common patterns:**

| Regex | Description | Matches | Doesn't Match |
|-------|-------------|---------|---------------|
| `^ads?\\.` | "ad" or "ads" prefix | `ad.example.com`, `ads.example.com` | `bad.example.com` |
| `\\.ad\\.` | ".ad." substring | `foo.ad.example.com` | `ad.example.com` |
| `(foo\|bar)` | "foo" or "bar" | `foo.com`, `bar.com` | `baz.com` |
| `^.*-ad-.*$` | Contains "-ad-" | `my-ad-server.com` | `myad.com` |
| `\\.co\\.uk$` | Ends with .co.uk | `example.co.uk` | `example.com` |

#### Escaping Special Characters

Regex special characters must be escaped:

```yaml
# Wrong: Will not work as expected
pattern: "ad.example.com"  # . matches any character

# Correct: Escape the dot
pattern: "ad\\.example\\.com"  # Matches literal "ad.example.com"
```

**Characters to escape**: `. * + ? ^ $ ( ) [ ] { } | \`

---

## Performance Characteristics

### Lookup Times

| Pattern Type | Average Lookup | QPS Capability | Memory |
|-------------|----------------|----------------|--------|
| **Exact** | 8ns | 125M QPS | ~140 bytes/domain |
| **Wildcard** | 50ns | 20M QPS | ~200 bytes/pattern |
| **Regex** | 200ns | 5M QPS | ~500 bytes/pattern |

### Optimization Strategy

The pattern matcher evaluates in order:

```
1. Exact match (fastest)
   ↓ if not matched
2. Wildcard patterns
   ↓ if not matched
3. Regex patterns (slowest)
```

This ensures:
- Common domains hit fast path (exact match)
- Less common patterns use wildcards
- Complex logic uses regex only when needed

### Scalability

**Blocklist Size Impact:**

| Domains | Exact Lookup | Wildcard (10 patterns) | Regex (10 patterns) |
|---------|-------------|----------------------|-------------------|
| 100K | 8ns | 500ns | 2µs |
| 500K | 8ns | 500ns | 2µs |
| 1M | 8ns | 500ns | 2µs |

**Key Insight**: Exact matching is O(1) and doesn't degrade with blocklist size. Wildcard and regex scale with number of patterns, not total domains.

---

## Configuration Examples

### Example 1: Mixed Patterns

```yaml
# config.yml
blocklists:
  - "https://raw.githubusercontent.com/StevenBlack/hosts/master/hosts"  # Exact

whitelist:
  - "analytics.google.com"  # Exact whitelist

whitelist_patterns:
  - "*.cloudfront.net"      # Wildcard: Allow all CloudFront
  - "*.s3.amazonaws.com"    # Wildcard: Allow all S3

blocklist_patterns:
  - "/^ad[s]?\\..+$/"       # Regex: Block ad.*, ads.*
  - "/.*-ads?-.*/"          # Regex: Block *-ad-*, *-ads-*
```

### Example 2: Subdomain Blocking

Block all subdomains of ad networks:

```yaml
blocklist_patterns:
  # Exact
  - "doubleclick.net"

  # Wildcard - blocks ALL subdomains
  - "*.doubleclick.net"
  - "*.googlesyndication.com"
  - "*.advertising.com"
```

### Example 3: Regex Patterns

Advanced blocking with regular expressions:

```yaml
blocklist_patterns:
  # Block anything starting with "ad" or "ads"
  - pattern: "^ads?\\..+"
    type: "regex"

  # Block tracker subdomains
  - pattern: "^track(er|ing)?\\..+"
    type: "regex"

  # Block telemetry domains
  - pattern: ".*\\.telemetry\\..*"
    type: "regex"

  # Block analytics with numbers (analytics1, analytics2, etc.)
  - pattern: "^analytics\\d+\\..+"
    type: "regex"
```

### Example 4: Allow CDNs, Block Ads

```yaml
# Whitelist all CDN domains
whitelist_patterns:
  - "*.cloudflare.net"
  - "*.fastly.net"
  - "*.akamaized.net"
  - "*.cloudfront.net"

# Block ad subdomains specifically
blocklist_patterns:
  - "*.ads.cdn.cloudflare.net"  # More specific, evaluated first
```

---

## Best Practices

### 1. Prefer Exact Matching

Use exact matching whenever possible for best performance:

```yaml
# Good: Fast O(1) lookup
blocklist:
  - "ads.example.com"
  - "tracker.example.com"

# Slower: O(n) wildcard
blocklist_patterns:
  - "*.example.com"
```

### 2. Minimize Regex Patterns

Regex is powerful but slow. Use sparingly:

```yaml
# Good: 10-20 regex patterns
blocklist_patterns:
  - "/^ad[s]?\\..+$/"
  - "/^track(er|ing)?\\..+$/"

# Bad: 1000+ regex patterns (slow)
```

### 3. Order Matters for Wildcards

More specific patterns should come first:

```yaml
whitelist_patterns:
  # Specific exception first
  - "secure.ads.example.com"  # Allow this specific one

  # General block second
  - "*.ads.example.com"       # Block all others
```

### 4. Test Pattern Performance

Benchmark your patterns:

```bash
# Check query performance
curl http://localhost:8080/api/stats

# Look for high avg_response_ms
# If > 10ms, patterns might be too complex
```

### 5. Use Wildcards for CDN Whitelisting

CDNs often have many subdomains - use wildcards:

```yaml
whitelist_patterns:
  - "*.cloudfront.net"
  - "*.fastly.net"
  - "*.akamaized.net"
  - "*.cloudflare.net"
```

---

## Common Patterns

### Block Ad Networks

```yaml
blocklist_patterns:
  # Exact domains
  - "doubleclick.net"
  - "googlesyndication.com"

  # All subdomains
  - "*.doubleclick.net"
  - "*.googlesyndication.com"

  # Regex: ad servers with numbers
  - "/^ad[s]?\\d*\\..+$/"
```

### Block Trackers

```yaml
blocklist_patterns:
  - "*.tracking.com"
  - "/^track(er|ing)\\..+$/"
  - "*.analytics.google.com"
  - "/.*\\.telemetry\\..+$/"
```

### Block Crypto Mining

```yaml
blocklist_patterns:
  - "*.coinhive.com"
  - "*.cryptoloot.pro"
  - "/.*miner.*\\.js$/"
  - "/.*crypto.*\\.worker\\.js$/"
```

### Allow Local Development

```yaml
whitelist_patterns:
  - "*.local"
  - "*.localhost"
  - "*.test"
  - "*.dev"
```

### Allow Microsoft/Google Services

```yaml
whitelist_patterns:
  # Microsoft
  - "*.microsoft.com"
  - "*.live.com"
  - "*.office.com"
  - "*.msftncsi.com"

  # Google (non-ad services)
  - "*.googleapis.com"
  - "*.gstatic.com"
  - "accounts.google.com"
```

---

## Troubleshooting

### Pattern Not Matching

**Problem**: Domain should match but doesn't.

**Debug:**
```bash
# Test query
dig @localhost ads.example.com

# Check logs with debug enabled
grep "Pattern match" /var/log/glory-hole/glory-hole.log
```

**Common causes:**
1. **Incorrect escaping**: `ad.example.com` vs `ad\\.example\\.com`
2. **Wrong anchor**: `ads\\.` doesn't match `test.ads.com` (use `.*ads\\.`)
3. **Case sensitivity**: Patterns should be lowercase

### Too Many Patterns Causing Slowdown

**Problem**: DNS queries are slow (> 10ms).

**Solution:**
```yaml
# Reduce regex patterns
# Before: 100 regex patterns
# After: 10-20 most important ones

# Convert regex to wildcards where possible
# Before: "/^ads\\..*$/"
# After: "*.ads.*" (if wildcard works)

# Convert to exact matches
# Before: "/^specific\\.domain\\.com$/"
# After: "specific.domain.com"
```

### Wildcard Not Working

**Problem**: `*.example.com` doesn't match subdomains.

**Check:**
```yaml
# Correct format
whitelist_patterns:
  - "*.example.com"  # ✅ Matches sub.example.com

# Wrong formats
whitelist_patterns:
  - ".*.example.com"  # ❌ This is regex syntax
  - "*example.com"    # ⚠️ Matches anythingexample.com
```

### Regex Compilation Error

**Problem**: Error: "failed to compile regex pattern".

**Solution:**
```yaml
# Check escaping
# Wrong: "ad.example.com" (. matches anything)
# Right: "ad\\.example\\.com"

# Validate regex online
# Use: https://regex101.com/ (set to Golang flavor)
```

---

## Pattern Testing

Test patterns before deploying:

### Test Exact Match

```bash
# Should be blocked
dig @localhost doubleclick.net

# Expected: NXDOMAIN (blocked)
```

### Test Wildcard Match

```bash
# Pattern: *.ads.example.com

# Should match
dig @localhost malware.ads.example.com   # ✅ Blocked

# Should NOT match
dig @localhost ads.example.com           # ❌ Not blocked (no subdomain)
```

### Test Regex Match

```bash
# Pattern: /^ad[s]?\\..*$/

# Should match
dig @localhost ad.example.com     # ✅ Blocked
dig @localhost ads.example.com    # ✅ Blocked

# Should NOT match
dig @localhost bad.example.com    # ❌ Not blocked
```

---

## Performance Tuning

### Benchmark Your Patterns

```bash
# Check current performance
curl http://localhost:8080/api/stats

# Run load test
for i in {1..1000}; do
  dig @localhost test$i.example.com &
done

# Monitor avg response time
watch -n 1 'curl -s http://localhost:8080/api/stats | jq .avg_response_ms'
```

### Optimize Pattern Count

**Guidelines:**
- Exact matches: Unlimited (no performance impact)
- Wildcard patterns: < 50 recommended
- Regex patterns: < 20 recommended

**If exceeding limits:**
1. Convert regex → wildcard where possible
2. Convert wildcard → exact for known domains
3. Use blocklist URLs instead of patterns

---

## Next Steps

- [Configuration Guide](configuration.md) - Full config reference
- [Usage Guide](usage.md) - Managing blocklists
- [Policy Engine](../api/policy-engine.md) - Advanced filtering rules

---

**Master Pattern Matching!** 🎯

Use the right tool for the job: exact for speed, wildcard for families, regex for complexity.
