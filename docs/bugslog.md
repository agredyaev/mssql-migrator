# Bug Hunting Log (`rmig` / `rmigd`)

Lifecycle: `Current`.

## Purpose

Record a static bug-hunting pass for the Rust workspace in this repository.

This document explains what `rmig` is supposed to do, which code paths were reviewed, which commands were used to validate the workspace state, and which defect candidates were identified. It exists so future fixes do not depend on chat history or memory.

## Scope

- Product purpose and operator contract: `README.md`, `docs/rmig-rust.md`, `docs/specs/rust/README.md`
- CLI contract and command flow: `crates/cli/src/main.rs`, `crates/cli/src/help.rs`, `crates/cli/src/args.rs`
- Engine orchestration: `crates/core/src/engine/mod.rs`, `crates/core/src/engine/run/mod.rs`, `crates/core/src/engine/run/database.rs`, `crates/core/src/engine/apply_run.rs`
- Apply and lock lifecycle: `crates/core/src/apply/mod.rs`, `crates/core/src/lock/mod.rs`, `sql/lock/acquire.sql`, `sql/lock/release.sql`
- Session proxy and daemon: `crates/core/src/session/mod.rs`, `crates/core/src/session/client.rs`, `crates/core/src/session/proxy.rs`, `crates/core/src/session/daemon/mod.rs`, `crates/core/src/session/daemon/serve.rs`
- Session auth inputs: `crates/core/src/session/auth.rs`, `crates/rmigd/src/main.rs`
- Config, auth, and env load: `crates/core/src/config/env.rs`, `crates/core/src/config/validate.rs`, `crates/core/src/config/catalog.rs`, `crates/core/src/config/default.rs`, `crates/core/src/config/cold.rs`
- SQL Server connect and exit-code mapping: `crates/core/src/driver/mssql.rs`, `crates/core/src/error.rs`
- Warm snapshot reuse: `crates/core/src/db/plan_snapshot.rs`, `crates/core/src/db/warm_snapshot.rs`, `ops/perf/cli_phase.sh`
- CI and operator contracts: `docs/ci-checkout.md`, `docs/operational-contract.md`, `docs/runbook.md`
- Ops harness scripts: `ops/perf/e2e.sh`, `ops/perf/e2e_all.sh`, `ops/perf/e2e_env.sh`, `ops/perf/prod_gate.sh`
- Validation commands run during this pass:
  - `cargo check --workspace --all-targets --message-format=short`
  - `cargo clippy --workspace --all-targets -- -D warnings`
  - ripgrep pattern scans for `unwrap`, `expect`, `panic!`, `unsafe`, `TODO`, `FIXME`, `XXX`, and `HACK`

## System Context

`rmig` is a repo-driven MSSQL schema migration tool. It scans the SQL layout under `RM_SQL_ROOT`, compares that layout with a live SQL Server catalog, computes a migration plan, and optionally applies it. `rmigd` is an optional Unix-socket daemon that keeps one warm SQL Server connection alive so repeated CLI invocations avoid a fresh TDS handshake.

This bug hunt started from the documented operator contract and then traced the Rust implementation. The workspace already passes `cargo check` and strict `cargo clippy`, so the focus here is semantic defects: mismatches between the published behavior and the actual runtime control flow.

A second pass focused on the intended production shape: SQL Server authentication with a service account, credentials injected through Azure DevOps environment variables, and the checked-in `ops/perf` scripts that are supposed to exercise the same runtime contract.

## Interfaces And Boundaries

- Inputs: CLI commands (`plan`, `migrate`, `validate`, `baseline`, `repair-checksum`, `version`), dotenv files, process `RM_*` / `RMIG_*` environment variables, SQL Server state, and the repo SQL tree under `RM_SQL_ROOT`
- Outputs: stdout JSON, stderr diagnostics, `.plan.json`, `.report.json`, SQL DDL/DML side effects, and SQL Server advisory-lock state
- Ownership boundaries:
  - `crates/cli/` owns argument parsing and top-level command framing
  - `crates/core/src/engine/` owns orchestration and command dispatch
  - `crates/core/src/session/` owns warm-session proxy behavior
  - `crates/core/src/driver/` owns SQL Server connection behavior
  - `crates/core/src/config/` owns dotenv parsing and runtime config shaping
  - `docs/` and `README.md` own the durable operator contract

## Assumptions And Constraints

- Assumptions:
  - This pass is static analysis only. No live SQL Server reproduction was run.
  - Severity reflects likely operator impact if the path is exercised.
  - The repo documentation is part of the product contract when it describes CLI behavior or recovery behavior.
- Constraints:
  - Findings are based on checked-in code and docs only.
  - `cargo check` and `cargo clippy` passing does not rule out runtime bugs, contract drift, or bad failure containment.
  - Existing unrelated repository changes were left untouched.

## Nominal Flow

1. Read `README.md`, `docs/rmig-rust.md`, and `docs/specs/rust/README.md` to establish the tool purpose and the expected CLI/session contracts.
2. Run `cargo check --workspace --all-targets --message-format=short` to confirm the Rust workspace builds cleanly before deeper review.
3. Run `cargo clippy --workspace --all-targets -- -D warnings` to confirm no lint-visible correctness issues are already flagged.
4. Search the tree for panic-prone patterns and unusual state paths with ripgrep.
5. Review the implementation paths most likely to hide operator-facing defects: JSON output framing, session fallback, lock release, auth mode selection, env-file loading, database creation side effects, warm snapshot reuse, and exit-code classification.
6. Record each identified issue with exact paths, operator impact, and concrete validation steps.

## Findings

### BG-001 Critical: Failed daemon-backed apply can leave the advisory lock attached to the warm SQL session

