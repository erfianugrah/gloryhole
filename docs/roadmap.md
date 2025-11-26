# Glory-Hole DNS Server - Development Roadmap

**Last Updated**: 2025-11-24
**Current Version**: v0.7.22
**Next Releases**: v0.8.0 (Functional UI), v0.9.0 (Advanced Features)

---

## Table of Contents

- [Immediate Next Steps](#immediate-next-steps-v080-sprint-3-kickoff)
- [Codebase Health Overview](#codebase-health-overview)
- [Technical Debt Registry](#technical-debt-registry)
- [Bug Tracker](#bug-tracker)
- [Documentation Gaps](#documentation-gaps)
- [UI Assessment](#ui-assessment)
- [v0.7.1 Release Plan](#v071-release-plan)
- [v0.8.0 Release Plan](#v080-release-plan)
- [Progress Tracking](#progress-tracking)

---

## Immediate Next Steps (v0.8.0 Sprint 3 Kickoff)

1. **Docs & telemetry follow-up** – Capture the new runtime-config + analytics flows in the admin guide and surface rate-limit/cache metrics so operators can observe behavior.
2. **Client analytics prep (Sprint 4)** – Define the `/api/stats/clients` contract and design the `/ui/clients` page so the next sprint can focus on per-client dashboards.
3. **Export/bulk UX planning** – Outline data-export formats and bulk policy operations (Sprints 5–6) to keep the UI momentum going once analytics lands.

---

## Codebase Health Overview

### Current Statistics

**Codebase Size**:
- Production Code: 5,874 lines
- Test Code: 9,170 lines
- UI Code: 1,673 lines (841 HTML + 832 CSS/JS)
- Total: 16,717 lines

**Test Coverage**: 82.4%
- Best: policy (95.2%), pattern (94.1%), localrecords (92.9%)
- Good: forwarder (87.1%), resolver (87.9%), config (82.7%)

**Dependencies**: 11 direct dependencies (minimal)

**Code Quality**:  Excellent
- 0 race conditions detected
- Clean golangci-lint results
- Comprehensive structured logging
- Good separation of concerns

### Strengths ✓

1. **Excellent Test Coverage**: 242 tests, 82.4% coverage
2. **Clean Architecture**: DDD principles, well-separated concerns
3. **Lock-Free Blocklist**: 8ns lookup, 372M QPS capability
4. **Comprehensive Metrics**: OpenTelemetry + Prometheus
5. **No SQL Injection**: Prepared statements only
6. **Graceful Shutdown**: Proper resource cleanup
7. **Structured Logging**: Consistent slog usage

### Areas for Improvement

1. **UI Functionality**: ~70% complete (needs editable settings + advanced analytics)
2. **Large Files**: Some files exceed 500 lines
3. **Complex Functions**: Several 100+ line functions
4. **Missing Features**: Rate limiting, CSRF protection, input validation

---

## Technical Debt Registry

### Active Items

#### TD-002: Whitelist Still Uses RWMutex
- **Priority**: 🔴 High (Performance)
- **Location**: `pkg/dns/server.go:833-876`
- **Issue**: Exact-match whitelist lookups continue to take a read lock while the blocklist and pattern matchers are lock-free.
- **Impact**: Adds unnecessary contention during spikes (especially when blocklist manager is hot-reloaded).
- **Plan**: Mirror the atomic pointer pattern used by `BlocklistManager` so whitelist snapshots can be swapped without locking.
- **Status**: 📋 Backlog (unassigned)

#### TD-004: Large Function / File Complexity
- **Priority**: 🟡 Medium
- **Locations**: `pkg/dns/server.go`, `pkg/config/config.go`, `pkg/api/handlers_policy.go`
- **Issue**: Core handlers exceed 600 lines with multiple responsibilities, slowing reviews and discouraging targeted tests.
- **Plan**: Extract reusable helpers (e.g., DNS response writers, policy validation helpers) and split config serialization from validation.
- **Status**: 📋 Backlog

#### TD-005: Ignored Close() Errors in Tests
- **Priority**: 🟢 Low
- **Scope**: 70+ defer blocks such as `defer func() { _ = resource.Close() }()`
- **Impact**: Masks intermittent cleanup failures when running integration tests under -race.
- **Plan**: Introduce a lightweight helper (e.g., `testutil.MustClose(t, c)`) and replace the `_ =` pattern in the few hot test helpers.
- **Status**: 📋 Backlog

### Recently Cleared (v0.7.4 – v0.7.6)

#### TD-001: Storage Logging Uses slog
- **Status**: ✅ Completed in v0.7.4
- **Evidence**: `SQLiteStorage.flushWorker` now emits structured errors via `slog.Default()` instead of `fmt.Printf` (`pkg/storage/sqlite.go:146-205`).

#### TD-003: Versioned Database Migrations
- **Status**: ✅ Completed in v0.7.4
- **Evidence**: `pkg/storage/migrations.go` plus embedded SQL files provide forward-only migrations, and `SQLiteStorage` applies them at startup (`pkg/storage/sqlite.go:82-128`).

---

## Bug Tracker

### Active Issues

#### BUG-007: No CSRF Protection
- **Priority**: 🟢 Low
- **Scope**: All POST/PUT/DELETE endpoints
- **Status**: 📋 Backlog
- **Notes**: Not urgent while deployment assumes trusted network, but still required before exposing UI on the open internet.

### Resolved Since v0.7.4

- **BUG-002 – Policy Validation**: `handleAddPolicy` / `handleUpdatePolicy` now compile expressions via `policy.Engine.AddRule` and return parser errors to the client (`pkg/api/handlers_policy.go:121-205`).
- **BUG-003 – Config Watcher Races**: `pkg/config/watcher.go` applies updates under an RWMutex and debounces fsnotify events, eliminating stale reads during reload.
- **BUG-005 – Request Size Limits**: Every policy + feature endpoint wraps the body with `http.MaxBytesReader`; 10MB for policies, 1MB for kill-switch routes.
- **BUG-006 – Query Pagination**: HTMX pagination updates `hx-get` offsets dynamically (`pkg/api/ui/templates/queries.html:15-42`) and the handler honors `limit`/`offset`.
- **BUG-001 – Dashboard Mock Data**: `/api/stats/timeseries` plus the dashboard fetch loop now deliver live storage data (`pkg/api/handlers.go:159-214`, `pkg/storage/sqlite.go:497-610`, `pkg/api/ui/templates/dashboard.html:120-205`).
- **BUG-004 – HTTP rate limiting**: API mux now enforces per-IP token buckets via `pkg/ratelimit`, emitting 429 responses when limits trip and logging offenders when configured (`pkg/api/middleware_ratelimit.go`, `cmd/glory-hole/main.go:150-260`).

---

## Documentation Gaps

### Active

#### DOC-003: Godoc Coverage Still Spotty
- **Priority**: 🟡 Medium
- **Status**: 🚧 Partially addressed
- **Notes**: Forwarder/telemetry packages now have doc comments, but `pkg/dns/server.go` and several API helpers remain undocumented; need a focused pass before v0.8.0.

#### DOC-005: Missing UI Screenshots
- **Priority**: 🟢 Low
- **Status**: 📋 Backlog
- **Notes**: `docs/screenshots/` is still empty; plan to capture updated dashboard/settings views once real data is wired up.

### Resolved

- **DOC-004 – README placeholders**: Operations section now documents kill-switches, hot reload, and telemetry instead of fake download URLs (`README.md:217-226`).
- **DOC-001 – Time-Series API docs**: `/api/stats/timeseries` is implemented/tested and documented in this roadmap plus API reference (`pkg/api/handlers.go:159-214`, `pkg/api/api_test.go:320-370`).
- **DOC-002 – Settings data binding**: `/api/config` response + template binding now documented and live (`pkg/api/handlers.go:214-248`, `pkg/api/ui/templates/settings.html:1-220`).

---

## UI Assessment

### Current State: 75% Complete

**Implemented Features** :
- Dashboard with real-time statistics
- Query log viewer with auto-refresh
- Query type distribution visualization
- Bar charts for top allowed/blocked domains and cache hit-rate trends
- Policy management CRUD interface
- Settings review page
- Kill-switch controls with HTMX timers (blocklist + policy engine)
- Responsive CSS (833 lines)
- HTMX integration
- Chart.js visualizations
- Mobile-friendly design

**Technical Stack**:
- Backend: Go templates (841 lines)
- Frontend: Vanilla JavaScript + HTMX
- Styling: Custom CSS (no frameworks)
- Charts: Chart.js

### Recently Resolved

- **UI-001 – Chart Mock Data**: Dashboard now streams `/api/stats/timeseries` results every 30 s (`pkg/api/handlers.go:159-214`, `pkg/api/ui/templates/dashboard.html:120-205`).
- **UI-002 – Static Settings**: Settings template binds to real config payloads via `/api/config` (`pkg/api/ui/templates/settings.html:1-220`).
- **UI-003 – Query Filters**: HTMX filters drive `storage.QueryFilter`, enabling domain/type/status/time filtering (`pkg/api/ui/templates/queries.html:1-150`, `pkg/storage/sqlite.go:513-642`).
- **UI-010 – Editable Settings**: `/api/config/{upstreams,cache,logging}` endpoints plus HTMX forms let operators adjust resolvers, TTLs, and logging without SSH (`pkg/api/handlers_config_update.go`, `pkg/api/settings_page.go`, `pkg/api/ui/templates/settings.html`).
- **UI-011 – HTTP API rate limiting**: API middleware now throttles per-IP using the existing token bucket manager and shares configuration/live status through `/api/config` (`pkg/api/middleware_ratelimit.go`, `cmd/glory-hole/main.go:150-260`).
- **UI-012 – Dashboard analytics**: `/api/stats/query-types` powers the doughnut chart and new Chart.js components now render cache hit rate plus top allowed/blocked domains using live API data (`pkg/storage/sqlite.go:514-596`, `pkg/api/handlers.go:189-225`, `pkg/api/ui/templates/dashboard.html`).
- **CONFIG-001 – Runtime config reloads**: Upstream DNS, cache, and logging edits apply immediately via hot-reload hooks (`cmd/glory-hole/main.go`, `pkg/blocklist/manager.go`).
- **UI-004 – Client Management**: `/clients` surfaces auto-discovered clients, last/first seen timestamps, blocked/NXDOMAIN counts, and editable names/notes/groups (`pkg/api/handlers_clients.go`, `pkg/api/ui/templates/clients.html`, `pkg/storage/sqlite_clients.go`).
- **UI-005 – Advanced Analytics (Phase 1)**: Dashboard cards now include CPU%, memory, and temperature alongside per-query total vs upstream latency, backed by `github.com/shirou/gopsutil` and the new `upstream_time_ms` column (`pkg/api/system_metrics.go`, `pkg/api/handlers.go`, `pkg/dns/server.go`, `pkg/storage/migrations.go`).
- **UI-008 – Export Functionality**: Operators can download the current rule set via `/api/policies/export` and the UI export button (`pkg/api/handlers_policy.go`, `pkg/api/ui/templates/policies.html`).
- **UI-013 – Guided Policy Builder**: Policies modal now offers a condition builder with presets, expression toggle, and inline tester to safely author expr rules (`pkg/api/ui/templates/policies.html`).
- **UI-014 – Blocklist Inspector**: `/blocklists` inventories sources, shows live counts, last-update time, and includes a tester against the active blocklist (`pkg/api/handlers_blocklists.go`, `pkg/api/ui/templates/blocklists.html`).

### Missing Features

#### UI-005: No Advanced Analytics
- **Priority**: 🟡 Medium
- **Status**: 🚧 Partially delivered (system metrics + latency drill-down shipped)
- **Next**: Per-client drill-downs (top domains, policy hits), rolling comparisons, exportable CSVs
- **Planned**: v0.8.x

#### UI-006: No Bulk Operations
- **Priority**: 🟡 Medium
- **Missing**: Cannot enable/disable multiple policies
- **Planned**: v0.8.0

#### UI-009: No Search in Policy List
- **Priority**: 🟢 Low
- **Status**: No filtering when policy list grows
- **Planned**: v0.8.0

#### UI-015: Bulk Policy & Client Actions
- **Priority**: 🟡 Medium
- **Missing**: Multi-select toggles/export/import for policies and client annotations
- **Planned**: v0.8.x

---

## v0.7.x Recap (v0.7.1 – v0.7.22)

**Theme**: Technical excellence, safety rails, and UX groundwork  
**Status**: ✅ Delivered (see `CHANGELOG.md`)

### Shipped Highlights

1. **Storage + Telemetry Hardening**
   - Replaced ad-hoc `fmt.Printf` logging with structured slog output during flush failures (`pkg/storage/sqlite.go`).
   - Added OpenTelemetry gauges/counters for cache, blocklist, and rate limiting plus end-to-end Prometheus exporter wiring (`pkg/telemetry/telemetry.go`).

2. **Security + Validation**
   - Wrapped every POST/PUT body with `http.MaxBytesReader` (10 MB for policies, 1 MB for kill-switch endpoints) to prevent oversized payload DoS.
   - Policy API now compiles expressions via `policy.Engine.AddRule`, surfacing syntax errors back to the UI before persisting.

3. **Database Migrations + Tooling**
   - Introduced an embedded SQL migration runner (`pkg/storage/migrations.go`) so schema upgrades are idempotent and automated.
   - Pi-hole import CLI (`cmd/glory-hole/import.go`) plus regex/wildcard pattern support (`pkg/pattern/`) eased onboarding.

4. **Kill-Switch + Web UI Controls**
   - Added persistent `enable_blocklist` / `enable_policies` flags, REST endpoints, countdown timers, and HTMX buttons in the Settings page.
   - KillSwitchManager handles temporary disables with re-enable timers and Prometheus metrics (`pkg/api/killswitch.go`).

5. **Resolver & DNS Fixes**
   - Centralized HTTP/DNS resolver usage so blocklist downloads honor configured upstreams.
   - Forwarder now passes SERVFAIL responses through immediately, matching DNSSEC expectations (v0.7.8).

6. **Performance & Ops (v0.7.22)**
   - Cache stats now use `atomic.Uint64` counters and shard cleanup runs in parallel, boosting throughput by ~5% and making cleanup 64× faster.
   - Added composite SQLite indexes for the analytics path plus struct alignment/field reordering for better cache locality (`docs/FINAL_SUMMARY.md`).
   - GitHub Actions, release automation, and documentation were finalized in the optimization pass.

7. **Quality of Life**
   - README operations guidance now documents hot reload and kill-switch flows instead of placeholder download URLs.
   - Query log UI pagination works through HTMX offset updates.

> These milestones close the original v0.7.1 backlog; the remaining roadmap below focuses on the still-missing UI functionality targeted for v0.8.0.

---

## v0.8.0 Release Plan

**Theme**: Functional Web UI
**Target**: Major release
**Timeline**: 3-6 weeks
**Status**: 📋 Planned

### Objectives

1.  100% functional Web UI (no mock data)
2.  Real-time data visualization
3.  Full configuration management
4.  Advanced analytics and insights
5.  Production-ready API
6.  Enhanced user experience

### Sprint 1: Core UI Fixes (Week 1)

#### Task 1.1: Implement Time-Series Statistics API
- **ID**: UI-001, BUG-001
- **Priority**: 🔴 Critical
- **Files**: `pkg/api/handlers.go`, `pkg/storage/sqlite.go`, `pkg/api/api_test.go`
- **Endpoint**: `GET /api/stats/timeseries?period=hour&points=24`
- **Returns**: Aggregated query statistics over time
- **Parameters**:
  - `period`: hour/day/week
  - `points`: number of data points
- **Effort**: 8 hours
- **Status**: ✅ Completed
- **Notes**: Storage exposes `GetTimeSeriesStats`, the API handler streams normalized buckets, and tests cover validation + payload shape.

**Response Format**:
```json
{
  "period": "hour",
  "points": 24,
  "data": [
    {
      "timestamp": "2025-01-22T00:00:00Z",
      "total_queries": 1234,
      "blocked_queries": 56,
      "cached_queries": 789,
      "avg_response_time_ms": 12.5
    }
  ]
}
```

#### Task 1.2: Update Dashboard JavaScript
- **Files**: `pkg/api/ui/templates/dashboard.html:153-174`
- **Changes**: Replace mock data with API call
- **Implementation**: Fetch from `/api/stats/timeseries`
- **Error Handling**: Show loading state, handle errors
- **Effort**: 2 hours
- **Status**: ✅ Completed
- **Notes**: `/api/stats/timeseries` now powers the Chart.js dataset; dashboard fetches live data every 30s.

#### Task 1.3: Dynamic Settings Page
- **ID**: UI-002, DOC-002
- **Priority**: 🔴 Major
- **Files**:
  - `pkg/api/handlers.go`
  - `pkg/api/ui/templates/settings.html`
- **Endpoint**: `GET /api/config`
- **Changes**: Pass actual config to template
- **Display**: Real values for all settings
- **Effort**: 4 hours
- **Status**: ✅ Completed
- **Notes**: Settings UI now binds to `/api/config`; cache/server/blocklist metrics reflect live values and config path is displayed.

#### Task 1.4: Query Filtering Implementation
- **ID**: UI-003
- **Priority**: 🔴 Major
- **Files**:
  - `pkg/api/handlers.go` (update handler)
  - `pkg/api/ui/templates/queries.html:10-16`
- **Endpoint**: `GET /api/queries?domain=&type=&status=&start=&end=`
- **Filters**:
  - Domain (substring match)
  - Query type (A, AAAA, PTR, etc.)
  - Status (allowed/blocked/cached)
  - Date range
- **Effort**: 6 hours
- **Status**: ✅ Completed
- **Notes**: Storage now supports server-side QueryFilter, `/api/queries` + HTMX UI respect domain/type/status/time inputs, and `/api/ui/queries` refreshes with the active filters.

### Sprint 2: Advanced Analytics (Week 2)

#### Task 2.1: Top Domains Endpoint
- **Endpoint**: `GET /api/stats/top-domains?limit=10&blocked=false`
- **Returns**: Most queried domains
- **Effort**: 3 hours
- **Status**: ✅ Completed
- **Notes**: `/api/top-domains` already exists (`pkg/api/handlers.go:173-214`) with `limit` and `blocked` parameters; only documentation cleanup remains.

#### Task 2.2: Top Blocked Domains Endpoint
- **Endpoint**: `GET /api/stats/top-blocked?limit=10`
- **Returns**: Most blocked domains
- **Effort**: 2 hours
- **Status**: ✅ Completed
- **Notes**: Covered by `/api/top-domains?blocked=true`; storage already supports the filter.

#### Task 2.3: Query Types Distribution Endpoint
- **Endpoint**: `GET /api/stats/query-types`
- **Returns**: Count by query type (A, AAAA, PTR, etc.)
- **Effort**: 2 hours
- **Status**: ✅ Completed
- **Notes**: SQLite aggregates per record type and the API handler serves JSON to power the dashboard doughnut chart.

#### Task 2.4: Enhanced Dashboard Charts
- **ID**: UI-005
- **Files**: `pkg/api/ui/templates/dashboard.html`, `pkg/api/ui/static/css/style.css`
- **New Charts**:
  - Top 10 queried domains (bar chart)
  - Query type distribution (pie chart)
  - Top 10 blocked domains (bar chart)
  - Cache hit rate over time (line chart)
- **Effort**: 6 hours
- **Status**: ✅ Completed
- **Notes**: Dashboard renders all charts using live `/api/top-domains`, `/api/stats/query-types`, and `/api/stats/timeseries` data.

### Sprint 3: Configuration Management (Week 3)

#### Task 3.1: Editable Settings Page (Phase 1)
- **Files**: `pkg/api/handlers_config_update.go`, `pkg/api/settings_page.go`, `pkg/api/ui/templates/settings.html`
- **Endpoints**:
  - `PUT /api/config/upstreams`
  - `PUT /api/config/cache`
  - `PUT /api/config/logging`
- **Features**:
  - Add/remove upstream DNS servers
  - Adjust cache TTL
  - Change log level
- **Validation**: Input validation with error feedback
- **Effort**: 8 hours
- **Status**: ✅ Completed
- **Notes**: Settings page now renders HTMX forms backed by `/api/config/{upstreams,cache,logging}` handlers that validate, persist via `config.Save`, and re-render partials with success/error states.

#### Task 3.2: Hot-Reload Configuration
- **Files**: `cmd/glory-hole/main.go`, `pkg/blocklist/manager.go`
- **Changes**: Trigger resolver/cache/logging refresh when config changes
- **Ensure**: Upstreams update the resolver + HTTP clients, cache settings recreate the cache, and logging swaps loggers without restarts
- **Effort**: 4 hours
- **Status**: ✅ Completed
- **Notes**: Config watcher now recreates resolvers, caches, and loggers after both file edits and UI saves; blocklist manager inherits the new HTTP client.

#### Task 3.3: Rate Limiting Middleware
- **ID**: BUG-004
- **Files**: New `pkg/api/middleware/ratelimit.go`
- **Default**: 100 req/min per IP
- **Configurable**: Via config file
- **Response**: 429 Too Many Requests
- **Effort**: 4 hours
- **Status**: ⬜ Not Started
- **Notes**: No HTTP middleware exists beyond logging/CORS; DNS rate limiting cannot protect the Web UI.

### Sprint 4: Client Management (Week 4)

#### Task 4.1: Client Statistics Endpoint
- **ID**: UI-004
- **Endpoint**: `GET /api/stats/clients?limit=50&offset=0`
- **Returns**: Per-client statistics
- **Data**:
  - Client IP
  - Total queries
  - Blocked percentage
  - Top domains queried
  - Last seen timestamp
- **Effort**: 6 hours
- **Status**: ⬜ Not Started

#### Task 4.2: Clients Page UI
- **Files**: New `pkg/api/ui/templates/clients.html`
- **Route**: `/ui/clients`
- **Features**:
  - Client list with statistics
  - Sortable columns
  - Pagination
  - Search/filter by IP
- **Effort**: 6 hours
- **Status**: ⬜ Not Started

### Sprint 5: Bulk Operations & Export (Week 5)

#### Task 5.1: Bulk Policy Operations
- **ID**: UI-006
- **Files**: `pkg/api/ui/templates/policies.html`
- **Features**:
  - Checkboxes for selection
  - Bulk enable/disable
  - Bulk delete
  - Bulk export
- **HTMX**: Powered updates
- **Effort**: 4 hours
- **Status**: ⬜ Not Started

#### Task 5.2: Export Functionality
- **ID**: UI-008
- **Endpoints**:
  - `GET /api/export/queries?format=csv&start=&end=`
  - `GET /api/export/stats?format=json`
  - `GET /api/export/policies?format=yaml`
- **Formats**: CSV, JSON, YAML
- **Streaming**: For large datasets
- **Effort**: 6 hours
- **Status**: ⬜ Not Started

### Sprint 6: Polish & Enhancements (Week 6)

#### Task 6.1: Dark Mode
- **ID**: UI-007
- **Files**: `pkg/api/ui/static/style.css`
- **Features**:
  - Theme toggle button
  - Dark theme CSS
  - localStorage preference
  - System preference detection
- **Effort**: 4 hours
- **Status**: ⬜ Not Started

#### Task 6.2: Policy Search/Filter
- **ID**: UI-009
- **Files**: `pkg/api/ui/templates/policies.html`
- **Features**:
  - Search box
  - Filter by name, action, status
  - Real-time filtering
- **Effort**: 2 hours
- **Status**: ⬜ Not Started

#### Task 6.3: Enhanced Query Log
- **Files**: `pkg/api/ui/templates/queries.html`
- **Features**:
  - Show response time per query
  - Color-code by status
  - Query details modal
  - Show upstream server used
- **Effort**: 4 hours
- **Status**: ⬜ Not Started

### API Endpoints Summary

**Available today**
```
GET  /api/stats?since=24h
GET  /api/stats/timeseries?period=hour&points=24
GET  /api/top-domains?limit=10&blocked=false
GET  /api/queries?limit=100&offset=0
GET  /api/features
GET  /api/config
PUT  /api/config/upstreams
PUT  /api/config/cache
PUT  /api/config/logging
POST /api/features/blocklist|policies/(disable|enable)
POST /api/blocklist/reload
POST /api/cache/purge
```

**Planned additions for v0.8.0**
```
GET  /api/stats/query-types
GET  /api/stats/clients?limit=50&offset=0
GET  /api/export/queries?format=csv&start=&end=
GET  /api/export/stats?format=json
GET  /api/export/policies?format=yaml
```

### Testing Checklist

- [ ] All API endpoints return correct data
- [ ] Dashboard shows real-time data (no mock)
- [x] Settings page reflects actual configuration
- [ ] Query filtering works correctly
- [ ] Charts render properly with real data
- [x] Configuration changes apply correctly
- [x] Rate limiting prevents abuse
- [ ] Export functionality works for large datasets
- [ ] Dark mode toggles correctly
- [ ] Mobile responsiveness maintained
- [ ] HTMX interactions work smoothly
- [ ] No console errors in browser

### Release Checklist

- [ ] Update version to 0.8.0 in `cmd/glory-hole/main.go`
- [ ] Update CHANGELOG.md with comprehensive feature list
- [ ] Take screenshots for documentation
- [ ] Update README with new UI features
- [ ] Run full test suite
- [ ] Manual UI testing on multiple browsers
- [ ] Performance testing with real load
- [ ] Tag release: `git tag -a v0.8.0 -m "v0.8.0: Functional Web UI"`
- [ ] Push tag: `git push origin v0.8.0`
- [ ] Verify CI/CD pipeline success
- [ ] Verify Docker images published
- [ ] Verify GitHub release created
- [ ] Update documentation with screenshots

---

## Progress Tracking

### v0.7.x Progress

**Overall**: ✅ Completed (delivered across releases v0.7.1 – v0.7.8)

- High-priority fixes (logging, request limits, policy validation, README cleanup) shipped in v0.7.4–v0.7.6.
- Medium-priority work (database migrations, config watcher safety, query pagination) is live today.
- Kill-switch and related UI/telemetry landed in v0.7.5.

### v0.8.0 Progress

**Overall**: 56% (10/18 tasks)

**Sprint 1** (4 tasks): 100% complete
- [x] Time-series API
- [x] Dashboard JS updates
- [x] Dynamic settings page
- [x] Query filtering

**Sprint 2** (4 tasks): 100% complete
- [x] Top domains endpoint (`/api/top-domains`)
- [x] Top blocked endpoint (`/api/top-domains?blocked=true`)
- [x] Query types endpoint
- [x] Enhanced charts

**Sprint 3** (3 tasks): 67% complete
- [x] Editable settings
- [ ] Hot-reload config
- [x] Rate limiting

**Sprint 4** (2 tasks): 0% complete
- [ ] Client stats endpoint
- [ ] Clients page UI

**Sprint 5** (2 tasks): 0% complete
- [ ] Bulk operations
- [ ] Export functionality

**Sprint 6** (3 tasks): 0% complete
- [ ] Dark mode
- [ ] Policy search
- [ ] Enhanced query log

---

## Version History

### v0.7.22 (2025-11-24) - Current Release
- Atomic cache counters and parallel shard cleanup (64× faster maintenance) keep throughput high under load.
- Composite SQLite indexes plus struct alignment optimizations make dashboard/analytics queries 10‑100× faster.
- Performance docs, verification checklist, and CI release workflow completed the optimization push.

### v0.7.8 (2025-11-23)
- SERVFAIL responses now pass through immediately (DNSSEC-compliant forwarder).
- Centralized resolver + HTTP client reuse so blocklist downloads honor upstream DNS.
- Regex/wildcard pattern engine and Pi-hole import CLI.
- Duration-based kill-switch controls exposed via API + UI.

### v0.8.0 (Planned) - Functional Web UI 📋
- Real-time data visualization (no mock data)
- Dynamic settings management
- Advanced analytics dashboard
- Client statistics
- Bulk operations
- Export functionality
- Dark mode support

---

## Notes

- This roadmap is a living document and will be updated as development progresses
- Priority and effort estimates may change based on implementation findings
- Community feedback may influence feature prioritization
- Security issues take precedence over planned features

**Last Review**: 2025-11-24
**Next Review**: Mid-way through v0.8.0 execution (after Sprint 3)
