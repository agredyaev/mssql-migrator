# WBS — Implementation Plan

> Work Breakdown Structure for the rmig rewrite. Each epic is a self-contained work stream. Tasks within each epic are ordered. Dependencies between epics are noted at the top.

**References**: `new_arch.txt` at project root. Cross-refs use `§N` (line number in new_arch.txt).

## Progress

| Epic | Status | Completed |
|---|---|---|
| 1.0 types/ | ✅ done | 7/7 | 2026-05-14 |
| 2.0 errors/ | ✅ done | 14/~10 | 2026-05-14 |
| 3.0 driver/ | ✅ done | 3/~5 | 2026-05-14 |
| 4.0 bus/ | ✅ done | 5/~4 | 2026-05-14 |
| 5.0 log/ | ✅ done | 14/~5 | 2026-05-14 |
| 6.0 lock/ | ✅ done | 7/~5 | 2026-05-14 |
| 7.0 fs/ | ✅ done | 14/8-10 | 2026-05-14 |
| 8.0 db/ | ✅ done | 8/6-8 | 2026-05-14 |
| 9.0 audit/ | ✅ done | 8/6-8 | 2026-05-14 |
| 10.0 diff/ | ✅ done | 15 | 2026-05-14 |
| 11.0 scaffold/ | ✅ done | 4 | 2026-05-14 |
| 12.0 apply/ | ✅ done | 9 | 2026-05-14 |
| 13.0 report/ | ✅ done | 2 | 2026-05-14 |
| 11.0–19.0 | ⬜ pending | — | — |

---

## Dependency Graph

```
 1.0 types/ ─┬── 2.0 errors/ ─── 5.0 bus/ ─── 4.0 log/
              │                                    │
              ├── 3.0 driver/ ─── 6.0 lock/        │
              │                                    │
              ├── 7.0 fs/ ──────── 8.0 db/ ────────┤
              │                    │                │
              │                    ├── 9.0 audit/ ──┤
              │                    │                │
              │                    ├── 10.0 diff/ ──┤
              │                    │                │
              │                    ├── 11.0 scaffold/ │
              │                    │                │
              │                    ├── 12.0 apply/ ──┤
              │                    │                │
              │                    └── 13.0 report/  │
              │                                      │
              └──────── 14.0 engine/ ────────────────┤
                                                     │
                             15.0 app/ ──────────────┘

16.0 SQL templates (sql/) — inline with 7.0/8.0/9.0/6.0
17.0 Legacy deletion — after 14.0 compiles
18.0 Integration tests — after 15.0
19.0 Documentation — continuous
```

---

## Epic 1.0 — types/package (Foundation)

**Depends on**: nothing
**Used by**: all other epics

Defines every shared data structure, event type, action constant, exit code, and configuration shape.

Ref: `§115-124`, `§156-169`

| ID | Task | Ref | Est. | Test? |
|---|---|---|---|---|
| 1.1 | Define `Config` struct — DB connection, fs paths, lock timeout, policy enums, env var tags | §116 | .5d | — |
| 1.2 | Define `Event` constants — one per event (run.started, diff.computed, scaffold.generated, etc.) | §157-169 | .25d | — |
| 1.3 | Define `Action` constants — planned action per object (create, adopt, reprocess, skip, blocked) | §118 | .25d | — |
| 1.4 | Define `ExitCode` constants — ExitOK=0, ExitConfigError=2, ExitPlanBlocked=3, etc. | §119 | .25d | — |
| 1.5 | Define `MigrationPlan`, `PlannedObject`, `PlannedSchema`, `PlanSummary` — plan data model | §121 | .5d | — |
| 1.6 | Define `RunRecord`, `ItemRecord`, `AttemptRecord` — audit data model | §122 | .5d | — |
| 1.7 | Define `MigrationReport`, `ValidationReport`, `ScriptResult`, `Failure` — report data model | §121 | .5d | — |
| 1.8 | Define event payload structs — `RunStarted`, `RunFinished`, `DiffResult`, `SchemaEvent`, `ObjectEvent`, `FailureEvent`, `ValidationEvent`, `ValidationResult`, `ScaffoldResult` | §124 | .5d | — |
| 1.9 | Define `PlanTarget`, `ValidationSummary` | §123 | .25d | — |
| 1.10 | Implement `NormalizedKey(schema, kind, name) string` — canonical key builder, used by fs/db/audit/diff | §701 | .25d | T1.10 |
| 1.11 | Define `sqlServerMaxParameters = 2100` constant | §699 | .1d | — |
| 1.12 | Unit test: `NormalizedKey` is deterministic, case-insensitive, handles all input combinations | §701 | .5d | ✅ |

