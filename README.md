# rmig

Lifecycle: `Current`.

## Purpose

`rmig` is the Go CLI in `cmd/rmig` for MSSQL reporting-layer migrations, validation, and metadata capture.

## Scope

- CLI entrypoint: `cmd/rmig/main.go`
- Runtime dispatch: `internal/app/app.go`
- Migration engine: `internal/migrator/*.go`
- Metadata store: `internal/metadata/*.go`
- Reports: `internal/reports/*.go`
- Validation: `internal/validate/*.go`
- Canonical solution: `docs/solution.md`
- Canonical operational contract: `docs/operational-contract.md`
- Canonical runbook: `docs/runbook.md`
- Canonical integration checks: `docs/integration-test-plan.md`

## System Context

The expected flow is branch work on `main`, a PR to `main`, then a production pipeline that runs `rmig` against SQL Server.
The tool reads SQL files from `./sql`, writes reports to `./reports`, and records execution history in `__migrator.schema_migrations`.

## Interfaces And Boundaries

- Inputs: `RM_*` environment variables, `--env`, `--plan-file`, `--up-to`, `--script`, `--confirm`, SQL files in `./sql`
- Outputs: JSON and text reports in `./reports`, metadata rows in `__migrator.schema_migrations`
- Ownership boundaries: SQL Server access is external; `rmig` owns planning, migration, validation, and metadata writes

## Assumptions And Constraints

- SQL Server is reachable with the configured credentials.
- `baseline` and `repair-checksum` require `--confirm`.
- Versioned scripts are applied once.
- Repeatable scripts rerun only when their checksum changes.
- Logs must not expose secrets.

## Nominal Flow

1. Build the binary with `go build -ldflags "-X main.version=0.1.0-dev -X main.commit=$(git rev-parse HEAD)" -o rmig ./cmd/rmig`.
2. Run `rmig plan --env prod` to write the approved plan artifact.
3. Run `rmig migrate --env prod --plan-file reports/migration-plan.json` to apply SQL and write migration reports.
4. Run `rmig validate --env prod` to refresh modules and execute check scripts.

## Off-Nominal Behavior And Failure Containment

- Checksum mismatch: `plan` returns a blocked result and `migrate` fails closed.
- Metadata write failure: `migrate` reports `critical_state` and stops.
- SQL execution failure: the current script fails, the attempt is recorded, and the run exits with a SQL error code.
- Concurrent migration: app lock blocks the second run.

## Verification And Validation

- `PATH=/usr/local/go/bin:$PATH go test ./...`
- `PATH=/usr/local/go/bin:$PATH go vet ./...`
- `PATH=/usr/local/go/bin:$PATH go build -o rmig ./cmd/rmig`
- `./rmig version`
- `docs/solution.md`
- `docs/operational-contract.md`
- `docs/integration-test-plan.md`

## Operations And Recovery

- Normal operation: run `plan`, then `migrate`, then `validate`.
- Recovery: follow `docs/runbook.md` after a failed migration or validation run.
- Historical baseline: use `rmig baseline --env prod --up-to <VERSION> --confirm` once per existing database.

## Open Issues And Non-Goals

- Open issues: live MSSQL integration validation still needs execution in the target environment.
- Non-goals: `rmig` does not provision SQL Server, manage secrets, or orchestrate the outer CI/CD pipeline.

## References

- `docs/implementation-plan.md`
- `docs/deployment-readiness.md`
- `docs/runbook.md`
- `docs/integration-test-plan.md`