- Evidence:
  - Before this fix cycle, `crates/core/src/engine/apply_run.rs` acquired the advisory lock before `crate::apply::execute_plan(...)` but called `crate::lock::release(...)` only after a successful apply return.
  - `crates/core/src/session/daemon/mod.rs` creates one shared `RawClient` and reuses it for every socket client.
  - `sql/lock/acquire.sql` and `sql/lock/release.sql` use `@LockOwner = 'Session'`.
  - Post-fix behavior:
    - `crates/core/src/lock/mod.rs` adds `release_after_body` (always attempt release, then return body result).
    - `crates/core/src/engine/apply_run.rs` acquires the lock, runs `execute_plan`, then calls `release_after_body` even when apply fails.
    - `scripts/check-advisory-lock-release.sh` enforces the static contract in `make arch`.
    - `cargo test -p migrator-core --test advisory_lock_guard_test -- --test-threads=1` covers direct TDS lock release when `RMIG_RUN_SQLSERVER_INTEGRATION=1`.
    - `cargo test -p migrator-core --test advisory_lock_rmigd_test -- --test-threads=1` covers the same contract through `rmigd` when `RMIG_USE_RMIGD=1` (included in `make sql-regression`).
- Why this is a bug:
  - A failing `migrate`, `baseline`, or `repair-checksum` request could return early after the lock is acquired but before explicit release.
  - When the request uses `RMIG_SESSION`, the SQL session is owned by `rmigd`, not by the short-lived CLI process. The lock could therefore outlive the failed request.
- Operator impact:
  - Other SQL sessions could block on the stale lock until `rmigd` exited or the lock was manually released.
  - The daemon session stayed in a dirty state after an apply failure.
- Validation steps:
  1. Start `rmigd`.
  2. Set `RMIG_SESSION` to the daemon socket.
  3. Trigger a `migrate` that fails after lock acquisition.
  4. Confirm a second mutate attempt can acquire the lock without restarting `rmigd`.
  5. Run `scripts/check-advisory-lock-release.sh`, `make sql-regression`, or `cargo test -p migrator-core --test advisory_lock_rmigd_test`.
- Immediate containment:
  - Deploy a build that includes `release_after_body` on the apply path; on older builds, restart `rmigd` after a failed daemon-backed mutate command.

### BG-002 High: `plan --json` writes two top-level JSON documents to stdout

- Evidence:
  - Before this fix cycle, `crates/cli/src/main.rs` called `print_timings_json(&out.timings)` on stdout for every `--json` command, then wrote plan JSON to stdout for `plan --json`.
  - `crates/core/src/engine/io.rs` implements `print_timings_json(...)` with `println!`, which writes to stdout.
  - Post-fix behavior in `crates/cli/src/main.rs`: `plan --json` writes plan JSON to stdout and timings JSON to stderr; other `--json` commands still write timings to stdout only.
  - `docs/specs/rust/module-cli.md` documents the split-stream contract.
  - `cargo test -p rmig --test plan_json_cli_test` covers happy, negative, edge, and regression cases.
- Why this is a bug:
  - The command emits one JSON object for timings and then a second JSON object for the plan on the same stream.
  - Single-document JSON consumers cannot treat stdout as one valid document.
- Operator impact:
  - `plan --json` is unsafe as a machine-readable one-shot stdout contract.
  - Downstream tools can fail with trailing-data errors or parse only the first document.
- Validation steps:
  1. Run `rmig --env <path> plan --json`.
  2. Pipe stdout into a single-document JSON parser such as `python3 -c 'import json,sys; json.load(sys.stdin)'`.
  3. Observe the extra-data failure caused by the second JSON document.
- Immediate containment:
  - Treat `plan --json` stdout as ambiguous until timings and plan data are separated by stream or wrapped in one envelope object.
  - Prefer `.plan.json` under `RM_REPORT_DIR` if a stable plan artifact is required.

### BG-003 High: The documented `RMIG_SESSION` fallback is not implemented, and socket failure is misclassified as config error

- Evidence:
  - `docs/rmig-rust.md` and `crates/core/src/session/mod.rs` describe automatic fallback to direct database connections when the daemon socket is unavailable.
  - Before this fix cycle, `crates/core/src/engine/run/database.rs` called `crate::session::connect_daemon(&cfg.session_socket).await?` whenever `RMIG_SESSION` was set and the run was not multi-database.
  - `crates/core/src/session/proxy.rs` still maps `UnixStream::connect(...)` failure to `Error::Config(...)`, but the engine no longer surfaces that error when fallback is possible.
  - Post-fix behavior: `crates/core/src/session/client.rs` implements `connect_session_or_direct(...)`; stale or missing daemon sockets log a stderr warning and fall back to direct TDS.
  - `cargo test -p migrator-core --test session_fallback_test` covers happy, negative, edge, and regression cases.
- Why this is a bug:
  - No fallback path exists in the actual command execution flow.
  - A runtime socket outage is reported as an input/config problem instead of either falling back to direct TDS or returning a connection-class failure.
- Operator impact:
  - If `rmigd` is down or the socket path is stale, the CLI stops working instead of degrading to cold-connect behavior.
  - Automation receives exit code `2` (`EXIT_CONFIG`) instead of the documented warm-path fallback or a connection exit.
- Validation steps:
  1. Set `RMIG_SESSION` to a nonexistent socket path.
  2. Run `rmig plan`.
  3. Observe that the command exits through the daemon connect error instead of running a direct SQL connection.
- Immediate containment:
  - Unset `RMIG_SESSION` whenever `rmigd` is not known to be healthy.
  - Do not rely on the documented fallback until the code matches the contract.

### BG-004 High: `RM_DB_AUTH` is dead configuration, and integrated auth is advertised but unreachable

