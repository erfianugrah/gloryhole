# Glory-Hole

[![CI](https://github.com/erfianugrah/gloryhole/actions/workflows/ci.yml/badge.svg)](https://github.com/erfianugrah/gloryhole/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/erfianugrah/gloryhole)](https://goreportcard.com/report/github.com/erfianugrah/gloryhole)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

Glory-Hole is a self-contained DNS resolver that combines blocklists, an expression-based policy engine, local-zone management, rate limiting, telemetry, and a responsive Web UI in a single Go binary.

## Highlights

- Multi-threaded UDP/TCP DNS server with hot reloading via `pkg/config/watcher`.
- Blocklist manager that downloads, de-duplicates, and atomically swaps sources (exact + wildcard + regex patterns) with optional auto-updates.
- Expr-powered policy engine that can BLOCK/ALLOW/REDIRECT/FORWARD queries per domain, client, or schedule.
- Authoritative local records (A/AAAA/CNAME/TXT/MX/PTR/SRV/NS/SOA/CAA) plus conditional forwarding, whitelist patterns, and kill-switches.
- LRU/sharded DNS cache, per-client token-bucket rate limiting, query logging with SQLite, and OpenTelemetry + Prometheus metrics.
- REST API + HTMX Web UI for live stats, query log, policies, blocklists, client roster, and feature toggles, plus a Pi-hole import tool for fast migrations.
- Guided policy builder with on-the-fly rule tester, JSON export, and live query latency breakdowns (total vs upstream) to quickly debug bottlenecks.

## Architecture Overview

| Area | Package(s) | Notes |
| --- | --- | --- |
| Entry point | `cmd/glory-hole` | CLI flags (`--config`, `--version`, `--health-check`, `import-pihole`) and lifecycle wiring. |
| Core DNS | `pkg/dns`, `pkg/forwarder`, `pkg/cache`, `pkg/ratelimit` | Request processing pipeline, upstream forwarding, caching, rate limiting, decision traces. |
| Filtering | `pkg/blocklist`, `pkg/pattern`, `pkg/policy`, `pkg/localrecords` | Blocklist manager, whitelist/pattern matcher, expression rules, local authority. |
| Config & storage | `pkg/config`, `pkg/storage`, `pkg/resolver` | YAML config + watcher, SQLite-backed query log/statistics, shared DNS resolver for HTTP clients. |
| API & UI | `pkg/api` | REST/JSON endpoints, HTMX UI templates, kill-switch controller, health checks. |
| Telemetry & logging | `pkg/telemetry`, `pkg/logging` | OpenTelemetry meter, Prometheus exporter, structured slog logger factory. |

Docs, deployment assets, and examples live under `docs/`, `deploy/`, `examples/`, and `test/` per the [repository guidelines](AGENTS.md).

## Getting Started

### Requirements

- Go 1.22+ (module target is 1.24).
- SQLite is bundled via `modernc.org/sqlite`; no external database is required.
- Optional: Docker / docker-compose for containerized runs, golangci-lint for local linting.

### Build & Run

```bash
git clone https://github.com/erfianugrah/gloryhole.git
cd gloryhole

# Copy and edit configuration
cp config/config.example.yml config.yml
$EDITOR config.yml

# Development run (no binary artefact)
make dev                          # go run ./cmd/glory-hole

# Build optimized binary with version info under bin/glory-hole
make build

# Run the binary against your config
./bin/glory-hole --config config.yml
```

Make targets:

| Command | Description |
| --- | --- |
| `make build`, `make build-all`, `make release` | Compile (optionally multi-arch) with ldflags that embed version/build metadata. |
| `make dev`, `make run` | Run via `go run` or run the freshly built binary. |
| `make test`, `make test-race`, `make test-coverage` | Execute unit/integration tests, race detector, and coverage report (`coverage.html`). |
| `make fmt`, `make lint`, `make vet` | Formatting and static analysis (`golangci-lint`). |

### Docker Compose

`docker-compose.yml` ships a multi-container stack (Glory-Hole, Prometheus, Grafana) with persisted config/log/data volumes, health checks, and published ports (53/udp+tcp, 8080 for the API/UI, 9090 Prometheus, 3000 Grafana). Bring it up with:

```bash
docker compose up -d
```

The container uses `/etc/glory-hole/config.yml`; mount your `config.yml` there or bake an image via the provided `Dockerfile`.

## Configuration

All runtime settings are defined in YAML (default `config.yml`). The watcher reloads updates in-place and notifies the handler, policy engine, blocklist manager, rate limiter, and conditional forwarder without restarts. Only socket-level changes (listen addresses) require a restart.

```yaml
server:
  listen_address: ":53"
  tcp_enabled: true
  udp_enabled: true
  web_ui_address: ":8080"
  decision_trace: false        # capture rule decisions in query logs
  enable_blocklist: true
  enable_policies: true

upstream_dns_servers:
  - "1.1.1.1:53"
  - "8.8.8.8:53"

blocklists:
  - "https://raw.githubusercontent.com/hagezi/dns-blocklists/main/adblock/ultimate.txt"
auto_update_blocklists: true
update_interval: "24h"
whitelist:
  - "taskassist-pa.clients6.google.com"
  - "*.proxy.cloudflare-gateway.com"

local_records:
  enabled: true
  records:
    - domain: "nas.local"
      type: "A"
      ips: ["192.168.1.100"]
    - domain: "*.dev.local"
      type: "A"
      wildcard: true
      ips: ["10.0.0.5"]

policy:
  enabled: true
  rules:
    - name: "Block social after 22:00"
      logic: "Hour >= 22 || Hour < 6"
      action: "BLOCK"
      enabled: true

conditional_forwarding:
  enabled: true
  rules:
    - name: "LAN split DNS"
      priority: 90
      domains: ["*.lan", "*.home.arpa"]
      upstreams: ["192.168.1.1:53"]

cache:
  enabled: true
  max_entries: 20000
  min_ttl: "60s"
  max_ttl: "24h"
  negative_ttl: "30s"
  blocked_ttl: "5m"
  shard_count: 8

database:
  enabled: true
  backend: "sqlite"
  sqlite:
    path: "./glory-hole.db"
    wal_mode: true
  retention_days: 90
  buffer_size: 1000
  flush_interval: "5s"

telemetry:
  enabled: true
  prometheus_enabled: true
  prometheus_port: 9090
  service_name: "glory-hole"

rate_limit:
  enabled: false
  requests_per_second: 50
  burst: 25
  on_exceed: "nxdomain"
```

Refer to `config/config.example.yml` and `docs/guide/configuration.md` for exhaustive fields and defaults.

## Feature Guide

### Filtering & Blocklists

- `pkg/blocklist` downloads every configured list using a shared HTTP client that respects your upstream DNS servers (`pkg/resolver`), deduplicates entries, and swaps them in via an atomic pointer.
- Pattern support mirrors Pi-hole behaviour: exact matches, wildcard suffixes/prefixes, and regex (through `pkg/pattern`). Stats are exposed in logs and the Web UI.
- Whitelists accept both literal domains and patterns; they short-circuit block decisions.
- Blocklist auto-updates run in the background when `auto_update_blocklists` is enabled; manual reloads are available via the UI or `POST /api/blocklist/reload`.

### Policy Engine

- Rules use [expr-lang/expr](https://github.com/expr-lang/expr) with helpers like `DomainMatches`, `IPInCIDR`, `QueryTypeIn`, `InTimeRange`, etc. (see `pkg/policy`).
- Actions: `BLOCK`, `ALLOW`, `REDIRECT`, and `FORWARD` (the latter pipes queries to the upstream list in `action_data`).
- Rules are compiled once, evaluated in-order, and can be managed through the API/UI (`/policies`).

### Local Resolution & Conditional Forwarding

- `pkg/localrecords` supports authoritative answers for A/AAAA/CNAME/TXT/MX/PTR/SRV/NS/SOA/CAA with custom TTLs, multiple IPs (round robin), and wildcard entries. CNAME chains and EDNS0 are handled automatically.
- `pkg/forwarder` + `pkg/forwarder/evaluator` implement priority-based conditional forwarding with optional domain patterns, client CIDRs, and query type filters. Matching rules forward queries to specific upstream pools before falling back to the default round-robin forwarder.

### Cache, Rate Limiting, and Resiliency

- `pkg/cache` provides TTL-aware LRU caching with optional sharding, negative caching, and blocked-response TTL overrides. Metrics (hits/misses/size) feed OpenTelemetry.
- `pkg/ratelimit` enforces per-client token buckets with CIDR overrides and configurable actions (`drop` or `nxdomain`) plus optional violation logging.
- Graceful shutdown handles DNS + API servers, blocklist manager, rate limiter, SQLite flusher, and telemetry exporters.

### Storage, Telemetry, and Monitoring

- Query logs are written asynchronously through `pkg/storage` (SQLite via modernc, WAL enabled, batched inserts, retention cleanup). Query entries include client IP, response code, cache/block flags, upstream, upstream response time, and optional block trace.
- `pkg/telemetry` exposes Prometheus metrics on `:9090/metrics` (configurable) and seeds OpenTelemetry meters for counters/histograms used throughout the codebase. The dashboard augments those stats with host-level CPU%, memory usage, and temperature readings collected via gopsutil.
- Health checks: `--health-check` CLI flag hits `/api/health`, Kubernetes/Compose manifests already wire them up.

### Web UI & REST API

- The API listens on `server.web_ui_address` (default `:8080`) and exposes `/api/health`, `/api/stats`, `/api/queries`, `/api/top-domains`, `/api/policies`, `/api/features`, `/api/cache/purge`, etc. HTMX endpoints under `/api/ui/*` feed the dashboard.
- Pages: `/` (dashboard with live charts + system metrics), `/queries` (filters, traces, total/upstream latency), `/policies` (guided builder + export + inline tester), `/settings`, `/clients`, and `/blocklists` (source inventory plus tester). UI actions include policy CRUD, blocklist reloads, cache flush, JSON export, client annotations/groups, and feature kill-switches with temporary disable timers.
- CORS is enabled for API routes so you can embed metrics elsewhere.

### Pi-hole Import Tool

`glory-hole import-pihole` converts Pi-hole Teleporter archives or raw files (`gravity.db`, `pihole.toml`, `custom.list`) into Glory-Hole YAML:

```bash
./bin/glory-hole import-pihole --zip ~/Downloads/pihole-teleporter.zip --output config.yml
# or
./bin/glory-hole import-pihole --gravity-db=/etc/pihole/gravity.db --pihole-config=/etc/pihole/pihole.toml --output config.yml
```

Use `--dry-run` to preview changes or `--validate=false` to skip config validation.

## Operations Notes

- **Hot reload**: Editing `config.yml` triggers `pkg/config/watcher`, which repopulates blocklists, local records, policies, whitelist patterns, conditional forwarding, and rate limits in-place.
- **Kill switch**: Global toggles for blocklists and policies live under `/api/features` and in the UI, with duration-based disable (30s/5m/30m/1h/indefinite) stored back into the config file.
- **Decision tracing**: Enable `server.decision_trace` to capture block reasoning per query (shown in the query log and API responses).
- **Health & readiness**: `/ready` checks dependency wiring (blocklist, database, cache); `/health` surfaces uptime/build metadata.
- **Metrics**: Prometheus scrape target defaults to `http://<host>:9090/metrics`; dashboards and alerts live in `deploy/grafana` and `deploy/prometheus`.

## Development

- Run `make fmt lint test` before sending patches. `golangci-lint` validates import ordering, staticcheck, gosec, and vet rules defined in `.golangci.yml`.
- Integration and regression suites live alongside packages plus `test/` (load fixtures, Pi-hole fixtures, etc.). Keep packages above ~80% coverage when touching them, and use `make test-coverage` to regenerate `coverage.html`.
- Docs in `docs/` cover architecture, deployment, troubleshooting, and roadmap. Update `docs/guide/configuration.md` and relevant manifests under `deploy/` when altering config knobs or defaults.

## Repository Layout

- `cmd/glory-hole/` – main program and Pi-hole importer CLI.
- `pkg/*` – application packages (API, DNS handler, cache, policy engine, blocklist manager, telemetry, storage, resolver, etc.).
- `config/` – sample configs (`config.example.yml`) and defaults.
- `deploy/` – Docker, Kubernetes, Prometheus, Grafana assets.
- `docs/`, `cf-docs/`, `examples/` – guides, architecture notes, policy examples.
- `scripts/` – helper utilities.
- `test/` – integration/load tests and fixtures.
- `bin/` – build output (ignored from VCS).

## License

This project is released under the [MIT License](LICENSE). Contributor terms are outlined in [CONTRIBUTING.md](CONTRIBUTING.md).
