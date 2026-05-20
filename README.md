# rmig

Lifecycle: `Current`.

## Purpose

`rmig` plans and applies **MSSQL** schema migrations from a **repo-driven SQL layout**. **Production operators use the Rust CLI** (`rust/crates/cli/`). The Go implementation under `internal/` remains as a reference and parity baseline. Durable contracts live under `docs/`.

## Scope

- **Production CLI:** `rust/crates/cli/src/main.rs` → `migrator_core::engine::run_command`
- **Production core:** `rust/crates/core/src/` — see `docs/specs/rust/README.md`
- Reference Go CLI: `cmd/rmig/main.go` → `internal/app.Run` — see `docs/specs/internals/README.md`
- Configuration: `RM_*` environment variables (`rust/crates/core/src/config/`, `internal/app/config.go`)
- Driver: `rust/crates/core/src/driver/`, `internal/driver/mssql`
- Product docs: `docs/solution.md`, `docs/operational-contract.md`, `docs/runbook.md`, `docs/rust-port-plan.md`

## System context

Build Rust with stable toolchain (`rust/` workspace). Configure database and repo paths through `RM_*` environment variables (typically via a dotenv file). Commands: `plan`, `migrate`, `validate`, `baseline`, `repair-checksum`, `version`.

Go 1.22+ remains available for reference tests and parity harness (`make check`, `make go-rust-e2e`).

## Interfaces and boundaries

- Inputs: required **`RM_DB_SERVER`**, **`RM_DB_DATABASE`**, **`RM_SQL_ROOT`**, **`RM_SQL_BASE`**; optional `RM_REPORT_DIR`, `RMIG_SESSION`, `RMIG_CATALOG_CACHE`, git metadata `RM_GIT_*`
- Outputs: logs to stderr, optional reports when `RM_REPORT_DIR` is set, SQL Server metadata updates owned by the tool
- Boundaries: SQL scripts live in Git; connectivity and server-side behavior belong to the operator’s environment

## Assumptions and constraints

- Assumptions: Go 1.22+ toolchain for `make check`; Docker only when running `make test-int` / `docker compose` MSSQL.
- Constraints: supported database engine is Microsoft SQL Server via `internal/driver/mssql`; `staticcheck` must be installed locally for `make check` (`Makefile`).

## Nominal flow

1. `make rust-check` — Rust format, clippy, unit tests (see `Makefile`).
2. Build: `cd rust && cargo build --release -p migrator-cli` or `make release-build`.
3. Run `rmig --env /path/to/.env plan` (Rust binary from `rust/target/release/rmig`).
4. Integration: `make rust-slo`, `make rust-prod-gate`, `make rust-workflow-fast` (see `ops/perf/README.md`).
5. Reference Go checks: `make check`, `make test-prod-gate` (parity baseline).

## Off-nominal behavior and failure containment

- Failure mode: `make rust-check` or integration gate fails.
  Containment: fix before merging; do not treat the tree as green.
- Failure mode: `rmig` exits non-zero after connect (planning, apply, or metadata error).
  Containment: use stderr and optional `RM_REPORT_DIR` artifacts; follow `docs/runbook.md`.

## Verification and validation

- Primary Rust gate: `make rust-check`
- SQL Server integration: `make db-up` then `make rust-slo`, `make rust-prod-gate`, `make rust-workflow-fast`
- Documentation gate: `make doc-check` (`ops/quality/scripts/` per `docs/specs/nasa-document-spec.md`)
- Reference Go gate: `make check`, `make test-prod-gate`

## Operations and recovery

- Routine operation: run `make rust-check` before every merge; run `make doc-check` when changing durable docs under `docs/` or `README.md`.
- Recovery: for production incidents, use `docs/runbook.md` with logs and `.plan.json` / `.report.json` when `RM_REPORT_DIR` is set.

## Open issues and non-goals

- Open issues: none tracked in this file.
- Non-goals: this README does not define SQL schema design or external CI provider configuration.

## References

- `docs/specs/rust/README.md` — NASA-style specs per Rust `migrator-core` module
- `docs/specs/internals/README.md` — Go reference package specs
- `docs/solution.md` — product-level solution
- `docs/rust-port-plan.md` — Rust SLO, milestones, Makefile targets
- `docs/specs/documentation-spec.md`, `docs/specs/nasa-document-spec.md` — authoring rules
- `ops/quality/scripts/README.md` — documentation validation scripts

