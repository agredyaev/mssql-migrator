# Technical Document: Operational Contract

Lifecycle: `Current`.

## Purpose

Define how **`rmig`** is built, configured, and operated. It acts as the primary agreement between repository code structures and target runtime environments.

## Scope

This contract governs the CLI entry points, execution commands, dynamic environment configurations, and produced planning report artifacts:

- **CLI entry**: `crates/cli/src/main.rs` → `migrator_core::engine::run_command`
- **Configuration loading**: Sourced fromdotenv and process environment (`config::build_config`)

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
| `RM_DB_DATABASE` | Target catalog database name | Sourced in config |
| `RM_SQL_ROOT` | Absolute root directory of the SQL schema tree | Sourced in config |
| `RM_SQL_BASE` | Base directory for dynamic scaffold migrations | Defaults to `RM_SQL_ROOT` |

### 3. Optional Operational Variables

| Variable | Default | Operational Effect |
| :--- | :---: | :--- |
| `RM_SKIP_GIT` | `0` | Set to `1` to bypass git diff metadata, triggering full catalog inspections. |
| `RM_REPORT_DIR` | - | Target path to write JSON migration execution reports on completion. |
| `RMIG_SESSION` | - | Unix domain socket address to communicate with `rmigd` session manager. |
| `RMIG_CATALOG_CACHE` | `1` | Set to `0` to disable the local catalog memory cache. |
| `RMIG_GATE_SKIP_DB_RESET` | `0` | Set to `1` during integration to bypass active DROP/CREATE steps. |

### 4. Produced Artifacts
- **Run Reports**: When `RM_REPORT_DIR` is set, `rmig` writes a compact JSON plan (`.plan.json`) and run report (`.report.json`) upon successful execution.

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

---

## Verification and Validation

Verify operational behavior by running:

```bash
make db-up      # Starts target MSSQL Docker containers
make e2e-all    # Validates full scenario execution matrices
make prod-gate  # Audits incremental migration plans
make slo        # Asserts execution duration SLO thresholds
```

---

## Operations and Recovery

- **Blocked Migration Recovery**: Inspect `_migrations/` scaffold files under the affected table directory. Commit a valid migration SQL script or revert the layout change before retrying.
- **Distributed Lock Release**: Refer to [runbook.md](file://~/.gemini/antigravity/worktrees/mssql-reporting-migrator/analyze-nasa-docs-compliance/docs/runbook.md) for unlocking the mutex when a deployment process terminates abnormally.

---

## Open Issues and Non-Goals

- **Non-Goals**: Env variable validation does not actively sanitize database passwords or check user authorization levels prior to connecting.

---

## References

- Comprehensive runbook: [runbook.md](file://~/.gemini/antigravity/worktrees/mssql-reporting-migrator/analyze-nasa-docs-compliance/docs/runbook.md)
- Product overview: [solution.md](file://~/.gemini/antigravity/worktrees/mssql-reporting-migrator/analyze-nasa-docs-compliance/docs/solution.md)
- Core index: [specs/README.md](file://~/.gemini/antigravity/worktrees/mssql-reporting-migrator/analyze-nasa-docs-compliance/docs/specs/rust/README.md)
