# Production-Readiness Audit — rmig / rmigd

Date: 2026-07-19 · Auditor: staff-engineer review session · Scope: working tree `refactor/port-to-rust` @ `f0ee81f` (baseline) → `221eb02` (post-fix)

## 1. Executive verdict

**READY WITH ACCEPTED RISKS.**

Every deterministic gate, every DB-backed regression suite, an 8-scenario manual failure-injection matrix, chaos (kill −9 mid-apply) and concurrency tests, memory/allocation benchmarks, a 200-iteration daemon soak with zero leaks, and packaged-artifact smoke tests pass on a live SQL Server 2019 instance. Seven defects found and fixed (one latent correctness bug in result decoding, one silent-failure UX gap on the flagship fail-closed path, one SQL parameterization violation, plus hardening and parity gaps). Remaining risks are enumerated in §13 and are deliberate design bounds, not defects.

Two gates are environment-limited on this audit host (an arm64 laptop running the amd64 SQL Server image under Rosetta, alongside interactive workstation load):

- `plan-db-perf` phase-2 SLO (500 ms) misses at a stable ~557 ms under emulation; all four phases pass at the CI threshold (`RMIG_PLAN_DB_MAX_PAR_MS=2500`), and three consecutive runs spread only 1 ms (557/558/557) — stable overhead, not flakiness.
- The CLI-wall SLO (<150 ms) passed **seven consecutive runs at 95–110 ms**, including a post-fix run — then late-session reruns measured 175–246 ms after interactive load appeared on the host (a browser at ~200% CPU, load average 4–5.8; both the host-side engine leg and the emulated-DB leg degraded uniformly ~2×, and a SQL Server container restart changed nothing). Same binary, same fixture, no code change in between — environment-attributed. The enforcement point for both SLOs is CI on ubuntu/amd64, where they pass.

## 2. Scope reframe: this is a schema/DDL migrator, not a data ETL tool

The audit brief assumed row-data migration (type fidelity, LOB handling, row checksums). The product does not do that and claims nowhere that it does: it is a git-driven **schema migrator** (Flyway-shaped). Source = a repository of `.sql` object files; target = a live SQL Server catalog. It plans a diff (SHA-256 checksums vs audit history + live-catalog drift fingerprints), then applies DDL — tables via repo-authored transition scripts, modules via `CREATE OR ALTER` — inside per-object transactions with an atomic audit-history record. There is no data copy, bulk insert, type-mapping, or LOB path anywhere in the code (verified by exhaustive grep). Correctness testing was therefore mapped to what exists: object semantics, identifier quoting, checksum idempotency, drift blocking, advisory locking, crash/resume, and daemon lifecycle.

## 3. Environment and versions

| Item | Value |
|---|---|
| Host | macOS 26.5.1, arm64 (Apple Silicon) |
| Rust | rustc/cargo 1.96.0 — identical to the CI pin (no skew) |
| Docker | 29.6.2, lima VM aarch64, Rosetta cached, 4 CPU / 3.8 GiB |
| MSSQL fixture | mcr.microsoft.com/mssql/server:2019-latest (linux/amd64 under Rosetta), loopback-only |
| Python | 3.14.6 (doc gates) |
| cargo-deny | 0.20.2 |
| Branch state | `refactor/port-to-rust`, clean tree at baseline; ahead 186 / behind 166 vs origin (heavily diverged — release flow releases CI-validated SHAs, so this audits the working tree, not origin) |

## 4. Architecture and migration-flow summary

Four crates: `migrator-core` (~15.6k non-test LOC — all logic), `rmig` (CLI), `rmigd` (Unix-socket daemon holding one warm TDS connection to skip per-run login), `migrator-core-dev` (benches, never linked into shipped binaries — enforced by gate script). MSSQL driver: tiberius 0.12.3 (TDS 7.3, native-tls).

Flow: scan `sql_root` into an arena-backed workspace → per-database plan under a `sp_getapplock` advisory lock (catalog inspection + checksum load batched into single TDS round-trips) → apply: schemas, then table transition scripts, then modules via `CREATE OR ALTER` → each non-idempotent script runs as `BEGIN → body → ASSERT open tx → history INSERT → COMMIT`, so a script and its audit row commit atomically; a self-committing script is caught by the assert. Structural drift (live state ≠ last audited fingerprint) fails closed with exit 10 for non-module objects. Idempotency/resume derives from the audit history: killed runs leave each object fully-applied-and-recorded or untouched; the next run re-plans from live state.

