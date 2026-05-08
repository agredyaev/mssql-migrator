# Technical Document: Operational Contract

Lifecycle: `Current`.

## Purpose

This document defines how `rmig` is built, validated, promoted, and operated in the PR-to-production flow.

## Scope

- Local build commands in `README.md`
- CI example in `docs/ci-example.yml`
- Secret redaction in `internal/logger/*.go`
- Production metadata injection in `cmd/rmig/main.go`
- Failure containment in `internal/migrator/*.go`

## System Context

The intended workflow is: develop on a branch, open a PR to `main`, pass compile and test checks, then run a production job that invokes `rmig` against SQL Server.
The production job must pass a real commit SHA into the binary and must not leak secrets in logs or reports.

## Interfaces And Boundaries

- Inputs: `RM_*` environment variables, SQL Server connection details, `--plan-file`, `--confirm`, build metadata
- Outputs: logs, JSON/text reports, SQL Server metadata rows, exit codes
- Ownership boundaries: the repository owns `rmig`; the release pipeline owns promotion and deployment timing

## Assumptions And Constraints

- `go test ./...`, `go vet ./...`, and `go build` are required before promotion.
- The binary is built with `-ldflags` so `rmig version` reports a real commit SHA.
- `baseline` and `repair-checksum` require `--confirm`.
- SQL Server credentials and secrets are provided externally.

## Nominal Flow

1. Run local verification: `go test ./...`, `go vet ./...`, `go build ...`.
2. Build the release binary as `rmig`.
3. Run `rmig version` to confirm the version and commit.
4. Run `rmig plan`, then `rmig migrate`, then `rmig validate` in the target environment.

## Off-Nominal Behavior And Failure Containment

- Secret leakage: logger redaction must strip password and token-like values.
- Plan drift: `migrate` checks the approved plan and fails closed if inputs changed.
- Concurrent deploy: app lock blocks the second migration.
- Metadata failure: the run stops instead of reporting success.

## Verification And Validation

- `PATH=/usr/local/go/bin:$PATH go test ./...`
- `PATH=/usr/local/go/bin:$PATH go vet ./...`
- `PATH=/usr/local/go/bin:$PATH go build -o rmig ./cmd/rmig`
- `./rmig version`
- `docs/integration-test-plan.md`

## Operations And Recovery

- Use `docs/runbook.md` for migration failure, validation failure, and metadata repair.
- Re-run `rmig plan` after any metadata repair.

## Open Issues And Non-Goals

- Open issues: live production verification is still required before claiming full production proof.
- Non-goals: this document does not describe the external CI provider configuration.

## References

- `README.md`
- `docs/solution.md`
- `docs/runbook.md`
- `internal/logger/logger.go`
