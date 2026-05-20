# Rust module specifications (`migrator-core`)

Lifecycle: `Current`.

## Purpose

This directory holds NASA-style specifications (`docs/specs/nasa-document-spec.md`, `docs/specs/documentation-spec.md`) for the production Rust migrator under `rust/crates/core/src/`. Go packages under `internal/` remain as a reference implementation; **production operators use Rust `rmig`** (see `docs/rust-port-plan.md`).

## Scope

- Core library: `rust/crates/core/src/` (all top-level modules in `lib.rs`).
- CLI binary: `rust/crates/cli/` (`module-cli.md`).
- Optional session daemon: `rust/crates/rmigd/` (documented in `module-cache-session.md`).
- Embedded T-SQL: `rust/sql/` (referenced from `module-db.md` and `module-audit.md`).

## Module index

| Rust module / crate | Specification |
|---------------------|---------------|
| `rust/crates/cli`, `rust/crates/rmigd` | `module-cli.md` |
| `engine` | `module-engine.md` |
| `scan` | `module-scan.md` |
| `plan` | `module-plan.md` |
| `db` | `module-db.md` |
| `driver` | `module-driver.md` |
| `audit` | `module-audit.md` |
| `apply` | `module-apply.md` |
| `gate` | `module-gate.md` |
| `domain` | `module-domain.md` |
| `cache`, `session` | `module-cache-session.md` |
| `scaffold` | `module-scaffold.md` |
| `config`, `export`, `timings`, `error` | `module-config-export.md` |
| Integration test harness | `module-test-harness.md` |
| `git`, `lock`, `perf`, `sql` | `module-supporting.md` |

## System context

`rust/crates/cli/src/main.rs` loads `RM_*` configuration, then calls `migrator_core::engine::run_command` for `plan`, `migrate`, `validate`, `baseline`, and `repair-checksum`. The engine scans the SQL tree (`scan`), loads audit/catalog state (`db`, `audit`), computes a plan (`plan`), optionally applies DDL (`apply`), and publishes phase timings (`timings`). Incremental promotion checks use `gate` (prod gate snapshot/compare).

## Interfaces and boundaries

- Inputs: maintainer questions about Rust subsystem ownership; links from `README.md`, `docs/solution.md`, `docs/rust-port-plan.md`.
- Outputs: per-module `module-*.md` files and this index.
- Ownership boundaries: these specs describe `rust/crates/core/src/` and CLI crates; they do not replace `docs/operational-contract.md`.

## Assumptions and constraints

- Assumptions: Microsoft SQL Server 2016+ with OPENJSON; co-located or low-latency SQL (see product SLO in `docs/rust-port-plan.md`).
- Constraints: when a module’s public surface or command flow changes, update the matching `module-*.md` in the same change.

## Nominal flow

1. Open this index to find the module file for a path under `rust/crates/core/src/`.
2. Read the linked `module-*.md` before changing that module.
3. Run `make rust-check` after code edits; run `make doc-check` when documentation changes.

## Off-nominal behavior and failure containment

- Failure mode: `module-*.md` drifts from code (wrong paths or behavior).
  Containment: `make doc-check` fails via `ops/quality/scripts/check_doc_sync.py`; fix docs before merge.

## Verification and validation

- `make rust-check` — arch, release dep guard, fmt, `clippy -D warnings`, unit + non-SQL tests (`Makefile`)
- `make rust-slo` — warm `cli_wall_ms` < 100 ms gate (`docs/rust-port-plan.md`)
- `make rust-plan-db-perf` — plan DB `parallel_wall_ms` ≤ 500 ms (`ops/perf/README.md`)
- `make rust-workflow-fast` — full workflow integration ~2 s (`ops/perf/rust_workflow_fast.sh`)
- `make rust-prod-gate` — incremental plan go/no-go (`docs/prod-gate.md`)
- `make doc-check` — documentation sync scripts

## Operations and recovery

- Routine operation: add a row to the **Module index** table and add `module-<name>.md` when introducing a new documented subsystem.
- Recovery: if the index and on-disk files disagree, `check_doc_sync.py` fails; align the table and filenames.

## Open issues and non-goals

- Open issues: parity gaps vs Go reference are tracked in `docs/rust-port-plan.md` (Remaining milestones).
- Non-goals: line-by-line Rust doc comments; Go `internal/` specs (see `docs/specs/internals/README.md`).

## References

- `docs/templates/document-template.md`
- `docs/specs/documentation-spec.md`
- `docs/rust-port-plan.md`
- `docs/prod-gate.md`
- `ops/perf/README.md`
- `rust/crates/core/src/lib.rs`