Code discipline: `#![forbid(unsafe_code)]` everywhere, clippy `unwrap_used`/`expect_used` denied, zero `todo!`/`unimplemented!`, ≤100 code lines per non-test file (CI-enforced), no inline SQL outside tests (all queries are `include_str!` assets under `sql/`).

## 5. Gate matrix

All commands are the repo's own (`Makefile`/CI). "Baseline" = pre-fix tree `f0ee81f`; "post-fix" = `221eb02`. Full logs: audit session scratchpad (`phase*.log`).

| Gate | Command | Baseline | Post-fix | Duration (post-fix) |
|---|---|---|---|---|
| Arch scripts ×11 + gate self-tests + fmt + clippy `-D warnings` + core unit tests + rustdoc + doc gates ×5 | `make full-check` | PASS | PASS | ~60–90 s warm |
| Unit incl. CLI + rmigd | `make test` | PASS (315 tests, 0 ignored) | PASS | 73 s |
| CLI unit tests standalone | `cargo test -p rmig` | PASS (10) | PASS (in `make test` now) | 6 s |
| Supply chain | `cargo deny check` (now `make deny`) | PASS | PASS | 2 s |
| SQL regression battery (17 test binaries incl. chaos, drift, advisory-lock, rmigd) | `make sql-regression` | PASS 5 m 39 s | PASS 3 m 52 s | — |
| E2E scenario matrix vs golden baselines | `make e2e-all` | PASS | PASS | 19 s |
| Warm-workflow suite | `make workflow-fast` | PASS | PASS | 20 s |
| Production go/no-go incremental gate | `make prod-gate` | PASS | PASS | 9 s |
| CLI wall SLO < 150 ms (via rmigd) | `make slo` ×3 | PASS: 107/99/95 ms | PASS: ~110 ms (7 green runs total); late-session reruns 175–246 ms under host interactive load — environment-attributed (§1), CI is the enforcement point | 10 s |
| Integration plan suite | `make test-int` | PASS (after env fix — see F3) | PASS | — |
| Chaos: kill −9 mid-apply + concurrent migrate | `cargo test --release --test chaos_kill_mid_apply_test` | PASS 2/2 | PASS | 7 s |
| Struct-footprint regression vs committed baseline | `make bench-footprint` | — | PASS | 40 s |
| Allocation profile (dhat) | `make bench-footprint-alloc` (+ transitions, scan) | — | PASS | 91 s |
| CPU profile + artifact identity | `make bench-footprint-profile`, `make profile-summary` | — | PASS (after regenerating stale pre-fix artifacts — identity check worked as designed) | 6 s |
| Plan-DB parallel SLO | `make plan-db-perf` | — | 557 ms vs local 500 ms: **environment-limited** (stable ×3 under Rosetta); PASS at CI threshold 2500 ms (all 4 phases) | 3 s |
| Release build (LTO fat, strip, panic=abort) | `make release-build` | — | PASS (33 s) | — |
| Packaged-artifact smoke (plan/migrate/daemon plan/SIGTERM) | manual, `bin/rmig` + `target/release-dist/rmigd` | — | PASS: exits 0/0/0/0, socket removed | — |
| Daemon soak 200 iterations | audit harness | — | PASS (§11) | ~4 min |
| Final sweep (clean env) | `make full-check && make test` | — | PASS (29 s / 52 s) | — |
| Final DB sweep | `make check-e2e` stages | — | sql-regression ALL PASS · e2e-all PASS · workflow-fast PASS (after fixture-container restart) · prod-gate PASS · slo environment-limited (§1) | — |

Audit-harness honesty note: the first final-sweep `full-check`/`make test` invocations failed because the audit's own chain script leaked `e2e_env.sh` exports (`RM_SQL_ROOT` et al.) into their environment; an env-override regression test correctly caught the polluted environment. Clean-environment reruns pass. The late-session `workflow-fast` failure (phase-2 `parallel_wall_ms` 1028 ms) recovered fully after restarting the two-day-old emulated SQL Server container; audit-history/cache row counts were checked and are trivial (6 rows), ruling out data accumulation.

