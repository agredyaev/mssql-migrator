#!/usr/bin/env bash
# Static checks: e2e scripts must use RM_SKIP_GIT (not RMIG_SKIP_GIT) per config/env.rs (BG-013).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"

fail() {
  echo "check-e2e-git-flag: $*" >&2
  exit 1
}

E2E_SH="$ROOT/ops/perf/e2e.sh"
E2E_ALL="$ROOT/ops/perf/e2e_all.sh"

grep -q 'RM_SKIP_GIT' "$E2E_SH" \
  || fail "happy path: e2e.sh must export RM_SKIP_GIT"

grep -q 'RM_SKIP_GIT' "$E2E_ALL" \
  || fail "happy path: e2e_all.sh must export RM_SKIP_GIT"

if grep -q 'RMIG_SKIP_GIT' "$E2E_SH" "$E2E_ALL"; then
  fail "negative path: e2e scripts must not reference RMIG_SKIP_GIT"
fi

grep -q 'unset RM_SKIP_GIT' "$E2E_ALL" \
  || fail "edge case: e2e_all.sh must unset RM_SKIP_GIT for git-delta scenario"

if grep -q 'unset RMIG_SKIP_GIT\|export RMIG_SKIP_GIT' "$E2E_ALL"; then
  fail "BG-013 regression: e2e_all.sh still toggles RMIG_SKIP_GIT instead of RM_SKIP_GIT"
fi

echo "check-e2e-git-flag: PASS"