**Tests**: T1.10 — edge cases: empty parts, special chars, mixed case, nil parent

---

## Epic 2.0 — errors/package (Foundation)

**Depends on**: 1.0 types/
**Used by**: all epics

Sentinel errors, classification, failure envelope, exit code mapping.

Ref: `§126-135`

| ID | Task | Ref | Est. |
|---|---|---|---|
| 2.1 | Define sentinel error vars — ErrConfig, ErrConnection, ErrChecksumMismatch, ErrSQLExecution, ErrValidation, ErrLockTimeout, ErrInvalidInput, ErrCriticalState, ErrApprovedPlanMissing, ErrApprovedPlanMismatch, ErrMetadataDrift, ErrMissingSchemaPermission, ErrMissingObjectPermission, ErrMissingParentObject | §127 | .5d |
| 2.2 | Implement `Wrap(base, cause)` — multi-error with `Unwrap()` chain | §128 | .25d |
| 2.3 | Implement `ClassifyDetails(base, cause) → Classification{Class, Path, Reason, SQL}` | §129 | .5d |
| 2.4 | Implement `Build(cfg, phase, err) → Failure` and `BuildWithCause(cfg, phase, base, cause) → Failure` | §130-131 | .5d |
| 2.5 | Implement `Envelope(failure) → string` — formatted stderr output | §132 | .25d |
| 2.6 | Implement `Evaluate(cfg, event, err) → Outcome{Failure, ExitCode}` | §133 | .5d |
| 2.7 | Implement `EvaluatePlanBlocked(cfg, plan) → Outcome` | §134 | .25d |
| 2.8 | Implement `ExitCode(err) → int` — walk error chain, map to exit code | §135 | .25d |
| 2.9 | Unit test: classification of every sentinel, wrapping, exit code mapping | §507 | 1d |

---

## Epic 3.0 — driver/ + driver/mssql/ (Foundation)

**Depends on**: 1.0 types/
**Used by**: 6.0 lock/, 8.0 db/, 9.0 audit/, 12.0 apply/, 14.0 engine/

Database adapter interface + MSSQL implementation.

Ref: `§181-203`

| ID | Task | Ref | Est. |
|---|---|---|---|
| 3.1 | Define `driver/conn.go` — `Rows` interface (`Scan`, `Close`, `Next`) | §184-188 | .25d |
| 3.2 | Define `Result` interface (`RowsAffected`) | §189-192 | .1d |
| 3.3 | Define `Conn` interface — `QueryContext`, `ExecContext`, `Ping`, `Close` | §194-199 | .25d |
| 3.4 | Implement `driver/mssql/` — `mssql.Conn` struct wrapping `sql.DB` from go-mssqldb | §201-202 | .5d |
| 3.5 | Implement `NewConn(cfg) (*mssql.Conn, error)` — build connection string from Config, create sql.DB | §202 | .25d |
| 3.6 | Unit test: mock `driver.Conn` to verify interface contract compiles | — | .25d |

**Note**: 3.4 requires `go get github.com/microsoft/go-mssqldb`

---

## Epic 4.0 — bus/package (Foundation)

**Depends on**: 1.0 types/
**Used by**: 9.0 audit/, 13.0 report/, 14.0 engine/, 15.0 app/

In-process pub/sub with synchronous delivery.

Ref: `§143-155`, `§157-169`

| ID | Task | Ref | Est. |
|---|---|---|---|
| 4.1 | Define `EventBus` interface — `Publish(event, payload any)`, `Subscribe(event, handler func(payload any))` | §145-149 | .25d |
| 4.2 | Implement `bus.New() *Bus` — map[Event][]func(any) + sync.Mutex | §150 | .25d |
| 4.3 | Implement `Publish` — iterate handlers, call each synchronously, panic recovery per handler | §151 | .25d |
| 4.4 | Implement `Subscribe` — append handler to slice | §148 | .1d |
| 4.5 | Unit test: publish → subscriber receives payload | §508 | .5d |
| 4.6 | Unit test: multiple subscribers for same event all receive it | — | .25d |
| 4.7 | Unit test: publishing to event with no subscribers does not panic | — | .25d |

---

## Epic 5.0 — log/package (Foundation)

