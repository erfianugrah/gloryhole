# Frontend Build Process

## Overview

The Glory-Hole UI uses npm to manage frontend dependencies. All external libraries (HTMX, Chart.js, fonts) are installed via npm and copied to the static directory during build.

## Dependencies

Frontend dependencies are defined in `package.json`:

- **htmx.org** (1.9.12) - HTMX library for dynamic HTML
- **idiomorph** (0.3.0) - Morphing algorithm for HTMX
- **chart.js** (4.4.5) - Charts and graphs
- **@fontsource/jetbrains-mono** - JetBrains Mono font (monospace)
- **@fontsource/space-grotesk** - Space Grotesk font (body text)

## Build Process

### Automatic Build

The Makefile automatically handles the frontend build:

```bash
make build        # Installs npm deps, builds frontend, then builds Go binary
make clean        # Removes all build artifacts including node_modules
make npm-install  # Only install npm dependencies
make npm-build    # Only build frontend assets
```

### Manual Build

If you need to build frontend assets manually:

```bash
npm install                # Install dependencies
npm run build:vendor       # Copy vendor files to static directories
```

## Output Structure

After building, the following files are generated:

```
pkg/api/ui/static/
├── js/
│   └── vendor/
│       ├── htmx.min.js           (48 KB)
│       ├── idiomorph-ext.min.js  (8.4 KB)
│       └── chart.umd.min.js      (209 KB)
└── fonts/
    ├── jetbrains-mono-latin-*.woff2  (4 files, ~84 KB total)
    ├── space-grotesk-latin-*.woff2   (4 files, ~52 KB total)
    └── fonts.css                      (font-face declarations)
```

Total size: **~401 KB** (compared to 801 KB with TTF files)

## Go Embedding

These files are embedded into the Go binary at compile time using `go:embed`:

```go
//go:embed ui/static/*
var staticFS embed.FS
```

The binary remains self-contained with zero runtime dependencies.

## Development Workflow

1. **First time setup:**
   ```bash
   make build
   ```

2. **Update dependencies:**
   ```bash
   # Edit package.json, then:
   make clean
   make build
   ```

3. **Add new vendor library:**
   - Add to `package.json` dependencies
   - Update `scripts/copy-vendor.js` to copy the files
   - Run `make build`

## Why npm?

Using npm provides several benefits over manual CDN downloads:

- **Version management**: Lock specific versions with package.json
- **Reproducible builds**: package-lock.json ensures consistent builds
- **Easy updates**: `npm update` to get latest compatible versions
- **Better fonts**: @fontsource provides optimized woff2 files (smaller, faster)
- **No external dependencies at runtime**: Everything embedded in Go binary

## CI/CD Integration

The Makefile integrates seamlessly with CI/CD:

```yaml
# Example GitHub Actions
- name: Build
  run: make build

# npm install and frontend build happen automatically
```

## Troubleshooting

**Error: "npm: command not found"**
- Install Node.js: https://nodejs.org/

**Error: "Module not found"**
- Run `npm install` first
- Check `package.json` for correct dependencies

**Fonts not loading:**
- Verify files in `pkg/api/ui/static/fonts/`
- Check browser console for 404 errors
- Rebuild: `make clean && make build`
