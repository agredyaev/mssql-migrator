# rmig

Lifecycle: `Current`.

## Purpose

`rmig` is a Go CLI that plans and applies **MSSQL** schema migrations from a **repo-driven SQL layout**. This `README.md` is the repository landing page; durable contracts live under `docs/`.

## Scope

- CLI entry: `cmd/rmig/main.go` → `internal/app.Run`
- Engine: `internal/engine/engine.go` (orchestrates plan / migrate / validate / baseline / repair-checksum)
- Configuration: `internal/app/config.go`, `internal/types/config.go` (`RM_*` environment variables)
- Driver: `internal/driver`, `internal/driver/mssql`
- **Internal module specifications:** `docs/specs/internals/README.md`
- Product docs: `docs/solution.md`, `docs/operational-contract.md`, `docs/runbook.md`

## System context

Build with Go 1.22+ (module `reporting-db-migrations`). Configure database and repo paths through environment variables (typically via a dotenv file; see `--env` in `internal/app/flags.go`). Commands: `plan`, `migrate`, `validate`, `baseline`, `repair-checksum`.

## Interfaces and boundaries

- Inputs: required **`RM_DB_SERVER`**, **`RM_DB_DATABASE`**, **`RM_SQL_ROOT`**, **`RM_SQL_BASE`**; optional `RM_REPORT_DIR`, `RM_PLAN_FILE`, `RM_REPAIR_SCRIPT`, git metadata `RM_GIT_*`, and other keys parsed in `internal/app/config.go`
- Outputs: logs to stderr, optional reports when `RM_REPORT_DIR` is set, SQL Server metadata updates owned by the tool
- Boundaries: SQL scripts live in Git; connectivity and server-side behavior belong to the operator’s environment

## Assumptions and constraints

- Assumptions: Go 1.22+ toolchain for `make check`; Docker only when running `make test-int` / `docker compose` MSSQL.
- Constraints: supported database engine is Microsoft SQL Server via `internal/driver/mssql`; `staticcheck` must be installed locally for `make check` (`Makefile`).

## Nominal flow

1. `make check` — `go build ./...`, `go test ./...`, `go vet`, `staticcheck`, `gofmt` check (see `Makefile`).
2. Local binary: `go build -o rmig ./cmd/rmig`, or optimized release build: `make release-build` → `bin/rmig` (`CGO_ENABLED=0`, `-trimpath`, `-buildvcs=false`, `-ldflags="-s -w"` per `Makefile`; background in the guide’s [compiler optimizations](https://psavelis.github.io/golang-performance-optimization/optimization/compiler/) chapter, especially [build optimization](https://psavelis.github.io/golang-performance-optimization/optimization/compiler/build-optimization.html)).
3. Run `rmig --env /path/to/.env plan` (or another command); see usage text in `internal/app/flags.go`.

## Off-nominal behavior and failure containment

- Failure mode: `make check` fails (tests, vet, `staticcheck`, or `gofmt` drift).
  Containment: fix before merging; do not treat the tree as green.
- Failure mode: `rmig` exits non-zero after connect (planning, apply, or metadata error).
  Containment: use stderr and optional `RM_REPORT_DIR` artifacts; follow `docs/runbook.md`.

## Verification and validation

- Primary gate: `make check`
- SQL Server integration: `make test-int` (requires Docker MSSQL per `Makefile` / `docker compose`)
- Documentation gate: `make doc-check` (`ops/quality/scripts/` per `docs/specs/nasa-document-spec.md`)

## Operations and recovery

- Routine operation: run `make check` before every merge to `main`; run `make doc-check` when changing durable docs under `docs/` or `README.md`.
- Recovery: for production incidents, use `docs/runbook.md` with logs and `.plan.json` / `.report.json` when `RM_REPORT_DIR` is set.

## Open issues and non-goals

- Open issues: none tracked in this file.
- Non-goals: this README does not define SQL schema design or external CI provider configuration.

## References

- `docs/specs/internals/README.md` — NASA-style specs per `internal/` package
- `docs/solution.md` — product-level solution aligned with `internal/app` and `internal/engine`
- `docs/specs/documentation-spec.md`, `docs/specs/nasa-document-spec.md` — authoring rules
- `ops/quality/scripts/README.md` — documentation validation scripts

