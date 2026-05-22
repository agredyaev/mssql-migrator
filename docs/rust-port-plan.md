# Rust migrator reference (<150ms `cli_wall_ms`)

Lifecycle: `Current`.

## Purpose

Canonical map of the Rust **`rmig`** workspace: product SLO, module layout, Makefile targets, and verification gates.

## Scope

### In scope

- Commands: `plan`, `migrate`, `validate`, `baseline`, `repair-checksum`, `version`
- Operator contract: `RM_*` env, plan JSON wire format, `azdo_deploy_meta` audit tables, T-SQL in [`sql/`](../sql/)
- Product SLO: cache-miss `plan` **`cli_wall_ms` < 150 ms**
- Accelerators: L1 cache (`.rmig/cache/`), `rmigd` / `RMIG_SESSION`, parallel plan DB phase

### Out of scope

- `migrate` apply wall time SLO
- Empty-DB DROP/CREATE perf harness (excluded from plan SLO)
- Windows integrated auth (`RM_DB_AUTH=integrated`)
- Enforcing `RM_PLAN_FILE` / `RM_REPAIR_SCRIPT` (reserved; not engine-enforced)
- WAN / remote SQL latency beyond co-located SQL (assume **≤5 ms** RTT)

## System context

Production code lives in `crates/core`, `crates/cli`, and `crates/rmigd` at repo root. Dev-only benches and footprint harness: `crates/core-dev/`. Module specs: [`docs/specs/rust/README.md`](specs/rust/README.md).

## Interfaces and boundaries

| Crate | Role |
|-------|------|
| `migrator-core` | Engine, plan DB, apply, gate |
| `rmig` | Operator CLI |
| `rmigd` | Session daemon (`RMIG_SESSION`) |
| `migrator-core-dev` | Criterion / dhat / footprint (not a release dependency) |

## Assumptions and constraints

- Reference env: Docker SQL Server 2019 + `.temp/sql` smoke tree.
- SLO metric: full CLI `plan` **`cli_wall_ms`** (connect through diff + CLI overhead), JSON when `--json`.
- Cold and warm runs share the same **< 150 ms** threshold.
- `make check` enforces arch guard, release dep check, clippy `-D warnings`, unit tests, and e2e scenario sync (`scripts/check-e2e-scenarios.sh`).

## Nominal flow

1. `make build` → release `rmig` + `rmigd`.
2. `make slo` → warm `cli_wall_ms` gate via [`ops/perf/cli_phase.sh`](../ops/perf/cli_phase.sh).
3. `make e2e-all` → scenario baseline matrix.
4. `make prod-gate` → incremental plan go/no-go.

## Off-nominal behavior

- SLO failure: inspect `artifacts/cli_phase_slo.json` and plan DB trace (`RMIG_PLAN_DB_TRACE=1`).
- Missing Docker or `.temp/sql`: SQL integration and e2e targets skip or fail at connect; see [`ops/perf/README.md`](../ops/perf/README.md).

## Verification

| Check | Command |
|-------|---------|
| Static + unit | `make check` |
| Docs | `make doc-check` |
| E2e baselines | `make e2e-all` |
| SLO | `make slo` |
| Prod gate | `make prod-gate` |
| Footprint | `make bench-footprint`, `make bench-footprint-alloc` |

SLO gate ([`integration_plan.rs`](../crates/core/tests/integration_plan.rs)): `cli_wall_ms < RMIG_SLO_MAX_CLI_WALL_MS` (default 150).

Reference timings: [`crates/core/tests/testdata/cli_phase/plan_full_cli_reference.json`](../crates/core/tests/testdata/cli_phase/plan_full_cli_reference.json).

## Operations and recovery

`make slo` uses:

| Env | Role |
|-----|------|
| `RMIG_USE_RMIGD=1` | Harness spawns `rmigd`, sets `RMIG_SESSION` |
| `RMIG_INTEGRATION_WARM_SNAPSHOT=1` | Reuse warm plan DB snapshot after L1 invalidate |
| `RMIG_SLO_MAX_CLI_WALL_MS=150` | Threshold |

Manual daemon (optional):

```bash
make build
RMIGD_SOCKET=/tmp/rmigd.sock RMIGD_ENV=.env ./target/release/rmigd &
export RMIG_SESSION=/tmp/rmigd.sock
make slo
```

Repository layout:

```text
Cargo.toml
crates/cli/          # binary rmig
crates/core/         # library migrator-core
crates/core-dev/     # benches + footprint (not linked from rmig/rmigd)
crates/rmigd/        # session daemon
sql/                 # T-SQL (include_str!)
crates/core/tests/testdata/   # SLO + e2e fixtures
```

Makefile target table: [`ops/perf/README.md`](../ops/perf/README.md).

## Open issues and non-goals

- Non-goals: `migrate` apply wall SLO; Windows integrated auth; CI-enforced footprint thresholds.
- Resolved: `version` reads root `VERSION` and git HEAD at build time (`crates/core/build.rs`, `buildinfo`).

## References

- [`docs/solution.md`](solution.md)
- [`docs/prod-gate.md`](prod-gate.md)
- [`docs/rmig-rust.md`](rmig-rust.md)
- [`docs/data-oriented-layout-policy.md`](data-oriented-layout-policy.md)
- [`docs/perf-footprint-audit.md`](perf-footprint-audit.md)
- [`docs/specs/rust/README.md`](specs/rust/README.md)
