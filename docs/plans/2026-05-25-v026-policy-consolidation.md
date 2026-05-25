# v0.26 — Policy engine consolidation

**Date:** 2026-05-25
**Status:** plan / not started
**Scope:** consolidate routing-relevant features under the Policy engine; document
features that intentionally stay distinct; clean up dead code.

## Why now

Live shipping experience with v0.25.0 → v0.25.1 surfaced that Conditional
Forwarding and Policy FORWARD do the same thing (both terminate in
`fwd.ForwardWithUpstreams`), and that several adjacent features have either
silently rotted (Whitelist, Override IP / CNAME Override, unwired
Timeout/MaxRetries/Failover knobs) or have layer-crossing semantic differences
that are worth pinning down rather than glossing over.

This plan is the output of an evidence-based audit (see commit history of this
file's review or session notes for full citations). Every claim below is
grounded in a specific file:line citation that was verified at audit time.

## Summary table

| Feature | Verdict | Effort | Risk | Section |
|---|---|---|---|---|
| Conditional Forwarding | **deprecate → merge-into-policy** | M | L | §1 |
| Whitelist | **delete** (already inert) | S | L | §2 |
| ClientGroups | **keep + extend DSL** with `InClientGroup()` | M | M | §3 |
| AllowedClients | **keep-distinct** | — | — | §4 |
| LocalRecords | **keep-distinct** | — | — | §5 |
| Block Page IP | **keep-distinct** | — | — | §6a |
| Override IP / CNAME Override | **delete** (dead code) | S | ~0 | §6b |
| Unbound Forward Zones | **keep-distinct** | — | — | §7 |

## Cross-cutting fixes (do these first)

These aren't tied to a single feature deprecation; they're hygiene work that
v0.26 should pick up regardless.

### CC-1. Migrator idempotency bug (production today)

`cmd/glory-hole/main.go:48-110` — `migrateWhitelistToPolicies` writes ALLOW
rules to `policy_rules` but **does not write `cfg.Whitelist = nil` back to
disk**, and `policy_rules` has **no `UNIQUE(name)`** constraint
(`pkg/storage/migrations.go:237-251`). On every restart with any `whitelist:`
entry remaining in YAML, duplicate `Allow X (migrated)` rows accumulate.

**Fix (any of, prefer all three):**
1. Migration v14: `CREATE UNIQUE INDEX policy_rules_name_uq ON policy_rules(name)`.
2. Persist the YAML mutation in the migrator (call `config.Save` after `cfg.Whitelist = nil`).
3. `dynamic_config` sentinel `whitelist_migrated_at` checked at startup.

This must be fixed before §1 ships its own migrator (which would inherit the same trap).

### CC-2. Dead trace + metric

- `pkg/dns/trace.go:12` — `traceStageWhitelist = "whitelist"` constant, never recorded.
- `pkg/telemetry/telemetry.go:46, 262, 334` — `DNSWhitelistedQueries` counter,
  declared and registered but never incremented.

Delete in v0.26 cleanup commit.

### CC-3. Persistence regime split — document explicitly

Today: Policies → SQLite; ConditionalForwarding/LocalRecords/Whitelist/BlockPage → YAML.

Recommend articulating the rule in `docs/architecture/`:

> **Dynamic, frequently-edited routing rules live in SQLite. Static system
> topology / infrastructure config lives in YAML.**

§1 migrates Conditional Forwarding from YAML to SQLite under that rule.

### CC-4. Sort direction is opposite

ConditionalForwarding: `Priority DESC` (highest first, range 1-100, default 50).
Policy: `sort_order ASC` (lowest first, no range constraint).

Decide canonical direction in v0.26 — recommend `sort_order ASC` (already
policy default). Document. The §1 migrator inverts: `sort_order = 100 - priority`.

### CC-5. Reuse `policy.ParseUpstreams`

`pkg/policy/engine.go:539-577` — auto-appends `:53` if no port specified, parses
comma-separated lists. ConditionalForwarding currently does no normalization;
the §1 migrator should call `policy.ParseUpstreams` instead of writing a
parallel parser. After deprecation, lift it to a shared internal package.

---

## §1 Conditional Forwarding → Policy FORWARD

### Verified subsumption

| ConditionalRule field | Matcher impl | Policy DSL equivalent |
|---|---|---|
| `Domains` | `forwarder.DomainMatcher` (exact / wildcard / regex) | `DomainMatches` / `DomainEndsWith` / `DomainRegex` |
| `ClientCIDRs` | `forwarder.CIDRMatcher` | `IPInCIDR(ClientIP, "10.0.0.0/24")` |
| `QueryTypes` | `forwarder.QueryTypeMatcher` | `QueryTypeIn(QueryType, "A","AAAA")` |
| `Upstreams` | passed to `ForwardWithUpstreams` | `Action: FORWARD`, `ActionData: "1.1.1.1:53,8.8.8.8:53"` |

Both code paths terminate in `fwd.ForwardWithUpstreams(ctx, r, upstreams)` with the
same upstream slice. Outcome metadata differs only in trace label
(`"policy_forward"` vs `"conditional_rule"`) — trivial to unify.

### Hard differences

1. **`timeout`, `max_retries`, `failover` knobs are dead code.** They live in
   `config.ForwardingRule` (`pkg/config/conditional_forwarding.go:13-22`) and
   round-trip through the API but **nothing reads them at runtime**.
   `compileRule` (`pkg/forwarder/evaluator.go:71-96`) doesn't copy them onto
   the runtime `ConditionalRule`; `ForwardWithUpstreams` uses global
   `f.retries` / `f.timeout`. **Drop these fields immediately in v0.26.**
2. **Priority direction is inverted** (see CC-4).
3. **YAML vs SQLite persistence** (see CC-3).
4. **`policy.ParseUpstreams` auto-appends `:53`**; ConditionalForwarding doesn't.
   Migration is an upgrade for affected rules.
5. **Validation: at least one matcher required** in CF
   (`conditional_forwarding.go:78-80`); Policy has no such guard. Migrator
   should reject empty-matcher (would silently expand to "match all").

### Migration steps

1. **v0.26 release notes:** mark `conditional_forwarding` deprecated.
2. **One-shot migrator** mirrored on the existing whitelist migrator pattern,
   but **with CC-1 fixes baked in**:
   - For each enabled rule: synthesize Policy DSL by AND-joining configured
     matchers; `Action: FORWARD`; `ActionData: strings.Join(rule.Upstreams, ",")`;
     `SortOrder: 1000 + (100 - rule.Priority)` (band offset to avoid collision
     with hand-curated low-numbered policies).
   - Insert via `stor.CreatePolicyRule`.
   - `cfg.ConditionalForwarding.Rules = nil` AND **persist back to disk via
     `config.Save`**.
   - Use `dynamic_config` sentinel `cf_migrated_at`.
3. **Deprecation warning** when `cfg.ConditionalForwarding.Enabled &&
   len(rules) > 0` after migration ran (signals user manually re-added entries).
4. **v0.26:** drop `Timeout / MaxRetries / Failover` fields outright (never
   wired; no breakage).
5. **v0.27:** delete `pkg/dns/handler_forwarding.go::handleConditionalForwarding`,
   call site at `handler.go:411-413`, API endpoints, UI page,
   `forwarder.RuleEvaluator` package surface, YAML schema. Re-route the UI to
   a "Forward" preset on the Policy creation page.

### Risk

- **Low DNS-runtime risk** — both paths terminate identically.
- **Order semantics** — migrator places at 1000+ band to avoid stomping on
  hand-curated low-numbered policies.
- **API/UI surface change** — keep `/api/conditionalforwarding` as a 410-Gone
  with deprecation header for one minor version, OR redirect to `/api/policies`.

---

## §2 Whitelist — delete

### Evidence of deadness

LSP-grade scan for `cfg.Whitelist`, `c.Whitelist`, `Whitelist:` field access:

| Site | Operation |
|---|---|
| `pkg/config/config.go:30` | declaration |
| `pkg/api/responses.go:242, 348` | `ConfigResponse.Whitelist` (read-only API output) |
| `pkg/api/api_test.go:400` | test fixture |
| `cmd/glory-hole/main.go:48-110` | one-shot migrator (read + clear in-memory) |
| `cmd/glory-hole/import.go:130` | Pi-hole importer writes to it |
| `pkg/api/ui/dashboard/src/lib/api.ts:326` | TypeScript type only |

**Zero DNS handler references. Zero writes via API.** No handler short-circuits
on whitelist match. The migrator already converts entries into Policy ALLOW.

### Steps

1. **v0.26**:
   - Fix migrator idempotency (CC-1).
   - Strip `Whitelist` from `ConfigResponse` (`pkg/api/responses.go:242, 348`).
   - Remove dashboard type field (`api.ts:326`).
   - Retarget Pi-hole importer (`cmd/glory-hole/import.go:349-412`) to write
     directly to `policy_rules` instead of populating `cfg.Whitelist`.
2. **v0.27**: delete `Config.Whitelist`, the migrator,
   `traceStageWhitelist`, `DNSWhitelistedQueries`.

### Risk

External tooling scraping `/api/config` for `whitelist[]` will break — emit
deprecation header for one minor version.

---

## §3 ClientGroups — extend the DSL, don't deprecate

### Current state

ClientGroups today is **strictly UI labeling**. SQLite tables `client_groups` +
`client_profiles` are joined into `GetClientSummaries` for `/api/clients`, but
**no DNS handler reads group membership**. Verified by exhaustive grep across
`pkg/dns/`: only mock fixtures reference `GroupName`.

### Opportunity

Add `InClientGroup(ClientIP, "kids")` to `compileRuleLogic`
(`pkg/policy/engine.go:107-271`), graduating ClientGroups from display-only to
a first-class policy input. Replaces today's awkward
`IPInCIDR(ClientIP, "192.168.1.50/32") || IPInCIDR(ClientIP, "192.168.1.51/32")`.

### Implementation

1. New `policy.ClientGroupResolver` interface, populated at engine init:
   ```go
   type ClientGroupResolver interface {
       IsInGroup(clientIP, groupName string) bool
   }
   ```
2. Atomic-pointer cache (mirror `BlocklistManager` pattern from AGENTS.md):
   - At engine init / config reload, load `(client_ip → []group_name)` map.
   - Hook into API `UpsertClientProfile` / `DeleteClientGroup` for invalidation.
3. Closure capture in `compileRuleLogic` so the new helper sees the resolver.
4. Unprofiled IP returns `false`, not error (avoid skip-rule semantics — see
   `engine.go:317-328`).
5. Hot-path constraint: O(1) cache hit, NO synchronous DB query per query.
6. UI: "Add Policy" form gets a "client group" matcher dropdown that
   synthesizes `InClientGroup(ClientIP, "kids")`.

### Risk

- Hot-path lookup cost — cache must hit. Audit `Engine.Evaluate` perf with the
  new helper enabled before shipping.
- Cache invalidation on profile edits — explicit hook required.
- Profile coverage — unprofiled IPs must return `false` (handled in step 4).

---

## §4 AllowedClients — keep distinct, document why

`pkg/dns/acl.go:11-90` + `pkg/dns/server_impl.go:381-393` — runs as a wrapper
**outside** the policy engine. Returns `RcodeRefused`. Bypassed by DoT/DoH
(TLS is the auth layer). Pre-handler, pre-question-parsing.

| Property | AllowedClients | Policy ALLOW |
|---|---|---|
| Layer | network | application |
| Response code | REFUSED | NXDOMAIN / blockpage IP |
| Domain context | none (pre-parse) | required |
| DoT/DoH coverage | bypassed (TLS auth) | enforced |
| Pre-amplification cost | O(1) ACL lookup | full evaluation |

Folding into Policy would: increase amplification surface; lose REFUSED
semantics; break DoT/DoH auth model.

**Action:** add a `docs/architecture/network-vs-application-acl.md` section
explaining the rationale.

---

## §5 LocalRecords — keep distinct, document why

LocalRecords produces **actual RR data**: A, AAAA, CNAME (with up-to-10-hop
chain resolution), TXT, MX (with priority), SRV (priority/weight/port/target),
NS, SOA, CAA, PTR. Per-record TTL.

Policy REDIRECT (`pkg/policy/engine.go:482-489`) accepts only a single
`net.ParseIP` ActionData. Hardcoded 300s TTL (`handler_policy.go:200,203`).
No CNAME chains, TXT, MX, SRV.

Folding requires either (a) major DSL extension where ActionData becomes a
structured payload, or (b) a new `SERVE_RR` action distinct from REDIRECT.
Either path adds DSL surface area larger than the entire feature it replaces.

**Action:** document the boundary in v0.26 architecture notes.

---

## §6 Blocklist overlays

### §6a Block Page IP — keep distinct

Output overlay on BLOCK decisions, not a competing routing decision. The DNS
side could in principle be a wildcard low-priority Policy REDIRECT, but BLOCK
terminates evaluation, so a "BLOCK then redirect" cascade isn't expressible in
Policy without semantic drift. The HTTP-side block page (HTML 403) is
HTTP-layer entirely.

**Action:** document; no code change.

### §6b Override IP / CNAME Override — delete

`Handler.Overrides map[string]net.IP` and `Handler.CNAMEOverrides
map[string]string` (`pkg/dns/handler.go:102-103`).

**Population sites — exhaustive search across all `.go` files:**

```
pkg/dns/handler.go:112-113       — initialized to empty maps
pkg/dns/handler_test.go          — test writes
pkg/dns/coverage_boost_test.go   — test writes
pkg/dns/final_coverage_test.go   — test writes
```

**Zero non-test population.** `hasOverrides.Load()` is always false in
production. The override branches in `handleFastBlocklistPath`
(`handler_blocklist.go:30-40`) and `handleLegacyBlocklistPath`
(`handler_blocklist.go:99-110`) are **unreachable**. `respondWithOverride` and
`respondWithCNAME` exist only for tests.

### Steps

Delete in v0.26:
- `Handler.Overrides`, `Handler.CNAMEOverrides`, `Handler.hasOverrides`
- `RefreshOverrideFlag()`, `lookupOverrides`
- Override branches in both blocklist paths
- `respondWithOverride`, `respondWithCNAME`
- Tests that write to these maps (delete; they test dead code)

### Risk

Effectively zero. Carries its own test mass — false sense of feature presence.
Removing tightens the codebase.

---

## §7 Unbound Forward Zones — keep distinct

Different layer (Unbound recursive pipeline). Folding into Policy FORWARD
loses:
- Unbound's message/RRset cache for forwarded responses
- DNSSEC validation (`forward_tls_upstream` for DoT-upstream)
- Hardening (`harden_glue`, `qname_minimisation`)
- dnstap correlation (`enrichFromUnbound` at `handler.go:248-261`)

Users running with Unbound chose Unbound for its recursive pipeline. Bypassing
it via Policy FORWARD is a feature — but the default for forward-zones is to
keep them in Unbound's hands.

**Action:** document; no code change.

---

## v0.26 release shape

### In scope
- §1 deprecation prep (migrator + warning + drop dead fields).
- §2 inert-feature cleanup (delete from API response, retarget importer, keep migrator running).
- §6b dead-code deletion.
- CC-1 migrator idempotency fix (UNIQUE index + sentinel + persist).
- CC-2 dead trace + metric removal.
- §3 design doc + DSL extension proposal (implementation can slip to v0.27).

### Out of scope (deferred)
- §1 actual deletion of Conditional Forwarding code paths (v0.27).
- §3 implementation if the cache-invalidation hooks need infra work.
- §4 / §5 / §6a / §7 — keep-distinct features get docs only.

### Test plan
- Migrator unit test: idempotent across N restarts (assert exactly one row per source entry).
- Conditional-forwarding migrator: round-trip a 5-rule fixture; assert
  - all 5 land in `policy_rules` with FORWARD action
  - `sort_order` preserves CF priority order
  - `cfg.ConditionalForwarding.Rules` is empty in YAML after migration
  - second restart adds zero new rows
- §6b deletion: confirm `make test` still green after dead test removal
  (orphan tests caught the dead path; nothing else should care).
- §3 DSL helper: hot-path bench (`make bench-whitelist` precedent) — assert
  `<10µs` per evaluation with a 10k-IP profile cache.

### Telemetry change-watch
- New: nothing required for §1 (existing forward-query counter labels suffice).
- Removed: `DNSWhitelistedQueries`. Anyone scraping it will see it disappear —
  release notes call it out.

---

## Open questions for the author

1. **§3 InClientGroup** — does the cache-invalidation infra land in v0.26 or
   v0.27? If the answer is "design only in v0.26", drop §3 from v0.26 scope.
2. **API surface for §1** — keep `/api/conditionalforwarding` as 410-Gone with
   pointer to `/api/policies`, or hard-remove? (User-facing — needs decision.)
3. **§6b risk** — is there ANY tooling outside this repo that calls
   `RefreshOverrideFlag()` (e.g. via SDK)? Verify before deletion.
