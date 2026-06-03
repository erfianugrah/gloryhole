# Config → YAML — reverse the SQLite source-of-truth split

Status: **planned** (deferred implementation). Author note: triggered by a
production data-loss incident during the fra → sin Fly region migration on
2026-06-03.

## Why now

A region migration cloned the machine and started the Singapore instance on a
**fresh volume** (query-log history was intentionally dropped). Preserving
`config.yml` was assumed sufficient to carry all configuration across. It was
not: **policy rules vanished**, because they live in SQLite (`policy_rules`),
not in `config.yml` (which carried `policy.rules: []`).

This is the direct cost of the v0.26 decision (CC-3 in
`2026-05-25-v026-policy-consolidation.md`):

> *Dynamic, frequently-edited routing rules live in SQLite. Static system
> topology / infrastructure config lives in YAML.*

The rule conflates two axes — *edit frequency* and *whether something is
configuration*. A policy rule is configuration regardless of how often it
changes. Storing it in the same store as the 680 MB query log means any
operation that resets the runtime DB (region move, volume rebuild, corruption
recovery, `rm *.db`) silently destroys user configuration with no warning and
no representation in the file the operator backs up.

**New rule:**

> **All configuration lives in `config.yml`. SQLite holds only runtime data —
> the query log and derived stats. Resetting the database must never lose
> configuration.**

## Why this is tractable

The YAML write-back machinery already exists and is proven in production:

- `config.Save(path, cfg)` — `pkg/config/config.go:333` — atomic temp-write +
  rename, mode 0600.
- `(*Server).persistConfigSection(...)` — `pkg/api/settings_page.go:50` — the
  shared API helper that mutates a config section, `config.Save`s it, and calls
  `configWatcher.Reload()`.

Seven sections already persist through this path on the writable Fly volume:
blocklists, upstreams, cache, logging, TLS, block page, local records. Policy
rules, `allowed_clients`, and client groups are the only mutable config that
deliberately opt out into SQLite.

The original CC-1 justification ("YAML write-back is unreliable") was actually a
single migrator bug — `migrateWhitelistToPolicies` wrote rules to SQLite but
failed to write `cfg.Whitelist = nil` back to disk, so rules duplicated every
restart. The chosen remedy was "make SQLite authoritative" instead of "fix the
write-back + add a `UNIQUE(name)` guard + a sentinel" (all three of which the
v0.26 plan also shipped). With the migrator idempotency now fixed, the reason to
keep config in SQLite is gone.

## Scope

### Moves to YAML (config)

| Item | Current store | Target YAML key | Notes |
|---|---|---|---|
| Policy rules | `policy_rules` table | `policy.rules` | The thing that bit us. Needs a stable `id`. |
| Allowed-clients ACL | `dynamic_config[allowed_clients]` | `server.allowed_clients` | Already half-YAML — startup seeds DB from YAML when empty. Just stop the DB round-trip. |
| Client groups | `client_groups` table | new `clients.groups` | No config struct exists today — must be created. |
| Client profile labels | `client_profiles` table | new `clients.profiles` | **Hybrid** — see Hard Problem 4. Only `display_name`/`notes`/`group_name` are config; the `client_ip` key is discovered. |

### Stays in SQLite (runtime)

`queries`, `client_stats`, `hourly_stats`, `unbound_queries`. No change.

### Migration sentinels

`dynamic_config[whitelist_migrated_at]`,
`dynamic_config[conditional_forwarding_migrated_at]` are genuine runtime
one-shot guards. They can stay in `dynamic_config` (the table survives; only the
`allowed_clients` key leaves it), or be retired if the migrators they guard are
removed. Recommend: leave them; `dynamic_config` becomes a pure
runtime-sentinel store.

## Hard problems + solutions

### HP-1. Stable rule IDs

The REST API addresses rules by integer `id` (SQLite `AUTOINCREMENT`). YAML
`PolicyRuleEntry` (`pkg/config/config.go:241`) has no ID — it is an ordered
list. Array-index IDs are unstable across reorders/deletes and break concurrent
clients.

**Solution:** add a stable `ID string` (`yaml:"id"`) to `PolicyRuleEntry`,
populated with a short ULID/uuid at creation. Add `SortOrder int`
(`yaml:"sort_order"`) or rely on list order — prefer explicit `sort_order` so a
hand-edited file with reordered entries behaves predictably. The reverse
migration (HP-3) assigns IDs to existing DB rows.

```go
type PolicyRuleEntry struct {
    ID         string `yaml:"id"`          // stable, assigned at creation
    Name       string `yaml:"name"`
    Logic      string `yaml:"logic"`
    Action     string `yaml:"action"`
    ActionData string `yaml:"action_data"`
    Enabled    bool   `yaml:"enabled"`
    SortOrder  int    `yaml:"sort_order"`
}
```

`UNIQUE(name)` (DB-enforced today) moves to app-level validation in the add/
update handlers.

### HP-2. Concurrent API writes

SQLite serializes writes; the YAML read-modify-write-save cycle does not. Two
simultaneous `POST /api/policies` would each `Load → mutate → Save`,
last-write-wins clobbering one. The existing `persistConfigSection` callers
share this latent race at lower write frequency.

**Solution:** a single `sync.Mutex` on `*Server` guarding the whole
read-modify-save cycle for *all* config mutations (not just policy). Cheap —
config writes are rare and human-driven even for "frequently edited" rules.
Wrap `persistConfigSection` and the new policy/client handlers in it.

