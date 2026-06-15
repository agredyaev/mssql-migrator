#!/usr/bin/env bash
# Static checks for ops/perf/prod_gate.sh DB reset credential wiring (BG-012).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SCRIPT="$ROOT/ops/perf/prod_gate.sh"

fail() {
  echo "check-prod-gate-reset: $*" >&2
  exit 1
}

RESET_BLOCK="$(
  awk '
    /RMIG_GATE_SKIP_DB_RESET/ { flag = 1 }
    flag { print }
    flag && /^fi$/ { exit }
  ' "$SCRIPT"
)"

[ -n "$RESET_BLOCK" ] || fail "could not locate DB reset block in $SCRIPT"

echo "$RESET_BLOCK" | grep -q '\$RM_DB_USER' \
  || fail "happy path: reset sqlcmd must use \$RM_DB_USER"

echo "$RESET_BLOCK" | grep -q '\$RM_DB_PASSWORD' \
  || fail "happy path: reset sqlcmd must use \$RM_DB_PASSWORD"

if echo "$RESET_BLOCK" | grep -qE "\-U sa -P 'yourStrong"; then
  fail "negative path: reset block must not hardcode sqlcmd -U/-P"
fi

echo "$RESET_BLOCK" | grep -q '\$RM_DB_SERVER' \
  || fail "edge case: reset sqlcmd must use \$RM_DB_SERVER"

if echo "$RESET_BLOCK" | grep -q "localhost -U sa -P 'yourStrong(!)Password'"; then
  fail "BG-012 regression: reset still uses hardcoded sa / docker default password"
fi

echo "check-prod-gate-reset: PASS"