**Depends on**: 1.0 types/
**Used by**: 15.0 app/

Structured logger with redaction.

Ref: `§137-141`

| ID | Task | Ref | Est. |
|---|---|---|---|
| 5.1 | Implement `StructuredLogger{JSON, Level, Writer, sync.Mutex}` | §138 | .25d |
| 5.2 | Implement `Debug/Info/Warn/Error(event, message string)` — structured output | §139 | .25d |
| 5.3 | Implement `ErrorEnvelope(event, envelope)` — plain text or JSON | §140 | .25d |
| 5.4 | Implement `Redact(value) string` — regex masking of passwords, tokens, secrets | §141 | .5d |
| 5.5 | Unit test: Redact masks known patterns (conn strings, passwords, tokens) | — | .5d |
| 5.6 | Unit test: ErrorEnvelope formatting matches expected output | — | .25d |

---

## Epic 6.0 — lock/package (Foundation)

**Depends on**: 1.0 types/, 3.0 driver/
**Used by**: 14.0 engine/

Advisory application lock via `sp_getapplock`.

Ref: `§171-179`, `§824`

| ID | Task | Ref | Est. |
|---|---|---|---|
| 6.1 | Define `Locker` interface — `Acquire(ctx, conn, timeout)`, `Release(ctx, conn)` | §172-176 | .1d |
| 6.2 | Implement MSSQL `lock.Locker` — `sp_getapplock` with Exclusive mode, session scope | §178-179 | .5d |
| 6.3 | SQL template: `sql/lock/acquire.sql` — `EXEC sp_getapplock @Resource, @LockMode='Exclusive', @LockOwner='Session', @LockTimeout` | §23 | .1d |
| 6.4 | SQL template: `sql/lock/release.sql` — `EXEC sp_releaseapplock @Resource, @LockOwner='Session'` | — | .1d |
| 6.5 | Unit test: mock driver.Conn, verify `acquire.sql` was executed with correct params | §506 | .5d |
| 6.6 | Unit test: timeout returns `ErrLockTimeout` | — | .25d |

---

## Epic 7.0 — fs/package

**Depends on**: 1.0 types/, 5.0 log/ (optional, for debug logging)
**Used by**: 8.0 db/, 9.0 audit/, 10.0 diff/, 11.0 scaffold/, 14.0 engine/

Lazy filesystem scanner — discovers layout, transitions, check scripts, computes checksums lazily.

Ref: `§205-278`

| ID | Task | Ref | Est. |
|---|---|---|---|
| 7.1 | Define `Scanner` interface — `Scan(ctx, root) (Layout, error)` | §207-209 | .1d |
| 7.2 | Define `Layout` struct — RootPath, Schemas, Objects, Transitions, Checks | §211-217 | .25d |
| 7.3 | Define `Schema` struct — Name, NormalizedName | §219-222 | .1d |
| 7.4 | Define `Object` struct — Path, NormalizedKey, SchemaName, Kind, ObjectName, ParentName, NoTransaction, Content (lazy func), Checksum (lazy func) | §224-236 | .25d |
| 7.5 | Define `TransitionScript` struct — Path, AbsolutePath, SchemaName, TableName, NormalizedKey, Checksum, Ordinal, Commit, Slug, NoTransaction, Scaffold, Content (lazy func) | §238-252 | .25d |
| 7.6 | Define `CheckScript` struct — Path, AbsolutePath, SchemaName, Name, Checksum, NoTransaction, Content (lazy func) | §254-263 | .25d |
| 7.7 | Implement `Scan(ctx, root)` — walk dir tree recursively, discover .sql files, classify by directory depth | §265-277 | 1d |
| 7.8 | Implement transition file parsing — validate `{ordinal}_{commit}_{slug}.sql` naming, extract fields | §268-270 | .5d |
| 7.9 | Implement lazy Content func — `sync.Once` reads file on first call, caches | §234 | .5d |
| 7.10 | Implement lazy Checksum func — `sync.Once` calls NormalizeSQL + SHA256, caches | §275 | .5d |
| 7.11 | Implement `HasExecutableTransition(scripts) bool` — returns false if all scaffolds | §272 | .25d |
| 7.12 | Implement Layout hash — SHA256 of sorted (normal object checksums + transition checksums) | §276 | .25d |
| 7.13 | Implement table parser — extract column definitions from CREATE TABLE for auto-migration comparison | §277 | .5d |
| 7.14 | Unit test: empty directory returns empty Layout | §525 | .25d |
| 7.15 | Unit test: discovers objects at correct paths, classifies by kind | — | .5d |
| 7.16 | Unit test: rejects malformed transition file names | §621-631 | .25d |
| 7.17 | Unit test: lazy Content not loaded on Scan, loaded on first call | §633-643 | .25d |
| 7.18 | Unit test: lazy Checksum cached — second call = zero filesystem I/O | — | .25d |
| 7.19 | Unit test: transition files correctly sorted by ordinal within each table group | §269 | .25d |
| 7.20 | Unit test: scaffold detection via `-- rmig: transition-scaffold` first line | §271 | .25d |
| 7.21 | Unit test: discovers CheckScript files under `<schema>/checks/` | — | .25d |

