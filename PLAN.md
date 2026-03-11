# Glory-Hole UI Migration: Astro + shadcn/ui

## Current State

The Glory-Hole dashboard is a server-side rendered UI built with Go `html/template` + HTMX + vanilla JS + a single 3,500-line hand-written CSS file. Everything is embedded into the Go binary via `go:embed`. The UI has 9 pages (Dashboard, Query Log, Policies, Local Records, Conditional Forwarding, Settings, Clients, Blocklists, Login), HTMX partials for dynamic updates, Chart.js for visualizations, and a custom "Surveillance Terminal" dark theme.

## Target State

A self-contained Astro 6 + React 19 + shadcn/ui + Tailwind v4 dashboard that builds to static files, which are then embedded into the Go binary. The Go API server continues to serve both the JSON API and the static UI files. All fonts, icons, and assets are bundled — zero external CDN/Google Fonts requests at runtime.

## Architecture (inspired by gatekeeper/dashboard)

```
pkg/api/ui/dashboard/                  # New Astro project root
├── astro.config.mjs
├── tsconfig.json
├── package.json
├── public/
│   └── favicon.svg
├── src/
│   ├── env.d.ts
│   ├── styles/
│   │   └── globals.css                # Tailwind v4 @theme{} config + custom utilities
│   ├── layouts/
│   │   └── DashboardLayout.astro      # Sidebar + header + main content slot
│   ├── pages/
│   │   ├── index.astro                # Dashboard (overview / stats)
│   │   ├── queries.astro              # Query log
│   │   ├── policies.astro             # Policy management
│   │   ├── localrecords.astro         # Local DNS records
│   │   ├── forwarding.astro           # Conditional forwarding
│   │   ├── settings.astro             # Server settings
│   │   ├── clients.astro              # Client management
│   │   ├── blocklists.astro           # Blocklist management
│   │   └── login.astro                # Login page (no layout)
│   ├── components/
│   │   ├── ui/                        # shadcn/ui primitives
│   │   ├── charts/                    # Recharts-based chart components
│   │   ├── DashboardOverview.tsx       # Stats cards + charts
│   │   ├── QueryLogPage.tsx           # Query log table + filters
│   │   ├── PoliciesPage.tsx           # Policy CRUD + condition editor
│   │   ├── PolicyBuilder.tsx          # Policy rule editor
│   │   ├── ConditionEditor.tsx        # Recursive condition tree editor
│   │   ├── LocalRecordsPage.tsx       # Local records table
│   │   ├── ForwardingPage.tsx         # Conditional forwarding table
│   │   ├── SettingsPage.tsx           # Settings forms
│   │   ├── ClientsPage.tsx            # Client table + groups
│   │   ├── BlocklistsPage.tsx         # Blocklist management
│   │   ├── LoginPage.tsx              # Login form (API key + basic auth)
│   │   ├── TablePagination.tsx        # Shared pagination
│   │   └── JsonHighlight.tsx          # JSON syntax highlighter
│   ├── hooks/
│   │   ├── use-pagination.ts
│   │   └── use-api.ts                 # Typed fetch wrapper
│   └── lib/
│       ├── api.ts                     # Centralized API client
│       ├── utils.ts                   # cn(), colors, helpers
│       └── typography.ts              # Typography constants
└── dist/                              # Build output → embedded by Go
```

## Migration Phases

---

### Phase 0: Project Scaffolding

**Goal:** Set up the Astro project with all tooling, bundled fonts, and the base theme.

#### 0.1 — Initialize Astro Project
- Create `pkg/api/ui/dashboard/` with Astro 6 + React integration
- `astro.config.mjs`: static output mode, `@tailwindcss/vite` plugin, `@astrojs/react`
- `tsconfig.json`: strict mode, `@/*` path alias to `./src/*`
- `package.json`: astro, react 19, tailwind v4, radix primitives, recharts, lucide-react, cva, clsx, tailwind-merge

#### 0.2 — Self-Host Fonts (No External Requests)
- Install `@fontsource/jetbrains-mono` and `@fontsource/space-grotesk` as npm dependencies
- Import woff2 files directly in `globals.css` via `@font-face` declarations
- Configure Tailwind `--font-sans` and `--font-mono` to reference these fonts
- Verify: build output contains font files in `_astro/`, no Google Fonts URLs anywhere

