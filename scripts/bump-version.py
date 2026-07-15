#!/usr/bin/env python3
import sys
import os
import re

def main():
    bump_type = "patch"
    if len(sys.argv) > 1:
        bump_type = sys.argv[1].lower()
        if bump_type not in ("major", "minor", "patch"):
            print("Usage: bump-version.py [major|minor|patch]", file=sys.stderr)
            sys.exit(1)

    repo_root = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
    version_path = os.path.join(repo_root, "VERSION")
    cargo_path = os.path.join(repo_root, "Cargo.toml")

    if not os.path.isfile(version_path):
        print(f"Error: VERSION file not found at {version_path}", file=sys.stderr)
        sys.exit(1)

    with open(version_path, "r", encoding="utf-8") as f:
        current_version = f.read().strip()

    print(f"Current version: {current_version}")

    match = re.match(r"^(\d+)\.(\d+)\.(\d+)(.*)$", current_version)
    if not match:
        print(f"Error: Version '{current_version}' is not valid semver.", file=sys.stderr)
        sys.exit(1)

    major = int(match.group(1))
    minor = int(match.group(2))
    patch = int(match.group(3))
    suffix = match.group(4)

    if bump_type == "major":
        major += 1
        minor = 0
        patch = 0
    elif bump_type == "minor":
        minor += 1
        patch = 0
    else:  # patch
        patch += 1

    new_version = f"{major}.{minor}.{patch}{suffix}"
    print(f"Bumping to: {new_version}")

    # Write new version to VERSION file
    with open(version_path, "w", encoding="utf-8") as f:
        f.write(new_version + "\n")

    # Update Cargo.toml
    if os.path.isfile(cargo_path):
        with open(cargo_path, "r", encoding="utf-8") as f:
            content = f.read()

        # We want to replace `version = "..."` under `[workspace.package]`
        pattern = r"(\[workspace\.package\]\s*\n(?:.*\n)*?version\s*=\s*\")[^\"]+(\")"
        new_content, count = re.subn(pattern, rf"\g<1>{new_version}\g<2>", content, count=1)
        if count > 0:
            with open(cargo_path, "w", encoding="utf-8") as f:
                f.write(new_content)
            print(f"Updated {cargo_path}")
        else:
            print(
                "Error: Could not find version inside [workspace.package] block in Cargo.toml.",
                file=sys.stderr,
            )
            sys.exit(1)

    print(f"Successfully bumped version to {new_version}")

if __name__ == "__main__":
    main()
