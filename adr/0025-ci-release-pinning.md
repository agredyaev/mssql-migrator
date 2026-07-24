# ADR-0025: CI/CD — pinned actions/toolchain, release only from CI-validated SHA

Status: Accepted
Date: 2026-07-21

## Context

Supply-chain and reproducibility risks in CI/CD: a mutable action tag can change
under you; an unpinned toolchain drifts clippy/fmt results; releasing a commit
that CI never validated ships untested code; a flaky release job that partially
ran leaves a half-published tag.

## Decision

- **SHA-pin every GitHub Action** (`.github/workflows/*`,
  `.github/actions/setup`): e.g. `actions/checkout@34e1148… # v4`,
  `dtolnay/rust-toolchain@c0e9df8… # 1.96.0`. Tag comment is documentation; the
  SHA is the pin.
- **Pin the toolchain** to the CI SHA (1.96.0) and mirror it locally via
  `rust-toolchain.toml` so local fmt/clippy match CI (ADR-0023).
- **Release only from a CI-validated SHA**: `release.yml` triggers on
  `workflow_run` after CI success on `main` (push only), or manual dispatch that
  refuses unless `gh run list` shows a green CI for the commit. It releases the
  exact validated SHA.
- **Resumable, race-safe release** (commit `2211b34`): re-running is idempotent
  (skip if the tag already points at HEAD and a GH Release exists); atomic
  branch+tag push so a loser of a race fails non-ff instead of dropping a release;
  docs-only diffs skip the release.
- Version source of truth: `[workspace.package].version` in `Cargo.toml`.
  Cargo exposes it as `CARGO_PKG_VERSION`; `build.rs` stamps only `RMIG_COMMIT`.

## Consequences

- A mutated upstream tag cannot change the build; local and CI lint results agree.
- No release ships a commit CI did not validate; a retried/partial release does
  not corrupt or duplicate a published version.
- Single-platform CI (ubuntu/amd64), amd64-only DB fixture — recorded limitation
  (`PRODUCTION-READINESS-AUDIT.md` §13). `cargo deny` runs in CI (lint stage) +
  local `make deny`.
- Local incremental builds can carry a stale `RMIG_COMMIT` until
  `migrator-core` rebuilds; CI release builds compile the validated checkout.
