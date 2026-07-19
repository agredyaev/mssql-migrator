#!/bin/bash
# Head-of-line probe: does the single warm rmigd session hold up under
# concurrent clients, and does it still beat direct (cold-connect) runs?
#
# For each N in CLIENTS, runs N concurrent `rmig plan` invocations two ways:
#   daemon — through one rmigd (serialized on the single warm session)
#   direct — each process opens its own cold TDS connection
# Prints per-mode p50/p95/max wall seconds and failure counts, plus how many
# head-of-line "queued for warm session" warnings rmigd logged.
#
# Usage: ROOT must be the repo root; requires the docker MSSQL fixture.
#   ./ops/perf/hol_probe.sh            # N = 1 2 4 8
#   CLIENTS="4 16" ./ops/perf/hol_probe.sh
set -u
cd "$(dirname "$0")/../.."
export ROOT="$PWD"
set -a; . ops/perf/e2e_env.sh; set +a
export RMIG_SESSION_TOKEN="${RMIG_SESSION_TOKEN:-rmig-integration-test-token}"
RMIG=target/release/rmig
RMIGD=target/release/rmigd
CLIENTS="${CLIENTS:-1 2 4 8}"
OUT="$(mktemp -d)"
trap 'rm -rf "$OUT"' EXIT

timed_plan() { # mode idx
  python3 - "$1" "$2" "$OUT" <<'PY'
import subprocess, sys, time, os
mode, idx, out = sys.argv[1], sys.argv[2], sys.argv[3]
env = dict(os.environ)
env["RMIG_USE_RMIGD"] = "1" if mode == "daemon" else "0"
t = time.time()
r = subprocess.run(["target/release/rmig", "--config", "config.toml", "plan"],
                   stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, env=env)
with open(f"{out}/{mode}-{idx}", "w") as f:
    f.write(f"{time.time()-t:.3f} {r.returncode}\n")
PY
}

stats() { # mode n
  python3 - "$1" "$2" "$OUT" <<'PY'
import glob, sys
mode, n, out = sys.argv[1], int(sys.argv[2]), sys.argv[3]
walls, fails = [], 0
for f in glob.glob(f"{out}/{mode}-*"):
    w, rc = open(f).read().split()
    walls.append(float(w))
    fails += rc != "0"
walls.sort()
p = lambda q: walls[min(len(walls)-1, int(q*len(walls)))]
print(f"{mode:6} N={n}: p50={p(0.5):.2f}s p95={p(0.95):.2f}s max={walls[-1]:.2f}s fails={fails}/{n}")
sys.exit(1 if fails else 0)
PY
}

run_mode() { # mode n
  rm -f "$OUT"/"$1"-*
  local pids=()
  for i in $(seq 1 "$2"); do
    timed_plan "$1" "$i" &
    pids+=("$!")
  done
  # Wait only the client pids: a bare `wait` would also wait on rmigd.
  wait "${pids[@]}"
  stats "$1" "$2" || FAILED=1
}

FAILED=0
rm -f "$HOME/.rmig/rmigd.sock"
"$RMIGD" 2> "$OUT/rmigd.err" &
DPID=$!
sleep 2
kill -0 "$DPID" 2>/dev/null || { echo "FATAL: rmigd failed to start"; cat "$OUT/rmigd.err"; exit 1; }
RMIG_USE_RMIGD=1 "$RMIG" --config config.toml plan >/dev/null 2>&1  # warm it

for n in $CLIENTS; do
  run_mode daemon "$n"
  run_mode direct "$n"
done

kill -TERM "$DPID" 2>/dev/null; wait "$DPID" 2>/dev/null
QUEUED=$(grep -c "queued for warm session" "$OUT/rmigd.err" || true)
echo "rmigd head-of-line warnings (>100ms queue): $QUEUED"
[ "$FAILED" -eq 0 ] && echo "hol_probe: PASS" || { echo "hol_probe: FAIL"; exit 1; }
