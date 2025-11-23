# Pi-hole to Glory-Hole Import Summary

This document details what was imported from your Pi-hole configuration into `config.personal.yml`.

## Import Source

- **Pi-hole Config**: `pihole/etc/pihole/pihole.toml`
- **Gravity Database**: `pihole/etc/pihole/gravity.db`
- **Import Date**: 2025-11-23

---

## Blocklists

Imported from `gravity.db` adlist table (enabled lists only):

### 1. Hagezi Ultimate (230,392 domains)
- **URL**: `https://raw.githubusercontent.com/hagezi/dns-blocklists/main/adblock/ultimate.txt`
- **Coverage**: Ads, tracking, malware, phishing, fake news, gambling, porn
- **Status**: ✅ Active in Pi-hole
- **Imported**: ✅ Yes

### 2. StevenBlack Fakenews+Gambling (107,316 domains)
- **URL**: `https://raw.githubusercontent.com/StevenBlack/hosts/master/alternates/fakenews-gambling/hosts`
- **Coverage**: Base blocklist + fake news + gambling sites
- **Status**: ✅ Active in Pi-hole
- **Imported**: ✅ Yes

**Total Blocking**: ~337,708 unique domains (after deduplication)

---

## Whitelist

Imported from `gravity.db` domainlist table (type=2, regex whitelist, enabled=1):

### Active Whitelists

1. **taskassist-pa.clients6.google.com**
   - **Purpose**: Keep Google Assistant working
   - **Pi-hole Pattern**: `(\.|^)taskassist-pa\.clients6\.google\.com$`
   - **Glory-Hole**: Converted to exact + wildcard match
   - **Imported**: ✅ Yes

2. **proxy.cloudflare-gateway.com**
   - **Purpose**: Keep Cloudflare WARP/Gateway working
   - **Pi-hole Pattern**: `(\.|^)proxy\.cloudflare-gateway\.com$`
   - **Glory-Hole**: Converted to exact + wildcard match
   - **Imported**: ✅ Yes

### Disabled Whitelists (Commented Out)

These were found in Pi-hole but disabled. They're included as comments in the config:

- nexusrules.officeapps.live.com (Office apps)
- upload.fp.measure.office.com (Office telemetry)
- asuracomic.net (Manga site)
- erfianugrah.com (Personal domain)
- google.com (Google main)
- mail.google.com (Gmail)
- passport.amazon.jobs (Amazon jobs portal)
- radarr.video (Media management)
- teams.microsoft.com (Microsoft Teams)
- www.gemini.com (Gemini crypto)

---

## Blacklist (Custom Block Rules)

Found in `gravity.db` domainlist table (type=3, regex blacklist, disabled):

### Disabled Blacklists (Commented Out in Policy Section)

- facebook.com
- instagram.com

**Note**: These are commented out in the policy engine section. Uncomment to enable.

---

## Upstream DNS

Imported from `pihole.toml` dns.upstreams:

- **Upstream**: `10.0.10.3:53`
- **Imported**: ✅ Yes

---

## Conditional Forwarding

Imported from `pihole.toml` dns.revServers:

### Active Rule

- **Network**: `10.0.0.0/8`
- **Forward To**: `10.0.69.1:53`
- **Domain**: `.local`
- **Purpose**: Forward local network queries to router for name resolution
- **Imported**: ✅ Yes (converted to 2 rules: local domains + reverse DNS)

---

## Local DNS Records

Checked `pihole.toml` dns.hosts:

- **Found**: None (empty array)
- **Imported**: N/A

---

## Cache Settings

Imported from `pihole.toml`:

| Setting | Pi-hole Value | Glory-Hole Value | Match |
|---------|--------------|------------------|-------|
| Cache Size | 10,000 entries | 10,000 entries | ✅ |
| Min TTL | 60s (optimizer) | 60s | ✅ |
| Max TTL | Default | 24h | ~ |
| Negative TTL | Default | 5m | ~ |

---

## Database Settings

Imported from `pihole.toml`:

| Setting | Pi-hole Value | Glory-Hole Value | Match |
|---------|--------------|------------------|-------|
| Retention | 91 days | 91 days | ✅ |
| WAL Mode | Enabled | Enabled | ✅ |
| Logging | Enabled | Enabled | ✅ |

