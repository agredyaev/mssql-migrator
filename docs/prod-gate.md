# Prod incremental go/no-go gate

Lifecycle: `Current`.

## Purpose

Define how operators and release pipelines decide **go/no-go** for `rmig` using **incremental plan analysis**: only SQL tree **changes** (delta) may differ from a committed baseline plan snapshot; unexpected diffs outside the delta fail the gate. Runner: **`make prod-gate`** (`crates/core/tests/prod_gate_integration.rs`).

## Scope

- Gate logic (Rust): [`crates/core/src/gate/`](../crates/core/src/gate/) - see `docs/specs/rust/module-gate.md`
- Makefile targets: `prod-gate`, `db-up` / `db-init`
- Runner script: [`ops/perf/prod_gate.sh`](../ops/perf/prod_gate.sh)

## System context

The gate runs `engine::run_command(Command::Plan, …)` (scan → parallel ensure ‖ checksums+inspect → diff) against SQL Server, builds a [`PlanSnapshot`](../crates/core/src/gate/snapshot.rs), and compares it to a **baseline JSON**. Changed files (auto git delta or test override) map to **normalized object keys**; only those keys may differ from baseline when delta is non-empty. With **empty delta**, current plan must **match baseline exactly** (strict mode).

Implementation: [`crates/core/src/gate/`](../crates/core/src/gate/), [`crates/core/tests/prod_gate_integration.rs`](../crates/core/tests/prod_gate_integration.rs).

Database **drop/create** is optional and **excluded** from plan wall SLO when `RMIG_GATE_SKIP_DB_RESET=1`.

## Interfaces and boundaries

### Inputs

| Input | Source |
|-------|--------|
| Baseline plan | `crates/core/tests/testdata/prod_gate/plan_baseline_empty_db.json` |
| Delta paths | **Auto:** [`gate::resolve_changed_paths`](../crates/core/src/gate/changed_paths.rs) - CI PR env or `git merge-base` (see [`docs/ci-checkout.md`](ci-checkout.md)). **Not used in prod:** `RMIG_GATE_CHANGED_FILES`, `RMIG_GATE_GIT_BASE` |
| Scoped inspect | [`plan::build_inspect_scope`](../crates/core/src/plan/scope_build.rs) + [`db::inspect_with_scope`](../crates/core/src/db/inspector.rs): hot keys hit catalog SQL; stable keys (file checksum == audit history, outside delta) are synthetic in state. Force full: `RM_SKIP_GIT=1`, `RMIG_INSPECT_FULL=1`, or no `.git` |
| SQL tree | `RM_SQL_ROOT` / `.temp/sql` in integration test |
| Database | `RM_*` connection vars (same as `make test-int`) |

### Outputs

| Output | Description |
|--------|-------------|
| Test pass/fail | `prod_gate_incremental_plan` exit code |
| JSON report | `RMIG_GATE_REPORT` (default via script: `ops/perf/artifacts/prod_gate_report.json`; gitignored) |
| `t.Log` | Phase timings and `timingConn` DB boundary summary |

### Gate verdict (`gate::evaluate_gate`)

- **NO-GO:** `plan.Blocked`, risky actions in delta (`fail`, `reprocess_changed_blocked`), plan changes **outside** delta keys (strict), optional **plan wall SLO** exceeded (`RMIG_GATE_MAX_PLAN_WALL_MS`)
- **GO:** otherwise

### Phase timing fields

| Field | Meaning |
|-------|---------|
| `inspect_ms` | Wall time of `InspectWithScope` after checksums (schemas + objects; **no** table columns) |
| `checksums_ms` | Wall time of checksum load (runs before scope build in plan DB batch) |
| `ensure_ms` | `audit::ensure_tables` when run inside the harness |
| `parallel_wall_ms` | Wall time of the inspect ‖ checksums join (Rust: [`plan_batch.rs`](../crates/core/src/db/plan_batch.rs) through catalog save boundary) |
| `audit_ms` | `ensure_ms` + `checksums_ms` (summed; **not** parallel overlap) |
| `plan_wall_ms` | Scan through diff end-to-end |

### Rust plan DB SLO (`parallel_wall_ms`)

Integration gate: [`crates/core/tests/workflow_integration.rs`](../crates/core/tests/workflow_integration.rs) asserts `parallel_wall_ms ≤ RMIG_PLAN_DB_MAX_PAR_MS` (default **500**) after each workflow phase (L1 hits exempt).

| Variable | Default | Role |
|----------|---------|------|
| `RMIG_PLAN_DB_MAX_PAR_MS` | 500 | Hard ceiling for plan DB parallel wall |
| `RMIG_PLAN_DB_TRACE=1` | off | Append per-phase trace to `ops/perf/artifacts/plan_db_trace.json` |

Runner: `make plan-db-perf` ([`ops/perf/plan_db_perf.sh`](../ops/perf/plan_db_perf.sh)). See [`ops/perf/README.md`](../ops/perf/README.md) for path → RT table and measured workflow timings on Docker SQL 2019 + `.temp/sql`.

