#!/usr/bin/env bash
set -euo pipefail

root="${1:-.}"
output="${2:-code_dump.txt}"

python3 - "$root" "$output" <<'PY'
import os
import sys

root = os.path.abspath(sys.argv[1])
output = os.path.abspath(sys.argv[2])

skip_dirs = {'.git', '.idea', '.vscode', '.tmp', 'docs', 'scripts', 'tools'}
skip_exts = {'.png', '.jpg', '.jpeg', '.gif', '.pdf', '.exe', '.dll', '.so', '.dylib'}

def rel(path):
    return os.path.relpath(path, root).replace(os.sep, '/')

def is_binary(path):
    ext = os.path.splitext(path)[1].lower()
    if ext in skip_exts:
        return True
    try:
        with open(path, 'rb') as f:
            return b'\0' in f.read(4096)
    except OSError:
        return True

files = []

for name in ('go.mod', 'go.sum'):
    path = os.path.join(root, name)
    if os.path.isfile(path) and os.path.abspath(path) != output:
        files.append(path)

for top in ('cmd', 'internal'):
    abs_top = os.path.join(root, top)
    if not os.path.isdir(abs_top):
        continue

    for current, dirs, names in os.walk(abs_top):
        dirs[:] = [d for d in sorted(dirs) if d not in skip_dirs]

        for name in sorted(names):
            path = os.path.join(current, name)

            if os.path.abspath(path) == output:
                continue
            if name.endswith('.md'):
                continue
            if is_binary(path):
                continue

            files.append(path)

with open(output, 'w', encoding='utf-8', newline='') as out:
    out.write('=== TREE ===\n')

    dirs_out = set()
    for path in files:
        parts = rel(path).split('/')[:-1]
        for i in range(1, len(parts) + 1):
            dirs_out.add('/'.join(parts[:i]) + '/')

    for entry in sorted(dirs_out):
        out.write(entry + '\n')

    out.write('\n=== FILES ===\n')

    for path in sorted(files, key=lambda p: rel(p).lower()):
        out.write(f'\n=== FILE: {rel(path)} ===\n')
        with open(path, 'r', encoding='utf-8', newline='') as src:
            out.write(src.read())
        out.write('\n=== END FILE ===\n')

print(output)
PY
