# ClientGroups as a first-class policy DSL input

**Status:** design (v0.26 §3) — implementation deferred to v0.27
**Linked plan:** [`docs/plans/2026-05-25-v026-policy-consolidation.md`](../plans/2026-05-25-v026-policy-consolidation.md) §3

## Problem

ClientGroups today is **strictly UI labeling**. The `client_groups` and
`client_profiles` SQLite tables are joined via `GetClientSummaries` to render
the `/api/clients` page, but **no DNS handler reads group membership**.
Verified by exhaustive grep across `pkg/dns/`: only mock fixtures reference
`GroupName`.

This is wasted infrastructure. The system already has a UI for assigning IPs
to groups like "kids" or "iot" — the only missing piece is making policy rules
able to say "if the client is in group 'kids', block this".

Today users have to write:

```text
IPInCIDR(ClientIP, "192.168.1.50/32") || IPInCIDR(ClientIP, "192.168.1.51/32") || IPInCIDR(ClientIP, "192.168.1.55/32")
```

We want them to write:

```text
InClientGroup(ClientIP, "kids")
```

…and have edits to the group membership in the Clients UI take effect on the
next DNS query without restarting glory-hole.

## Goals

1. New DSL helper `InClientGroup(clientIP, groupName) bool` available to all
   policy rule logic.
2. Sub-microsecond hot-path cost on cache hit (no DB query per DNS request).
3. Group-membership edits in the UI take effect on the next DNS query (no
   restart, no per-rule recompile).
4. Unprofiled IPs return `false` cleanly — no skip-rule semantics, no errors.
5. Cache invalidation is explicit and cheap (single atomic swap, mirror of
   `BlocklistManager` pattern).

## Non-goals

- Group hierarchy / nesting. Flat string names only.
- Time-based group membership (e.g. "in 'kids' from 18:00 to 20:00"). Compose
  with `InTimeRange` in the same rule logic — out of scope here.
- Per-group default actions. Groups are *inputs* to rules, not policies
  themselves.
- Negative lookups (`NotInGroup`) — write `!InClientGroup(...)` in the DSL.

## Design

### Data flow

```
   Clients UI                         DNS query
       │                                  │
       ▼                                  ▼
  PUT /api/clients/profiles          handler → policy.Evaluate(ctx)
       │                                  │
       ▼                                  ▼
  SQLite UPDATE                      InClientGroup(ip, name)
       │                                  │
       ▼                                  ▼
  reloadClientGroupCache()       resolver.IsInGroup(ip, name)
       │                                  │
       └────────► atomic.Store ────►──── atomic.Load
                  (build map)             (O(1) hit)
```

### Resolver interface

New in `pkg/policy/clientgroups.go`:

```go
package policy

// ClientGroupResolver answers "is this IP in this group?" — backed by an
// in-memory cache that is rebuilt when the source-of-truth (SQLite) changes.
//
// Implementations MUST be safe for concurrent reads from the DNS hot path.
// IsInGroup MUST NOT block on I/O.
type ClientGroupResolver interface {
    IsInGroup(clientIP, groupName string) bool
}

// noopResolver is the default until SetClientGroupResolver is called.
// All membership queries return false. This means InClientGroup() in rule
// logic cleanly evaluates to false on systems that haven't wired up a
// resolver — no compile errors, no runtime errors, no skip-rule semantics.
type noopResolver struct{}

func (noopResolver) IsInGroup(_, _ string) bool { return false }
```

### Cache shape

In-memory: `map[string]map[string]struct{}` — outer key is client IP, inner
set is the groups that IP belongs to. Total cardinality bounded by row count
in `client_profiles` (typically <1000 on any home deployment).

```go
type cachedResolver struct {
    cache atomic.Pointer[map[string]map[string]struct{}]
}

func (r *cachedResolver) IsInGroup(clientIP, groupName string) bool {
    m := r.cache.Load()
    if m == nil {
        return false
    }
    groups, ok := (*m)[clientIP]
    if !ok {
        return false
    }
    _, ok = groups[groupName]
    return ok
}
```

`atomic.Pointer[T]` swap is the same idiom as `pkg/blocklist/manager.go:41`.
Reads are lock-free, writes are a single pointer store.

### Wiring

Package-level resolver, set once at engine init:

