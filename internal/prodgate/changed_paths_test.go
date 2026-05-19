package prodgate

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestResolveChangedPaths_NoGitFullInspect(t *testing.T) {
	dir := t.TempDir()
	res, err := ResolveChangedPaths(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !res.FullInspect {
		t.Fatalf("expected FullInspect without .git, got source=%q", res.Source)
	}
}

func TestResolveChangedPaths_GitDiff(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	writeFile(t, filepath.Join(dir, "a.sql"), "v1\n")
	commitAll(t, dir, "c1")
	if out, err := exec.Command("git", "-C", dir, "branch", "-M", "main").CombinedOutput(); err != nil {
		t.Fatalf("git branch -M main: %v\n%s", err, out)
	}
	if out, err := exec.Command("git", "-C", dir, "checkout", "-b", "feature").CombinedOutput(); err != nil {
		t.Fatalf("git checkout -b feature: %v\n%s", err, out)
	}
	writeFile(t, filepath.Join(dir, "b.sql"), "v2\n")
	commitAll(t, dir, "c2")

	res, err := ResolveChangedPaths(dir)
	if err != nil {
		t.Fatal(err)
	}
	if res.FullInspect {
		t.Fatalf("unexpected full inspect, source=%q", res.Source)
	}
	found := false
	for _, p := range res.Paths {
		if p == "b.sql" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected b.sql in changed paths, got %v (source=%s)", res.Paths, res.Source)
	}
}

func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	for _, args := range [][]string{
		{"init"},
		{"config", "user.email", "t@t.com"},
		{"config", "user.name", "t"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
}

func commitAll(t *testing.T, dir, msg string) {
	t.Helper()
	if out, err := exec.Command("git", "-C", dir, "add", ".").CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, out)
	}
	if out, err := exec.Command("git", "-C", dir, "commit", "-m", msg).CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v\n%s", err, out)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
