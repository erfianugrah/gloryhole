# Feature Design Library

Authoritative home for Glory-Hole feature-level architecture and implementation references. Each document captures the “finished” design for a capability so operators and contributors can understand intent, constraints, and migration expectations without digging through historical working notes.

---

## Available Designs

| Document | Scope | Highlights |
| --- | --- | --- |
| [`kill-switch-design.md`](kill-switch-design.md) | Runtime feature toggles | Config schema, DNS handler wiring, REST/UI flows, telemetry, and rollout plan. |
| [`conditional-forwarding-plan.md`](conditional-forwarding-plan.md) | Conditional forwarding architecture | Use cases, data model, evaluation order, and configuration patterns. |
| [`v0.7.0-conditional-forwarding-implementation.md`](v0.7.0-conditional-forwarding-implementation.md) | Implementation deep dive | File-by-file changes, benchmarks, migration guide, and verification evidence. |
| [`dns-record-types-design-v0.7.2.md`](dns-record-types-design-v0.7.2.md) | Expanded authoritative record support | Data structures, resolver changes, storage schema, and compatibility notes. |
| [`v0.7.4-logging-metrics-plan.md`](v0.7.4-logging-metrics-plan.md) | Logging + telemetry overhaul | Structured logging strategy, kill-switch metrics, duration-based toggles, and rollout phases. |
| [`v0.7.8-dnssec-metrics-plan.md`](v0.7.8-dnssec-metrics-plan.md) | DNSSEC telemetry improvements | Metric catalog, Prometheus/OpenTelemetry integration, and validation workflow. |

---

## How To Use

1. Start here when you need to understand how a feature is intended to work (before reading working notes).
2. Use the linked migration/test sections to verify deployments or backports.
3. When finalizing a new feature, move the completed design document into this directory so it becomes discoverable by users/operators.
4. Keep in-progress brainstorming or per-sprint implementation details in `working-docs/` until the design stabilizes.
