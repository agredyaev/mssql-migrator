# Technical Document: Module `internal/apply`

Lifecycle: `Current`.

## Purpose

Describe **DDL execution**: translate `types.MigrationPlan` + `fs.Layout` into ordered SQL batches, run through `driver.Conn`, and publish structured events to `bus.EventBus` for audit and reporting.

## Scope

- `internal/apply/apply.go` — `Executor`, `Execute`, transactional batching, statement collection
- `internal/apply/apply_test.go`, `internal/apply/apply_bench_test.go`

## System context

`engine` calls `apply.New()` and uses it as the `Applier` interface. Apply runs after plan validation / lock acquisition on migrate paths.

## Interfaces and boundaries

- Inputs: `context.Context`, `driver.Conn`, `types.MigrationPlan`, `fs.Layout`, `bus.EventBus`
- Outputs: `*ApplyResult`, `error`
- Side effects: `ExecContext` on the connection; events for applied/failed objects

## Assumptions and constraints

- Assumption: `Layout` path indexes are coherent with plan object paths (engine ensures scan before apply).
- Constraint: transaction mode and update policy come from `types.Config` (`EffectiveTransactionMode`, `EffectiveUpdatePolicy`).

## Nominal flow

1. Build path indexes on layout if needed.
2. Group work into batches (size limits, kind ordering — see code).
3. Execute each batch; on failure, containment path may retry per statement (read `executeTxBatch`).

## Off-nominal behavior and failure containment

- Batch failure triggers rollback then per-statement isolation for diagnosis (see `apply.go`).

## Verification and validation

- `make check`

## Operations and recovery

- Any new `types.ObjectKind` handling must update apply ordering and tests.

## Open issues and non-goals

- Non-goals: apply does not rescan the repository; layout is input-only.

## References

- `internal/driver/conn.go`
- `internal/bus/payload.go`
- `docs/specs/internals/module-bus.md`
