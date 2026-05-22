# rmig - Rust implementation

Lifecycle: `Current`.

Operator and maintainer reference for the Rust workspace at the repository root.

## Build

```bash
make build
# target/release/rmig, target/release/rmigd
```

## Verify

```bash
make check           # arch, fmt, clippy -D warnings, unit tests
make test-int        # SQL Server integration_plan (Docker + .temp/sql)
make slo             # cache-miss cli_wall_ms gate (rmigd + RMIG_SESSION)
make e2e-all         # scenario matrix vs committed baselines
make integration     # apply + git workflow integration
make prod-gate       # incremental plan go/no-go
```

## Session daemon (optional)

```bash
cargo build --release -p rmigd
RMIGD_SOCKET=/tmp/rmigd.sock RMIGD_ENV=.env ./target/release/rmigd
export RMIG_SESSION=/tmp/rmigd.sock
```

`make slo` spawns `rmigd`, sets `RMIG_USE_RMIGD=1`, and asserts `cli_wall_ms` below `RMIG_SLO_MAX_CLI_WALL_MS` (default 150).

## Profiling

`make build` uses `[profile.release]` with debug symbols for flamegraphs. Operator artifacts from `make release-build` use `[profile.release-dist]` (`lto = "fat"`, `strip`, `codegen-units = 1`, `panic = "abort"`, `incremental = false`).

| Tool | Command |
|------|---------|
| Struct sizes + diff bench | `make bench-footprint` |
| CPU flamegraph (5k diff) | `make bench-footprint-profile` |
| dhat alloc audit | `make bench-footprint-alloc` |
| Integration flamegraph | `ops/perf/flamegraph.sh` |

Artifacts: `ops/perf/artifacts/` (gitignored). Runbook: [`perf-footprint-audit.md`](perf-footprint-audit.md).

## Environment

Standard `RM_*` variables (see [`operational-contract.md`](operational-contract.md)). Rust-specific:

| Variable | Default | Meaning |
|----------|---------|---------|
| `RMIG_SLO_MAX_CLI_WALL_MS` | `150` | Plan SLO threshold |
| `RMIG_SESSION` | - | Unix socket for `rmigd` |
| `RMIG_USE_RMIGD` | `0` | Harness: spawn `rmigd` and set `RMIG_SESSION` |
| `RMIG_INTEGRATION_WARM_SNAPSHOT` | `0` | Reuse warm catalog/checksums after L1 invalidate |
| `RMIG_CATALOG_CACHE` | on | DB catalog cache (set `0` to count exact SQL RTs) |

L1 filesystem cache: `.rmig/cache/` under the working directory (gitignored).

## Module documentation

Per-module specs: [`specs/README.md`](specs/README.md).
