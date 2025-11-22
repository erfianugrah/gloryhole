# Getting Started with Glory-Hole DNS Server

This guide will help you get Glory-Hole DNS Server up and running in under 5 minutes.

## Table of Contents

- [Prerequisites](#prerequisites)
- [Installation Methods](#installation-methods)
  - [From Binary](#from-binary)
  - [Using Docker](#using-docker)
  - [Using Kubernetes](#using-kubernetes)
  - [From Source](#from-source)
- [Quick Start](#quick-start)
- [First DNS Query Test](#first-dns-query-test)
- [Accessing the Web UI](#accessing-the-web-ui)
- [Next Steps](#next-steps)

## Prerequisites

### System Requirements

- **Operating System**: Linux, macOS, or Windows
- **Memory**: Minimum 128MB RAM (recommended 512MB+)
- **Disk Space**: 100MB for binary + 50MB for blocklists + database
- **Network**: Port 53 (DNS) and 8080 (Web UI) available

### Optional Requirements

- **Go 1.24+** - Only needed if building from source
- **Docker 20.10+** - For Docker deployment
- **Kubernetes 1.20+** - For Kubernetes deployment

## Installation Methods

### From Binary

Download the latest release for your platform:

```bash
# Linux (amd64)
wget https://github.com/erfianugrah/gloryhole/releases/latest/download/glory-hole-linux-amd64
chmod +x glory-hole-linux-amd64
sudo mv glory-hole-linux-amd64 /usr/local/bin/glory-hole

# macOS (amd64)
wget https://github.com/erfianugrah/gloryhole/releases/latest/download/glory-hole-darwin-amd64
chmod +x glory-hole-darwin-amd64
sudo mv glory-hole-darwin-amd64 /usr/local/bin/glory-hole

# macOS (arm64 / Apple Silicon)
wget https://github.com/erfianugrah/gloryhole/releases/latest/download/glory-hole-darwin-arm64
chmod +x glory-hole-darwin-arm64
sudo mv glory-hole-darwin-arm64 /usr/local/bin/glory-hole
```

Verify the installation:

```bash
glory-hole --version
```

### Using Docker

Pull and run the official Docker image:

```bash
# Pull the image
docker pull erfianugrah/gloryhole:latest

# Create a config file
wget https://raw.githubusercontent.com/erfianugrah/gloryhole/main/config/config.example.yml -O config.yml

# Run the container
docker run -d \
  --name glory-hole \
  -p 53:53/udp \
  -p 53:53/tcp \
  -p 8080:8080 \
  -v $(pwd)/config.yml:/etc/glory-hole/config.yml:ro \
  -v glory-hole-data:/var/lib/glory-hole \
  erfianugrah/gloryhole:latest
```

Check the container status:

```bash
docker ps | grep glory-hole
docker logs glory-hole
```

### Using Kubernetes

Deploy to Kubernetes using the provided manifests:

```bash
# Clone the repository
git clone https://github.com/erfianugrah/gloryhole.git
cd glory-hole

# Apply Kubernetes manifests
kubectl apply -f deploy/kubernetes/

# Check deployment status
kubectl get pods -l app=glory-hole
kubectl logs -l app=glory-hole
```

### From Source

Build from source if you want the latest development version:

```bash
# Install Go 1.24 or later
# https://golang.org/doc/install

# Clone the repository
git clone https://github.com/erfianugrah/gloryhole.git
cd glory-hole

# Build the binary
go build -o glory-hole ./cmd/glory-hole

# Optionally, install system-wide
sudo mv glory-hole /usr/local/bin/

# Verify installation
glory-hole --version
```

## Quick Start

Follow these 3 steps to get Glory-Hole running:

### Step 1: Create Configuration File

Create a basic `config.yml`:

```yaml
server:
  listen_address: ":53"
  tcp_enabled: true
  udp_enabled: true
  web_ui_address: ":8080"

upstream_dns_servers:
  - "1.1.1.1:53"
  - "8.8.8.8:53"

blocklists:
  - "https://raw.githubusercontent.com/StevenBlack/hosts/master/hosts"

cache:
  enabled: true
  max_entries: 10000

database:
  enabled: true
  backend: "sqlite"
  sqlite:
    path: "./glory-hole.db"

logging:
  level: "info"
  format: "text"
  output: "stdout"

telemetry:
  enabled: true
  prometheus_enabled: true
  prometheus_port: 9090
```

For a complete configuration example, see [config.example.yml](../../config/config.example.yml).

### Step 2: Start the Server

```bash
# Run as root or with CAP_NET_BIND_SERVICE capability (needed for port 53)
sudo glory-hole -config config.yml
```

Or without sudo if using a non-privileged port:

```yaml
server:
  listen_address: ":5353"  # Non-privileged port
```

```bash
glory-hole -config config.yml
```

You should see output similar to:

```
INFO Glory Hole DNS starting version=0.7.1
INFO Initializing blocklist manager sources=1
INFO Blocklist manager started domains=101348
INFO Storage initialized successfully backend=sqlite
INFO Glory Hole DNS server is running dns_address=:53 api_address=:8080
```

### Step 3: Configure Your Device

Point your device's DNS settings to Glory-Hole:

**Linux/macOS:**
```bash
# Temporary (until reboot)
sudo networksetup -setdnsservers Wi-Fi 127.0.0.1

# Or edit /etc/resolv.conf
echo "nameserver 127.0.0.1" | sudo tee /etc/resolv.conf
```

**Windows:**
```powershell
# Open Network Connections
# Right-click your network adapter → Properties
# Select "Internet Protocol Version 4 (TCP/IPv4)" → Properties
# Enter 127.0.0.1 as the preferred DNS server
```

**Router-wide (recommended):**
- Log into your router's admin panel
- Find DNS settings (usually under DHCP or WAN settings)
- Set primary DNS to Glory-Hole server's IP address
- All devices on your network will now use Glory-Hole

## First DNS Query Test

Verify Glory-Hole is working:

### Test 1: Basic DNS Resolution

```bash
# Test allowed domain
dig @localhost google.com

# Or using nslookup
nslookup google.com localhost

# Expected output: Should return IP address
```

### Test 2: Blocked Domain

```bash
# Test blocked domain (ad/tracking domain)
dig @localhost doubleclick.net

# Expected output: NXDOMAIN (blocked)
```

### Test 3: Query via Web API

```bash
# Get recent queries
curl http://localhost:8080/api/queries?limit=10

# Get statistics
curl http://localhost:8080/api/stats
```

### Test 4: Health Check

```bash
# Check server health
curl http://localhost:8080/api/health

# Expected output:
# {
#   "status": "ok",
#   "uptime": "5m30s",
#   "version": "0.7.1"
# }
```

## Accessing the Web UI

Open your browser and navigate to:

```
http://localhost:8080
```

The Web UI provides:

- **Dashboard** - Real-time statistics and query activity charts
- **Query Log** - Live view of DNS queries with filtering
- **Policies** - Manage filtering rules
- **Settings** - View configuration and trigger blocklist reload

Default pages:
- Dashboard: `http://localhost:8080/`
- Queries: `http://localhost:8080/queries`
- Policies: `http://localhost:8080/policies`
- Settings: `http://localhost:8080/settings`

## Next Steps

Now that Glory-Hole is running, explore these topics:

### Configuration

- [Complete Configuration Guide](configuration.md) - All configuration options
- [Local DNS Records](configuration.md#local-dns-records) - Custom A/AAAA/CNAME records
- [Policy Engine](configuration.md#policy-engine) - Advanced filtering rules
- [Blocklist Configuration](configuration.md#blocklists) - Multiple sources and auto-updates

### Usage

- [Usage Guide](usage.md) - Day-to-day operations
- [Managing Policies](usage.md#managing-policies) - Create time-based and client-based rules
- [Viewing Statistics](usage.md#viewing-statistics) - Monitor DNS activity
- [Updating Blocklists](usage.md#updating-blocklists) - Manual and automatic updates

### Deployment

- [Docker Deployment](../deployment/docker.md) - Production Docker setup
- [Kubernetes Deployment](../deployment/kubernetes.md) - K8s manifests and scaling
- [Systemd Service](../deployment/systemd.md) - Run as a system service
- [Monitoring Setup](../deployment/monitoring.md) - Prometheus and Grafana

### Troubleshooting

- [Troubleshooting Guide](troubleshooting.md) - Common issues and solutions
- [Performance Tuning](troubleshooting.md#performance-issues) - Optimize for your workload
- [Debugging](troubleshooting.md#debugging) - Enable debug logging and traces

### API and Integration

- [REST API Reference](../api/rest-api.md) - Complete API documentation
- [Web UI Guide](../api/web-ui.md) - Using the web interface
- [Prometheus Metrics](../deployment/monitoring.md#prometheus-metrics) - Available metrics

## Common First-Time Issues

### Port 53 Permission Denied

**Problem**: `bind: permission denied` when starting on port 53

**Solution**: Either run as root or grant the binary capability:

```bash
# Option 1: Run as root
sudo glory-hole -config config.yml

# Option 2: Grant capability (Linux only)
sudo setcap 'cap_net_bind_service=+ep' /usr/local/bin/glory-hole
glory-hole -config config.yml

# Option 3: Use non-privileged port
# Edit config.yml: listen_address: ":5353"
```

### Blocklists Not Loading

**Problem**: Server starts but blocklists show 0 domains

**Solution**: Check internet connectivity and blocklist URLs:

```bash
# Test connectivity
curl https://raw.githubusercontent.com/StevenBlack/hosts/master/hosts

# Check logs for errors
glory-hole -config config.yml 2>&1 | grep -i blocklist
```

### Web UI Not Accessible

**Problem**: Cannot access `http://localhost:8080`

**Solution**: Verify the API server is running:

```bash
# Check if port is listening
netstat -tlnp | grep 8080
# or
lsof -i :8080

# Check firewall rules
sudo ufw status | grep 8080  # Ubuntu/Debian
sudo firewall-cmd --list-ports  # CentOS/RHEL
```

## Getting Help

If you encounter issues:

1. Check the [Troubleshooting Guide](troubleshooting.md)
2. Review server logs: `journalctl -u glory-hole -f` (systemd) or `docker logs glory-hole` (Docker)
3. Enable debug logging: Set `logging.level: "debug"` in config.yml
4. Open an issue on [GitHub](https://github.com/erfianugrah/gloryhole/issues)
5. Join our [Discord community](https://discord.gg/glory-hole)

## Quick Reference Card

```bash
# Start server
sudo glory-hole -config config.yml

# Check version
glory-hole --version

# Health check
glory-hole --health-check

# View logs (systemd)
sudo journalctl -u glory-hole -f

# Reload blocklists
curl -X POST http://localhost:8080/api/blocklist/reload

# Test DNS query
dig @localhost example.com

# Access Web UI
open http://localhost:8080
```

Congratulations! You now have Glory-Hole DNS Server running. Explore the guides above to unlock advanced features like policy-based filtering, local DNS records, and monitoring.
