# Technical Document: Module `internal/diff`

Lifecycle: `Current`.

## Purpose

Describe the **planning core**: `Computer` computes `types.MigrationPlan` from `fs.Layout`, live `db.State`, and prior checksum map.

## Scope

- `internal/diff/diff.go` — `Computer`, `Compute`, warmup, per-object action selection
- `internal/diff/diff_test.go`, `internal/diff/diff_bench_test.go`

## System context

`engine.runPlan` supplies a populated `db.State` (from inspector), checksums from `audit.LoadChecksums`, and the scanned `fs.Layout`. `Compute` returns a plan with objects, schemas, optional blockers, and `Blocked` flag.

## Interfaces and boundaries

- Public constructor: `diff.NewComputer()` returns `*Computer` implementing `engine.Computer` interface (method set matches `Compute`).
- Inputs: `context.Context`, `fs.Layout`, `*db.State`, `map[string][32]byte` checksums
- Outputs: `*types.MigrationPlan`, `error`
- Must not import `internal/apply` (plan only).

## Assumptions and constraints

- Assumption: checksum entries align with `types` key normalization used elsewhere.
- Constraint: warmup uses bounded concurrency on cold paths (see `warmupAll` in `diff.go`).

## Nominal flow

1. Allocate plan containers with capacities derived from layout sizes.
2. Match objects against DB state and checksums; assign actions (`ActionCreateObject`, skip, block, etc.).
3. Populate optional transition maps when layout has transitions.

## Off-nominal behavior and failure containment

- Nil `state` or `checksums` handled as documented in `Compute` (early branches); benchmarks rely on explicit initialization.

## Verification and validation

- `make check`

## Operations and recovery

- Plan JSON shape changes belong in `internal/types` with tests; `diff` must stay consistent with `PlannedObject` serialization rules.

## Open issues and non-goals

- Non-goals: `diff` does not write reports or execute DDL.

## References

- `internal/types/plan.go`, `internal/types/planned_json.go`
- `internal/db/inspector_impl.go` (state shapes from inspection)
- `docs/specs/internals/module-types.md`
