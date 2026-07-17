# Modules `config`, `export`, `timings`, `error`

Lifecycle: `Current`.

## Purpose

Describe **configuration loading**, **catalog-derived database names**, **plan JSON wire export**, **phase timings struct**, and **error / exit codes**.

## Scope

- `crates/core/src/config/env.rs`, `validate.rs`, `catalog.rs`, `mod.rs`
- `crates/core/src/export/plan_json/mod.rs`, `report.rs`, `checksum_json.rs`, `mod.rs`
- `crates/core/src/timings/mod.rs`
- `crates/core/src/error.rs`

## System context

CLI loads env → `Config` → `validate_config` → `discover_catalog_databases` → (`ensure_catalog_databases_exist` for mutate commands only) → engine.

SQL layout:

```text
RM_SQL_ROOT/<database>/<schema>/<kind>/<object>.sql
```

The first path segment under `RM_SQL_ROOT` is the **SQL Server database name** (e.g. `dactests`). It is not passed via `RM_DB_DATABASE`.

## Interfaces and boundaries

- `build_config`, `validate_config(&mut Config)`, `Config`
- `discover_catalog_databases`, `ensure_catalog_databases_exist`, `normalize_catalog_paths`
- `MigrationPlan` serde types in `export`
- `PhaseTimings` emitted to stderr / JSON when `--json`

### Required environment (Rust production)

| Variable | Required | Notes |
|----------|----------|-------|
| `RM_DB_SERVER` | yes | Host |
| `RM_SQL_ROOT` | yes | Root of catalog tree |
| `RM_DB_AUTH` | no | Only `sql` (the default) is accepted. `integrated` / `windows` are rejected with `Error::Config` at `driver/mssql_auth.rs`; target SQL Server 2019 has no workload/managed-identity support, so token auth is out of scope. |
| `RM_DB_USER` / `RM_DB_PASSWORD` | yes | Always required: `sql_credentials_required` returns true unconditionally (`config/auth_mode.rs`). |
| `RM_DB_DATABASE` | **no** | Derived from catalog; field on `Config` is runtime-only |
| `RM_SQL_BASE` | no | Defaults to `RM_SQL_ROOT` (scaffold/migrations path) |

### Multi-database repos

If `RM_SQL_ROOT` contains multiple child directories with schema subfolders (e.g. `dactests/`, `warehouse/`), `run_command` loops: ensure each DB exists on the server, scan once, filter workspace per DB, connect to that database, and merge per-DB plans into one final result.

`RMIG_SESSION` / `rmigd` is used only for single-database catalogs (multi-DB forces direct TDS per DB).

## Assumptions and constraints

- SQL Server 2016+ with OPENJSON.
- Catalog directory must contain at least one `<database>/<schema>/` subtree.

## Nominal flow

1. Load dotenv → `build_config` (does not read `RM_DB_DATABASE`).
2. `validate_config` → require `RM_DB_SERVER`, `RM_SQL_ROOT`, and non-empty `RM_DB_USER` / `RM_DB_PASSWORD` (always, since only SQL auth is supported); then set `sql_base` and discover DB name when exactly one catalog DB.
3. Engine: for each catalog DB, first probe direct connectivity to that target database; only fall back to `master` create-db preflight when the target connection fails. Then scan → per-DB plan/migrate.

## Off-nominal behavior

- Failure mode: no catalog databases under `RM_SQL_ROOT`.
  Containment: `Error::Config` before connect.
- Failure mode: multiple catalog DBs without operator narrowing `RM_SQL_ROOT`.
  Containment: engine processes each DB sequentially and merges the per-DB plans.

## Verification and validation

- `crates/core/src/config/catalog.rs` unit test `discover_databases_from_layout`
- `crates/core/src/config/validate.rs` unit tests for SQL credential preflight
- `make check`, `make integration`
- `crates/core/tests/plan_json_roundtrip_test.rs`, `exit_code_test.rs`, `db_auth_test.rs`

## Operations and recovery

- Routine: run `make check` after changes to `config/`, `export/`, or `error.rs`.
- Recovery: invalid `RM_SQL_ROOT` layout → fix catalog tree; re-run `validate_config` via CLI.

## Open issues and non-goals

- Non-goals: `RM_PLAN_FILE` / `RM_REPAIR_SCRIPT` are not engine-enforced.
- Open issues: none for `version`; release metadata is wired via `crates/core/build.rs` (`VERSION` + `git rev-parse --short HEAD`).

## References

- `docs/specs/rust/module-test-harness.md`
- `docs/operational-contract.md`
- `scripts/check-rust-release-deps.sh`
