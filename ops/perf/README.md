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
