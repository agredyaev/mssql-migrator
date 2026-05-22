# Technical Document: Runbook

Lifecycle: `Current`.

## Purpose

Operator steps after a failed **`rmig`** run: what to read, safe re-run, and common exit codes.

## Scope

This runbook applies to the following repository paths and components:
- Configuration validation: dotenv inputs, environment variables (`RM_DB_SERVER`, `RM_DB_DATABASE`, `RM_SQL_ROOT`).
- Run failure triage: stderr logs, structured JSON log outputs, and report directories.
- Failure recovery procedures: exit code mapping and lock containment.

## System Context

`rmig` operates statelessly to match declared database schema directories with SQL Server catalog structures. When an execution fails due to locks, blocking DDL shifts, or schema validation errors, operators require a deterministic path to isolate the failure and safely recover the target catalog state.

## Interfaces and Boundaries

- **Inputs**:
  - Console stderr logs and JSON structured output from `rmig`.
  - JSON execution reports (`.plan.json` and `.report.json`) written to the directory specified by `RM_REPORT_DIR`.
  - Environment variables: `RM_DB_SERVER`, `RM_DB_DATABASE`, and `RM_SQL_ROOT`.
- **Outputs**:
  - Restored migration progress.
  - Active distributed locks released.
- **Ownership boundaries**:
  - `crates/core/src/error.rs` defines the standard exit codes returned to the shell caller.

## Assumptions and Constraints

- **Assumptions**:
  - The database instance is running and accessible using the provided environment credentials.
  - Operator has standard access to execute CLI commands in the target environment shell.
- **Constraints**:
  - Do not manually alter system metadata tables in SQL Server to bypass standard schema validation.
  - Dynamic SQL layouts must match the structural standards declared in `docs/data-oriented-layout-policy.md`.

## Nominal Flow

1. Capture and analyze the exit code, console stdout, and stderr output from the failed `rmig` process.
2. Verify that environment variables (`RM_DB_SERVER`, `RM_DB_DATABASE`, `RM_SQL_ROOT`) point to the correct SQL Server host and source tree.
3. If `RM_REPORT_DIR` was set, inspect the output files `.plan.json` and `.report.json` to identify where the plan halted.

## Off-Nominal Behavior and Failure Containment

- **Exit Code 10: Plan Blocked (DDL Shift)**
  - *Cause*: A DDL layout shift has occurred without a matching migration file.
  - *Containment*: Inspect the auto-generated `_migrations/` scaffold files under the affected table directory. Create a valid migration SQL script or revert the layout change before retrying.
- **Exit Code 7: Session Lock Held**
  - *Cause*: Another process is currently executing migrations or failed to clean up its distributed lock.
  - *Containment*: Let the lock release naturally or follow the recovery steps to release the lock manually.
- **Prod Gate Failure**
  - *Cause*: Schema changes fail standard verification rules.
  - *Containment*: Execute the validation script locally:
    ```bash
    make prod-gate
    ```
    using the baseline under `crates/core/tests/testdata/prod_gate/`.

## Verification and Validation

Verify integration status and local environment correctness with:

```bash
make db-up
make e2e-all
```

## Operations and Recovery

- **Blocked Migration Recovery**: Inspect the scaffolded migration scripts in the target directory, then write and commit a valid migration SQL file or revert the un-migrated schema changes.
- **Distributed Lock Release**: Inspect active locks in SQL Server and release the session mutex if the process holding it has terminated abnormally.
- **Common Exit Codes**:
  - `0`: Success.
  - `7`: Session lock held by another process.
  - `10`: Plan blocked (requires transition migration).

## Open Issues and Non-Goals

- **Open issues**: Automated lock cleanup when a process is forcefully terminated via `SIGKILL`.
- **Non-Goals**: This document does not cover network layer issues, credential management, or manual data backfills.

## References

- Operational specification: [operational-contract.md](operational-contract.md)
- Product design overview: [solution.md](solution.md)
- Core module engine details: [specs/rust/module-engine.md](specs/rust/module-engine.md)
