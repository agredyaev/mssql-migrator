# Technical Document: Prod incremental go/no-go gate

Lifecycle: `Current`.

## Purpose

Define how operators and release pipelines decide **go/no-go** for `rmig` using **incremental plan analysis**: only SQL tree **changes** (delta) may differ from a committed baseline plan snapshot; unexpected diffs outside the delta fail the gate. This complements **full CLI** phase profiling (`make test-int-phase-cli`) and harness timings (`make test-int-phase`) with a **business-logic** check suitable for production promotion.

## Scope

- Gate logic: [`internal/prodgate/`](../internal/prodgate/) (`snapshot.go`, `delta.go`, `compare.go`, `gate.go`)
- Integration test: [`internal/app/prod_gate_integration_test.go`](../internal/app/prod_gate_integration_test.go) (`TestProdGate_IncrementalPlan`)
- Baseline artifact: [`internal/app/testdata/prod_gate/plan_baseline_empty_db.json`](../internal/app/testdata/prod_gate/plan_baseline_empty_db.json)
- Runner script: [`ops/perf/prod_gate.sh`](../ops/perf/prod_gate.sh)
- Makefile targets: `test-prod-gate`, `test-prod-gate-update-baseline`, `db-up` / `db-init`

## System context

The gate runs [`RunPlanPipeline`](../internal/app/plan_pipeline_integration.go) aligned with `engine.runPlan` (scan → **parallel** `EnsureTables` ‖ (`LoadChecksums` → scoped `InspectWithScope`) → diff) against SQL Server, builds a [`PlanSnapshot`](../internal/prodgate/snapshot.go), and compares it to a **baseline JSON**. Changed files (auto git delta or test override) map to **normalized object keys**; only those keys may differ from baseline when delta is non-empty. With **empty delta**, current plan must **match baseline exactly** (strict mode).

Database **drop/create** (`ensureTestDatabase`) is optional and **excluded** from plan wall SLO when `RMIG_GATE_SKIP_DB_RESET=1`.

## Interfaces and boundaries

### Inputs

| Input | Source |
|-------|--------|
| Baseline plan | `internal/app/testdata/prod_gate/plan_baseline_empty_db.json` |
| Delta paths | **Auto:** [`ResolveChangedPaths`](../internal/prodgate/changed_paths.go) — CI PR env or `git merge-base` (see [`docs/ci-checkout.md`](ci-checkout.md)). **Not used in prod:** `RMIG_GATE_CHANGED_FILES`, `RMIG_GATE_GIT_BASE` |
| Scoped inspect | [`engine.BuildInspectScope`](../internal/engine/inspect_scope.go) + [`db.InspectWithScope`](../internal/db/inspector_impl.go): hot keys hit catalog SQL; stable keys (file checksum == audit history, outside delta) are synthetic in state. Force full: `RM_SKIP_GIT=1`, `RMIG_INSPECT_FULL=1`, or no `.git` |
| SQL tree | `RM_SQL_ROOT` / `.temp/sql` in integration test |
| Database | `RM_*` connection vars (same as `make test-int`) |

### Outputs

| Output | Description |
|--------|-------------|
| Test pass/fail | `TestProdGate_IncrementalPlan` exit code |
| JSON report | `RMIG_GATE_REPORT` (default via script: `ops/perf/artifacts/prod_gate_report.json`; gitignored) |
| `t.Log` | Phase timings and `timingConn` DB boundary summary |

### Gate verdict (`internal/prodgate.Evaluate`)

- **NO-GO:** `plan.Blocked`, risky actions in delta (`fail`, `reprocess_changed_blocked`), plan changes **outside** delta keys (strict), optional **plan wall SLO** exceeded (`RMIG_GATE_MAX_PLAN_WALL_MS`)
- **GO:** otherwise

### Phase timing fields

