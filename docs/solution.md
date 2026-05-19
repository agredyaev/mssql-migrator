# Technical Document: Solution

Lifecycle: `Current`.

## Purpose

Describe the **implemented** `rmig` solution: how the CLI loads configuration, how `internal/engine` orchestrates scan → inspect → diff → (optional) apply, and where artifacts are written. This document is the product-level companion to **`docs/specs/internals/README.md`** (per-package specs).

## Scope

- CLI entry and wiring: `cmd/rmig/main.go`, `internal/app/app.go`, `internal/app/flags.go`, `internal/app/config.go`, `internal/app/wire.go`
- Version string for **`rmig version`:** `internal/buildinfo/buildinfo.go`, repository root **`VERSION`**, `Makefile` `release-build`
- Orchestration: `internal/engine/engine.go`
- Repository scan and layout: `internal/fs/scanner.go`, `internal/fs/layout.go`
- Planning: `internal/diff/diff.go`
- Catalog inspection: `internal/db/inspector_impl.go` (and embedded SQL under `internal/db/sql/`)
- Execution: `internal/apply/apply.go`
- Metadata and audit: `internal/audit/load.go`, `internal/audit/subscriber.go`
- Events and reports: `internal/bus`, `internal/report/report.go`
- Database access boundary: `internal/driver/conn.go`, `internal/driver/mssql`

## System context

`rmig` is a Go binary built from `cmd/rmig`. `main` passes a `driver.Conn` factory into `internal/app.Run`, which parses CLI flags, then either handles **`version`** immediately (no env file, no connect) or loads a dotenv-style file, builds `types.Config`, connects to SQL Server, wires `bus.EventBus`, audit/report subscribers, and `engine.Engine`, then runs one command: `plan`, `migrate`, `validate`, `baseline`, or `repair-checksum`.

Schema and object scope come from the SQL tree rooted at **`RM_SQL_ROOT`** (see `(*engine.Engine).runPlan`, which calls `Scanner.Scan(ctx, cfg.SQLRoot)`).

## Interfaces and boundaries

### CLI (`internal/app/flags.go`)

- **Usage:** `rmig [--env <path>] [--json] <command>`
- **`--env <path>`:** path to a dotenv-style file (key=value per line). If the flag is omitted, `internal/app/app.go` still attempts to load the default file named **`.env`** in the current working directory when present **for all commands except `version`**. The **`version`** command returns immediately after `parseFlags` and does **not** read `.env` or call `validateConfig`, even if `--env` is present.
- **`--json`:** sets JSON structured logs (`cfg.JSONLogs`) for engine commands; for **`version`**, it selects a single JSON object on stdout with `version` and `commit` keys instead of the default one-line text. It does **not** select machine-readable plan output on stdout (there is no `plan --json` flag today).
- **`RM_SKIP_GIT=1`:** skips git metadata preload during scan and omits git fields from plans (`internal/fs`, `internal/diff`).
- **`RM_REPORT_SYNC=1`:** fsyncs `.plan.json` / `.report.json` after write (default: flush only).

### Version metadata (`internal/buildinfo`)

- **`rmig version`** prints **`rmig <semver> <short-commit>`** to stdout (defaults `0.0.0-dev` / `unknown` when built with plain `go build` without `-X`). If `Commit` was not set at link time, `internal/buildinfo` may shorten **`vcs.revision`** from `runtime/debug.ReadBuildInfo` when the binary was built with VCS metadata.
- **`make release-build`** sets **`internal/buildinfo.Version`** from the repository root **`VERSION`** file and **`internal/buildinfo.Commit`** from `git rev-parse --short HEAD` via `-ldflags -X` (see `Makefile`).

### Configuration (`internal/app/config.go`)

