# Performance harness

Lifecycle: `Current`.

## Scripts

| Script | Purpose |
|--------|---------|
| [`prod_gate.sh`](prod_gate.sh) | Incremental plan go/no-go vs baseline (`make test-prod-gate`) |
| [`cli_phase.sh`](cli_phase.sh) | Full CLI phase timings (`cold`, `warm`, `migrate-cold`, `profile`) |
| [`footprint_bench.sh`](footprint_bench.sh) | Struct sizes + diff bench baseline (`make bench-footprint`) |

## Environment

| Variable | Used by |
|----------|---------|
| `RMIG_RUN_SQLSERVER_INTEGRATION=1` | All integration perf tests |
| `RMIG_GATE_SKIP_DB_RESET=1` | Warm prod gate (no DROP/CREATE) |
| `RMIG_PHASE_SKIP_DB_RESET=1` | Warm full CLI plan (`cli_phase.sh warm`) |
| `RMIG_CLI_PHASE_REPORT` | Write phase JSON from CLI tests |
| `RMIG_GATE_REPORT` | Prod gate result JSON |
| `RMIG_FOOTPRINT_UPDATE_BASELINE` | Rewrite [`footprint_baseline.json`](../internal/app/testdata/perf/footprint_baseline.json) |
| `RMIG_FOOTPRINT_BENCH` | Run slow benchmark regression test in `internal/perf` |
| `RMIG_PLAN_DB_TRACE=1` | Append plan DB trace JSON (`artifacts/rust_plan_db_trace.json`) |
| `RMIG_PLAN_DB_MAX_PAR_MS` | Hard SLO for Rust `parallel_wall_ms` in workflow test (default **500**) |

**Git delta (prod):** no manual `RMIG_GATE_GIT_BASE` — use full clone / `fetch-depth: 0` per [`docs/ci-checkout.md`](../docs/ci-checkout.md).

**Catalog cache (phase 3):** on by default; set `RMIG_CATALOG_CACHE=0` only when tests must count exact SQL round-trips.

## Artifacts

`artifacts/*.json`, `*.prof`, `*.trace` are gitignored. Committed references:

- CLI phases: [`internal/app/testdata/cli_phase/plan_full_cli_reference.json`](../internal/app/testdata/cli_phase/plan_full_cli_reference.json)
- Footprint (struct sizes + diff benches): [`internal/app/testdata/perf/footprint_baseline.json`](../internal/app/testdata/perf/footprint_baseline.json)

### Footprint baseline (phase 0)

```bash
make bench-footprint              # bench output + struct size report
make bench-footprint-profile      # cpu/mem pprof for 5k diff (artifacts/)
make bench-footprint-update-baseline  # refresh committed JSON after intentional changes
```

Compare profiles after refactors:

```bash
go tool pprof -base ops/perf/artifacts/footprint_5k.cpu.prof ops/perf/artifacts/footprint_5k_after.cpu.prof
```

SQL wall baseline (integration): `make test-cli-phase-cold` / `ops/perf/cli_phase.sh profile` (cpu + mem + trace).

### CPU / heap profiles (pprof)

| Profile | Command | Files |
|---------|---------|-------|
| In-process diff 5k | `make bench-footprint-profile` | `artifacts/footprint_5k.cpu.prof`, `footprint_5k.mem.prof` |
| Full CLI plan (SQL) | `ops/perf/cli_phase.sh profile` (needs `make db-up`) | `artifacts/cli_plan.cpu.prof`, `cli_plan.mem.prof`, `cli_plan.trace` |
| Text summary | `make profile-summary` | `artifacts/profile_summary.txt` |

Interactive UI:

```bash
go tool pprof -http=:0 ops/perf/artifacts/footprint_5k.cpu.prof
go tool pprof -http=:0 -alloc_space ops/perf/artifacts/footprint_5k.mem.prof
```

**Note:** CLI CPU profile is mostly **idle waiting on SQL Server** (~10ms CPU samples on ~4s wall). Use phase JSON (`cli_phase_plan_cold.json`) for inspect wall; use **mem** profile for alloc regressions. In-process mem shows `diff.Compute` + scan checksums dominate on 5k layout bench.

## Go↔Rust e2e matrix

