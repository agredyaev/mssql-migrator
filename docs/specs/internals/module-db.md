# Technical Document: Module `internal/db`

Lifecycle: `Current`.

## Purpose

Describe **SQL Server catalog inspection**: build queries from `fs.Layout`, execute through `driver.Conn`, return `db.State` used by planning. Table column metadata is loaded **on demand** for scaffold only.

## Scope

- `internal/db/inspector_impl.go` — inspector implementation, scope caching, OpenJSON queries
- `internal/db/inspector.go` — `Inspector` interface (`Inspect`, `InspectWithScope`, `LoadTableColumns`)
- `internal/db/inspect_scope.go` — `InspectScope` (full / hot refs / stable objects)
- `internal/db/sql/` — catalog inspect via `buildCatalogStateSQL` (`catalog_*.sql` fragments, one round-trip); column load via `columns_openjson.sql`; scope uses `types.MarshalObjectScopeJSON` **(schema, kind, object)** triples
- `internal/db/inspector_test.go`, `internal/db/inspector_bench_test.go`, fuzz tests

## System context

`engine` constructs `db.NewInspector()` and calls `InspectWithScope(ctx, conn, layout, scope)` during `runPlan` (scope from `engine.BuildInspectScope` after checksums load). `Inspect` is equivalent to `FullInspect: true`. Results feed `diff.Compute`. When a migrate plan is **blocked**, `engine` calls `LoadTableColumns` before `scaffold.Ensure`.

## Interfaces and boundaries

- Constructor: `db.NewInspector()` (returns `Inspector`).
- `Inspect` / `InspectWithScope`: schemas + objects only; `State.TableColumns` is empty. Scoped path merges **stable** objects from audit/file checksum match without catalog SQL; **hot** refs use kind-filtered `buildCatalogStateSQL` (or empty-DB fast path).
- `LoadTableColumns`: column metadata for tables in layout scope (OpenJSON query).
- Inputs: `driver.Conn`, `fs.Layout`
- Outputs: `*db.State`, `error`
- Must not import `internal/apply`.

## Assumptions and constraints

- **SQL Server 2016+** with **OPENJSON** (compatibility level 130+). Chunked `IN` fallback paths were removed.
- Constraint: invalidate shared inspector cache and **persistent catalog cache** (`azdo_deploy_meta.catalog_*`) after apply via `InvalidateInspectorCache(conn)` from `internal/apply`.
- Persistent catalog cache (phase 3): [`catalog_cache.go`](../../db/catalog_cache.go); disabled with `RMIG_CATALOG_CACHE=0`.

## Nominal flow

1. Derive scope from layout maps (or hot-ref subset when scoped); cache key = connection generation + scope digest (`scopeKey` / SHA-256 hex; scoped slots include hot-ref hash).
2. `Inspect` / `InspectWithScope`:
   - **No layout objects:** `queryExistingLayoutSchemas` only (one OpenJSON query).
   - **Empty DB fast path:** `catalog_scoped_hit.sql` only; if no hit, return empty `Objects` / `Schemas` (one round-trip).
   - **Otherwise:** `buildCatalogStateSQL` (kind-filtered CTEs, one round-trip for schemas + objects).
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
