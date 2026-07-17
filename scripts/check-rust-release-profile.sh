#!/usr/bin/env bash
# Assert production release profile matches operator release-build expectations.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

CARGO_TOML="$ROOT/Cargo.toml"
fail=0

# Extract exactly one TOML table's body: keys found in UNRELATED tables must
# never satisfy this gate.
section() {
  awk -v s="[$1]" 'index($0, s) == 1 { f = 1; next } /^\[/ { f = 0 } f' "$CARGO_TOML"
}

DIST="$(section profile.release-dist)"
RELEASE="$(section profile.release)"

require() {
  local key="$1"
  local expected="$2"
  if ! printf '%s\n' "$DIST" | rg -q "^\s*${key}\s*=\s*${expected}\s*$"; then
    echo "release-profile: [profile.release-dist] must set ${key} = ${expected}" >&2
    fail=1
  fi
}

if [[ -z "$DIST" ]]; then
  echo "release-profile: missing or empty [profile.release-dist] in Cargo.toml" >&2
  fail=1
fi

if ! printf '%s\n' "$DIST" | rg -q '^\s*lto\s*=\s*("fat"|true)\s*$'; then
  echo "release-profile: [profile.release-dist] must set lto = true or lto = \"fat\"" >&2
  fail=1
fi

require strip true
require codegen-units 1
require debug false
require panic \"abort\"
require incremental false

if [[ -z "$RELEASE" ]]; then
  echo "release-profile: missing or empty [profile.release]" >&2
  fail=1
fi

if ! printf '%s\n' "$RELEASE" | rg -q '^\s*debug\s*=\s*true\s*$'; then
  echo "release-profile: [profile.release] should keep debug = true for flamegraphs" >&2
  fail=1
fi

if [[ "$fail" -ne 0 ]]; then
  exit 1
fi

echo "release-profile: OK (release-dist: fat LTO/strip/codegen-units=1; release: debug symbols)"
