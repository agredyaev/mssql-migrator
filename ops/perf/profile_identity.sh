#!/usr/bin/env bash

rmig_profile_identity() {
  local root="$1" revision version target dirty
  revision="$(git -C "$root" rev-parse HEAD 2>/dev/null || echo unknown)"
  version="$(tr -d '[:space:]' < "$root/VERSION")"
  target="$(rustc -vV | awk '$1 == "host:" { print $2 }')"
  dirty=false
  if ! git -C "$root" diff --quiet -- . ':(exclude)ops/perf/artifacts/**'; then
    dirty=true
  fi
  printf 'rmig-profile revision=%s version=%s target=%s dirty=%s\n' \
    "$revision" "$version" "$target" "$dirty"
}
