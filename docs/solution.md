# Solution

Lifecycle: `Current`.

## Purpose

Product-level description of **`rmig`**: CLI entry, engine pipeline, and artifacts. Module details: [`specs/rust/README.md`](specs/rust/README.md).

## Scope

| Layer | Path |
|-------|------|
| CLI | `crates/cli/src/main.rs` |
| Engine | `crates/core/src/engine/` |
| Scan | `crates/core/src/scan/` |
| Plan DB + catalog | `crates/core/src/db/`, `crates/core/src/audit/` |
| Diff / plan | `crates/core/src/plan/` |
| Apply | `crates/core/src/apply/` |
| Gate / reports | `crates/core/src/gate/`, `crates/core/src/export/` |
| TDS driver | `crates/core/src/driver/` |
| Embedded SQL | `sql/` → `crates/core/src/sql/mod.rs` |

## System context

`rmig` reads a SQL file tree under `RM_SQL_ROOT`, compares it to SQL Server catalog state and audit history, emits a migration plan, and optionally applies DDL/DML. Session acceleration uses optional `rmigd` (`RMIG_SESSION`).

## Interfaces and boundaries

| Command | Primary output |
|---------|----------------|
| `plan` | Migration plan JSON, phase timings |
| `migrate` | Applied objects, audit rows, exit 10 when blocked |
| `validate` / `baseline` / `repair-checksum` | Operator maintenance (see module specs) |

Configuration surface: `RM_*` env vars via `config::build_config`. Optional reports: `RM_REPORT_DIR` → `.plan.json` / `.report.json` via `export::report`.

## Assumptions and constraints

- Required env: `RM_DB_SERVER`, `RM_SQL_ROOT`.
- Database name comes from catalog directories under `RM_SQL_ROOT` (`config/catalog.rs`); `RM_SQL_BASE` defaults to `RM_SQL_ROOT` when empty.
- Co-located SQL Server for product SLO measurements (see [`rust-port-plan.md`](rust-port-plan.md)).

## Nominal flow

Commands: `plan`, `migrate`, `validate`, `baseline`, `repair-checksum`, `version` — routed through `engine::run_command`.

**Plan / migrate pipeline:**

1. `scan::populate` — walk `RM_SQL_ROOT`, optional git preload.
2. `db::run_plan_db_phase` — parallel ensure ‖ checksums → catalog inspect (direct connect) or sequential batch (`RMIG_SESSION`).
3. `plan::compute_diff` — scenario dispatch, blocked detection.
4. **Migrate only:** lock → filter applied migrations → `apply::execute_plan` → audit flush; or blocked scaffold + exit 10.

## Off-nominal behavior

- Blocked DDL without committed transition scaffold → exit **10** (`EXIT_PLAN_BLOCKED`).
- Missing catalog layout or DB connectivity → config or driver error before apply.

## Verification

| Check | Command |
|-------|---------|
| Unit + static | `make check` |
| E2e | `make e2e-all` |
| Prod gate | `make prod-gate` |
| SLO | `make slo` |

## Operations and recovery

- Routine release check: `make check-e2e` or individual gates in [`ops/perf/README.md`](../ops/perf/README.md).
- Recovery from bad migrate: use audit history and git tree state; see [`runbook.md`](runbook.md).

## Open issues and non-goals

- Non-goals: incremental apply execution in prod gate (gate compares plan snapshots only).

## References

- [`operational-contract.md`](operational-contract.md)
- [`prod-gate.md`](prod-gate.md)
- [`rmig-rust.md`](rmig-rust.md)
