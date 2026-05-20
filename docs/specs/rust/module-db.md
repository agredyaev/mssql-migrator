# Technical Document: Module `db`

Lifecycle: `Current`.

## Purpose

Describe **SQL Server catalog and audit I/O for the plan phase**: batched TDS round-trips, persistent catalog cache, checksum load, and plan DB performance tracing.

## Scope

- `rust/crates/core/src/db/plan_batch.rs` — batched plan DB runner (`run_batch`)
- `rust/crates/core/src/db/plan_snapshot.rs` — L1 + `run_plan_db_phase` entry
- `rust/crates/core/src/db/plan_db_trace.rs` — `PlanDbTrace`, SLO env, trace JSON
- `rust/crates/core/src/db/batch.rs` — combined SQL batch builder
- `rust/crates/core/src/db/catalog.rs` — catalog SQL composition, row merge
- `rust/crates/core/src/db/catalog_cache.rs` — strict and relaxed cache load
- `rust/crates/core/src/db/catalog_cache_save.rs` — batched cache save, workspace snapshot
- `rust/crates/core/src/db/columns.rs` — on-demand column load (scaffold)
- `rust/crates/core/src/db/state.rs` — `CatalogState`, `ChecksumMap`
- `rust/sql/catalog/`, `rust/sql/audit/` — embedded via `rust/crates/core/src/sql/mod.rs`

## System context

`engine::run_command` calls `run_plan_db_phase`, which tries L1 (`cache::l1`) then `run_batch`. Paths: `cold_full`, `git_delta`, `incremental`, `cache_hit`. Git-delta fast path loads relaxed `catalog_cache` + checksums in one RT; skips hot catalog SQL when cache covers delta keys. Catalog save runs **outside** `parallel_wall_ms`.

## Interfaces and boundaries

- Public: `run_plan_db_phase`, `PlanDbResult`, `PlanDbTrace`, `save_batched`, `save_workspace_snapshot`, `load_table_columns`
- Inputs: `Config`, `TimingConn`, `Workspace`
- Outputs: checksums map, catalog state, phase timings fields
- Must not import `apply` (apply imports `db` for cache invalidation)

## Assumptions and constraints

- SQL Server OPENJSON; scope triples `(schema, kind, object)`.
- `RMIG_PLAN_DB_MAX_PAR_MS` (default 500) — workflow SLO on `parallel_wall_ms`.
- `RMIG_CATALOG_CACHE=0` disables persistent cache.
- `RMIG_PLAN_DB_TRACE=1` appends to `ops/perf/artifacts/rust_plan_db_trace.json`.

## Nominal flow

1. Resolve git delta (`gate::resolve_changed_paths`).
2. Optional L1 hit → return immediately.
3. **Git delta:** batch relaxed cache load + checksums; optional hot catalog; merge stable objects.
4. **Cold / incremental:** checksums batch → scoped catalog batch.
5. `save_batched` after plan when cache enabled; `save_workspace_snapshot` after apply (engine).

## Off-nominal behavior and failure containment

- Missing catalog tables: graceful cache miss (`missing_catalog_table`).
- Empty history on cold DB: skip catalog query when scope empty.

## Verification and validation

- `make rust-plan-db-perf` — SLO gate + trace
- `make rust-workflow-fast` — full workflow with fast DB reset
- Unit: `rust/crates/core/src/db/batch.rs` tests, `catalog.rs` tests

## Operations and recovery

- SQL template changes: update `rust/sql/` and matching tests.
- Perf regression: compare `rust_plan_db_trace.json` between runs.

## Open issues and non-goals

- Non-goals: `db` does not open connections (caller owns `TimingConn`).

## References

- `docs/prod-gate.md` — plan DB SLO section
- `ops/perf/README.md`
- `docs/specs/rust/module-audit.md`
- `docs/specs/rust/module-driver.md`
