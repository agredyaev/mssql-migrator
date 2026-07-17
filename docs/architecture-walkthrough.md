# Architecture Walkthrough (`rmig`)

Lifecycle: `Current`.

<!-- A beginner-oriented, end-to-end explanation of how rmig works: what it is, the
data flow, and a step-by-step trace of a run. Written in English with exact repository
paths and component names, per docs/specs/nasa-document-spec.md. -->

## Purpose

Give a newcomer a complete, plain-language mental model of `rmig` before they read the
per-module specs under `docs/specs/rust/`. It answers: "What does this tool actually do,
and what happens, step by step, when I run it?"

In one sentence: **`rmig` is "git-diff-and-apply" for a Microsoft SQL Server database's
schema.** You keep a folder of `.sql` files describing the schema you *want*; `rmig` reads
what the database *actually* has, computes the difference, and applies only the needed
changes inside a lock and a transaction, recording an audit trail.

The useful analogy is a **thermostat**: you set the target (the folder), it reads the room
(the live database), and it changes only what is off (the plan) — it never demolishes the
house to do it.

## Scope

In scope: the runtime behavior of the Rust workspace and its embedded SQL.

| Crate / path | Binary | Role |
| :--- | :--- | :--- |
| `crates/cli/` | `rmig` | Operator entry point; parses args, builds config, calls the core. |
| `crates/core/` | lib `migrator-core` | The engine: scan, connect, diff, apply, audit, lock, cache. |
| `crates/rmigd/` | `rmigd` | Optional session daemon holding one warm connection open. |
| `crates/core-dev/` | — | Benchmarks and the footprint guard; never shipped in production binaries. |
| `sql/` | — | Fixed T-SQL baked in with `include_str!` (catalog reads, lock, apply, audit). |

Out of scope: exhaustive per-module APIs (see `docs/specs/rust/README.md`), the CI gates
(see `docs/ci-usage.md`), and the operator command reference (see `docs/rmig-rust.md`).

## System Context

The whole tool is one loop with four steps:

```text
   YOUR FOLDER OF .sql              THE LIVE DATABASE
   ("desired" state)                ("actual" state)
          |                                |
          v  scan::populate                v  db::run_plan_db_phase
      Workspace  ------------+   +------ CatalogState + ChecksumMap
                             v   v
                     plan::compute_diff      "what is different?"
                             |
                             v
                       MigrationPlan         a list of decisions
                             |
                             v  apply::execute_plan (only for `migrate`)
             run the SQL, under a lock, in transactions,
                    recording an audit history row
```

Read what you want, read what exists, diff into a plan, then (optionally) apply it safely.
The database driver is `tiberius` (a pure-Rust TDS client) over `tokio` TCP — no ODBC.

## Interfaces And Boundaries

- Inputs: the command line, environment variables (from `--env <path>` / `.env` and the
  process environment), and a declarative folder tree
  `<database>/<schema>/<kind>/<name>.sql` (kinds: tables, views, procedures, functions,
  triggers, indexes, types, sequences, synonyms).
- Outputs: a process exit code, `tracing` logs on stderr, optional plan/report JSON
  (stdout for `plan --json`; files when `RM_REPORT_DIR` is set), and durable audit rows in
  the `azdo_deploy_meta.history` table.
- Ownership boundary: the CLI does no planning; all logic delegates to
  `migrator_core::engine::run_command`.

Commands (`crates/cli/src/args.rs`, `crates/core/src/engine/run/types.rs`):

| Command | Mutates DB? | Purpose |
| :--- | :--- | :--- |
| `plan` | no | Scan + inspect + diff, print what would change. No lock. |
| `validate` | no | Checks only. No lock. |
| `migrate` | yes | Plan + apply pending changes under the advisory lock. |
| `baseline` | yes | Adopt objects already present in the DB as the checksum baseline. |
| `repair-checksum` | yes | Recompute / store audit checksums. |
| `version` | no | Print version + git commit; never loads env or connects. |

Key environment variables: `RM_DB_SERVER`, `RM_DB_PORT`, `RM_DB_USER`, `RM_DB_PASSWORD`,
`RM_SQL_ROOT`, `RM_SQL_BASE`, `RM_REPORT_DIR`, `RM_LOG_LEVEL`, `RM_LOCK_TIMEOUT`,
`RM_COMMAND_TIMEOUT`, `RM_DB_ENCRYPT`, `RM_DB_TRUST_SERVER_CERTIFICATE`, `RMIG_SESSION`,
`RMIG_SESSION_TOKEN`, `RMIG_CATALOG_CACHE`.

## Assumptions And Constraints

- Target engine is Microsoft SQL Server 2016+ (OPENJSON is used); non-MSSQL engines are a
  non-goal.
- Checksums and layout digests are **SHA-256** (`sha2::Sha256`), stored as `[u8; 32]`.
- Connectivity uses `tiberius` with **no connection pool**: one connection per run, or one
  warm connection reused via `rmigd`. This is correct because a migration runs sequentially
  under a single lock.
- **SQL authentication only.** `select_auth_method` rejects `integrated` / `windows`;
  SQL user + password are always required. Rationale: the target SQL Server 2019 has no
  workload/managed-identity support, so token-based auth is out of scope.

## Nominal Flow

A run of `rmig --env .env migrate`, step by step:

