# `rmig` Rust Implementation

Lifecycle: `Current`.

## Purpose

Provide an operator and maintainer reference for the Rust workspace at the repository root, detailing compilation, testing, profiling, and daemon-based operation.

## Scope

This specification governs the Rust workspace compilation structures, profiling suites, session daemons, and L1 cache behaviors:

- **Workspace Crates**: `crates/core`, `crates/cli`, `crates/rmigd`
- **Embedded Cache**: `.rmig/cache/` located in the working directory (gitignored)

---

## System Context

`rmig` compiles into a CLI utility which communicates directly with SQL Server. To improve execution speeds and bypass recurring connection handshakes under high-frequency executions, operators can optionally route traffic through the `rmigd` session proxy daemon via Unix domain sockets.

---

## Interfaces and Boundaries

### 1. Optional Session Daemon (`rmigd`)
- **Unix Domain Socket Integration**: Spawning the daemon exposes a Unix socket for warm connection multiplexing:
  ```bash
  cargo build --release -p rmigd
  RM_DB_USER=sa RM_DB_PASSWORD='***' RMIG_SESSION_TOKEN='***' \
    RMIGD_SOCKET=/tmp/rmigd.sock RMIGD_CONFIG=config.toml ./target/release/rmigd
  export RMIG_SESSION=/tmp/rmigd.sock
  ```
- **SLO Enforcement**: Running `make slo` automatically spawns `rmigd`, sets `RMIG_USE_RMIGD=1`, and asserts that the `cli_wall_ms` remains below `RMIG_SLO_MAX_CLI_WALL_MS` (default `150`).

### 2. Rust-Specific Environment Variables

| Variable | Default | Meaning / Effect |
| :--- | :---: | :--- |
| `RMIG_SLO_MAX_CLI_WALL_MS` | `150` | Wall time threshold for the plan SLO gate (in milliseconds). |
| `RMIG_SESSION` | - | Unix domain socket path connecting the CLI client to the active `rmigd` daemon. |
| `RMIG_USE_RMIGD` | `0` | Test harness override: automatically spawns `rmigd` and configures `RMIG_SESSION`. |
| `RMIG_INTEGRATION_WARM_SNAPSHOT` | `0` | Directs the engine to reuse warm metadata catalog snapshots after L1 invalidation. |
| `RMIG_CATALOG_CACHE` | `on` | Toggle for the local DB catalog memory cache (set `0` to count raw SQL RTs). |

---

## Assumptions and Constraints

- **Platform Compatibility**: Daemon sockets (`rmigd`) require POSIX-compliant environments supporting Unix domain sockets (Unix/macOS).
- **Compilation for Profiling**: Deep CPU/allocation profiling requires target compilations retaining debug symbols (`[profile.release]` with `debug=true`).

---

## Nominal Flow

Compile the core library and CLI components in the workspace:

```bash
make build
# Compiles target/release/rmig and target/release/rmigd
```

`make release-build` compiles `[profile.release-dist]` featuring fat LTO, symbol stripping, and a single codegen unit to produce highly optimized, minimal production binaries.

---

## Off-Nominal Behavior and Failure Containment

- **Daemon Socket Disruption**: If `rmigd` crashes or the socket under `RMIG_SESSION` becomes inaccessible, the CLI client automatically fails safe by falling back to standard direct database TDS connections, logging the connection event to stderr.
- **Cache Corruption**: If the local L1 cache under `.rmig/cache/` becomes corrupted or stale, execution can be recovered by purging the cache folder (`rm -rf .rmig/cache`).

---

## Verification and Validation

### 1. Code Quality & Integration Tests

```bash
make check           # Run rustfmt, clippy (with -D warnings), and library unit tests
make test-int        # Run SQL Server integration_plan tests against Docker MSSQL
make e2e-all         # Execute full E2E scenario matrices vs committed baselines
make integration     # Run apply and git workflow integration suites
make prod-gate       # Execute incremental plans and go/no-go checks
```

`ops/perf/sql_regression.sh` claims the repo-local lock directory `.rmig/sql-regression.lock` before it starts `rmigd` or the shared SQL Server regression battery. This prevents overlapping runs from contending on the same advisory-lock resource or fixed integration database names and turning the battery flaky.

### 2. Memory & Performance Auditing

| Profiling Target | Tool / Command | Generated Artifacts |
| :--- | :--- | :--- |
| Struct sizes & baseline diffs | `make bench-footprint` | `artifacts/struct_sizes.json` |
| CPU flamegraph (5k objects) | `make bench-footprint-profile` | `artifacts/plan_diff_5k_flamegraph.svg` |
| DHAT heap allocations | `make bench-footprint-alloc` | `artifacts/plan_diff_dhat.txt` |
| Full integration flamegraph | `ops/perf/flamegraph.sh` | Profiling artifacts |

---

## Operations and Recovery

- **Profiling Recovery**: Purge gitignored profiling output files located under `ops/perf/artifacts/` to free local disk space.
- **Stale Baselines**: When struct definitions change intentionally in `crates/core`, committed baselines must be updated via `make bench-footprint-update-baseline`.

---

## Open Issues and Non-Goals

- **Non-Goals**: The `rmigd` daemon is not designed to support Windows Named Pipes or run as a Windows Service natively without emulation.

---

## References

- Deep footprint audit: [perf-footprint-audit.md](perf-footprint-audit.md)
- Environment contract: [operational-contract.md](operational-contract.md)
- Core specs: [specs/rust/README.md](specs/rust/README.md)
