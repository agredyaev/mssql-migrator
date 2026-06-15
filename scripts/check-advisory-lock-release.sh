#!/usr/bin/env bash
# apply_run must release advisory lock even when apply body fails (BG-001).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"

fail() {
  echo "check-advisory-lock-release: $*" >&2
  exit 1
}

LOCK="$ROOT/crates/core/src/lock/mod.rs"
APPLY="$ROOT/crates/core/src/engine/apply_run.rs"

grep -q 'release_after_body' "$LOCK" \
  || fail "happy path: lock module must define release_after_body"

grep -q 'release_after_body' "$APPLY" \
  || fail "happy path: apply_run must call release_after_body"

if grep -q 'execute_plan.*await\?;' "$APPLY" && grep -q 'lock::release' "$APPLY"; then
  fail "negative path: apply_run must not call lock::release only on success path"
fi

grep -q 'body_result' "$APPLY" \
  || fail "edge case: apply_run must finish body before release"

if ! grep -q 'release(conn)' "$LOCK" || ! grep -q 'body_result' "$LOCK"; then
  fail "BG-001 regression: release_after_body must always attempt release after body"
fi

echo "check-advisory-lock-release: PASS"
