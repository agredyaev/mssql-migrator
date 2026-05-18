# Technical Document: Supporting internal modules

Lifecycle: `Current`.

## Purpose

Document smaller **cross-cutting** packages: structured errors, logging, report subscription, session locks, transition/file scaffolding helpers, and test doubles.

## Scope

| Package | Role | Primary paths |
|---------|------|----------------|
| `internal/errors` | Wrap and classify errors; map to exit codes | `internal/errors/wrap.go`, `internal/errors/classify.go` |
| `internal/log` | Leveled logging to `io.Writer` (stderr in CLI) | `internal/log/log.go` |
| `internal/report` | Subscribe to bus events; write plan/migration/validation artifacts when `RM_REPORT_DIR` set | `internal/report/report.go` |
| `internal/lock` | SQL Server application lock acquire/release around migrate | `internal/lock/locker.go`, `internal/lock/applock.go` |
| `internal/scaffold` | Create transition scaffold / auto files when plan is blocked | `internal/scaffold/scaffold.go`, `internal/scaffold/auto_migrate.go` |
| `internal/testutil` | Shared mocks (`MockConn`, row types) for unit tests | `internal/testutil/conn.go`, `internal/testutil/rows.go` |

## System context

`app` wires `log.New`, attaches `report.NewSubscriber`, passes `lock.New()` into `engine`, and uses `scaffold.New()` as the scaffolder interface implementation.

## Interfaces and boundaries

- `errors`: imported by `engine` / `apply` / `app` to keep exit semantics consistent.
- `log`: not a global singleton; `Logger` instance owned by `app`.
- `report`: side-effect subscriber only; failures warn via `SetErrorHandler`.
- `lock`: uses `driver.Conn` and `cfg.LockTimeout`.
- `scaffold`: mutates repo working tree for blocked transitions (filesystem writes).
- `testutil`: **test-only** dependency; must not leak into production binaries except via `_test.go`.

## Assumptions and constraints

- Assumption: `lock` session matches SQL Server `sp_getapplock` semantics documented in `applock.go`.
- Constraint: `testutil` must stay free of imports from `internal/app` to avoid cycles.

## Nominal flow

- `log`: Info/Warn/Error with string component + message.
- `lock`: Acquire at migrate start, release on success and failure paths (see `engine`).
- `scaffold`: Invoked when plan blocked to ensure operator-facing files exist.

## Off-nominal behavior and failure containment

- `scaffold.Ensure` errors abort migrate before apply.
- `report` subscriber errors are non-fatal warnings unless escalated elsewhere.

## Verification and validation

- `make check`

## Operations and recovery

- Changing report filenames or formats requires updates in `internal/report` and product docs (`docs/runbook.md`).

## Open issues and non-goals

- Non-goals: `testutil` is not a public SDK.

## References

- `internal/app/wire.go`
- `internal/engine/engine.go`
- `docs/specs/internals/module-app.md`, `docs/specs/internals/module-engine.md`
