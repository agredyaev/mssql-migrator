# Security Review

Lifecycle: `Current`.

## Purpose And Scope

This is the canonical repository security assessment and remediation record.
It covers the code, configuration, scripts, build, CI, Docker, and dependency
surfaces listed in sections 1 and 2.

## System Context, Interfaces, And Boundaries

`rmig` converts a repository SQL tree into SQL Server plan/apply operations;
`rmigd` optionally proxies those operations over an authenticated local Unix
socket. Section 2 defines the inputs, actors, assets, and ownership boundaries.

## Assumptions And Constraints

Repository paths and configuration are untrusted. Authored SQL bodies and the
protected process environment are trusted. Deployment controls remain outside
the repository review.

## Nominal Flow

Load bounded configuration, validate repository paths and scripts, connect to
the environment-selected peer, plan or apply under database safety controls,
then write confined reports and caches.

## Off-Nominal Behavior And Failure Containment

Invalid peer configuration, paths, sizes, TLS booleans, daemon authentication,
or SQL state fail before the affected privileged operation. Sections 3 and 4
record exact containment and unresolved verification.

## Verification, Operations, And Recovery

Section 9 records executed commands. Section 10 defines the remaining release
gate and deployment-owned recovery boundaries.

## Open Issues And Non-Goals

The live Docker SQL gates run against the loopback Docker SQL Server. This review
does not certify production SQL permissions, certificates, backups, HA, or host
hardening.

## 1. Executive Summary

This review covers the Rust workspace containing:

- `crates/cli`: the `rmig` command-line application.
- `crates/core`: migration, SQL Server, filesystem, cache, and session logic.
- `crates/rmigd`: a token-authenticated local Unix-socket daemon with one warm TDS connection.
- `crates/core-dev`: non-release benchmarks and profiling tools.

The review inspected Cargo manifests and lock data, Rust source and tests,
repository scripts, SQL assets, Docker configuration, GitHub workflows,
configuration examples, release gates, and durable documentation.

Nine reachable findings were identified: two High, four Medium, and three Low.
All nine have code or script fixes and focused regression tests. The highest
risk paths were repository TOML selecting the peer that received
environment-provided secrets and manual release runs accepting unprotected
branch or pull-request CI provenance under a contents-write token.

Current release assessment: `ACCEPTABLE WITH DOCUMENTED RISKS`. The approved
loopback SQL regression and full E2E gates passed. Remaining risks are recorded
in section 10. No repository release blocker remains after the applied fixes.

## 2. Architecture And Threat Model

### Entry Points

- Operator CLI arguments and process environment enter through `crates/cli/src/main.rs`.
- TOML enters through `crates/core/src/config/toml_config.rs`.
- Repository paths and SQL bytes enter through `crates/core/src/scan/`.
- Release refs and CI-run metadata enter through `.github/workflows/release.yml`.
- SQL Server responses enter through the Tiberius driver under `crates/core/src/driver/`.
- Local daemon requests enter through the Unix socket managed by `crates/rmigd`.
- CI and local integration orchestration enter through `Makefile` and `ops/perf/`.

### Trust Boundaries And Actors

- A migration/config contributor controls repository paths, TOML, and SQL files.
- SQL script authors are trusted for arbitrary SQL execution. Repository paths
  and non-script configuration are not trusted.
- An operator or CI runner controls process environment and SQL credentials.
- A local operating-system process may race writable report, scaffold, cache,
  or socket paths.
- A collaborator permitted to create branches and dispatch GitHub Actions may
  supply release source and CI provenance.
- A compromised dependency or CI contribution may affect build output.
- Remote unauthenticated users are not relevant; the repository has no HTTP,
  browser, RPC server, or remotely exposed daemon.

### Protected Assets And Privileged Operations

- `RM_DB_USER` and `RM_DB_PASSWORD`.
- `RMIG_SESSION_TOKEN` and the authenticated local daemon session.
- SQL Server catalog data, audit history, and migration transactions.
- Repository SQL, generated scaffold files, and reports.
- Release binaries and CI credentials.

