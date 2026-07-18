# Rust SQL Server integration test harness

Lifecycle: `Current`.

## Purpose

Document how `crates/core/tests/` runs against Docker SQL Server: which helpers exist, how database names are resolved, and why test binaries only compile the modules they need (no `dead_code` under `-D warnings`).

## Scope

- Integration tests: `apply_e2e_integration.rs`, `workflow_integration.rs`, `scenario_e2e_integration.rs`, `integration_plan.rs`, `prod_gate_integration.rs`, bugslog regression crates (`multi_db_plan_test.rs`, `plan_deferred_bootstrap_test.rs`, `advisory_lock_guard_test.rs`, `advisory_lock_rmigd_test.rs`, and others).
- Shared helpers under `crates/core/tests/common/`.
- Orchestrators: `ops/perf/sql_regression.sh`, `ops/perf/integration.sh`, `ops/perf/workflow_fast.sh`, `ops/perf/e2e_all.sh`.

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
| `db_reset.rs` | apply, workflow, e2e | DROP/CREATE catalog DB from `cfg.database` |
| `engine_smoke.rs` | apply, workflow | `baseline_migrate`, `plan` (skip git) |
| `state_smoke.rs` | apply, workflow | smoke schema/table/view + audit row probes |
| `state_ddl.rs` | workflow only | column + scaffold file probes |
| `workflow_git.rs` | workflow only | `.temp` git commits + reset on drop |
| `workflow_engine.rs` | workflow only | git `migrate`, plan DB SLO asserts |
| `db_reset_workflow.rs` | workflow | fast truncate vs full DROP/CREATE |

**Non-obvious:** each integration test crate lists only the `#[path = "..."]` modules it needs. Compiling all of `workflow.rs` inside `apply_e2e` caused `dead_code` warnings; split files avoid that without `#![allow(dead_code)]`.

**Git cleanup:** `GitRestore::drop` runs `git reset --hard` and deletes `{database}/smoke/tables/_migrations/smoke_table/*` under `RM_SQL_ROOT` so the next test starts from committed SQL only.

## Assumptions and constraints

- `RMIG_RUN_SQLSERVER_INTEGRATION=1` gates SQL tests (otherwise `return` early).
- `RM_DB_*` connection vars required. Default catalog database name is discovered from the sole top-level directory under `RM_SQL_ROOT` (same as `discover_catalog_databases` in `config/catalog.rs`); `RM_DB_DATABASE` overrides that name for shell-side `DROP/CREATE` only (`ops/perf/e2e_env.sh`).
- `RM_SQL_BASE` defaults to `RM_SQL_ROOT` when empty.
- On Apple Silicon with Colima, Docker SQL Server images require Rosetta-backed amd64 emulation. Use `colima start --vz-rosetta --memory 4 --cpu 4` (or set `rosetta: true` in `~/.config/colima/default/colima.yaml`) while keeping the VM on `arch: aarch64`. With `rosetta: false`, `mcr.microsoft.com/mssql/server:*` can fail during startup with `Invalid mapping of address ... below 0x400000000000`.
- Profiler crates (`pprof`, `criterion`, `dhat`) are **dev-dependencies only**; `scripts/check-rust-release-deps.sh` asserts they are absent from `cargo tree -e normal` for `rmig` / `rmigd` / `migrator-core`.

## Nominal flow

1. On Apple Silicon + Colima, run `colima start --vz-rosetta --memory 4 --cpu 4` before starting Docker SQL Server.
2. `make db-up` - Docker SQL Server only (databases created on first `migrate`).
3. `make check` - arch guard, release dep check, `clippy -D warnings`, unit + non-SQL integration tests (SQL suites skip without `RMIG_RUN_SQLSERVER_INTEGRATION=1`).
4. `make sql-regression` - bugslog SQL regression battery via `ops/perf/sql_regression.sh` (includes `apply_integrity_integration` and `rmigd` lock tests).
5. `make check-e2e` - `sql-regression` + scenario matrix + workflow + SLO + prod gate (ADO merge gate).
6. `make integration` - `apply_e2e_integration` + `adopt_e2e_integration` + `drift_e2e_integration` + `workflow_integration` on `.temp/sql` (subset; prefer `make check-e2e` before merge).

