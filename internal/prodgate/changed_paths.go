package prodgate

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ChangedPathsResult is the outcome of automatic git delta resolution.
type ChangedPathsResult struct {
	Paths       []string
	RepoRoot    string
	Source      string // e.g. "ci-github", "git-merge-base", "full-inspect"
	FullInspect bool   // when true, caller should inspect entire layout (git unavailable or forced)
}

// ResolveChangedPaths discovers changed SQL paths without manual RMIG_* env (prod default).
// Override env (tests only): RMIG_GATE_CHANGED_FILES, RMIG_GATE_GIT_BASE.
func ResolveChangedPaths(sqlRoot string) (ChangedPathsResult, error) {
	if os.Getenv("RMIG_INSPECT_FULL") == "1" {
		return ChangedPathsResult{Source: "env-full-inspect", FullInspect: true}, nil
	}
	if paths := changedPathsFromOverrideEnv(); len(paths) > 0 {
		root, _ := FindRepoRoot(sqlRoot)
		return ChangedPathsResult{Paths: paths, RepoRoot: root, Source: "env-changed-files"}, nil
	}
	if base := strings.TrimSpace(os.Getenv("RMIG_GATE_GIT_BASE")); base != "" {
		root, ok := FindRepoRoot(sqlRoot)
		if !ok {
			return ChangedPathsResult{}, fmt.Errorf("RMIG_GATE_GIT_BASE set but no .git found from %s", sqlRoot)
		}
		paths, err := ChangedPathsFromGit(root, base)
		if err != nil {
			return ChangedPathsResult{}, err
		}
		return ChangedPathsResult{Paths: paths, RepoRoot: root, Source: "env-git-base"}, nil
	}

	root, ok := FindRepoRoot(sqlRoot)
	if !ok {
		return ChangedPathsResult{Source: "no-git", FullInspect: true}, nil
	}

	if paths, source, ok := changedPathsFromCI(root); ok {
		return ChangedPathsResult{Paths: paths, RepoRoot: root, Source: source}, nil
	}

	if paths, err := changedPathsMergeBase(root); err == nil {
		return ChangedPathsResult{Paths: paths, RepoRoot: root, Source: "git-merge-base"}, nil
	}

	// Last resort: diff against HEAD (may be empty on clean tree).
	paths, err := ChangedPathsFromGit(root, "HEAD")
	if err != nil {
		return ChangedPathsResult{RepoRoot: root, Source: "git-failed", FullInspect: true}, nil
	}
	return ChangedPathsResult{Paths: paths, RepoRoot: root, Source: "git-head"}, nil
}

func changedPathsFromOverrideEnv() []string {
	if p := ChangedPathsFromEnv(); len(p) > 0 {
		return p
	}
	raw := strings.TrimSpace(os.Getenv("RMIG_CHANGED_FILES"))
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

func changedPathsFromCI(repoRoot string) ([]string, string, bool) {
	if paths, ok := changedPathsGitHubPR(repoRoot); ok {
		return paths, "ci-github-pr", true
	}
	if paths, ok := changedPathsGitLabMR(repoRoot); ok {
		return paths, "ci-gitlab-mr", true
	}
	if paths, ok := changedPathsAzureDevOpsPR(repoRoot); ok {
		return paths, "ci-ado-pr", true
	}
	return nil, "", false
}

func changedPathsGitHubPR(repoRoot string) ([]string, bool) {
	if os.Getenv("GITHUB_EVENT_NAME") != "pull_request" {
		return nil, false
	}
	eventPath := os.Getenv("GITHUB_EVENT_PATH")
	if eventPath == "" {
		return nil, false
	}
	data, err := os.ReadFile(eventPath)
	if err != nil {
		return nil, false
	}
	var payload struct {
		PullRequest struct {
			Base struct {
				SHA string `json:"sha"`
			} `json:"base"`
		} `json:"pull_request"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, false
	}
	baseSHA := strings.TrimSpace(payload.PullRequest.Base.SHA)
	if baseSHA == "" {
		return nil, false
	}
	paths, err := gitDiffNameOnly(repoRoot, baseSHA, "HEAD")
	if err != nil {
		return nil, false
	}
	return paths, true
}

func changedPathsGitLabMR(repoRoot string) ([]string, bool) {
	if os.Getenv("CI_MERGE_REQUEST_IID") == "" {
		return nil, false
	}
	base := strings.TrimSpace(os.Getenv("CI_MERGE_REQUEST_DIFF_BASE_SHA"))
	head := strings.TrimSpace(os.Getenv("CI_COMMIT_SHA"))
	if base == "" || head == "" {
		return nil, false
	}
	paths, err := gitDiffNameOnly(repoRoot, base, head)
	if err != nil {
		return nil, false
	}
	return paths, true
}

func changedPathsAzureDevOpsPR(repoRoot string) ([]string, bool) {
	target := strings.TrimSpace(os.Getenv("SYSTEM_PULLREQUEST_TARGETBRANCH"))
	if target == "" && os.Getenv("BUILD_REASON") != "PullRequest" {
		return nil, false
	}
	if target == "" {
		return nil, false
	}
	target = strings.TrimPrefix(target, "refs/heads/")
	remote := "origin/" + target
	_ = exec.Command("git", "-C", repoRoot, "fetch", "origin", target+":"+target, "--depth=1").Run()
	base, err := gitMergeBase(repoRoot, "HEAD", remote)
	if err != nil {
		paths, err2 := ChangedPathsFromGit(repoRoot, remote)
		if err2 != nil {
			return nil, false
		}
		return paths, true
	}
	paths, err := gitDiffNameOnly(repoRoot, base, "HEAD")
	if err != nil {
		return nil, false
	}
	return paths, true
}

func changedPathsMergeBase(repoRoot string) ([]string, error) {
	for _, remote := range []string{"origin/main", "origin/master", "main", "master"} {
		base, err := gitMergeBase(repoRoot, "HEAD", remote)
		if err != nil {
			continue
		}
		return gitDiffNameOnly(repoRoot, base, "HEAD")
	}
	return nil, fmt.Errorf("no merge-base with origin/main or origin/master")
}

func gitMergeBase(repoRoot, head, ref string) (string, error) {
	cmd := exec.Command("git", "-C", repoRoot, "merge-base", head, ref)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func gitDiffNameOnly(repoRoot, base, head string) ([]string, error) {
	cmd := exec.Command("git", "-C", repoRoot, "diff", "--name-only", base, head)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git diff --name-only %s %s: %w", base, head, err)
	}
	var paths []string
	for line := range strings.SplitSeq(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			paths = append(paths, filepath.ToSlash(line))
		}
	}
	return paths, nil
}
