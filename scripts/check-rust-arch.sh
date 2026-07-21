#!/usr/bin/env bash
# Architecture guardrails for the Rust port:
# - crate responsibility boundaries (cli / rmigd must stay thin)
# - layer imports inside migrator-core (domain/export/scan)
# - megastructures: pub field count per struct (allowlist for wire/domain aggregates)
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
RUST="$ROOT/crates"
MAX_FIELDS=12
fail=0

err() { echo "arch: $*" >&2; fail=1; }

# --- Crate boundaries: binary crates may only touch allowed migrator-core surfaces ---
CLI_SRC="$RUST/cli/src"
RMIGD_SRC="$RUST/rmigd/src"

if [[ -d "$CLI_SRC" ]]; then
  while IFS= read -r line; do
    mod="${line#use migrator_core::}"
    mod="${mod%%::*}"
    case "$mod" in
      config|engine|error|export) ;;
      *)
        err "cli must not import migrator_core::$mod (allowed: config, engine, error, export): $line"
        ;;
    esac
  done < <(find "$CLI_SRC" -name '*.rs' -exec grep -hE '^use migrator_core::' {} + 2>/dev/null || true)
fi

if [[ -f "$RMIGD_SRC/main.rs" ]]; then
  if find "$RMIGD_SRC" -name '*.rs' -exec grep -hE '^use migrator_core::' {} + 2>/dev/null \
      | grep -Ev '^use migrator_core::(session|config|error)' | grep -q .; then
    err "rmigd must only import migrator_core::session/config/error"
  fi
  if ! grep -q 'migrator_core::session::run_daemon' "$RMIGD_SRC/main.rs"; then
    err "rmigd main must call migrator_core::session::run_daemon"
  fi
fi

# --- migrator-core layers: lower modules must not pull upper layers ---
CORE_SRC="$RUST/core/src"

check_layer() {
  local dir="$1"
  shift
  local forbidden=("$@")
  [[ -d "$dir" ]] || return 0
  for mod in "${forbidden[@]}"; do
    while IFS= read -r hit; do
      [[ -z "$hit" ]] && continue
      local file="${hit%%:*}"
      local rest="${hit#*:}"
      local line="${rest%%:*}"
      local content="${rest#*:}"
      if [[ "$content" =~ ^[[:space:]]*(//|/\*|\*) ]]; then
        continue
      fi
      err "layer violation: $(realpath --relative-to="$ROOT" "$dir" 2>/dev/null || echo "$dir") imports crate::$mod"
      echo "$hit" >&2
    done < <(rg -n "crate::${mod}(::|\s|;)" "$dir" 2>/dev/null || true)
  done
}

check_layer "$CORE_SRC/domain" driver db apply engine scan plan gate lock audit session export
check_layer "$CORE_SRC/export" driver db apply engine scan plan gate lock audit session
check_layer "$CORE_SRC/scan" apply db driver engine plan gate lock audit session export
check_layer "$CORE_SRC/git" apply db driver engine scan plan gate lock audit session export

# --- Megastructures: count `pub` fields per struct (exclude tests) ---
ALLOWED_MEGA=(
  Config
  PlannedObject
  PhaseTimings
  ObjectEntry
  Script
  Workspace
  WorkspaceCold
  MigrationPlan
  PlanSnapshot
  GateInput
)

python3 - "$CORE_SRC" "$MAX_FIELDS" "${ALLOWED_MEGA[@]}" <<'PY'
import re
import sys
from pathlib import Path

core_src = Path(sys.argv[1])
max_fields = int(sys.argv[2])
allowed = set(sys.argv[3:])

struct_re = re.compile(r"^pub struct (\w+)")
field_re = re.compile(r"^\s+pub \w+")

violations = []
for path in sorted(core_src.rglob("*.rs")):
    if "/tests/" in str(path):
        continue
    text = path.read_text().splitlines()
    i = 0
    while i < len(text):
        m = struct_re.match(text[i])
        if not m:
            i += 1
            continue
        name = m.group(1)
        depth = 0
        fields = 0
        i += 1
        while i < len(text):
            line = text[i]
            if line.strip().startswith("pub struct ") and depth == 0:
                break
            depth += line.count("{") - line.count("}")
            if depth <= 1 and field_re.match(line):
                fields += 1
            if depth < 0:
                break
            i += 1
        if fields > max_fields and name not in allowed:
            violations.append((path, name, fields))

for path, name, n in violations:
    print(f"MEGA {n} fields > {max_fields}: {path} struct {name}")
    print(f"  allowlist or split into smaller types (allowed: {', '.join(sorted(allowed))})")

sys.exit(1 if violations else 0)
PY

# --- No clippy suppressions: use `cargo clippy -- -D warnings` instead ---
while IFS= read -r hit; do
  err "forbidden #[allow(clippy::...)] (fix or refactor): $hit"
done < <(grep -rn 'allow(clippy::' "$RUST" --include='*.rs' 2>/dev/null || true)

if [[ $fail -ne 0 ]]; then
  echo "check-rust-arch: failed" >&2
  exit 1
fi
echo "check-rust-arch: ok"
