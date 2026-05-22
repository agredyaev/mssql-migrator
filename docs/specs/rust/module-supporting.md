# Technical Document: Supporting modules (`git`, `lock`, `sql`)

Lifecycle: `Current`.

## Purpose

Describe **auxiliary crates modules** that support scan, apply, embedded SQL, and maintainer benchmarks.

## Scope

| Module | Path | Role |
|--------|------|------|
| `git` | `crates/core/src/git/` | Git log/diff helpers for scan preload |
| `lock` | `crates/core/src/lock/mod.rs` | Migrate apply mutex via `azdo_deploy_meta` lock table |
| `sql` | `crates/core/src/sql/mod.rs` | `include_str!` embed of `sql/**/*.sql` |

Footprint/bench harness (not in `migrator-core`): [`crates/core-dev/`](../../../crates/core-dev/) - struct sizes, criterion/dhat benches.

Embedded SQL directories:

- `sql/audit/` - bootstrap, checksums, history
- `sql/catalog/` - scoped inspect, cache
- `sql/apply/` - transaction helpers
- `sql/lock/` - acquire/release

## Interfaces and boundaries

- Inputs: callers pass SQL fragments via `sql` module constants only at compile time.
- Outputs: embedded strings consumed by `audit`, `db`, `apply`, `lock`.
- Boundaries: no runtime read of `sql/` from disk in production binary.

## Assumptions and constraints

- Assumption: SQL files are UTF-8 and ship inside the binary via `include_str!`.
- Constraint: SQL edits require rebuild and integration re-run.

## Nominal flow

1. Developer edits `sql/**/*.sql`.
2. `cargo build` embeds content into `sql` module.
3. Integration tests validate behavior against Docker MSSQL.

## Off-nominal behavior and failure containment

- Failure mode: invalid T-SQL in embedded file.
  Containment: integration tests fail at SQL exec time.

## Operations and recovery

- Co-locate SQL changes with `module-db.md` / `module-audit.md` doc updates.

## System context

- `lock`: acquired in `apply_run` before `execute_plan`.
- `sql`: consumed by `audit`, `db`, `apply`, `lock`; no runtime filesystem read of SQL.

## Verification and validation

- `make bench-footprint` (struct sizes + diff bench via `migrator-core-dev`)
- SQL changes require integration tests (`make plan-db-perf`)

## Open issues and non-goals

- Non-goals: `perf` module is not a production runtime dependency for operators.

## References

- `docs/specs/rust/module-db.md`
- `docs/specs/rust/module-apply.md`
- `docs/perf-footprint-audit.md`