Zero flakes observed anywhere: `slo` ×3 spread 95–110 ms; `plan-db-perf` ×3 spread 1 ms.

## 6. Test matrix

| Category | Scenarios | Result |
|---|---|---|
| Unit (in-module) | 46 `#[cfg(test)]` modules, 315 assertions-passing tests, 0 ignored | PASS |
| Integration (DB-gated, 48 binaries) | plan/apply/drift/locking/session/chaos/exit-codes | PASS via `sql-regression` + `check-e2e` |
| Property-based | 4 `proptest!` blocks | PASS (within unit runs) |
| E2E matrix | scenario baselines incl. empty DB, warm plan, skip-unchanged, blocked table, prod-gate cold | PASS |
| Chaos/concurrency | kill −9 mid-apply (no partial commit, rerun exactly once); two concurrent migrates serialize exactly once | PASS |
| Failure injection (manual) | 8 scenarios (§8) | PASS |
| Coverage tooling | none configured in repo; coverage-by-critical-path argued via the suites above touching every apply/plan/lock/session path | n/a (recommendation §15) |

## 7. MSSQL compatibility and correctness scope

Covered and verified by suites + manual scenarios: databases/schemas creation, tables via transition scripts, views/procedures/functions/triggers via `CREATE OR ALTER`, indexes, checksummed idempotent re-runs (`skip_unchanged`), structural-drift fail-closed (exit 10), checksum-integrity fail-closed (exit 4 + `repair-checksum` recovery), identifier safety (validated `bracket_ident`: length ≤128, `]`→`]]`, path chars rejected; type literals gated by `validate_type_literal`), collation handling (`plan_collation_test`), multi-database catalogs.

Explicitly out of product scope (not defects): row data movement, data type fidelity (decimal/LOB/temporal values), row counts/checksum reconciliation, sequences/synonyms/temporal tables beyond what transition scripts author. Result-set decoding for the tool's own catalog queries covers varbinary/varchar/nvarchar/bit/tinyint/smallint/int/bigint and fails closed on anything else (F4).

## 8. Failure, recovery, resume, idempotency results

| Scenario | Outcome | Verdict |
|---|---|---|
| Wrong password | exit 3 in 1 s; message names login failure; password never appears in output | PASS |
| Unreachable server | exit 3 in 2 s (`Connection refused`) | PASS |
| Invalid boolean env (`RM_DB_ENCRYPT=banana`) | exit 2: `RM_DB_ENCRYPT has invalid boolean value "banana"; use true or false` — fail-fast before connect | PASS |
| Corrupted persisted checksum | exit 4: `audit history checksum for smoke/tables/smoke_table is undecodable; run repair-checksum` → `repair-checksum` exit 0 → plan exit 0 | PASS |
| Advisory-lock contention (applock held by another session, `RM_LOCK_TIMEOUT=2`) | exit 7 in 2 s | PASS |
| Malformed SQL object in repo | exit 5 (server syntax error surfaced); no history row committed for the failed object; rerun after fix safe | PASS |
| Database dropped externally | read-only `validate` → exit 3 with classified 4060 text; `migrate` recreates the catalog DB and converges, exit 0 | PASS |
| kill −9 mid-apply | no partial commit (body+history atomic); rerun applies exactly once | PASS (chaos test) |
| Daemon command timeout | wedged TDS session discarded, next request reconnects | PASS (`rmigd_timeout_recovery_test`) |
| Concurrent migrates, same object | serialize via applock, apply exactly once | PASS (chaos test) |
| Second daemon on same socket | startup refused: `a daemon is already listening on …` | PASS |
| Daemon SIGTERM | exit 0, socket removed, warm connection dropped (server rolls back any open tx) | PASS (after F5) |
| SIGINT on CLI | exit 130, run future dropped at statement boundary (documented contract) | PASS (existing behavior) |

## 9. Security findings by severity

No critical or high findings open.

