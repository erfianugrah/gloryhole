# Glory-Hole Deployment Guide

## VyOS 1.4 Container Deployment

### Prerequisites

1. **VyOS router** running version 1.4 or higher
2. **Podman network** configured (e.g., `podman-2`)
3. **Directory structure** on VyOS

### Setup Steps

#### 1. Create Directory Structure

```bash
# SSH into VyOS
ssh admin@your-vyos-router

# Create directories
mkdir -p /config/erfi/glory-hole/{etc,data,logs}
```

#### 2. Upload Configuration

Copy your personal config to VyOS:

```bash
# From your local machine
scp config/config.personal.yml admin@your-vyos-router:/config/erfi/glory-hole/etc/config.yml
```

#### 3. Set Permissions

```bash
# On VyOS
chown -R 1000:1000 /config/erfi/glory-hole
chmod 755 /config/erfi/glory-hole/{etc,data,logs}
chmod 644 /config/erfi/glory-hole/etc/config.yml
```

#### 4. Configure Container

```bash
# Enter configuration mode
configure

# Apply container configuration (from config/vyos-container.conf)
set container name glory-hole cap-add 'net-bind-service'
set container name glory-hole environment TZ value 'Europe/Amsterdam'
set container name glory-hole host-name 'glory-hole'
set container name glory-hole image 'ghcr.io/yourusername/glory-hole:latest'
set container name glory-hole memory '1024'
set container name glory-hole network podman-2 address '10.0.10.4'
set container name glory-hole restart 'always'
set container name glory-hole shared-memory '512'

# DNS ports
set container name glory-hole port dns-udp destination '53'
set container name glory-hole port dns-udp protocol 'udp'
set container name glory-hole port dns-udp source '53'
set container name glory-hole port dns-tcp destination '53'
set container name glory-hole port dns-tcp protocol 'tcp'
set container name glory-hole port dns-tcp source '53'

# Web UI port
set container name glory-hole port web-ui destination '8080'
set container name glory-hole port web-ui protocol 'tcp'
set container name glory-hole port web-ui source '8080'

# Prometheus metrics port
set container name glory-hole port prometheus destination '9090'
set container name glory-hole port prometheus protocol 'tcp'
set container name glory-hole port prometheus source '9090'

# Volume mounts
set container name glory-hole volume config destination '/etc/glory-hole'
set container name glory-hole volume config mode 'rw'
set container name glory-hole volume config source '/config/erfi/glory-hole/etc'

set container name glory-hole volume data destination '/var/lib/glory-hole'
set container name glory-hole volume data mode 'rw'
set container name glory-hole volume data source '/config/erfi/glory-hole/data'

set container name glory-hole volume logs destination '/var/log/glory-hole'
set container name glory-hole volume logs mode 'rw'
set container name glory-hole volume logs source '/config/erfi/glory-hole/logs'

set container name glory-hole volume localtime destination '/etc/localtime'
set container name glory-hole volume localtime mode 'ro'
set container name glory-hole volume localtime source '/etc/localtime'

# Commit and save
commit
save
```

#### 5. Verify Deployment

```bash
# Check container status
show container

# View container logs
show log container glory-hole

# Test DNS resolution
dig @10.0.10.4 example.com

# Access Web UI
# Open browser: http://10.0.10.4:8080
```

### Container Management

```bash
# Restart container
restart container glory-hole

# Stop container
configure
delete container name glory-hole
commit

# View logs
show log container glory-hole tail 100

# Enter container shell
container shell glory-hole
```

---

## Docker Compose Deployment

### Prerequisites

1. **Docker** and **Docker Compose** installed
2. **Configuration file** ready

### Setup Steps

#### 1. Prepare Configuration

```bash
# Copy personal config
cp config/config.personal.yml config.yml

# Or use the example config
cp config/config.example.yml config.yml

# Edit as needed
vim config.yml
```

#### 2. Build and Run

```bash
# Build the image
docker-compose build

# Start services (glory-hole only)
docker-compose up -d glory-hole

# Or start with monitoring stack (Prometheus + Grafana)
docker-compose up -d
```

#### 3. Verify

```bash
# Check status
docker-compose ps

# View logs
docker-compose logs -f glory-hole

# Test DNS
dig @localhost example.com

# Access Web UI
# Open browser: http://localhost:8080
```

### Management Commands

```bash
# Stop services
docker-compose stop

# Restart services
docker-compose restart glory-hole

# Update and rebuild
docker-compose build --no-cache
docker-compose up -d

# Remove everything
docker-compose down -v
```

---

## Docker Run (Standalone)