Deployment-owned SQL permissions, certificate issuance, host hardening,
backups, and production network policy are outside the repository boundary.
GitHub branch protection and the identities allowed to dispatch releases remain
repository-administration assumptions.

## 3. Confirmed Findings

### SEC-001

ID: `SEC-001`
Title: Repository TOML could redirect environment credentials and daemon tokens
Severity: High
Confidence: Confirmed
Category: Trust-boundary and credential handling
CWE: CWE-15
Affected location: `crates/core/src/config/toml_config.rs` (`TomlConfig`),
`crates/core/src/config/env_build.rs` (`build_config`),
`crates/core/src/config/env_parse.rs` (`apply_tls`)
Threat actor: Malicious repository configuration contributor
Attacker prerequisites: The operator loads repository TOML while providing SQL
credentials or `RMIG_SESSION_TOKEN` through the environment
Trust boundary: Repository files to protected process environment
Source: Former `[database]` and `[session].socket` TOML fields
Sink: TDS peer selection and `ProxyClient` authentication handshake
Data flow: TOML peer value → `build_config` → Tiberius or Unix socket connect →
environment secret sent to the selected peer
Violated invariant: Repository files must not choose the recipient of a
protected environment secret
Security impact: SQL credential or daemon-token disclosure to an attacker-owned
peer
Evidence: The former environment-over-TOML fallback accepted server, TLS, and
socket values while user/password/token remained environment-only
Safe reproduction: `rejects_environment_only_settings_without_echoing_them`
Minimal remediation: Permit only `[paths]` and `[execution]` in TOML; read all
peer identity and TLS settings from named process variables
Regression test: `crates/core/src/tests/toml_config_test.rs`,
`crates/core/src/tests/env_build_test.rs`
Residual risk: A process-environment controller remains trusted and can still
select the peer by design.

### SEC-008

ID: `SEC-008`
Title: Manual releases accepted unprotected branch and pull-request CI provenance
Severity: High
Confidence: Confirmed
Category: CI/CD release integrity
CWE: CWE-78, CWE-269
Affected location: `.github/workflows/release.yml` (`Resolve release state`,
`Require green CI for this exact commit`, and `Commit, tag & push version bump`)
Threat actor: Malicious or compromised collaborator allowed to create a branch
and dispatch the existing Release workflow
Attacker prerequisites: A branch commit with a successful pull-request CI run
and permission to invoke `workflow_dispatch`
Trust boundary: Collaborator-controlled branch and workflow-run metadata to a
job with `contents: write`
Source: `github.ref_name` and a CI lookup filtered only by commit and workflow
Sink: Branch-controlled release build and atomic branch/tag push followed by
GitHub Release publication
Data flow: Manual branch dispatch → any successful CI event for the commit
accepted → branch code and build scripts run with the release token → version
commit, tag, binaries, and release published
Violated invariant: A release must use `main` or `master`, and its exact source
commit must have a successful push CI run on that same branch; GitHub context
values must enter shell scripts through environment variables
Security impact: Attacker-controlled release binaries or repository refs, with
possible command execution in the contents-write job
Evidence: The former manual path accepted every branch, did not constrain the
matching CI run to a push on that branch, and interpolated the branch context
directly into a shell assignment
Safe reproduction: The offline release-workflow contract test checks branch,
event, SHA, and shell-input invariants without dispatching or publishing
Minimal remediation: Restrict manual releases to branch refs named `main` or
`master`; require successful push CI for the exact SHA and branch; pass branch
context through step environment variables
Regression test: `ops/quality/scripts/tests/run.sh`
Residual risk: GitHub repository administration must protect `main`/`master`
and restrict release dispatch and workflow modification to trusted maintainers.

### SEC-002