---

## Special Domain Blocking

Pi-hole has built-in blocks for DNS bypass methods. These are replicated in Glory-Hole's policy engine:

1. **Mozilla DoH Canary** (`use-application-dns.net`)
   - Prevents Firefox from using DNS-over-HTTPS
   - ✅ Imported as policy rule

2. **iCloud Private Relay** (`mask.icloud.com`, `mask-h2.icloud.com`)
   - Prevents Apple devices from bypassing DNS
   - ✅ Imported as policy rule

---

## Network Settings

From `pihole.toml`:

| Setting | Pi-hole | Glory-Hole | Notes |
|---------|---------|------------|-------|
| Interface | eth0 | N/A | Not applicable (listens on all) |
| Listening Mode | SINGLE | ALL | Glory-Hole listens on all interfaces |
| Port | 53 | 53 | ✅ Same |
| Domain Needed | true | N/A | Handled by conditional forwarding |
| Bogus Priv | true | N/A | Handled by conditional forwarding |
| Rate Limiting | Disabled (0) | N/A | Not implemented yet |

---

## Not Imported (Not Supported Yet)

The following Pi-hole features are not yet supported in Glory-Hole:

### 1. Regex Support
- **Status**: 🚧 Planned for future version
- **Impact**: Regex whitelist/blacklist patterns converted to exact matches
- **Workaround**: Use wildcard patterns or policy engine with `DomainMatches()`

### 2. DHCP Server
- **Status**: ❌ Not planned
- **Impact**: None (use router's DHCP)

### 3. DNSSEC Validation
- **Status**: 🚧 Might be added later
- **Impact**: Upstream DNS (10.0.10.3) likely handles this

### 4. Rate Limiting
- **Status**: 🚧 Planned for future version
- **Impact**: No per-client rate limiting

### 5. Groups/Clients Management
- **Status**: 🚧 Planned for future version
- **Impact**: No per-client custom rules yet

---

## Migration Verification

After deploying Glory-Hole, verify these work:

### 1. Blocking Works
```bash
# Should be blocked (from blocklists)
dig @10.0.10.4 ads.doubleclick.net
dig @10.0.10.4 tracker.example.com
```

### 2. Whitelist Works
```bash
# Should NOT be blocked
dig @10.0.10.4 taskassist-pa.clients6.google.com
dig @10.0.10.4 proxy.cloudflare-gateway.com
```

### 3. Local Domain Resolution
```bash
# Should be forwarded to 10.0.69.1
dig @10.0.10.4 router.local
dig @10.0.10.4 somedevice.local
```

### 4. Regular Queries Work
```bash
# Should resolve normally via 10.0.10.3
dig @10.0.10.4 google.com
dig @10.0.10.4 github.com
```

---

## Differences from Pi-hole

### Advantages
- ✅ More flexible policy engine (time-based, IP-based, complex logic)
- ✅ Better observability (Prometheus metrics, structured logging)
- ✅ RESTful API for automation
- ✅ Conditional forwarding with priority system
- ✅ Duration-based kill switches (like Pi-hole's temporary disable)

### Limitations
- ❌ No regex support yet (converted to exact/wildcard)
- ❌ No web-based blocklist management yet
- ❌ No groups/clients management yet
- ❌ No DHCP server
- ❌ No query privacy levels

---

## Next Steps

1. **Deploy Glory-Hole** on VyOS (IP: 10.0.10.4)
2. **Test with one device** first before switching network
3. **Monitor logs** for any issues
4. **Compare statistics** with Pi-hole
5. **Keep Pi-hole running** as backup during testing

---

## v0.7.6 Preview: Automated Import Tool

Coming soon:
```bash
glory-hole import-pihole \
  --gravity-db=/path/to/gravity.db \
  --config=/path/to/pihole.toml \
  --output=config.yml

Features:
✅ Automatic blocklist import
✅ Whitelist/blacklist conversion
✅ Upstream DNS detection
✅ Conditional forwarding rules
✅ Settings migration
✅ Validation and warnings
```

This will automate what was done manually for this import.