Supported keys are read from the env file first, then `os.LookupEnv` for any key not set in the file (see `buildConfig`). **`validateConfig` requires `RM_DB_SERVER`, `RM_DB_DATABASE`, `RM_SQL_ROOT`, and `RM_SQL_BASE`** before a non-`version` command runs. Other variables (`RM_GIT_COMMIT`, `RM_REPORT_DIR`, …) are optional at validation time unless a subsystem needs them at runtime.

### Reports (`internal/report/report.go`)

When **`RM_REPORT_DIR`** is non-empty, the report subscriber writes under that directory:

- **`.plan.json`** — deferred to `EventRunFinished` (plan stashed on `EventDiffComputed`; same run as `.report.json`).
- **`.report.json`** — on `EventRunFinished` (payload contains `*types.RunFinished`).

There is **no** separate `migration-plan.txt` / `migration-report.json` writer in the current tree; filenames are fixed as above.

### Commands (`internal/engine/engine.go`)

The engine implements **`plan`**, **`migrate`**, **`validate`**, **`baseline`**, and **`repair-checksum`**. The **`version`** command is implemented only in **`internal/app/app.go`** (early return; no engine).

- **`plan`:** `runPlan` → publish `EventDiffComputed` → `EventRunFinished` (success).
- **`migrate`:** `runPlan`; if `plan.Blocked`, `scaffold.Ensure` then return `errors.ErrPlanBlocked`; else acquire session lock, `filterAppliedMigrations`, `applier.Execute`, then finish.
- **`validate`:** same planning pipeline as `plan` (not execution of `layout.Checks` SQL); publishes `ModulesRefreshed` from changed-object count.
- **`baseline`** and **`repair-checksum`:** `executeLocked` — same lock + `applier.Execute` path with command name `baseline` or `repair` in bus payloads (see code for exact `RunFinished.Command` values).

## Assumptions and constraints

- SQL Server is the only supported database; the concrete driver is `internal/driver/mssql`. Catalog inspect and audit checksum/history writes require **OPENJSON** (SQL Server 2016+ / compatibility level 130+).
- `RM_DB_AUTH` follows `internal/types/config.go` (`sql` vs `integrated`).
- Durations `RM_COMMAND_TIMEOUT`, `RM_SCRIPT_TIMEOUT`, `RM_LOCK_TIMEOUT` are parsed with `time.ParseDuration`; invalid values leave the corresponding `time.Duration` at zero (see `buildConfig`).
- **`RM_PLAN_FILE`** and **`RM_REPAIR_SCRIPT`** are read into `types.Config.PlanFile` and `types.Config.RepairTarget` (`internal/app/config.go`). **`internal/engine`** and **`internal/apply`** do not consult these fields yet; they are reserved for a future approved-plan / repair-target gate.

## Nominal flow

1. Operator sets `RM_*` in the process environment or in the file passed to `--env`.
2. Operator runs `rmig --env /path/to/.env plan` (or another command).
3. `engine.runPlan`: `fs.Scanner.Scan` → **`db.Inspect`** in parallel with **`audit.EnsureTables` + `LoadChecksums`** (ensure and checksums share one goroutine) → `diff.Compute`. Empty history skips OpenJSON checksum load; audit index is deferred until migrate flush. Table columns load only when a blocked migrate needs scaffold (`db.Inspector.LoadTableColumns`).
4. Subscribers react to bus events; if `RM_REPORT_DIR` is set, `.plan.json` / `.report.json` are updated as described above.
5. `internal/log` writes human-readable lines to stderr (or JSON when `--json`).

## Off-nominal behavior and failure containment

- Config parse errors and missing required `RM_*` keys (`validateConfig`): stderr message, non-zero exit before connect.
- Connect failure: stderr message, non-zero exit (`internal/errors` exit mapping).
- Engine failures: `publishRunFailed` emits `EventRunFinished` with `Result: failure` before returning the error to `app.Run` (see `engine.go`).

## Verification and validation

