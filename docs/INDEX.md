# Documentation Index

Complete guide to Glory-Hole DNS Server documentation.

---

## 📚 Quick Start

- **[Main README](../README.md)** - Project overview and quick start
- **[Installation Guide](guide/installation.md)** - Install and setup instructions
- **[Configuration Guide](guide/configuration.md)** - Complete configuration reference
- **[Contributing Guide](../CONTRIBUTING.md)** - How to contribute to the project

---

## 🎯 Current Release Documentation

### Performance & Optimization
- **[Final Summary](FINAL_SUMMARY.md)** - Complete v0.7.22 optimization summary
- **[Verification Checklist](VERIFICATION_CHECKLIST.md)** - Quality assurance checklist
- **[Performance Results](performance/OPTIMIZATION_RESULTS.md)** - Benchmark comparisons
- **[Performance Guide](performance/README.md)** - How to run benchmarks

### Core Documentation
- **[README](README.md)** - Documentation overview
- **[Roadmap](roadmap.md)** - Project roadmap and future plans
- **[Changelog](../CHANGELOG.md)** - Version history and changes

---

## 🏗️ Architecture & Design

### Architecture Documents
- **[Architecture Overview](architecture/README.md)** - System architecture
- **[DNS Handler Design](architecture/dns-handler.md)** - DNS query processing
- **[Cache Design](architecture/cache.md)** - Caching system architecture
- **[Kill Switch Design](kill-switch-design.md)** - Emergency kill switch design

### API Documentation
- **[API Overview](api/README.md)** - REST API documentation
- **[API Endpoints](api/endpoints.md)** - Complete endpoint reference
- **[WebSocket API](api/websocket.md)** - Real-time updates

---

## 📖 User Guides

### Configuration & Setup
- **[Configuration Guide](guide/configuration.md)** - Complete config reference
- **[Installation Guide](guide/installation.md)** - Installation instructions
- **[Blocklists Guide](guide/blocklists.md)** - Managing blocklists
- **[Policy Engine](guide/policy-engine.md)** - Advanced filtering rules
- **[Local Records](guide/local-records.md)** - Custom DNS records

### Deployment
- **[Docker Deployment](deployment/docker.md)** - Docker and Docker Compose
- **[Kubernetes Deployment](deployment/kubernetes.md)** - Kubernetes setup
- **[Cloudflare Workers](deployment/cloudflare-workers.md)** - Edge deployment
- **[Systemd Service](deployment/systemd.md)** - Linux service setup

---

## 🔧 Development

### Development Setup
- **[Development Setup](development/setup.md)** - Developer environment setup
- **[Testing Guide](development/testing.md)** - Running and writing tests

### Version Planning (Historical)
- **[v0.6.1 Plan](development/v0.6.1-plan.md)**
- **[v0.7.0 Conditional Forwarding](development/v0.7.0-conditional-forwarding-implementation.md)**
- **[v0.7.2 DNS Record Types](development/dns-record-types-design-v0.7.2.md)**
- **[v0.7.3 Technical Debt](development/v0.7.3-technical-debt-plan.md)**
- **[v0.7.4 Logging & Metrics](development/v0.7.4-logging-metrics-plan.md)**
- **[v0.7.8 DNSSEC Metrics](development/v0.7.8-dnssec-metrics-plan.md)**
- **[v0.7.8 Test Report](development/v0.7.8-test-report.md)**
- **[v0.8.0 Client Management](development/v0.8.0-client-management-plan.md)**

---

## 📊 Performance & Benchmarking

### Current Performance Work
- **[Optimization Results](performance/OPTIMIZATION_RESULTS.md)** - v0.7.22 improvements
- **[Performance Guide](performance/README.md)** - How to benchmark
- **[Baseline Benchmarks](performance/baseline_cache.txt)** - Pre-optimization data
- **[Phase 2 Benchmarks](performance/phase2_benchmarks.txt)** - Post-optimization data

### Archived Performance Docs
- **[Performance Roadmap](archive/PERFORMANCE-ROADMAP.md)** - Previous roadmap
- **[Phase 1 Results](archive/PHASE1-BENCHMARK-RESULTS.md)** - Earlier optimization work

---

## 📝 Reports & Analysis

- **[Reports Directory](reports/)** - Various analysis reports

---

## 🗂️ Directory Structure

```
docs/
├── INDEX.md                    # This file
├── README.md                   # Documentation overview
├── FINAL_SUMMARY.md            # Latest release summary
├── VERIFICATION_CHECKLIST.md   # QA checklist
│
├── api/                        # API documentation
├── architecture/               # Architecture documents
├── deployment/                 # Deployment guides
├── development/                # Development docs
├── guide/                      # User guides
├── performance/                # Performance docs
├── reports/                    # Analysis reports
└── archive/                    # Archived/superseded docs
```

---

## 🔍 Finding Documentation

### By Topic

**Installation & Setup**
- New users → [Installation Guide](guide/installation.md)
- Docker users → [Docker Deployment](deployment/docker.md)
- Kubernetes users → [Kubernetes Deployment](deployment/kubernetes.md)

**Configuration**
- Basic config → [Configuration Guide](guide/configuration.md)
- Blocklists → [Blocklists Guide](guide/blocklists.md)
- Advanced filtering → [Policy Engine](guide/policy-engine.md)
- Custom DNS → [Local Records](guide/local-records.md)

**Development**
- Contributing → [Contributing Guide](../CONTRIBUTING.md)
- Setting up dev environment → [Development Setup](development/setup.md)
- Writing tests → [Testing Guide](development/testing.md)

**Performance**
- Latest optimizations → [Final Summary](FINAL_SUMMARY.md)
- Benchmark results → [Performance Results](performance/OPTIMIZATION_RESULTS.md)
- Running benchmarks → [Performance Guide](performance/README.md)

**API & Integration**
- REST API → [API Overview](api/README.md)
- Real-time updates → [WebSocket API](api/websocket.md)

---

## 📢 Getting Help

1. Check this index for relevant documentation
2. Read the [README](../README.md) for project overview
3. See [CONTRIBUTING.md](../CONTRIBUTING.md) for contribution guidelines
4. Check [CHANGELOG.md](../CHANGELOG.md) for version history
5. Report issues on GitHub

---

**Last Updated**: 2025-11-25
**Version**: v0.7.22+