1. Process start (`crates/cli/src/main.rs`): `#[tokio::main]` builds a multi-threaded async
   runtime; `async fn main` returns an `ExitCode` that becomes the shell exit status. The
   binary is `#![forbid(unsafe_code)]`.
2. Argument parsing (`crates/cli/src/args.rs`): a hand-written loop reads `--env`, `--json`,
   `-h/--help`, and one bare command word, mapped to the `Command` enum.
3. `version` short-circuit: printed before any env load or DB access.
4. Config: `.env` is loaded (`config/env.rs`), then `build_config` (`config/env_build.rs`)
   resolves each key with precedence process-env > dotenv > default, and `validate_config`
   (`config/validate.rs`) enforces required vars, validates the port, and derives the
   database name from the folder layout.
5. Logging: `init_tracing` (`crates/cli/src/logging.rs`) installs a stderr subscriber.
6. Signal race: `tokio::select!` runs the work against `shutdown_signal()`; Ctrl-C drops the
   run future, closing the connection so the server rolls back any open transaction.
7. Core entry (`crates/core/src/engine/run/mod.rs`): discover target databases, then
   `scan::populate` (`crates/core/src/scan/mod.rs`) reads every `.sql` at runtime, SHA-256s
   its bytes, and builds a `Workspace` (the desired state).
8. Per database (`engine/run/database.rs`): connect (`driver/mssql.rs`, 3 retries on
   connection errors only); mutating commands take the advisory lock via
   `apply_run::run_locked`, read-only commands do not.
9. Inspect (`db/plan_snapshot.rs`): query `sys.objects` / `sys.schemas` (the
   `sql/catalog/*.sql` files) into a `CatalogState`; load prior checksums into a
   `ChecksumMap`. `migrate` bypasses the local `.rmig/cache` L1 cache so it always diffs
   live state.
10. Diff (`plan/diff.rs`, `plan/diff_decide.rs`): per object, compare prior checksum vs
    current file checksum — exists+unchanged = Skip; exists+no-record = Adopt; missing =
    Create; exists+changed = Reprocess, or Block (a changed table with no migration script).
11. Apply (`apply/mod.rs`), under the lock, in dependency-safe kind order
    (`apply/kind.rs`): each object's SQL body plus its audit-history INSERT run in one
    `SET XACT_ABORT ON` transaction; a blocked plan exits 10.
12. Finish: release the lock, invalidate caches, optionally write `.plan.json` /
    `.report.json` (temp-file-then-rename), return the exit code.

## Off-Nominal Behavior And Failure Containment

- Concurrent deployers: a session-scoped advisory lock (`sp_getapplock`, resource
  `reporting_layer_migration`) serializes migrators. Planning and applying happen under the
  same lock, so the plan cannot go stale; if a process crashes, its connection closes and
  the lock auto-releases.
- Mid-statement SQL error: `SET XACT_ABORT ON; BEGIN TRANSACTION` (`sql/apply/*.sql`) makes
  the whole change roll back; nothing is half-applied.
- Crash mid-apply: because each object's DDL and its history row commit together, a killed
  run never leaves a committed change without its record. The chaos test
  `crates/core/tests/chaos_kill_mid_apply_test.rs` proves "applied exactly once."
- Objects present in the DB but absent from the folder are never dropped or altered
  (`crates/core/tests/unmanaged_objects_test.rs`): the diff is repository-bounded.
- Exit codes (`crates/core/src/error.rs`): `0` ok, `2` config, `3` connect, `5` sql,
  `7` lock-timeout, `8` invalid-input, `10` plan-blocked, `130` interrupted.

## Verification And Validation

- Contracts and checks: `make check` (fmt, clippy `-D warnings`, tests, rustdoc, arch
  guards), `make doc-check` (documentation gates), `make check-e2e` (SQL matrix, SLO,
  prod gate). Chaos/fuzz/regression tests live under `crates/core/tests/`.
- Read-only observation: `./target/release/rmig --env .env plan --json` prints the plan to
  stdout and phase timings to stderr while touching no data.
- Exit criteria: lint/test/doc/arch green offline; the E2E matrix green against live SQL.

## Operations And Recovery

- Routine: run `rmig plan` (or `validate`) to preview, then `rmig migrate` to apply.
- Speed: start `rmigd` and set `RMIG_SESSION` to its socket to reuse a warm connection; if
  the daemon is down, `rmig` connects directly (`crates/core/src/session/client.rs`).
- Recovery from a blocked or stuck migration: see `docs/runbook.md`. The advisory lock is
  session-scoped, so it frees automatically when a crashed connection closes.

## Open Issues And Non-Goals

- Beginner gotchas: argument parsing is hand-rolled (not `clap`); tracing initializes after
  config, so `help`/`version` emit no logs; the audit schema `azdo_deploy_meta` and lock
  resource `reporting_layer_migration` are load-bearing names and must not be renamed.
- Non-goals: supporting non-Microsoft SQL Server engines; Windows/integrated authentication;
  a general connection pool.

## References

- `docs/rmig-rust.md` - operator command reference.
- `docs/solution.md` - product overview.
- `docs/migration-flow.md` - the plan-and-apply flow in depth.
- `docs/runbook.md` - recovery and unlock procedures.
- `docs/specs/rust/README.md` - per-module specifications index.
- `ops/perf/README.md` - performance and SLO gates.