#### 0.3 — Tailwind v4 Theme ("Surveillance Terminal" → "Lovelace-inspired")
Create `src/styles/globals.css` with the adapted color palette:

```css
@import "tailwindcss";

@theme {
  /* Backgrounds */
  --color-background: #0a0e27;
  --color-foreground: #e0e0e0;
  --color-card: #141830;
  --color-card-foreground: #e0e0e0;
  --color-popover: #141830;
  --color-popover-foreground: #e0e0e0;

  /* Primary — neon green (from current theme) */
  --color-primary: #00ff41;
  --color-primary-foreground: #0a0e27;

  /* Secondary */
  --color-secondary: #1a1f3d;
  --color-secondary-foreground: #e0e0e0;

  /* Accents */
  --color-accent: #1e2448;
  --color-accent-foreground: #e0e0e0;
  --color-muted: #1a1f3d;
  --color-muted-foreground: #8888aa;

  /* Semantic */
  --color-destructive: #ff006e;
  --color-border: #2a2f52;
  --color-input: #2a2f52;
  --color-ring: #00ff41;

  /* Sidebar-specific */
  --color-sidebar: #070b1f;
  --color-sidebar-foreground: #e0e0e0;
  --color-sidebar-border: #1a1f3d;
  --color-sidebar-accent: #141830;
  --color-sidebar-accent-foreground: #00ff41;

  /* Extended palette */
  --color-gh-green: #00ff41;
  --color-gh-green-dim: #00cc33;
  --color-gh-pink: #ff006e;
  --color-gh-blue: #4a6cf7;
  --color-gh-cyan: #00d4ff;
  --color-gh-yellow: #ffd866;
  --color-gh-orange: #ff9500;

  /* Chart colors */
  --color-chart-1: #00ff41;
  --color-chart-2: #ff006e;
  --color-chart-3: #4a6cf7;
  --color-chart-4: #ffd866;
  --color-chart-5: #00d4ff;

  /* Fonts */
  --font-sans: "Space Grotesk", system-ui, sans-serif;
  --font-mono: "JetBrains Mono", "Fira Code", monospace;

  /* Radius */
  --radius: 0.5rem;
}
```

Add custom utilities:
```css
.font-data { font-family: var(--font-mono); }
.glow-green { text-shadow: 0 0 10px rgba(0, 255, 65, 0.5), 0 0 20px rgba(0, 255, 65, 0.3); }
```

#### 0.4 — Install shadcn/ui Primitives
Manually create the following in `src/components/ui/` (same pattern as gatekeeper):

| Component | Purpose |
|-----------|---------|
| `button.tsx` | Buttons (default, destructive, outline, secondary, ghost) |
| `badge.tsx` | Status badges (allowed, blocked, cached, etc.) |
| `card.tsx` | Stat cards, content panels |
| `dialog.tsx` | Create/edit modals (policies, records, etc.) |
| `input.tsx` | Form inputs |
| `label.tsx` | Form labels |
| `select.tsx` | Dropdowns (time range, record type, etc.) |
| `separator.tsx` | Visual dividers |
| `skeleton.tsx` | Loading states |
| `table.tsx` | Data tables (queries, records, policies) |
| `tabs.tsx` | Settings tabs, login mode toggle |
| `tooltip.tsx` | Hover info |
| `switch.tsx` | Feature toggles (enable/disable blocklist, policies) |
| `textarea.tsx` | Policy expression editor |
| `dropdown-menu.tsx` | Action menus on table rows |

#### 0.5 — Shared Libraries
- `src/lib/utils.ts`: `cn()` helper, status color maps, chart palette
- `src/lib/typography.ts`: Typography class constants (matching gatekeeper pattern)
- `src/lib/api.ts`: Typed API client wrapping all `/api/*` endpoints
- `src/hooks/use-pagination.ts`: Client-side pagination hook

---

### Phase 1: Layout & Navigation