```go
// pkg/policy/clientgroups.go
var resolver atomic.Pointer[ClientGroupResolver]

func init() {
    var n ClientGroupResolver = noopResolver{}
    resolver.Store(&n)
}

func SetClientGroupResolver(r ClientGroupResolver) {
    resolver.Store(&r)
}

// InClientGroup is the DSL primitive registered with expr-lang.
func InClientGroup(clientIP, groupName string) bool {
    return (*resolver.Load()).IsInGroup(clientIP, groupName)
}
```

Why package-level rather than per-engine: `compileRuleLogic` is a free
function (not a method on Engine), and `Rule.Compile()` runs before the rule
is attached to any engine instance. A package-level atomic pointer is the
minimum-surface change. It also matches the existing pattern — `IPInCIDR` and
friends are already package-level.

Registration at compile time mirrors existing helpers in `engine.go:174-186`:

```go
expr.Function("InClientGroup",
    func(params ...any) (any, error) {
        ip, e := asString(params[0], "InClientGroup.ip")
        if e != nil {
            return false, e
        }
        group, e := asString(params[1], "InClientGroup.group")
        if e != nil {
            return false, e
        }
        return InClientGroup(ip, group), nil
    },
    new(func(string, string) bool),
),
```

### Cache lifecycle

Build the cache from `storage.GetClientSummaries`. Each summary row has a
`ClientIP` and `GroupName` — a flat join already.

```go
type sqliteResolver struct {
    storage storage.Storage
    cache   atomic.Pointer[map[string]map[string]struct{}]
}

func (r *sqliteResolver) Reload(ctx context.Context) error {
    summaries, err := r.storage.GetClientSummaries(ctx, /* limit=all */)
    if err != nil {
        return err
    }
    m := make(map[string]map[string]struct{}, len(summaries))
    for _, s := range summaries {
        if s.GroupName == "" {
            continue
        }
        groups, ok := m[s.ClientIP]
        if !ok {
            groups = make(map[string]struct{}, 1)
            m[s.ClientIP] = groups
        }
        groups[s.GroupName] = struct{}{}
    }
    r.cache.Store(&m)
    return nil
}
```

Note: a client can be in **multiple groups** in this design even though the
current schema (`client_profiles.group_name` is a single `TEXT` column) has a
1:1 IP→group constraint. Future schema migration to a join table is trivial
without changing the resolver interface.

### Invalidation hooks

Three mutation paths exist for the underlying tables:

| Endpoint | Handler | Hook |
|---|---|---|
| `POST /api/clients/profiles` | `handleUpsertClientProfile` | call `resolver.Reload(ctx)` after success |
| `POST /api/clients/groups` | `handleCreateClientGroup` | no IP membership change — skip |
| `PUT /api/clients/groups/{name}` | `handleUpdateClientGroup` | rename: rebuild |
| `DELETE /api/clients/groups/{name}` | `handleDeleteClientGroup` | clears `group_name` to NULL on rows — rebuild |

`Reload` is cheap: <1ms even at 10k profiles (single SQLite query, single map
build). It's still strictly better to skip it on the no-op paths — keep the
HTTP latency tight.

The hook lives on `*Server` in `pkg/api/handlers_clients.go`:

```go
func (s *Server) reloadClientGroupCache(ctx context.Context) {
    if s.clientGroupResolver == nil {
        return
    }
    if err := s.clientGroupResolver.Reload(ctx); err != nil {
        s.logger.WarnContext(ctx, "client group cache reload failed",
            slog.String("err", err.Error()))
    }
}
```

Engine init wires it once:

```go
// in pkg/engine startup wiring (cmd/glory-hole/main.go)
resolver := policy.NewSQLiteClientGroupResolver(stor)
if err := resolver.Reload(ctx); err != nil {
    logger.WarnContext(ctx, "initial client group cache build failed",
        slog.String("err", err.Error()))
}
policy.SetClientGroupResolver(resolver)
apiServer.SetClientGroupResolver(resolver) // for the invalidation hooks
```

## Hot-path performance

Cost per `InClientGroup` evaluation:

1. `atomic.Pointer.Load()` — 1 ns, lock-free.
2. Outer map lookup by string IP — ~10-20 ns at typical cache sizes.
3. Inner set membership — ~10 ns.

Total: **<50 ns**. The plan's `<10µs` budget is two orders of magnitude over
this — easily met.