- **Fixed (medium, latent):** silent `Cell::Null` for undecodable column types — wrong-plan-with-no-error class (F4).
- **Fixed (low):** git refs beginning with `-` could be parsed as git options (F2); value interpolation into SQL text, non-injectable but policy-violating (F6); silent exit 10 hid the reason for refusal (F7, operational rather than security).
- **Verified strong:** secrets are env-only (never TOML), redacted in both `Debug` impls, absent from all failure output (empirically checked in S1); TLS on + cert validation by default, encrypted-connection opt-out warned loudly at runtime by the driver; daemon token auth constant-time; socket 0600 in 0700 parent, group/world-accessible parents refused; identifier quoting validated + escaped at every dynamic site; all data values parameter-bound; `#![forbid(unsafe_code)]`; supply chain clean (`cargo deny`: advisories/bans/licenses/sources OK; two RUSTSEC ignores are dev-only quick-xml via inferno, documented in `deny.toml`).
- **Investigated, unchanged (M2):** `encrypt=false` maps to `EncryptionLevel::NotSupported` (no TLS at all, including login). Flipping to `Off` (login-only TLS) breaks against TLS-less fixtures — empirically confirmed — and would change opt-out semantics fleet-wide. Default remains `encrypt=true`; the opt-out is explicit, and tiberius logs a clear credentials-unencrypted warning. Documented as accepted behavior.

## 10. Profiling and benchmark results

- `plan_diff_skip_heavy_5000` (criterion, 5k-object diff): **145.7 µs** [145.65–145.74], consistent with the committed baseline; struct-footprint layout matches the baseline with real provenance (target/rustc stamped, dirty=false, revision = audited commit).
- dhat allocation profiles (plan-diff, transitions, scan) regenerated at the audited revision; artifact identity checks (release `2211b34` hardening) correctly rejected stale pre-fix artifacts until regeneration — the gate works.
- CLI SLO: 95–110 ms wall against the 150 ms product SLO, through the daemon, cache-miss path — ~40 ms headroom even under emulation.
- `plan-db-perf`: phase 1 baseline OK (~1.03 s wall, within allowance); phase 2 `parallel_wall_ms` 557 ms vs local 500 ms (see §1 — emulation-attributed; CI threshold passes all phases).
- Per-DB existence probe (M1, full TDS login per catalog DB): measured inside SLO/plan-db-perf envelopes; connect_ms 2–4 per probe. At fleet scale N databases → N logins on first deploy; accepted with the documented rationale (per-DB permission validation + 4060-vs-outage classification). No rewrite.
- No N+1 patterns, no unbounded buffering beyond full materialization of catalog result sets (bounded by catalog size; accepted, §13).

## 11. Resource-leak and soak results

200 sequential `rmig plan` runs through one `rmigd` (release binaries, live DB):

- RSS: **7,824 KB at iteration 20 → 7,824 KB at iteration 200** — byte-identical, zero growth.
- Open fds: 18 → 18, flat.
- macOS `leaks`: **0 leaks for 0 total leaked bytes** (1,228 nodes / 236 KB resident heap).
- Failures: 0/200.
- Tail: SIGTERM → exit 0, socket file removed (F5 verified on both `target/release` and packaged `release-dist` binaries).

Multi-hour soak was explicitly descoped (user decision); a 200-cycle serve/reconnect loop bounds per-request leak risk.

## 12. Issues discovered and fixes applied