- Evidence:
  - `crates/core/src/config/env.rs` reads `RM_DB_AUTH` into `cfg.db_auth`.
  - `crates/core/src/config/default.rs` initializes `db_auth` to `sql`.
  - `crates/cli/src/help.rs` says `RM_DB_USER / RM_DB_PASSWORD` are required unless integrated auth is used.
  - Before this fix cycle, `crates/core/src/driver/mssql.rs` always called `tiberius::AuthMethod::sql_server(&cfg.user, &cfg.password)` and never checked `cfg.db_auth`.
  - Post-fix behavior: `driver::mssql::select_auth_method(...)` maps `sql` / `integrated` / `windows` to the matching `tiberius::AuthMethod`; tiberius is built with `integrated-auth-gssapi` (Unix) and `winauth` (Windows).
  - `cargo test -p migrator-core --lib select_auth_method` and `cargo test -p migrator-core --test db_auth_test` cover happy, negative, edge, and regression cases.
- Why this is a bug:
  - The CLI contract and config surface imply more than one auth mode.
  - The driver implementation hard-codes SQL authentication, so the alternate mode cannot work.
- Operator impact:
  - Operators can follow the documented integrated-auth path and still fail at connection time.
  - The repo exposes a configuration knob that never changes runtime behavior.
- Validation steps:
  1. Set `RM_DB_AUTH=integrated`.
  2. Omit `RM_DB_USER` and `RM_DB_PASSWORD`.
  3. Run `rmig plan`.
  4. Observe that the driver still attempts SQL authentication.
- Immediate containment:
  - Document SQL auth as the only supported mode until `driver::mssql::connect(...)` selects auth based on `cfg.db_auth`.

### BG-005 Medium: `--env <path>` silently degrades to ambient process environment or empty config on any file read error

- Evidence:
  - Before this fix cycle, `crates/core/src/config/env.rs` returned `Ok(HashMap::new())` whenever `std::fs::read_to_string(path)` failed.
  - Post-fix behavior:
    - `load_env_file(...)` stays optional for the default `.env` path (missing file → empty map).
    - `load_env_file_required(...)` fails with `env file not found` or `env file unreadable` when `--env <path>` is explicit.
    - `crates/cli/src/main.rs` uses the required loader for `--env`; `rmigd` uses `env_required=true` when `RMIGD_ENV` is set.
  - `cargo test -p migrator-core --lib load_env_file` and `cargo test -p rmig --test env_file_cli_test` cover happy, negative, edge, and regression cases.
- Why this is a bug:
  - A missing file, bad path, or permission problem does not produce a direct error.
  - The command can silently use shell-exported `RM_*` variables instead of the requested dotenv file.
- Operator impact:
  - A typo in `--env` can target the wrong SQL Server or SQL root.
  - Troubleshooting gets misleading errors such as missing variables instead of the real unreadable-file cause.
- Validation steps:
  1. Export valid `RM_*` variables in the shell.
  2. Run `rmig --env ./does-not-exist.env plan`.
  3. Observe that the command still uses the ambient environment instead of failing on the bad path.
- Immediate containment:
  - Verify the env file path before running the CLI.
  - Avoid relying on ambient `RM_*` variables when validating a specific dotenv file.

### BG-006 Medium: `plan` and `validate` are not side-effect-free when the target catalog database does not exist

- Evidence:
  - `crates/cli/src/help.rs` describes `plan` as a preview and `validate` as checks without applying.
  - Before this fix cycle, `crates/core/src/engine/run/mod.rs` always called `ensure_catalog_databases_exist(cfg, &databases).await?` before command-specific dispatch.
  - `crates/core/src/config/catalog.rs` implements `ensure_catalog_databases_exist(...)` with `CREATE DATABASE` from the `master` database.
  - Post-fix behavior in `crates/core/src/engine/run/mod.rs`: only `migrate`, `baseline`, and `repair-checksum` call `ensure_catalog_databases_exist(...)`.
  - `cargo test -p migrator-core --test plan_no_db_side_effect_test -- --test-threads=1` covers happy, negative, edge, and regression cases for `plan`/`validate` without auto-create.
- Why this is a bug:
  - Commands that read as non-mutating can still create databases on the server before the engine reaches plan-only or validate-only behavior.
  - The engine side effect is broader than the CLI/operator wording.
- Operator impact:
  - `plan` or `validate` can mutate SQL Server state.
  - Operators without create-database privilege can see read-style commands fail before plan logic starts.
- Validation steps:
  1. Point `RM_SQL_ROOT` at a catalog database name that does not yet exist on the target server.
  2. Run `rmig validate`.
  3. Observe either database creation or a create-database permission failure before validation logic.
- Immediate containment:
  - Pre-create target databases before using `plan` or `validate`.
  - Clarify the operator contract or narrow the mutation to apply-style commands only.

### BG-007 Medium: Warm snapshot reuse ignores server and database identity

- Evidence:
  - Before this fix cycle, `crates/core/src/db/warm_snapshot.rs` stored only `digest`, `checksums`, and `catalog`.
  - `crates/core/src/db/plan_snapshot.rs` reused the warm snapshot when `RMIG_INTEGRATION_WARM_SNAPSHOT` is enabled and the layout digest matched.
  - `ops/perf/cli_phase.sh` enables `RMIG_INTEGRATION_WARM_SNAPSHOT=1` by default for the performance harness.
  - The L1 cache key includes `server_database`, but the warm snapshot key did not.
  - Post-fix behavior: warm snapshot store/reuse keys on `{server}_{database}` plus layout digest (same fingerprint as L1).
  - `cargo test -p migrator-core --lib warm_snapshot` covers happy, negative, edge, and regression cases.
- Why this is a bug:
  - The in-process shortcut can replay plan-db state from one database or server against another target that happens to share the same layout digest.
  - The feature gate is environment-driven, not test-only at compile time.
