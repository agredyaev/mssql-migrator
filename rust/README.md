# Rust migrator

Lifecycle: `Current`.

Side-by-side Rust port of `rmig`. Canonical plan: [`../docs/rust-port-plan.md`](../docs/rust-port-plan.md).

## Build

```bash
cd rust
cargo build --release -p rmig -p rmigd
```

Binaries: `rust/target/release/rmig`, `rust/target/release/rmigd`

## Verify

```bash
make rust-build      # release rmig
make rust-arch       # crate boundaries + megastructure limits
make rust-test-int   # SQL Server integration (Docker + .temp/sql)
make rust-slo        # plan cache-miss SLO gate
```

Scope and boundaries: [Scope](../docs/rust-port-plan.md#scope). Parity backlog: [Remaining milestones (M8+)](../docs/rust-port-plan.md#remaining-milestones-m8). SLO status: [Status](../docs/rust-port-plan.md#status).

## Profiling (early-stage)

Workspace pins **`tokio = 1.52.3`**. Release builds keep debug symbols (`[profile.release] debug = true`) for stack traces.

| Tool | Command |
|------|---------|
| Criterion + flamegraph | `cd rust && cargo bench -p migrator-core --bench scan_digest -- --profile-time=2` |
| Integration test profile | `RMIG_PPROF=1 RMIG_RUN_SQLSERVER_INTEGRATION=1 cargo test -p migrator-core --test integration_plan -- --nocapture` |
| Full CLI flamegraph | `cargo install flamegraph` then `ops/perf/rust_flamegraph.sh` |
| Makefile | `make rust-bench`, `make rust-flamegraph` |
| Footprint / alloc audit | `make rust-bench-footprint`, `rust-bench-footprint-profile`, `rust-bench-footprint-alloc` |

Artifacts: `ops/perf/artifacts/rust_*` (gitignored). Layout policy: [`docs/data-oriented-layout-policy.md`](../docs/data-oriented-layout-policy.md). Measurement runbook: [`docs/perf-footprint-audit.md`](../docs/perf-footprint-audit.md).

## Session daemon (optional)

```bash
cd rust && cargo build --release -p rmigd
RMIGD_SOCKET=/tmp/rmigd.sock RMIGD_ENV=../.env ./target/release/rmigd
export RMIG_SESSION=/tmp/rmigd.sock
```

`make rust-slo` builds release `rmigd`, sets `RMIG_USE_RMIGD=1`, and runs the cache-miss SLO test (`cli_wall_ms` &lt; 150).

## Environment

Same `RM_*` variables as the Go CLI. Rust-specific:

| Variable | Default | Meaning |
|----------|---------|---------|
| `RMIG_SLO_MAX_CLI_WALL_MS` | `150` | Plan SLO threshold |
| `RMIG_SESSION` | — | Unix socket for `rmigd` |
| `RMIG_USE_RMIGD` | `0` | Integration/SLO harness: spawn `rmigd`, set `RMIG_SESSION` |
| `RMIG_INTEGRATION_WARM_SNAPSHOT` | `0` | SLO harness: reuse warm catalog/checksums after L1 invalidate |
| `RMIG_INSPECT_FULL` | `0` | Force full catalog inspect |
| `RMIG_CATALOG_CACHE` | on | Reserved for DB catalog cache parity |

L1 filesystem cache: `.rmig/cache/` (gitignored).