---

## Epic 8.0 — db/package

**Depends on**: 1.0 types/, 3.0 driver/, 7.0 fs/, 16.0 sql/catalog/
**Used by**: 10.0 diff/, 11.0 scaffold/, 14.0 engine/

Scoped, cached, lazy database inspector — reads live system catalog only.

Ref: `§279-314`

| ID | Task | Ref | Est. |
|---|---|---|---|
| 8.1 | Define `Inspector` interface — `Inspect(ctx, conn, scope) (State, error)` | §281-284 | .1d |
| 8.2 | Define `State` struct — Schemas map, Objects map, TableColumns map | §286-290 | .25d |
| 8.3 | Define `db.Object` struct — SchemaName, Kind, ObjectName, ParentName | §292-296 | .1d |
| 8.4 | Define `TableColumn` struct — Name, NormalizedName, TypeName, Length, Precision, Scale, Nullable | §299-307 | .1d |
| 8.5 | SQL template: `sql/catalog/schemas.sql` — query sys.schemas filtered by names | §19, §310 | .25d |
| 8.6 | SQL template: `sql/catalog/objects.sql` — query sys.objects filtered by schema+scope, scoped | §19, §310 | .25d |
| 8.7 | SQL template: `sql/catalog/columns.sql` — query sys.columns for all tables in scope | §19, §311 | .25d |
| 8.8 | SQL template: `sql/catalog/table_types.sql` — query sys.table_types for scope | — | .1d |
| 8.9 | Implement `Inspect(ctx, conn, Layout) (State, error)` — build queries from scope, exec, cache | §309-313 | 1d |
| 8.10 | Implement internal caching — map keyed by `layout hash`, same scope → 0 round-trips | §314 | .5d |
| 8.11 | Implement chunked query builder — IF len(keys) > 2100, split into chunks | §313 | .5d |
| 8.12 | Unit test: empty scope returns empty State | — | .25d |
| 8.13 | Unit test: same scope twice — second call returns cached, zero queries | §648-658 | .25d |
| 8.14 | Unit test: chunking kicks in at 2101 keys | — | .5d |
| 8.15 | Unit test: connection failure returns wrapped error | — | .25d |
| 8.16 | Unit test: TableColumns map is populated for table-type objects only | — | .25d |

---

## Epic 9.0 — audit/package

**Depends on**: 1.0 types/, 3.0 driver/, 4.0 bus/, 16.0 sql/audit/
**Used by**: 14.0 engine/ (via LoadChecksums)

Two responsibilities: query API (LoadChecksums) and bus subscriber (INSERT/UPDATE audit tables).

Ref: `§379-398`

