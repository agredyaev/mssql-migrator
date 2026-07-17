# CI Usage

Lifecycle: `Current`.

## Purpose

Tell a CI owner how to run `rmig` reliably: which commands to invoke, which
environment variables to set, what exit codes mean, and how the tool avoids
hanging or leaving an unclear state. It answers: "How do I wire this into a
pipeline and interpret the result?"

## Scope

- CLI entry and exit mapping: `crates/cli/src/main.rs`, `crates/core/src/error.rs`.
- Configuration: `crates/core/src/config/env_build.rs`, `crates/core/src/config/validate.rs`.
- Pipeline definition: `.github/workflows/ci.yml`, `.github/workflows/lint.yml`, `.github/workflows/test.yml`, and `Makefile`.
- Out of scope: migration semantics (see `docs/migration-flow.md`) and secret rules (see `docs/security-review.md`).

## System Context

CI runs `rmig` against a SQL Server. The GitHub pipeline (`.github/workflows/ci.yml`)
runs lint and unit tests on every push/PR, then build; the integration job starts
SQL Server via `docker compose` and runs `make check-e2e`. Locally and in CI the
same `Makefile` targets apply.

## Interfaces And Boundaries

- Inputs: environment variables (below) and the repository tree under `RM_SQL_ROOT`.
- Outputs: a process exit code, structured logs, and (with `--json`) a plan document.
- CLI form: `rmig [--env <path>] [--json] <command>` where `<command>` is one of `plan`, `migrate`, `validate`, `baseline`, `repair-checksum`, `version`.
- Ownership boundaries: exit-code mapping is owned by `crates/core/src/error.rs`; config parsing by `crates/core/src/config/env_build.rs`.

## Assumptions And Constraints

- Required variables: `RM_DB_SERVER`, `RM_SQL_ROOT`, and for SQL authentication `RM_DB_USER` and `RM_DB_PASSWORD` (validated in `crates/core/src/config/validate.rs`).
- Optional variables: `RM_DB_PORT`, `RM_DB_AUTH`, `RM_DB_ENCRYPT`, `RM_DB_TRUST_SERVER_CERTIFICATE`, `RM_SQL_BASE`, `RM_SKIP_GIT`, `RM_LOG_LEVEL`.
- Timeouts: `RM_COMMAND_TIMEOUT` (per query/connect; default 30s) and `RM_LOCK_TIMEOUT` (advisory lock). A value of zero disables the command timeout.
- Daemon: `RMIG_SESSION` (client socket path), `RMIG_SESSION_TOKEN` (shared auth token), `RMIG_USE_RMIGD` (enable the `rmigd` path in the ops scripts).
- Reproducibility: set `RMIG_PLANNED_AT` (RFC3339) or `SOURCE_DATE_EPOCH` (Unix seconds) to pin `plannedAt` in plan JSON.
- Constraint: secrets must never be echoed in CI logs; see `docs/security-review.md`.

## Nominal Flow

1. Export the required variables (and credentials via a secret store, not plaintext).
2. Run lint and unit checks: `make check` (or `cargo test -p migrator-core --lib` for unit only).
3. For database behavior, start SQL Server and run `make check-e2e`.
4. Run `rmig validate` or `rmig plan` to preview, then `rmig migrate` to apply.
5. Read the exit code: `0` means success.

## Off-Nominal Behavior And Failure Containment

- Exit codes (`crates/core/src/error.rs`): `0` ok; `1` general/IO; `2` config (missing/invalid env); `3` connect failure or connect timeout; `5` SQL error or query timeout; `7` advisory-lock timeout; `8` invalid input (bad repository structure/identifier); `10` plan blocked; `130` interrupted (SIGINT/SIGTERM — the run future is dropped, the connection closes, and the server rolls back any open transaction).
- Failure mode: SQL Server unreachable or hung.
  Containment: connect and per-command timeouts fail with codes 3/5 instead of hanging the job.
- Failure mode: invalid repository structure (duplicate object/ordinal, bad path).
  Containment: scan fails closed with exit 8 before any deployment.
- Failure mode: a deploy step fails.
  Containment: apply stops at the first failure, rolls back its transaction, and exits 5; re-running after a fix is safe.

## Verification And Validation

- Contracts and checks: `make check` (rustfmt, clippy `-D warnings`, unit/integration tests, rustdoc, architecture guards), `make doc-check` (documentation gates), `make check-e2e` (SQL regression, E2E matrix, SLO, prod gate).
- Evidence artifacts: CI job logs and the `--json` plan output.
- Exit criteria: lint/test/doc/arch are green offline; the E2E matrix is green against a live SQL Server.

## Operations And Recovery

- Routine operation: gate merges on `make check` plus `make doc-check`; run `make check-e2e` on a runner with `docker compose`.
- Recovery or rollback: on a failed `migrate`, inspect the reported error, fix the offending script, and re-run; idempotent re-run skips already-applied objects. See `docs/runbook.md`.

## Open Issues And Non-Goals

- Open issues: `make check-e2e` requires Docker and SQL Server and cannot run in a sandbox without them.
- Non-goals: this document does not define the repository contract or internal SQL generation.

## References

- Canonical source paths: `crates/cli/src/main.rs`, `crates/core/src/error.rs`, `crates/core/src/config/env_build.rs`.
- Related contracts and scripts: `Makefile`, `ops/perf/sql_regression.sh`, `ops/perf/prod_gate.sh`, `docs/operational-contract.md`.
- Related runbooks or ADRs: `docs/runbook.md`, `docs/prod-gate.md`.
