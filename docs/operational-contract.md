# Technical Document: Operational Contract

Lifecycle: `Current`.

## Purpose

This document defines how `rmig` is built, validated, promoted, and operated in the PR-to-production flow.

## Scope

- Local build commands in `README.md`
- CI example in `docs/ci-example.yml`
- Secret redaction in `internal/logger/*.go`
- Runtime metadata injection in `internal/app/app.go`
- Failure containment in `internal/migrator/*.go`

## System Context

The intended workflow is: develop on a branch, open a PR to `main`, pass compile and test checks, then run a production job that invokes `rmig` against SQL Server.
The production job must pass a real commit SHA into the binary and must not leak secrets in logs or reports.
See `README.md` for the CLI wrapper contract.

## Interfaces And Boundaries

- Inputs: `RM_*` environment variables, SQL Server connection details, `--plan-file`, `--confirm`, `--skip-validate`, build metadata
- Inputs: `RM_*` environment variables, optional env files selected through `--env-file` or `RM_ENV_FILE`, SQL Server connection details, `--plan-file`, `--confirm`, `--skip-validate`, build metadata
- Outputs: logs, `reports/migration-plan.*`, `reports/migration-report.*`, `reports/validation-report.*`, SQL Server metadata rows, exit codes
- Ownership boundaries: the repository owns `rmig`; the release pipeline owns promotion and deployment timing

## Assumptions And Constraints

- `go test ./...`, `go vet ./...`, and a release build of the CLI wrapper are required before promotion.
- The binary is built with `-ldflags` so `rmig version` reports a real commit SHA.
- `baseline` and `repair-checksum` require `--confirm`.
- `plan`, `migrate`, `baseline`, and `repair-checksum` require `RM_GIT_COMMIT`.
- `--env` and `RM_ENV` accept only `pred` and `prod`.
- `migrate` requires `--plan-file` and runs validation by default; `--skip-validate` or `RM_SKIP_VALIDATE` disables the step.
- SQL Server authentication settings are provided externally. `RM_DB_AUTH=sql` uses `RM_DB_USER` and `RM_DB_PASSWORD`. `RM_DB_AUTH=integrated` uses the current Windows session or an explicit Windows user value passed in `RM_DB_USER`.
- Optional dotenv loading must be explicitly enabled. When enabled, CLI flags still win over process environment, and process environment still wins over values loaded from the env file.

## Nominal Flow

1. Run local verification: `go test ./...`, `go vet ./...`, `go build -ldflags "-X main.version=0.1.0-dev -X main.commit=$(git rev-parse HEAD)" -o rmig ./cmd/rmig`.
2. Run `rmig version` to confirm the version and commit.
3. Run `rmig plan`, then `rmig migrate`, then `rmig validate` in the target environment.
4. Run `rmig baseline` or `rmig repair-checksum` only when the runbook allows metadata repair.

## Off-Nominal Behavior And Failure Containment

- Secret leakage: redaction must strip password and token-like values from logs, reports, and stored error text.
- Plan drift: `migrate` checks the approved plan and fails closed if inputs or the approved script set changed.
- Concurrent deploy: app lock blocks the second migration.
- Metadata failure: the run stops instead of reporting success.
- Validation failure: the run stops and writes `reports/validation-report.*`.

## Verification And Validation

- See `README.md` for shared build and unit-test commands.
- `docs/integration-test-plan.md`

## Operations And Recovery

- Use `docs/runbook.md` for migration failure, validation failure, and metadata repair.
- Re-run `rmig plan` after any metadata repair.
- Retain `reports/migration-plan.*`, `reports/migration-report.*`, and `reports/validation-report.*` with the release record for signoff.

## Open Issues And Non-Goals

- Open issues: live production verification is still required before claiming full production proof.
- Non-goals: this document does not describe the external CI provider configuration.

## References

- `README.md`
- `docs/solution.md`
- `docs/runbook.md`
- `internal/logger/logger.go`