| ID | Task | Ref | Est. |
|---|---|---|---|
| 9.1 | Implement `LoadChecksums(ctx, conn, keys) (map[string]string, error)` — batched VALUES query | §382, §388-390 | .5d |
| 9.2 | Implement chunked INSERT builder for keys > 2100 | §390 | .25d |
| 9.3 | SQL template: `sql/audit/bootstrap.sql` — CREATE TABLE `__migrator.runs`, `__migrator.items`, `__migrator.attempts`, etc. | §23 | .5d |
| 9.4 | SQL template: `sql/audit/load_checksums.sql` — SELECT normalized_key, checksum FROM `__migrator.object_state` WHERE key IN (...) | §23 | .25d |
| 9.5 | SQL template: `sql/audit/insert_run.sql` | — | .1d |
| 9.6 | SQL template: `sql/audit/insert_items.sql` | — | .25d |
| 9.7 | SQL template: `sql/audit/insert_attempt.sql` | — | .1d |
| 9.8 | SQL template: `sql/audit/update_run.sql` | — | .1d |
| 9.9 | Implement `audit.NewSubscriber(bus, conn, cfg)` — subscribe to events, bootstrap metadata on run.started | §385, §392 | .5d |
| 9.10 | Implement run.started handler — bootstrap DDL, INSERT runs row | §393 | .25d |
| 9.11 | Implement diff.computed handler — INSERT items rows (one per planned object) | §394 | .25d |
| 9.12 | Implement object.(applied|skipped|failed) handler — INSERT attempts rows | §395 | .25d |
| 9.13 | Implement run.finished handler — UPDATE runs row with success/failure | §396 | .25d |
| 9.14 | Unit test: LoadChecksums with 0 keys returns empty map | — | .25d |
| 9.15 | Unit test: LoadChecksums with 2500 keys → 2 batch queries | §529 | .5d |
| 9.16 | Unit test: bootstrap metadata on run.started event | — | .5d |
| 9.17 | Unit test: object.applied event → INSERT attempt record | — | .5d |

---

## Epic 10.0 — diff/package

**Depends on**: 1.0 types/, 7.0 fs/, 8.0 db/
**Used by**: 11.0 scaffold/, 14.0 engine/

Pure computation — no I/O, no side effects. Compares layout vs state vs checksums → plan.

Ref: `§316-335`

| ID | Task | Ref | Est. |
|---|---|---|---|
| 10.1 | Define `Computer` interface — `Compute(ctx, layout, state, checksums) (*MigrationPlan, error)` | §318-320 | .1d |
| 10.2 | Implement object iteration — for each obj in layout, determine PlannedAction | §322-335 | 1d |
| 10.3 | Implement existence check — obj.NormalizedKey in db.State?.Objects | §323 | .25d |
| 10.4 | Implement checksum comparison — layout checksum vs audit checksum → changed/unchanged | §324 | .25d |
| 10.5 | Implement policy dispatch — module_only (views, procs, funcs) vs all_supported (tables, indexes, types) | §326 | .5d |
| 10.6 | Implement table special case — transition detection, blocked plan when changed + no transitions | §327-331 | .5d |
| 10.7 | Implement ActionAdoptExisting — table exists, no prior audit row (baseline) | §332 | .25d |
| 10.8 | Implement ActionCreateObject — new object (not in DB) | §333 | .25d |
| 10.9 | Implement ActionUpdateExistingModule / ActionReprocessChanged for non-tables | §334 | .5d |
| 10.10 | Populate `PlannedObject.TransitionPaths` for tables with checked-in transitions | §335 | .25d |
| 10.11 | Compute PlanSummary — blocked status, block reasons, object counts per action | — | .25d |
| 10.12 | **Edge-case tests**: | §525-538, §549-583 | 3d |
| 10.12a | Table changed, no transitions → blocked plan with `ActionReprocessChangedBlocked` | §552-574 | — |
| 10.12b | Table changed, transitions exist → `ActionReprocessChanged`, TransitionPaths populated | — | — |
| 10.12c | Table unchanged, matching checksum → skip (ActionSkip) | — | — |
| 10.12d | Table new (not in DB) → `ActionCreateObject` | — | — |
| 10.12e | Table new, missing parent → `ActionCreateObject` with parent | — | — |
| 10.12f | Table changed, all scaffolds → blocked (keep blocked) | §330 | — |
| 10.12g | View changed → `ActionUpdateExistingModule` | — | — |
| 10.12h | Trigger missing parent object → blocked | — | — |
| 10.12i | Empty layout → empty plan, no actions | — | — |
| 10.12j | Nil state → all objects planned as `ActionCreateObject` | — | — |
| 10.12k | Object kind changes (view→table) → blocked | — | — |
| 10.12l | Duplicate NormalizedKey in layout → error | — | — |

---

## Epic 11.0 — scaffold/package

**Depends on**: 1.0 types/, 7.0 fs/, 8.0 db/, 10.0 diff/
**Used by**: 14.0 engine/

Auto-generates transition scaffold files when diff detects changed table with no transitions.

Ref: `§400-413`

