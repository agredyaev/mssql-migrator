# ADR-0023: Code-structure discipline enforced by CI gates

Status: Accepted
Date: 2026-07-21

## Context

A migration tool touching production catalogs must stay reviewable and hard to
foot-gun. Large files, tangled cross-layer imports, `unwrap` panics on I/O, and
`unsafe` are the usual erosions. Reviewer diligence alone does not hold the line.

## Decision

Enforce structure with CI gates (`make arch` + Cargo lints), failing the build:

- **≤100 code lines per non-test `.rs`** (`scripts/check-rust-loc.sh`, non-blank
  non-comment). Forces small, single-purpose modules.
- **Crate import boundaries** (`scripts/check-rust-arch.sh`): cli→config/engine/
  error/export only; rmigd→session/config/error; core lower layers (domain/
  export/scan/git) cannot pull upper layers. Also: megastruct cap (>12 `pub`
  fields fails, small allowlist); `#[allow(clippy::…)]` banned.
- **`#![forbid(unsafe_code)]`** in every crate; zero `unsafe` blocks.
- **`#![deny(clippy::unwrap_used, clippy::expect_used)]`** in non-test core.
- **Clippy `-D warnings`**, rustfmt `--check`, rustdoc `-D warnings`.
- **No inline SQL** (ADR-0001), release-profile/deps gates, doc gates.

Gate scripts have their own regression self-tests (`make script-tests`) — the
gate has a gate.

## Consequences

- Every file is small and layer-clean; a cross-layer dependency or a swallowed
  clippy warning cannot merge.
- No `unsafe`, no `unwrap`/`expect` panic on production paths (poisoned mutexes
  recover rather than panic; arena "can't-happen" asserts are the only panics,
  documented).
- Cost: features sometimes need extraction to fit 100 lines (e.g. ADR-0009 moved
  `acquire_session` out of `serve_loop`). Accepted — keeps modules honest.
- CI is single-platform (ubuntu/amd64); toolchain pinned via `rust-toolchain.toml`
  = the CI SHA (ADR-0025).
