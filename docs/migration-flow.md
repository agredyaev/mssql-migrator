# Migration Flow

Lifecycle: `Current`.

## Purpose

Describe the end-to-end migration lifecycle of `rmig`: how a repository tree
becomes a plan and is applied to SQL Server, and exactly how failures are
contained. It answers: "What happens, in order, when I run `plan` or `migrate`?"

## Scope

- Engine entry and per-database run: `crates/core/src/engine/run/mod.rs`, `crates/core/src/engine/run/database.rs`.
- Discovery and parsing: `crates/core/src/scan` (see `docs/repository-contract.md`).
- Catalog load and diff: `crates/core/src/db`, `crates/core/src/plan/diff.rs`.
- Apply and transactions: `crates/core/src/apply` and `sql/apply/begin_transaction.sql`.
- Out of scope: CI invocation (see `docs/ci-usage.md`) and the on-disk contract (see `docs/repository-contract.md`).

## System Context

`rmig` runs as a CLI (`crates/cli/src/main.rs`), optionally proxying SQL through
the `rmigd` daemon for a warm connection. It is stateless and idempotent: it
reads the catalog, computes a diff, and applies only what changed. Re-running a
clean migration is a safe no-op.

The diff is repository-bounded: it enumerates only the repository objects in the
workspace, never the full catalog. Objects present in the live database but absent
from the repository tree are therefore never created, altered, or dropped — see
`docs/repository-contract.md` for the managed/unmanaged/orphaned model.

## Interfaces And Boundaries

- Inputs: the normalized workspace from scan, plus the live catalog state.
- Outputs: a `MigrationPlan` (and optional JSON), and applied changes plus audit history rows.
- Ownership boundaries: planning is owned by `crates/core/src/plan`; execution by `crates/core/src/apply`; connection handling by `crates/core/src/driver/mssql.rs` and `crates/core/src/session`.

## Assumptions And Constraints

- Assumptions: the catalog is reachable; the repository passed contract validation.
- Constraints:
  - Plan output is deterministic; `plannedAt` is metadata only and can be pinned with `RMIG_PLANNED_AT` or `SOURCE_DATE_EPOCH`.
  - Every catalog query and connect is bounded by `command_timeout` (default 30s) so the tool cannot hang.
  - Transactional object kinds and table transitions run inside `SET XACT_ABORT ON; BEGIN TRANSACTION ... COMMIT TRANSACTION`.
  - Mutating commands acquire the `sp_getapplock` advisory lock before inspecting the catalog, so planning and apply observe one consistent, locked database state; read-only commands do not lock.
  - `CREATE SCHEMA` is idempotent and batch-safe: it is guarded by `IF SCHEMA_ID(N'...') IS NULL EXEC('CREATE SCHEMA [...]')`.
  - The diff iterates only workspace objects (`crates/core/src/plan/diff.rs`), never the full catalog, so an object's absence from the tree cannot generate a `DROP` or `ALTER`. There is no drop action in the plan model.
  - `baseline` (first adoption) records a checksum only for repository objects already present in the database; database-only objects are not recorded or managed.

## Nominal Flow

1. Discover databases (one per top-level directory, or `cfg.database`) and scan the tree once (`crates/core/src/engine/run/mod.rs`).
2. Connect (direct TDS or via `rmigd`) — `crates/core/src/engine/run/database.rs`.
3. Read-only commands (`plan`, `validate`) inspect the catalog and diff without the lock (`crates/core/src/engine/run/plan_phase.rs`).
4. Mutating commands (`migrate`, `baseline`, `repair-checksum`) acquire the advisory lock first, then inspect, diff, and apply entirely inside the lock so a concurrent migrator cannot make the plan stale (`crates/core/src/engine/apply_run.rs`).
5. Apply order: ensure audit tables, apply schemas (idempotent `IF SCHEMA_ID ... EXEC('CREATE SCHEMA ...')`), then objects, then table transitions (`crates/core/src/apply/mod.rs`).
6. Flush audit history, invalidate caches, release the lock, and return exit code 0 on success.

