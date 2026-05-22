# Module specifications (`migrator-core`)

Lifecycle: `Current`.

## Purpose

One specification per top-level module in `crates/core/src/lib.rs`, plus CLI/daemon crates and the integration test harness.

## Scope

- Source: `crates/core/src/`, `crates/cli/`, `crates/rmigd/`, `crates/core/tests/`
- Embedded SQL: repo-root [`sql/`](../../../sql/) (wired in `crates/core/src/sql/mod.rs`)

## System context

Operator-facing build/env docs: [`docs/rmig-rust.md`](../../rmig-rust.md). This index links module specs to source paths.

## Interfaces and boundaries

| Code | Path under `crates/core/src/` | Specification |
|------|-------------------------------|---------------|
| CLI / daemon | `../cli/`, `../rmigd/` | [`module-cli.md`](module-cli.md) |
| `engine` | orchestration (`run_command`, plan/apply routing) | [`module-engine.md`](module-engine.md) |
| `scan` | filesystem + git layout ingest | [`module-scan.md`](module-scan.md) |
| `plan` | diff, scenarios, scope, filter | [`module-plan.md`](module-plan.md) |
| `db` | plan DB phase, catalog, L1, cache | [`module-db.md`](module-db.md) |
| `driver` | TDS client, timings, session proxy | [`module-driver.md`](module-driver.md) |
| `audit` | history, checksums, bootstrap | [`module-audit.md`](module-audit.md) |
| `apply` | migrate execution, transactions | [`module-apply.md`](module-apply.md) |
| `gate` | prod gate, e2e reports, changed paths | [`module-gate.md`](module-gate.md) |
| `domain` | workspace, object store, arena | [`module-domain.md`](module-domain.md) |
| `cache`, `session` | L1 cache, `rmigd` RPC | [`module-cache-session.md`](module-cache-session.md) |
| `scaffold` | blocked DDL migration files | [`module-scaffold.md`](module-scaffold.md) |
| `config`, `export`, `timings`, `error` | env, plan JSON, exit codes | [`module-config-export.md`](module-config-export.md) |
| `git`, `lock`, `sql`, `sql_ident` | helpers + embedded SQL | [`module-supporting.md`](module-supporting.md) |
| Tests | `crates/core/tests/` | [`module-test-harness.md`](module-test-harness.md) |

## Assumptions and constraints

- Integration examples assume Docker SQL Server and the `.temp/sql` fixture unless a spec states otherwise.

## Nominal flow

1. `crates/cli` loads `RM_*` from env / `--env` file.
2. `engine::run_command` scans layout (`scan`), runs plan DB phase (`db` + `audit`), computes plan (`plan`).
3. On `migrate`: optional blocked scaffold (`scaffold`), session lock (`lock`), apply (`apply`), audit flush (`audit`).
4. Phase timings via `timings`; optional reports via `export`.

## Off-nominal behavior

- Missing or stale `module-*.md` for a changed module → update the spec in the same PR as the code change.

## Verification

| Gate | Command |
|------|---------|
| Static + unit | `make check` |
| Docs | `make doc-check` |
| E2e baselines | `make e2e-all` |
| Warm CLI SLO | `make slo` |
| Prod gate | `make prod-gate` |
| Plan DB perf | `make plan-db-perf` |
| Git workflow | `make workflow-fast` |

Harness details: [`ops/perf/README.md`](../../../ops/perf/README.md), [`docs/prod-gate.md`](../../prod-gate.md).

## Operations and recovery

- Add a new top-level module: create `module-*.md` and link it from this index (`check_doc_sync.py`).

## Open issues and non-goals

- Non-goals: exhaustive rustdoc for every private helper.

## References

- `crates/core/src/lib.rs`
- [`docs/rmig-rust.md`](../../rmig-rust.md)
- [`docs/templates/document-template.md`](../../templates/document-template.md)