**Goal:** Replicate the sidebar navigation pattern from gatekeeper, adapted for glory-hole's pages.

#### 1.1 — DashboardLayout.astro
Create the main layout following gatekeeper's pattern:
- Fixed sidebar (w-60) with collapsible section groups
- Top header bar with page title + health indicator
- Main content area with scroll-to-top button
- Mobile hamburger menu with overlay
- Section collapse state persisted to localStorage

#### 1.2 — Navigation Structure
```typescript
const navLinks = [
  { id: "dashboard", label: "Dashboard",    href: "/",                    icon: "grid",     section: "" },
  // DNS
  { id: "queries",   label: "Query Log",    href: "/queries",             icon: "list",     section: "DNS" },
  { id: "policies",  label: "Policies",     href: "/policies",            icon: "shield",   section: "DNS" },
  // Records
  { id: "local",     label: "Local Records", href: "/localrecords",       icon: "database", section: "Records" },
  { id: "forwarding",label: "Forwarding",   href: "/forwarding",          icon: "forward",  section: "Records" },
  // Filtering
  { id: "blocklists",label: "Blocklists",   href: "/blocklists",          icon: "block",    section: "Filtering" },
  { id: "clients",   label: "Clients",      href: "/clients",             icon: "users",    section: "Filtering" },
  // System
  { id: "settings",  label: "Settings",     href: "/settings",            icon: "settings", section: "System" },
];
```

#### 1.3 — Health Check + Auth Status
- Inline script: poll `GET /api/health` every 30s, show green/red dot
- Sidebar footer: show logged-in status (if auth enabled)
- Logout button in sidebar footer

#### 1.4 — Login Page
- Standalone page (no DashboardLayout)
- Tabs: "API Key" / "Username & Password" (matching current login.html)
- Redirect to `?next=` path after login

---

### Phase 2: Dashboard (Overview) Page

**Goal:** Migrate the main dashboard with stat cards and charts.

#### 2.1 — Stat Cards
Replace the current HTMX `stats_partial.html` with React stat cards using shadcn Card:
- Total Queries, Blocked, Allowed, Cached (with percentage)
- Auto-refresh via `setInterval` (configurable, default 10s)
- Skeleton loading states

