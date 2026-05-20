# Technical Document: Supporting modules (`git`, `lock`, `perf`, `sql`)

Lifecycle: `Current`.

## Purpose

Describe **auxiliary crates modules** that support scan, apply, embedded SQL, and maintainer benchmarks.

## Scope

| Module | Path | Role |
|--------|------|------|
| `git` | `rust/crates/core/src/git/` | Git log/diff helpers for scan preload |
| `lock` | `rust/crates/core/src/lock/mod.rs` | Migrate apply mutex via `azdo_deploy_meta` lock table |
| `perf` | `rust/crates/core/src/perf/` | Struct size reports, bench baselines |
| `sql` | `rust/crates/core/src/sql/mod.rs` | `include_str!` embed of `rust/sql/**/*.sql` |

Embedded SQL directories:

- `rust/sql/audit/` — bootstrap, checksums, history
- `rust/sql/catalog/` — scoped inspect, cache
- `rust/sql/apply/` — transaction helpers
- `rust/sql/lock/` — acquire/release

## Interfaces and boundaries

- Inputs: callers pass SQL fragments via `sql` module constants only at compile time.
- Outputs: embedded strings consumed by `audit`, `db`, `apply`, `lock`.
- Boundaries: no runtime read of `rust/sql/` from disk in production binary.

## Assumptions and constraints

- Assumption: SQL files are UTF-8 and ship inside the binary via `include_str!`.
- Constraint: SQL edits require rebuild and integration re-run.

## Nominal flow

1. Developer edits `rust/sql/**/*.sql`.
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

- `make bench-footprint` (Go-side footprint; Rust `perf` module mirrors patterns)
- SQL changes require integration tests (`make rust-plan-db-perf`)

## Open issues and non-goals

- Non-goals: `perf` module is not a production runtime dependency for operators.

## References

- `docs/specs/rust/module-db.md`
- `docs/specs/rust/module-apply.md`
- `docs/perf-footprint-audit.md`
