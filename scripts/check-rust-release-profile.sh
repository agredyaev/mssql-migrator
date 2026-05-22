#!/usr/bin/env bash
# Assert production release profile matches operator release-build expectations.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

CARGO_TOML="$ROOT/Cargo.toml"
fail=0

require() {
  local key="$1"
  local expected="$2"
  if ! rg -q "^\s*${key}\s*=\s*${expected}\s*$" "$CARGO_TOML"; then
    echo "release-profile: [profile.release-dist] must set ${key} = ${expected}" >&2
    fail=1
  fi
}

if ! rg -q '^\[profile\.release-dist\]' "$CARGO_TOML"; then
  echo "release-profile: missing [profile.release-dist] in Cargo.toml" >&2
  fail=1
fi

if ! rg -q '^\s*lto\s*=\s*("fat"|true)\s*$' "$CARGO_TOML"; then
  echo "release-profile: [profile.release-dist] must set lto = true or lto = \"fat\"" >&2
  fail=1
fi

require strip true
require codegen-units 1
require debug false
require panic \"abort\"
require incremental false

if ! rg -q '^\[profile\.release\]' "$CARGO_TOML"; then
  echo "release-profile: missing [profile.release]" >&2
  fail=1
fi

if ! rg -q '^\s*debug\s*=\s*true\s*$' "$CARGO_TOML"; then
  echo "release-profile: [profile.release] should keep debug = true for flamegraphs" >&2
  fail=1
fi

if [[ "$fail" -ne 0 ]]; then
  exit 1
fi

echo "release-profile: OK (release-dist: fat LTO/strip/codegen-units=1; release: debug symbols)"