#### 2.2 — Charts (Recharts Migration)
Replace Chart.js with Recharts (matching gatekeeper's approach):
- **Queries over time** — AreaChart (stacked: allowed, blocked, cached)
- **Query types distribution** — PieChart (A, AAAA, CNAME, MX, etc.)
- **Top blocked domains** — horizontal BarChart
- **Top allowed domains** — horizontal BarChart
- Time range selector (1h, 6h, 24h, 7d, 30d)
- Chart tooltips styled with theme colors

#### 2.3 — System Metrics
- CPU, Memory, Temperature cards (if available)
- Uptime display
- DNS server status indicator

---

### Phase 3: Query Log Page

**Goal:** Full-featured query log with filtering, search, and pagination.

#### 3.1 — Query Table
- shadcn Table with columns: Time, Client, Domain, Type, Status, Latency, Upstream
- Status badges: Allowed (green), Blocked (pink), Cached (blue)
- Click row to expand → decision trace detail

#### 3.2 — Filters & Search
- Search input (domain, client IP)
- Status filter tabs (All, Allowed, Blocked, Cached)
- Query type filter (Select dropdown)
- Time range filter
- Client filter

#### 3.3 — Decision Trace Modal
- Dialog showing the full decision trace for a query
- Policy matches, blocklist hits, cache status
- Styled JSON detail view

#### 3.4 — Pagination
- Reuse `TablePagination` component + `usePagination` hook
- Page size options: 25, 50, 100

---

### Phase 4: Policies Page

**Goal:** Policy management with the advanced condition editor from gatekeeper.

#### 4.1 — Policy Table
- Table: Priority, Name, Expression, Action (Allow/Block), Enabled, Client/Group
- Toggle switch for enable/disable
- Drag-to-reorder priority (stretch goal)

#### 4.2 — Policy Create/Edit Dialog
- Dialog with form fields: Name, Expression (textarea with mono font), Action (select), Client/Group (select), Enabled (switch)
- Expression syntax help/reference tooltip
- Test expression button (calls `POST /api/policies/test`)

#### 4.3 — ConditionEditor (from gatekeeper)
Adapt gatekeeper's `ConditionEditor.tsx` for DNS policy conditions:
- Fields: `client.ip`, `client.name`, `client.group`, `domain`, `query_type`, `time.hour`, `time.weekday`
- Operators: eq, ne, contains, starts_with, ends_with, wildcard, regex, in, not_in
- Recursive AND/OR/NOT groups
- This supplements the expr-lang text expression, not replaces it

#### 4.4 — Policy Import/Export
- Export all policies as JSON
- Import from JSON file

---

### Phase 5: Records & Forwarding Pages

**Goal:** CRUD pages for local records and conditional forwarding.

#### 5.1 — Local Records Page
- Table: Domain, Type (A/AAAA/CNAME/MX/TXT), Value, TTL
- Create dialog with form validation
- Delete with confirmation
- Empty state with "Add your first record" prompt

#### 5.2 — Conditional Forwarding Page
- Table: Domain/Pattern, Upstream Server(s)
- Create dialog
- Delete with confirmation

---

### Phase 6: Blocklists & Clients Pages

#### 6.1 — Blocklists Page
- Table: Source URL, Domain Count, Last Updated, Status
- Reload button (calls `POST /api/blocklist/reload`)
- Domain check input (calls `GET /api/blocklists/check?domain=...`)
- Feature toggle: enable/disable blocklist (with optional duration)

#### 6.2 — Clients Page
- Table: Client IP/Name, Group, Query Count, Last Seen
- Edit client metadata dialog (name, group assignment)
- Client groups management (CRUD)
- Per-client query stats

---

### Phase 7: Settings Page

**Goal:** Unified settings page with tabbed sections.

#### 7.1 — Settings Tabs
Using shadcn Tabs:
- **DNS** — Upstream servers, listen addresses, protocols
- **Cache** — Cache size, TTL settings, purge button
- **Blocklist** — Sources list, refresh interval, enable/disable
- **Logging** — Log level, query log retention, storage reset
- **TLS** — Certificate settings, ACME/DNS-01
- **Authentication** — Enable/disable, change password/API key
- **About** — Version, uptime, build info

#### 7.2 — Settings Forms
- Each tab is a form section with Save button
- Validation feedback on fields
- Success/error toast notifications
- Dangerous actions (storage reset, cache purge) require confirmation dialog

---

### Phase 8: Build Integration

**Goal:** Wire the Astro build into the Go build pipeline.

#### 8.1 — Build Pipeline
Update `Makefile`:
```makefile
.PHONY: ui
ui:
	cd pkg/api/ui/dashboard && npm ci && npm run build

.PHONY: build
build: ui
	go build -ldflags "..." -o glory-hole ./cmd/glory-hole
```

#### 8.2 — Astro Build Config
```javascript
// astro.config.mjs
import { defineConfig } from 'astro/config';
import react from '@astrojs/react';
import tailwindcss from '@tailwindcss/vite';

export default defineConfig({
  output: 'static',
  outDir: '../static/dist',     // Build directly into Go embed path
  integrations: [react()],
  vite: {
    plugins: [tailwindcss()],
  },
});
```

#### 8.3 — Go Embed Update
Update `pkg/api/ui_handlers.go`:
```go
//go:embed ui/static/dist/*
var distFS embed.FS

// Serve the Astro SPA from embedded files
// Keep the legacy /static/* route for backward compat during migration
```

- Static files served at root (Astro generates `index.html` per route)
- API routes unchanged (`/api/*`, `/dns-query`)
- Auth middleware updated to protect new static paths

#### 8.4 — Dockerfile Update
Add Node.js stage for Astro build:
```dockerfile
FROM node:22-alpine AS ui-builder
WORKDIR /app/pkg/api/ui/dashboard
COPY pkg/api/ui/dashboard/package*.json ./
RUN npm ci
COPY pkg/api/ui/dashboard/ ./
RUN npm run build

FROM golang:1.24-alpine AS go-builder
COPY --from=ui-builder /app/pkg/api/ui/static/dist pkg/api/ui/static/dist
# ... rest of Go build
```

#### 8.5 — CI Update
Update `.github/workflows/ci.yml` to install Node.js and build the UI before Go build/test.

---

### Phase 9: Cleanup & Polish

#### 9.1 — Remove Legacy UI
Once all pages are migrated and verified:
- Remove `pkg/api/ui/templates/*.html` (all 17 template files)
- Remove `pkg/api/ui/static/css/style.css` (3,543-line monolith)
- Remove `pkg/api/ui/static/js/` (all vanilla JS modules)
- Remove `pkg/api/ui/static/fonts/` (old font files — now bundled by Astro)
- Remove `pkg/api/ui/static/js/vendor/` (htmx, idiomorph, chart.js)
- Remove HTMX UI partial handlers from Go (`/api/ui/*`)
- Remove Go template rendering functions from `ui_handlers.go`
- Remove `scripts/copy-vendor.js`
- Clean up `package.json` root-level npm deps (htmx, idiomorph, chart.js, fontsource)

#### 9.2 — Accessibility Audit
- Keyboard navigation for all interactive elements
- ARIA labels on icon buttons, status indicators
- Focus trapping in dialogs
- Color contrast verification (WCAG AA minimum)
- Screen reader announcements for dynamic updates

#### 9.3 — Responsive Design
- Sidebar collapses to hamburger menu below 1024px (matching gatekeeper)
- Tables scroll horizontally on mobile
- Stat cards stack vertically on mobile
- Charts resize responsively

#### 9.4 — Performance
- Astro partial hydration: only interactive islands get React (`client:load`)
- Route-level code splitting (each page's React component is a separate chunk)
- Font subsetting (only latin character set)
- All assets hashed for cache busting
- Verify total bundle size < 500KB (gzipped)

---

## Key Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Framework | Astro 6 (static) + React 19 | SSG output embeds into Go binary. React islands for interactivity. Same proven pattern as gatekeeper. |
| CSS | Tailwind v4 (Vite plugin) | CSS-first config, no separate tailwind.config.js. Modern, tree-shaken. |
| Components | shadcn/ui (manual) | No CLI dependency. Copy primitives into `components/ui/`. Full control. |
| Charts | Recharts | React-native charting. Replaces Chart.js (which requires Canvas/imperative API). |
| Icons | lucide-react | Tree-shakeable, consistent with shadcn ecosystem. |
| Fonts | @fontsource (self-hosted) | Zero external requests. Bundled in build output. Keeps JetBrains Mono + Space Grotesk. |
| Theme | Dark-only (like gatekeeper) | Matches the surveillance/terminal aesthetic. Simplifies CSS. Light theme is a stretch goal. |
| State | React useState + useEffect | No global state library needed. Each page fetches its own data. Same pattern as gatekeeper. |
| Auth | Existing Go auth middleware | No changes to auth flow. Login page reimplemented in React. Session cookie continues to work. |
| API | Existing JSON API | No API changes needed. New UI is a pure consumer of existing `/api/*` endpoints. |

## Migration Strategy

**Parallel deployment during migration:**
The new Astro UI and the old HTMX UI can coexist. The Go server can serve both:
- Old UI: `/old/*` (move legacy templates here temporarily)
- New UI: `/*` (Astro static files)
- API: `/api/*` (unchanged)

This allows page-by-page migration with instant rollback. Once all pages are migrated and validated, remove the old UI in Phase 9.

## Estimated Effort

| Phase | Description | Effort |
|-------|-------------|--------|
| 0 | Scaffolding, theme, shadcn | 1-2 days |
| 1 | Layout, nav, login | 1 day |
| 2 | Dashboard/overview | 1-2 days |
| 3 | Query log | 1-2 days |
| 4 | Policies | 2-3 days |
| 5 | Records & forwarding | 1 day |
| 6 | Blocklists & clients | 1-2 days |
| 7 | Settings | 1-2 days |
| 8 | Build integration | 0.5 day |
| 9 | Cleanup & polish | 1-2 days |
| **Total** | | **~10-16 days** |
