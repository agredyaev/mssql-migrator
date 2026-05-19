# Technical Document: Module `internal/engine`

Lifecycle: `Current`.

## Purpose

Describe the **orchestration engine**: scan repository layout, load DB state and checksums, compute migration plan, optional scaffold, apply under lock, and publish bus events for subscribers.

## Scope

- `internal/engine/engine.go` — `Engine` struct, `New`, `Plan`, `Migrate`, `Validate`, `Baseline`, `RepairChecksum`, `runPlan`, `filterAppliedMigrations`, bootstrap gate
- `internal/engine/engine_test.go` — unit tests

## System context

`Engine` is constructed in `internal/app/wire.go` with interfaces for scanner, inspector, loader, diff computer, scaffolder, applier, and locker. Commands are one entry point per user-facing operation.

## Interfaces and boundaries

- Local interfaces (defined on `Engine`): `Scanner`, `Inspector`, `Loader`, `Computer`, `Scaffolder`, `Applier` — allow tests to substitute fakes without exporting engine-wide interfaces elsewhere.
- Inputs: `context.Context`, `types.Config`, `driver.Conn`, `bus.EventBus`
- Outputs: `error` per command; side effects on DB, filesystem (scaffold), and bus events
- Downstream packages: `internal/fs`, `internal/db`, `internal/diff`, `internal/apply`, `internal/lock`, `internal/audit` (via loader adapter), `internal/scaffold`

## Assumptions and constraints

- Assumption: `runPlan` is the shared first phase for commands that need a plan.
- Constraint: `BootstrapChecker` (`SetBootstrapChecker`) gates migrate-style paths when audit bootstrap failed.
- Constraint: blocked plans stop `migrate` after optional `scaffold.Ensure` (see `Migrate` in `engine.go`).

## Nominal flow (high level)

1. Publish `EventRunStarted`.
2. `runPlan`: scan → **parallel** `EnsureTables` ‖ (`LoadChecksums` → `BuildInspectScope` → `InspectWithScope`) → `diff.Compute`. Git delta + audit checksums classify objects as hot (catalog SQL) or stable (synthetic `db.Object`). `RM_SKIP_GIT=1` or `RMIG_INSPECT_FULL=1` forces full inspect.
3. `Migrate`: if blocked, `LoadTableColumns` + `scaffold.Ensure` then fail with `errors.ErrPlanBlocked`; else acquire lock, apply, release.
4. `Validate`: same planning pipeline as `plan` (does not execute `layout.Checks` SQL); publishes changed-module count.
5. `Baseline` / `RepairChecksum`: follow their respective paths in `engine.go`.

## Off-nominal behavior and failure containment

- Any `runPlan` error: `publishRunFailed` and return wrapped error.
- Lock acquisition failure: migrate stops before apply.
- Apply errors: surfaced to caller after lock release (see implementation for ordering).

## Verification and validation

- `make check` (`internal/engine/engine_test.go`, `internal/engine/benchmark_profiling_test.go` compile with tests)

## Operations and recovery

- When adding a command: extend `internal/app` dispatch and add an exported method on `Engine` with explicit bus events for observability.

## Open issues and non-goals

- Non-goals: engine does not open connections (owned by `app`).

## References

- `internal/app/wire.go`
- `internal/diff/diff.go`
- `internal/apply/apply.go`
- `docs/specs/internals/module-fs.md`, `docs/specs/internals/module-diff.md`, `docs/specs/internals/module-apply.md`
