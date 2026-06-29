# Performance harness

Lifecycle: `Current`.

## Makefile targets

| Target | Script | Purpose |
|--------|--------|---------|
| `make e2e` | [`e2e.sh`](e2e.sh) | Plan subset: `empty_db_plan` + `warm_db_plan` |
| `make e2e-all` | [`e2e_all.sh`](e2e_all.sh) | Full scenario matrix vs baselines |
| `make e2e-timings` | [`e2e_timings.py`](e2e_timings.py) | Baseline vs run phase report |
| `make integration` | [`integration.sh`](integration.sh) | Apply + git workflow integration |
| `make slo` | [`cli_phase.sh`](cli_phase.sh) | Warm `cli_wall_ms` SLO |
| `make prod-gate` | [`prod_gate.sh`](prod_gate.sh) | Incremental plan go/no-go |
| `make plan-db-perf` | [`plan_db_perf.sh`](plan_db_perf.sh) | Plan DB trace + `parallel_wall_ms` |
| `make workflow-fast` | [`workflow_fast.sh`](workflow_fast.sh) | Full git workflow (~2 s reset) |
| `make bench-footprint` | [`footprint_bench.sh`](footprint_bench.sh) | Struct sizes + diff bench |
| `make bench-footprint-profile` | same | CPU flamegraph (5k diff) |
| `make bench-footprint-alloc` | same | dhat alloc tree via [`dhat_alloc_tree.py`](dhat_alloc_tree.py) |
| `make check-e2e` | Makefile | `e2e-all` + `workflow-fast` + `slo` + `prod-gate` |

**Requires:** Docker SQL (`make db-up`), fixture `.temp/sql`, `RMIG_RUN_SQLSERVER_INTEGRATION=1`. Shared env defaults: [`e2e_env.sh`](e2e_env.sh) (sources `docker-compose.yml` credentials). Default catalog database is discovered from the sole top-level dir under `RM_SQL_ROOT`; set `RM_DB_DATABASE` to override shell-side `DROP/CREATE` only.

## E2e scenario matrix (`make e2e-all`)

Baselines: [`crates/core/tests/testdata/e2e/`](../crates/core/tests/testdata/e2e/). Harness: [`scenario_e2e_integration.rs`](../crates/core/tests/scenario_e2e_integration.rs).

| Scenario | Behavior |
|----------|----------|
| `empty_db_plan` | 6x `create_object`; DB reset |
| `prod_gate_cold` | Gate GO vs [`plan_baseline_empty_db.json`](../crates/core/tests/testdata/prod_gate/plan_baseline_empty_db.json) |
| `warm_db_plan` | 6x `skip_unchanged` after apply setup |
| `skip_unchanged_plan` | unchanged adopt path |
| `catalog_cache_plan` | `RMIG_CATALOG_CACHE=1` |
| `blocked_table_plan` | exit **10**, scaffold file |
| `apply_smoke_result` | cold apply; DB reset; `audit_migration_rows=0` |
| `ddl_transition_apply` | blocked DDL → transition migrate; `audit_migration_rows=1`, catalog meta/cache filled; **last step** — leaves migration row in `history` |

Run artifacts: `artifacts/e2e_<scenario>.json`. Report: `artifacts/e2e_all_report.txt`.

After `make e2e-all`, inspect `azdo_deploy_meta.history`: expect `kind=migration`, `event=applied`, key under `*/_migrations/*.sql`.

## Integration (`make integration`)

| Step | Test | Verifies |
|------|------|----------|
| 1 | `apply_e2e_integration.rs` | Cold migrate, catalog + audit history |
| 2 | `workflow_integration.rs` | Git DDL, blocked migrate, view update |

## Footprint

Committed baseline: [`crates/core/tests/testdata/perf/footprint_baseline.json`](../crates/core/tests/testdata/perf/footprint_baseline.json).

```bash
make bench-footprint
make bench-footprint-alloc
make bench-footprint-update-baseline   # after intentional layout change
make profile-summary
# phase profilers (scan_root, L1 serde) — CPU flamegraph + dhat:
ops/perf/footprint_bench.sh profile-load-scan
ops/perf/footprint_bench.sh profile-load-cache
ops/perf/footprint_bench.sh alloc scan_root
ops/perf/footprint_bench.sh alloc cache
```

Runbook: [`docs/perf-footprint-audit.md`](../docs/perf-footprint-audit.md).

## Environment (common)

| Variable | Role |
|----------|------|
| `RMIG_RUN_SQLSERVER_INTEGRATION=1` | Enable SQL integration tests |
| `RM_DB_SERVER`, `RM_DB_USER`, `RM_DB_PASSWORD` | SQL Server connection (defaults in [`e2e_env.sh`](e2e_env.sh)) |
| `RM_DB_DATABASE` | Override catalog DB name for shell `DROP/CREATE`; unset → discover sole dir under `RM_SQL_ROOT` |
| `RM_SQL_ROOT` | Catalog tree root (default `$REPO/.temp/sql`) |
| `RMIG_GATE_SKIP_DB_RESET=1` | Skip DROP/CREATE between scenarios |
| `RMIG_PLAN_DB_TRACE=1` | Write `artifacts/plan_db_trace.json` |
| `RMIG_PLAN_DB_MAX_PAR_MS` | `parallel_wall_ms` SLO in workflow (default 500) |
| `RMIG_E2E_SCENARIO` | Scenario id for e2e harness |
| `RMIG_E2E_BASELINE_REPORT` | Override baseline JSON path |
| `RMIG_E2E_REPORT` | Write run report JSON under `artifacts/` |

## Artifacts (generated)

| File | Producer |
|------|----------|
| `e2e_<scenario>.json` | `make e2e-all` |
| `e2e_timings_report.md` | `make e2e-timings` |
| `footprint_bench.txt` | `make bench-footprint` |
| `plan_diff_5k_flamegraph.svg` | `make bench-footprint-profile` |
| `plan_diff_dhat.txt` | `make bench-footprint-alloc` |
| `alloc_flame.txt` | `dhat_alloc_tree.py` (called from alloc bench) |
| `scan_5k_load_flamegraph.svg`, `scan_dhat.txt` | `footprint_bench.sh profile-load-scan` / `alloc scan_root` |
| `cache_serde_load_flamegraph.svg`, `cache_serde_dhat.txt` | `footprint_bench.sh profile-load-cache` / `alloc cache` |
| `plan_db_trace.json` | `make plan-db-perf` |
| `prod_gate_report.json` | `make prod-gate` |
| `cli_phase_slo.json` | `make slo` |

All under `ops/perf/artifacts/` (gitignored except committed summaries).