Critical bug regressions are mandatory:

- `BUG-013`: `scripts/check-sql-regression-manifest.sh` rejects existence-only release-binary builds; `chaos_kill_mid_apply_test` runs the rebuilt `rmig`.
- `BUG-023`: `apply_integrity_integration::transition_rollback_body_cannot_commit_history_regression` proves rollback cannot leave history.
- `BUG-033`: `apply_integrity_integration::migrate_blocks_implicit_adoption_without_flag_regression` uses a structurally different live table and proves fail-closed adoption.
- `BUG-073`: `daemon_endpoint_tests` checks server, port, login, and TLS-policy mismatches under `cargo test -p migrator-core --all-features --lib --tests`.
- `BUG-078`: `apply_script_read_tests::verified_body_rejects_changed_file_regression` rejects bytes changed after scan.
- `BUG-100`: `apply_integrity_integration::baseline_and_repair_never_execute_ddl_regression` proves module and pending-transition DDL remain unexecuted.

High-severity regressions added by the completeness audit are also mandatory:

- `BUG-026`: `advisory_lock_rmigd_test::preexisting_socket_override_is_never_removed_regression` proves the harness does not stop or remove an unowned path.
- `BUG-029`: `drift_e2e_integration::malformed_checksum_blocks_every_command_except_repair_regression` proves audit corruption fails closed against SQL Server.
- `BUG-040`, `BUG-046`, `BUG-095`: `ops/quality/scripts/tests/run.sh` pins branch-only manual dispatch, exact green SHA, atomic ref push, and partial-publication resume.
- `BUG-062`: the same script gate rejects broad `pkill -f` cleanup; `sql_regression.sh` only inspects its validated repository socket.
- `BUG-106`: `delta_scope_test::mutating_full_scope_inspects_every_object_regression` plus the script call-site guard prove mutating plans do not use fixed spot checks.
- `BUG-107`: `driver_mssql_tests::session_init_step_is_bounded_regression`, `ensure_db_tests::create_database_step_is_bounded_regression`, and the script call-site guard pin both raw-operation timeouts.

## Off-nominal behavior

- Missing `RM_SQL_ROOT` or empty catalog dirs → `validate_config` / `discover_catalog_databases` error before connect.
- Multiple top-level DB dirs → engine runs plan/migrate once per database (see `engine/run/mod.rs`).
- Apple Silicon + Colima without Rosetta → Docker MSSQL can crash before bind/listen with `Invalid mapping of address ... below 0x400000000000`; fix the Colima profile first, then rerun `make db-up`.

## Verification

```bash
colima start --vz-rosetta --memory 4 --cpu 4
make db-up
make check
make sql-regression
make check-e2e
scripts/check-sql-regression-manifest.sh
```

## Operations and recovery

- Routine: run `make check-e2e` before merging harness changes; use `GitRestore::drop` to reset `.temp/` git fixture.
- CI: `.github/workflows/test.yml` integration job runs `make check-e2e` against Docker MSSQL with `RMIG_USE_RMIGD=1`.
- Recovery: if Docker MSSQL fails immediately on Apple Silicon, confirm the active Colima profile still has Rosetta enabled before debugging `rmig` itself.
- Recovery: stuck scaffold files under `{database}/smoke/tables/_migrations/` → `workflow_git` cleanup on test drop or manual delete.

## Open issues and non-goals

- Non-goals: this harness does not cover scenario baseline JSON refresh (maintainer updates under `tests/testdata/e2e/`).

## References

- `docs/specs/rust/module-config-export.md`
- `ops/perf/README.md`
- `crates/core/src/config/catalog.rs`
