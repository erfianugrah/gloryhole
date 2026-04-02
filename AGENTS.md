# Agent Guidelines

Instructions for AI coding agents working on this repository.

## Commit Rules

- Commits must not include co-author trailers or AI attribution.
- Write concise commit messages focused on the "why" rather than the "what".
- Use conventional commit prefixes: `feat:`, `fix:`, `refactor:`, `test:`, `docs:`, `ci:`.

## Architecture

Glory-Hole is a Go DNS server with an embedded Astro/React dashboard.

### Go Backend

- Entry point: `cmd/glory-hole/main.go`
- DNS pipeline: `pkg/dns/` (handler) -> `pkg/blocklist/` + `pkg/policy/` (filtering) -> `pkg/cache/` -> `pkg/forwarder/` (upstream) or `pkg/unbound/` (recursive)
- API: `pkg/api/` — REST handlers, middleware, embedded UI serving
- Config: `pkg/config/` — YAML config with hot-reload via watcher
- Storage: `pkg/storage/` — SQLite query logging
- All packages use `pkg/logging` (slog wrapper) and `pkg/telemetry` (OpenTelemetry/Prometheus)

### Frontend Dashboard

- Location: `pkg/api/ui/dashboard/`
- Stack: Astro 6 (static SSG) + React 19 (islands) + shadcn/ui + Tailwind v4
- Build output: `pkg/api/ui/static/dist/` — embedded via `go:embed`
- 12 pages, each an Astro page with a React component using `client:load`
- API client: `src/lib/api.ts` — typed fetch wrapper for all `/api/*` endpoints
- Components follow shadcn/ui patterns in `src/components/ui/`

### Unbound Integration

- Location: `pkg/unbound/`
- Supervisor manages Unbound as a child process on `127.0.0.1:5353`
- Config model: typed structs serialized to `unbound.conf` via `text/template`
- 11 API endpoints under `/api/unbound/*`

## Build & Test

```bash
make build          # Build UI + Go binary
make test           # Run all Go tests
make lint           # golangci-lint
go test ./pkg/...   # Package-level tests

# Docker
docker build -t glory-hole:dev -f Dockerfile .

# Fly.io deployment
make fly-deploy     # Build Fly image, push, deploy
make docker-push    # Build and push main image to DockerHub
```

## Key Patterns

- Handler methods are on `*Server` receiver in `pkg/api/handlers_*.go`
- Config hot-reload: `cfgWatcher.OnChange()` callback in `main.go`
- Components that can be nil (blocklist, cache, policy, unbound) are nil-checked before use
- shadcn/ui primitives live in `src/components/ui/`, page components in `src/components/`
- Typography constants in `src/lib/typography.ts` (`T.pageTitle`, `T.cardTitle`, etc.)

## Deployment

- **Docker**: `Dockerfile` for the main multi-arch image, `Dockerfile.release` for GoReleaser
- **Fly.io**: `Dockerfile.fly` bakes `config.fly.yml` into the image; `fly.toml` defines services
  - Per-protocol listen: `udp_listen_address`/`tcp_listen_address` in `ServerConfig` override `listen_address`
  - ACME cert obtain is non-blocking; DNS/API start immediately, DoT comes online when cert is ready
  - CI deploys to Fly.io on tag push (see `.github/workflows/release.yml`)
- **Config**: `config.fly.yml` (gitignored) for Fly, `config.yml` (gitignored) for local/router

## Files to Never Commit

- `config.yml`, `config.*.yml` (contain secrets — use `config/config.example.yml` as template)
- `*.db`, `*.sqlite` files
- `CLAUDE.md`, `.claude/` directory