### HP-3. Reverse migration on upgrade

Existing deployments have live rows in SQLite that YAML lacks. A one-shot
SQLite → YAML export is required at startup, mirroring the existing
sentinel-guarded forward migrators (`main.go:76`, `:198`).

**Solution:** `migratePolicyRulesToYAML` + `migrateClientConfigToYAML` +
`migrateAllowedClientsToYAML`, each:
1. Guard on a `dynamic_config` sentinel (`policy_rules_to_yaml_at` etc.) — skip
   if set.
2. Read all rows from the respective table.
3. Merge into `cfg.*` (assign stable IDs for policy rules), `config.Save`.
4. Set the sentinel.
5. (Optional, later release) drop the now-dead tables in a follow-up migration
   once the export is confirmed across all deployments — keep them one release
   for rollback safety.

Idempotent by sentinel; safe to run on a read-only baked config because the
runtime config path on the Fly volume is writable (proven by the 7 existing
YAML sections).

### HP-4. Discovered-vs-declarative mixing (`client_profiles`)

`client_profiles.client_ip` is runtime-discovered; only the labels are config.
Dumping the whole table into YAML grows the file unboundedly as new clients
appear and inflates every watcher reload.

**Solution:** split the concern.
- Keep client *discovery* + counts in `client_stats` (runtime, SQLite) — IPs are
  observed, not configured.
- Put only *labels for IPs the operator has annotated* into
  `clients.profiles` (rows where `display_name`/`notes`/`group_name` are
  non-empty). An un-annotated client has no YAML entry; it is purely a runtime
  stat row. The join happens in the read path / resolver cache.

### HP-5. Hot-reload self-trigger + rebuild cost

`config.Save` fires fsnotify (Write) → `OnChange`, *and* `persistConfigSection`
calls `configWatcher.Reload()` explicitly → double reload. Adding policy/client
rebuilds to `OnChange` means every API write rebuilds the policy engine twice.

**Solution:** add policy + allowed-clients + client-group branches to the
`OnChange` callback (`main.go:1019`) with old-vs-new diff guards (the existing
branches already do this — e.g. cache, logging). Add `equalPolicyRules` /
`equalAllowedClients` comparators so a no-op diff skips the rebuild. Remove the
explicit `configWatcher.Reload()` from `persistConfigSection` *or* make
`OnChange` idempotent under double-fire — prefer the diff-guard approach since
it also protects against hand-edits.

Delete the stale note at `main.go:1029-1031`.

## Implementation order

1. **Config structs** — add `ID`/`SortOrder` to `PolicyRuleEntry`; add
   `ClientsConfig` (`groups`, `profiles`) with structs; wire into top-level
   `Config`. Add validation (unique names, valid actions, valid CIDRs).
2. **Server mutation mutex** (HP-2) — wrap `persistConfigSection`.
3. **Policy handlers** (`handlers_policy.go`) — rewrite Add/Update/Delete to
   mutate `cfg.Policy.Rules` + `config.Save` instead of `storage.*PolicyRule`.
   Drop `rebuildPolicyEngine`-from-DB; rebuild from `cfg` in `OnChange`.
4. **Allowed-clients** (`handlers_config_update.go:245`) — route through
   `persistConfigSection`; remove DB seed/load in `main.go:875-890`.
5. **Client handlers** (`handlers_clients.go`) — rewrite upsert/delete to YAML
   per HP-4.
6. **OnChange branches + diff guards** (HP-5).
7. **Reverse migrators** (HP-3) — sentinel-guarded SQLite→YAML export.
8. **Startup load** (`main.go:796-858`, `:868-874`) — build engine + ACL +
   client resolver from `cfg`, not the DB.
9. **Tests** — see below.
10. **Follow-up release** — drop dead tables (`policy_rules`, `client_groups`,
    `client_profiles`; `dynamic_config[allowed_clients]` key) after one
    release's rollback window.

## Test plan

- Config round-trip: `Save` → `Load` preserves policy rules incl. `id`/
  `sort_order`; YAML schema golden test.
- Reverse migration: seed `policy_rules` + `client_groups` in a temp DB, run
  startup, assert `config.yml` now contains them with stable IDs and the
  sentinel is set; re-run asserts no-op (idempotent).
- Concurrency: N goroutines each `POST /api/policies`; assert no lost writes,
  final rule count == N, no duplicate IDs (HP-2).
- Hot-reload: edit `policy.rules` on disk; assert the engine rebuilds and a
  matching query is now blocked/allowed; assert a no-op save does **not** rebuild
  (diff guard, HP-5).
- The migration-loss regression: simulate "fresh volume + preserved config.yml"
  → assert all policy rules + client groups + ACL are present after boot. This is
  the exact scenario that motivated the plan.

## Out of scope

- Dropping the dead SQLite tables (deferred one release for rollback safety).
- Any change to query-log / stats storage — those correctly stay in SQLite.
- TOML/JSON config formats — YAML stays the single config format.

## Recovery note (this incident)

The fra volume snapshots **survived the volume destroy**
(`vs_BBYRq2w02RXh02yAmDKg`, ~21 h before teardown, 5-day retention). If the lost
policy rules are wanted back, restore the snapshot to a temp volume, attach a
throwaway machine, `sqlite3 glory-hole.db 'SELECT * FROM policy_rules'`, and
re-create the rules via the API. Window closes ~2026-06-08.
