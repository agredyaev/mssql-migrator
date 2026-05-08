# Technical Document: Solution

Lifecycle: `Current`.

## Purpose

This document describes the chosen `rmig` solution for MSSQL reporting-layer migrations.
It exists so a maintainer can understand the design choice without reading chat history or a deleted planning note.

## Scope

- CLI entrypoint: `cmd/rmig/main.go`
- Runtime dispatch: `internal/app/app.go`
- Migration engine: `internal/migrator/*.go`
- Metadata storage: `internal/metadata/*.go`
- Planning logic: `internal/planner/planner.go`
- Report writers: `internal/reports/write.go`
- Validation logic: `internal/validate/validate.go`

## System Context

The solution is a Go CLI named `rmig`.
It is run in a branch-to-PR-to-main workflow, then in an external production job against SQL Server.
The selected design keeps migration history in `__migrator.schema_migrations` and writes reports to `./reports`.

## Interfaces And Boundaries

- Inputs: SQL files in `./sql`, `RM_*` environment variables, command flags, approved plan files
- Outputs: migration reports, validation reports, metadata rows, exit codes, logs
- Ownership boundaries: SQL files are owned in Git; `rmig` owns execution, metadata writes, and report generation

## Assumptions And Constraints

- SQL Server is the execution target.
- Versioned scripts are one-time changes.
- Repeatable scripts are tied to Git and rerun only when their checksum changes.
- Logs must not expose secrets.
- Dangerous repair commands require `--confirm`.

## Nominal Flow

1. Build `rmig` with a real commit SHA.
2. Run `rmig plan --env prod` to produce the approved plan artifact.
3. Run `rmig migrate --env prod --plan-file reports/migration-plan.json` to apply SQL and store history.
4. Run `rmig validate --env prod` to refresh managed objects and execute checks.

## Off-Nominal Behavior And Failure Containment

- Checksum mismatch: the plan is blocked and migration stops.
- SQL execution failure: the failed attempt is recorded and the run exits with a SQL error code.
- Metadata failure after SQL success: the run fails closed and surfaces a critical state error.

## Verification And Validation

- `PATH=/usr/local/go/bin:$PATH go test ./...`
- `PATH=/usr/local/go/bin:$PATH go vet ./...`
- `PATH=/usr/local/go/bin:$PATH go build -o rmig ./cmd/rmig`
- `./rmig version`
- `docs/integration-test-plan.md`

## Operations And Recovery

- Use `docs/runbook.md` after a failed migration or validation run.
- Use `rmig baseline` only for an existing database that needs a starting point.
- Use `rmig repair-checksum` only for already applied scripts with bad stored checksums.

## Open Issues And Non-Goals

- Open issues: the live SQL Server integration suite is still the final proof step.
- Non-goals: this document does not define the outer CI/CD pipeline or SQL Server provisioning.

## References

- `README.md`
- `docs/operational-contract.md`
- `docs/runbook.md`
- `docs/integration-test-plan.md`