- `make check` (`Makefile`)
- `rmig version` and `make release-build && ./bin/rmig version` (semver from `VERSION`, commit from `git` at link time)
- SQL Server–backed tests: `make test-int` (`internal/app/integration_test.go`, build tag `integration`)
- **Prod incremental go/no-go:** `make test-prod-gate` — [`docs/prod-gate.md`](prod-gate.md), [`internal/prodgate/`](../internal/prodgate/), baseline [`internal/app/testdata/prod_gate/plan_baseline_empty_db.json`](../internal/app/testdata/prod_gate/plan_baseline_empty_db.json)
- **Phase timings (plan harness):** `make test-int-phase` — shared [`RunPlanPipeline`](../internal/app/plan_pipeline_integration.go) (`TestIntegration_PhaseReport_PlanPipeline`).
- **Phase timings (full CLI, prod-like):** `make test-int-phase-cli` — `runWithLookup` + `engine.PhaseObserver` (`TestIntegration_PhaseReport_CLI_Plan` / `_Migrate`); optional `ARGS='-cpuprofile=… -trace=…'`.

### Runtime profiling (integration)

**Purpose:** Repeatable view of **where wall time goes** (connect, ensure, scan, inspect, checksums, diff, apply, report write, audit flush) vs time spent inside **`driver.Conn` methods** (`Query*`, `ExecContext`, `Ping`) against SQL Server.

**How it works:**

- **Harness:** `timingConn` + [`RunPlanPipeline`](../internal/app/plan_pipeline_integration.go) (parallel inspect ‖ checksums; matches `engine.runPlan`).
- **Full CLI:** same `timingConn` around `mssql.Open`, `enableIntegrationPhaseTrace` wires `engine.PhaseObserver`, report flush observer, and audit flush observer during `app.Run`.

**Limitation:** `timingConn` attributes **Query return**, **Rows.Next/Scan** (`fetch_ms`), **Exec**, and **Ping** separately. On smoke fixtures `fetch_ms` is sub-ms; inspect cost is **SQL Server catalog query** wall time. CPU profiles are mostly idle; use phase JSON + `ops/perf/cli_phase.sh` for regression.

**Scan sub-phases** (when integration trace enabled): `scan_walk_ms`, `scan_git_ms`, `scan_checksums_ms` via `fs.Scanner.OnPhase` (`RM_SKIP_GIT=1` in CLI tests zeros git).

**How to run:** `make db-up`, then `make test-int-phase-cli` (canonical) or `make test-int-phase`; analyze with `go tool pprof` / `go tool trace` when profiles are enabled. CLI profile mode also writes **memprofile:** `ops/perf/cli_phase.sh profile`.

**Validated by:** `go test -tags=integration ./internal/app/ -run TestIntegration_PhaseReport -count=1` with `RMIG_RUN_SQLSERVER_INTEGRATION=1` (see [`Makefile`](Makefile)).

### Footprint baseline (in-process, phase 0)

**Purpose:** Committed reference for **struct sizes** (types ≥40 B) and **diff.Compute** bench (500 / 5000 objects) before DOD refactors.

### Scoped inspect (phase 1)

**Purpose:** Reduce catalog SQL on unchanged objects using **git delta** + **audit checksums**.

| Component | Path |
|-----------|------|
| Git delta | [`internal/prodgate/changed_paths.go`](../internal/prodgate/changed_paths.go) — CI auto-detect; merge-base fallback |
| Delta → keys | [`internal/prodgate/delta.go`](../internal/prodgate/delta.go), [`closure.go`](../internal/prodgate/closure.go) |
| Scope build | [`internal/plan/scope.go`](../internal/plan/scope.go), [`internal/engine/inspect_scope.go`](../internal/engine/inspect_scope.go) |
| Catalog | [`internal/db/inspector_impl.go`](../internal/db/inspector_impl.go) — `readStateScoped` |

**Force full inspect:** `RM_SKIP_GIT=1` (scan + inspect), `RMIG_INSPECT_FULL=1`, or no `.git` at repo root.