| Field | Meaning |
|-------|---------|
| `inspect_ms` | Wall time of `InspectWithScope` after checksums (schemas + objects; **no** table columns) |
| `checksums_ms` | Wall time of `LoadChecksums` (same goroutine as inspect; runs before scope build) |
| `ensure_ms` | `audit.EnsureTables` when run inside the harness |
| `parallel_wall_ms` | Wall time of the inspect ‖ checksums join |
| `audit_ms` | `ensure_ms` + `checksums_ms` (summed; **not** parallel overlap) |
| `plan_wall_ms` | Scan through diff end-to-end |

## Plan-phase performance reference

### Gate harness (`make test-prod-gate`)

Measured on Docker SQL Server 2019 and `.temp/sql` smoke tree via `RunPlanPipeline` (`EnsureAudit: true`). **Not** a production SLA; use for regression comparison.

| Scenario | inspect_ms | plan_wall_ms | Notes |
|----------|------------|--------------|--------|
| Legacy baseline (pre-optimization, sequential inspect **with columns**) | 3608 | 3950 | Historical `ops/perf` sample |
| After optimization, cold DB (`DROP/CREATE`) | ~1281 | ~1664 | Lazy columns + parallel inspect ‖ checksums |
| After optimization, warm DB (`RMIG_GATE_SKIP_DB_RESET=1`) | ~100 | ~250 | Inspector/checksum caches hot |

### Full CLI (`make test-int-phase-cli`)

Canonical **prod-like** path: `runWithLookup` → `app.Run` (connect, subscribers, `engine.Plan` / `Migrate`; `EnsureTables` inside `engine.runPlan`) with `engine.PhaseObserver` and optional `RM_REPORT_DIR`. Tests: `TestIntegration_PhaseReport_CLI_Plan`, `TestIntegration_PhaseReport_CLI_Migrate` in [`phase_report_integration_test.go`](../internal/app/phase_report_integration_test.go).

| Source | Matches production `app.Run`? |
|--------|------------------------------|
| `engine.runPlan` | Yes (parallel inspect ‖ checksums) |
| `RunPlanPipeline` (gate / `test-int-phase`) | Almost (ensure inside harness; no bus/report) |
| `TestIntegration_PhaseReport_CLI_*` | **Yes** (full CLI) |

Additional CLI JSON fields: `connect_ms`, `ensure_ms`, `engine_ms`, `cli_wall_ms`, `report_write_ms` (migrate/plan with `RM_REPORT_DIR`), `apply_ms` / `audit_flush_ms` (migrate).

**Measured full CLI plan** (Docker SQL Server 2019, `.temp/sql`, `RM_SKIP_GIT=1`):

| Scenario | inspect_ms | cli_wall_ms | Notes |
|----------|------------|-------------|--------|
| Cold (`ops/perf/cli_phase.sh cold`) | ~570–650 | ~630–700 | Empty DB: one scoped-hit query; parallel inspect ‖ ensure+checksums |
| Warm (`RMIG_PHASE_SKIP_DB_RESET=1`, `cli_phase.sh warm`) | ~70–110 | ~170 | SQL plan + history probe hot |

Scripts: [`ops/perf/cli_phase.sh`](../ops/perf/cli_phase.sh) (`cold`, `warm`, `migrate-cold`, `profile`). Makefile: `make test-cli-phase-cold`, `make test-int-phase-cli-warm`. Reference JSON: [`internal/app/testdata/cli_phase/plan_full_cli_reference.json`](../internal/app/testdata/cli_phase/plan_full_cli_reference.json). Optional report path: `RMIG_CLI_PHASE_REPORT`.

**Profiling notes:** `-cpuprofile` on integration tests shows little CPU (work is I/O wait on SQL Server). `fetch_ms` in CLI timings (`timingRows`) is typically sub-ms; **inspect wall ≈ `Query` round-trip**, not `rows.Next` decode. Inspect runs **parallel schema + object** OpenJSON queries (since phase-2 follow-up).

Optional profiles: `ops/perf/cli_phase.sh profile` or `make test-int-phase-cli ARGS='-cpuprofile=… -trace=…'`.

