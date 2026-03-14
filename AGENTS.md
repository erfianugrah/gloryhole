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
```

## Key Patterns

- Handler methods are on `*Server` receiver in `pkg/api/handlers_*.go`
- Config hot-reload: `cfgWatcher.OnChange()` callback in `main.go`
- Components that can be nil (blocklist, cache, policy, unbound) are nil-checked before use
- shadcn/ui primitives live in `src/components/ui/`, page components in `src/components/`
- Typography constants in `src/lib/typography.ts` (`T.pageTitle`, `T.cardTitle`, etc.)

## Files to Never Commit

- `config.yml` (contains secrets — use `config/config.example.yml` as template)
- `*.db`, `*.sqlite` files
- `CLAUDE.md`, `.claude/` directory
