# Technical Document: Module `gate`

Lifecycle: `Current`.

## Purpose

Describe **incremental prod gate**: git delta resolution, plan snapshot wire format, baseline compare, go/no-go evaluation, e2e report types.

## Scope

- `crates/core/src/gate/changed_paths.rs`, `changed_paths_ci.rs` - git delta paths
- `crates/core/src/gate/delta.rs` - object key closure for changed files
- `crates/core/src/gate/snapshot.rs`, `compare.rs` - plan snapshot JSON
- `crates/core/src/gate/evaluate.rs` - gate verdict
- `crates/core/src/gate/e2e_report.rs` - e2e scenario report wire types
- `crates/core/src/gate/repo_root.rs`, `git_diff.rs` - repo discovery helpers
- Tests: `crates/core/tests/prod_gate_integration.rs`, `scenario_e2e_integration.rs`

## System context

Operators run `make prod-gate` against Docker SQL and baseline JSON (`crates/core/tests/testdata/prod_gate/plan_baseline_empty_db.json`). Changed paths map to normalized keys; only delta keys may differ from baseline when delta is non-empty.

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

- `make prod-gate`
- `make e2e` (subset parity)
- `crates/core/tests/gate_snapshot_test.rs`, `golden_baseline_test.rs`

## Off-nominal behavior and failure containment

- Failure mode: plan change outside git delta.
  Containment: gate NO-GO; pipeline stops promotion.

## Operations and recovery

- Refresh baseline: set `RMIG_GATE_UPDATE_BASELINE=1` and run `make prod-gate` (writes `crates/core/tests/testdata/prod_gate/plan_baseline_empty_db.json`).

## Open issues and non-goals

- Non-goals: gate does not apply migrations.

## References

- `docs/prod-gate.md`
- `ops/perf/prod_gate.sh`
