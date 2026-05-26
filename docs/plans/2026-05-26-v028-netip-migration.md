# v0.28 — `net.IP` → `netip.Addr` migration

**Status:** planned
**Target release:** v0.28 (post-v0.26 policy consolidation, post-v0.27 Conditional Forwarding deletion)
**Effort:** 1 focused day, mostly mechanical
**Risk:** Low — pure refactor, behavior-preserving, every call site has a direct equivalent.

---

## Why

Two compounding reasons:

1. **It's strictly better today.** `netip.Addr` is comparable, allocates zero, fits in a register, is map-key safe, and has explicit IPv4/IPv6 semantics (`Is4()` / `Is4In6()` / `Is6()`). `net.IP` is a `[]byte`, comparable only via `Equal()`, allocates on parse, and conflates v4 and v4-mapped-v6 in nasty ways (`net.ParseIP("1.2.3.4")` returns a 16-byte slice that is *not* `Equal()` to a 4-byte literal).
2. **It's a free down-payment on a future miekg/dns v2 port.** The v2 porting guide ([codeberg.org/miekg/dns/_doc/README-v1-to-v2.md](https://codeberg.org/miekg/dns/src/branch/main/_doc/README-v1-to-v2.md)) explicitly recommends doing this conversion *first*, before touching the dns library. Quote: *"Convert to netip.Addr first. Before you convert to dns v2, convert from `net.IP` to `netip.Addr`. DNS v2 uses `netip.Addr` to represent IP addresses. Converting two modules at the same time is much more difficult than converting them in sequence."*