| ID | Task | Ref | Est. |
|---|---|---|---|
| 11.1 | Define `Scaffolder` interface — `EnsureTransitionFiles(ctx, cfg, layout, plan, columns) (bool, error)` | §402-404 | .1d |
| 11.2 | Implement auto-migration for simple ADD COLUMN — compare repo columns vs live columns, generate ALTER TABLE if only nullable/default cols added | §408-409, §751-761 | 1d |
| 11.3 | Implement scaffold placeholder generation — write `-- rmig: transition-scaffold` file | §410-413, §734-749 | .5d |
| 11.4 | Implement file naming — `{ordinal}_{commit_token}_{slug}.sql` | §411 | .25d |
| 11.5 | Implement ordinal detection — find highest existing ordinal in target dir, increment | — | .25d |
| 11.6 | Implement commit token extraction — attempt `git rev-parse --short HEAD`, fall back to `0000000` | — | .25d |
| 11.7 | Generate `scaffold.generated` event with created file paths | §413 | .25d |
| 11.8 | **Edge-case tests**: | — | 1.5d |
| 11.8a | Only nullable columns added → auto ALTER TABLE, not scaffold | — | — |
| 11.8b | Column type changed → scaffold placeholder | — | — |
| 11.8c | Column dropped → scaffold placeholder | — | — |
| 11.8d | Target dir does not exist → create dir structure | — | — |
| 11.8e | Target file already exists → skip (idempotent) | — | — |

---

## Epic 12.0 — apply/package

**Depends on**: 1.0 types/, 3.0 driver/, 4.0 bus/, 16.0 SQL execution
**Used by**: 14.0 engine/

Executes planned SQL changes, publishes events per object.

Ref: `§337-377`

| ID | Task | Ref | Est. |
|---|---|---|---|
| 12.1 | Define `Executor` interface — `Execute(ctx, conn, plan, transitions, bus) (ApplyResult, error)` | §339-341 | .1d |
| 12.2 | Implement execution order: schemas first, then transitions for tables, then new tables, then adopted, then views/procs/funcs, then indexes/types | §346-372, §764-776 | 1d |
| 12.3 | Implement transition lookup — map `TransitionScript.Path` ↔ `PlannedObject.TransitionPaths` | §344 | .25d |
| 12.4 | Implement per-object event publishing — applied, skipped, failed | §374, §375 | .5d |
| 12.5 | Implement transaction management — explicit tx for non-check objects | §376 | .5d |
| 12.6 | Publish one `object.applied` event per transition file executed | §375 | .25d |
| 12.7 | **Edge-case tests**: | — | 2d |
| 12.7a | Empty plan → no DB calls, empty ApplyResult | — | — |
| 12.7b | Execution order: schema created before table, table before view | — | — |
| 12.7c | Transition ordering correct (001 before 002 before 003) | — | — |
| 12.7d | Error mid-transition → rollback whole batch | — | — |
| 12.7e | Object with NoTransaction flag → executed outside explicit tx envelope | — | — |
| 12.7f | Verify correct event payload per object type (applied / skipped / failed) | — | — |
| 12.7g | Connection failure mid-execution → context cancel | — | — |
| 12.7h | `ActionCreateObject` for table → uses CREATE TABLE from .sql file | — | — |
| 12.7i | `ActionAdoptExisting` → zero SQL executed, zero events | — | — |

---

## Epic 13.0 — report/package

**Depends on**: 1.0 types/, 4.0 bus/
**Used by**: 15.0 app/ (wires subscriber)

Subscribes to bus events, writes JSON report files.

Ref: `§415-422`

| ID | Task | Ref | Est. |
|---|---|---|---|
| 13.1 | Implement `NewSubscriber(bus, cfg)` — subscribe to diff.computed, run.finished | §417-418 | .25d |
| 13.2 | On diff.computed — `json.Marshal` plan summary → `.plan.json` | §420 | .25d |
| 13.3 | On run.finished — `json.Marshal` run results → `.report.json` | §421 | .25d |
| 13.4 | Support `--json` flag → stdout instead of files | §420 | .25d |
| 13.5 | Unit test: correct .plan.json shape on diff.computed event | — | .5d |
| 13.6 | Unit test: correct .report.json shape on run.finished | — | .5d |
| 13.7 | Unit test: atomic write — partial write on crash does not leave corrupt file | — | .5d |

---

## Epic 14.0 — engine/package

**Depends on**: 1.0–13.0 (all foundation + layer epics)
**Used by**: 15.0 app/

Five public methods — pure orchestration, no business logic.

Ref: `§424-453`

