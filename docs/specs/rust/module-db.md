# Module `db`

Lifecycle: `Current`.

## Purpose

Describe **SQL Server catalog and audit I/O for the plan phase**: batched TDS round-trips, persistent catalog cache, checksum load, parallel ensure, and plan DB performance tracing.

## Scope

- `crates/core/src/db/plan_snapshot.rs` - L1 + warm snapshot + `run_plan_db_phase` entry
- `crates/core/src/db/plan_parallel.rs` - direct-connect parallel runner (`tokio::join` ensure ‖ checksums→inspect)
- `crates/core/src/db/plan_batch.rs` - sequential runner (`RMIG_SESSION` / rmigd)
- `crates/core/src/db/plan_common.rs` - shared cold / incremental / git-delta logic
- `crates/core/src/db/plan_db_trace.rs` - `PlanDbTrace` (`PlanDbTimings`, `PlanDbFlags`), SLO env, trace JSON
- `crates/core/src/db/catalog_inspect_cache.rs` - in-process inspector scope cache
- `crates/core/src/db/batch.rs` - combined SQL batch builder (plan bootstrap = tables only)
- `crates/core/src/db/catalog.rs` - catalog SQL composition, row merge
- `crates/core/src/db/catalog_cache.rs` - strict and relaxed cache load
- `crates/core/src/db/catalog_cache_save.rs` - batched cache save, workspace snapshot
- `crates/core/src/db/columns.rs` - on-demand column load (scaffold)
- `crates/core/src/db/state.rs` - `CatalogState`, `ChecksumMap`
- `sql/catalog/`, `sql/audit/` - embedded via `crates/core/src/sql/mod.rs`

## System context

`engine::run_command` calls `run_plan_db_phase`, which tries L1 (`cache::l1`), then warm snapshot, then plan DB I/O. On **direct connect** (`session_socket` empty), `plan_parallel::run_parallel` overlaps `audit::ensure_tables` (tables only) with checksum load + catalog inspect on a second connection. With `RMIG_SESSION`, `plan_batch::run_batch` runs the same logic sequentially on one connection.

Paths: `cold_full`, `git_delta`, `incremental`, `cache_hit`, `warm_snapshot`. Empty audit history skips OPENJSON checksum load (`audit::load_checksums` zero-digest map). Plan-path bootstrap omits `BOOTSTRAP_INDEX` (deferred to apply). Catalog save runs **outside** `parallel_wall_ms`.

## Interfaces and boundaries

- Public: `run_plan_db_phase`, `PlanDbResult`, `PlanDbTrace`, `save_batched`, `save_workspace_snapshot`, `load_table_columns`, `invalidate_inspect_cache`
- Inputs: `Config`, `TimingConn`, `Workspace`
- Outputs: checksums map, catalog state, phase timings fields
- Must not import `apply` (apply imports `db` for cache invalidation)

## Assumptions and constraints

- SQL Server OPENJSON; scope triples `(schema, kind, object)`.
- `RMIG_PLAN_DB_MAX_PAR_MS` (default 500) - workflow SLO on `parallel_wall_ms`.
- `RMIG_CATALOG_CACHE=0` disables persistent catalog cache.
- `RMIG_PLAN_DB_TRACE=1` appends to `ops/perf/artifacts/plan_db_trace.json`.

## Nominal flow

1. Resolve git delta (`gate::resolve_changed_paths`).
2. Optional L1 hit → return immediately.
3. Optional warm snapshot → seed L1 and return.
4. **Parallel (direct connect):** `tokio::join!(ensure_tables on conn₂, checksums→inspect on conn₁)`; `parallel_wall_ms = max(ensure, checksums+inspect)`.
5. **Git delta:** `audit::load_checksums` fast path; optional relaxed cache load; hot catalog SQL or inspector cache hit.
6. **Cold / incremental:** `audit::load_checksums` (skip OPENJSON when history empty); scoped catalog batch or single RT bootstrap+catalog when cold full + empty history.
7. `save_batched` after plan when cache enabled; `save_workspace_snapshot` after apply (engine).

## Off-nominal behavior and failure containment

- Missing catalog tables: graceful cache miss (`missing_catalog_table`).
- Empty history on cold DB: checksum probe only; catalog full inspect when configured.
- Inspector scope cache invalidated on full audit cache drop (`invalidate_inspect_cache`).

## Verification and validation

- `make plan-db-perf` - SLO gate + trace
- `make e2e-timings` - side-by-side phase report from e2e artifacts
- `make workflow-fast` - full workflow with fast DB reset
- Unit: `crates/core/src/db/batch.rs` tests, `catalog.rs` tests

## Operations and recovery

- SQL template changes: update `sql/` and matching tests.
- Perf regression: compare `plan_db_trace.json` between runs (`history_empty`, `checksums_skipped`, `round_trips`, `catalog_sql_ms`, `intern_catalog_ms`).

## Open issues and non-goals

- Non-goals: `db` does not open the primary connection (caller owns `TimingConn`); parallel path opens one extra direct connection for ensure overlap.

## References

- `docs/prod-gate.md` - plan DB SLO section
- `ops/perf/README.md`
- `docs/specs/rust/module-audit.md`
- `docs/specs/rust/module-driver.md`