Orchestrator: [`go_rust_e2e_all.sh`](go_rust_e2e_all.sh). Rust compare harness: [`rust/crates/core/tests/go_rust_scenario_integration.rs`](../rust/crates/core/tests/go_rust_scenario_integration.rs).

| Make target | Scope |
|-------------|--------|
| `make go-rust-e2e` | Subset: `empty_db_plan` + `warm_db_plan` ([`go_rust_e2e.sh`](go_rust_e2e.sh)) |
| `make go-rust-e2e-all` | Full matrix below |
| `make check-e2e` | `go-rust-e2e-all` + `go-rust-io-debug` + `rust-workflow-fast` + `rust-slo` + `rust-prod-gate` |

**Requires:** Docker SQL (`make db-up`), `.temp/sql` fixture, `RMIG_RUN_SQLSERVER_INTEGRATION=1`. Database names come from top-level dirs under **`RM_SQL_ROOT`** (e.g. `dactests/` → SQL Server database `dactests`); created automatically if missing.

| Scenario ID | Go export test | Expected behavior |
|-------------|----------------|-------------------|
| `empty_db_plan` | `TestE2E_ExportScenarioReport` | 6× `create_object`; DB reset |
| `prod_gate_cold` | `TestE2E_ExportGateReport` | Gate GO vs [`plan_baseline_empty_db.json`](../internal/app/testdata/prod_gate/plan_baseline_empty_db.json); reuses cold DB after empty plan |
| `apply_smoke_result` | `TestE2E_ExportApplyReport` | Plan + `apply.Execute` on shared DB; Go publishes `run.finished` so audit flushes; Rust compare after DB reset (`applied=7`, `audit_object_rows≥6`) |
| `warm_db_plan` | `TestE2E_ExportScenarioReport` | 6× `adopt_existing`; no DB reset |
| `skip_unchanged_plan` | `TestE2E_ExportScenarioReport` | 6× `adopt_existing` (audit empty → not skip); `RMIG_IO_DEBUG_SKIP_L1_INVALIDATE` via scenario env |
| `catalog_cache_plan` | `TestE2E_ExportScenarioReport` | 6× `adopt_existing`; `RMIG_CATALOG_CACHE=1` |
| `blocked_table_plan` | `TestE2E_ExportBlockedMigrate` | Orchestrator resets DB; baseline migrate + git column + blocked migrate → exit **10**, scaffold file |

**Env (orchestrator sets unless noted):**

| Variable | Role |
|----------|------|
| `RMIG_E2E_SCENARIO` | Scenario ID |
| `RMIG_E2E_EXPORT_REPORT` | Go JSON output path |
| `RMIG_E2E_GO_REPORT` / `RMIG_E2E_RUST_REPORT` | Compare inputs / Rust output |
| `RMIG_GATE_SKIP_DB_RESET=1` | Skip Rust/Go DB reset (warm path) |
| `RMIG_CATALOG_CACHE=1` | Catalog cache scenario only |

Artifacts: `artifacts/go_e2e_*.json`, `artifacts/rust_e2e_*.json`, `artifacts/go_rust_e2e_all_report.txt`.

## Rust-only e2e (`make rust-e2e`)

Orchestrator: [`rust_e2e.sh`](rust_e2e.sh). No Go tests.

| Step | Harness | What it verifies |
|------|---------|------------------|
| 1 | [`apply_e2e_integration.rs`](../rust/crates/core/tests/apply_e2e_integration.rs) | Cold DB reset → `migrate` → `smoke` schema + objects in catalog → `azdo_deploy_meta.history` ≥ 6 object rows |
| 2 | [`workflow_integration.rs`](../rust/crates/core/tests/workflow_integration.rs) | Git commits: DDL column (blocked → migration → apply), view `UpdateExistingModule`, negative SQL; audit row count grows after each apply |

```bash
make db-up
make rust-e2e   # RM_SQL_ROOT=.temp/sql → database name from catalog dir (e.g. dactests)
```

## Workflow integration (Rust-only state tests)

Harness: [`rust/crates/core/tests/workflow_integration.rs`](../rust/crates/core/tests/workflow_integration.rs) — one test `workflow_git_scenarios_single_session`, asserts **DB + filesystem** after git commits on `.temp/sql`.