- Operator impact:
  - If the flag leaks outside the intended integration/perf harness, the engine can build a plan from stale foreign catalog data.
- Validation steps:
  1. Enable `RMIG_INTEGRATION_WARM_SNAPSHOT=1`.
  2. Run a plan against database A.
  3. Switch to database B with the same repo tree and invalidate L1.
  4. Run plan again and inspect whether warm snapshot reuse bypasses the correct live catalog.
- Immediate containment:
  - Keep `RMIG_INTEGRATION_WARM_SNAPSHOT` disabled outside `ops/perf/cli_phase.sh` and the integration harness.

### BG-008 Medium: TDS handshake and auth failures are classified as SQL runtime errors instead of connection failures

- Evidence:
  - Before this fix cycle, `crates/core/src/driver/mssql.rs` wrapped handshake problems as `Error::Sql(format!("tds handshake: {e}"))`.
  - `crates/core/src/error.rs` only mapped `Error::Sql(...)` to `EXIT_CONN` when the message started with or contained `"connect "`.
  - Post-fix behavior: handshake errors are prefixed with `connect {addr}:` and the exit-code classifier also treats `tds handshake` as connection class.
  - `cargo test -p migrator-core --test exit_code_test` covers happy, negative, edge, and regression cases.
- Why this is a bug:
  - Authentication, TLS, and TDS handshake failures happen before a usable SQL session exists.
  - The exit-code classifier treats them as runtime SQL failures (`EXIT_SQL`) instead of connection failures (`EXIT_CONN`).
- Operator impact:
  - Automation can misroute recovery because the exit code does not match the documented failure class.
  - Auth and TLS issues look like regular SQL execution failures.
- Validation steps:
  1. Point `rmig` at a reachable SQL Server.
  2. Use bad credentials or force a TLS mismatch.
  3. Run `rmig plan`.
  4. Observe an exit code in the SQL-runtime class instead of the connection class.
- Immediate containment:
  - Treat handshake/auth failures as connection-class incidents in operational triage even if the current exit code is `5`.

### BG-009 High: Every non-version command requires a login that can connect to `master`

- Evidence:
  - `crates/core/src/engine/run/mod.rs` always calls `ensure_catalog_databases_exist(cfg, &databases).await?` before command-specific execution.
  - Before this fix cycle, `crates/core/src/config/catalog.rs` cloned the config, rewrote `database` to `master`, and connected there before issuing `IF DB_ID(...) IS NULL CREATE DATABASE ...`.
  - Live reproduction with a contained SQL user that could connect to `containedplan` but not `master` showed:
    - direct `sqlcmd` access to `containedplan` succeeded;
    - direct `sqlcmd` access to `master` failed;
    - `rmig plan` failed during `ensure_catalog_databases_exist(...)` with a `master` login error;
    - post-fix runtime logs showed `target database probe succeeded` for `containedplan` with no `master` preflight log.
- Why this is a bug:
  - The `master` connection happens even when the target catalog database already exists.
  - A SQL-auth service account that only has access to the target catalog database cannot use `plan`, `validate`, `migrate`, `baseline`, or `repair-checksum`, because the run fails before the actual target-database connect.
- Operator impact:
  - Azure DevOps service-account deployments can fail despite valid credentials and correct permissions on the real target database.
  - The tool implicitly requires broader server-level access than the CLI and top-level docs advertise.
- Validation steps:
  1. Create or use a SQL login that can access only the target catalog database and not `master`.
  2. Point `RM_DB_USER` and `RM_DB_PASSWORD` at that login.
  3. Run `rmig plan`.
  4. Observe that the command fails during `ensure_catalog_databases_exist(...)` before the target-database flow.
- Immediate containment:
  - Fixed in the current working tree by probing the target database first and using `master` only as a create-db fallback when the target connection fails.

### BG-010 Medium: `validate_config` does not fail fast when SQL-auth credentials are missing

- Evidence:
  - Before this fix cycle, `crates/core/src/config/validate.rs` only checked `RM_DB_SERVER` and `RM_SQL_ROOT`.
  - `crates/core/src/driver/mssql.rs` always uses `tiberius::AuthMethod::sql_server(&cfg.user, &cfg.password)`.
  - Post-fix behavior: `validate_config` rejects empty `RM_DB_USER` / `RM_DB_PASSWORD` when `RM_DB_AUTH` is SQL auth (default).
  - Unit tests in `crates/core/src/config/validate.rs` cover happy, negative, edge (`integrated`), and regression cases.
  - `docs/specs/rust/module-config-export.md` and `crates/cli/src/help.rs` both describe `RM_DB_USER` / `RM_DB_PASSWORD` as required for SQL auth.
- Why this is a bug:
  - In the current implementation, a missing Azure DevOps secret mapping is not caught during config validation.
  - The failure is deferred to TDS login, which makes the root cause look like a generic connection failure instead of a missing required input.
- Operator impact:
  - A pipeline variable typo or an unbound secret reaches runtime instead of failing immediately with a precise config error.
  - Troubleshooting is slower because the CLI error points at SQL connect/login instead of the absent credential variable.
- Validation steps:
  1. Set `RM_DB_SERVER` and `RM_SQL_ROOT`.
  2. Leave `RM_DB_USER` or `RM_DB_PASSWORD` empty while using SQL auth.
  3. Run `rmig validate` or `rmig plan`.
  4. Observe that `validate_config(...)` succeeds and the failure only appears during connect/login.
- Immediate containment:
  - Add an Azure DevOps preflight step that explicitly checks `RM_DB_USER` and `RM_DB_PASSWORD` are non-empty before invoking `rmig`.

### BG-011 High: `RMIG_SESSION_TOKEN` from `.env` or `RMIGD_ENV` is ignored by the actual auth path

