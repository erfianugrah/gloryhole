# Documentation Index

Complete map of Glory-Hole documentation.

---

## Quick Start

- **[Project README](../README.md)** -- Overview, build/run, architecture summary
- **[Getting Started](guide/getting-started.md)** -- Installation and first DNS queries
- **[Configuration Reference](guide/configuration.md)** -- Exhaustive YAML options with examples
- **[Usage Guide](guide/usage.md)** -- Everyday operations and workflows
- **[Changelog](../CHANGELOG.md)** -- Full release history

---

## Architecture

- **[System Overview](architecture/overview.md)** -- End-to-end architecture map
- **[Component Details](architecture/components.md)** -- Deep dives per subsystem
- **[Design Decisions](architecture/design-decisions.md)** -- ADR-style rationale
- **[Design Library](designs/README.md)** -- Active feature design proposals

---

## User Guides

- **[Getting Started](guide/getting-started.md)** -- Install, bootstrap, and smoke tests
- **[Configuration](guide/configuration.md)** -- Settings, schema, and examples
- **[Usage](guide/usage.md)** -- Web UI, CLI, and automation flows
- **[DNS over HTTPS](dns-over-https.md)** -- DoH setup and client configuration
- **[Pi-hole Migration](guide/pihole-migration.md)** -- Import blocklists and local records
- **[Pattern Matching](guide/pattern-matching.md)** -- Exact, wildcard, and regex behaviour
- **[Performance Tuning](guide/performance-tuning.md)** -- Cache, rate limit, telemetry knobs
- **[Troubleshooting](guide/troubleshooting.md)** -- Common failures and fixes

---

## Deployment

- **[Docker Deployment](deployment/docker.md)** -- Compose stack and images
- **[VyOS + Docker](deployment/vyos-docker-guide.md)** -- On-router container workflows
- **[Monitoring](deployment/monitoring.md)** -- Prometheus, Grafana, and alerting
- **[Kubernetes Manifests](../deploy/kubernetes/README.md)** -- Cluster deployment
- **[Deploy Directory](../deploy/README.md)** -- Additional manifests and automation

---

## API & UI

- **[REST API Reference](api/rest-api.md)** -- Endpoints, payloads, and examples
- **[Policy Engine API](api/policy-engine.md)** -- Rule schema and lifecycle
- **[Web UI Guide](api/web-ui.md)** -- Dashboard features and operator tooling

---

## Development

- **[Development Setup](development/setup.md)** -- Toolchain, dependencies, local config
- **[Frontend Build](development/frontend.md)** -- Astro/React dashboard build process
- **[Testing Guide](development/testing.md)** -- Unit/integration/benchmark suites
- **[Contributing](../CONTRIBUTING.md)** -- Branching, coding standards, review policy
- **[Makefile Targets](../Makefile)** -- Common automation

---

## Directory Structure

```
docs/
├── INDEX.md            # This file
├── README.md           # Documentation overview
├── dns-over-https.md
├── api/                # REST API, policy engine, web UI docs
├── architecture/       # System overview, components, design decisions
├── deployment/         # Docker, Kubernetes, monitoring
├── designs/            # Active feature design proposals
├── development/        # Dev setup, testing, frontend build
└── guide/              # User guides, configuration, troubleshooting
```
