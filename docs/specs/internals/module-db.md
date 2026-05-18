# Technical Document: Module `internal/db`

Lifecycle: `Current`.

## Purpose

Describe **SQL Server catalog inspection**: build queries from `fs.Layout`, execute through `driver.Conn`, return `db.State` and column metadata used by planning and scaffold.

## Scope

- `internal/db/inspector_impl.go` — inspector implementation, caching, `OPENJSON` vs chunked `IN` fallback
- `internal/db/sql/` — embedded query text (`.sql` files)
- `internal/db/inspector_test.go`, `internal/db/inspector_bench_test.go`, fuzz tests

## System context

`engine` constructs `db.NewInspector()` and calls `Inspect(ctx, conn, layout)`. Results feed `diff.Compute` and `scaffold.Ensure` (table columns).

## Interfaces and boundaries

- Constructor: `db.NewInspector()` (returns concrete type used as `engine.Inspector`).
- Inputs: `driver.Conn`, `fs.Layout`
- Outputs: `*db.State` (schemas, objects, columns, live checksums as needed by queries), `error`
- Must not import `internal/apply`.

## Assumptions and constraints

- Assumption: compatibility level probe (`sys.databases`) determines `OPENJSON` eligibility.
- Constraint: parameter batching respects `driver.DefaultMaxParameters` and `internal/types` chunk helpers on fallback paths.

## Nominal flow

1. Derive scope (schema/object names) from layout maps.
2. Run cached or fresh queries per scope digest (`scopeKey` / SHA-256 hex key — see code).
3. Merge results into `db.State`.

## Off-nominal behavior and failure containment

- Query errors return to engine; `Inspect` caching stores failures per scope until invalidated (see `sync.Once` usage in implementation).

## Verification and validation

- `make check`
- Fuzz tests in `internal/db` where present

## Operations and recovery

- SQL template changes require matching tests and, when applicable, golden updates.

## Open issues and non-goals

- Non-goals: `db` package does not open connections.

## References

- `internal/driver/conn.go`
- `internal/types/chunk.go`
- `docs/specs/internals/module-driver.md`