- Evidence:
  - `crates/core/src/config/env.rs` loads `RMIG_SESSION_TOKEN` into `cfg.session_token`.
  - Before this fix cycle, `crates/core/src/session/auth.rs` read the token only from `std::env::var("RMIG_SESSION_TOKEN")`.
  - `crates/core/src/session/proxy.rs` sent auth based on `session_token_from_env()`, not on `Config`.
  - Post-fix behavior:
    - `session::resolve_session_token(Some(&cfg))` prefers `cfg.session_token` then process env.
    - `ProxyClient::connect(..., Some(cfg))` and `connect_daemon(..., cfg)` pass the loaded token to the client auth handshake.
    - `run_daemon` calls `apply_session_token_from_config(&cfg)` so daemon-side `verify_token` honors `RMIGD_ENV` / dotenv.
  - `cargo test -p migrator-core --lib resolve_session_token` and `cargo test -p migrator-core --test session_token_test` cover happy, negative, edge, and regression cases.
- Why this is a bug:
  - Operators can place `RMIG_SESSION_TOKEN` into `.env` or `RMIGD_ENV` and assume the daemon/client session auth is enabled.
  - In reality, the daemon and client only honor the token when it is exported in the real process environment.
  - If the token exists only in the env file, daemon auth silently disables itself and the client sends no `Auth` request.
- Operator impact:
  - Session auth can be unintentionally disabled with a false sense of protection.
  - `--env <path>` and `RMIGD_ENV=<path>` do not fully define the session-security behavior they appear to configure.
- Validation steps:
  1. Put `RMIG_SESSION_TOKEN=secret-token` in `.env`.
  2. Start `rmigd` with `RMIGD_ENV=.env` but without exporting `RMIG_SESSION_TOKEN` in the parent shell.
  3. Run the CLI with `--env .env` and `RMIG_SESSION=<socket>`.
  4. Observe from the code path that `token_required()` and the client `Auth` request both read an empty process env and therefore skip token auth.
- Immediate containment:
  - Export `RMIG_SESSION_TOKEN` in the actual process environment for both `rmigd` and the CLI; do not rely on `.env` alone for session auth.

### BG-012 Medium: `ops/perf/prod_gate.sh` ignores injected SQL credentials during database reset

- Evidence:
  - `ops/perf/prod_gate.sh` exports `RM_DB_SERVER`, `RM_DB_USER`, and `RM_DB_PASSWORD`.
  - Before this fix cycle, the reset path using `sqlcmd` hard-coded `-S localhost -U sa -P 'yourStrong(!)Password'` instead of the exported variables.
  - Post-fix behavior: reset uses `-S "$RM_DB_SERVER" -U "$RM_DB_USER" -P "$RM_DB_PASSWORD"` (same contract as `ops/perf/e2e_all.sh`).
  - `scripts/check-prod-gate-reset.sh` covers happy, negative, edge, and regression static checks; wired into `make arch`.
- Why this is a bug:
  - The script advertises an env-driven connection contract but bypasses it during reset.
  - Azure DevOps credentials passed through variables do not actually control the database reset step.
- Operator impact:
  - `make prod-gate` or `ops/perf/prod_gate.sh` fails outside the local Docker-default credential set.
  - The script can target the wrong SQL Server or wrong login during reset.
- Validation steps:
  1. Set `RM_DB_SERVER`, `RM_DB_USER`, and `RM_DB_PASSWORD` to non-default values.
  2. Run `ops/perf/prod_gate.sh` with `RMIG_GATE_SKIP_DB_RESET` unset.
  3. Observe that the reset step still uses `localhost`, `sa`, and the hard-coded password.
- Immediate containment:
  - Set `RMIG_GATE_SKIP_DB_RESET=1` when the local Docker default credentials are not valid.
  - Patch the script to use `"$RM_DB_SERVER"`, `"$RM_DB_USER"`, and `"$RM_DB_PASSWORD"` consistently.

### BG-013 Medium: `ops/perf/e2e.sh` and `ops/perf/e2e_all.sh` toggle the wrong git-inspect variable

- Evidence:
  - Before this fix cycle, `ops/perf/e2e.sh` and `ops/perf/e2e_all.sh` exported and unset `RMIG_SKIP_GIT`.
  - `crates/core/src/config/env.rs` only reads `RM_SKIP_GIT`.
  - Post-fix behavior: both scripts use `RM_SKIP_GIT` (including the `ddl_transition_apply` unset/restore cycle in `e2e_all.sh`).
  - `scripts/check-e2e-git-flag.sh` covers happy, negative, edge, and regression static checks; wired into `make arch`.
- Why this is a bug:
  - The e2e scripts do not actually force the full-inspect mode they claim to configure.
  - The later `unset RMIG_SKIP_GIT` / `export RMIG_SKIP_GIT=1` transitions in `ops/perf/e2e_all.sh` do not affect the Rust runtime at all.
- Operator impact:
  - Scenario baselines and performance claims from those scripts can differ from the intended git mode.
  - Debugging git-delta behavior through these scripts becomes misleading.
- Validation steps:
  1. Run `ops/perf/e2e.sh` or `ops/perf/e2e_all.sh` with `RM_SKIP_GIT` unset.
  2. Observe that the scripts only modify `RMIG_SKIP_GIT`.
  3. Confirm in `config::build_config` that the Rust code never reads that variable.
- Immediate containment:
  - Export `RM_SKIP_GIT=1` explicitly before invoking those scripts if full inspect is required.

### BG-014 Low: Root operator docs still instruct `RM_DB_DATABASE`, but runtime ignores it

