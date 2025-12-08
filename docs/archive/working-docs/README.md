# Working & Implementation Docs

This directory hosts living design notes, historical release plans, validation reports, and raw benchmark data. These files are intentionally separate from `docs/` so the published documentation stays concise while deep technical context remains preserved.

---

## Directory Map

- `plans/` – Active release plans, backlog tracking, and not-yet-finalized designs.
- `reports/` – Verification notes, QA summaries, and cleanup/postmortem reports.
- `performance-data/` – Raw benchmark artifacts that back the published performance numbers.
- `archive/` – Superseded performance docs that remain useful for regression analysis.

---

## Plans (`plans/`)

Finalized feature designs now live under `docs/designs/`. This directory is reserved for WIP/backlog planning artifacts:

| File | Focus | Highlights |
| --- | --- | --- |
| [`v0.6.1-plan.md`](plans/v0.6.1-plan.md) | Legacy override removal | Breaking-change analysis and migration workstream. |
| [`v0.7.3-technical-debt-plan.md`](plans/v0.7.3-technical-debt-plan.md) | Tech-debt burn down | Priority list, effort sizing, acceptance criteria. |
| [`v0.8.0-client-management-plan.md`](plans/v0.8.0-client-management-plan.md) | Client lifecycle | UI/API requirements, persistence model, staged rollout. |

---

## Reports (`reports/`)

| File | Scope | Notes |
| --- | --- | --- |
| [`v0.7.8-test-report.md`](reports/v0.7.8-test-report.md) | Release qualification | Full test coverage, defect log, and acceptance status. |
| [`test-report-2025-11-23.md`](reports/test-report-2025-11-23.md) | Weekly regression run | Pass/fail status plus follow-up tasks. |
| [`cleanup-report-2025-11-23.md`](reports/cleanup-report-2025-11-23.md) | Cache cleanup performance | Results from shard cleanup improvements and benchmarks. |

---

## Performance Data (`performance-data/`)

| File | Description |
| --- | --- |
| [`baseline_cache.txt`](performance-data/baseline_cache.txt) | Cache micro-benchmarks before the v0.7.22 optimization pass. |
| [`baseline_load.txt`](performance-data/baseline_load.txt) | Load-test output prior to the optimization pass. |
| [`phase2_benchmarks.txt`](performance-data/phase2_benchmarks.txt) | Post-optimization comparison data. |

These files are intentionally kept separate from the published docs but are referenced from `docs/performance/README.md` and `docs/INDEX.md`.

---

## Archive (`archive/`)

- [`PHASE1-BENCHMARK-RESULTS.md`](archive/PHASE1-BENCHMARK-RESULTS.md) – Phase 1 optimization report (superseded).
- [`PERFORMANCE-ROADMAP.md`](archive/PERFORMANCE-ROADMAP.md) – Historical performance roadmap and milestone tracking.

---

## Usage Guidelines

1. Treat these documents as living artefacts—update them as work progresses.
2. Once a design stabilizes, move it into `docs/designs/` (leaving only planning breadcrumbs here if needed).
3. Reference files using relative paths (as done above) so moves remain easy to track.
4. When adding new working material, also update this README to keep the index exhaustive.
