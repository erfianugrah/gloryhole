# Kubernetes Deployment for Glory-Hole DNS

This directory contains Kubernetes manifests for deploying Glory-Hole DNS Server.

## Quick Start

### Prerequisites

- Kubernetes cluster (v1.24+)
- `kubectl` configured to access your cluster
- (Optional) LoadBalancer support for DNS service
- (Optional) Ingress controller for Web UI

### Deploy

```bash
# Apply all manifests
kubectl apply -f deploy/kubernetes/

# Check deployment status
kubectl get pods -l app=glory-hole
kubectl get svc

# View logs
kubectl logs -l app=glory-hole -f
```

### Configuration

Edit `configmap.yaml` to customize your configuration before deploying.

### Access

**DNS Service:**
```bash
# Get DNS service external IP
kubectl get svc glory-hole-dns

# Test DNS
dig @<EXTERNAL-IP> google.com
```

**Web UI:**
```bash
# Port forward to access locally
kubectl port-forward svc/glory-hole-ui 8080:80

# Access at http://localhost:8080
```

**Metrics:**
```bash
# Port forward Prometheus metrics
kubectl port-forward svc/glory-hole-metrics 9090:9090

# Access at http://localhost:9090/metrics
```

## Components

- `deployment.yaml` - Glory-Hole deployment with 2 replicas
- `service.yaml` - Services for DNS, UI, and metrics
- `configmap.yaml` - Configuration file
- `pvc.yaml` - Persistent volume for query logs
- `ingress.yaml` - Ingress for Web UI (optional)

## Scaling

```bash
# Scale replicas
kubectl scale deployment glory-hole --replicas=3

# Horizontal autoscaling
kubectl autoscale deployment glory-hole --cpu-percent=70 --min=2 --max=10
```

## Monitoring

The deployment includes:
- Liveness probe on `/health`
- Readiness probe on `/api/health`
- Prometheus metrics on port 9090

## Security

- Runs as non-root user (UID 1000)
- Read-only config mount
- Resource limits configured
- NetworkPolicies recommended (not included)

## Troubleshooting

```bash
# Check pod status
kubectl describe pod -l app=glory-hole

# View events
kubectl get events --sort-by='.lastTimestamp'

# Check logs
kubectl logs -l app=glory-hole --tail=100

# Debug DNS
kubectl run -it --rm debug --image=busybox --restart=Never -- nslookup google.com <POD-IP>
```