ID: `SEC-002`
Title: Local integration runners could reset a non-loopback SQL Server
Severity: Medium
Confidence: Confirmed
Category: Destructive target validation
CWE: CWE-20
Affected location: `ops/perf/e2e_env.sh`, `ops/perf/prod_gate.sh`,
`ops/perf/sql_regression.sh`
Threat actor: CI configuration contributor or operator with an inherited
`RM_DB_SERVER` value
Attacker prerequisites: Valid remote SQL credentials and execution of a reset
runner
Trust boundary: Process environment to destructive SQL test orchestration
Source: `RM_DB_SERVER`
Sink: `DROP DATABASE`, `CREATE DATABASE`, and mutating integration suites
Data flow: Environment host → test runner → `sqlcmd`/Tiberius → destructive SQL
Violated invariant: Repository test reset workflows target only the local
Docker SQL Server
Security impact: Unintended remote database destruction
Evidence: The former scripts preserved arbitrary `RM_DB_SERVER` values before
their reset paths
Safe reproduction: Offline script tests use a synthetic remote host and mock
Docker/Cargo commands
Minimal remediation: Allow only `localhost`, `127.0.0.1`, and `::1` before any
reset; retain explicit remote plan-only operation with
`RMIG_GATE_SKIP_DB_RESET=1`
Regression test: `ops/quality/scripts/tests/run.sh`,
`scripts/check-prod-gate-reset.sh`
Residual risk: Remote plan-only operation still requires correct production
credentials and TLS policy.

### SEC-003

ID: `SEC-003`
Title: Output paths could follow pre-created symlinks
Severity: Medium
Confidence: High confidence
Category: Filesystem confinement
CWE: CWE-22, CWE-59
Affected location: `crates/core/src/export/report.rs` (`write_atomic`),
`crates/core/src/scaffold/dir.rs` (`write_file`)
Threat actor: Local process able to create paths in a writable output directory
Attacker prerequisites: Ability to pre-create a temporary or scaffold path
Trust boundary: Attacker-controlled directory entries to filesystem writes
Source: Pre-created temporary and scaffold paths
Sink: Temporary report writes and scaffold writes
Data flow: Output path → create/truncate → filesystem operation
Violated invariant: Generated writes never follow an existing final path
Security impact: Overwrite of another writable path
Evidence: Former temporary and scaffold writes used truncating create semantics
Safe reproduction: Dangling-symlink regression tests
Minimal remediation: Use create-new files followed by atomic rename where
applicable
Regression test: `crates/core/tests/scaffold_test.rs`,
`crates/core/tests/report_test.rs`
Residual risk: Configured output roots must remain owned by the runner.

### SEC-004

ID: `SEC-004`
Title: Repository SQL and TOML reads had no byte limit
Severity: Medium
Confidence: Confirmed
Category: Resource exhaustion
CWE: CWE-400
Affected location: `crates/core/src/file_io.rs` (`read_bounded`) and callers
under `config`, `scan`, `apply`, and `scaffold`
Threat actor: Malicious repository or configuration author
Attacker prerequisites: Ability to place an oversized input in the checkout
Trust boundary: Repository/configuration bytes to process memory
Source: SQL or TOML files
Sink: Former whole-file `read` and `read_to_string` allocations
Data flow: Attacker-sized file → allocation → checksum/parser/deserializer
Violated invariant: Untrusted files have a fixed per-file memory ceiling
Security impact: CLI or CI runner memory exhaustion
Evidence: Multiple scan workers could buffer whole files with no cap
Safe reproduction: Exact-limit and one-byte-over tests plus oversized SQL/config
tests
Minimal remediation: One standard-library bounded reader; 4 MiB SQL and 1 MiB
TOML limits
Regression test: `crates/core/src/file_io.rs`,
`crates/core/tests/scan_walk_test.rs`,
`crates/core/src/tests/apply_script_read_test.rs`,
`crates/core/src/tests/toml_config_test.rs`
Residual risk: The repository has no global file-count/total-byte cap. CI
checkout and storage policy remain the outer limit.

### SEC-009

