# ADR-0023: Code-structure discipline enforced by CI gates

Status: Accepted
Date: 2026-07-21

## Context

A migration tool touching production catalogs must stay reviewable and hard to
foot-gun. Large files, tangled cross-layer imports, `unwrap` panics on I/O, and
`unsafe` are the usual erosions. Reviewer diligence alone does not hold the line.

## Decision

Enforce structure with `make arch` and Cargo lints:

- **≤500 code lines per production `.rs`** (`scripts/check-rust-loc.sh`,
  excluding tests, blank lines, and comments).
- **Crate import boundaries** (`scripts/check-rust-arch.sh`): cli→config/engine/
  error/export only; rmigd→session/config/error; core lower layers (domain/
  export/scan/git) cannot pull upper layers. Also: megastruct cap (>12 `pub`
  fields fails, small allowlist); `#[allow(clippy::…)]` banned.
- **`#![forbid(unsafe_code)]`** in every crate; zero `unsafe` blocks.
- **`#![deny(clippy::unwrap_used, clippy::expect_used)]`** in non-test core.
- **Clippy `-D warnings`**, rustfmt `--check`, rustdoc `-D warnings`.
- **No inline SQL** (ADR-0001), release-profile/dependency gates, and doc gates.

## Consequences

- Cross-layer dependencies and swallowed clippy warnings cannot merge.
- No `unsafe`, no `unwrap`/`expect` panic on production paths (poisoned mutexes
  recover rather than panic).
- The 500-line ceiling catches genuinely oversized production modules without
  forcing one-function files to satisfy the former 100-line limit.
- Policy scripts are verified by their behavior and executable tests. CI does
  not keep shell scripts whose only purpose is checking another test manifest.
- CI is single-platform (ubuntu/amd64); toolchain pinned via `rust-toolchain.toml`
  = the CI SHA (ADR-0025).