Dominant cost before optimization was **column catalog** inside inspect; removing it from the default path is the largest win. OpenJSON scope uses **(schema, kind, object)** triples from layout.

**Inspect refactor (catalog):** `db.Inspect` uses one **`buildCatalogStateSQL`** round-trip (schemas + objects) instead of separate schema/object queries; **kind-filtered** CTEs skip `type_rows` / `index_rows` / `sys_object_rows` when those kinds are absent from the layout. **Audit/checksums:** `engine.runPlan` runs **`EnsureTables` + `LoadChecksums`** in parallel with inspect; empty history skips OpenJSON; history index is created on first migrate flush. Warm full CLI inspect is typically **&lt;200ms**; cold plan `cli_wall_ms` is typically **&lt;800ms** on Docker SQL 2019 + `.temp/sql`. Use `RMIG_PHASE_SKIP_DB_RESET=1` / `ops/perf/cli_phase.sh warm` for prod-like timings.

## Assumptions and constraints

- Baseline reflects a **known-good** plan on the reference fixture (empty DB + `.temp/sql` smoke tree). Updating baseline requires maintainer intent: `make test-prod-gate-update-baseline`.
- Incremental gate validates **plan business semantics**; use `make test-int-phase-cli` for full CLI wiring and subscriber timings.
- Delta mapping uses layout path indexes; transition-only paths map to transition keys.
- Git delta requires a `.git` directory at repo root (discovered from `SQLRoot`). No manual `RMIG_GATE_GIT_BASE` in production CI.
- On a feature branch, merge-base against `main` / `origin/main` defines changed paths; on `main` with no remote, delta may be empty (strict baseline match).
- SQL Server must support **OPENJSON** (see `docs/solution.md`).

## Nominal flow

1. `make db-up` (or existing SQL Server).
2. `make test-prod-gate` (or `ops/perf/prod_gate.sh`).
3. Test connects, runs plan pipeline, loads baseline, resolves delta paths → keys, calls `prodgate.Evaluate`.
4. On **GO**, pipeline may proceed; on **NO-GO**, inspect `RMIG_GATE_REPORT` and phase logs.

## Off-nominal behavior and failure containment

- **Missing baseline file:** test fails; run `make test-prod-gate-update-baseline` once after intentional fixture/plan contract change.
- **Unexpected change outside delta:** gate fails closed (strict); fix SQL or widen delta only if change is intentional.
- **SLO exceeded:** gate fails even if plan matches; tune SQL Server/network or raise SLO only with evidence.

## Verification and validation

- Unit tests: `go test ./internal/prodgate/...`
- Integration gate: `make test-prod-gate` with `RMIG_RUN_SQLSERVER_INTEGRATION=1` and Docker MSSQL
- Phase profiling (harness): `make test-int-phase`
- Phase profiling (full CLI): `make test-int-phase-cli`

## Operations and recovery

- **Refresh baseline:** `make test-prod-gate-update-baseline` after reviewed plan contract change; commit updated JSON under `internal/app/testdata/prod_gate/`.
- **PR check with delta:** run `make test-prod-gate` on a PR branch (CI auto-detect); local repro via temp git fixture or undocumented `RMIG_GATE_CHANGED_FILES`
- **Prod-like run (no DB recreate):** `RMIG_GATE_SKIP_DB_RESET=1 make test-prod-gate`

## Open issues and non-goals

- Open issues: incremental **execution** (apply only dirty objects) is not implemented; gate and plan path use incremental **inspect** (catalog SQL for hot keys only) and incremental **analysis** (delta vs baseline).
- Non-goals: automatic CI SLO thresholds without operator configuration; full-tree performance certification on every object in production repos.

## References

- [`docs/solution.md`](solution.md) — runtime profiling and `RM_*` flags
- [`docs/ci-checkout.md`](ci-checkout.md) — CI checkout requirements for git delta
- [`docs/operational-contract.md`](operational-contract.md)
- [`internal/app/phase_report_integration_test.go`](../internal/app/phase_report_integration_test.go)
