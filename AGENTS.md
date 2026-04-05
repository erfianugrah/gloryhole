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
- API: `pkg/api/` — REST handlers, middleware, embedded UI serving, DoH endpoint
- Config: `pkg/config/` — YAML config with hot-reload via fsnotify watcher
- Storage: `pkg/storage/` — SQLite query logging with buffered async writes
- All packages use `pkg/logging` (slog wrapper) and `pkg/telemetry` (OpenTelemetry/Prometheus)

### Frontend Dashboard

- Location: `pkg/api/ui/dashboard/`
- Stack: Astro 6 (static SSG) + React 19 (islands) + shadcn/ui + Tailwind v4
- Build output: `pkg/api/ui/static/dist/` — embedded via `go:embed`
- 13 pages, each an Astro page with a React component using `client:load`
- API client: `src/lib/api.ts` — typed fetch wrapper for all `/api/*` endpoints
- Components follow shadcn/ui patterns in `src/components/ui/`
- Color palette: Lovelace theme (`gh-*` colors) defined in `src/styles/globals.css`
- Typography: `src/lib/typography.ts` (`T.pageTitle`, `T.tableCellMono`, etc.)

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
- Config hot-reload: `cfgWatcher.OnChange()` callback in `main.go` — blocklist reloads run async
- Blocklist manager uses `atomic.Pointer` for lock-free reads; `sync.Mutex` serializes downloads
- Policy engine uses `atomic.Int32` for `Count()` — avoids RLock on every DNS query
- Policy rule compilation is shared via `compileRuleLogic()` (safe type wrappers, no panics)
- CIDR and regex caches are bounded (`maxCIDRCacheSize=256`, `maxRegexCacheSize=1024`)
- Components that can be nil (blocklist, cache, policy, unbound) are nil-checked before use
- shadcn/ui primitives live in `src/components/ui/`, page components in `src/components/`
- Query log detail rows use `DetailRow` component with label/value/badge pattern

## Security

- API has per-IP rate limiting (5/min login, 60/s API) in `pkg/api/middleware_ratelimit.go`
- CSRF protection via `X-Requested-With` header on mutating `/api/*` calls
- Trusted proxy config (`trusted_proxies` in ServerConfig) gates X-Forwarded-For trust
- Unbound config template inputs are sanitized before rendering (reject newlines/quotes)
- All request bodies have `MaxBytesReader` limits
- Blocklist URLs must use http/https scheme; downloads limited to 100MB
- Session cookies set `Secure` flag when behind trusted proxy with HTTPS

## Deployment

- **Docker**: `Dockerfile` for the main multi-arch image, `Dockerfile.release` for GoReleaser
- **Fly.io**: `Dockerfile.fly` bakes `config.fly.yml` + `docker-entrypoint.sh` into the image; `fly.toml` defines services
  - VM: `shared-cpu-1x` / `512mb` with `GOMEMLIMIT=384MiB`
  - Entrypoint copies baked config to persistent volume (`/var/lib/glory-hole/config.yml`) on first boot
  - Subsequent deploys use the volume copy, preserving API-written changes
  - To force a config reset: `fly ssh console` then `rm /var/lib/glory-hole/config.yml` and restart
  - Per-protocol listen: `udp_listen_address`/`tcp_listen_address` override `listen_address`
  - TCP DNS and DoT use PROXY protocol for real client IPs; DoH uses `trusted_proxies` + X-Forwarded-For
  - UDP DNS on Fly.io cannot provide real client IPs (platform limitation — Fly NATs the source)
  - ACME cert obtain is non-blocking; DNS/API start immediately, DoT comes online when cert is ready
  - Deploy: `fly deploy --remote-only` (no local Docker needed)
  - CI deploys to Fly.io on tag push (see `.github/workflows/release.yml`)
- **Config**: `config.fly.yml` (gitignored) for Fly, `config.yml` (gitignored) for local/router

## Performance Notes

- Blocklist reloads are async and serialized with `TryLock` — concurrent attempts are skipped, not queued
- Blocklist maps are pre-allocated based on previous size to avoid rehashing
- Old blocklist map is not referenced during download, allowing GC to reclaim it (halves peak memory)
- QueryLogger uses channel-close for shutdown (workers drain via range loop, no stranded entries)
- Cache TTL cleanup decrements the Prometheus CacheSize gauge to prevent drift
- Session manager cleanup goroutine is stopped during API server shutdown

## Files to Never Commit

- `config.yml`, `config.*.yml` (contain secrets — use `config/config.example.yml` as template)
- `*.db`, `*.sqlite` files
- `CLAUDE.md`, `.claude/` directory
