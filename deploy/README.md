# Glory-Hole DNS Server - Deployment Files

This directory contains deployment configurations for various platforms.

## Directory Structure

```
deploy/
├── prometheus/           # Prometheus configuration
│   └── prometheus.yml
├── grafana/             # Grafana configuration
│   ├── provisioning/
│   │   ├── datasources/
│   │   │   └── prometheus.yml
│   │   └── dashboards/
│   │       └── default.yml
│   └── dashboards/       # Dashboard JSON files (to be created)
└── systemd/             # systemd service files
    ├── glory-hole.service
    └── install.sh
```

## Docker Deployment

### Using docker-compose (Recommended)

1. **Copy and edit config:**
   ```bash
   cp config.example.yml config.yml
   nano config.yml
   ```

2. **Start the stack:**
   ```bash
   docker-compose up -d
   ```

3. **View logs:**
   ```bash
   docker-compose logs -f glory-hole
   ```

4. **Stop the stack:**
   ```bash
   docker-compose down
   ```

### Services Included:
- **glory-hole**: DNS server (ports 53, 8080, 9090)
- **prometheus**: Metrics collection (port 9091)
- **grafana**: Metrics visualization (port 3000)

### Access Points:
- DNS Server: `localhost:53`
- API: `http://localhost:8080/api/health`
- Prometheus: `http://localhost:9091`
- Grafana: `http://localhost:3000` (admin/admin)

### Using Dockerfile directly

```bash
# Build image
docker build -t glory-hole:latest \
  --build-arg VERSION=1.0.0 \
  --build-arg BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  .

# Run container
docker run -d \
  --name glory-hole \
  -p 53:53/udp \
  -p 53:53/tcp \
  -p 8080:8080 \
  -p 9090:9090 \
  -v $(pwd)/config.yml:/etc/glory-hole/config.yml:ro \
  -v glory-hole-data:/var/lib/glory-hole \
  glory-hole:latest
```

---

## systemd Deployment (Linux)

For bare-metal Linux servers using systemd.

### Installation

1. **Navigate to systemd directory:**
   ```bash
   cd deploy/systemd
   ```

2. **Run installation script:**
   ```bash
   sudo ./install.sh
   ```

3. **Edit configuration:**
   ```bash
   sudo nano /etc/glory-hole/config.yml
   ```

4. **Enable and start service:**
   ```bash
   sudo systemctl enable glory-hole
   sudo systemctl start glory-hole
   ```

5. **Check status:**
   ```bash
   sudo systemctl status glory-hole
   sudo journalctl -u glory-hole -f
   ```

### Manual Installation

If you prefer manual installation:

```bash
# Create user
sudo useradd --system --user-group --no-create-home glory-hole

# Create directories
sudo mkdir -p /etc/glory-hole /var/lib/glory-hole /var/log/glory-hole
sudo chown -R glory-hole:glory-hole /etc/glory-hole /var/lib/glory-hole /var/log/glory-hole

# Copy binary
sudo cp ../../glory-hole /usr/local/bin/glory-hole
sudo chmod 755 /usr/local/bin/glory-hole
sudo setcap 'cap_net_bind_service=+ep' /usr/local/bin/glory-hole

# Copy config
sudo cp ../../config.example.yml /etc/glory-hole/config.yml
sudo chown glory-hole:glory-hole /etc/glory-hole/config.yml

# Install service
sudo cp glory-hole.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable glory-hole
sudo systemctl start glory-hole
```

### Service Management

```bash
# Start service
sudo systemctl start glory-hole

# Stop service
sudo systemctl stop glory-hole

# Restart service
sudo systemctl restart glory-hole

# Check status
sudo systemctl status glory-hole

# View logs
sudo journalctl -u glory-hole -f

# View last 100 lines
sudo journalctl -u glory-hole -n 100

# Disable service
sudo systemctl disable glory-hole
```

---

## Fly.io Deployment

Deploy Glory-Hole as a public/private anycast DNS resolver on Fly.io with full ad-blocking, DNS-over-TLS, DNS-over-HTTPS, and Unbound recursive resolution.

**What you get:** DNS (UDP/TCP), DoT (port 853), DoH (`/dns-query`), HTTPS dashboard — all on a single ~$5/mo machine.

### Prerequisites

