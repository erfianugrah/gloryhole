# Documentation Index

Complete, accurate map of every Markdown/text asset that ships with Glory-Hole.

---

## 📚 Quick Start

- **[Project README](../README.md)** – Overview, build/run instructions, and architecture summary
- **[Getting Started](guide/getting-started.md)** – Installation plus first successful DNS queries
- **[Configuration Reference](guide/configuration.md)** – Exhaustive YAML options with examples
- **[Usage Guide](guide/usage.md)** – Everyday operations and workflows
- **[Contributing Guide](../CONTRIBUTING.md)** – Branching, coding standards, and review policy

---

## 🎯 Release Documentation

- **[Documentation Overview](README.md)** – How the docs are structured
- **[Final Summary](FINAL_SUMMARY.md)** – v0.7.22 optimization deliverables
- **[Verification Checklist](VERIFICATION_CHECKLIST.md)** – QA verification status
- **[Roadmap](roadmap.md)** – Active and upcoming epics
- **[Changelog](../CHANGELOG.md)** – Full release history

---

## 🏗️ Architecture & Design

- **[System Overview](architecture/overview.md)** – End-to-end architecture map
- **[Component Details](architecture/components.md)** – Deep dives per subsystem
- **[Design Decisions](architecture/design-decisions.md)** – ADR-style rationale
- **[Performance Architecture](architecture/performance.md)** – High-level perf model
- **[Design Library](designs/README.md)** – Feature-level architecture docs (kill-switch, conditional forwarding, metrics, DNSSEC, etc.)

---

## 🧬 Feature Design Library

- **[Kill Switch Design](designs/kill-switch-design.md)** – Runtime kill-switch architecture, API/UI integration, and metrics
- **[Conditional Forwarding Plan](designs/conditional-forwarding-plan.md)** – Requirements, data model, evaluation order, deployment patterns
- **[Conditional Forwarding Implementation](designs/v0.7.0-conditional-forwarding-implementation.md)** – File-by-file implementation log, testing, and migration guide
- **[DNS Record Types Design](designs/dns-record-types-design-v0.7.2.md)** – Expanded authoritative record handling
- **[Logging & Metrics Plan](designs/v0.7.4-logging-metrics-plan.md)** – Structured logging + telemetry improvements with kill-switch metrics
- **[DNSSEC Metrics Plan](designs/v0.7.8-dnssec-metrics-plan.md)** – DNSSEC telemetry architecture and validation workflow

---

## 📖 User Guides

- **[Getting Started](guide/getting-started.md)** – Install, bootstrap, and smoke tests
- **[Configuration](guide/configuration.md)** – Settings, schema, and examples
- **[Usage](guide/usage.md)** – Web UI, CLI, and automation flows
- **[Pi-hole Migration](guide/pihole-migration.md)** – Import blocklists and local records
- **[Pattern Matching](guide/pattern-matching.md)** – Exact, wildcard, and regex behaviour
- **[Performance Tuning](guide/performance-tuning.md)** – Cache, rate limit, telemetry knobs
- **[Troubleshooting](guide/troubleshooting.md)** – Common failures and fixes

---

## 🚀 Deployment & Operations

- **[Docker Deployment](deployment/docker.md)** – Compose stack and images
- **[VyOS + Docker](deployment/vyos-docker-guide.md)** – On-router container workflows
- **[Cloudflare D1](deployment/cloudflare-d1.md)** – Edge deployment steps
- **[Monitoring](deployment/monitoring.md)** – Prometheus, Grafana, and alerting
- **[Kubernetes Manifests](../deploy/kubernetes/README.md)** – Cluster deployment reference
- **[Deploy Directory](../deploy/README.md)** – Additional manifests and automation

---

## 🔌 API & UI

- **[REST API Reference](api/rest-api.md)** – Endpoints, payloads, and examples
- **[Policy Engine API](api/policy-engine.md)** – Rule schema and lifecycle hooks
- **[Web UI Guide](api/web-ui.md)** – HTMX flows and operator tooling

---

## 🧑‍💻 Development Guide

- **[Development Setup](development/setup.md)** – Toolchain, dependencies, and local config
- **[Testing Guide](development/testing.md)** – Running unit/integration/benchmark suites
- **[Makefile Targets](../Makefile)** – Reference for common automation
- **[Contribution Process](../CONTRIBUTING.md)** – Reviews, CI, and release expectations

---

## 📊 Performance & Benchmarking

- **[Benchmark Guide](performance/README.md)** – How to execute perf suites locally
- **[Optimization Results](performance/OPTIMIZATION_RESULTS.md)** – Detailed before/after analysis
- **Raw benchmark datasets**: `../working-docs/performance-data/baseline_cache.txt`, `baseline_load.txt`, and `phase2_benchmarks.txt`
- **Historical performance docs**: see `../working-docs/archive/`

---

## 🧱 Working & Implementation Docs

All in-depth implementation notes, reports, and historical data now live under [`../working-docs/README.md`](../working-docs/README.md):

- `working-docs/plans/` – Active release plans and backlog/roadmap artifacts
- `working-docs/reports/` – Validation, QA, and cleanup reports
- `working-docs/performance-data/` – Raw benchmark datasets
- `working-docs/archive/` – Superseded or historical documents

---

## 🗂️ Directory Overview

```
docs/
├── README.md
├── INDEX.md
├── FINAL_SUMMARY.md
├── VERIFICATION_CHECKLIST.md
├── roadmap.md
├── api/
├── architecture/
├── designs/
├── deployment/
├── development/
├── guide/
└── performance/

working-docs/
├── README.md
├── plans/
├── reports/
├── performance-data/
└── archive/
```

---

## 🔍 Finding Documentation

1. Start with this index or `docs/README.md`.
2. Need architecture? Jump to `docs/architecture/overview.md`.
3. Need deployment help? See `docs/deployment/docker.md` or `../deploy/kubernetes/README.md`.
4. Need feature-level design details? Start with `docs/designs/README.md`. Need roadmap context? Check `working-docs/plans/`.
5. Still blocked? File an issue via the [issue tracker](https://github.com/erfianugrah/gloryhole/issues).

---

**Last Updated**: 2025-11-25  
**Version**: v0.7.22+