- Evidence:
  - Before this fix cycle, `docs/operational-contract.md` and `docs/runbook.md` described `RM_DB_DATABASE` as an active `rmig` config input.
  - `crates/core/src/config/env.rs` explicitly clears `cfg.database`; `validate_config` derives the target from catalog layout under `RM_SQL_ROOT`.
  - Post-fix behavior: root docs state that `RM_DB_DATABASE` is shell-helper only (`ops/perf/e2e_env.sh`); canonical target selection is `RM_SQL_ROOT` layout.
  - `cargo test -p migrator-core --test catalog_database_env_test` and `scripts/check-rm-db-database-contract.sh` cover happy, negative, edge, and regression cases.
- Why this is a bug:
  - The root docs tell operators and pipeline authors to configure a variable that the runtime does not honor.
  - In Azure DevOps, a maintainer can set `RM_DB_DATABASE` expecting target selection while `rmig` still derives the target from `RM_SQL_ROOT`.
- Operator impact:
  - Misconfigured pipelines can appear correct on paper while targeting a different database tree than intended.
  - Failure triage follows the wrong variable.
- Validation steps:
  1. Set `RM_DB_DATABASE` to a value that does not match the top-level directory under `RM_SQL_ROOT`.
  2. Run `rmig plan`.
  3. Observe from the code path that `build_config(...)` ignores `RM_DB_DATABASE` and the runtime still derives the database from `RM_SQL_ROOT`.
- Immediate containment:
  - Treat `RM_SQL_ROOT` as the canonical database selector for `rmig`.
  - Use `RM_DB_DATABASE` only in shell helpers that explicitly document that they consume it.

### BG-015 High: Multi-database engine returns only the last per-database plan

- Evidence:
  - `crates/core/src/engine/run/mod.rs` kept only `last_plan` from the per-database loop before this fix cycle.
  - Live SQL Server reproduction via `crates/core/tests/multi_db_plan_test.rs` produced runtime logs in `.cursor/debug-0e99d5.log` showing:
    - two discovered databases: `dactests` and `warehouse`;
    - a per-database plan with `plan_object_count=1` for `dactests`;
    - a per-database plan with `plan_object_count=1` for `warehouse`;
    - a final merged engine output with `final_plan_object_count=1` and `final_first_object_database="warehouse"` before the fix.
  - The post-fix verification run against the same live SQL Server returned `final_plan_object_count=2` and `final_summary_object_count=2`.
- Why this is a bug:
  - A multi-database `plan` run scanned and diffed every catalog database correctly, but the top-level engine result only exposed the final database processed in the loop.
  - Any downstream consumer of `RunOutput.plan`, `.plan.json`, or report/export paths received a truncated plan for multi-database trees.
- Operator impact:
  - Multi-database plans can silently omit objects from earlier databases in the directory order under `RM_SQL_ROOT`.
  - CI/CD checks or human review can approve an incomplete plan while believing it covers the full repository tree.
  - Follow-on export and reporting paths serialize an incomplete plan even though the engine already did the work for all databases.
- Validation steps:
  1. Start a live SQL Server and point `RM_SQL_ROOT` at a tree with at least two top-level database directories, such as `dactests/` and `warehouse/`.
  2. Run `RMIG_RUN_SQLSERVER_INTEGRATION=1 cargo test -p migrator-core --test multi_db_plan_test -- --nocapture --test-threads=1`.
  3. Inspect `crates/core/src/engine/run/mod.rs` debug logs for the per-database counts and the final returned count.
  4. Confirm that the final plan preserves both databases rather than only the last one processed.
- Immediate containment:
  - Until the fix is present, run `rmig plan` once per top-level database directory instead of against a multi-database tree.
  - Treat any historical multi-database `.plan.json` produced by affected builds as potentially incomplete.

### BG-016 High: Fresh SQL Server batch plan queried audit history before bootstrap

- Evidence:
  - Live reproduction on project `docker compose` SQL Server (`localhost:1433`) failed in `crates/core/tests/integration_plan.rs` with `Invalid object name 'azdo_deploy_meta.history'`.
  - Post-fix regression suite: `cargo test -p migrator-core --test plan_deferred_bootstrap_test -- --test-threads=1` (happy, negative, edge, BG-016 regression).
  - Runtime logs in `.cursor/debug-0e99d5.log` showed the exact ordering:
    - `prepare_execute` set `need_bootstrap=true`, `defer_bootstrap=true`, and `bootstrap_in_sql=true`;
    - sequential execution then logged `will_ensure_before_body=false`;
    - the next step was `history empty probe starting` with `tables_ensured_cached=false`;
    - the probe immediately failed on `azdo_deploy_meta.history` before the deferred bootstrap happened.
  - After the fix, the same test passed and the probe log showed `tables_ensured_cached=true` before querying history.
- Why this is a bug:
  - On a fresh catalog database, the batch/sequential `plan` path deferred audit bootstrap but still executed the checksum/history probe first.
  - `plan` therefore failed against an empty SQL Server even though the runtime already knew it needed bootstrap.
- Operator impact:
  - `integration_plan` and any real `plan` run that uses the same deferred bootstrap path can fail on a clean database before any migrations exist.
  - The failure looks like a raw SQL object-missing error instead of a recoverable first-run bootstrap path.
- Validation steps:
  1. Start the project SQL Server with `docker compose up -d` on Apple Silicon with Colima Rosetta enabled.
  2. Run `RMIG_RUN_SQLSERVER_INTEGRATION=1 RM_DB_SERVER=localhost RM_DB_PORT=1433 RM_DB_USER=sa RM_DB_PASSWORD='yourStrong(!)Password' RM_DB_ENCRYPT=false RM_DB_TRUST_SERVER_CERTIFICATE=true cargo test -p migrator-core --test integration_plan -- --nocapture --test-threads=1`.
  3. Confirm the pre-fix failure references `azdo_deploy_meta.history`.
  4. Confirm the fixed build passes and the runtime logs show bootstrap state before the history probe.
