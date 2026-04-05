# Frontend Build Process

## Overview

The Glory-Hole dashboard is built with **Astro 6 + React 19 + shadcn/ui + Tailwind v4**. It compiles to static HTML/JS/CSS files that are embedded into the Go binary via `go:embed`. All fonts, icons, and assets are bundled -- zero external CDN or Google Fonts requests at runtime.

## Architecture

```
pkg/api/ui/dashboard/          # Astro project root
├── astro.config.mjs           # Static output, React integration, Tailwind v4
├── package.json               # Dependencies: astro, react, radix, recharts, lucide
├── tsconfig.json              # Strict mode, @/* path alias
├── src/
│   ├── styles/globals.css     # Tailwind v4 @theme{} + custom utilities
│   ├── layouts/DashboardLayout.astro
│   ├── pages/                 # 12 Astro pages (SSG)
│   ├── components/            # React components + shadcn/ui primitives
│   ├── hooks/                 # use-pagination
│   └── lib/                   # API client, utils, typography
└── dist/                      # Build output (NOT committed)

pkg/api/ui/static/dist/        # Astro outDir — embedded by Go
```

## Build

```bash
# Full build (UI + Go binary)
make build

# UI only
make ui

# Development
cd pkg/api/ui/dashboard && npm run dev
```

The `make build` target runs `npm ci && npm run build` in the dashboard directory before compiling the Go binary. The Astro build outputs to `pkg/api/ui/static/dist/`, which is embedded via:

```go
//go:embed all:ui/static
var staticFS embed.FS
```

## Pages (12)

| Page | Path | Component |
|------|------|-----------|
| Dashboard | `/` | `DashboardOverview.tsx` |
| Query Log | `/queries` | `QueryLogPage.tsx` |
| Policies | `/policies` | `PoliciesPage.tsx` |
| Local Records | `/localrecords` | `LocalRecordsPage.tsx` |
| Forwarding | `/forwarding` | `ForwardingPage.tsx` |
| Blocklists | `/blocklists` | `BlocklistsPage.tsx` |
| Clients | `/clients` | `ClientsPage.tsx` |
| Settings | `/settings` | `SettingsPage.tsx` (7 tabs) |
| Resolver Overview | `/resolver` | `ResolverOverviewPage.tsx` |
| Resolver Settings | `/resolver/settings` | `ResolverSettingsPage.tsx` |
| Resolver Zones | `/resolver/zones` | `ResolverZonesPage.tsx` |
| Login | `/login` | Standalone (no layout) |

## Bundle Size

- **JS + CSS gzipped**: ~248KB (target: <500KB)
- **Fonts**: ~136KB (JetBrains Mono + Space Grotesk, latin woff2 only)
- **Total dist/**: ~1.5MB uncompressed

## Key Dependencies

- `astro` 6.x — static site generator with React islands
- `react` 19.x — interactive components via `client:load`
- `@radix-ui/*` — accessible primitives (dialog, select, switch, tabs, tooltip)
- `recharts` — React charting (area, bar, pie)
- `lucide-react` — tree-shakeable icons
- `tailwindcss` 4.x — CSS-first config via Vite plugin
- `@fontsource/jetbrains-mono`, `@fontsource/space-grotesk` — self-hosted fonts

## Development Workflow

1. Start the Go server: `make dev`
2. In another terminal: `cd pkg/api/ui/dashboard && npm run dev`
3. The Astro dev server proxies `/api/*` to the Go server (configure in `astro.config.mjs`)
4. Changes hot-reload in the browser

For production, always run `make build` which builds the UI first, then compiles everything into the Go binary.