ID: `SEC-009`
Title: Repository TOML could enable implicit name-only object adoption
Severity: Medium
Confidence: Confirmed
Category: Privileged operation configuration
CWE: CWE-15
Affected location: `crates/core/src/config/toml_config.rs`
(`ExecutionConfig`), `crates/core/src/config/env_build.rs` (`build_config`),
`crates/core/src/engine/adopt_gate.rs` (`ensure_adopt_allowed`)
Threat actor: Repository configuration contributor
Attacker prerequisites: An operator runs `migrate` against a database containing
an unaudited object with the same key as a repository object
Trust boundary: Repository TOML to the operator-only adoption decision
Source: Former `[execution].allow_adopt` TOML field
Sink: `ensure_adopt_allowed` before migration apply
Data flow: TOML `allow_adopt = true` → `build_config` → `Config::allow_adopt` →
adoption gate returns success
Violated invariant: Only the process operator may explicitly permit `migrate`
to trust an existing object by name without verifying its definition
Security impact: An unverified live object can be recorded as managed while its
definition differs from the repository, hiding integrity drift
Evidence: The former typed TOML field was a fallback for `RMIG_ALLOW_ADOPT`
Safe reproduction: The config regression rejects the TOML key and the existing
apply-integrity regression proves that migration remains blocked without the
process opt-in
Minimal remediation: Remove the TOML field, reject it with
`RMIG_ALLOW_ADOPT` migration guidance, and read the opt-in only from the process
environment
Regression test: `crates/core/src/tests/toml_config_test.rs`,
`crates/core/src/tests/env_build_test.rs`,
`crates/core/tests/apply_integrity_integration.rs`
Residual risk: `RMIG_ALLOW_ADOPT=1` intentionally trusts object identity by
name. Operators should prefer explicit `rmig baseline` when adopting a database.

### SEC-005

ID: `SEC-005`
Title: SQL passwords were exposed in `sqlcmd` process arguments
Severity: Low
Confidence: Confirmed
Category: Secret handling
CWE: CWE-214
Affected location: `ops/perf/prod_gate.sh`,
`ops/perf/sql_regression.sh`, `docker-compose.yml`
Threat actor: Another local process with permission to inspect command lines
Attacker prerequisites: Concurrent access to the test host or container
Trust boundary: Protected environment secret to process metadata
Source: `RM_DB_PASSWORD` or `MSSQL_SA_PASSWORD`
Sink: Former `sqlcmd -P` argument
Data flow: Environment variable → shell expansion → process argument vector
Violated invariant: Passwords must not appear in command arguments
Security impact: Local test database password disclosure
Evidence: Three checked invocations used `-P`
Safe reproduction: Static script contract test
Minimal remediation: Supply `SQLCMDPASSWORD` through the child environment
Regression test: `ops/quality/scripts/tests/run.sh`
Residual risk: Same-user/root processes may inspect process environments; host
process isolation remains required.

### SEC-006

ID: `SEC-006`
Title: Control characters in repository paths could reach terminal diagnostics
Severity: Low
Confidence: Confirmed
Category: Log and terminal injection
CWE: CWE-117
Affected location: `crates/core/src/sql_ident.rs` (`validate_path_component`),
`crates/core/src/scan/walk.rs` (`relative_sql_path`),
`crates/core/src/config/catalog.rs` (`discover_catalog_databases`)
Threat actor: Malicious repository path author
Attacker prerequisites: A filesystem accepting the crafted filename
Trust boundary: Filesystem names to logs and CLI errors
Source: Repository path component
Sink: `tracing` fields and error output
Data flow: Filename → relative path → warning/error renderer
Violated invariant: Untrusted names cannot inject terminal control sequences or
new log records
Security impact: Misleading CI logs or terminal output
Evidence: The former validator rejected separators/NUL but accepted newline and
escape characters
Safe reproduction: Newline path regression
Minimal remediation: Reject all Unicode control characters before parsing or
logging; render diagnostic paths with debug escaping
Regression test: `crates/core/src/tests/sql_ident_test.rs`,
`crates/core/tests/scan_walk_test.rs`
Residual risk: Valid Unicode remains accepted and relies on the consumer's
normal Unicode rendering.

### SEC-007