The miekg/dns v2 migration itself is deferred indefinitely (v2 is still pre-1.0, breaking-change license active, won't ship v1.0 until ~2028). This plan is independent of that decision and stands on its own merits.

## Scope audit

`rg -n 'net\.IP|net\.ParseIP|net\.IPNet|net\.ParseCIDR'` over non-test Go files: **68 hits across 8 packages**.

| Package | Non-test sites | Character |
|---|---:|---|
| `pkg/localrecords` | 17 | Stores `[]net.IP` in records — biggest internal-state exposure |
| `pkg/api` | 16 | Trusted-proxy CIDRs, DoH handler, request validation |
| `pkg/dns` | 11 | ACL, response.go, blocklist + policy handlers, server_impl client extraction |
| `cmd/glory-hole` | 8 | Pi-hole / DNSMasq import paths |
| `pkg/policy` | 7 | CIDR cache, `IPInCIDR` DSL primitive, CIDRsEqual |
| `pkg/forwarder` | 5 | Host parsing, CIDRMatcher |
| `pkg/unbound` | 2 | dnstap reader (`net.IP(addr).String()` from raw bytes) |
| `pkg/resolver` | 2 | LookupIP signature |

Test files: another 129 sites — those flip naturally as the production code changes.

## Boundaries that **stay** `net.IP`

These are external-API boundaries — converting them would require either a fork or a translation layer that defeats the point.

1. **`miekg/dns.A.A` / `dns.AAAA.AAAA`** — `net.IP` field on the RR struct. We must hand the library a `net.IP` when building responses.
   - Internal type: `netip.Addr`. Conversion at the boundary: `addr.AsSlice()`.
2. **`net.Addr` from `dns.ResponseWriter.RemoteAddr()`** — comes from the stdlib `net` package, no avoiding it. We extract it once via `netip.ParseAddrPort` or `netip.AddrFromSlice`.
3. **`net.IP` arguments to `dns.A` builders in `pkg/dns/response.go`** — keep the signature unless we cascade the change *into* the response builders, in which case `addARecord(msg, domain, addr netip.Addr, ttl)` and convert at the `&dns.A{}` literal.
   - Recommended: cascade. The stdlib boundary lives at exactly one site (`&dns.A{A: addr.AsSlice()}`) and everything upstream stays typed.

## Conversion cheat-sheet

| v1 (`net`) | v2 (`netip`) |
|---|---|
| `net.ParseIP(s)` returns `net.IP` (16-byte if v4) | `netip.ParseAddr(s)` returns `(Addr, error)`; `.Is4()` / `.Is6()` are explicit |
| `net.IPNet{IP, Mask}` | `netip.Prefix` (returned by `netip.ParsePrefix`) |
| `net.ParseCIDR(s)` returns `(IP, *IPNet, error)` | `netip.ParsePrefix(s)` returns `(Prefix, error)` |
| `ipNet.Contains(ip)` | `prefix.Contains(addr)` (note: `Contains` doesn't auto-mask the addr like `IPNet.Contains` does — call `prefix.Masked().Contains(addr)` if you got the prefix from a user) |
| `ip.Equal(other)` | `addr == other` (real `==`, with v4 / v4-in-v6 normalized) |
| `ip.To4() != nil` (i.e. is v4) | `addr.Is4()` or `addr.Unmap().Is4()` if it might be v4-in-v6 |
| `ip.String()` | `addr.String()` (note: writes canonical IPv6 form, lowercase) |
| `net.IP{1,2,3,4}` literal | `netip.AddrFrom4([4]byte{1,2,3,4})` |
| `net.IP(addr).String()` (raw `[]byte`) | `netip.AddrFromSlice(addr)` then `.String()` |
| Map-key `string(ip)` | Map-key `addr` directly (Addr is comparable) |

## Migration order

Do leaf packages first so each PR can land + ship green tests independently. Each step is its own commit on the v0.28 branch.

### Step 1: `pkg/localrecords` (1-2 hours)

Biggest single chunk; pure internal state.

- `LocalRecord.IPs` field: `[]net.IP` → `[]netip.Addr`.
- `parseIP` / `parseIPs`: return `netip.Addr` / `[]netip.Addr`.
- `LookupA` / `LookupAAAA` / `ResolveCNAME` / `extractIPs`: return `[]netip.Addr`.
- `NewARecord` / `NewAAAARecord` constructors: take `netip.Addr`.
- `Clone` deep-copy stays the same shape.
- Wire up to `pkg/dns/handler_local_records.go` consumer — it already converts to `dns.A.A` so this is `addr.AsSlice()` at the call site.

**Test impact:** all `localrecords` tests use `net.ParseIP("...")` literals — sweep with `sd 'net\.ParseIP\(("[^"]+")\)' 'netip.MustParseAddr($1)'` and verify.

### Step 2: `pkg/dns` (1 hour)

- `pkg/dns/acl.go` — `nets []*net.IPNet` → `[]netip.Prefix`, `singles []net.IP` → `[]netip.Addr`. The single+net split goes away because `netip.Prefix` handles single IPs naturally as `/32` or `/128`.
- `pkg/dns/server_impl.go::clientIPFromRemoteAddr` — extract via `netip.AddrFromSlice` / `netip.ParseAddrPort` rather than `net.SplitHostPort`. (Faster, no allocation.)
- `pkg/dns/handler_blocklist.go` + `pkg/dns/handler_policy.go` — block-page IP and policy ALLOW IP both go through `netip.ParseAddr`.
- `pkg/dns/response.go` — convert at the boundary: `&dns.A{A: addr.AsSlice()}`.

### Step 3: `pkg/policy` (45 min)

- `cidrCache` — `sync.Map` keyed on string CIDR → cached `netip.Prefix`. Bound stays at 256.
- `IPInCIDR` DSL primitive — `netip.ParseAddr` + `prefix.Contains`. Mask once via `prefix.Masked()` to handle hostbits-set user input.
- `CIDRsEqual` — `netip.Prefix` equality is structural, no special handling needed.
- ACTION validation in `pkg/policy/engine.go:648` — `netip.ParseAddr` instead of `net.ParseIP`.

### Step 4: `pkg/forwarder` (30 min)

- `pkg/forwarder/matcher.go::CIDRMatcher` — `[]*net.IPNet` → `[]netip.Prefix`.
- `pkg/forwarder/forwarder.go:524` host parsing.

### Step 5: `pkg/api` (1 hour, most surface area)

- `Server.trustedProxies []*net.IPNet` → `[]netip.Prefix`. The bare-IP-vs-CIDR branch in `pkg/api/api.go:116-129` collapses (`netip.ParsePrefix` accepts both `1.2.3.4/24` and bare IP becomes one helper).
- `pkg/api/api.go:502::isTrustedProxy` — single `prefix.Contains`.
- `pkg/api/handler_doh.go::dohResponseWriter` — `RemoteAddr()` must still return `net.Addr` (interface from miekg/dns). Build `&net.TCPAddr{IP: addr.AsSlice()}` at the boundary.
- `pkg/api/handlers_config_update.go` — validation uses `netip.ParseAddr` / `netip.ParsePrefix`.
- `pkg/api/handlers_localrecords.go` + `pkg/api/handlers_policy.go` — ditto.
- `pkg/api/block_page.go:127` — same, the special-case strings stay literal.

### Step 6: `pkg/unbound/dnstap_reader.go` (15 min)

The interesting case:
```go
entry.ClientIP = net.IP(addr).String()
```
becomes
```go
if a, ok := netip.AddrFromSlice(addr); ok {
    entry.ClientIP = a.Unmap().String()
}
```
The `Unmap()` is important — `AddrFromSlice` of a 4-byte slice gives a v4 Addr, but a 16-byte slice always gives v6 even if it's `::ffff:1.2.3.4`. Without `Unmap()` you'd surface v4-mapped-v6 to the query log. **This is a latent bug fix** — current `net.IP(addr).String()` does the same thing.

### Step 7: `pkg/resolver` (15 min)

`LookupIP` signature returns `[]net.IP` — change to `[]netip.Addr`. Single caller is internal.

### Step 8: `cmd/glory-hole` import paths (30 min)

- `cmd/glory-hole/main.go:609,636,1141,1157` — Pi-hole / DNSMasq importer parses string IPs into `[]net.IP` for `LocalRecord.IPs`. Change source-of-truth to `netip.Addr`.
- The `ip.To4() != nil` checks at 1143 and `ip.To4() == nil` at 1159 (v4 vs v6 routing) become `addr.Is4()` and `addr.Is6()` after `Unmap()`.

### Step 9: stdlib boundary audit

Run `rg -n 'net\.IP\b' --type go pkg cmd | rg -v _test` post-migration and confirm only these survive:

- `pkg/dns/response.go` (`&dns.A{A: ...}` boundary)
- `pkg/api/handler_doh.go` (`&net.TCPAddr` boundary for `dns.ResponseWriter.RemoteAddr()`)

If any others remain, audit individually — they're either bugs or legitimate stdlib boundaries.

## Test plan

- Unit tests update mechanically (`net.ParseIP("...")` → `netip.MustParseAddr("...")`).
- Existing integration tests (`test/integration_test.go`, `test/load/dns_load_test.go`) cover the live DNS path — all must stay green.
- New test in `pkg/unbound/dnstap_reader_test.go`: assert v4-mapped-v6 client IPs surface as bare v4 strings (regression guard for the `Unmap()` fix).
- Bench: `go test -bench=BenchmarkPolicyEval -benchmem ./pkg/policy/` before + after. Expect:
  - `IPInCIDR` ~2x faster (no `net.ParseIP` allocation per call after cache hit, and the cached `netip.Prefix` is value-typed).
  - Allocation count drop on the hot path.

## Risks

| Risk | Mitigation |
|---|---|
| `netip.AddrFromSlice` ambiguity on 16-byte v4-mapped slices | Always `.Unmap()` after `AddrFromSlice` when you don't know the source |
| `netip.Prefix.Contains` doesn't auto-mask user input | Call `prefix.Masked().Contains(addr)` for any prefix that came from user-provided CIDR strings |
| Trusted-proxies bare IP entries (no `/32`) | `netip.ParsePrefix` rejects bare IPs — wrap with `netip.ParseAddr` fallback that constructs `prefix := netip.PrefixFrom(addr, addr.BitLen())` |
| Test fixtures in YAML/JSON expect canonical lowercase `String()` output | `netip.Addr.String()` is canonical lowercase; `net.IP.String()` was also canonical so same result. Verify with `rg -i '::FFFF:' test/` for any uppercase v6 expectations |
| Public API JSON shape | All IPs are already serialized as strings via `String()`; JSON shape unchanged |

## Out of scope

- **No changes to `pkg/blocklist`** — already uses `string` keys, no `net.IP` usage.
- **No changes to `pkg/cache`** — already uses `string` keys derived from question name+type+client.
- **No changes to `pkg/storage`** — query log stores client IP as `TEXT`, not as bytes.
- **No miekg/dns v2 port** — explicitly deferred. See decision log entry 2026-05-26.

## Sequencing

This block lands as a single coherent v0.28. Commits per step (9 commits) so any step can revert independently. CI runs `make test` between each.

Targeted ship: first week of v0.28 development cycle, before any DNS-runtime feature work that would conflict.
