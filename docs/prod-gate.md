# Technical Document: Prod incremental go/no-go gate

Lifecycle: `Current`.

## Purpose

Define how operators and release pipelines decide **go/no-go** for `rmig` using **incremental plan analysis**: only SQL tree **changes** (delta) may differ from a committed baseline plan snapshot; unexpected diffs outside the delta fail the gate. This complements full runtime profiling (`make test-int-phase`) with a **business-logic** check suitable for production promotion.

## Scope

- Gate logic: [`internal/prodgate/`](../internal/prodgate/) (`snapshot.go`, `delta.go`, `compare.go`, `gate.go`)
- Integration test: [`internal/app/prod_gate_integration_test.go`](../internal/app/prod_gate_integration_test.go) (`TestProdGate_IncrementalPlan`)
- Baseline artifact: [`internal/app/testdata/prod_gate/plan_baseline_empty_db.json`](../internal/app/testdata/prod_gate/plan_baseline_empty_db.json)
- Runner script: [`ops/perf/prod_gate.sh`](../ops/perf/prod_gate.sh)
- Makefile targets: `test-prod-gate`, `test-prod-gate-update-baseline`, `db-up` / `db-init`

## System context

The gate runs the **plan pipeline** aligned with `engine.runPlan` (scan → `EnsureTables` → **parallel** `Inspect` + `LoadChecksums` → diff) against SQL Server, builds a [`PlanSnapshot`](../internal/prodgate/snapshot.go), and compares it to a **baseline JSON**. Changed files (from git or env) map to **normalized object keys**; only those keys may differ from baseline when delta is non-empty. With **empty delta**, current plan must **match baseline exactly** (strict mode).

Database **drop/create** (`ensureTestDatabase`) is optional and **excluded** from plan wall SLO when `RMIG_GATE_SKIP_DB_RESET=1`.

## Interfaces and boundaries

### Inputs

| Input | Source |
|-------|--------|
| Baseline plan | `internal/app/testdata/prod_gate/plan_baseline_empty_db.json` |
| Delta paths | `RMIG_GATE_CHANGED_FILES` (comma-separated) **or** `git diff --name-only $RMIG_GATE_GIT_BASE HEAD` |
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
| `inspect_ms` | Wall time of `db.Inspect` goroutine (schemas + objects; **no** table columns) |
| `audit_ms` | `EnsureTables` + wall time of `LoadChecksums` goroutine (summed, not overlap) |
| `plan_wall_ms` | Scan through diff end-to-end |

## Plan-phase performance reference

Measured with `make test-prod-gate` on Docker SQL Server 2019 and `.temp/sql` smoke tree (integration harness in `runPlanPipelineForGate`). **Not** a production SLA; use for regression comparison.

| Scenario | inspect_ms | plan_wall_ms | Notes |
|----------|------------|--------------|--------|
| Legacy baseline (pre-optimization, sequential inspect **with columns**) | 3608 | 3950 | Historical `ops/perf` sample |
| After optimization, cold DB (`DROP/CREATE`) | ~1281 | ~1664 | Lazy columns + parallel inspect ‖ checksums |
| After optimization, warm DB (`RMIG_GATE_SKIP_DB_RESET=1`) | ~100 | ~250 | Inspector/checksum caches hot |

Dominant cost before optimization was **column catalog** inside inspect; removing it from the default path is the largest win.

## Assumptions and constraints

- Baseline reflects a **known-good** plan on the reference fixture (empty DB + `.temp/sql` smoke tree). Updating baseline requires maintainer intent: `make test-prod-gate-update-baseline`.
- Incremental gate validates **plan business semantics**, not full `app.Run` CLI wiring.
- Delta mapping uses layout path indexes; transition-only paths map to transition keys.
- Git delta requires a git checkout at repo root when using `RMIG_GATE_GIT_BASE`.
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
- Phase profiling (optional): `make test-int-phase`

## Operations and recovery

- **Refresh baseline:** `make test-prod-gate-update-baseline` after reviewed plan contract change; commit updated JSON under `internal/app/testdata/prod_gate/`.
- **PR check with delta:** `RMIG_GATE_GIT_BASE=origin/main make test-prod-gate`
- **Prod-like run (no DB recreate):** `RMIG_GATE_SKIP_DB_RESET=1 make test-prod-gate`

## Open issues and non-goals

- Open issues: incremental **execution** (scan/inspect only dirty objects) is not implemented; gate is incremental **analysis** only.
- Non-goals: automatic CI SLO thresholds without operator configuration; full-tree performance certification on every object in production repos.

## References

- [`docs/solution.md`](solution.md) — runtime profiling and `RM_*` flags
- [`docs/operational-contract.md`](operational-contract.md)
- [`internal/app/phase_report_integration_test.go`](../internal/app/phase_report_integration_test.go)