ID: `SEC-007`
Title: Malformed TOML errors echoed source lines containing secrets
Severity: Low
Confidence: Confirmed
Category: Error information exposure
CWE: CWE-209
Affected location: `crates/core/src/config/toml_config.rs` (`load_toml_config_inner`)
Threat actor: Operator mistake or malicious malformed config
Attacker prerequisites: A secret-like value appears in malformed TOML and the
error reaches retained logs
Trust boundary: Configuration contents to diagnostic output
Source: TOML source line
Sink: CLI error text
Data flow: Parser display → `Error::Config` → stderr
Violated invariant: Config errors never echo secret values
Security impact: Credential disclosure in terminal or CI logs
Evidence: A synthetic malformed password produced its full source line
Safe reproduction: `malformed_toml_error_does_not_echo_source_line_regression`
Minimal remediation: Use the parser's source-free message and file path only
Regression test: `crates/core/src/tests/toml_config_test.rs`
Residual risk: The config file path and non-secret error category remain visible
for diagnosis.

## 4. Needs Verification

- The non-UTF-8 catalog-name regression is compiled for Linux only. macOS/APFS
  rejected construction of the fixture with `Illegal byte sequence`.
- SQL Server least privilege, production certificate validation, hosted CI
  secret permissions, backup/restore, HA/failover, and deployment rollback need
  environment-specific evidence.

Confirmation of deployment-owned items requires production records and cannot
be established by this repository run.

## 5. Defense-In-Depth Improvements

- Use a deployment-specific SQL login so accidental disclosure does not expose
  unrelated SQL Server instances.
- Keep `.rmig`, report, and SQL roots writable only by the runner account.

No generic validation framework, custom cryptography, or global rate limiter is
needed for this local migrator.

## 6. Unsafe Rust And Soundness Assessment

`crates/core`, `crates/cli`, `crates/rmigd`, and `crates/core-dev` use
`#![forbid(unsafe_code)]`. No production unsafe block, unsafe function, manual
`Send`/`Sync`, custom allocator, raw-pointer API, or FFI boundary was found.

The only unsafe block is test-only
`crates/core/tests/common/rmigd.rs` (`register_exit_cleanup`). It registers an
`extern "C" fn()` with POSIX `atexit` so a test-owned daemon is killed and
reaped. The callback captures no stack data, accesses only static synchronized
state, handles a poisoned lock without panicking, and does not unwind across the
C ABI. It is not linked into release binaries.

Miri was not run. It cannot validate the external `atexit` call, and there is no
production unsafe Rust requiring Miri coverage.

## 7. Dependency And Supply-Chain Status

- `cargo deny check`: pass for advisories, bans, licenses, and sources. Duplicate
  versions remain warnings and are mostly split between production TLS and
  development profiling/testing trees.
