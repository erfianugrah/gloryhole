# Glory-Hole Security & Performance Audit — Remediation Plan

> Generated: 2026-04-04
> Scope: Full codebase (~109 Go files, 49 frontend files, all config/deploy)
> Every finding below has been verified against the actual source code.

---

## Table of Contents

1. [Findings Summary](#findings-summary)
2. [CRITICAL Findings](#critical-findings)
3. [HIGH Findings](#high-findings)
4. [MEDIUM Findings](#medium-findings)
5. [LOW Findings](#low-findings)
6. [Performance Hotspots](#performance-hotspots)
7. [Efficiency Audit — Fly.io shared-cpu-1x / 512MB](#efficiency-audit)
8. [Feature: Local Records Edit Capability](#feature-local-records-edit)
9. [Positive Security Patterns](#positive-security-patterns)
10. [Remediation Phases](#remediation-phases)

---

## Findings Summary

| Category | Count | Verified |
|----------|-------|----------|
| CRITICAL (Security) | 2 | 2/2 |
| HIGH (Security) | 6 | 6/6 |
| MEDIUM (Security) | 14 | 14/14 |
| LOW (Security) | 10 | 10/10 |
| CRITICAL (Efficiency) | 3 | 3/3 |
| HIGH (Efficiency) | 3 | 3/3 |
| MEDIUM (Efficiency) | 3 | 3/3 |
| Feature Gap | 1 | 1/1 |
| **Total** | **42** | **42/42** |

---

## CRITICAL Findings

### C1. No Rate Limiting on API or Login Endpoints

**Status:** VERIFIED
**Files:** `pkg/api/api.go:237-242`, `pkg/api/middleware.go`
**Attack vector:** Brute-force `/login`, DoS on expensive endpoints

**Evidence:**
The middleware chain at `api.go:238-242` is:
```
handler = authMiddleware(handler)
handler = loggingMiddleware(handler)
handler = securityHeadersMiddleware(handler)
handler = corsMiddleware(handler)
handler = blockPageMiddleware(handler)
```
No rate limiter exists anywhere in the chain. The login handler at `ui_handlers.go:82` accepts unlimited POST attempts. Destructive endpoints like `POST /api/storage/reset` (handlers_storage.go:29), `POST /api/cache/purge`, and `POST /api/blocklist/reload` have no throttling.

The HTTP server has `ReadTimeout: 10s` and `WriteTimeout: 10s` (api.go:250-251), which limits slow-loris but not rapid-fire requests.

**Remediation plan:**
1. Add a rate limiting middleware using a token bucket (e.g., `golang.org/x/time/rate` or per-IP map with `sync.Map`)
2. Apply strict limits to `/login`: 5 attempts per minute per IP
3. Apply moderate limits to API endpoints: 30-60 req/s per IP
4. Apply tight limits to destructive endpoints: 1 req/10s for `/api/storage/reset`, `/api/cache/purge`
5. Return `429 Too Many Requests` with `Retry-After` header

**Implementation location:** New file `pkg/api/middleware_ratelimit.go`, wire into `api.go:238`

**Effort:** Medium (new middleware + config options)

---

### C2. No CSRF Protection on State-Mutating Operations

**Status:** VERIFIED
**Files:** `pkg/api/ui_handlers.go:82-121`, all POST/PUT/DELETE handlers
**Attack vector:** Cross-site form submission to reset DB, purge cache, modify policies

**Evidence:**
Session cookies use `SameSite=Lax` (`session.go:149`) but no CSRF token mechanism exists. The login form at `ui_handlers.go:82` parses form data directly. All API mutations (policy create/delete, config update, storage reset, cache purge, unbound config changes) rely solely on the session cookie.

`SameSite=Lax` mitigates most CSRF but:
- Allows top-level navigation POST (form action= to another site)
- Not enforced by all browsers (especially older ones)
- Does not protect same-site (subdomain) attacks

**Remediation plan:**
1. For API endpoints (`/api/*`): Require a custom header `X-Requested-With: XMLHttpRequest` on all POST/PUT/DELETE. CORS preflight will block cross-origin requests from adding custom headers. The frontend `api.ts` already sets `Content-Type: application/json` which triggers preflight.
2. For form-based endpoints (`/login`, `/logout`): Generate a per-session CSRF token, embed in a hidden form field, validate on POST.
3. Add `csrf.go` for token generation/validation.
4. Update `api.ts` to send the custom header on all mutating requests.

**Implementation locations:**
- New file `pkg/api/middleware_csrf.go`
- Edit `pkg/api/ui_handlers.go` (login/logout forms)
- Edit `pkg/api/ui/dashboard/src/lib/api.ts` (add custom header)

**Effort:** Medium

---

## HIGH Findings

### H1. `defer clientPool.Put` Inside Retry Loop — Pool Drain

**Status:** VERIFIED
**Files:** `pkg/forwarder/forwarder.go:122-123` (Forward), `pkg/forwarder/forwarder.go:274-275` (ForwardWithUpstreams)
**Impact:** Under retries, N clients are taken from pool but only returned at function exit

**Evidence:**
```go
// forwarder.go:113-123
for i := 0; i < attempts; i++ {
    upstream, err := f.selectUpstream()
    // ...
    client := f.clientPool.Get().(*dns.Client)
    defer f.clientPool.Put(client)  // BUG: runs at function exit, not loop end
```
Same pattern at line 274-275 in `ForwardWithUpstreams`.

With `attempts=3` and high concurrency, this drains the pool. Each concurrent query doing retries holds 3 clients simultaneously instead of 1. The `sync.Pool` must allocate fresh `dns.Client` objects.

**Remediation plan:**
Replace `defer` with explicit `Put` at the end of each iteration and before each `continue`:
```go
for i := 0; i < attempts; i++ {
    client := f.clientPool.Get().(*dns.Client)
    // ... use client ...
    f.clientPool.Put(client)
    if queryErr != nil {
        continue
    }
    return resp, nil
}
```
Apply to both `Forward()` and `ForwardWithUpstreams()`.

**Effort:** Low (straightforward refactor, ~20 lines changed)

---

### H2. Unbound Config Template Injection

**Status:** VERIFIED
**File:** `pkg/unbound/writer.go:43-44, 62-63, 161-163`
**Attack vector:** API user injects arbitrary unbound.conf directives via newlines in string fields

**Evidence:**
The template at `writer.go` uses `text/template` (not `html/template`), which does NO escaping:
```
    access-control: {{ .Netblock }} {{ .Action }}         // line 44
    domain-insecure: "{{ . }}"                             // line 63
    name: "{{ .Name }}"                                    // line 161
    forward-addr: {{ . }}                                  // line 163
```

The API endpoints that write to these fields:
- `handleAddForwardZone` (handlers_unbound.go:145-179) — accepts `Name` and `ForwardAddrs` via JSON
- `handleUpdateForwardZone` (handlers_unbound.go:182-210) — same
- `handleUpdateUnboundServer` (handlers_unbound.go:103-131) — accepts `AccessControl`, `DomainInsecure`

An attacker with API access can set `Name` to:
```
"\n    include: /etc/shadow\n    name: "
```
This injects `include:` directives into the rendered unbound.conf.

`unbound-checkconf` (validator.go) runs AFTER writing, which catches syntax errors but NOT semantically valid malicious config.

**Remediation plan:**
1. Add input validation in `pkg/unbound/validator.go`:
   - Reject strings containing `\n`, `\r`, `"`, `\` 
   - Validate netblocks match `^[0-9a-fA-F.:\/]+$`
   - Validate domain names match `^[a-zA-Z0-9._-]+\.?$`
   - Validate forward-addrs match `^[0-9a-fA-F.:@#]+$` (IP:port format)
2. Call validation BEFORE template rendering in `WriteConfig()`
3. Add validation in API handlers before calling `applyUnboundConfig()`

**Effort:** Medium

---

### H3. Unbounded Blocklist Download — Memory Exhaustion

**Status:** VERIFIED
**File:** `pkg/blocklist/blocklist.go:52-62`
**Attack vector:** Malicious/compromised blocklist URL serves multi-GB response

**Evidence:**
```go
// blocklist.go:52-62
resp, err := d.client.Do(req)
// ...
domains, err := d.parseHostsFile(resp.Body)  // reads entire body via bufio.Scanner
```
`parseHostsFile` uses `bufio.Scanner` which reads line by line but accumulates results in a `map[string]struct{}`. There is no limit on the response body size or the number of domains parsed.

The HTTP client has a 60-second timeout (blocklist.go:38) but a fast connection can deliver gigabytes in 60 seconds.

**Remediation plan:**
1. Wrap `resp.Body` with `io.LimitReader`:
   ```go
   maxSize := int64(100 * 1024 * 1024) // 100MB
   limitedBody := io.LimitReader(resp.Body, maxSize)
   domains, err := d.parseHostsFile(limitedBody)
   ```
2. Add a max domain count in `parseHostsFile` (e.g., 5 million)
3. Make limits configurable in `BlocklistConfig`

**Effort:** Low (~5 lines)

---

### H4. Trusted Proxy Headers Without Validation

**Status:** VERIFIED
**File:** `pkg/api/api.go:420-444`
**Attack vector:** IP spoofing via `X-Forwarded-For` header from any client

**Evidence:**
```go
// api.go:421-431
func (s *Server) getClientIP(r *http.Request) string {
    if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
        ips := strings.Split(xff, ",")
        if len(ips) > 0 {
            clientIP := strings.TrimSpace(ips[0])
            if clientIP != "" {
                return clientIP  // trusts ANY client
            }
        }
    }
    if xri := r.Header.Get("X-Real-IP"); xri != "" {
        return strings.TrimSpace(xri)
    }
```

Any client can set `X-Forwarded-For: 10.0.0.1` and impersonate an internal IP. This affects:
- Query logging (incorrect client attribution)
- Session tracking
- Any future IP-based rate limiting

**Remediation plan:**
1. Add `trusted_proxies` config field (list of CIDRs)
2. Only trust `X-Forwarded-For`/`X-Real-IP` when `r.RemoteAddr` is in `trusted_proxies`
3. When trusted, use rightmost-untrusted IP from XFF chain (not leftmost)
4. Default: empty (don't trust proxy headers)

**Implementation:**
- Edit `pkg/config/config.go` — add `TrustedProxies []string` to `ServerConfig`
- Edit `pkg/api/api.go:420-444` — conditional proxy header trust

**Effort:** Medium

---

### H5. Logging `file_path` Config Update Allows Arbitrary File Write

**Status:** VERIFIED
**File:** `pkg/api/handlers_config_update.go:123`
**Attack vector:** Authenticated user sets log output to arbitrary file path

**Evidence:**
```go
// handlers_config_update.go:119-123
updated := *cfg
updated.Logging.Level = payload.Level
updated.Logging.Format = payload.Format
updated.Logging.Output = payload.Output
updated.Logging.FilePath = payload.FilePath  // No path validation
```

The `MaxBytesReader` is applied (line 112) but the path itself is not validated. An attacker with dashboard access could set `file_path` to `/etc/crontab`, overwriting system files with log output.

Same issue with TLS config at `handlers_config_update.go:588-593`:
```go
result.TLS.CertFile = req.CertFile   // No path validation
result.TLS.KeyFile = req.KeyFile     // No path validation
```

**Remediation plan:**
1. Validate all file paths in config update handlers:
   - Resolve to absolute path
   - Reject paths containing `..`
   - Optionally restrict to an allowed base directory
2. Add `validateFilePath()` helper in `handlers_config_update.go`
3. Apply to: `FilePath` (logging), `CertFile`/`KeyFile` (TLS), `CacheDir` (ACME)

**Effort:** Low

---

### H6. Session Cookie `Secure` Flag Ignores Reverse Proxy TLS

**Status:** VERIFIED
**File:** `pkg/api/session.go:138, 164`
**Attack vector:** Session cookie sent over HTTP when behind reverse proxy

**Evidence:**
```go
// session.go:138
secure := r != nil && r.TLS != nil
```
Behind Fly.io, Cloudflare Tunnel, nginx, or any TLS-terminating proxy, `r.TLS` is `nil` even though the external connection is HTTPS. The session cookie is then set without the `Secure` flag, allowing it to be transmitted over unencrypted connections.

Same pattern at line 164 (revoke path).

**Remediation plan:**
1. Check `X-Forwarded-Proto` header when behind a trusted proxy:
   ```go
   secure := r != nil && (r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https")
   ```
2. Gate this on the `trusted_proxies` config (from H4) to prevent spoofing
3. Add a `force_https_cookies` config option as an alternative

**Effort:** Low (ties into H4 trusted proxy work)

---

## MEDIUM Findings

### M1. Open Resolver by Default

**Status:** VERIFIED
**File:** `pkg/dns/acl.go:22-26`
**Evidence:** Empty `allowed_clients` sets `acl.empty = true`, which means `IsAllowed()` returns true for all IPs. DoT explicitly bypasses ACL (server_impl.go has separate handler without ACL check).

**Remediation:**
- Log a WARNING at startup when ACL is open: "DNS server is operating as an open resolver — configure allowed_clients to restrict access"
- Add documentation callout in config.example.yml
- Consider requiring explicit `allowed_clients: ["0.0.0.0/0"]` for intentional open mode

**Effort:** Low

---

### M2. DNS Amplification — No Response Size Limiting

**Status:** VERIFIED
**Files:** `pkg/dns/handler.go` (ServeDNS flow), `pkg/dns/edns.go:9-16`
**Evidence:** EDNS0 buffer capped at 4096 but no enforcement that UDP responses don't exceed the client's advertised buffer size. No TC bit forced on oversized responses.

**Remediation:**
- In `writeMsg`, check if transport is UDP and response size exceeds EDNS0 buffer (or 512 without EDNS0)
- If so, set TC bit and truncate to force TCP retry
- Combined with rate limiting (C1), this significantly reduces amplification risk

**Effort:** Medium

---

### M3. CORS Wildcard + Credentials

**Status:** VERIFIED
**File:** `pkg/api/middleware.go:14-16, 38-41`
**Evidence:**
```go
if allowed == "*" || origin == allowed {  // line 39
    return true
}
// Then:
w.Header().Set("Access-Control-Allow-Origin", origin)       // reflects actual origin
w.Header().Set("Access-Control-Allow-Credentials", "true")  // with credentials
```
When `cors_allowed_origins: ["*"]`, any website can make authenticated API requests.

**Remediation:**
- When `allowed == "*"`, do NOT set `Access-Control-Allow-Credentials: true`
- Add `Vary: Origin` header when reflecting origin
- Log warning at startup if wildcard + auth is configured

**Effort:** Low

---

### M4. Missing `MaxBytesReader` on Multiple Request Bodies

**Status:** VERIFIED
**Files and lines (no MaxBytesReader applied):**
| Handler | File:Line | Method |
|---------|-----------|--------|
| handleUpdateUnboundServer | handlers_unbound.go:107 | `io.ReadAll(r.Body)` |
| handleAddForwardZone | handlers_unbound.go:147 | `json.NewDecoder(r.Body)` |
| handleUpdateForwardZone | handlers_unbound.go:186 | `json.NewDecoder(r.Body)` |
| handleUpdateClient | handlers_clients.go:95 | `json.NewDecoder(r.Body)` |
| upsertClientGroup | handlers_clients.go:170 | `json.NewDecoder(r.Body)` |
| handleStorageReset | handlers_storage.go:29 | `json.NewDecoder(r.Body)` |
| handleAddLocalRecord | handlers_localrecords.go:102 | `io.ReadAll(r.Body)` |
| handleAddConditionalForwarding | handlers_conditionalforwarding.go:102 | `io.ReadAll(r.Body)` |

Note: `handleAddPolicy` (line 162) and config update handlers DO apply `MaxBytesReader` correctly.

**Remediation:**
- Add `r.Body = http.MaxBytesReader(w, r.Body, 1*1024*1024)` at the top of each handler
- Or create a middleware that applies a default max body size to all non-file-upload routes

**Effort:** Low (mechanical, ~8 one-line additions)

---

### M5. Unbounded Regex Cache in Policy Engine

**Status:** VERIFIED
**File:** `pkg/policy/engine.go:399, 404-416`
**Evidence:** `var regexCache sync.Map` grows without limit. Each unique regex pattern is cached forever. While patterns typically come from config-defined policies, the cache is never evicted.

**Remediation:**
- Replace `sync.Map` with a bounded LRU cache (e.g., 1000 entries)
- Or accept the current behavior since patterns are config-defined and count is bounded by policy count
- Add a `regexCacheSize` metric for monitoring

**Effort:** Low (if accepting current behavior with monitoring)

---

### M6. `DomainMatches` Uses Substring Match

**Status:** VERIFIED
**File:** `pkg/policy/engine.go:368-375`
**Evidence:**
```go
func DomainMatches(domain, pattern string) bool {
    domain = strings.ToLower(domain)
    pattern = strings.ToLower(pattern)
    if strings.Contains(domain, pattern) {  // "ad" matches "readme.io"
        return true
    }
```

**Remediation:**
- Change the `strings.Contains` check to require dot-boundary or exact matching:
  ```go
  // Exact match
  if domain == pattern || domain == pattern+"." {
      return true
  }
  // Subdomain match: pattern "example.com" matches "sub.example.com"
  if strings.HasSuffix(domain, "."+pattern) || strings.HasSuffix(domain, "."+pattern+".") {
      return true
  }
  ```
- This is a **breaking change** for users relying on substring behavior — needs migration path
- Consider a `DomainContains` function for explicit substring matching

**Effort:** Medium (behavioral change, needs testing)

---

### M7. Cache Key Missing DNSSEC Flags

**Status:** VERIFIED
**File:** `pkg/cache/cache.go:343-358`
**Evidence:** Cache key is `domain:qtype`. Does not include CD (Checking Disabled) or DO (DNSSEC OK) bits. A response cached from a CD=1 query could be served to a CD=0 client.

**Remediation:**
- Include DO and CD bits in cache key:
  ```go
  func (c *Cache) makeKey(domain string, qtype uint16, do, cd bool) string {
  ```
- Update all `makeKey` callers in both `cache.go` and `sharded_cache.go`

**Effort:** Medium

---

### M8. Blocklist URL SSRF Potential

**Status:** VERIFIED (two locations)
**Files:**
- `pkg/blocklist/blocklist.go:47` — `http.NewRequestWithContext(ctx, "GET", url, nil)` — no scheme validation
- `pkg/api/handlers_blocklists.go:134` — `url.ParseRequestURI(trimmed)` — accepts `file://`, `javascript:`, etc.

**Remediation:**
1. In `handlers_blocklists.go`, validate scheme:
   ```go
   u, err := url.Parse(trimmed)
   if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
       // reject
   }
   ```
2. In `blocklist.go:47`, validate before creating request:
   ```go
   if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
       return nil, fmt.Errorf("unsupported URL scheme")
   }
   ```
3. Optionally block RFC1918/link-local/loopback addresses in blocklist downloads

**Effort:** Low

---

### M9. Error Messages Leak Internal Details

**Status:** VERIFIED
**Locations:**
| File:Line | Leaks |
|-----------|-------|
| handlers_policy.go:122 | `"Failed to load policies: "+err.Error()` |
| handlers_policy.go:172 | `fmt.Sprintf("Invalid JSON: %v", err)` |
| handlers_policy.go:188 | `fmt.Sprintf("Failed to compile expression: %v", err)` |
| handlers_unbound.go:79 | `"Failed to retrieve Unbound stats: "+err.Error()` |
| handlers_unbound.go:113 | `"Invalid JSON: "+err.Error()` |
| handler_doh.go:458-459 | `err.Error()` in JSON response |
| handlers_clients.go:96 | `fmt.Sprintf("Invalid payload: %v", err)` |
| handlers_config_update.go:108 | `err.Error()` for config unavailable |

**Remediation:**
- Log detailed errors server-side at ERROR level
- Return generic messages to clients: "Invalid request", "Internal error", "Service unavailable"
- Exception: validation errors (policy compile) can return user-facing detail since the input is from the same user

**Effort:** Low (mechanical)

---

### M10. SQLite Database File Permissions Not Explicitly Set

**Status:** VERIFIED
**File:** `pkg/storage/sqlite.go:64`
**Evidence:** `sql.Open("sqlite", cfg.SQLite.Path)` creates the database with default umask permissions (typically 0644). The database contains client IPs, queried domains, and block traces — all PII.

**Remediation:**
Add after database open:
```go
if err := os.Chmod(cfg.SQLite.Path, 0600); err != nil {
    logger.Warn("Failed to set database permissions", "error", err)
}
```

**Effort:** Low (1 line)

---

### M11. Dnstap Socket World-Writable (0666)

**Status:** VERIFIED
**File:** `pkg/unbound/dnstap_reader.go:54-55`
**Evidence:**
```go
if err := os.Chmod(r.socketPath, 0666); err != nil {
```
Any local process can connect and inject fabricated dnstap messages, polluting query logs.

**Remediation:**
- Change to 0660 and set group to the Unbound process group
- Or use 0600 and run Unbound as the same user

**Effort:** Low

---

### M12. Prometheus Metrics Unauthenticated

**Status:** VERIFIED
**File:** `pkg/telemetry/telemetry.go:159-179`
**Evidence:** Separate HTTP server on port 9090 with `promhttp.Handler()` and no auth middleware. Exposes query volumes, blocking rates, cache stats, client counts, and infrastructure health.

**Remediation:**
- Document that port 9090 should be firewalled in production
- Optionally add basic auth support for metrics endpoint
- Consider serving metrics on the main API port behind auth

**Effort:** Low (documentation) / Medium (auth)

---

### M13. Missing HSTS Header

**Status:** VERIFIED
**File:** `pkg/api/middleware.go:47-57`
**Evidence:** `securityHeadersMiddleware` sets X-Content-Type-Options, X-Frame-Options, X-XSS-Protection, Referrer-Policy, CSP — but not Strict-Transport-Security.

**Remediation:**
Add conditional HSTS:
```go
if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
    w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
}
```

**Effort:** Low (2 lines)

---

### M14. Login Error Reflection for Social Engineering

**Status:** VERIFIED
**File:** `pkg/api/ui/dashboard/src/pages/login.astro:128-134`
**Evidence:**
```js
const error = params.get("error");
if (error) {
    errEl.textContent = error;  // safe from XSS (textContent, not innerHTML)
```
But allows crafting phishing URLs: `/login?error=Account+locked.+Call+1-800-SCAM`

**Remediation:**
Replace with error code enum:
```js
const errorMessages = {
    "invalid": "Invalid credentials",
    "form": "Invalid form submission",
    "expired": "Session expired",
};
const code = params.get("error");
if (code && errorMessages[code]) {
    errEl.textContent = errorMessages[code];
}
```
Update Go handlers to redirect with error codes instead of messages.

**Effort:** Low

---

## LOW Findings

### L1. `RATE_LIMIT` Policy Action Is a No-Op

**Status:** VERIFIED — `pkg/dns/handler_policy.go:320-330`
The function logs a warning and returns `false`. Users configuring `RATE_LIMIT` policies get no enforcement.

**Remediation:** Either implement rate limiting or remove the action type and reject it during policy validation.

---

### L2. Type Assertions in Policy Expression Functions Can Panic

**Status:** VERIFIED — `pkg/policy/engine.go:95-97`
```go
func(params ...any) (any, error) {
    return DomainMatches(params[0].(string), params[1].(string)), nil
},
```
If `expr` library has a bug or type mismatch, this panics and crashes the goroutine.

**Remediation:** Use safe assertions: `s, ok := params[0].(string)`. Or add `recover()` in `Evaluate()`.

---

### L3. Non-Sharded Cache Cleanup Holds Write Lock for Full Scan

**Status:** VERIFIED — `pkg/cache/cache.go:467-485`
Iterates entire map under `c.mu.Lock()`. With large caches, this blocks all lookups.

**Remediation:** Use sharded cache by default, or implement incremental cleanup.

---

### L4. Log Rotation Not Implemented

**Status:** VERIFIED — `pkg/logging/logger.go:30-31`
Comment says "In production, you might want to use lumberjack for rotation" but rotation config fields (`MaxSize`, `MaxBackups`, `MaxAge`) exist in config without any implementation.

**Remediation:** Integrate `gopkg.in/natefinished/lumberjack.v2` for log rotation. The config fields already exist.

---

### L5. `warningLogged` Bool Is Racy Under RLock

**Status:** VERIFIED — `pkg/storage/sqlite.go:184,190-193`
Multiple goroutines under `RLock` can read and write `warningLogged` concurrently. Not a data corruption risk but can produce duplicate log warnings.

**Remediation:** Use `atomic.Bool` instead of plain `bool`.

---

### L6. Config Watcher `OnChange` Has Unsynchronized Write

**Status:** VERIFIED — `pkg/config/watcher.go:61-62`
`w.onChange = fn` writes without mutex, while `Start()` reads from the event loop goroutine. Currently safe by calling order (OnChange before Start) but not enforced.

**Remediation:** Hold `w.mu.Lock()` in `OnChange()`.

---

### L7. Global Logger Pointer Has No Synchronization

**Status:** VERIFIED — `pkg/logging/logger.go:131,138-140`
`global = logger` is a plain pointer write. Set once at startup and then only read, so practically safe.

**Remediation:** Use `atomic.Pointer[Logger]` or `sync.Once`.

---

### L8. Legacy Log Channel Silently Drops Queries

**Status:** VERIFIED — `pkg/dns/handler.go:42-43, 348-357`
Channel capacity is 10000. Under DDoS, queries are dropped with only a log warning. The 4 `init()` workers (line 45-49) are never stopped — goroutine leak in tests.

**Remediation:**
- Track dropped query count as a metric (already done via `metrics.AddDroppedQuery`)
- Stop workers on shutdown or use the newer `QueryLogger` path exclusively

---

### L9. CSP Allows `unsafe-inline` for Scripts

**Status:** VERIFIED — `pkg/api/middleware.go:54`
Required by Astro's inline `<script>` blocks in `DashboardLayout.astro:291-418`.

**Remediation:** Migrate to nonce-based CSP or move inline scripts to external files. High effort; low priority.

---

### L10. Plaintext Password Backward Compatibility

**Status:** VERIFIED — `pkg/api/api.go:53` (`basicPass string`), `pkg/api/middleware_auth.go:166-170`
Constant-time comparison is used, which is good. But plaintext passwords in config files are a risk.

**Remediation:** Set deprecation timeline, log warning at startup (already done at api.go:287-288), eventually remove.

---

## Performance Hotspots

### P1. Forwarder Pool Drain (= H1)
Already covered. `defer` in loop holds N clients.

### P2. Non-Sharded Cache Lock Contention (= L3)
Full-map scan under write lock during cleanup.

### P3. Three Serial Stat Transactions Per Flush
**File:** `pkg/storage/sqlite.go` — `domainStatsWorker`, `updateClientStats`, `updateHourlyStats`
Each opens a separate transaction per flush cycle. Under high QPS, these serial writes compete for SQLite's single writer.

**Remediation:** Combine into a single transaction per flush cycle.

### P4. No TCP Fallback on Truncation
**File:** `pkg/forwarder/forwarder.go:86-91`
Default forwarding always uses UDP. `ForwardTCP` exists but is never called. Truncated responses (TC bit) are returned as-is, causing resolution failures for large records (DKIM, DNSSEC).

**Remediation:** After UDP exchange, if `resp.Truncated == true`, retry with TCP automatically.

### P5. 2-Second Timeout for All Transports
**File:** `pkg/forwarder/forwarder.go:66`
Same 2s timeout for UDP and TCP. TCP handshake + TLS + query can easily exceed 2s under load.

**Remediation:** Make timeout configurable per transport. Default: 2s UDP, 5s TCP.

---

## Efficiency Audit — Fly.io shared-cpu-1x / 512MB {#efficiency-audit}

> Instance: `shared-cpu-1x`, 512MB RAM, `GOMEMLIMIT=384MiB` (fly.toml:58-59)
> All numbers verified against actual source code.

### Memory Budget at Steady State

| Component | Estimated RSS | Source |
|-----------|--------------|--------|
| Go runtime + binary (text, BSS, heap metadata) | ~25-30 MB | — |
| Embedded Astro dashboard (`go:embed`) | ~5-10 MB | `pkg/api/ui/static/` |
| **Blocklist** (100K domains) | **~13 MB** | `manager.go:219` — `map[string]BlockEntry`, ~107 bytes/domain |
| DNS Cache (10K entries default, sharded 64) | ~4 MB | `sharded_cache.go:104` — ~400 bytes/entry |
| Shard overhead (64 shards + pre-alloc maps) | ~0.2 MB | `sharded_cache.go:100` |
| **QueryLogger channel** (50K slots) | **~0.4 MB** | `main.go:298` — `make(chan *QueryLog, 50000)` |
| **Legacy log channel** (10K slots, always alloc) | **~0.24 MB** | `handler.go:43` — `make(chan legacyLogRequest, 10000)` |
| **SQLite page cache** (`cache_size=-4096`) | **~4 MB** | `config.go:408` |
| **SQLite mmap** (resident portion) | **~10-50 MB** | `config.go:411` — `mmap_size=268435456` (256MB virtual!) |
| SQLite WAL file | ~1-4 MB | WAL mode default |
| Storage channels (buffer+domainStats+unbound) | ~0.1 MB | `sqlite.go:127-129` |
| Telemetry (OTel + Prometheus) | ~0.3 MB | `telemetry.go` — 14 instruments |
| Policy engine (10 compiled rules) | ~0.04 MB | `engine.go:91` |
| Goroutine stacks (~28 goroutines) | ~0.1 MB | ~4KB each |
| Unbound ReplyBuffer (2000 entries) | ~0.14 MB | `main.go:656` |
| **Unbound child process** | **~80-150 MB** | defaults.go: 32m msg + 64m rrset + 16m key cache + 2 threads |
| HTTP clients, TLS state, config, misc | ~2 MB | — |
| | | |
| **Total WITHOUT Unbound** | **~60-120 MB** | |
| **Total WITH Unbound** | **~140-270 MB** | Leaves 240-370 MB for 512 MB instance |

### CRITICAL Efficiency Issues (Verified)

#### E1. SQLite `mmap_size` Default Is 256MB — RSS Bloat on Constrained Instance

**Status:** VERIFIED
**File:** `pkg/config/config.go:410-411`

```go
if c.Database.SQLite.MMapSize == 0 {
    c.Database.SQLite.MMapSize = 268435456 // 256MB
}
```

While mmap is virtual (not all RSS until faulted), the kernel maps the entire DB file. On a busy DNS server, hot pages stay resident. With a multi-MB query log DB, this easily consumes 30-80MB RSS. On a 512MB instance with `GOMEMLIMIT=384MiB`, this is the single largest source of RSS bloat.

**Remediation:**
- Detect available memory at startup (or via config) and scale mmap accordingly
- For Fly.io 512MB: default to `32 * 1024 * 1024` (32MB)
- Add a `fly` or `low_memory` config profile that sets conservative defaults

**Effort:** Low (change one default)

---

#### E2. Unbound Default Cache Settings Are Too Large for 512MB

**Status:** VERIFIED
**File:** `pkg/unbound/defaults.go:13-15`

```go
MsgCacheSize:   "32m",   // 32MB
RRSetCacheSize: "64m",   // 64MB
KeyCacheSize:   "16m",   // 16MB
NumThreads:     2,
```

Total Unbound cache: **112MB** allocated at startup. Combined with Unbound's base footprint (~30MB), the child process consumes **~140MB** before resolving a single query. Plus `NumThreads: 2` on a single shared vCPU adds context switch overhead.

**Remediation:**
For Fly.io `shared-cpu-1x`:
```go
MsgCacheSize:   "4m",
RRSetCacheSize: "8m",
KeyCacheSize:   "4m",
NumThreads:     1,
OutgoingRange:  512,       // down from 4096
NumQueriesPerThread: 256,  // down from 1024
```
This drops Unbound from ~140MB to ~46MB.

**Effort:** Low (new defaults or config profile)

---

#### E3. 64 Cache Shards With Parallel Cleanup — CPU Spikes on Single vCPU

**Status:** VERIFIED
**File:** `pkg/cache/sharded_cache.go:76, 494-521`

```go
if shardCount <= 0 {
    shardCount = 64 // Default to 64 shards
}
```

And cleanup at line 494-521 spawns **64 goroutines in parallel** via `sync.WaitGroup` every 60 seconds. On a shared-cpu-1x (fractional vCPU), this causes:
- 64 goroutine creation/scheduling overhead every minute
- CPU spike as all 64 goroutines compete for the single core
- 64 × 4KB = 256KB transient stack allocation

With only 10K cache entries spread across 64 shards, each shard has ~156 entries. A single-threaded loop over 156 entries takes microseconds. The parallelism buys nothing.

**Remediation:**
- Default to 4 shards for single-core, 16 for multi-core
- Or: use `runtime.NumCPU()` to auto-size: `shardCount = max(4, runtime.NumCPU() * 4)`
- Change cleanup to serial iteration (single goroutine) for shard counts <= 16

**Effort:** Low

---

### HIGH Efficiency Issues (Verified)

#### E4. QueryLogger Buffer: 50K Slots Pre-Allocated Regardless of Traffic

**Status:** VERIFIED
**File:** `cmd/glory-hole/main.go:296-298`

```go
bufferSize := cfg.Server.QueryLogger.BufferSize
if bufferSize == 0 {
    bufferSize = 50000 // Default: 50K queries
}
```

50K × 8 bytes (pointer) = **400KB** allocated upfront for the channel. A home DNS server doing 1K queries/minute fills only ~83 slots between 5-second flush intervals. 99.8% of the buffer is wasted.

Additionally, 8 worker goroutines (default) × 4KB stack = 32KB sitting idle.

**Remediation:**
- Lower default to 5000 (still handles burst of 5K queries between flushes)
- Lower default workers to 2-4 (single-core doesn't benefit from 8)
- Make these auto-scale based on traffic or available memory

**Effort:** Low (change 2 defaults)

---

#### E5. Legacy `legacyLogCh` Always Allocated — Dead Code Path

**Status:** VERIFIED
**File:** `pkg/dns/handler.go:42-49`

```go
var legacyLogCh = make(chan legacyLogRequest, 10000)

func init() {
    for i := 0; i < 4; i++ {
        go legacyLogWorker()
    }
}
```

This channel (10K × 24 bytes = **240KB**) and 4 goroutines (16KB) are **always allocated** via `init()`, even when the new `QueryLogger` is active (which it is by default — `main.go:294`). The legacy path at `handler.go:348` is only reached when `h.QueryLogger == nil`.

**Remediation:**
- Move legacy channel init to a `sync.Once` or lazy init inside `asyncLogQuery`
- Or remove legacy path entirely if QueryLogger is the only supported path now
- Saves ~256KB + 4 idle goroutines

**Effort:** Low

---

#### E6. `unboundBuffer` 10K Slots Allocated Even When Unbound Is Disabled

**Status:** VERIFIED
**File:** `pkg/storage/sqlite.go:129`

```go
unboundBuffer: make(chan *UnboundQueryLog, 10000),
```

Always allocated in `NewSQLiteStorage()`. If Unbound is disabled, this 10K-slot channel (80KB) and the `unboundFlushWorker` goroutine are wasted.

**Remediation:** Lazy-initialize only when Unbound is enabled, or use a smaller default (1000).

**Effort:** Low

---

### MEDIUM Efficiency Issues (Verified)

#### E7. Blocklist Map Not Pre-Allocated on Reload

**Status:** VERIFIED — `pkg/blocklist/manager.go:219`
```go
merged := make(map[string]BlockEntry)  // starts at size 0
```
On reload with 100K domains, the map grows through ~15 resize-and-copy cycles. Pre-allocating with the previous count would save CPU and temporary memory during blocklist updates.

**Remediation:** `merged := make(map[string]BlockEntry, previousCount)` where `previousCount` is the last known blocklist size.

---

#### E8. Default `MaxEntries` Is 10K — Memory Hog for Small Deployments

**Status:** VERIFIED — `pkg/config/config.go:432-433`
```go
if c.Cache.MaxEntries == 0 {
    c.Cache.MaxEntries = 10000
}
```
10K entries × 400 bytes = 4MB. Plus 64-shard overhead. For a personal DNS with ~100 unique domains/hour, 2000-4000 entries is plenty.

**Remediation:** Lower default to 4000, or auto-tune based on available memory.

---

#### E9. `ForwardTCP()` Creates New dns.Client Every Call — Not Pooled

**Status:** VERIFIED — `pkg/forwarder/forwarder.go:213-220`
While the UDP `Forward()` path uses `clientPool`, `ForwardTCP()` allocates a fresh `dns.Client{Net: "tcp"}` on every call. Under DoT/DoH load with TCP fallback, this creates GC pressure.

**Remediation:** Add a separate `tcpClientPool` or make the existing pool transport-aware.

---

### Goroutine Census (Verified Startup Count)

| # | Goroutine | Always? | Memory |
|---|-----------|---------|--------|
| 1 | Config watcher (fsnotify) | Yes | 4KB |
| 2 | Retention cleanup ticker (1h) | Yes | 4KB |
| 3-4 | DNS server (UDP + TCP) | Yes | 8KB |
| 5 | API HTTP server | Yes | 4KB |
| 6-13 | QueryLogger workers (8 default) | Yes | 32KB |
| 14-17 | **Legacy log workers (4)** | **Always (init)** | **16KB waste** |
| 18 | Blocklist auto-update (24h) | Yes | 4KB |
| 19 | Cache cleanup (1m) | Yes | 4KB |
| 20 | SQLite flush worker | Yes | 4KB |
| 21 | SQLite domain stats worker | Yes | 4KB |
| 22 | **SQLite unbound flush worker** | **Always** | **4KB waste if no Unbound** |
| 23 | Unbound health loop (10s) | If Unbound | 4KB |
| 24 | Unbound process monitor | If Unbound | 4KB |
| 25-26 | Unbound log forwarders | If Unbound | 8KB |
| 27 | Prometheus HTTP server | If enabled | 4KB |
| 28 | Dnstap reader | If Unbound | 4KB |
| | **Total** | | **~112KB** |

Plus: 64 transient goroutines every 60s for cache cleanup (256KB spike).

### What's Already Well-Optimized

| Pattern | Location | Notes |
|---------|----------|-------|
| `sync.Pool` for dns.Msg | `handler.go:28` | Eliminates per-query alloc |
| `sync.Pool` for serveDNSOutcome | `handler_state.go:26` | Eliminates per-query alloc |
| `sync.Pool` for blockTraceRecorder | `trace.go:24` | With pre-alloc cap=4 |
| Hand-rolled int-to-string in `makeKey` | `cache.go:343` | Avoids `fmt.Sprintf` alloc |
| Atomic counters for cache stats | `cache.go` | Lock-free reads |
| Buffered channel for query logging | `sqlite.go:127` | Non-blocking DNS path |
| Pre-compiled policy expressions | `engine.go:91` | No parse-per-query |
| `atomic.Pointer` for blocklist swap | `manager.go` | Lock-free reads on hot path |
| WAL mode for concurrent reads | `sqlite.go:96` | Dashboard doesn't block DNS |
| Per-query allocation: 3-5 heap allocs | `handler.go` ServeDNS | Very lean hot path |

### Recommended Fly.io Efficiency Profile

Create a `config.fly-optimized.yml` or detect low-memory at startup:

```yaml
cache:
  max_entries: 4000
  shard_count: 4        # Not 64

database:
  sqlite:
    cache_size: 1024    # 1MB, not 4MB
    mmap_size: 33554432 # 32MB, not 256MB
  buffer_size: 500      # Already default

server:
  query_logger:
    buffer_size: 5000   # Not 50K
    workers: 2          # Not 8

unbound:
  server:
    msg_cache_size: "4m"     # Not 32m
    rrset_cache_size: "8m"   # Not 64m
    key_cache_size: "4m"     # Not 16m
    num_threads: 1           # Not 2
    outgoing_range: 512      # Not 4096
    num_queries_per_thread: 256
```

**Estimated savings: ~150-200MB RSS**, bringing total from ~270MB down to ~70-120MB on the 512MB instance. Leaves healthy headroom for traffic spikes.

---

## Feature: Local Records Edit Capability {#feature-local-records-edit}

> The UI currently supports Add and Delete but NOT Edit for local records.
> Users must delete and re-add records to change values.

### Current State (Verified)

| Layer | What Exists | What's Missing |
|-------|-------------|----------------|
| **Manager** (`pkg/localrecords/records.go`) | `AddRecord()` :73, `RemoveRecord()` :101 | No `UpdateRecord()` method |
| **API** (`pkg/api/handlers_localrecords.go`) | `GET /api/localrecords` :53, `POST /api/localrecords` :95, `DELETE /api/localrecords/{id}` :216 | No `PUT /api/localrecords/{id}` endpoint |
| **API client** (`pkg/api/ui/dashboard/src/lib/api.ts`) | `fetchLocalRecords()` :398, `createLocalRecord()` :409, `deleteLocalRecord()` :418 | No `updateLocalRecord()` function |
| **UI** (`LocalRecordsPage.tsx`) | Add dialog :170, Delete button :159 | No edit button, no edit dialog mode, no form pre-population |

### Key Design Challenge: Record Identity

Records are stored as a flat `[]LocalRecordEntry` in config. The current ID scheme is `domain:type:index` generated on-the-fly in `handleGetLocalRecords` (:61). This is fragile:
- Adding/removing records shifts indices
- The delete handler ignores the index and matches on `domain:type` only (:244-248), which deletes ALL records for that domain+type pair
- No stable unique identifier exists

### Implementation Plan

#### Step 1: Backend — Add `PUT /api/localrecords/{id}` endpoint

**File:** `pkg/api/handlers_localrecords.go`

1. Add `LocalRecordUpdateRequest` type (same fields as `LocalRecordAddRequest`)
2. Add `handleUpdateLocalRecord(w, r)` handler:
   - Parse `{id}` from path → extract `domain:type` 
   - Read JSON body with `MaxBytesReader` (fix M4 while at it)
   - Validate the new record (reuse existing validation from `handleAddLocalRecord`)
   - In `persistLocalRecordsConfig`: find the matching record by domain+type+old-value, replace with new values
   - Reload DNS handler's local records
3. Register route in `api.go`: `mux.HandleFunc("PUT /api/localrecords/{id}", s.handleUpdateLocalRecord)`

**Identity strategy:** Use `domain:type:value` as the composite key (more stable than index). For A records: `fritz.box:A:10.0.0.1`. For multi-value records (TXT with multiple strings), use the first value or a hash.

#### Step 2: API Client — Add `updateLocalRecord()`

**File:** `pkg/api/ui/dashboard/src/lib/api.ts`

```typescript
export async function updateLocalRecord(
  id: string,
  data: LocalRecordCreateRequest
): Promise<LocalRecord> {
  return apiFetch(`/api/localrecords/${encodeURIComponent(id)}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(data),
  });
}
```

#### Step 3: UI — Add Edit Button + Edit Dialog Mode

**File:** `pkg/api/ui/dashboard/src/components/LocalRecordsPage.tsx`

Changes needed:
1. **Add edit button per row** — Pencil icon next to the existing trash icon
2. **Add edit state** — `editingRecord: LocalRecord | null`
3. **Modify dialog to support edit mode:**
   - When `editingRecord` is set, dialog title = "Edit Local Record", button = "Save"
   - Pre-populate form fields from `editingRecord`
   - On submit: call `updateLocalRecord(editingRecord.id, data)` instead of `createLocalRecord(data)`
4. **Add click handler:**
   ```tsx
   function handleEdit(record: LocalRecord) {
     setEditingRecord(record);
     setDomain(record.domain);
     setRecordType(record.type);
     // ... populate all form fields
     setDialogOpen(true);
   }
   ```
5. **Table row actions column:** Add Pencil2 icon button before trash button

**UI mockup of the row:**
```
| fritz.box | A | 10.0.0.1 | 300s | [pencil] [trash] |
```

#### Step 4: Improve Record Identity

**File:** `pkg/api/handlers_localrecords.go`

Change ID generation (:61) from `domain:type:index` to `domain:type:value`:
```go
id := fmt.Sprintf("%s:%s:%s", rec.Domain, rec.Type, primaryValue(rec))
```
Where `primaryValue` returns IPs[0] for A/AAAA, Target for CNAME/MX/SRV/PTR/NS, TxtRecords[0] for TXT, etc.

This makes IDs stable across add/delete operations and enables reliable edits.

#### Effort Estimate

| Step | Effort |
|------|--------|
| Backend PUT handler | 2-3 hours |
| API client function | 15 min |
| UI edit button + dialog mode | 2-3 hours |
| Record identity improvement | 1 hour |
| Testing | 1-2 hours |
| **Total** | **~1 day** |

---

## Positive Security Patterns (Preserve These)

These are done well and should NOT be changed:

| Pattern | Location |
|---------|----------|
| Parameterized SQL (`?` placeholders) everywhere | `pkg/storage/sqlite.go` — all queries |
| No `dangerouslySetInnerHTML` in React | All 16 component TSX files |
| `textContent` for DOM updates (not innerHTML) | `DashboardLayout.astro`, `login.astro` |
| `crypto/subtle.ConstantTimeCompare` for auth | `middleware_auth.go:150,166` |
| Bcrypt password hashing | `middleware_auth.go:156` |
| Session fixation prevention (revoke before create) | `ui_handlers.go:112` |
| 32-byte crypto random session tokens | `session.go:123-124` |
| `HttpOnly` + `SameSite=Lax` cookies | `session.go:147-149` |
| Open redirect protection via `sanitizeRedirectTarget` | `middleware_auth.go:236-259` |
| CORS defaults to deny (empty origin list) | `middleware.go:33-34` |
| Secret stripping in config API responses | `responses.go:299-300` |
| `X-Frame-Options: DENY` | `middleware.go:51` |
| `X-Content-Type-Options: nosniff` | `middleware.go:50` |
| `exec.Command` with separate args (no shell) | `pkg/unbound/supervisor.go` |
| No credentials in frontend JS | `api.ts` |
| `encodeURIComponent` on path params | `api.ts:464,487,511,760` |
| WAL mode + buffered writes for SQLite | `sqlite.go:96, 127` |
| Circuit breaker on upstream forwarders | `forwarder/circuit_breaker.go` |
| ReadTimeout/WriteTimeout/IdleTimeout on HTTP server | `api.go:250-252` |
| ReadHeaderTimeout on Prometheus server | `telemetry.go:169` |

---

## Remediation Phases

### Phase 1: Quick Wins — Security + Efficiency (1-2 days, HIGH impact)

| # | Finding | Effort | Files |
|---|---------|--------|-------|
| H1 | Fix `defer` in forwarder loop | 30 min | `pkg/forwarder/forwarder.go` |
| H3 | Add `io.LimitReader` on blocklist download | 15 min | `pkg/blocklist/blocklist.go` |
| H5 | Validate logging/TLS file paths | 1 hr | `pkg/api/handlers_config_update.go` |
| M4 | Add `MaxBytesReader` to 8 handlers | 30 min | 4 handler files |
| M8 | Validate blocklist URL scheme | 15 min | `handlers_blocklists.go`, `blocklist.go` |
| M10 | `os.Chmod(db, 0600)` | 5 min | `pkg/storage/sqlite.go` |
| M11 | Change dnstap socket to 0660 | 5 min | `pkg/unbound/dnstap_reader.go` |
| M13 | Add HSTS header | 5 min | `pkg/api/middleware.go` |
| M3 | Fix CORS wildcard + credentials | 15 min | `pkg/api/middleware.go` |
| M9 | Sanitize error messages | 1 hr | Multiple handler files |
| M14 | Error code enum on login page | 30 min | `login.astro`, `ui_handlers.go` |
| M1 | Log warning for open resolver | 15 min | `pkg/dns/acl.go` or `cmd/main.go` |
| L5 | `atomic.Bool` for warningLogged | 10 min | `pkg/storage/sqlite.go` |
| **E1** | **Lower mmap_size default to 32MB** | **5 min** | `pkg/config/config.go:411` |
| **E2** | **Lower Unbound cache defaults for Fly** | **15 min** | `pkg/unbound/defaults.go:13-15,31` |
| **E3** | **Lower shard count default to 4** | **5 min** | `pkg/cache/sharded_cache.go:76` |
| **E4** | **Lower QueryLogger buffer to 5K, workers to 2** | **5 min** | `cmd/glory-hole/main.go:298,302` |
| **E5** | **Lazy-init legacy log channel (or remove)** | **30 min** | `pkg/dns/handler.go:42-49` |
| **E6** | **Lazy-init unbound buffer in storage** | **15 min** | `pkg/storage/sqlite.go:129` |

### Phase 2: Core Security + Feature (3-5 days)

| # | Finding | Effort | Files |
|---|---------|--------|-------|
| C1 | Rate limiting middleware | 1-2 days | New `middleware_ratelimit.go`, `api.go`, `config.go` |
| C2 | CSRF protection | 1 day | New `middleware_csrf.go`, `ui_handlers.go`, `api.ts` |
| H4 | Trusted proxy config | 1 day | `config.go`, `api.go` |
| H6 | Session Secure flag with proxy awareness | 30 min | `session.go` (depends on H4) |
| H2 | Unbound template input validation | 1 day | `pkg/unbound/validator.go`, `writer.go`, handler files |
| **F1** | **Local records edit capability** | **1 day** | `handlers_localrecords.go`, `LocalRecordsPage.tsx`, `api.ts` |

### Phase 3: Defense in Depth + Performance (1-2 weeks)

| # | Finding | Effort | Files |
|---|---------|--------|-------|
| M2 | DNS response size limiting / TC bit | 2 days | `pkg/dns/handler.go`, `edns.go` |
| M6 | Fix `DomainMatches` substring behavior | 2 days | `pkg/policy/engine.go` + migration |
| M7 | Cache key with DNSSEC flags | 1 day | `pkg/cache/cache.go`, `sharded_cache.go` |
| P4 | TCP fallback on truncation | 1 day | `pkg/forwarder/forwarder.go` |
| P5 | Per-transport timeout config | 0.5 day | `pkg/forwarder/forwarder.go`, `config.go` |
| L4 | Implement log rotation with lumberjack | 0.5 day | `pkg/logging/logger.go` |
| **E3b** | **Serial cache cleanup for small shard counts** | **1 hr** | `pkg/cache/sharded_cache.go:494` |
| **E7** | **Pre-allocate blocklist map on reload** | **15 min** | `pkg/blocklist/manager.go:219` |
| **E8** | **Lower default cache MaxEntries to 4000** | **5 min** | `pkg/config/config.go:432` |

### Phase 4: Hardening (ongoing)

| # | Finding | Effort |
|---|---------|--------|
| L1 | Implement or remove RATE_LIMIT action | 1-3 days |
| L2 | Safe type assertions in policy functions | 1 hr |
| L9 | Nonce-based CSP | 2-3 days |
| M12 | Prometheus auth | 1 day |
| M5 | Bounded regex cache | 0.5 day |
| P3 | Combine stat transactions | 1 day |
| E9 | Pool TCP dns.Client in forwarder | 0.5 day |

---

## Notes

- **H1 (sync.Pool on dns.Msg)** was initially flagged as CRITICAL but after verification, `miekg/dns` `WriteMsg` serializes synchronously and `asyncLogQuery` does not reference `msg`. The pool pattern is safe in current code. Downgraded to informational/code quality. Retained as a comment in case future code changes break the invariant.

- **SQL injection** was thoroughly checked across all 13 storage files. All queries use parameterized `?` placeholders. The one `DELETE FROM "+table` in `Reset()` uses a hardcoded table list (sqlite.go:1287), not user input. **No SQL injection found.**

- **XSS** was thoroughly checked across all 49 frontend files. No `dangerouslySetInnerHTML`, no `innerHTML`, all dynamic data rendered via React JSX or `textContent`. **No XSS found.**

- **Shell injection** was checked in all `exec.Command` calls in `pkg/unbound/supervisor.go`. All use separate argument passing, never shell interpolation. **No shell injection found.**

- **Open redirect** was checked. `sanitizeRedirectTarget` at `middleware_auth.go:236-259` properly blocks non-path redirects, protocol-relative URLs, and encoded bypass attempts. **No open redirect found.**
