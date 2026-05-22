# rmig

Lifecycle: `Current`.

## Purpose

`rmig` plans and applies **MSSQL** schema migrations from a **repo-driven SQL layout**. Implementation: Rust workspace in this repository (`crates/core`, `crates/cli`, `crates/rmigd`).

## Build

```bash
make build              # target/release/rmig + rmigd (debug symbols for profiling)
make release-build      # target/release-dist/rmig → bin/rmig (LTO + strip)
./target/release/rmig --env .env plan
```

Release profiles:

| Profile | Used by | Settings |
|---------|---------|----------|
| `release` | `make build`, SLO/profiling | `opt-level=3`, `debug=true` |
| `release-dist` | `make release-build` → `bin/rmig` | `lto=fat`, `strip`, `codegen-units=1`, `panic=abort`, `debug=false` |
| `release-fast` | integration tests | inherits `release`, `debug=false` |
| `profiling` | Criterion / pprof benches | inherits `release`, `debug=true` |

## Verification

| Check | Command |
|-------|---------|
| Arch + fmt + clippy + unit tests | `make check` |
| Docs structure and sync | `make doc-check` |
| Both | `make full-check` |
| E2e matrix (Docker SQL) | `make e2e-all` |
| Apply + git workflow | `make integration` |
| SLO / prod gate / workflow | `make slo`, `make prod-gate`, `make workflow-fast` |
| Footprint / alloc | `make bench-footprint-alloc` |

## Documentation

| Topic | Path |
|-------|------|
| Module specs (NASA-style) | [`docs/specs/rust/README.md`](docs/specs/rust/README.md) |
| Operator env and profiling | [`docs/rmig-rust.md`](docs/rmig-rust.md) |
| Prod gate | [`docs/prod-gate.md`](docs/prod-gate.md) |
| Perf harness | [`ops/perf/README.md`](ops/perf/README.md) |
| Product overview | [`docs/solution.md`](docs/solution.md) |

## Layout

```
crates/core/src/   # migrator-core library (see lib.rs modules)
crates/cli/        # rmig binary
crates/rmigd/      # optional session daemon
crates/core-dev/   # benches and footprint harness (not in production deps)
sql/               # embedded T-SQL (include_str!)
.temp/sql/         # integration fixture layout
```