| Phase | Checks |
|-------|--------|
| 1 baseline | migrate from repo → `sys.objects` + audit |
| 2 DDL | commit column → blocked (10) → scaffold → commit migration → column in DB |
| 3 module | commit view change → `UpdateExistingModule` → column in view |
| 4 negative | broken view SQL → migrate fails → view unchanged |

Direct TDS (`RMIG_USE_RMIGD` unset). **One** DB reset, shared warm DB.

**Measured (test binary only, `release-fast`, warm compile):**

| Metric | Value |
|--------|-------|
| Wall | **~8–13 s** |
| Peak RSS | **~9 MB** |

`470 MB / 36 s` was `cargo test --release` (includes **2 min compile** with `debug=true` on release profile + 4× redundant baselines). Use `release-fast` and run the test binary directly to measure runtime.

```bash
make db-up
RMIG_RUN_SQLSERVER_INTEGRATION=1 cargo test --profile release-fast -p migrator-core --test workflow_integration -- --nocapture --test-threads=1

# Measure test only (after compile):
/usr/bin/time -l env RMIG_RUN_SQLSERVER_INTEGRATION=1 \
  rust/target/release-fast/deps/workflow_integration-* \
  workflow_git_scenarios_single_session --nocapture --test-threads=1
```

Profile `release-fast` in [`rust/Cargo.toml`](../rust/Cargo.toml): `inherits = "release"`, `debug = false`.

## Plan DB phase SLO (Rust `parallel_wall_ms`)

Target: **`parallel_wall_ms` ≤ 500 ms** on Docker SQL Server 2019 + `.temp/sql` smoke tree (plan inspect ‖ checksums batch in [`plan_batch.rs`](../rust/crates/core/src/db/plan_batch.rs)).

| Make target | Command |
|-------------|---------|
| `make rust-plan-db-perf` | [`rust_plan_db_perf.sh`](rust_plan_db_perf.sh) — workflow integration + trace JSON |

```bash
make rust-plan-db-perf
# or
RMIG_RUN_SQLSERVER_INTEGRATION=1 RMIG_PLAN_DB_TRACE=1 RMIG_PLAN_DB_MAX_PAR_MS=500 \
  cargo test --profile release-fast -p migrator-core --test workflow_integration -- --nocapture --test-threads=1
```

Trace artifact (gitignored): `artifacts/rust_plan_db_trace.json`.

### Path → round-trip budget (workflow, post-bootstrap)

| Phase | `path` | RT (`q`) | Typical `par` |
|-------|--------|----------|---------------|
| 1-baseline (cold, pre-bootstrapped DB) | `cold_full` | 1 | ~140 ms |
| 2-ddl-blocked (git delta + relaxed catalog cache) | `git_delta` | 1 | ~185 ms |
| 2-ddl-apply / 3-view-plan | `git_delta` | 1 | ~80–85 ms |
| 3-view-apply (L1 hit) | `cache_hit` | 0 | 0 ms |

Git-delta fast path: batched relaxed `catalog_cache` load + checksums; skip hot catalog SQL when cache covers delta keys (see [`catalog_cache_load_relaxed.sql`](../rust/sql/catalog/catalog_cache_load_relaxed.sql)). Post-apply [`save_workspace_snapshot`](../rust/crates/core/src/db/catalog_cache_save.rs) warms cache after baseline migrate.

Pre-bootstrap audit tables in test DB reset: [`db_reset.rs`](../rust/crates/core/tests/common/db_reset.rs) runs `BOOTSTRAP_TABLES` + `BOOTSTRAP_INDEX` after `CREATE DATABASE` so cold plan skips bootstrap DDL.

### Fast full workflow (dev iteration)

| Make target | Reset mode | Typical TOTAL |
|-------------|------------|---------------|
| `make rust-plan-db-perf` | DROP/CREATE + pre-bootstrap | **~6–7 s** |
| `make rust-workflow-fast` | Truncate smoke + audit (`RMIG_WORKFLOW_FAST_RESET=1`) | **~2 s** |

Fast reset keeps `azdo_deploy_meta` tables; drops `smoke` schema objects and clears audit/catalog rows. Same test assertions and plan DB SLO gate.
