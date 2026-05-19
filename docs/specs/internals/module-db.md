# Technical Document: Module `internal/db`

Lifecycle: `Current`.

## Purpose

Describe **SQL Server catalog inspection**: build queries from `fs.Layout`, execute through `driver.Conn`, return `db.State` used by planning. Table column metadata is loaded **on demand** for scaffold only.

## Scope

- `internal/db/inspector_impl.go` — inspector implementation, scope caching, OpenJSON queries
- `internal/db/inspector.go` — `Inspector` interface (`Inspect`, `LoadTableColumns`)
- `internal/db/sql/` — embedded OpenJSON query text (`schemas_openjson.sql`, `objects_openjson.sql`, `columns_openjson.sql`)
- `internal/db/inspector_test.go`, `internal/db/inspector_bench_test.go`, fuzz tests

## System context

`engine` constructs `db.NewInspector()` and calls `Inspect(ctx, conn, layout)` during `runPlan`. Results feed `diff.Compute`. When a migrate plan is **blocked**, `engine` calls `LoadTableColumns` before `scaffold.Ensure`.

## Interfaces and boundaries

- Constructor: `db.NewInspector()` (returns `Inspector`).
- `Inspect`: schemas + objects only; `State.TableColumns` is empty.
- `LoadTableColumns`: column metadata for tables in layout scope (OpenJSON query).
- Inputs: `driver.Conn`, `fs.Layout`
- Outputs: `*db.State`, `error`
- Must not import `internal/apply`.

## Assumptions and constraints

- **SQL Server 2016+** with **OPENJSON** (compatibility level 130+). Chunked `IN` fallback paths were removed.
- Constraint: invalidate shared inspector cache after apply via `InvalidateInspectorCache(conn)` from `internal/apply`.

## Nominal flow

1. Derive scope from layout maps; cache key = connection generation + scope digest (`scopeKey` / SHA-256 hex).
2. `Inspect`: one OpenJSON query for schemas (if needed), one for objects (if needed).
3. `LoadTableColumns` (optional): one OpenJSON query for `sys.columns` filtered to layout tables.

## Off-nominal behavior and failure containment

- Query errors return to engine; `Inspect` caching stores failures per scope until invalidated (`sync.Once` per cache entry).

## Verification and validation

- `make check`
- Integration: `internal/db/inspector_integration_test.go` (tag `integration`)
- Fuzz tests in `internal/db` where present

## Operations and recovery

- SQL template changes require matching tests and, when applicable, golden updates.

## Open issues and non-goals

- Non-goals: `db` package does not open connections.

## References

- `internal/driver/conn.go`
- `internal/types/chunk.go` (used by callers, not inspector SQL path)
- `docs/specs/internals/module-driver.md`
- `docs/prod-gate.md` — measured plan-phase timings