## Plan-phase performance reference

### Gate harness (`make prod-gate`)

Measured on Docker SQL Server 2019 and `.temp/sql` smoke tree via `engine::run_command(Plan)`. **Not** a production SLA; use for regression comparison.

| Scenario | inspect_ms | plan_wall_ms | Notes |
|----------|------------|--------------|--------|
| Cold DB (`DROP/CREATE`) | ~1281 | ~1664 | Lazy columns + parallel inspect ‖ checksums |
| Warm DB (`RMIG_GATE_SKIP_DB_RESET=1`) | ~100 | ~250 | Inspector/checksum caches hot |

### Full CLI SLO (`make slo`)

Canonical **prod-like** path: release `rmig` with `RMIG_USE_RMIGD=1` (harness spawns `rmigd`, sets `RMIG_SESSION`). Runner: [`ops/perf/cli_phase.sh`](../ops/perf/cli_phase.sh). Integration test: [`integration_plan.rs`](../crates/core/tests/integration_plan.rs) asserts `cli_wall_ms < RMIG_SLO_MAX_CLI_WALL_MS` (default 150).

Reference timings JSON: [`crates/core/tests/testdata/cli_phase/plan_full_cli_reference.json`](../crates/core/tests/testdata/cli_phase/plan_full_cli_reference.json). Optional report: `RMIG_CLI_PHASE_REPORT` (default under `ops/perf/artifacts/`).

| Scenario | inspect_ms | cli_wall_ms | Notes |
|----------|------------|-------------|--------|
| Warm SLO gate | ~70–110 | &lt;150 | `make slo` with L1 + session reuse |
| Cold plan (no session) | ~570–650 | ~630–700 | Historical reference sample |

Dominant cost before optimization was **column catalog** inside inspect; removing it from the default path is the largest win. Inspect runs **parallel schema + object** OpenJSON queries on cache miss.

## Assumptions and constraints

- Baseline reflects a **known-good** plan on the reference fixture (empty DB + `.temp/sql` smoke tree). Updating baseline: `RMIG_GATE_UPDATE_BASELINE=1 make prod-gate`.
- Incremental gate validates **plan business semantics**; use `make slo` for full CLI wall-time SLO.
- Delta mapping uses layout path indexes; transition-only paths map to transition keys.
- Git delta requires a `.git` directory at repo root (discovered from `SQLRoot`). No manual `RMIG_GATE_GIT_BASE` in production CI.
- On a feature branch, merge-base against `main` / `origin/main` defines changed paths; on `main` with no remote, delta may be empty (strict baseline match).
- SQL Server must support **OPENJSON** (see `docs/solution.md`).

## Nominal flow

1. `make db-up` (or existing SQL Server).
2. `make prod-gate` (or `ops/perf/prod_gate.sh`).
3. Test connects, runs plan pipeline, loads baseline, resolves delta paths → keys, calls `evaluate_gate`.
4. On **GO**, pipeline may proceed; on **NO-GO**, inspect `RMIG_GATE_REPORT` and phase logs.

## Off-nominal behavior and failure containment

- **Missing baseline file:** test fails; run `RMIG_GATE_UPDATE_BASELINE=1 make prod-gate` once after intentional fixture/plan contract change.
- **Unexpected change outside delta:** gate fails closed (strict); fix SQL or widen delta only if change is intentional.
- **SLO exceeded:** gate fails even if plan matches; tune SQL Server/network or raise SLO only with evidence.

## Verification and validation

- Unit tests: `cargo test -p migrator-core --test gate_snapshot_test --test changed_paths_test --test golden_baseline_test`
- Integration gate: `make prod-gate` with Docker MSSQL
- CLI SLO: `make slo`
- Plan DB perf: `make plan-db-perf`

## Operations and recovery

- **Refresh baseline:** `RMIG_GATE_UPDATE_BASELINE=1 make prod-gate` after reviewed plan contract change; commit updated JSON under `crates/core/tests/testdata/prod_gate/`.
- **PR check with delta:** run `make prod-gate` on a PR branch (CI auto-detect); local repro via temp git fixture or undocumented `RMIG_GATE_CHANGED_FILES`
- **Prod-like run (no DB recreate):** `RMIG_GATE_SKIP_DB_RESET=1 make prod-gate`

## Open issues and non-goals

- Open issues: incremental **execution** (apply only dirty objects) is not implemented; gate and plan path use incremental **inspect** (catalog SQL for hot keys only) and incremental **analysis** (delta vs baseline).
- Non-goals: automatic CI SLO thresholds without operator configuration; full-tree performance certification on every object in production repos.

## References

- [`docs/solution.md`](solution.md) - runtime profiling and `RM_*` flags
- [`docs/ci-checkout.md`](ci-checkout.md) - CI checkout requirements for git delta
- [`docs/operational-contract.md`](operational-contract.md)
- [`docs/specs/rust/module-gate.md`](specs/rust/module-gate.md)
- [`crates/core/tests/integration_plan.rs`](../crates/core/tests/integration_plan.rs)
