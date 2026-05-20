# Internal module specifications

Lifecycle: `Current`.

## Purpose

This directory holds **NASA-style** specifications for Go packages under `internal/`. Go is a **reference implementation** for parity tests; production operators use Rust (`docs/specs/rust/README.md`).

## Scope

- All production packages under `internal/` except `cmd/` (CLI lives in `cmd/rmig`).
- Each `module-*.md` file maps to one or a small group of related packages.

## Module index

| Package(s) | Specification |
|------------|----------------|
| `internal/app` | `module-app.md` |
| `internal/engine` | `module-engine.md` |
| `internal/fs` | `module-fs.md` |
| `internal/diff` | `module-diff.md` |
| `internal/apply` | `module-apply.md` |
| `internal/db` | `module-db.md` |
| `internal/driver`, `internal/driver/mssql` | `module-driver.md` |
| `internal/audit` | `module-audit.md` |
| `internal/bus` | `module-bus.md` |
| `internal/types` | `module-types.md` |
| `internal/errors`, `internal/log`, `internal/report`, `internal/lock`, `internal/scaffold`, `internal/testutil` | `module-supporting.md` |

## System context

`cmd/rmig/main.go` calls `internal/app.Run`, which parses flags and environment, opens `driver.Conn`, wires `bus.EventBus`, subscribers, and `engine.Engine`. Commands delegate to `(*engine.Engine).Plan`, `Migrate`, `Validate`, `Baseline`, or `RepairChecksum`.

## Interfaces and boundaries

- Inputs: reader questions about ownership of a subsystem; links from `README.md` and `docs/solution.md`.
- Outputs: per-module `module-*.md` files and this index.
- Ownership boundaries: these specs describe `internal/` only; they do not redefine product contracts in `docs/operational-contract.md`.

## Assumptions and constraints

- Assumptions: the Go module layout under `internal/` remains the primary decomposition.
- Constraints: when a package’s public surface or command flow changes, update the matching `module-*.md` in the same change.

## Nominal flow

1. Open `docs/specs/internals/README.md` to find the module file for a package.
2. Read the linked `module-*.md` before changing that package.
3. Run `make check` after code edits; run `make doc-check` when documentation changes.

## Off-nominal behavior and failure containment

- Failure mode: `module-*.md` drifts from code (wrong paths or behavior).
  Containment: treat failing `make doc-check` like any other quality gate; fix docs or paths before merge.

## Verification and validation

- `make check` from the repository root (`Makefile`)
- `make doc-check` for documentation scripts under `ops/quality/scripts/`
- Optional SQL Server integration: `make test-int` (Docker MSSQL; see `Makefile` `test-int` target)

## Operations and recovery

- Routine operation: add a row to the **Module index** table and add `module-<name>.md` when introducing a new documented subsystem.
- Recovery: if the index and on-disk files disagree, `check_doc_sync.py` fails; align the table and filenames.

## Open issues and non-goals

- Open issues: none tracked in this index file.
- Non-goals: this index does not duplicate line-by-line API documentation from Go doc comments.

## References

- `docs/templates/document-template.md`
- `docs/specs/documentation-spec.md`
- `docs/specs/nasa-document-spec.md`
- `README.md`
- `ops/quality/scripts/README.md`