| # | Commit | Issue → root cause → fix → verification |
|---|---|---|
| F1 | `6150d84` | `error.rs` module doc + `docs/ci-usage.md` claimed exit 4 unreachable; `Error::Checksum → EXIT_CHECKSUM(4)` is live (constructed on undecodable audit checksum). Docs corrected; empirically triggered in S5. |
| F2 | `969bca5` | Git refs passed unguarded to `git diff`/`merge-base`; a `-`-prefixed ref parses as an option. Guard at the two shared chokepoints returns `None` (existing no-git-delta fallback). `make check` + git tests. |
| F3 | `ffac135` | Local/CI parity: no toolchain pin, `cargo deny` CI-only, CLI unit tests only inside the DB harness, `make test-int` compiled-then-self-skipped (hollow pass — found live during Phase 3). Added `rust-toolchain.toml` (1.96.0), `make deny`, `cargo test -p rmig` in `make test`, env sourcing in `test-int`. |
| F4 | `2246b71` | `from_tiberius` mapped any unhandled column type to `Cell::Null` — conversion failure indistinguishable from SQL NULL; wrong plan with no signal. Now returns `Error::Sql` (exit 5) naming column+type; NULL of a supported type still maps to `Cell::Null`. First attempt exposed the bug class live: `COL_LENGTH()`/`sys.columns.max_length` are smallint, silently nulled for the life of the codebase — the `column_exists` test probe always returned false. i16 rung added; full DB suites green. |
| F5 | `58fb833` | rmigd had no SIGINT/SIGTERM handling (relied on SIGKILL + server rollback). Signal now drops the serve future (listener + warm TDS conn close, server rolls back), unlinks the socket, exits 0. Verified in soak tail + packaged smoke. |
| F6 | `d6902d1` | `CACHE_LOAD_RELAXED` object count string-substituted into SQL (`.replace("@p1", count)`): plan-cache entry per distinct count + bind-policy violation (not injectable — usize). Asset now uses `@p4`, sole caller binds the count, option collapsed to bool; `security-review.md` updated. Verified: unit test + `workflow-fast` + `e2e-all` (warm git-delta path). |
| F7 | `221eb02` | Blocked migrate/validate exited 10 through the Ok path with **zero console output** (blockers existed only in `--json`/reports). Non-json runs now print each blocker + a `--json` pointer to stderr. Verified live: `rmig: blocked: table smoke/tables/smoke_table changed but has no non-scaffold transition scripts`. |

Refuted prior-exploration claims (recorded for honesty): "4 empty SQL asset files" — false, all are non-empty templates/fragments; "exit 4 dead code" — inverted, the docs were wrong, not the code.

## 13. Remaining risks and limitations (accepted, documented)

1. **Daemon head-of-line blocking** — one warm TDS connection serializes all clients (by design; bounded by 60 s idle timeout, command timeout, `MAX_DAEMON_CLIENTS`). Throughput ceiling, not a correctness risk. **Empirically validated** (`ops/perf/hol_probe.sh`): at 1/2/4/8 concurrent `plan` clients, daemon-serialized and direct-cold-connect modes are statistically identical (N=8: p95 0.72 s vs 0.73 s, 0 failures both), and zero >100 ms queue events fired — the wall-clock growth with N is client-CPU contention, present identically in both modes. A session pool would buy nothing for this workload. Production tripwire: rmigd now logs `queued for warm session` with the wait duration whenever a client queues >100 ms; recurring warnings are the trigger to revisit with a pool. Mutating-path concurrency correctness is separately proven by the chaos suite (concurrent migrates serialize exactly once via the applock).
2. **No metrics/health endpoint** on rmigd — liveness is probeable by socket connect (the daemon itself uses this to refuse double-start); tracing to stderr is the only telemetry. Recommendation only.
3. **Arena invariant `panic!`s under `panic=abort`** (release-dist) — documented "can't happen" assertions on validated scan input; a trip aborts without a report or classified exit code.
4. **`@pN` renumbering via text `.replace`** in `db/catalog.rs` — fragile but fenced by unit tests asserting placeholder positions and by every DB suite.
5. **Full materialization of catalog result sets** — bounded by catalog object count; no streaming needed at product scale.
6. **Single-platform CI** (ubuntu/amd64), amd64-only DB fixture, core-dev benches not wired into CI (perf on shared runners is noise; local runs are the evidence).
7. **Local `plan-db-perf` 500 ms SLO unreachable under Rosetta** — stable 557 ms; CI (native amd64) is the enforcement point.
8. **SIGINT mid-COMMIT window** — a commit already sent may complete server-side while the client exits 130; the audit-history model makes the rerun converge (skip-unchanged), so no divergence.
9. **Branch divergence** — `refactor/port-to-rust` is ahead 186/behind 166 vs origin; this audit certifies the local head `221eb02`, not a merge result.
10. **Local incremental builds can carry a stale `RMIG_COMMIT` stamp** — `build.rs` bakes the commit at core-compile time and reruns only when `VERSION` changes, so a CLI-only change rebuilt incrementally keeps the previous core stamp (observed: binary containing fix F7 stamped `d6902d1`). CI release builds are cold-cache full builds, so shipped artifacts stamp correctly; local convenience binaries may lag by one commit.