- Raw `cargo audit`: non-zero for
  [RUSTSEC-2026-0194](https://rustsec.org/advisories/RUSTSEC-2026-0194.html) and
  [RUSTSEC-2026-0195](https://rustsec.org/advisories/RUSTSEC-2026-0195.html) in
  `quick-xml 0.26.0`.
- Chain: `quick-xml` → `inferno` → `pprof` → `migrator-core-dev`.
  `scripts/check-rust-release-deps.sh` proves this tree is absent from `rmig`
  and `rmigd` release dependencies. Inferno uses the SVG writer; the vulnerable
  Reader/NsReader attribute and namespace parsers are not invoked.
- `cargo outdated --workspace` found no compatible `pprof`/`inferno` update;
  reported direct updates are unrelated to these advisories.
- `cargo audit --ignore RUSTSEC-2026-0194 --ignore RUSTSEC-2026-0195`: pass.
- No git dependency or nonstandard registry is permitted by `deny.toml`.
- Production networking uses `native-tls`/OpenSSL through Tiberius. No custom
  cryptography was found.
- CI actions are SHA-pinned. Repository scripts contain no newly introduced
  download-and-execute path.

No dependency upgrade is recommended until `pprof`/`inferno` provides a
compatible `quick-xml` update. A broad lockfile upgrade would not close a
production attack path.

## 8. Changes Applied

- `SEC-001`, `SEC-007`: environment-only peer selection, bounded TOML, and
  source-free parse errors under `crates/core/src/config/`.
- `SEC-002`, `SEC-005`: loopback reset guards, secure remote no-reset defaults,
  and `SQLCMDPASSWORD` in `ops/perf/` and `docker-compose.yml`.
- `SEC-003`: removed the unused filesystem cache; report and scaffold writes
  use create-new files.
- `SEC-004`: shared `crates/core/src/file_io.rs` bounded reader.
- `SEC-008`: protected release branch, exact push-CI provenance, and
  environment-bound GitHub branch context in `.github/workflows/release.yml`.
- `SEC-009`: environment-only implicit-adoption opt-in under
  `crates/core/src/config/`.
- `SEC-006`: control-character and non-UTF-8 path rejection before parsing or
  logging.
- Configuration examples, ADRs, module specifications, CI guidance, and this
  canonical review were updated in the same change.

Compatibility effects:

- Former TOML `[database]` and `[session]` keys fail with environment-variable
  migration guidance.
- Former TOML `[execution].allow_adopt` fails with `RMIG_ALLOW_ADOPT` guidance.
- Legacy `.rmig/cache` entries are unused and remain safe to delete manually.
- SQL files over 4 MiB and TOML over 1 MiB are rejected.

## 9. Validation

Passed:

- `git diff --check`
- `cargo fmt --all -- --check`
- `cargo check --workspace --all-targets --all-features`
- `cargo clippy --workspace --all-targets --all-features -- -D warnings`
- `cargo test --workspace --all-features`
- `cargo test -p migrator-core --lib config::` (`23` passed)
- `bash ops/quality/scripts/tests/run.sh` (`34` passed)
- `make check` (`34` offline script tests; Rust tests and rustdoc passed)
- `make doc-check`
- `make release-build`
- `docker compose config --quiet`
- `cargo deny check`
- `cargo audit --ignore RUSTSEC-2026-0194 --ignore RUSTSEC-2026-0195`
- `cargo tree --workspace --all-features`
- `cargo tree --workspace --all-features --duplicates`
- `cargo tree --workspace --all-features --invert quick-xml`
- `cargo outdated --workspace`
- `gh run list --help` confirmed the release workflow's commit, workflow, event,
  branch, and success-status filters are supported
- Ruby standard-library YAML parsing accepted `.github/workflows/release.yml`
- `make sql-regression`: `ALL PASS` against the loopback Docker SQL Server
- `make check-e2e`: `ALL PASS`, including the full E2E matrix, workflow, rmigd
  CLI phase, and production gate
- E2E artifacts: `ops/perf/artifacts/e2e_all_report.txt` and
  `ops/perf/artifacts/prod_gate_report.json`

Expected non-zero result:

- `cargo audit`: reports only `RUSTSEC-2026-0194` and
  `RUSTSEC-2026-0195` through the non-release profiling chain documented above.

Sandbox-only intermediate results:

- The first `cargo test --workspace --all-features` run reached
  `session_use_timeout_test` and failed to bind its temporary Unix socket with
  `Operation not permitted`. The same complete command passed with local socket
  permission.
- The first sandboxed `cargo deny check` could not lock its advisory cache. The
  permitted rerun passed.

Blocked or unavailable:

- `cargo geiger`, `cargo machete`, `actionlint`: not installed; no tools were
  installed.
- Miri/Loom: not applicable to a confirmed production unsafe or concurrency
  finding in this change.

## 10. Remaining Risks

- Production SQL permissions, TLS certificate issuance, and secret-store policy
  remain deployment-owned.
- Same-user/root processes may inspect environment variables.
- Total repository file count is not capped, although every security-relevant
  individual file read is bounded.
- The test-only `atexit` FFI remains necessary for process cleanup and is
  outside release artifacts.

Release assessment: `ACCEPTABLE WITH DOCUMENTED RISKS`

## References

- Canonical configuration decision:
  `adr/0020-config-env-over-toml-secrets-env-only.md`
- Cache decision: `adr/0018-l1-plan-cache.md`
- Runtime contract: `docs/operational-contract.md`
- Configuration specification: `docs/specs/rust/module-config-export.md`
- Validation entry points: `Makefile`, `ops/quality/scripts/tests/run.sh`