- [flyctl](https://fly.io/docs/flyctl/install/) installed and authenticated (`fly auth login`)
- A Fly.io account with a payment method

### Setup

1. **Create the app and allocate resources (one-time):**
   ```bash
   fly apps create <your-app-name>
   fly ips allocate-v4                              # $2/mo, required for UDP DNS
   fly volumes create dns_data --size 1 --region ams # 1GB persistent volume
   ```

2. **Update `fly.toml`** with your app name:
   ```toml
   app = "<your-app-name>"
   ```

3. **Create `config.fly.yml`** (see [Configuration](#fly-configuration) below).

4. **Build and deploy:**
   ```bash
   make fly-deploy
   ```
   Or manually:
   ```bash
   docker build -f Dockerfile.fly -t <your-dockerhub-user>/glory-hole:fly .
   docker push <your-dockerhub-user>/glory-hole:fly
   fly deploy
   ```

5. **Add TLS certificate for dashboard/DoH domain:**
   ```bash
   fly certs add <your-doh-domain> -a <your-app-name>
   ```

6. **Create DNS records** (in your DNS provider):
   - `A` record: DoT hostname → Fly IPv4 (from `fly ips list`)
   - `AAAA` record: DoT hostname → Fly IPv6
   - `CNAME` record: DoH/dashboard hostname → `<your-app-name>.fly.dev`

#### Fly Configuration

Create `config.fly.yml` in the project root (gitignored — contains secrets):

```bash
cp config/config.example.yml config.fly.yml
```

Key settings to configure:
```yaml
server:
  udp_listen_address: "fly-global-services:53"  # Required for Fly.io UDP routing
  dot_enabled: true
  tls:
    acme:
      enabled: true
      dns_provider: cloudflare
      hosts:
        - "dns.yourdomain.com"
      cloudflare:
        api_token: "<your-cloudflare-api-token>"
        zone_id: "<your-zone-id>"

auth:
  enabled: true        # Always enable for internet-facing deployments
  api_key: "<your-api-key>"
  username: "<your-username>"
  password_hash: "<bcrypt-hash>"  # Generate with: glory-hole hash-password "yourpass"

unbound:
  enabled: true
  managed: true

logging:
  format: json          # Structured logs for Fly.io log drains

upstream_dns_servers: [] # Unused when Unbound is enabled
```

The config is baked into the Docker image by `Dockerfile.fly`. Rebuild and redeploy to change config.

#### Files

| File | Tracked | Purpose |
|------|---------|---------|
| `fly.toml` | yes | Fly.io service definitions (ports, VM size, volume mount) |
| `Dockerfile.fly` | yes | Full build + bakes `config.fly.yml` into image |
| `config.fly.yml` | no (gitignored) | Fly-specific config with secrets |

### How It Works

| Protocol | Port | How |
|----------|------|-----|
| DNS (UDP/TCP) | 53 | Fly.io dedicated IPv4 anycast. UDP binds to `fly-global-services`, TCP to `0.0.0.0` — handled by `udp_listen_address` config. |
| DNS-over-TLS | 853 | Glory-Hole terminates TLS natively using ACME DNS-01 certs via Cloudflare. |
| DNS-over-HTTPS | 443 | Fly.io terminates HTTPS, proxies to Glory-Hole's `/dns-query` endpoint on port 8080. |
| Dashboard | 443 | Same HTTPS service as DoH. |

### Architecture

```
Client ──UDP/53──► Fly Anycast ──► Glory-Hole ──► Unbound (localhost:5353)
Client ──DoT/853─► Fly TCP ──────► Glory-Hole (TLS) ──► Unbound
Client ──DoH/443─► Fly HTTPS ────► Glory-Hole /dns-query ──► Unbound
```

### Per-Protocol Listen Addresses

Fly.io requires UDP services to bind to the special hostname `fly-global-services` while TCP must bind to `0.0.0.0`. Glory-Hole supports this via `udp_listen_address` and `tcp_listen_address` config overrides:

```yaml
server:
  listen_address: ":53"                          # Default for TCP
  udp_listen_address: "fly-global-services:53"   # Override for UDP only
```

These are generic config options — not Fly-specific code. They work for any split-bind scenario.

### Cost Estimate

| Resource | Cost/mo |
|----------|---------|
| shared-cpu-1x 512MB | $3.19 |
| Dedicated IPv4 | $2.00 |
| 1GB Volume | $0.15 |
| **Total** | **~$5.34** |

Add ~$3.34 per additional region for multi-region anycast.

### Multi-Region

```bash
fly scale count 2 --region ams,fra
# Create a volume in each region (primary already has one)
fly volumes create dns_data --size 1 --region fra
```

Each region gets its own Unbound cache and SQLite database. Fly's anycast routes clients to the nearest region automatically.

### Troubleshooting

**UDP not working:**
- Verify you have a dedicated IPv4: `fly ips list`
- Check that `udp_listen_address: "fly-global-services:53"` is set in config
- Ensure `EXPOSE 53/udp` is in the Dockerfile

**DoT certificate issues:**
- Check logs: `fly logs`
- Verify Cloudflare API token has Zone:DNS:Edit permissions
- ACME cache is on the persistent volume at `/var/lib/glory-hole/.cache/acme/`

**Memory pressure:**
- Default VM is 512MB. Unbound + Go + blocklists use ~250-350MB.
- `GOMEMLIMIT=384MiB` is set in `fly.toml` to prevent Go from over-allocating.
- For large blocklists, consider upgrading to 1GB: update `fly.toml` memory to `1024mb`.

---

## Kubernetes Deployment

Kubernetes manifests coming soon. For now, you can use the Docker image with standard Kubernetes resources:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: glory-hole
spec:
  replicas: 1
  selector:
    matchLabels:
      app: glory-hole
  template:
    metadata:
      labels:
        app: glory-hole
    spec:
      containers:
      - name: glory-hole
        image: glory-hole:latest
        ports:
        - containerPort: 53
          protocol: UDP
        - containerPort: 53
          protocol: TCP
        - containerPort: 8080
        - containerPort: 9090
        volumeMounts:
        - name: config
          mountPath: /etc/glory-hole
        livenessProbe:
          exec:
            command:
            - /usr/local/bin/glory-hole
            - --health-check
          initialDelaySeconds: 10
          periodSeconds: 30
        readinessProbe:
          exec:
            command:
            - /usr/local/bin/glory-hole
            - --health-check
          initialDelaySeconds: 5
          periodSeconds: 10
      volumes:
      - name: config
        configMap:
          name: glory-hole-config
```

---

## Monitoring Setup

### Prometheus

Prometheus is configured to scrape metrics from glory-hole every 10 seconds.

**Metrics endpoint:** `http://localhost:9090/metrics`

### Grafana

Grafana is pre-configured with Prometheus as a datasource.

**Default credentials:** admin/admin (change on first login)

**Dashboard creation:**
1. Access Grafana at `http://localhost:3000`
2. Navigate to Dashboards → New Dashboard
3. Add panels for DNS metrics
4. Save dashboard JSON to `deploy/grafana/dashboards/`

---

## Security Considerations

### Port 53 Binding

Port 53 is a privileged port (< 1024). We handle this using:

**Docker:**
- `setcap` in Dockerfile grants `CAP_NET_BIND_SERVICE` to the binary
- Container runs as non-root user (uid 1000)

**systemd:**
- `AmbientCapabilities=CAP_NET_BIND_SERVICE` in service file
- Service runs as non-root user `glory-hole`

### Firewall Configuration

If using `ufw`:

```bash
# Allow DNS
sudo ufw allow 53/udp
sudo ufw allow 53/tcp

# Allow API (restrict to localhost)
sudo ufw allow from 127.0.0.1 to any port 8080

# Allow Prometheus metrics (restrict to monitoring server)
sudo ufw allow from MONITORING_IP to any port 9090
```

If using `firewalld`:

```bash
sudo firewall-cmd --permanent --add-service=dns
sudo firewall-cmd --permanent --add-port=8080/tcp
sudo firewall-cmd --permanent --add-port=9090/tcp
sudo firewall-cmd --reload
```

---

## Troubleshooting

### Docker: Permission denied on port 53

**Problem:** Container can't bind to port 53

**Solution:**
1. Ensure `setcap` ran successfully in Dockerfile
2. Try running container as root (not recommended):
   ```bash
   docker run --user root ...
   ```

### systemd: Service fails to start

**Check logs:**
```bash
sudo journalctl -u glory-hole -xe
```

**Common issues:**
1. Binary not found: Check `/usr/local/bin/glory-hole` exists
2. Config error: Validate `/etc/glory-hole/config.yml`
3. Port 53 in use: Check `sudo netstat -tulpn | grep :53`
4. Capability missing: Run `sudo setcap 'cap_net_bind_service=+ep' /usr/local/bin/glory-hole`

### Health check failing

Test manually:
```bash
/usr/local/bin/glory-hole --health-check
echo $?  # Should be 0 if healthy
```

---

## Environment Variables

### Docker Compose

Create a `.env` file in the project root:

```bash
# Version tag
VERSION=1.0.0

# Timezone
TZ=America/New_York

# Grafana credentials
GF_ADMIN_USER=admin
GF_ADMIN_PASSWORD=secure_password_here
```

---

## Next Steps

After successful deployment:

1.  Verify DNS is working: `dig @localhost example.com`
2.  Check API health: `curl http://localhost:8080/api/health`
3.  View metrics: `curl http://localhost:9090/metrics`
4.  Set up Grafana dashboards
5.  Configure your devices to use the DNS server
6.  Monitor query logs and blocked domains

---

## Support

- **Documentation:** See `docs/` directory
- **Issues:** Report on GitHub
- **Examples:** See `examples/` directory for config examples