- Immediate containment:
  - Until the fix is deployed, pre-create the audit tables or avoid the batch full-inspect path on a fresh SQL Server.

### BG-017 Medium: `write_plan_json` required `Workspace` when plan rows and objects were already materialized

- Evidence:
  - Before this fix cycle, `crates/core/src/export/plan_json/io.rs` required `workspace` whenever `plan.rows` was non-empty, even when `plan.objects.len() == plan.rows.len()`.
  - Report export (`crates/core/src/export/report.rs`) and slim-row plans that already called `ensure_objects_materialized(...)` therefore failed with `workspace required for slim plan rows`.
  - Post-fix behavior: `fully_materialized = !rows.is_empty() && objects.len() == rows.len()` uses the objects-only wire path without `Workspace`.
  - `cargo test -p migrator-core --test plan_json_roundtrip_test` and `crates/core/tests/report_test.rs` cover happy, negative, edge, and regression cases.
- Why this is a bug:
  - The export layer rejected already-materialized plans and forced callers to retain a `Workspace` solely for serialization.
  - `.report.json` generation could fail after a successful `plan` even though all object payloads were present.
- Operator impact:
  - `RM_REPORT_DIR` report writes could fail on materialized plans.
  - Library callers holding only `MigrationPlan` could not serialize plan JSON without the original workspace scan state.
- Validation steps:
  1. Build a `MigrationPlan` with one slim row and one matching `PlannedObject`.
  2. Call `write_plan_json(&plan, None, &mut buf)`.
  3. Observe the pre-fix `workspace required for slim plan rows` error.
  4. Confirm the fixed build writes JSON and `cargo test -p migrator-core --test plan_json_roundtrip_test` passes.
- Immediate containment:
  - Pass `Some(&workspace)` to report export until the fixed binary is deployed, or ensure `objects` is empty and rely on workspace-backed slim rows only.

### BG-018 High: `plan` failed on contained databases with Latin1_General collation

- Evidence:
  - After `BG-009` was fixed, live `rmig plan` under contained user `rmig_plan_contained` against `containedplan` failed with:
    - `Cannot resolve the collation conflict between "Latin1_General_100_CI_AS_KS_WS_SC" and "SQL_Latin1_General_CP1_CI_AS" in the equal to operation.`
  - The failure came from catalog/checksum SQL batches that compared `OPENJSON` / `JSON_VALUE` strings against `sys.*` catalog names and `azdo_deploy_meta.history.normalized_key` without a shared collation.
  - Relevant SQL paths: `sql/catalog/catalog_scope_header.sql`, `sql/catalog/catalog_sys_objects.sql`, `sql/catalog/columns_openjson.sql`, `sql/audit/load_checksums_openjson.sql`.
  - Post-fix verification:
    - `RM_DB_USER=rmig_plan_contained ... ./target/release/rmig plan` against `.temp/bg009sql/containedplan` exits 0.
    - `cargo test -p migrator-core --test plan_collation_test -- --test-threads=1` passes all four cases (happy, negative, edge, regression).
- Why this is a bug:
  - Contained or non-default-collation databases are a normal SQL Server deployment shape for service-account auth.
  - `plan` compared JSON-derived Unicode strings with catalog metadata using incompatible collations and failed before diff/report output.
- Operator impact:
  - `plan` could fail on contained databases even when the target DB was reachable and `master` preflight was no longer required.
  - The error surfaced as a raw SQL collation conflict instead of a migration plan.
- Validation steps:
  1. Create a contained database with `COLLATE Latin1_General_100_CI_AS_KS_WS_SC` and a `db_owner` contained user.
  2. Point `RM_SQL_ROOT` at a one-table layout and run `rmig plan` under the contained user.
  3. Confirm the pre-fix failure is SQL error 468 (collation conflict).
  4. Confirm the fixed build returns a plan and `cargo test -p migrator-core --test plan_collation_test` passes.
- Immediate containment:
  - Recreate the database with `SQL_Latin1_General_CP1_CI_AS` collation, or run `plan` with a login against a database whose default collation matches the instance catalog collation.

## Off-Nominal Behavior And Failure Containment

- Failure mode: mutate command fails after advisory lock acquisition while running through `rmigd` on builds before the BG-001 fix.
  Containment: deploy the `release_after_body` apply guard; on older builds, restart `rmigd` after a failed daemon-backed mutate command.
- Failure mode: `plan --json` is consumed as a single stdout JSON document on builds before the BG-002 fix.
  Containment: use a build where plan JSON is on stdout and timings JSON is on stderr, or read `.plan.json` under `RM_REPORT_DIR`.
- Failure mode: `RMIG_SESSION` points at an unavailable socket.
  Containment: unset `RMIG_SESSION`; do not rely on fallback that is not implemented.
- Failure mode: operators depend on `RM_DB_AUTH`, `--env <path>`, or `validate`/`plan` semantics exactly as the help text suggests.
  Containment: treat those paths as contract drift until implementation and docs are reconciled.
- Failure mode: a multi-database tree produces a plan that only contains the final catalog database.
  Containment: split the run by top-level database until the aggregation fix is deployed.
- Failure mode: a fresh catalog database fails during `plan` because the audit history probe runs before bootstrap.
  Containment: bootstrap the audit schema first or run a build that includes the deferred-bootstrap fix.
- Failure mode: `plan` fails on contained or Latin1_General databases with SQL error 468 (collation conflict).
  Containment: align database collation with instance catalog collation, or deploy the `COLLATE DATABASE_DEFAULT` SQL batch fix.

## Verification And Validation