## Off-Nominal Behavior And Failure Containment

- Failure mode: a planned object fails to execute.
  Containment: its transaction rolls back (`SET XACT_ABORT ON` plus a `@@TRANCOUNT`-guarded ROLLBACK), the apply stops at the first failure, remaining objects/transitions are not attempted, accumulated audit history is still flushed, and the run returns `Error::Sql` (exit 5).
- Failure mode: the ROLLBACK itself fails.
  Containment: the rollback error is recorded in the result, signalling unknown connection state instead of running the next object in a zombie transaction (`crates/core/src/apply/objects_exec.rs`).
- Failure mode: SQL Server is unreachable or unresponsive.
  Containment: connect and per-command timeouts return a clear error (exit 3 for connect, exit 5 for query timeout) rather than hanging.
- Failure mode: the plan is blocked (structural gate).
  Containment: `migrate` returns exit 10 (`EXIT_PLAN_BLOCKED`) before touching data.
- Failure mode: a second migrator runs concurrently against the same database.
  Containment: the advisory lock serializes them; the second blocks until the first releases, then plans against the now-current catalog (the plan is computed under the lock), so it cannot apply a stale plan. A lock-acquisition timeout returns exit 7 (`EXIT_LOCK_TIMEOUT`).
- Failure mode: a schema in the plan already exists (cache drift).
  Containment: `CREATE SCHEMA` is guarded by `IF SCHEMA_ID(...) IS NULL`, so re-creating an existing schema is a safe no-op rather than an aborting error 2714.
- Failure mode: the database contains objects not represented in the repository (an existing database adopted by a partial tree).
  Containment: planning never enumerates them, so `plan`, `migrate`, and `baseline` leave them untouched; no code path drops or alters an object based on its absence from the tree.

## Verification And Validation

- Contracts and checks: `crates/core/src/apply/tx.rs` test (XACT_ABORT wrapping), `crates/core/src/tests/proxy_test.rs` (command timeout), `crates/core/src/tests/schema_sql_test.rs` (idempotent, injection-safe `CREATE SCHEMA`), `crates/core/src/tests/command_mutates_test.rs` (only mutating commands lock), and the SQL integration suite under `ops/perf/sql_regression.sh`.
- Existing-database safety: `crates/core/tests/unmanaged_objects_test.rs` (offline diff guards — absence never drops; the production gate is fail-closed on blocked plans) and `crates/core/tests/existing_db_adoption_integration.rs` (real-database preservation across `migrate` and read-only `plan`).
- Evidence artifacts: plan JSON, audit history rows, and `make check-e2e` output.
- Exit criteria: a clean apply succeeds; a failing apply stops at the first error, reports it, and leaves a clear (non-hanging) state.

## Operations And Recovery

- Routine operation: run `plan` to preview, `migrate` to apply; see `docs/ci-usage.md`.
- Recovery or rollback: because apply fails fast and each object is individually transactional, re-running after fixing the failing script is safe; already-applied objects are skipped by checksum.

## Open Issues And Non-Goals

- Open issues: the apply phase is not a single all-or-nothing transaction across all objects; recovery relies on fail-fast plus per-object transactions and idempotent re-run. The live concurrency, lock-ordering, and `CREATE SCHEMA` effects are verified only by `make check-e2e` (Docker + SQL Server), not by offline checks.
- Non-goals: this document does not define the repository contract or CI wiring.

## References

- Canonical source paths: `crates/core/src/engine/run/database.rs`, `crates/core/src/engine/run/plan_phase.rs`, `crates/core/src/engine/apply_run.rs`, `crates/core/src/apply/mod.rs`, `crates/core/src/apply/objects_exec.rs`, `sql/apply/begin_transaction.sql`.
- Related contracts and scripts: `docs/repository-contract.md`, `docs/ci-usage.md`, `docs/prod-gate.md`.
- Related runbooks or ADRs: `docs/runbook.md`, `docs/solution.md`.
