# Modules `config`, `export`, `timings`, `error`

Lifecycle: `Current`.

## Purpose

Describe **configuration loading**, **catalog-derived database names**, **plan JSON wire export**, **phase timings struct**, and **error / exit codes**.

## Scope

- `crates/core/src/config/toml_config.rs`, `env_build.rs`, `validate.rs`, `catalog.rs`, `mod.rs`
- `crates/core/src/export/plan_json/mod.rs`, `report.rs`, `checksum_json.rs`, `mod.rs`
- `crates/core/src/timings/mod.rs`
- `crates/core/src/error.rs`

## System context

CLI loads typed TOML plus process env → `Config` → `validate_config` → `discover_catalog_databases` → (`ensure_catalog_databases_exist` for mutate commands only) → engine.

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

### TOML and process environment

`config.toml` accepts `[paths]` (`sql_root`, `sql_base`, `report_dir`) and
non-privileged `[execution]` flags and timeouts. The adoption opt-in is
excluded.

Database peer settings, TLS policy, SQL credentials, and the daemon socket/token
are process-environment only. TOML occurrences fail with `Error::Config` naming
the required variable. This binds credentials and tokens to a peer selected
outside the repository.

| Variable | Required | Notes |
|----------|----------|-------|
| `RM_DB_SERVER` | yes | SQL Server host; process environment only |
| `RM_DB_PORT` | no | SQL Server port; process environment only; default `1433` |
| `RM_SQL_ROOT` | no | Override for `paths.sql_root` |
| `RM_DB_USER` / `RM_DB_PASSWORD` | yes | Process environment only; required by SQL authentication. |
| `RMIG_SESSION_TOKEN` | for `rmigd` | Process environment only; required for daemon transport. |
| `RMIG_SESSION` | no | Daemon Unix socket; process environment only. |
| `RMIG_ALLOW_ADOPT` | no | Process-only operator opt-in for name-only adoption during `migrate`; default `false`. |
| `RM_DB_ENCRYPT` | no | Defaults to `true`. Set `false` only for an explicitly accepted local/test transport. |
| `RM_DB_TRUST_SERVER_CERTIFICATE` | no | Defaults to `false`; normal certificate validation remains enabled. |
| `RM_DB_DATABASE` | **no** | Derived from catalog; field on `Config` is runtime-only |
| `RM_SQL_BASE` | no | Defaults to `RM_SQL_ROOT` (scaffold/migrations path) |

### Multi-database repos

If `RM_SQL_ROOT` contains multiple child directories with schema subfolders (e.g. `dactests/`, `warehouse/`), `run_command` loops: ensure each DB exists on the server, scan once, filter workspace per DB, connect to that database, and merge per-DB plans into one final result.

`RMIG_SESSION` / `rmigd` is used only for single-database catalogs (multi-DB forces direct TDS per DB).

## Assumptions and constraints

- SQL Server 2016+ with OPENJSON.
- Catalog directory must contain at least one `<database>/<schema>/` subtree.
- Present boolean environment variables must use a recognized value (`true`/`false`, `1`/`0`, `yes`/`no`, `on`/`off`, `y`/`n`, or `enabled`/`disabled`). Typos fail with `Error::Config`; they never disable TLS implicitly.
- Implicit adoption is disabled unless the process operator sets
  `RMIG_ALLOW_ADOPT`; repository TOML cannot opt in.

## Nominal flow

1. Load typed TOML paths/execution settings → `build_config` reads peer settings and secrets from the process environment (does not read `RM_DB_DATABASE`).
2. `validate_config` → require `RM_DB_SERVER`, `RM_SQL_ROOT`, and non-empty
   `RM_DB_USER` / `RM_DB_PASSWORD`; then set `sql_base` and discover the
   database name when exactly one catalog database exists.
3. Engine: for each catalog DB, first probe direct connectivity to that target database; only fall back to `master` create-db preflight when the target connection fails. Then scan → per-DB plan/migrate.

## Off-nominal behavior

- Failure mode: no catalog databases under `RM_SQL_ROOT`.
  Containment: `Error::Config` before connect.
- Failure mode: multiple catalog DBs without operator narrowing `RM_SQL_ROOT`.
  Containment: engine processes each DB sequentially and merges the per-DB plans.
- Failure mode: `RM_DB_ENCRYPT=ture` or another invalid boolean.
  Containment: `validate_config` returns exit `2` before any network connection.
- Failure mode: TOML contains `[database]` or `[session]` peer settings.
  Containment: loading fails before config construction and names the required process variable.
- Failure mode: TOML contains `[execution].allow_adopt`.
  Containment: loading fails and names `RMIG_ALLOW_ADOPT`; the repository cannot
  bypass the migration adoption gate.
- Failure mode: TOML exceeds 1 MiB or is malformed.
  Containment: loading stops at `MAX_CONFIG_BYTES`; parse errors omit source lines and values.

## Verification and validation

- `crates/core/src/config/catalog.rs` unit test `discover_databases_from_layout`
- `crates/core/src/config/validate.rs` unit tests for SQL credential preflight; `config::env_parse::tests::invalid_boolean_is_rejected_regression`
- `crates/core/src/tests/toml_config_test.rs`,
  `crates/core/src/tests/env_build_test.rs`
- `make check`, `make check-e2e`
- `crates/core/tests/plan_json_roundtrip_test.rs`, `exit_code_test.rs`, `db_auth_test.rs`

## Operations and recovery

- Routine: run `make check` after changes to `config/`, `export/`, or `error.rs`.
- Recovery: invalid `RM_SQL_ROOT` layout → fix catalog tree; re-run `validate_config` via CLI.

## Open issues and non-goals

- Non-goals: `RM_PLAN_FILE` / `RM_REPAIR_SCRIPT` are not engine-enforced.
- Open issues: none for `version`; Cargo reads
  `[workspace.package].version` from `Cargo.toml`, and
  `crates/core/build.rs` stamps `git rev-parse --short HEAD`.

## References

- `docs/specs/rust/module-test-harness.md`
- `docs/operational-contract.md`
- `scripts/check-rust-release-deps.sh`
