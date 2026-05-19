package prodgate

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"reporting-db-migrations/internal/fs"
)

// KeysForChangedPaths maps repository-relative SQL paths to normalized object keys
// using layout path indexes. Transition paths add their own key; parent object keys
// are not inferred automatically (strict delta = explicit changed files only).
func KeysForChangedPaths(layout fs.Layout, changedPaths []string) map[string]struct{} {
	keys := make(map[string]struct{})
	if len(changedPaths) == 0 {
		return keys
	}
	objByPath := layout.ObjectsByPath()
	transByPath := layout.TransitionsByPath()
	for _, raw := range changedPaths {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		p := filepath.ToSlash(raw)
		if obj, ok := objByPath[p]; ok && obj != nil {
			keys[obj.NormalizedKey] = struct{}{}
			continue
		}
		if ts, ok := transByPath[p]; ok && ts != nil {
			keys[ts.NormalizedKey] = struct{}{}
			continue
		}
		// Suffix match for paths passed as absolute or with different prefix.
		for path, obj := range objByPath {
			if strings.HasSuffix(path, p) || strings.HasSuffix(p, path) {
				keys[obj.NormalizedKey] = struct{}{}
			}
		}
		for path, ts := range transByPath {
			if strings.HasSuffix(path, p) || strings.HasSuffix(p, path) {
				keys[ts.NormalizedKey] = struct{}{}
			}
		}
	}
	return keys
}

// ChangedPathsFromGit returns paths changed between baseRef and HEAD under repoRoot.
func ChangedPathsFromGit(repoRoot, baseRef string) ([]string, error) {
	if baseRef == "" {
		baseRef = "HEAD"
	}
	cmd := exec.Command("git", "diff", "--name-only", baseRef, "HEAD")
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git diff --name-only %s HEAD: %w", baseRef, err)
	}
	var paths []string
	for line := range strings.SplitSeq(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			paths = append(paths, line)
		}
	}
	return paths, nil
}

// ChangedPathsFromEnv reads RMIG_GATE_CHANGED_FILES (comma-separated repo-relative paths).
func ChangedPathsFromEnv() []string {
	raw := strings.TrimSpace(os.Getenv("RMIG_GATE_CHANGED_FILES"))
	if raw == "" {
		return nil
	}
	var paths []string
	for part := range strings.SplitSeq(raw, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			paths = append(paths, part)
		}
	}
	return paths
}