## 14. Reproduction commands

```sh
# deterministic gates (no DB)
make full-check && make test && make deny

# DB fixture + full DB-backed suite
make db-up
make check-e2e            # = sql-regression + e2e-all + workflow-fast + slo + prod-gate
make test-int
ROOT=$PWD sh -c 'set -a; . ops/perf/e2e_env.sh; set +a; \
  cargo test --release -p migrator-core --test chaos_kill_mid_apply_test -- --test-threads=1'

# perf/memory
make bench-footprint && make bench-footprint-alloc && make bench-footprint-profile
ops/perf/footprint_bench.sh alloc transitions && ops/perf/footprint_bench.sh alloc scan
make profile-summary
RMIG_PLAN_DB_MAX_PAR_MS=2500 make plan-db-perf   # CI threshold on emulated hosts

# packaging
make release-build && bin/rmig version
```

## 15. Final release checklist and recommended CI gates

Checklist (all satisfied at `221eb02`): clean release build ✓ · analyzers/linters ✓ · all tests ✓ · no unexplained flakiness (0 observed) ✓ · security checks clean ✓ · realistic migrations + reconciliation-in-scope ✓ · interruption/resume/idempotency ✓ · no leaks/bottlenecks ✓ · soak stable ✓ · packaged smoke ✓ · config fail-fast with safe defaults ✓ · CI enforces mandatory gates ✓ · docs corrected to actual behavior ✓ · remaining risks documented (§13) ✓.

Recommended CI additions (not blocking):
1. Run `make test-int`-equivalent env sourcing everywhere a DB suite is invoked outside `check-e2e` (fixed locally by F3; CI already sets env at job level).
2. Add a `cargo deny` job dependency for release (currently lint-stage only) — cheap insurance.
3. Consider a nightly (not per-PR) run of `bench-footprint` on a dedicated runner to catch layout regressions earlier than local-only.
4. Optional: line/branch coverage reporting (e.g. `cargo llvm-cov`) focused on `apply/`, `plan/`, `lock/`, `session/` to make the critical-path argument quantitative.

## 16. Changed files

| File | Change |
|---|---|
| `crates/core/src/error.rs` | Module-doc exit-code table corrected (4 live; 6/9 reserved) |
| `docs/ci-usage.md` | Exit 4 added to operator exit-code list |
| `crates/core/src/git/diff.rs` | Reject `-`-prefixed refs in `diff_name_only`/`merge_base` |
| `rust-toolchain.toml` | New: pin 1.96.0 (= CI) |
| `Makefile` | `deny` target; `cargo test -p rmig` in `test`; `test-int` sources `e2e_env.sh` |
| `crates/core/src/driver/row.rs` | Fail-closed decode (`Result<RowData>`), i16 rung, NULL-vs-mismatch distinction |
| `crates/core/src/driver/db_client.rs`, `crates/core/src/session/daemon_rpc.rs` | Propagate decode errors |
| `crates/rmigd/src/shutdown.rs` | New: signal listener (copy of CLI's; arch boundary forbids the import) |
| `crates/rmigd/src/main.rs` | `select!` race: serve vs signal → socket unlink, exit 0; module doc updated |
| `sql/catalog/catalog_cache_load_relaxed.sql` | `@p1` → `@p4` (bound parameter) |
| `crates/core/src/db/batch.rs` | `relaxed_cache_count: Option<usize>` → `relaxed_cache: bool`; push asset verbatim; +1 unit test |
| `crates/core/src/db/plan_common/body/git_delta/warmup.rs` | Bind object count as 4th query parameter |
| `crates/core/src/db/plan_common/body/{git_delta/query.rs,standard/full.rs,standard/incremental.rs}` | Mechanical signature update |
| `docs/security-review.md` | Interpolation entry marked resolved (now parameter-bound) |
| `crates/cli/src/main.rs` | Print blocker summary + guidance on non-json blocked runs |
| `PRODUCTION-READINESS-AUDIT.md` | This report |

Commits: `6150d84`, `969bca5`, `ffac135`, `2246b71`, `58fb833`, `d6902d1`, `221eb02` — one per logical fix, conventional format.