| ID | Task | Ref | Est. |
|---|---|---|---|
| 14.1 | Define `Engine` struct with all dependencies | §426-437 | .1d |
| 14.2 | Implement `NewEngine(cfg, bus, conn, fs, db, audit, diff, apply, scaffold, lock) *Engine` | — | .1d |
| 14.3 | Implement `Plan(ctx)` — scan → inspect → load checksums → compute → publish diff.computed | §439, §445-453, §459-489 | .5d |
| 14.4 | Implement `Migrate(ctx)` — plan + lock + scaffold if blocked + apply | §440, §814-828 | .5d |
| 14.5 | Implement `Validate(ctx)` — plan + run checks + publish validation.done | §441 | .5d |
| 14.6 | Implement `Baseline(ctx)` — adopt existing objects without transitions | §442 | .5d |
| 14.7 | Implement `RepairChecksum(ctx)` — recalculate and update audit | §443 | .5d |
| 14.8 | Implement error handling — classify through errors package, publish run.finished with failure | §453 | .5d |
| 14.9 | **Edge-case tests**: | — | 1.5d |
| 14.9a | Plan: blocked plan → does NOT call apply | §602-616 | — |
| 14.9b | Plan: publishes diff.computed | §588-600 | — |
| 14.9c | Migrate: acquires lock before apply | — | — |
| 14.9d | Migrate: blocked plan → calls scaffold, returns ErrPlanBlocked | — | — |
| 14.9e | Migrate: lock timeout → returns lock error | — | — |
| 14.9f | Validate: publishes validation.start and validation.done | — | — |
| 14.9g | All commands publish run.started and run.finished | — | — |
| 14.9h | Error in any step → run.finished published with failure payload | — | — |

---

## Epic 15.0 — app/package

**Depends on**: 1.0 types/, 4.0 bus/, 5.0 log/, 14.0 engine/
**Used by**: `cmd/rmig/main.go`

CLI entry point — flag parsing, env file, dependency wiring.

Ref: `§108-113`

| ID | Task | Ref | Est. |
|---|---|---|---|
| 15.1 | Define CLI flags — `--config`, `--env`, `--json`, subcommands (plan, migrate, validate, baseline, repair) | §109 | .5d |
| 15.2 | Implement env file loading — `.env.` precedence: cli flag > env file > defaults | §110 | .25d |
| 15.3 | Implement dependency wiring — create real implementations, wire into Engine | §110-111 | .5d |
| 15.4 | Wire bus subscribers — audit, report, log → bus.Subscribe | §112-113 | .25d |
| 15.5 | Call engine method based on subcommand | §111 | .25d |
| 15.6 | Map engine error → os.Exit via errors.ExitCode | §109 | .1d |
| 15.7 | Unit test: flag parsing for each subcommand | §505 | .5d |
| 15.8 | Unit test: env file precedence — env var overrides .env file default | — | .5d |
| 15.9 | Unit test: missing required config → ErrConfig + ExitCode 2 | — | .25d |

---

## Epic 16.0 — SQL Templates (sql/)

**Depends on**: aligns with 6.0, 7.0, 8.0, 9.0
**Cross-cutting**: created inline with those epics, listed here for audit

Ref: `§16-23`

| ID | Task | Ref | Est. |
|---|---|---|---|
| 16.1 | Create `sql/` directory structure — catalog/, audit/, lock/ | §80-83 | .1d |
| 16.2 | `sql/catalog/schemas.sql` — query sys.schemas filtered by schema names | — | .25d |
| 16.3 | `sql/catalog/objects.sql` — sys.objects by schema+kind | — | .25d |
| 16.4 | `sql/catalog/columns.sql` — sys.columns for tables | — | .25d |
| 16.5 | `sql/catalog/table_types.sql` — sys.table_types | — | .1d |
| 16.6 | `sql/audit/bootstrap.sql` — DDL for `__migrator.*` tables | — | .5d |
| 16.7 | `sql/audit/load_checksums.sql` — batched checksum SELECT | — | .25d |
| 16.8 | `sql/audit/insert_run.sql` | — | .1d |
| 16.9 | `sql/audit/insert_items.sql` | — | .1d |
| 16.10 | `sql/audit/insert_attempt.sql` | — | .1d |
| 16.11 | `sql/audit/update_run.sql` | — | .1d |
| 16.12 | `sql/lock/acquire.sql` — `sp_getapplock` | — | .1d |
| 16.13 | `sql/lock/release.sql` — `sp_releaseapplock` | — | .1d |

---

## Epic 17.0 — Legacy Deletion

