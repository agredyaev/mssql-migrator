# Operational Contract

Lifecycle: `Current`.

## Purpose

Define how **`rmig`** is built, configured, and operated. It acts as the primary agreement between repository code structures and target runtime environments.

## Scope

This contract governs the CLI entry points, execution commands, dynamic environment configurations, and produced planning report artifacts:

- **CLI entry**: `crates/cli/src/main.rs` → `migrator_core::engine::run_command`
- **Configuration loading**: Sourced from dotenv and process environment (`config::build_config`)

---

## System Context

`rmig` derives database connections dynamically and enforces catalog structural layouts. It operates statelessly by matching the declared filesystem SQL schema layouts directly with SQL Server's live system tables.

---

## Interfaces and Boundaries

### 1. Invocations and Commands
- **CLI Syntax**: `rmig [--env <path>] [--json] <command>` (invoking without arguments or with `--help` prints usage)
- **Supported Commands**: `plan`, `migrate`, `validate`, `baseline`, `repair-checksum`, `version`

### 2. Required Environment Variables

| Variable | Meaning | Path Reference |
| :--- | :--- | :--- |
| `RM_DB_SERVER` | Target SQL Server host address | Sourced in config |
| `RM_SQL_ROOT` | Absolute root directory of the SQL schema tree; **database name(s) are the top-level directories here** (e.g. `dactests/`) | Sourced in config |
| `RM_SQL_BASE` | Base directory for dynamic scaffold migrations | Defaults to `RM_SQL_ROOT` |
| `RM_DB_USER` / `RM_DB_PASSWORD` | SQL authentication credentials | Sourced in config (required for SQL auth) |

`RM_DB_DATABASE` is **not** read by `rmig` / `migrator_core::config::build_config`. Shell helpers under `ops/perf/` may set it for Docker `DROP/CREATE` orchestration only; see `ops/perf/e2e_env.sh`.

### 3. Optional Operational Variables

| Variable | Default | Operational Effect |
| :--- | :---: | :--- |
| `RM_SKIP_GIT` | `0` | Set to `1` to bypass git diff metadata, triggering full catalog inspections. |
| `RM_REPORT_DIR` | - | Target path to write JSON migration execution reports on completion. |
| `RMIG_SESSION` | - | Unix domain socket address to communicate with `rmigd` session manager. |
| `RMIG_CATALOG_CACHE` | `1` | Set to `0` to disable the local catalog memory cache. |
| `RMIG_GATE_SKIP_DB_RESET` | `0` | Set to `1` during integration to bypass active DROP/CREATE steps. |
| `RM_LOG_LEVEL` / `RUST_LOG` | `info` | Controls structured stderr logging from `crates/cli/src/main.rs` and `crates/rmigd/src/main.rs`. Simple levels such as `info` apply to `migrator_core`, `rmig`, and `rmigd` while dependencies stay at `warn`. `RUST_LOG` takes precedence when set. |
| `RM_LOCK_TIMEOUT` | `60s` | Advisory lock wait time. Values are converted to SQL Server `int` milliseconds and clamped at `2147483647`. |

### 4. Produced Artifacts
- **Run Reports**: When `RM_REPORT_DIR` is set, `rmig` writes a compact JSON plan (`.plan.json`) and run report (`.report.json`) upon successful execution.
- **Runtime Logs**: `rmig` and `rmigd` write structured key-value logs to stderr. `rmig --json <command>` keeps command output on stdout and still writes logs to stderr.

---

## Assumptions and Constraints

- **Single-Database Scopes**: Session daemon warm connections (`rmigd`) are strictly limited to single-database repositories. Multi-database setups force fresh TDS connections per database.
- **TDS Configuration**: Assumes SQL Server authentication is enabled when passing user/password credentials.

---

## Nominal Flow

Compile and verify the binary in the local workspace using:

```bash
make build           # target/release/rmig, rmigd (includes debug symbols)
make release-build   # bin/rmig (highly optimized: fat LTO, stripped)
make check           # checks codebase architecture, formatting, and unit tests
```

---

## Off-Nominal Behavior and Failure Containment

- **Blocked Migrations**: If a migration planning step is blocked (e.g., due to DDL shifts without valid migrations), `rmig` exits with code **10**, containment is achieved by writing a skeleton migration scaffold and aborting further executions.
- **Lock Contention**: If another deployment process holds the catalog lock, `rmig` fails closed, writes lock details to stderr, and exits with code **7**.
- **Cache Poisoning**: Process-local audit and catalog inspect caches recover poisoned mutexes, emit a warning log, and continue so later commands can re-check SQL Server state.

---

## Verification and Validation

Verify operational behavior by running:

```bash
make db-up           # Starts target MSSQL Docker containers
make check           # Fast gate: arch, fmt, clippy, unit + non-SQL tests
make sql-regression  # Bugslog SQL regression battery (ops/perf/sql_regression.sh)
make check-e2e       # ADO merge gate: sql-regression + e2e matrix + workflow + slo + prod-gate
```

Azure DevOps / production release: the CI **Integration & E2E (MSSQL)** job (`.github/workflows/test.yml`) must pass `make check-e2e`. Unit tests alone are not sufficient.

---

## Operations and Recovery

- **Blocked Migration Recovery**: Inspect `_migrations/` scaffold files under the affected table directory. Commit a valid migration SQL script or revert the layout change before retrying.
- **Distributed Lock Release**: Refer to [runbook.md](runbook.md) for unlocking the mutex when a deployment process terminates abnormally.

---

## Open Issues and Non-Goals

- **Non-Goals**: Env variable validation does not actively sanitize database passwords or check user authorization levels prior to connecting.

---

## References

- Runbook: [runbook.md](runbook.md)
- Product overview: [solution.md](solution.md)
- Core index: [specs/rust/README.md](specs/rust/README.md)