```bash
# Build image
docker build -t glory-hole:latest .

# Run container
docker run -d \
  --name glory-hole \
  --hostname glory-hole \
  -p 53:53/udp \
  -p 53:53/tcp \
  -p 8080:8080 \
  -p 9090:9090 \
  -v $(pwd)/config/config.personal.yml:/etc/glory-hole/config.yml:ro \
  -v glory-hole-data:/var/lib/glory-hole \
  -v glory-hole-logs:/var/log/glory-hole \
  -e TZ=Europe/Amsterdam \
  --cap-add NET_BIND_SERVICE \
  --restart always \
  glory-hole:latest
```

---

## Network Configuration

### Setting as Primary DNS

#### VyOS Configuration

```bash
configure

# Set glory-hole as DNS server for DHCP clients
set service dhcp-server shared-network-name LAN subnet 10.0.0.0/8 name-server 10.0.10.4

# Or set as system DNS (careful - this affects VyOS itself)
set system name-server 10.0.10.4

commit
save
```

#### Client Configuration

**Manual DNS (any device):**
- Primary DNS: `10.0.10.4`
- Secondary DNS: `10.0.10.2` (your Pi-hole) or `10.0.10.3` (unbound)

---

## Migration from Pi-hole

### Parallel Running (Recommended)

1. Deploy glory-hole on a different IP (e.g., `10.0.10.4`)
2. Test with a few devices first
3. Monitor logs and blocked queries
4. Gradually migrate more devices
5. Keep Pi-hole as backup

### Direct Replacement

1. Stop Pi-hole container
2. Deploy glory-hole on same IP (`10.0.10.2`)
3. Update firewall rules if needed
4. Test immediately

### Data Migration

Available in v0.7.7:
- Import Pi-hole blocklists from gravity.db
- Import whitelist/blacklist
- Import local DNS records
- Import upstream DNS configuration

---

## Troubleshooting

### Container Won't Start

```bash
# Check logs
show log container glory-hole

# Common issues:
# - Port 53 already in use (check with: ss -tulpn | grep :53)
# - Config file syntax error (validate YAML)
# - Insufficient permissions on volumes
```

### DNS Not Resolving

```bash
# Test from VyOS
dig @10.0.10.4 google.com

# Test from client
nslookup google.com 10.0.10.4

# Check upstream connectivity
# (from within container)
container shell glory-hole
dig @10.0.10.3 google.com
```

### Web UI Not Accessible

```bash
# Check if port is listening
ss -tulpn | grep 8080

# Test health endpoint
curl http://10.0.10.4:8080/health

# Check firewall rules on VyOS
show firewall
```

### High Memory Usage

```bash
# Check current usage
show container

# Adjust limits in config
configure
set container name glory-hole memory '2048'
commit
restart container glory-hole
```

---

## Performance Tuning

### Cache Size

Adjust in `config.yml`:
```yaml
cache:
  max_entries: 20000  # Increase for more caching
```

### Database Retention

```yaml
database:
  retention_days: 30  # Reduce for less disk usage
```

### Memory Limits

VyOS container config:
```bash
set container name glory-hole memory '2048'
set container name glory-hole shared-memory '1024'
```

---

## Monitoring

### Prometheus Metrics

Available at: `http://10.0.10.4:9090/metrics`

Key metrics:
- `dns_queries_total` - Total DNS queries
- `dns_blocked_queries_total` - Blocked queries
- `dns_cache_hits_total` - Cache hits
- `dns_cache_misses_total` - Cache misses
- `active_clients` - Current active clients

### Logs

```bash
# VyOS
show log container glory-hole tail 100

# Docker
docker-compose logs -f glory-hole

# Container
tail -f /config/erfi/glory-hole/logs/glory-hole.log
```

### Web UI

Access at: `http://10.0.10.4:8080`

Features:
- Dashboard with statistics
- Query log with filtering
- Top blocked/allowed domains
- Settings and configuration
- Kill-switch controls

---

## Backup and Restore

### Backup

```bash
# VyOS
tar czf glory-hole-backup-$(date +%Y%m%d).tar.gz \
  /config/erfi/glory-hole/

# Docker
docker run --rm \
  -v glory-hole-data:/data \
  -v $(pwd):/backup \
  alpine \
  tar czf /backup/glory-hole-data-$(date +%Y%m%d).tar.gz /data
```

### Restore

```bash
# VyOS
tar xzf glory-hole-backup-20250123.tar.gz -C /

# Docker
docker run --rm \
  -v glory-hole-data:/data \
  -v $(pwd):/backup \
  alpine \
  tar xzf /backup/glory-hole-data-20250123.tar.gz -C /
```

---

## Updates

### VyOS Container

```bash
configure

# Update image
set container name glory-hole image 'ghcr.io/yourusername/glory-hole:v0.7.7'

commit
restart container glory-hole
```

### Docker Compose

```bash
# Pull latest
docker-compose pull glory-hole

# Restart with new image
docker-compose up -d glory-hole
```
