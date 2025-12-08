# Docker Deployment Guide

Complete guide for deploying Glory-Hole DNS Server with Docker.

## Prerequisites

- Docker 20.10+ installed
- Docker Compose 2.0+ (optional but recommended)
- 256MB RAM minimum
- 1GB disk space

## Quick Start

```bash
# Pull latest image
docker pull erfianugrah/gloryhole:latest

# Create config file
wget https://raw.githubusercontent.com/erfianugrah/gloryhole/main/config/config.example.yml -O config.yml

# Run container
docker run -d \
  --name glory-hole \
  -p 53:53/udp \
  -p 53:53/tcp \
  -p 8080:8080 \
  -v $(pwd)/config.yml:/etc/glory-hole/config.yml \
  -v glory-hole-data:/var/lib/glory-hole \
  erfianugrah/gloryhole:latest
```

## Building from Source

```bash
# Clone repository
git clone https://github.com/erfianugrah/gloryhole.git
cd glory-hole

# Build image
docker build -t glory-hole:latest .

# Or with build args
docker build \
  --build-arg VERSION=0.7.8 \
  --build-arg BUILD_TIME=$(date -u +"%Y-%m-%dT%H:%M:%SZ") \
  -t glory-hole:0.7.8 \
  .
```

## Docker Run Options

### Basic Deployment

```bash
docker run -d \
  --name glory-hole \
  --restart unless-stopped \
-p 53:53/udp \
-p 53:53/tcp \
-p 8080:8080 \
-p 9090:9090 \
  -v $(pwd)/config.yml:/etc/glory-hole/config.yml \
  -v glory-hole-data:/var/lib/glory-hole \
  erfianugrah/gloryhole:latest
```

**Port Mappings:**
- `53:53/udp` - DNS queries (UDP)
- `53:53/tcp` - DNS queries (TCP)
- `8080:8080` - Web UI and REST API
- `9090:9090` - Prometheus metrics

### Production Deployment

```bash
docker run -d \
  --name glory-hole \
  --restart always \
  --memory 512m \
  --cpus 1.0 \
  --log-driver json-file \
  --log-opt max-size=10m \
  --log-opt max-file=3 \
  -p 53:53/udp \
  -p 53:53/tcp \
  -p 8080:8080 \
  -p 9090:9090 \
  -v $(pwd)/config.yml:/etc/glory-hole/config.yml \
  -v glory-hole-data:/var/lib/glory-hole \
  -v glory-hole-logs:/var/log/glory-hole \
  -e TZ=America/New_York \
  --health-cmd="/usr/local/bin/glory-hole --health-check" \
  --health-interval=30s \
  --health-timeout=3s \
  --health-retries=3 \
  erfianugrah/gloryhole:latest
```

**Resource Limits:**
- `--memory 512m` - Limit memory to 512MB
- `--cpus 1.0` - Limit to 1 CPU core

**Logging:**
- `--log-driver json-file` - Use JSON file logging
- `--log-opt max-size=10m` - 10MB per log file
- `--log-opt max-file=3` - Keep 3 old files

> UI edits and policy changes only persist if the mounted `config.yml` is writable and passed with `--config /etc/glory-hole/config.yml` (default). Use `:ro` only when you intend read-only config.

## Docker Compose

### Basic Setup

Create `docker-compose.yml`:

```yaml
version: '3.8'

services:
  glory-hole:
    image: erfianugrah/gloryhole:latest
    container_name: glory-hole
    restart: unless-stopped
    
    ports:
      - "53:53/udp"
      - "53:53/tcp"
      - "8080:8080"
      - "9090:9090"
    
    volumes:
      - ./config.yml:/etc/glory-hole/config.yml:ro
      - glory-hole-data:/var/lib/glory-hole
    
    environment:
      - TZ=UTC
    
    healthcheck:
      test: ["/usr/local/bin/glory-hole", "--health-check"]
      interval: 30s
      timeout: 3s
      retries: 3

volumes:
  glory-hole-data:
```

**Start services:**
```bash
docker-compose up -d
```

### Complete Stack with Monitoring

```yaml
version: '3.8'

services:
  glory-hole:
    image: erfianugrah/gloryhole:latest
    container_name: glory-hole
    restart: unless-stopped
    ports:
      - "53:53/udp"
      - "53:53/tcp"
      - "8080:8080"
      - "9090:9090"
    volumes:
      - ./config.yml:/etc/glory-hole/config.yml:ro
      - glory-hole-data:/var/lib/glory-hole
    networks:
      - glory-hole-network
    healthcheck:
      test: ["/usr/local/bin/glory-hole", "--health-check"]
      interval: 30s
      timeout: 3s
      retries: 3

  prometheus:
    image: prom/prometheus:latest
    container_name: prometheus
    restart: unless-stopped
    ports:
      - "9091:9090"
    volumes:
      - ./deploy/prometheus/prometheus.yml:/etc/prometheus/prometheus.yml:ro
      - prometheus-data:/prometheus
    command:
      - '--config.file=/etc/prometheus/prometheus.yml'
      - '--storage.tsdb.retention.time=30d'
    networks:
      - glory-hole-network
    depends_on:
      - glory-hole

  grafana:
    image: grafana/grafana:latest
    container_name: grafana
    restart: unless-stopped
    ports:
      - "3000:3000"
    volumes:
      - grafana-data:/var/lib/grafana
      - ./deploy/grafana/provisioning:/etc/grafana/provisioning:ro
    environment:
      - GF_SECURITY_ADMIN_USER=admin
      - GF_SECURITY_ADMIN_PASSWORD=admin
    networks:
      - glory-hole-network
    depends_on:
      - prometheus

networks:
  glory-hole-network:
    driver: bridge

volumes:
  glory-hole-data:
  prometheus-data:
  grafana-data:
```

