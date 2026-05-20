# Technical Document: Module `gate`

Lifecycle: `Current`.

## Purpose

Describe **incremental prod gate**: git delta resolution, plan snapshot wire format, baseline compare, go/no-go evaluation, Go↔Rust e2e report types.

## Scope

- `rust/crates/core/src/gate/changed_paths.rs`, `changed_paths_ci.rs` — git delta paths
- `rust/crates/core/src/gate/delta.rs` — object key closure for changed files
- `rust/crates/core/src/gate/snapshot.rs`, `compare.rs` — plan snapshot JSON
- `rust/crates/core/src/gate/evaluate.rs` — gate verdict
- `rust/crates/core/src/gate/e2e_report.rs` — e2e scenario report wire types
- `rust/crates/core/src/gate/repo_root.rs`, `git_diff.rs` — repo discovery helpers
- Tests: `rust/crates/core/tests/prod_gate_integration.rs`, `go_rust_scenario_integration.rs`

## System context

Operators run `make rust-prod-gate` against Docker SQL and baseline JSON (`internal/app/testdata/prod_gate/plan_baseline_empty_db.json`). Changed paths map to normalized keys; only delta keys may differ from baseline when delta is non-empty.

## Interfaces and boundaries

- Public: `resolve_changed_paths`, `evaluate_gate`, snapshot compare helpers, e2e report builders
- Inputs: plan JSON, baseline file, git repo at SQL root
- Outputs: `GateResult`, compare diffs, e2e parity messages

## Assumptions and constraints

- CI git delta: merge-base or PR env (see `docs/ci-checkout.md`).
- `RMIG_GATE_MAX_PLAN_WALL_MS` optional wall SLO on gate harness.

## Nominal flow

1. Run plan pipeline → `PlanSnapshot`.
2. Resolve changed paths → key set.
3. `evaluate_gate` → GO / NO-GO + reasons.

## Verification and validation

- `make rust-prod-gate`
- `make go-rust-e2e` (subset parity)
- `rust/crates/core/tests/gate_snapshot_test.rs`, `golden_baseline_test.rs`

## Off-nominal behavior and failure containment

- Failure mode: plan change outside git delta.
  Containment: gate NO-GO; pipeline stops promotion.

## Operations and recovery

- Refresh baseline: `make test-prod-gate-update-baseline` (Go reference path); Rust gate reads same JSON file.

## Open issues and non-goals

- Non-goals: gate does not apply migrations.

## References

- `docs/prod-gate.md`
- `ops/perf/rust_prod_gate.sh`