**Depends on**: epic 14.0 compiles (engine works), all new packages exist
**When**: after engine passes tests but before feature work

Remove every file marked `remove` in audit.csv, merge `refactor` files into new packages.

Ref: `§681-691`

| ID | Task | Ref | Est. |
|---|---|---|---|
| 17.1 | Delete `internal/migrator/` — all 20+ files, redistributed to new packages | §685 | .5d |
| 17.2 | Delete `internal/parser/layout_selection.go` — selection logic in config | §687 | .1d |
| 17.3 | Delete `internal/planner/catalog_reader.go` — replaced by `db/` | §688 | .1d |
| 17.4 | Merge `internal/runreport/reports.go` → `report/` | §689 | .25d |
| 17.5 | Merge `internal/attempts/builders.go` → `audit/` | §690 | .25d |
| 17.6 | Merge `internal/checksum/` → `fs/` | §691 | .25d |
| 17.7 | Delete `internal/failure_helpers.go` — replaced by `errors/` | §686 | .1d |
| 17.8 | Update Build: remove old `internal/` dirs from build | — | .1d |
| 17.9 | Verify: `go build ./...` passes, all imports updated | — | .5d |

---

## Epic 18.0 — Integration Tests + Docker

**Depends on**: 15.0 app/ compiles (binary exists)
**Cross-cutting**: Docker infra already created (docker-compose.yml, Makefile, scripts/sql/create-test-db.sql)

| ID | Task | Ref | Est. |
|---|---|---|---|
| 18.1 | Write integration test: `TestPlan_WithRealDB_ReturnsCorrectPlan` — scans repo, connects to MSSQL, runs Plan | — | 1d |
| 18.2 | Write integration test: `TestMigrate_WithRealTableDrift_CreatesTransition` — creates table, detects drift, scaffolds | — | 1d |
| 18.3 | Write integration test: `TestMigrate_EndToEnd_RunMigrateTwice` — first run applies, second run skips everything | — | 1d |
| 18.4 | Write integration test: `TestLock_ConcurrentAccess_SecondFails` — two simultaneous migrates, lock timeout | — | 1d |
| 18.5 | Write integration test: `TestValidation_AfterMigrate_Passes` — migrate then validate | — | .5d |
| 18.6 | Add CI job (GitHub Actions): start docker, run tests, tear down | — | 1d |
| 18.7 | Document integration test prerequisites in runbook.md | — | .25d |

---

## Epic 19.0 — Documentation

**Cross-cutting**: continuous throughout implementation

| ID | Task | Ref | Est. |
|---|---|---|---|
| 19.1 | Update `docs/runbook.md` with new CLI flags, subcommands, exit codes | — | .5d |
| 19.2 | Document `_migrations/` directory convention in solution.md | — | .25d |
| 19.3 | Write `README.md` — what rmig does, quick start, architecture overview (2 paragraphs) | — | .5d |
| 19.4 | Document env vars in `operational-contract.md` | — | .25d |
| 19.5 | Verify all `.sql` templates have corresponding doc in spec | — | .5d |

---

## Summary

| Epic | Tasks | Est. (days) | Tests |
|---|---|---|---|
| 1.0 types/ | 12 | 3.6 | 1 |
| 2.0 errors/ | 9 | 3.5 | 1 |
| 3.0 driver/ | 6 | 1.6 | 1 |
| 4.0 bus/ | 7 | 1.6 | 3 |
| 5.0 log/ | 6 | 1.8 | 2 |
| 6.0 lock/ | 6 | 1.6 | 2 |
| 7.0 fs/ | 21 | 6.5 | 8 |
| 8.0 db/ | 16 | 5.0 | 6 |
| 9.0 audit/ | 17 | 6.5 | 5 |
| 10.0 diff/ | 12+ | 7.0 | 12 |
| 11.0 scaffold/ | 8+ | 4.0 | 5 |
| 12.0 apply/ | 7+ | 4.5 | 9 |
| 13.0 report/ | 7 | 2.0 | 3 |
| 14.0 engine/ | 9+ | 4.0 | 8 |
| 15.0 app/ | 9 | 3.0 | 3 |
| 16.0 sql/ | 13 | 2.5 | — |
| 17.0 legacy | 9 | 2.0 | — |
| 18.0 integration | 7 | 4.8 | 5 (integration) |
| 19.0 docs | 5 | 2.0 | — |
| **Total** | **~165** | **~65 days** | **~70 fast + ~50 slow** |
