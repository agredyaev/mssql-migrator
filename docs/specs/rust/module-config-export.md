# Technical Document: Modules `config`, `export`, `timings`, `error`

Lifecycle: `Current`.

## Purpose

Describe **configuration loading**, **catalog-derived database names**, **plan JSON wire export**, **phase timings struct**, and **error / exit codes**.

## Scope

- `rust/crates/core/src/config/env.rs`, `validate.rs`, `catalog.rs`, `mod.rs`
- `rust/crates/core/src/export/plan_json.rs`, `report.rs`, `checksum_json.rs`, `mod.rs`
- `rust/crates/core/src/timings.rs`
- `rust/crates/core/src/error.rs`

## System context

CLI loads env → `Config` → `validate_config` → `discover_catalog_databases` → `ensure_catalog_databases_exist` → engine.

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
| `RM_DB_USER` / `RM_DB_PASSWORD` | yes for SQL auth | |
| `RM_DB_DATABASE` | **no** | Derived from catalog; field on `Config` is runtime-only |
| `RM_SQL_BASE` | no | Defaults to `RM_SQL_ROOT` (scaffold/migrations path) |

### Multi-database repos

If `RM_SQL_ROOT` contains multiple child directories with schema subfolders (e.g. `dactests/`, `warehouse/`), `run_command` loops: ensure each DB exists on the server, scan once, filter workspace per DB, connect to that database, plan/apply.

`RMIG_SESSION` / `rmigd` is used only for single-database catalogs (multi-DB forces direct TDS per DB).

## Assumptions and constraints

- SQL Server 2016+ with OPENJSON.
- Catalog directory must contain at least one `<database>/<schema>/` subtree.

## Nominal flow

1. Load dotenv → `build_config` (does not read `RM_DB_DATABASE`).
2. `validate_config` → set `sql_base`, discover DB name when exactly one catalog DB.
3. Engine: ensure DBs → scan → per-DB plan/migrate.

## Off-nominal behavior

- Failure mode: no catalog databases under `RM_SQL_ROOT`.
  Containment: `Error::Config` before connect.
- Failure mode: multiple catalog DBs without operator narrowing `RM_SQL_ROOT`.
  Containment: engine processes each DB sequentially; reports last plan.

## Verification and validation

- `rust/crates/core/src/config/catalog.rs` unit test `discover_databases_from_layout`
- `make rust-check`, `make rust-e2e`
- `rust/crates/core/tests/plan_json_roundtrip_test.rs`, `exit_code_test.rs`

## Operations and recovery

- Routine: run `make rust-check` after changes to `config/`, `export/`, or `error.rs`.
- Recovery: invalid `RM_SQL_ROOT` layout → fix catalog tree; re-run `validate_config` via CLI.

## Open issues and non-goals

- Non-goals: `RM_PLAN_FILE` / `RM_REPAIR_SCRIPT` are not engine-enforced (same as Go reference).
- Open issues: `version` CLI output is a stub until release build metadata is wired.

## References

- `docs/specs/rust/module-test-harness.md`
- `docs/operational-contract.md`
- `scripts/check-rust-release-deps.sh`
