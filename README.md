# `rmig` (MSSQL Reporting Migrator)

Lifecycle: `Current`.

## Purpose

`rmig` plans and applies **MSSQL** schema migrations from a **repo-driven SQL layout**. It is designed to act as a zero-downtime database deployment orchestrator, avoiding shared state locks and pipeline latency.

## Scope

The scope of this repository is the Rust workspace and embedded SQL structures:

```text
crates/core/src/   # migrator-core library (see lib.rs modules)
crates/cli/        # rmig binary
crates/rmigd/      # optional session daemon
crates/core-dev/   # benches and footprint harness (not in production deps)
sql/               # embedded T-SQL (include_str!)
.temp/sql/         # integration fixture layout
```

---

## System Context

`rmig` walks a declarative filesystem directory structure representing database schemas and views, compares it with an active SQL Server database catalog, plans required modifications, and executes migrations sequentially inside secure transaction blocks.

---

## Interfaces and Boundaries

- **Inputs**: Environment variable configurations (loaded from a `.env` file or process context) and a declarative T-SQL folder layout.
- **Outputs**: Compiled native binaries (`bin/rmig` and `rmigd`), plans, and execution audit reports.

---

## Assumptions and Constraints

- **Database Engine**: Strictly compatible with Microsoft SQL Server (MSSQL) 2016 and above.
- **Build Requirements**: Requires a stable Rust compiler toolchain and Python 3 for quality checks.

---

## Nominal Flow

Compile the production binaries using the following commands:

```bash
make build              # target/release/rmig + rmigd (debug symbols for profiling)
make release-build      # target/release-dist/rmig → bin/rmig (LTO + strip)
```

### Release Profiles

| Profile | Used by | Settings |
| :--- | :--- | :--- |
| **`release`** | `make build`, SLO/profiling | `opt-level=3`, `debug=true` |
| **`release-dist`** | `make release-build` → `bin/rmig` | `lto=fat`, `strip`, `codegen-units=1`, `panic=abort`, `debug=false` |
| **`release-fast`** | integration tests | inherits `release`, `debug=false` |
| **`profiling`** | Criterion / pprof benches | inherits `release`, `debug=true` |

---

## Off-Nominal Behavior and Failure Containment

- **Compilation Errors**: Catchable via static cargo analysis (`make check`).
- **Database Lock/Constraint Violations**: Aborts the current transaction immediately, rolls back partial transitions, and exits with a dedicated code.

---

## Verification and Validation

Verify the workspace using the following verification suite:

| Check Type | Target | Command |
| :--- | :--- | :--- |
| Static analysis | Architecture, fmt, clippy, unit tests | `make check` |
| Quality | Docs structure and sync checks | `make doc-check` |
| Aggregated checks | Both of the above | `make full-check` |
| Scenarios | E2E matrix (Docker SQL) | `make e2e-all` |
| Integration | Apply + git workflow integration | `make integration` |
| Performance gates | SLO / prod gate / workflow | `make slo`, `make prod-gate`, `make workflow-fast` |
| Memory audit | Footprint / allocation benchmarks | `make bench-footprint-alloc` |

---

## Operations and Recovery

- **Routine Execution**: Run plans using:
  ```bash
  ./target/release/rmig --env .env plan
  ```
- **Recovery**: Refer to [runbook.md](docs/runbook.md) for unlocking active sessions or recovering from blocked migrations.

---

## Open Issues and Non-Goals

- **Non-Goals**: Not intended to support non-Microsoft SQL Server databases.

---

## References

| Topic | Canonical Path |
| :--- | :--- |
| Architecture walkthrough (start here) | [docs/architecture-walkthrough.md](docs/architecture-walkthrough.md) |
| Core specifications | [docs/specs/rust/README.md](docs/specs/rust/README.md) |
| Operator reference | [docs/rmig-rust.md](docs/rmig-rust.md) |
| Production gate | [docs/prod-gate.md](docs/prod-gate.md) |
| Performance harness | [ops/perf/README.md](ops/perf/README.md) |
| Product overview | [docs/solution.md](docs/solution.md) |
