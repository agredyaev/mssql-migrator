# ADR-0019: Typed exit-code scheme for CI automation

Status: Accepted
Date: 2026-07-21

## Context

Runs in CI. A pipeline must react differently to config error vs connect failure
vs plan blocked vs lock contention — retry, alert, or fail. A single non-zero
exit code loses that signal.

## Decision

`Error` variants map to stable, documented process exit codes
(`crates/core/src/error.rs::exit_code`):

| Code | Meaning |
|------|---------|
| 0 | success (`EXIT_OK`) |
| 1 | uncategorized / I/O (`EXIT_GENERAL`) |
| 2 | config / invalid env (`EXIT_CONFIG`) |
| 3 | connect failure or connect timeout (`EXIT_CONN`) |
| 4 | undecodable persisted audit checksum → run `repair-checksum` (`EXIT_CHECKSUM`) |
| 5 | SQL error or query timeout (`EXIT_SQL`) |
| 7 | advisory-lock timeout / contention (`EXIT_LOCK_TIMEOUT`) |
| 8 | invalid input / bad repository structure or identifier (`EXIT_INVALID_INPUT`) |
| 10 | plan structurally blocked (`EXIT_PLAN_BLOCKED`) |
| 130 | interrupted (SIGINT/SIGTERM; ADR-0012) |

Codes 6 (`EXIT_VALIDATION`) and 9 (`EXIT_CRITICAL`) are reserved constants, not
currently returned. Documented in `docs/ci-usage.md`, `docs/runbook.md`.

## Consequences

- A pipeline can auto-recover: e.g. 3 (connect) is retryable, 10 (blocked) needs
  an author fix, 4 needs `repair-checksum`, 7 needs a serialized retry.
- The mapping is a contract; `crates/core/src/tests/error_test.rs` pins it.
- Coarse edges accepted: I/O and `Other` both collapse to 1.
