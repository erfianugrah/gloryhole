# v0.27 — Conditional Forwarding deletion + ClientGroups DSL

**Status:** in progress
**Predecessor:** v0.26 — policy consolidation (migrator + design doc shipped)
**Linked docs:**
- v0.26 plan: [`2026-05-25-v026-policy-consolidation.md`](2026-05-25-v026-policy-consolidation.md)
- §3 design: [`../designs/client-groups-as-policy-input.md`](../designs/client-groups-as-policy-input.md)

## Scope

Two coherent changes that both touch `pkg/policy` and benefit from a single coherent test pass:

1. **§1 deletion** — drop the now-redundant Conditional Forwarding code paths. Migrator stays (still needed on v0.25 → v0.27 upgrades), config schema stays one more cycle, runtime + API + UI go.
2. **§3 implementation** — `InClientGroup(ClientIP, "kids")` DSL helper per the design doc.

## §1 — Conditional Forwarding deletion

### Delete

| File | Action |
|---|---|
| `pkg/dns/handler_forwarding.go` | Drop `handleConditionalForwarding` (89 lines). Keep `forwardToUpstream` — called from elsewhere. |
| `pkg/dns/handler.go` | Remove `ruleEvaluator` field (L84), `getRuleEvaluator` (L132), wiring (L215), call site (L397). |
| `pkg/dns/conditional_forwarding_test.go` | Whole file. |
| `pkg/api/handlers_conditionalforwarding.go` | Whole file. |
| `pkg/api/handlers_conditionalforwarding_test.go` | Whole file. |
| `pkg/api/ui_handlers.go::handleConditionalForwardingPage` | Function only (L151-...). |
| `pkg/forwarder/evaluator.go` | Whole file. |
| `pkg/forwarder/evaluator_test.go` | Whole file. |
| `pkg/forwarder/matcher.go` | Whole file — used only by `evaluator.go`, verified by `rg`. |
| `pkg/forwarder/matcher_test.go` | Whole file. |
| `pkg/api/ui/dashboard/src/pages/forwarding.astro` | Whole file. |
| `pkg/api/ui/dashboard/src/components/ForwardingPage.tsx` | Whole file. |
| `pkg/api/ui/dashboard/src/lib/api.ts` | Drop `ConditionalForwardingRule` type + `fetchForwardingRules` / `addForwardingRule` / `removeForwardingRule` (L218, L494-512). |

### 410-Gone surface

The three `/api/conditionalforwarding*` routes get a single shared handler that returns:

```http
HTTP/1.1 410 Gone
Content-Type: application/json

{"error": "gone", "migrate_to": "/api/policies", "since_version": "0.26.0"}
```

Same handler swallows `GET`, `POST`, `DELETE`. Logged at `info` (not `warn`) so it doesn't drown the logs but is observable. Drop the 410 stub in v0.28 alongside the YAML schema.

The 2 UI routes (`/conditionalforwarding`, `/forwarding`) hard-remove — frontend has no reason to hit them after the page is gone, and any leftover bookmarks should land on the 404 page.

### Keep (deliberately)

- `pkg/config/conditional_forwarding.go` — YAML schema. Migrator still needs to read `conditional_forwarding:` blocks from old YAML on v0.25 → v0.27 upgrades. Drop in v0.28+ once the upgrade window has passed.
- `cmd/glory-hole/main.go::migrateConditionalForwardingToPolicies` + sentinel + `equalConditionalForwardingConfig` — same reason.
- `cmd/glory-hole/main.go::buildPolicyLogicFromCFRule` — used by the migrator.

### Call sites in `cmd/glory-hole/main.go` to clean up

- L883-884: `forwarder.NewRuleEvaluator(&cfg.ConditionalForwarding)` boot-time setup → delete.
- L1113-1116: hot-reload branch on CF config change → delete.
- L1434: `equalConditionalForwardingConfig` — keep, still used by migrator's drift check (verify; otherwise delete).

### Verification gates

- `go test ./...` green (whole module).
- `go build ./...` green.
- Bash search post-delete: `rg -n 'RuleEvaluator|handleConditionalForwarding|/api/conditionalforwarding' --type go pkg cmd` returns ONLY the migrator and the 410 stub.
- Frontend build: `npm run build` in `pkg/api/ui/dashboard/`.
- Manual smoke: glory-hole boots clean with a v0.25-shape YAML containing `conditional_forwarding:` rules; rules migrate to policies; second restart logs sentinel-hit warning; `GET /api/conditionalforwarding` returns 410.

## §3 — ClientGroups DSL implementation

Per [`docs/designs/client-groups-as-policy-input.md`](../designs/client-groups-as-policy-input.md). Summary of files added/touched:

| File | Action | Contents |
|---|---|---|
| `pkg/policy/clientgroups.go` | new | `ClientGroupResolver` interface, `noopResolver`, package-level `atomic.Pointer`, `SetClientGroupResolver`, `InClientGroup` DSL primitive, `sqliteResolver` |
| `pkg/policy/clientgroups_test.go` | new | resolver behavior tests + concurrent read race test |
| `pkg/policy/engine.go` | edit | register `InClientGroup` in `compileRuleLogic` |
| `pkg/policy/engine_bench_test.go` | edit | add `BenchmarkInClientGroup` — assert `<100 ns/op` and `0 allocs/op` |
| `cmd/glory-hole/main.go` | edit | construct `sqliteResolver`, initial `Reload`, `policy.SetClientGroupResolver(...)`, `apiServer.SetClientGroupResolver(...)` |
| `pkg/api/api.go` | edit | `Server` field `clientGroupResolver` + `SetClientGroupResolver` |
| `pkg/api/handlers_clients.go` | edit | call `s.reloadClientGroupCache(ctx)` after successful upsert / group-update / group-delete |

UI is intentionally unchanged in this release. Users with the visual-policy editor open can already write `InClientGroup(ClientIP, "kids")` in the freeform expression field. The dropdown integration ships in v0.28+.

### Verification gates

- All tests green including the new race test.
- Bench passes: `go test -bench=BenchmarkInClientGroup -benchmem ./pkg/policy/` shows `<100 ns/op` and `0 allocs/op` on a 10k-profile fixture.
- Manual smoke: create a group, assign an IP, write a policy `Action: BLOCK; Logic: InClientGroup(ClientIP, "kids")`, fire query from that IP → blocked. Reassign IP via API, no restart, fire query → allowed.

## Sequencing

§1 deletion lands first as 4 commits (runtime, API, forwarder, frontend) — pure removal, no test breakage outside the deleted files. §3 lands as 2-3 commits (impl + wiring + tests) on a clean tree.

Total: 6-7 commits on `main`. Tag as `v0.27.0` when both shipped, push.

## Risks

| Risk | Mitigation |
|---|---|
| Live config has `conditional_forwarding:` block at v0.27 boot | Migrator runs first thing in main.go; tested for idempotency. |
| 3rd-party tooling polls `/api/conditionalforwarding` | 410-Gone stub gives clear migration target. |
| Hot-path regression from new `InClientGroup` registration | Bench gate in CI. |
| `noopResolver` shipped to production by accident if init fails | Resolver init failure logs `WARN` but does not block boot — rules using `InClientGroup` evaluate to false, which is the safe default (no false-positive blocks). |