Validation: extend `pkg/policy/engine_bench_test.go` with a benchmark that:
- builds a cache of 10k profiles, each in 1 of 10 groups
- evaluates a rule `InClientGroup(ClientIP, "kids")` 1M times
- assert `<100 ns/op` and `0 allocs/op`

The 0-allocs claim hinges on:
- `(*resolver.Load()).IsInGroup(...)` — pointer deref through interface, no boxing
- map lookups by string IP — no allocation if the key is already a `string`
  (which it is — `Context.ClientIP` is `string`)

## UI integration

"Add Policy" form gets a new matcher dropdown alongside the existing
"client IP" / "domain" / "query type" fields:

- **Client group** — populates from `GET /api/clients/groups`. Selection
  synthesizes `InClientGroup(ClientIP, "kids")` into the rule logic field.

When loading an existing rule for edit, the visual editor already parses the
expression tree (`pkg/api/ui/dashboard/src/components/policy/ConditionEditor.tsx`).
A new node type `ClientGroupNode` joins the existing `IPInCIDRNode`,
`DomainMatchesNode`, etc.

API: no new endpoint required. The frontend reads `GET /api/clients/groups`
to populate the dropdown.

## Test plan

Unit tests in `pkg/policy/clientgroups_test.go`:

- `TestNoopResolver_AlwaysFalse` — sanity check default state.
- `TestSQLiteResolver_BuildAndQuery` — seed a mock storage with 3 profiles in
  2 groups, build, verify each lookup.
- `TestSQLiteResolver_ReloadAfterMutation` — mutate the underlying mock
  storage, call Reload, verify the cache reflects the change.
- `TestSQLiteResolver_UnprofiledIP` — IP not in any profile returns false for
  every group name.
- `TestSQLiteResolver_MissingGroupName` — IP in a profile but with empty
  `GroupName` returns false for every group.
- `TestSQLiteResolver_ConcurrentReadDuringReload` — `t.Parallel` with one
  reload goroutine and N reader goroutines; race detector clean.

Integration test in `test/integration_test.go`:

- Spin up glory-hole with a test SQLite, create a group "kids", add an IP to
  it, load a policy `Action: BLOCK; Logic: InClientGroup(ClientIP, "kids")`,
  fire a DNS query from that IP, assert NXDOMAIN.
- Mutate group membership via API, assert the next DNS query reflects the
  new state without restart.

Bench test as described above.

## Risks and mitigations

| Risk | Mitigation |
|---|---|
| Hot-path lookup blocks on storage | Atomic-pointer cache means I/O never happens during DNS evaluation |
| Cache stale after manual SQL edit | Documented limitation. Recovery: restart, or hit any client API endpoint to retrigger reload |
| Resolver registered too late (rules compiled before SetClientGroupResolver) | `noopResolver` is the default — rules compile and evaluate to false until the real resolver lands. No panic, no NPE |
| Multi-group future schema change | Resolver interface is stable — only the cache builder changes |
| Profile coverage gaps surface as "no policy match" silently | Telemetry: emit a counter `policy_client_group_unknown_ip_total` on the first lookup for an unprofiled IP, sampled |

## Open questions

1. **Should `InClientGroup` accept variadic groups?**
   `InClientGroup(ClientIP, "kids", "iot")` returning true if the IP is in
   *any* of them. Marginal ergonomic win, ~2x the inner-loop cost. Defer
   until a user asks.

2. **Should `client_profiles.group_name` migrate to a join table now?**
   The resolver's data structure already supports multi-group. Migration is
   straightforward but invasive (touches the upsert handler, the listing
   endpoints, the UI badge). Defer to v0.29+ unless a user request lands.

3. **Should we expose group-of-IP as a separate DSL primitive?**
   E.g. `ClientGroup(ClientIP) == "kids"`. Allows `==`, `!=`, etc. comparisons.
   Awkward in the multi-group future since the function would need to return
   a list. Rejected — `InClientGroup` is the right shape.

## v0.26 vs v0.27 split

Per the v0.26 plan, **only this design doc lands in v0.26**. Implementation
ships in v0.27 alongside the §1 Conditional Forwarding code-path deletion —
both touch `pkg/policy` and benefit from a single coherent test pass.

This answers the v0.26 plan's open question 1: "InClientGroup — does the
cache-invalidation infra land in v0.26 or v0.27? If the answer is 'design
only in v0.26', drop §3 from v0.26 scope." → **design only in v0.26;
implementation in v0.27.**
