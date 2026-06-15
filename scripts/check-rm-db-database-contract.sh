#!/usr/bin/env bash
# Docs must not claim RM_DB_DATABASE is an rmig runtime config input (BG-014).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"

fail() {
  echo "check-rm-db-database-contract: $*" >&2
  exit 1
}

OPS="$ROOT/docs/operational-contract.md"
RUNBOOK="$ROOT/docs/runbook.md"
SPEC="$ROOT/docs/specs/rust/module-config-export.md"

grep -q 'RM_SQL_ROOT' "$OPS" \
  || fail "happy path: operational-contract must document RM_SQL_ROOT"

grep -q 'not passed via `RM_DB_DATABASE`' "$SPEC" \
  || fail "happy path: module-config-export must state RM_DB_DATABASE is ignored"

if grep -q 'RM_DB_DATABASE.*Sourced in config' "$OPS"; then
  fail "negative path: operational-contract must not list RM_DB_DATABASE as rmig config input"
fi

grep -q 'does not override `rmig`' "$RUNBOOK" \
  || fail "edge case: runbook must state RM_DB_DATABASE does not override rmig"

grep -q 'Shell helpers' "$OPS" \
  || fail "BG-014 regression: operational-contract must document shell-only RM_DB_DATABASE"

echo "check-rm-db-database-contract: PASS"
