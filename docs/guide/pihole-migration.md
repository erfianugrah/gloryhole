# Pi-hole Migration Guide

**Last Updated**: 2025-11-23
**Version**: 0.7.8

This guide helps you migrate from Pi-hole to Glory-Hole DNS Server, including importing your existing configuration, blocklists, and local DNS records.

---

## Table of Contents

- [Before You Start](#before-you-start)
- [Import Tool Overview](#import-tool-overview)
- [Backing Up Pi-hole Data](#backing-up-pi-hole-data)
- [Running the Import](#running-the-import)
- [What Gets Imported](#what-gets-imported)
- [Post-Import Steps](#post-import-steps)
- [Troubleshooting](#troubleshooting)
- [Rollback Plan](#rollback-plan)

---

## Before You Start

### Prerequisites

- Glory-Hole v0.7.8 or later
- Access to Pi-hole's `gravity.db` file
- Basic familiarity with command-line tools

### What You'll Need

1. **Pi-hole database**: `/etc/pihole/gravity.db`
2. **Backup of current Glory-Hole config** (if applicable)
3. **15-30 minutes** for the migration process

### Compatibility

The import tool supports:
- Pi-hole v5.0+
- Blocklists (gravity.db)
- Whitelist/blacklist entries
- Local DNS records (A, AAAA, CNAME)
- Upstream DNS configuration

---

## Import Tool Overview

Glory-Hole includes a command-line tool to import Pi-hole configuration:

```bash
glory-hole import pihole --help
```

**Features:**
- Imports blocklists from gravity.db
- Preserves whitelist and blacklist entries
- Migrates local DNS records
- Converts upstream DNS servers
- Generates Glory-Hole-compatible YAML config

---

## Backing Up Pi-hole Data

Before starting the migration, create backups of your Pi-hole data:

### 1. Backup Pi-hole Database

```bash
# On Pi-hole server
sudo cp /etc/pihole/gravity.db /tmp/gravity.db.backup

# Transfer to Glory-Hole server (if different machine)
scp /tmp/gravity.db.backup user@gloryhole-server:/tmp/
```

### 2. Backup Current Glory-Hole Config (if applicable)

```bash
# On Glory-Hole server
cp /etc/glory-hole/config.yml /etc/glory-hole/config.yml.backup
```

---

## Running the Import

### Basic Import

Import from Pi-hole's gravity.db:

```bash
glory-hole import pihole \
  --gravity-db /tmp/gravity.db.backup \
  --output /tmp/glory-hole-config.yml
```

### Advanced Import Options

```bash
glory-hole import pihole \
  --gravity-db /etc/pihole/gravity.db \
  --output /tmp/glory-hole-config.yml \
  --merge-with /etc/glory-hole/config.yml \
  --preserve-comments \
  --verbose
```

**Options:**

| Flag | Description | Default |
|------|-------------|---------|
| `--gravity-db` | Path to Pi-hole gravity.db | (required) |
| `--output` | Output YAML config file | stdout |
| `--merge-with` | Merge with existing config | none |
| `--preserve-comments` | Keep comments from existing config | false |
| `--verbose` | Show detailed import progress | false |

---

## What Gets Imported

### Blocklists

**From Pi-hole:**
```
- adlist table in gravity.db
- All enabled blocklist URLs
- Domain counts per list
```

**To Glory-Hole:**
```yaml
blocklists:
  - "https://raw.githubusercontent.com/StevenBlack/hosts/master/hosts"
  - "https://adguardteam.github.io/HostlistsRegistry/assets/filter_1.txt"
  # ... all your Pi-hole blocklists
```

### Whitelist / Blacklist

**From Pi-hole:**
```
- domainlist table (type = whitelist)
- domainlist table (type = blacklist)
```

**To Glory-Hole:**
```yaml
whitelist:
  - "analytics.google.com"
  - "github-cloud.s3.amazonaws.com"

blocklist:  # Additional static blocks
  - "manual-block-domain.com"
```

### Local DNS Records

**From Pi-hole:**
```
- custom.list (A/AAAA records)
- cname records from dnsmasq
```

**To Glory-Hole:**
```yaml
local_records:
  enabled: true
  records:
    - domain: "nas.local"
      type: "A"
      ips:
        - "192.168.1.100"

    - domain: "storage.local"
      type: "CNAME"
      target: "nas.local"
```

### Upstream DNS Servers

**From Pi-hole:**
```
- setupVars.conf (PIHOLE_DNS_1, PIHOLE_DNS_2, etc.)
```

**To Glory-Hole:**
```yaml
upstream_dns_servers:
  - "1.1.1.1:53"
  - "8.8.8.8:53"
```

---

## Post-Import Steps

### 1. Review Generated Config

```bash
# View generated config
cat /tmp/glory-hole-config.yml

# Check for any import warnings
grep -i "warning\|error" /tmp/glory-hole-import.log
```

### 2. Validate Configuration

```bash
# Test config syntax (if supported)
glory-hole --config /tmp/glory-hole-config.yml --validate

# Or do a dry-run
glory-hole --config /tmp/glory-hole-config.yml --dry-run
```

### 3. Deploy New Configuration

```bash
# Backup current config
sudo cp /etc/glory-hole/config.yml /etc/glory-hole/config.yml.backup

# Deploy imported config
sudo cp /tmp/glory-hole-config.yml /etc/glory-hole/config.yml

# Restart Glory-Hole
sudo systemctl restart glory-hole
```

### 4. Verify Operation

```bash
# Check server health
curl http://localhost:8080/api/health

# Verify blocklist loaded
curl http://localhost:8080/api/stats | jq '.blocked_domains'

# Test DNS resolution
dig @localhost google.com
dig @localhost nas.local  # Test local record
```

### 5. Update Client DNS Settings

Once verified, point your devices to Glory-Hole:

**Router-wide:**
- Log into router admin
- Change primary DNS to Glory-Hole server IP
- Save and reboot router

**Individual devices:**
- Update network settings
- Set DNS server to Glory-Hole IP
- Test with `dig` or `nslookup`

---

## Troubleshooting

### Import Failed: "Cannot open database"

**Problem**: Permission denied or file not found

**Solution:**
```bash
# Check file exists
ls -l /etc/pihole/gravity.db

# Fix permissions
sudo chmod 644 /etc/pihole/gravity.db

# Or copy to accessible location
sudo cp /etc/pihole/gravity.db /tmp/gravity.db
chmod 644 /tmp/gravity.db
```

### Import Warning: "Skipping invalid domain"

**Problem**: Some Pi-hole entries have invalid syntax

**Solution:**
- Check import log for specific domains
- Review and manually add if needed
- These are typically malformed entries that can be safely skipped

### Local DNS Records Not Working

**Problem**: Imported local records not resolving

**Solution:**
```bash
# Verify records in config
grep -A 10 "local_records:" /etc/glory-hole/config.yml

# Check if enabled
grep "enabled: true" /etc/glory-hole/config.yml

# Test resolution
dig @localhost nas.local
```

### Blocklists Not Loading

**Problem**: Import succeeded but domains not blocked

**Solution:**
```bash
# Check blocklist URLs are accessible
curl -I https://raw.githubusercontent.com/StevenBlack/hosts/master/hosts

# Force reload blocklists
curl -X POST http://localhost:8080/api/blocklist/reload

# Check logs
sudo journalctl -u glory-hole | grep -i blocklist
```

---

## Rollback Plan

If you need to revert to Pi-hole:

### 1. Revert DNS Settings

Point devices back to Pi-hole server IP.

### 2. Keep Both Running (Recommended)

Run Glory-Hole on different IP for testing:

```yaml
# Glory-Hole config
server:
  listen_address: "192.168.1.53:53"  # Different IP
  web_ui_address: ":8080"
```

Pi-hole continues on original IP (192.168.1.2:53).

### 3. Full Rollback

```bash
# Stop Glory-Hole
sudo systemctl stop glory-hole

# Restore Pi-hole DNS settings on router
# Point DNS back to Pi-hole IP

# Restart Pi-hole (if stopped)
pihole restartdns
```

---

## Migration Checklist

Use this checklist to track your migration:

- [ ] Backup Pi-hole gravity.db
- [ ] Backup current Glory-Hole config (if applicable)
- [ ] Run import tool
- [ ] Review generated config for accuracy
- [ ] Validate config syntax
- [ ] Test Glory-Hole with new config
- [ ] Verify DNS resolution works
- [ ] Verify blocklists loaded correctly
- [ ] Test local DNS records
- [ ] Update router DNS settings
- [ ] Monitor for 24-48 hours
- [ ] Decommission Pi-hole (optional)

---

## Comparison: Pi-hole vs Glory-Hole

### Feature Parity

| Feature | Pi-hole | Glory-Hole |
|---------|---------|------------|
| Ad/tracker blocking | ✅ | ✅ |
| Local DNS records | ✅ | ✅ (A, AAAA, CNAME, TXT, MX, SRV, CAA, PTR, NS, SOA) |
| Web UI | ✅ | ✅ |
| API | ✅ | ✅ |
| Query logging | ✅ | ✅ |
| Statistics | ✅ | ✅ |
| Blocklist management | ✅ | ✅ |
| Whitelist/blacklist | ✅ | ✅ |
| DHCP server | ✅ | ❌ (not needed) |
| Regex blocking | ✅ | ✅ (pattern matching) |
| Group management | ✅ | ✅ (via policies) |

### Glory-Hole Advantages

1. **Policy Engine**: Time-based, client-based, complex rules
2. **Performance**: Lock-free blocklist (8ns lookups vs ~100ns)
3. **Modern Architecture**: Go-based, single binary
4. **Pattern Matching**: Exact, wildcard, and regex support
5. **More DNS Record Types**: CAA, SRV, SOA, NS, PTR support
6. **Lightweight**: ~50MB RAM vs ~200MB for Pi-hole

### Pi-hole Advantages

1. **Mature ecosystem**: 7+ years of development
2. **Large community**: More support resources
3. **DHCP server**: Built-in DHCP functionality
4. **Web UI features**: More polished interface (currently)

---

## Getting Help

If you encounter issues during migration:

1. Check [Troubleshooting Guide](troubleshooting.md)
2. Review import logs: `/tmp/glory-hole-import.log`
3. Enable debug logging: `logging.level: "debug"` in config
4. Open issue: [GitHub Issues](https://github.com/erfianugrah/gloryhole/issues)

---

## Next Steps

After successful migration:

- [Usage Guide](usage.md) - Learn day-to-day operations
- [Policy Engine](../api/policy-engine.md) - Set up advanced filtering rules
- [Pattern Matching Guide](pattern-matching.md) - Use exact/wildcard/regex patterns
- [Monitoring Setup](../deployment/monitoring.md) - Set up Prometheus/Grafana

---

**Migration Complete!** 🎉

You've successfully migrated from Pi-hole to Glory-Hole. Enjoy improved performance and advanced policy-based filtering!
