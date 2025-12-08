# Glory-Hole Documentation

Welcome to the Glory-Hole DNS Server documentation. This guide will help you understand, deploy, and operate Glory-Hole in any environment.

> **📖 [Complete Documentation Index](INDEX.md)** - Find all documentation organized by topic

## Latest Release

- **Current:** v0.9.5 (see [Changelog](../CHANGELOG.md))
- Config/UI now support DoT TLS modes (manual PEM, autocert HTTP-01, native Cloudflare DNS-01) and editable rate limiting.

## Documentation Structure

### [User Guide](guide/)
Start here if you're new to Glory-Hole or want to deploy it.

- **[Getting Started](guide/getting-started.md)** - Installation and quick setup
- **[Configuration](guide/configuration.md)** - Complete configuration reference
- **[Usage Guide](guide/usage.md)** - Day-to-day operations
- **[Troubleshooting](guide/troubleshooting.md)** - Common issues and solutions

### [Architecture](architecture/)
Understand how Glory-Hole works internally.

- **[System Overview](architecture/overview.md)** - High-level architecture
- **[Component Details](architecture/components.md)** - Deep dive into each component
- **[Performance](architecture/performance.md)** - Benchmarks and optimizations
- **[Design Decisions](architecture/design-decisions.md)** - Architecture decision records
- **[Design Library](designs/)** - Feature-level architecture docs (kill-switch, conditional forwarding, metrics, DNSSEC, etc.)

### [Development](development/)
Contributing to Glory-Hole development.

- **[Development Setup](development/setup.md)** - Set up your dev environment
- **[Testing Guide](development/testing.md)** - Running and writing tests
- **[Roadmap](roadmap.md)** - Future plans and milestones

### [Working & Implementation Docs](../working-docs/README.md)
Status reports, in-progress release plans, verification/cleanup logs, and raw benchmark artifacts for contributors. Finalized feature designs live under [docs/designs/](designs/).

### [Deployment](deployment/)
Deploy Glory-Hole in production.

- **[VyOS & Docker Guide](deployment/vyos-docker-guide.md)** - VyOS container and Docker deployment
- **[Docker](deployment/docker.md)** - Containerized deployment
- **[Cloudflare D1](deployment/cloudflare-d1.md)** - Deferred; guide retained for future D1 reintroduction (v0.9 supports SQLite only)
- **DNS-over-TLS (DoT)** – configurable listener with manual TLS, HTTP-01 autocert, or native Cloudflare DNS-01. See the configuration guide for the end-to-end steps and Android Private DNS setup.
- **[Monitoring](deployment/monitoring.md)** - Observability and monitoring

### [API Reference](api/)
API and integration documentation.

- **[REST API](api/rest-api.md)** - HTTP API reference
- **[Web UI](api/web-ui.md)** - Web interface guide
- **[Policy Engine](api/policy-engine.md)** - Policy configuration reference

## Quick Links

### For Users
- [Quick Start Guide](guide/getting-started.md#quick-start)
- [Docker Deployment](deployment/docker.md)
- [Configuration Examples](guide/configuration.md#examples)

### For Developers
- [Development Setup](development/setup.md)
- [Running Tests](development/testing.md)
- [Architecture Overview](architecture/overview.md)

### For Operators
- [Kubernetes Deployment](../deploy/kubernetes/README.md)
- [Monitoring Setup](deployment/monitoring.md)
- [Troubleshooting](guide/troubleshooting.md)

## Additional Resources

- [Changelog](../CHANGELOG.md) - Version history and release notes
- [GitHub Repository](https://github.com/erfianugrah/gloryhole)
- [Issue Tracker](https://github.com/erfianugrah/gloryhole/issues)

## Getting Help

- **Documentation Issues**: [File an issue](https://github.com/erfianugrah/gloryhole/issues/new?labels=documentation)
- **Questions**: Check [Troubleshooting](guide/troubleshooting.md) first
- **Bugs**: [Report a bug](https://github.com/erfianugrah/gloryhole/issues/new?labels=bug)
- **Feature Requests**: [Request a feature](https://github.com/erfianugrah/gloryhole/issues/new?labels=enhancement)

## Documentation Standards

All Glory-Hole documentation follows these principles:
- **Accurate**: Verified against actual code and behavior
- **Complete**: No missing steps or assumptions
- **Current**: Updated with each release
- **Tested**: All examples and commands are tested
- **Clear**: Written for users of all skill levels

---

**Version:** 0.9.5
**Last Updated:** 2025-12-08
**Status:** Production Ready