**Validated by:** `go test ./internal/prodgate/... ./internal/plan/...`; prod gate uses same delta resolver as plan pipeline ([`docs/prod-gate.md`](prod-gate.md)).

### Persistent catalog cache (phase 3)

**Purpose:** On warm databases, skip catalog OPENJSON when layout digest matches the last persisted snapshot in `azdo_deploy_meta.catalog_cache` / `catalog_meta`.

| Piece | Path |
|-------|------|
| Tables | [`internal/audit/sql/bootstrap_tables.sql`](../internal/audit/sql/bootstrap_tables.sql) |
| Load/save | [`internal/db/catalog_cache.go`](../internal/db/catalog_cache.go) |
| Invalidate | `db.InvalidateInspectorCache` (apply) clears in-process + SQL cache |

**Defaults:** cache **on**. Disable for SQL round-trip tests: `RMIG_CATALOG_CACHE=0`. Optional drift sampling: `RMIG_CATALOG_SPOTCHECK=N` re-checks `N` stable keys per plan.

**Validated by:** `go test ./internal/db/...`; integration with Docker + warm `cli_phase.sh warm`.

### DOD layout footprint (phase 4)

**Purpose:** Shrink hot layout structs and deduplicate metadata strings before diff/inspect.

| Change | Path |
|--------|------|
| `CachedFile` on heap (`Object.File *CachedFile`) | [`internal/fs/layout.go`](../internal/fs/layout.go) — `fs.Object` **408 B → 176 B** |
| String interning at scan | [`internal/fs/arena.go`](../internal/fs/arena.go), `RebuildPathIndexes` |
| Dense object index | [`internal/fs/store.go`](../internal/fs/store.go) — `ObjectStore`, `objectRow` |

**Benchmarks (darwin/arm64, `make bench-footprint`):** compare [`ops/perf/artifacts/footprint_phase4_before.json`](../ops/perf/artifacts/footprint_phase4_before.json) vs committed [`footprint_baseline.json`](../internal/app/testdata/perf/footprint_baseline.json). `diff.Compute` alloc/op is dominated by `[]PlannedObject` output (~240 B × N); struct shrink improves scan/RSS, not yet plan slice alloc.

**Commands:**

```bash
make bench-footprint
make bench-footprint-profile
make bench-footprint-update-baseline   # after intentional perf contract change
```

| Command | Output |
|---------|--------|
| `make bench-footprint` | `ops/perf/artifacts/footprint_bench.txt` + struct log |
| `make bench-footprint-profile` | `footprint_5k.cpu.prof`, `footprint_5k.mem.prof` |
| `make bench-footprint-update-baseline` | [`internal/app/testdata/perf/footprint_baseline.json`](../internal/app/testdata/perf/footprint_baseline.json) |

**Package:** [`internal/perf/`](../internal/perf/). Regression: `TestFootprintBaselineMatch` in `go test ./...`; slow bench regression: `RMIG_FOOTPRINT_BENCH=1`. See [`ops/perf/README.md`](../ops/perf/README.md).

**Dependencies:** Docker SQL Server per [`docker-compose.yml`](docker-compose.yml); `.temp/sql` smoke tree; `RM_SKIP_GIT=1` in CLI phase tests.

**Does not cover:** CI perf thresholds; not a substitute for SQL Server tuning (indexes, waits).

## Operations and recovery

- After a failed run, use **`docs/runbook.md`** with stderr, logs, and (if configured) `.plan.json` / `.report.json` under `RM_REPORT_DIR`.

## Open issues and non-goals

- Open issues: none related to `rmig version` (implemented in `internal/app/app.go` with metadata in `internal/buildinfo`).
- Non-goals: this document does not describe CI/CD outside this repository.

## References

- `README.md`
- `docs/specs/internals/README.md`
- `docs/operational-contract.md`
- `docs/runbook.md`
