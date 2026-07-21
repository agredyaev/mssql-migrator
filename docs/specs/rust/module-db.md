# Module `db`

Lifecycle: `Current`.

## Purpose

Describe **SQL Server catalog and audit I/O for the plan phase**: batched TDS round-trips, persistent catalog cache, checksum load, bootstrap ensure, and plan DB performance tracing.

## Scope

- `crates/core/src/db/plan_snapshot.rs` - L1 + warm snapshot + `run_plan_db_phase` entry
- `crates/core/src/db/plan_common/execute/mod.rs` - plan-body runner (`execute`): sequential ensure-then-body, direct or via rmigd
- `crates/core/src/db/plan_common/mod.rs` - shared cold / incremental / git-delta logic
- `crates/core/src/db/plan_db_trace.rs` - `PlanDbTrace` (`PlanDbTimings`, `PlanDbFlags`), SLO env, trace JSON
- `crates/core/src/db/catalog_inspect_cache.rs` - in-process inspector scope cache
- `crates/core/src/db/batch.rs` - combined SQL batch builder (plan bootstrap = tables only)
- `crates/core/src/db/catalog.rs` - catalog SQL composition, row merge
- `crates/core/src/db/catalog_cache.rs` - strict and relaxed cache load
- `crates/core/src/db/catalog_cache_save/mod.rs` - batched cache save, workspace snapshot
- `crates/core/src/db/columns.rs` - on-demand column load (scaffold)
- `crates/core/src/db/state.rs` - `CatalogState`, `ChecksumMap`
- `sql/catalog/`, `sql/audit/` - embedded via `crates/core/src/sql/mod.rs`

## System context

`engine::run_command` calls `run_plan_db_phase`. Managed workspaces always perform plan DB I/O because every object needs a fresh live-state fingerprint; an L1 or warm snapshot cannot prove that SQL Server was not changed out of band. Empty workspaces may still use those snapshots. `plan_common::execute` runs a single sequential ensure-then-body path on the primary TDS session, whether **direct connect** (`session_socket` empty) or through `rmigd` (`RMIG_SESSION`).

Paths: `cold_full`, `git_delta`, `incremental`, `cache_hit`, `warm_snapshot`. Empty audit history skips OPENJSON checksum load (plan-side `load_checksums_plan` zero-digest map). Plan-path bootstrap omits `BOOTSTRAP_INDEX` (deferred to apply). Catalog save runs **outside** `parallel_wall_ms`.

## Interfaces and boundaries

- Public: `run_plan_db_phase`, `PlanDbResult`, `PlanDbTrace`, `save_batched`, `save_workspace_snapshot`, `load_table_columns`, `invalidate_inspect_cache`
- Inputs: `Config`, `TimingConn`, `Workspace`
- Outputs: checksums map, catalog state, phase timings fields
- Must not import `apply` (apply imports `db` for cache invalidation)

## Assumptions and constraints

- SQL Server OPENJSON; scope triples `(schema, kind, object)`.
- `RMIG_PLAN_DB_MAX_PAR_MS` (default 500) - workflow SLO on `parallel_wall_ms`.
- `RMIG_CATALOG_CACHE=0` disables persistent catalog cache.
- `RMIG_PLAN_DB_TRACE=1` appends to `plan_db_trace.json` under `ops/perf/artifacts/` (generated).

## Nominal flow

1. Resolve git delta (`gate::resolve_changed_paths`).
2. For an empty workspace only, an optional L1/warm-snapshot hit may return immediately.
3. For a managed workspace, load fresh audit/live-state checksums; local snapshots are bypassed.
4. **Bootstrap ensure:** when bootstrap is needed it runs before the plan body — eager under the command timeout on direct connect, or deferred into the body SQL batch under `rmigd`; `parallel_wall_ms` records the ensure-through-body wall time.
5. **Git delta:** plan-side `load_checksums_plan` fast path; optional relaxed cache load; hot catalog SQL or inspector cache hit.
6. **Cold / incremental:** plan-side `load_checksums_plan` (skip OPENJSON when history empty); scoped catalog batch or single RT bootstrap+catalog when cold full + empty history.
7. `save_batched` after plan when cache enabled; `save_workspace_snapshot` after apply (engine).

## Off-nominal behavior and failure containment

- Missing catalog tables: graceful cache miss (`missing_catalog_table`).
- Empty history on cold DB: checksum probe only; catalog full inspect when configured.
- Inspector scope cache invalidated on full audit cache drop (`invalidate_inspect_cache`).
- A stale local snapshot cannot hide managed-object drift because managed workspaces never return from the top-level L1/warm snapshot.

## Verification and validation

- `make plan-db-perf` - SLO gate + trace
- `make e2e-timings` - side-by-side phase report from e2e artifacts
- `make workflow-fast` - full workflow with fast DB reset
- Unit: `crates/core/src/db/batch.rs` tests, `catalog.rs` tests

## Operations and recovery

- SQL template changes: update `sql/` and matching tests.
- Perf regression: compare `plan_db_trace.json` between runs (`history_empty`, `checksums_skipped`, `round_trips`, `catalog_sql_ms`, `intern_catalog_ms`).

## Open issues and non-goals

- Non-goals: `db` does not open the primary connection (caller owns `TimingConn`); it does not maintain a general connection pool.

## References

- `docs/prod-gate.md` - plan DB SLO section
- `ops/perf/README.md`
- `docs/specs/rust/module-audit.md`
- `docs/specs/rust/module-driver.md`