- Contracts and checks executed:
  - `cargo check --workspace --all-targets --message-format=short`
  - `cargo clippy --workspace --all-targets -- -D warnings`
  - ripgrep searches for panic-prone and unsafe patterns
  - `make sql-regression` (`ops/perf/sql_regression.sh`) — bugslog SQL regression battery including `advisory_lock_rmigd_test`
  - `make check-e2e` — SQL regression + scenario matrix + workflow + SLO + prod gate (CI integration job)
  - `scripts/check-sql-regression-manifest.sh` — manifest drift guard in `make arch`
  - `cargo test -p migrator-core --lib validate_config -- --nocapture`
  - `cargo test -p migrator-core --lib load_env_file -- --nocapture`
  - `cargo test -p migrator-core --lib warm_snapshot -- --nocapture`
  - `cargo test -p migrator-core --test plan_json_roundtrip_test -- --nocapture`
  - `cargo test -p rmig --test env_file_cli_test -- --nocapture`
  - `cargo test -p migrator-core --test session_fallback_test -- --nocapture`
  - `cargo test -p migrator-core --lib select_auth_method -- --nocapture`
  - `cargo test -p migrator-core --test db_auth_test -- --nocapture`
- Evidence artifacts:
  - This document at `docs/bugslog.md`
  - The checked-in source paths listed in each finding
- Exit criteria for closing a finding:
  - Add or update a focused regression test for the affected path.
  - Update the relevant durable docs if the operator contract changes.
  - Re-run the Rust and doc validation commands after the code fix.

## Operations And Recovery

- Routine operation:
  - Use this file as the current triage list for bug-fix work in the Rust workspace.
  - Fix findings in severity order; `BG-001` through `BG-018` are addressed in this fix cycle except any item still listed under open issues.
- Recovery or rollback:
  - `BG-001` (pre-fix builds only): restart `rmigd` to drop the stale SQL session and its session-owned lock.
  - `BG-003`: remove `RMIG_SESSION` to force direct SQL connections.
  - `BG-005`: verify `--env` file readability before running the CLI.
  - `BG-006`: pre-create catalog databases before using `plan` or `validate`.
  - `BG-007`: keep `RMIG_INTEGRATION_WARM_SNAPSHOT` disabled outside the integration/perf harness.
  - `BG-009`: use a login that can connect to `master` until the unconditional preflight is removed or narrowed.
  - `BG-010`: add explicit non-empty checks for `RM_DB_USER` and `RM_DB_PASSWORD` in Azure DevOps before invoking `rmig`.
  - `BG-011`: export `RMIG_SESSION_TOKEN` in the actual daemon and CLI process environment; do not rely on `.env` alone.
  - `BG-012`: avoid the `prod_gate.sh` reset path or patch it to use the injected `RM_DB_*` variables.
  - `BG-013`: set `RM_SKIP_GIT`, not `RMIG_SKIP_GIT`, when reproducing full-inspect e2e runs.
  - `BG-014`: treat `RM_SQL_ROOT` as the runtime source of database selection.
  - `BG-015`: run one database at a time until the multi-database plan merge fix is present in the deployed binary.
  - `BG-016`: pre-create audit tables or avoid fresh-db batch full-inspect until the deferred-bootstrap fix is present in the deployed binary.
  - `BG-018`: recreate the database with `SQL_Latin1_General_CP1_CI_AS` collation or deploy the collation-safe SQL batches before running `plan` on contained databases.

## Open Issues And Non-Goals

- Open issues:
  - Earlier findings in this file were initially identified by static analysis; `BG-001` through `BG-018` were reproduced or regression-tested during this fix cycle.
  - This pass did not measure how many existing tests already cover the identified bugs; it only identified the missing or risky behavior from the current code paths.
- Non-goals:
  - This document does not fix the bugs.
  - This document does not replace module specifications under `docs/specs/rust/`.
  - This document does not define the final patch design for any finding.

## References

- Canonical source paths:
  - `README.md`
  - `docs/rmig-rust.md`
  - `docs/specs/rust/README.md`
  - `docs/specs/rust/module-cli.md`
  - `docs/specs/rust/module-engine.md`
  - `docs/specs/rust/module-config-export.md`
  - `docs/specs/rust/module-cache-session.md`
  - `docs/specs/rust/module-apply.md`
- Related implementation paths:
  - `crates/cli/src/main.rs`
  - `crates/cli/src/help.rs`
  - `crates/core/src/engine/run/mod.rs`
  - `crates/core/src/engine/run/merge.rs`
  - `crates/core/src/engine/run/database.rs`
  - `crates/core/src/engine/apply_run.rs`
  - `crates/core/src/apply/mod.rs`
  - `crates/core/src/session/proxy.rs`
  - `crates/core/src/session/auth.rs`
  - `crates/core/src/session/daemon/mod.rs`
  - `crates/rmigd/src/main.rs`
  - `crates/core/src/config/env.rs`
  - `crates/core/src/config/catalog.rs`
  - `crates/core/src/driver/mssql.rs`
  - `crates/core/src/error.rs`
  - `crates/core/src/db/plan_snapshot.rs`
  - `crates/core/src/db/warm_snapshot.rs`
  - `sql/lock/acquire.sql`
  - `sql/lock/release.sql`
- Related validation and operations paths:
  - `crates/core/tests/multi_db_plan_test.rs`
  - `crates/core/tests/plan_deferred_bootstrap_test.rs`
  - `crates/core/tests/catalog_object_pipeline_test.rs`
  - `ops/perf/cli_phase.sh`
  - `ops/perf/e2e.sh`
  - `ops/perf/e2e_all.sh`
  - `ops/perf/e2e_env.sh`
  - `ops/perf/prod_gate.sh`
  - `docs/ci-checkout.md`
  - `docs/operational-contract.md`
  - `docs/runbook.md`
  - `ops/quality/scripts/check_doc_structure.py`
  - `ops/quality/scripts/check_doc_context.py`
  - `ops/quality/scripts/check_doc_path_references.py`
  - `ops/quality/scripts/check_doc_language.py`