## Volume Management

### Named Volumes

Create named volumes:
```bash
docker volume create glory-hole-data
docker volume create glory-hole-logs
```

List volumes:
```bash
docker volume ls | grep glory-hole
```

Inspect volume:
```bash
docker volume inspect glory-hole-data
```

### Bind Mounts

Use specific host paths:

```bash
docker run -d \
  --name glory-hole \
  -v /opt/glory-hole/config.yml:/etc/glory-hole/config.yml:ro \
  -v /opt/glory-hole/data:/var/lib/glory-hole \
  -v /var/log/glory-hole:/var/log/glory-hole \
  erfianugrah/gloryhole:latest
```

**Advantages:**
- Direct access from host
- Easy to backup
- Known location

**Disadvantages:**
- Must manage permissions
- Less portable

### Backup and Restore

**Backup volume:**
```bash
docker run --rm \
  -v glory-hole-data:/data \
  -v $(pwd):/backup \
  alpine tar czf /backup/glory-hole-backup.tar.gz /data
```

**Restore volume:**
```bash
docker run --rm \
  -v glory-hole-data:/data \
  -v $(pwd):/backup \
  alpine tar xzf /backup/glory-hole-backup.tar.gz -C /
```

## Environment Variables

Override configuration via environment:

```bash
docker run -d \
  --name glory-hole \
  -e TZ=America/New_York \
  -e GLORY_HOLE_LOG_LEVEL=debug \
  erfianugrah/gloryhole:latest
```

**Supported variables:**
- `TZ` - Timezone
- `GLORY_HOLE_LOG_LEVEL` - Log level
- (More in future versions)

## Container Management

### Start/Stop

```bash
# Start
docker start glory-hole

# Stop (graceful shutdown)
docker stop glory-hole

# Restart
docker restart glory-hole

# Force kill
docker kill glory-hole
```

### View Logs

```bash
# Follow logs
docker logs -f glory-hole

# Last 100 lines
docker logs --tail 100 glory-hole

# Since timestamp
docker logs --since 2025-11-22T10:00:00 glory-hole

# With timestamps
docker logs -t glory-hole
```

### Execute Commands

```bash
# Open shell in container
docker exec -it glory-hole sh

# Run health check
docker exec glory-hole /usr/local/bin/glory-hole --health-check

# Check version
docker exec glory-hole /usr/local/bin/glory-hole --version
```

### Resource Usage

```bash
# Real-time stats
docker stats glory-hole

# Current resource usage
docker inspect glory-hole | jq '.[0].HostConfig'
```

## Networking

### Host Network Mode

Use host networking (no NAT overhead):

```bash
docker run -d \
  --name glory-hole \
  --network host \
  -v $(pwd)/config.yml:/etc/glory-hole/config.yml:ro \
  erfianugrah/gloryhole:latest
```

**Note:** Port mappings are ignored in host mode.

### Custom Bridge Network

Create custom network:

```bash
docker network create --driver bridge glory-hole-network

docker run -d \
  --name glory-hole \
  --network glory-hole-network \
  -p 53:53/udp -p 8080:8080 \
  erfianugrah/gloryhole:latest
```

### macvlan Network

Assign container its own MAC address:

```bash
docker network create -d macvlan \
  --subnet=192.168.1.0/24 \
  --gateway=192.168.1.1 \
  -o parent=eth0 \
  glory-hole-macvlan

docker run -d \
  --name glory-hole \
  --network glory-hole-macvlan \
  --ip 192.168.1.53 \
  erfianugrah/gloryhole:latest
```

## Health Checks

Built-in health check:

```dockerfile
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD ["/usr/local/bin/glory-hole", "--health-check"]
```

Check health status:

```bash
docker ps  # Look for (healthy) or (unhealthy)
docker inspect glory-hole | jq '.[0].State.Health'
```

## Security Considerations

### Run as Non-Root

Container runs as user `glory-hole` (UID 1000) by default.

```dockerfile
USER glory-hole
```

Port 53 binding is allowed via capabilities:
```dockerfile
RUN setcap 'cap_net_bind_service=+ep' /usr/local/bin/glory-hole
```

