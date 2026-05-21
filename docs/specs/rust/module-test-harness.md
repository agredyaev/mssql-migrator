# Technical Document: Rust SQL Server integration test harness

Lifecycle: `Current`.

## Purpose

Document how `rust/crates/core/tests/` runs against Docker SQL Server: which helpers exist, how database names are resolved, and why test binaries only compile the modules they need (no `dead_code` under `-D warnings`).

## Scope

- Integration tests: `apply_e2e_integration.rs`, `workflow_integration.rs`, `go_rust_scenario_integration.rs`, `integration_plan.rs`, `prod_gate_integration.rs`.
- Shared helpers under `rust/crates/core/tests/common/`.
- Orchestrators: `ops/perf/rust_e2e.sh`, `ops/perf/rust_workflow_fast.sh`.

## System context

Production `rmig` resolves SQL Server **database names from catalog directories** under `RM_SQL_ROOT` (see `config/catalog.rs`). Integration tests use the same rule via `validate_config` → `discover_catalog_databases`.

Layout contract:

```text
RM_SQL_ROOT/
  dactests/           ← database name on SQL Server
    smoke/            ← schema
      tables/*.sql
```

Tests point `RM_SQL_ROOT` at `$REPO/.temp/sql` (fixture in `.temp/`, not committed).

## Interfaces and boundaries

| Helper file | Used by | Role |
|-------------|---------|------|
| `integration_config.rs` | apply + workflow + `common/mod.rs` | `parity_config()`, `workflow_config()`, `enabled()` |
| `db_reset.rs` | apply, workflow, go_rust | DROP/CREATE catalog DB from `cfg.database` |
| `engine_smoke.rs` | apply, workflow | `baseline_migrate`, `plan` (skip git) |
| `state_smoke.rs` | apply, workflow | smoke schema/table/view + audit row probes |
| `state_ddl.rs` | workflow only | column + scaffold file probes |
| `workflow_git.rs` | workflow only | `.temp` git commits + reset on drop |
| `workflow_engine.rs` | workflow only | git `migrate`, plan DB SLO asserts |
| `db_reset_workflow.rs` | workflow | fast truncate vs full DROP/CREATE |

**Non-obvious:** each integration test crate lists only the `#[path = "..."]` modules it needs. Compiling all of `workflow.rs` inside `apply_e2e` caused `dead_code` warnings; split files avoid that without `#![allow(dead_code)]`.

**Git cleanup:** `GitRestore::drop` runs `git reset --hard` and deletes `dactests/smoke/tables/_migrations/smoke_table/*` so the next test starts from committed SQL only.

## Assumptions and constraints

- `RMIG_RUN_SQLSERVER_INTEGRATION=1` gates SQL tests (otherwise `return` early).
- `RM_DB_*` connection vars still required; **`RM_DB_DATABASE` is not used** — database name comes from catalog dir (`dactests`).
- `RM_SQL_BASE` defaults to `RM_SQL_ROOT` when empty.
- Profiler crates (`pprof`, `criterion`, `dhat`) are **dev-dependencies only**; `scripts/check-rust-release-deps.sh` asserts they are absent from `cargo tree -e normal` for `rmig` / `rmigd` / `migrator-core`.

## Nominal flow

1. `make db-up` — Docker SQL Server only (databases created on first `migrate`).
2. `make rust-check` — arch guard, release dep check, `clippy -D warnings`, unit + non-SQL integration tests.
3. `make rust-e2e` — `apply_e2e_integration` + `workflow_integration` on `.temp/sql`.

## Off-nominal behavior

- Missing `RM_SQL_ROOT` or empty catalog dirs → `validate_config` / `discover_catalog_databases` error before connect.
- Multiple top-level DB dirs → engine runs plan/migrate once per database (see `engine/run.rs`).

## Verification

```bash
make rust-check
RMIG_RUN_SQLSERVER_INTEGRATION=1 make rust-e2e
RUSTFLAGS="-D warnings" cargo test -p migrator-core --test apply_e2e_integration --test workflow_integration
```

## Operations and recovery

- Routine: run `make rust-e2e` before merging harness changes; use `GitRestore::drop` to reset `.temp/` git fixture.
- Recovery: stuck scaffold files under `dactests/smoke/tables/_migrations/` → `workflow_git` cleanup on test drop or manual delete.

## Open issues and non-goals

- Non-goals: this harness does not replace Go parity tests (`make go-rust-e2e-all`).
- Open issues: pending SQL integration tests for catalog cache save and migration re-run (see `docs/rust-port-plan.md`).

## References

- `docs/specs/rust/module-config-export.md`
- `ops/perf/README.md`
- `rust/crates/core/src/config/catalog.rs`
