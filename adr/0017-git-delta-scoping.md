# ADR-0017: git-delta scoping of live-catalog inspection

Status: Accepted
Date: 2026-07-21

## Context

The live-catalog inspection (query `sys.objects`/`columns`/`indexes` to compare
against repo) is O(inspected objects). Inspecting the whole catalog on every plan
is wasteful when git says only a few files changed since the base branch.

## Decision

Use git to scope the live-catalog inspection to objects that could have changed.
`build_inspect_scope` (`crates/core/src/plan/scope_build.rs`) partitions objects
using checksums (not bodies):
- **hot** (need live DDL inspection): modules always; objects in the git
  changed-path delta closure; objects whose file checksum ≠ history checksum.
- **stable** (unchanged): file checksum == history checksum → catalog entry
  synthesized from key/kind/parent, no SQL lookup; merged via
  `merge_stable_catalog`.

Changed paths come from `git diff` since the merge-base with the main branch
(`crates/core/src/git/diff.rs::merge_base_paths`, tries `origin/main`,
`origin/master`, `main`, `master`). `RM_SKIP_GIT` disables git-delta → full
inspection.

## Consequences

- The expensive live-catalog DDL inspection scales with the change set, not the
  catalog, on a git checkout. Common CI re-plan inspects near-zero objects.
- git-delta scopes ONLY catalog inspection. It does NOT scope the checksum load
  (all keys, ADR-0011) nor — before ADR-0005 — the drift fingerprint. ADR-0005
  made the fingerprint incremental by a separate mechanism (object_ddl), because
  git changes cannot reveal out-of-band DB drift.
- git ref hardening: refs beginning with `-` rejected before invoking git
  (argument-injection guard, `git/diff.rs`).
- CI must fetch enough history for the merge-base (`docs/ci-checkout.md`).