### Read-Only Root Filesystem

```bash
docker run -d \
  --name glory-hole \
  --read-only \
  --tmpfs /tmp:rw,noexec,nosuid,size=100m \
  -v glory-hole-data:/var/lib/glory-hole:rw \
  erfianugrah/gloryhole:latest
```

### Drop Capabilities

```bash
docker run -d \
  --name glory-hole \
  --cap-drop ALL \
  --cap-add NET_BIND_SERVICE \
  erfianugrah/gloryhole:latest
```

### Resource Limits

```bash
docker run -d \
  --name glory-hole \
  --memory 512m \
  --memory-reservation 256m \
  --cpus 1.0 \
  --cpu-shares 1024 \
  --pids-limit 100 \
  erfianugrah/gloryhole:latest
```

## Troubleshooting

### Container won't start

```bash
# Check logs
docker logs glory-hole

# Check events
docker events --filter container=glory-hole

# Inspect container
docker inspect glory-hole
```

### Port already in use

```bash
# Find process using port 53
sudo lsof -i :53
sudo netstat -tlnp | grep :53

# Stop conflicting service
sudo systemctl stop systemd-resolved
```

### Permission denied errors

```bash
# Check volume permissions
docker exec glory-hole ls -la /var/lib/glory-hole

# Fix permissions
docker exec -u root glory-hole chown -R glory-hole:glory-hole /var/lib/glory-hole
```

### Out of memory

```bash
# Check memory usage
docker stats glory-hole

# Increase memory limit
docker update --memory 1024m glory-hole
```

### DNS not resolving

```bash
# Test from host
docker exec glory-hole nslookup google.com localhost

# Check if listening
docker exec glory-hole netstat -tlnup

# Check firewall
sudo iptables -L -n | grep 53
```

## Updates and Upgrades

### Update to latest version

```bash
# Pull new image
docker pull erfianugrah/gloryhole:latest

# Stop and remove old container
docker stop glory-hole
docker rm glory-hole

# Run new container
docker run -d \
  --name glory-hole \
  -p 53:53/udp -p 53:53/tcp -p 8080:8080 \
  -v $(pwd)/config.yml:/etc/glory-hole/config.yml:ro \
  -v glory-hole-data:/var/lib/glory-hole \
  erfianugrah/gloryhole:latest

# Or with docker-compose
docker-compose pull
docker-compose up -d
```

### Rollback to previous version

```bash
# Pull specific version
docker pull erfianugrah/gloryhole:0.5.0

# Run with specific tag
docker run -d --name glory-hole erfianugrah/gloryhole:0.5.0
```

## Production Best Practices

1. **Use specific image tags** (not `latest`)
2. **Set resource limits** (memory, CPU)
3. **Configure log rotation** (max-size, max-file)
4. **Use health checks** (built-in flag)
5. **Enable restart policy** (`--restart unless-stopped`)
6. **Backup volumes regularly**
7. **Monitor with Prometheus/Grafana**
8. **Use secrets for sensitive data** (future)
9. **Run security scans** (`docker scan glory-hole`)
10. **Keep images updated**

## Example Production Setup

```bash
#!/bin/bash
# Production deployment script

# Configuration
IMAGE="erfianugrah/gloryhole:0.7.8"
CONTAINER_NAME="glory-hole"
CONFIG_PATH="/opt/glory-hole/config.yml"
DATA_PATH="/opt/glory-hole/data"

# Stop and remove existing container
docker stop $CONTAINER_NAME 2>/dev/null || true
docker rm $CONTAINER_NAME 2>/dev/null || true

# Pull latest image
docker pull $IMAGE

# Run container
docker run -d \
  --name $CONTAINER_NAME \
  --restart always \
  --memory 512m \
  --cpus 1.0 \
  --log-driver json-file \
  --log-opt max-size=10m \
  --log-opt max-file=3 \
  -p 53:53/udp \
  -p 53:53/tcp \
  -p 8080:8080 \
  -p 9090:9090 \
  -v $CONFIG_PATH:/etc/glory-hole/config.yml:ro \
  -v $DATA_PATH:/var/lib/glory-hole \
  -e TZ=America/New_York \
  --health-cmd="/usr/local/bin/glory-hole --health-check" \
  --health-interval=30s \
  --health-timeout=3s \
  --health-retries=3 \
  $IMAGE

# Wait for health check
echo "Waiting for container to become healthy..."
timeout 60 bash -c 'until docker inspect --format="{{.State.Health.Status}}" glory-hole | grep -q healthy; do sleep 1; done'

# Check status
if docker ps | grep -q $CONTAINER_NAME; then
  echo "✓ Glory-Hole DNS Server is running"
  docker ps | grep $CONTAINER_NAME
else
  echo "✗ Failed to start container"
  docker logs $CONTAINER_NAME
  exit 1
fi
```

Save as `deploy-docker.sh` and run:
```bash
chmod +x deploy-docker.sh
sudo ./deploy-docker.sh
```
